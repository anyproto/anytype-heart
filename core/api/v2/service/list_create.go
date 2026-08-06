package v2service

// list_create.go implements POST sets and POST collections (APIV2.md §2
// Phase 2). Sets follow §8/R10: ObjectCreateSet takes no filters, so the set
// is built as one AnyBlock document whose initial state carries a fully-
// formed dataview block — one change set, honestly atomic — reusing the
// generic create path. Collections use the AnyBlock items import path.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// dataviewBlockId is the block id the set/collection editors expect their
// dataview under (template.DataviewBlockId) — a fresh id would make the
// editor add a second, default dataview at first open.
const dataviewBlockId = "dataview"

// CreateSet implements POST /v2/spaces/{spaceId}/sets.
func (s *V2Service) CreateSet(ctx context.Context, spaceId string, req v2model.CreateSetRequest, dryRun bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	if req.Name == "" {
		return nil, v2model.ValidationFailed("name is required",
			v2model.Issue{Path: "/name", Message: "a set needs a name"})
	}
	if req.Type == "" {
		return nil, v2model.ValidationFailed("type is required",
			v2model.Issue{Path: "/type", Message: "a set queries one type — name its key", Hint: fmt.Sprintf("list keys with GET /v2/spaces/%s/types", spaceId)})
	}
	// C6: filter and filters are mutually exclusive; both → ambiguous_input
	if req.Filter != "" && len(req.Filters) > 0 {
		return nil, v2model.AmbiguousInput("provide filter or filters, not both",
			v2model.Issue{Path: "/filter", Message: "conflicts with filters"},
			v2model.Issue{Path: "/filters", Message: "conflicts with filter"})
	}
	if len(req.Views) > 0 && (req.Filter != "" || len(req.Filters) > 0 || len(req.Sorts) > 0) {
		return nil, v2model.AmbiguousInput("provide views or top-level filter/filters/sorts, not both",
			v2model.Issue{Path: "/views", Message: "views carry their own filters and sorts"})
	}

	// the queried type must exist in the space — its property keys are the
	// R9 reference set for the filters
	typeId, ok := s.typeIdInSpace(spaceId, req.Type)
	if !ok {
		return nil, s.unknownTypeKeyError(spaceId, req.Type, "/type")
	}

	// the compact filter string (SPEC §6.2.1) parses to the structured array
	// through the same reference set the structured form is validated
	// against; the set document stores the structured array (export keeps
	// writing it — the document field `filter` stays reserved post-v1).
	// Option names are deliberately NOT parse-validated here: a set create is
	// a WRITE, where select option names create-missing (R9/§8.1) — unlike
	// the read-only query path.
	if req.Filter != "" {
		// "type" joins the reference set only so the discovery-served grammar
		// example (`type IN (…)`) parses to a targeted error below instead of
		// an unknown-key message that cannot explain itself
		refKeys := appendMissing(append(s.typePropertyKeys(spaceId, typeId), "name", "type"), v2SystemQueryKeys...)
		sort.Strings(refKeys)
		parsed, err := filterstring.Parse(req.Filter, filterstring.Options{
			KnownKeys:     refKeys,
			ResolveFormat: s.formatNameResolver(spaceId),
		})
		if err != nil {
			return nil, filterStringError(err)
		}
		req.Filter = ""
		req.Filters = parsed
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
func (s *V2Service) CreateCollection(ctx context.Context, spaceId string, req v2model.CreateCollectionRequest, dryRun bool) (*v2model.CreateResult, error) {
	if err := s.ensureSpaceWrite(ctx, spaceId); err != nil {
		return nil, err
	}
	if req.Name == "" {
		return nil, v2model.ValidationFailed("name is required",
			v2model.Issue{Path: "/name", Message: "a collection needs a name"})
	}

	// referential validation: items must be existing objects in the space
	var issues []v2model.Issue
	index := s.store.SpaceIndex(spaceId)
	for i, itemId := range req.Items {
		details, err := index.GetDetails(itemId)
		if err != nil || details.GetString(bundle.RelationKeyId) == "" {
			issues = append(issues, v2model.Issue{
				Path:    fmt.Sprintf("/items/%d", i),
				Message: fmt.Sprintf("object %q not found in space %q", itemId, spaceId),
				Hint:    "items are full object ids — find them with GET /v2/spaces/{spaceId}/objects",
			})
		}
	}
	if len(issues) > 0 {
		return nil, v2model.ValidationFailed("unknown collection items", issues...)
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
	// IncludeTime distinguishes "omitted" from an explicit false — the
	// search path defaults date sorts to second granularity only when the
	// request did not decide (search.go).
	IncludeTime *bool `json:"includeTime"`
}

type viewProbe struct {
	GroupBy string            `json:"groupBy"`
	Sorts   []sortProbe       `json:"sorts"`
	Filters []filterNodeProbe `json:"filters"`
	Columns []sortProbe       `json:"columns"` // columns carry `property` too
}

// collectViewPropertyKeys gathers every property key the request's filters,
// sorts and views address, each with its JSON path.
func collectViewPropertyKeys(req v2model.CreateSetRequest) ([]viewKeyRef, error) {
	var refs []viewKeyRef
	if len(req.Filters) > 0 {
		var nodes []filterNodeProbe
		if err := json.Unmarshal(req.Filters, &nodes); err != nil {
			return nil, v2model.ValidationFailed("invalid filters",
				v2model.Issue{Path: "/filters", Message: err.Error(), Hint: "filters is the SPEC §6.2 array of filter nodes"})
		}
		collectFilterKeys(nodes, "/filters", &refs)
	}
	if len(req.Sorts) > 0 {
		var sorts []sortProbe
		if err := json.Unmarshal(req.Sorts, &sorts); err != nil {
			return nil, v2model.ValidationFailed("invalid sorts",
				v2model.Issue{Path: "/sorts", Message: err.Error(), Hint: "sorts is the SPEC §6.2 array of sort objects"})
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
			return nil, v2model.ValidationFailed("invalid views",
				v2model.Issue{Path: "/views", Message: err.Error(), Hint: "views is the SPEC §6.2 array of view objects"})
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
// the R9 error lists the type's actual keys. The Phase-4 system-key
// allowlist (createdDate, lastModifiedDate, creator, lastOpenedDate) is
// always part of the reference set: those keys appear in no type's
// recommended lists yet back bread-and-butter queries (rule 2 — the
// widening of the shipped R9 sets rule).
func (s *V2Service) validateViewKeys(spaceId, typeId, typeKey string, refs []viewKeyRef) error {
	if len(refs) == 0 {
		return nil
	}
	typeKeys := s.typePropertyKeys(spaceId, typeId)
	allowed := map[string]bool{"name": true} // universal
	for _, key := range v2SystemQueryKeys {
		allowed[key] = true
	}
	for _, key := range typeKeys {
		allowed[key] = true
	}
	var issues []v2model.Issue
	for _, ref := range refs {
		if allowed[ref.key] {
			continue
		}
		if ref.key == "type" {
			// the search surface takes `type` as a pseudo-key; a set carries
			// its scope in setOf already, so the leaf is redundant here — say
			// that instead of "unknown property"
			issues = append(issues, v2model.Issue{
				Path:    ref.path,
				Message: fmt.Sprintf("a set is already scoped to type %q — drop the type filter", typeKey),
				Hint:    "to query across types use POST /v2/spaces/{spaceId}/search, where type is a filterable pseudo-key",
			})
			continue
		}
		issues = append(issues, v2model.Issue{
			Path:    ref.path,
			Message: fmt.Sprintf("type %q has no property %q — %s", typeKey, ref.key, listKnown("property keys of the type", typeKeys)),
			Hint:    didYouMean(ref.key, typeKeys, fmt.Sprintf("inspect the type with GET /v2/spaces/%s/types/%s", spaceId, typeKey)),
		})
	}
	if len(issues) > 0 {
		return v2model.ValidationFailed(fmt.Sprintf("the view addresses properties type %q does not have", typeKey), issues...)
	}
	return nil
}

// buildSetDocument synthesizes the set's AnyBlock document: name + setOf in
// properties, and one dataview block (id "dataview") carrying the views —
// the §8/R10 initial-state construction.
func (s *V2Service) buildSetDocument(spaceId, typeId string, req v2model.CreateSetRequest, referenced []viewKeyRef) ([]byte, error) {
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
		// the format vocabulary has a single text name: the stored
		// longtext/shorttext split folds into "text" (§3), so `name` resolves
		// through the same path as every other key
		format := "text"
		if f, ok := resolve(domain.RelationKey(key)); ok {
			if name := anyblockjson.FormatName(f); name != "" {
				format = name
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
