package service

// v2_list_create.go implements POST sets and POST collections (APIV2.md §2
// Phase 2). Sets follow §8/R10: ObjectCreateSet takes no filters, so the set
// is built as one AnyBlock document whose initial state carries a fully-
// formed dataview block — one change set, honestly atomic — reusing the
// generic create path. Collections use the AnyBlock items import path.

import (
	"context"
	"encoding/json"
	"fmt"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// dataviewBlockId is the block id the set/collection editors expect their
// dataview under (template.DataviewBlockId) — a fresh id would make the
// editor add a second, default dataview at first open.
const dataviewBlockId = "dataview"

// CreateSet implements POST /v2/spaces/{spaceId}/sets.
func (s *V2Service) CreateSet(ctx context.Context, spaceId string, req apimodel.V2CreateSetRequest, dryRun bool) (*apimodel.V2CreateResult, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, err
	}
	if req.Name == "" {
		return nil, apimodel.V2ValidationFailed("name is required",
			apimodel.V2Issue{Path: "/name", Message: "a set needs a name"})
	}
	if req.Type == "" {
		return nil, apimodel.V2ValidationFailed("type is required",
			apimodel.V2Issue{Path: "/type", Message: "a set queries one type — name its key", Hint: fmt.Sprintf("list keys with GET /v2/spaces/%s/types", spaceId)})
	}
	// C6: filter and filters are mutually exclusive; both → ambiguous_input
	if req.Filter != "" && len(req.Filters) > 0 {
		return nil, apimodel.V2AmbiguousInput("provide filter or filters, not both",
			apimodel.V2Issue{Path: "/filter", Message: "conflicts with filters"},
			apimodel.V2Issue{Path: "/filters", Message: "conflicts with filter"})
	}
	if req.Filter != "" {
		// the compact filter string is the Phase-4 parser build item
		return nil, apimodel.NewV2Error(501, apimodel.V2CodeNotImplemented,
			"the compact filter string is not implemented yet — use the structured filters array (SPEC §6.2)")
	}
	if len(req.Views) > 0 && (len(req.Filters) > 0 || len(req.Sorts) > 0) {
		return nil, apimodel.V2AmbiguousInput("provide views or top-level filters/sorts, not both",
			apimodel.V2Issue{Path: "/views", Message: "views carry their own filters and sorts"})
	}

	// the queried type must exist in the space — its property keys are the
	// R9 reference set for the filters
	typeId, ok := s.typeIdInSpace(spaceId, req.Type)
	if !ok {
		return nil, s.unknownTypeKeyError(spaceId, req.Type, "/type")
	}

	// R9 referential validation: every property key the view addresses must
	// be one the type actually recommends
	referenced, err := collectViewPropertyKeys(req)
	if err != nil {
		return nil, err
	}
	if err := s.validateViewKeys(spaceId, typeId, req.Type, referenced); err != nil {
		return nil, err
	}

	doc, err := s.buildSetDocument(spaceId, typeId, req, referenced)
	if err != nil {
		return nil, err
	}
	return s.createFromDocument(ctx, spaceId, doc, docCreateOptions{dryRun: dryRun})
}

// CreateCollection implements POST /v2/spaces/{spaceId}/collections: the
// AnyBlock items import path builds the collection store.
func (s *V2Service) CreateCollection(ctx context.Context, spaceId string, req apimodel.V2CreateCollectionRequest, dryRun bool) (*apimodel.V2CreateResult, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, err
	}
	if req.Name == "" {
		return nil, apimodel.V2ValidationFailed("name is required",
			apimodel.V2Issue{Path: "/name", Message: "a collection needs a name"})
	}

	// referential validation: items must be existing objects in the space
	var issues []apimodel.V2Issue
	index := s.store.SpaceIndex(spaceId)
	for i, itemId := range req.Items {
		details, err := index.GetDetails(itemId)
		if err != nil || details.GetString(bundle.RelationKeyId) == "" {
			issues = append(issues, apimodel.V2Issue{
				Path:    fmt.Sprintf("/items/%d", i),
				Message: fmt.Sprintf("object %q not found in space %q", itemId, spaceId),
				Hint:    "items are full object ids — find them with GET /v2/spaces/{spaceId}/objects",
			})
		}
	}
	if len(issues) > 0 {
		return nil, apimodel.V2ValidationFailed("unknown collection items", issues...)
	}

	fields := map[string]json.RawMessage{}
	var err error
	if fields["version"], err = rawJSON(anyblockjson.FormatVersion); err != nil {
		return nil, err
	}
	if fields["type"], err = rawJSON(string(bundle.TypeKeyCollection)); err != nil {
		return nil, err
	}
	if fields["properties"], err = rawJSON(map[string]string{"name": req.Name}); err != nil {
		return nil, err
	}
	if len(req.Items) > 0 {
		if fields["items"], err = rawJSON(req.Items); err != nil {
			return nil, err
		}
	}
	doc, err := encodeEnvelope(fields)
	if err != nil {
		return nil, err
	}
	return s.createFromDocument(ctx, spaceId, doc, docCreateOptions{dryRun: dryRun})
}

// viewKeyRef is one property reference inside the requested views, with its
// JSON path for error addressing.
type viewKeyRef struct {
	key  string
	path string
}

// filterNodeProbe decodes the §6.2 filter tree just deep enough to collect
// property keys.
type filterNodeProbe struct {
	Property string            `json:"property"`
	Filters  []filterNodeProbe `json:"filters"`
}

type sortProbe struct {
	Property string `json:"property"`
}

type viewProbe struct {
	GroupBy string            `json:"groupBy"`
	Sorts   []sortProbe       `json:"sorts"`
	Filters []filterNodeProbe `json:"filters"`
	Columns []sortProbe       `json:"columns"` // columns carry `property` too
}

// collectViewPropertyKeys gathers every property key the request's filters,
// sorts and views address, each with its JSON path.
func collectViewPropertyKeys(req apimodel.V2CreateSetRequest) ([]viewKeyRef, error) {
	var refs []viewKeyRef
	if len(req.Filters) > 0 {
		var nodes []filterNodeProbe
		if err := json.Unmarshal(req.Filters, &nodes); err != nil {
			return nil, apimodel.V2ValidationFailed("invalid filters",
				apimodel.V2Issue{Path: "/filters", Message: err.Error(), Hint: "filters is the SPEC §6.2 array of filter nodes"})
		}
		collectFilterKeys(nodes, "/filters", &refs)
	}
	if len(req.Sorts) > 0 {
		var sorts []sortProbe
		if err := json.Unmarshal(req.Sorts, &sorts); err != nil {
			return nil, apimodel.V2ValidationFailed("invalid sorts",
				apimodel.V2Issue{Path: "/sorts", Message: err.Error(), Hint: "sorts is the SPEC §6.2 array of sort objects"})
		}
		for i, sort := range sorts {
			if sort.Property != "" {
				refs = append(refs, viewKeyRef{key: sort.Property, path: fmt.Sprintf("/sorts/%d/property", i)})
			}
		}
	}
	if len(req.Views) > 0 {
		var views []viewProbe
		if err := json.Unmarshal(req.Views, &views); err != nil {
			return nil, apimodel.V2ValidationFailed("invalid views",
				apimodel.V2Issue{Path: "/views", Message: err.Error(), Hint: "views is the SPEC §6.2 array of view objects"})
		}
		for i, view := range views {
			prefix := fmt.Sprintf("/views/%d", i)
			if view.GroupBy != "" {
				refs = append(refs, viewKeyRef{key: view.GroupBy, path: prefix + "/groupBy"})
			}
			for j, sort := range view.Sorts {
				if sort.Property != "" {
					refs = append(refs, viewKeyRef{key: sort.Property, path: fmt.Sprintf("%s/sorts/%d/property", prefix, j)})
				}
			}
			for j, column := range view.Columns {
				if column.Property != "" {
					refs = append(refs, viewKeyRef{key: column.Property, path: fmt.Sprintf("%s/columns/%d/property", prefix, j)})
				}
			}
			collectFilterKeys(view.Filters, prefix+"/filters", &refs)
		}
	}
	return refs, nil
}

func collectFilterKeys(nodes []filterNodeProbe, path string, refs *[]viewKeyRef) {
	for i, node := range nodes {
		nodePath := fmt.Sprintf("%s/%d", path, i)
		if node.Property != "" {
			*refs = append(*refs, viewKeyRef{key: node.Property, path: nodePath + "/property"})
		}
		collectFilterKeys(node.Filters, nodePath+"/filters", refs)
	}
}

// validateViewKeys rejects filter/sort/view property keys the type lacks —
// the R9 error lists the type's actual keys.
func (s *V2Service) validateViewKeys(spaceId, typeId, typeKey string, refs []viewKeyRef) error {
	if len(refs) == 0 {
		return nil
	}
	typeKeys := s.typePropertyKeys(spaceId, typeId)
	allowed := map[string]bool{"name": true} // universal
	for _, key := range typeKeys {
		allowed[key] = true
	}
	var issues []apimodel.V2Issue
	for _, ref := range refs {
		if allowed[ref.key] {
			continue
		}
		issues = append(issues, apimodel.V2Issue{
			Path:    ref.path,
			Message: fmt.Sprintf("type %q has no property %q — %s", typeKey, ref.key, listKnown("property keys of the type", typeKeys)),
			Hint:    didYouMean(ref.key, typeKeys, fmt.Sprintf("inspect the type with GET /v2/spaces/%s/types/%s", spaceId, typeKey)),
		})
	}
	if len(issues) > 0 {
		return apimodel.V2ValidationFailed(fmt.Sprintf("the view addresses properties type %q does not have", typeKey), issues...)
	}
	return nil
}

// buildSetDocument synthesizes the set's AnyBlock document: name + setOf in
// properties, and one dataview block (id "dataview") carrying the views —
// the §8/R10 initial-state construction.
func (s *V2Service) buildSetDocument(spaceId, typeId string, req apimodel.V2CreateSetRequest, referenced []viewKeyRef) ([]byte, error) {
	fields := map[string]json.RawMessage{}
	var err error
	if fields["version"], err = rawJSON(anyblockjson.FormatVersion); err != nil {
		return nil, err
	}
	if fields["type"], err = rawJSON(string(bundle.TypeKeySet)); err != nil {
		return nil, err
	}
	if fields["properties"], err = rawJSON(map[string]any{"name": req.Name, "setOf": []string{typeId}}); err != nil {
		return nil, err
	}

	views := req.Views
	if len(views) == 0 {
		view := map[string]any{"name": "All"}
		if len(req.Filters) > 0 {
			view["filters"] = req.Filters
		}
		if len(req.Sorts) > 0 {
			view["sorts"] = req.Sorts
		}
		if views, err = json.Marshal([]any{view}); err != nil {
			return nil, fmt.Errorf("encode default view: %w", err)
		}
	}

	dataview := map[string]any{
		"id":         dataviewBlockId,
		"type":       "dataview",
		"properties": s.dataviewProperties(spaceId, referenced),
		"views":      views,
	}
	if fields["blocks"], err = rawJSON([]any{dataview}); err != nil {
		return nil, err
	}
	return encodeEnvelope(fields)
}

// dataviewProperties lists the dataview's available properties ({key,
// format}, §6.2): name plus every referenced key, formats resolved from the
// space (falling back to text).
func (s *V2Service) dataviewProperties(spaceId string, referenced []viewKeyRef) []map[string]string {
	resolve := storeFormatResolver(s, spaceId)
	keys := []string{"name"}
	seen := map[string]bool{"name": true}
	for _, ref := range referenced {
		if !seen[ref.key] {
			seen[ref.key] = true
			keys = append(keys, ref.key)
		}
	}
	out := make([]map[string]string, 0, len(keys))
	for _, key := range keys {
		format := "shortText"
		if key != "name" {
			format = "text"
			if f, ok := resolve(domain.RelationKey(key)); ok {
				if name := anyblockjson.FormatName(f); name != "" {
					format = name
				}
			}
		}
		out = append(out, map[string]string{"key": key, "format": format})
	}
	return out
}

// storeFormatResolver builds a bundle-aware format resolver over the space.
func storeFormatResolver(s *V2Service, spaceId string) anyblockjson.FormatResolver {
	reads := newCreatingResolvers(context.Background(), s.mw, spaceId, s.store.SpaceIndex(spaceId), true)
	return func(key domain.RelationKey) (format model.RelationFormat, ok bool) {
		if rel, err := bundle.GetRelation(key); err == nil {
			return rel.Format, true
		}
		return reads.ResolveFormat(key)
	}
}
