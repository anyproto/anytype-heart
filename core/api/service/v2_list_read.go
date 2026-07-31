package service

// v2_list_read.go implements the Phase-4 sets/collections read path
// (APIV2.md §2 Phase 4):
//
//	GET /v2/spaces/{spaceId}/sets/{setId}/objects?view=&fields=
//	GET /v2/spaces/{spaceId}/sets/{setId}/views
//	GET /v2/spaces/{spaceId}/collections/{collectionId}/objects?view=&fields=
//	GET /v2/spaces/{spaceId}/collections/{collectionId}/views
//
// One implementation branches on layout exactly as v1's GetObjectsInList
// does — but a set addressed through the collections route (or vice versa)
// is a 400 naming the other route. Execution is the direct store-query path
// (database.Query over the set's source / the collection's store slice) —
// explicitly NOT v1's shared-subId ObjectSearchSubscribe hack, whose
// constant subId is racy under concurrent requests. Stored-view execution
// substitutes the SPEC §6.2 dynamic placeholders server-side; any
// placeholder that cannot resolve degrades to a C6 warning, never a silent
// no-match (v1's silent-empty-result bug).

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/types"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// listKind names which route addressed the object.
type listKind int

const (
	listKindSet listKind = iota
	listKindCollection
)

// storeSliceKey is the collection membership key in the object's store.
const storeSliceKey = "objects"

// filterTemplateHost and filterTemplateUser are the SPEC §6.2 dynamic
// placeholders with defined substitutions: the object hosting the view and
// the current user.
const (
	filterTemplateHost = "_filter_template_1_"
	filterTemplateUser = "_filter_template_2_"
)

// listTarget is one resolved set/collection read target.
type listTarget struct {
	read     apicore.ObjectRead
	dataview *model.BlockContentDataview // nil when the object has none
}

// GetSetViews implements GET /v2/spaces/{spaceId}/sets/{setId}/views.
func (s *V2Service) GetSetViews(ctx context.Context, spaceId, setId string, offset, limit int) ([]json.RawMessage, int, bool, error) {
	return s.listViews(ctx, spaceId, setId, listKindSet, offset, limit)
}

// GetCollectionViews implements GET /v2/spaces/{spaceId}/collections/{collectionId}/views.
func (s *V2Service) GetCollectionViews(ctx context.Context, spaceId, collectionId string, offset, limit int) ([]json.RawMessage, int, bool, error) {
	return s.listViews(ctx, spaceId, collectionId, listKindCollection, offset, limit)
}

// GetSetObjects implements GET /v2/spaces/{spaceId}/sets/{setId}/objects.
func (s *V2Service) GetSetObjects(ctx context.Context, spaceId, setId, viewRef string, fields []string, offset, limit int) ([]apimodel.V2ObjectRow, int, bool, []apimodel.V2Issue, error) {
	return s.listObjects(ctx, spaceId, setId, listKindSet, viewRef, fields, offset, limit)
}

// GetCollectionObjects implements GET /v2/spaces/{spaceId}/collections/{collectionId}/objects.
func (s *V2Service) GetCollectionObjects(ctx context.Context, spaceId, collectionId, viewRef string, fields []string, offset, limit int) ([]apimodel.V2ObjectRow, int, bool, []apimodel.V2Issue, error) {
	return s.listObjects(ctx, spaceId, collectionId, listKindCollection, viewRef, fields, offset, limit)
}

// readListTarget reads the addressed object live and enforces the layout ↔
// route contract: the sets route requires a set, the collections route a
// collection, and a wrong-layout target is a 400 naming the other route.
func (s *V2Service) readListTarget(ctx context.Context, spaceId, listId string, want listKind) (listTarget, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return listTarget{}, err
	}
	read, err := s.reader.ReadObject(ctx, spaceId, listId)
	if err != nil {
		return listTarget{}, mapReadError(spaceId, listId, err)
	}

	layout := model.ObjectTypeLayout(-1)
	if read.Snapshot != nil && read.Snapshot.Details != nil {
		if v, ok := read.Snapshot.Details.Fields[bundle.RelationKeyResolvedLayout.String()]; ok {
			layout = model.ObjectTypeLayout(v.GetNumberValue())
		}
	}
	isSet := layout == model.ObjectType_set
	isCollection := layout == model.ObjectType_collection
	switch {
	case want == listKindSet && isCollection:
		return listTarget{}, apimodel.V2ValidationFailed(
			fmt.Sprintf("object %q is a collection, not a set — use GET /v2/spaces/%s/collections/%s/objects", listId, spaceId, listId))
	case want == listKindCollection && isSet:
		return listTarget{}, apimodel.V2ValidationFailed(
			fmt.Sprintf("object %q is a set, not a collection — use GET /v2/spaces/%s/sets/%s/objects", listId, spaceId, listId))
	case !isSet && !isCollection:
		return listTarget{}, apimodel.V2ValidationFailed(
			fmt.Sprintf("object %q is neither a set nor a collection — sets read via /v2/spaces/{spaceId}/sets/{setId}/objects, collections via /v2/spaces/{spaceId}/collections/{collectionId}/objects", listId))
	}

	target := listTarget{read: read}
	// prefer the canonical "dataview" block id; fall back to the first
	// dataview-content block (legacy objects)
	var fallback *model.BlockContentDataview
	for _, block := range read.Snapshot.Blocks {
		dv := block.GetDataview()
		if dv == nil {
			continue
		}
		if block.Id == dataviewBlockId {
			target.dataview = dv
			break
		}
		if fallback == nil {
			fallback = dv
		}
	}
	if target.dataview == nil {
		target.dataview = fallback
	}
	return target, nil
}

// listViews returns the object's views as raw §6.2 view objects — the same
// shape the AnyBlock document carries them in (C2: one vocabulary), with
// option names resolved and object refs full.
func (s *V2Service) listViews(ctx context.Context, spaceId, listId string, want listKind, offset, limit int) ([]json.RawMessage, int, bool, error) {
	target, err := s.readListTarget(ctx, spaceId, listId, want)
	if err != nil {
		return nil, 0, false, err
	}
	if target.dataview == nil {
		page, hasMore := pagination.Paginate([]json.RawMessage{}, offset, limit)
		return page, 0, hasMore, nil
	}

	// render the dataview block through the format's own §6.2 serialization
	// (no compaction: a fragment has no refs legend to resolve labels)
	dvBlock := &model.Block{
		Id:      dataviewBlockId,
		Content: &model.BlockContentOfDataview{Dataview: target.dataview},
	}
	raw, err := anyblockjson.MarshalBlockSubtree([]*model.Block{dvBlock}, storeresolver.New(s.store.SpaceIndex(spaceId)).Options())
	if err != nil {
		return nil, 0, false, fmt.Errorf("marshal dataview of %s: %w", listId, err)
	}
	var blocks []struct {
		Views []json.RawMessage `json:"views"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, 0, false, fmt.Errorf("decode dataview of %s: %w", listId, err)
	}
	var views []json.RawMessage
	if len(blocks) > 0 {
		views = blocks[0].Views
	}
	total := len(views)
	page, hasMore := pagination.Paginate(views, offset, limit)
	return page, total, hasMore, nil
}

// listObjects executes the set query / collection membership, optionally
// through one stored view's filters and sorts.
func (s *V2Service) listObjects(ctx context.Context, spaceId, listId string, want listKind, viewRef string, fields []string, offset, limit int) ([]apimodel.V2ObjectRow, int, bool, []apimodel.V2Issue, error) {
	target, err := s.readListTarget(ctx, spaceId, listId, want)
	if err != nil {
		return nil, 0, false, nil, err
	}

	var (
		filters  []database.FilterRequest
		sorts    []database.SortRequest
		warnings []apimodel.V2Issue
	)
	if viewRef != "" {
		view, err := resolveViewRef(target.dataview, viewRef, listId)
		if err != nil {
			return nil, 0, false, nil, err
		}
		viewFilters, viewWarnings := s.substitutePlaceholders(spaceId, listId, view.Filters)
		warnings = viewWarnings
		filters = append(filters, database.FiltersFromProto(viewFilters)...)
		sorts = database.SortsFromProto(view.Sorts)
	}

	var members []string // collection membership, in store-slice order
	switch want {
	case listKindSet:
		sourceFilters, err := s.setSourceFilters(spaceId, listId, target.read)
		if err != nil {
			return nil, 0, false, nil, err
		}
		filters = append(filters, sourceFilters...)
	case listKindCollection:
		members = storeSlice(target.read.Snapshot)
		if len(members) == 0 {
			return []apimodel.V2ObjectRow{}, 0, false, warnings, nil
		}
		filters = append(filters, database.FilterRequest{
			RelationKey: bundle.RelationKeyId,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.StringList(members),
		})
	}

	index := s.store.SpaceIndex(spaceId)
	var (
		records []database.Record
		total   int
	)
	if want == listKindCollection && len(sorts) == 0 {
		// no stored sort: a collection reads in its curated store-slice
		// order, so the matching members are fetched in full and reordered
		all, err := index.Query(database.Query{Filters: filters})
		if err != nil {
			return nil, 0, false, nil, fmt.Errorf("query collection %s: %w", listId, err)
		}
		orderByMembership(all, members)
		total = len(all)
		records = pageRecords(all, offset, limit)
	} else {
		if len(sorts) == 0 {
			sorts = []database.SortRequest{{
				RelationKey: bundle.RelationKeyLastModifiedDate,
				Type:        model.BlockContentDataviewSort_Desc,
				IncludeTime: true,
			}}
		}
		records, total, err = index.QueryAndCount(database.Query{
			Filters: filters,
			Sorts:   sorts,
			Offset:  offset,
			Limit:   limit,
		})
		if err != nil {
			return nil, 0, false, nil, fmt.Errorf("query list %s: %w", listId, err)
		}
	}

	builder, err := s.newObjectRowBuilder(spaceId, fields)
	if err != nil {
		return nil, 0, false, nil, err
	}
	rows := make([]apimodel.V2ObjectRow, 0, len(records))
	for _, record := range records {
		rows = append(rows, builder.row(record))
	}
	return rows, total, offset+len(records) < total, warnings, nil
}

// resolveViewRef picks a stored view by exact id or unique suffix (the C4
// leniency block refs get).
func resolveViewRef(dv *model.BlockContentDataview, viewRef, listId string) (*model.BlockContentDataviewView, error) {
	if dv == nil || len(dv.Views) == 0 {
		return nil, apimodel.V2NotFound(fmt.Sprintf("view %q not found — object %q has no views", viewRef, listId))
	}
	ids := make([]string, len(dv.Views))
	for i, view := range dv.Views {
		ids[i] = view.Id
	}
	idx, matches := matchBlockRef(ids, viewRef)
	switch {
	case matches == 1:
		return dv.Views[idx], nil
	case matches > 1:
		return nil, apimodel.V2AmbiguousInput(
			fmt.Sprintf("view reference %q matches more than one view — use the full view id", viewRef),
			apimodel.V2Issue{Path: "view", Message: "the reference is a suffix of several view ids"})
	default:
		return nil, apimodel.V2NotFound(
			fmt.Sprintf("view %q not found in object %q — view ids: %s", viewRef, listId, strings.Join(ids, ", ")))
	}
}

// setSourceFilters resolves the set's source (setOf) into store filters:
// object-type sources become `type In […]`, relation sources become
// `key NotEmpty`, OR-combined — the dataview resolution order, without v1's
// silent degradation to an unscoped query.
func (s *V2Service) setSourceFilters(spaceId, setId string, read apicore.ObjectRead) ([]database.FilterRequest, error) {
	var sources []string
	if read.Snapshot != nil && read.Snapshot.Details != nil {
		if v, ok := read.Snapshot.Details.Fields[bundle.RelationKeySetOf.String()]; ok {
			for _, entry := range v.GetListValue().GetValues() {
				if id := entry.GetStringValue(); id != "" {
					sources = append(sources, id)
				}
			}
		}
	}
	if len(sources) == 0 {
		return nil, apimodel.V2ValidationFailed(
			fmt.Sprintf("set %q queries nothing — its source (setOf) is empty", setId))
	}

	index := s.store.SpaceIndex(spaceId)
	var typeIds []string
	var relationKeys []string
	for _, entry := range sources {
		if uk, err := domain.UnmarshalUniqueKey(entry); err == nil {
			switch uk.SmartblockType() {
			case coresb.SmartBlockTypeObjectType:
				if details, err := index.GetObjectByUniqueKey(uk); err == nil {
					typeIds = append(typeIds, details.GetString(bundle.RelationKeyId))
					continue
				}
			case coresb.SmartBlockTypeRelation:
				relationKeys = append(relationKeys, uk.InternalKey())
				continue
			}
		}
		if _, err := index.GetObjectType(entry); err == nil {
			typeIds = append(typeIds, entry)
			continue
		}
		if relation, err := index.GetRelationById(entry); err == nil {
			relationKeys = append(relationKeys, relation.Key)
			continue
		}
		return nil, apimodel.V2ValidationFailed(
			fmt.Sprintf("set %q has an unresolvable source %q — setOf entries are type or property object ids", setId, entry))
	}

	var alternatives []database.FilterRequest
	if len(typeIds) > 0 {
		alternatives = append(alternatives, database.FilterRequest{
			RelationKey: bundle.RelationKeyType,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.StringList(typeIds),
		})
	}
	for _, key := range relationKeys {
		alternatives = append(alternatives, database.FilterRequest{
			RelationKey: domain.RelationKey(key),
			Condition:   model.BlockContentDataviewFilter_NotEmpty,
		})
	}
	if len(alternatives) == 1 {
		return alternatives, nil
	}
	return []database.FilterRequest{{
		Operator:      model.BlockContentDataviewFilter_Or,
		NestedFilters: alternatives,
	}}, nil
}

// storeSlice reads the collection membership ids from the snapshot's store.
func storeSlice(snapshot *model.SmartBlockSnapshotBase) []string {
	if snapshot == nil || snapshot.Collections == nil {
		return nil
	}
	v, ok := snapshot.Collections.Fields[storeSliceKey]
	if !ok {
		return nil
	}
	var out []string
	for _, entry := range v.GetListValue().GetValues() {
		if id := entry.GetStringValue(); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// orderByMembership reorders records to the collection's store-slice order.
func orderByMembership(records []database.Record, members []string) {
	position := make(map[string]int, len(members))
	for i, id := range members {
		position[id] = i
	}
	// insertion sort keeps it simple; collection pages are small
	byPos := func(r database.Record) int {
		return position[r.Details.GetString(bundle.RelationKeyId)]
	}
	for i := 1; i < len(records); i++ {
		for j := i; j > 0 && byPos(records[j]) < byPos(records[j-1]); j-- {
			records[j], records[j-1] = records[j-1], records[j]
		}
	}
}

// substitutePlaceholders resolves the SPEC §6.2 dynamic placeholders in a
// stored view's filters before execution: `_filter_template_2_` → the
// caller's participant id, `_filter_template_1_` → the hosting object id.
// Any other placeholder — or the user placeholder when no account identity
// is wired — drops its leaf and degrades to a C6 warning: evaluated
// literally it would match nothing, v1's silent-empty-result bug. The
// snapshot is a per-read copy, so substitution never touches live state.
func (s *V2Service) substitutePlaceholders(spaceId, hostId string, filters []*model.BlockContentDataviewFilter) ([]*model.BlockContentDataviewFilter, []apimodel.V2Issue) {
	var warnings []apimodel.V2Issue
	substitute := func(value string) (string, bool) {
		switch value {
		case filterTemplateHost:
			return hostId, true
		case filterTemplateUser:
			if s.accountId == "" {
				warnings = append(warnings, apimodel.V2Issue{
					Path:    "view",
					Message: fmt.Sprintf("the current-user placeholder %q could not be resolved — the filter carrying it was ignored", value),
				})
				return "", false
			}
			return domain.NewParticipantId(spaceId, s.accountId), true
		default:
			if strings.HasPrefix(value, "_filter_template_") {
				warnings = append(warnings, apimodel.V2Issue{
					Path:    "view",
					Message: fmt.Sprintf("%q is an unresolvable placeholder — the filter carrying it was ignored", value),
				})
				return "", false
			}
			return value, true
		}
	}

	var walk func(nodes []*model.BlockContentDataviewFilter) []*model.BlockContentDataviewFilter
	walk = func(nodes []*model.BlockContentDataviewFilter) []*model.BlockContentDataviewFilter {
		out := make([]*model.BlockContentDataviewFilter, 0, len(nodes))
		for _, node := range nodes {
			if node == nil {
				continue
			}
			if len(node.NestedFilters) > 0 {
				node.NestedFilters = walk(node.NestedFilters)
				if len(node.NestedFilters) == 0 {
					continue // a group whose children all dropped is a no-op
				}
				out = append(out, node)
				continue
			}
			if node.Value == nil {
				out = append(out, node)
				continue
			}
			keep := true
			switch kind := node.Value.GetKind().(type) {
			case *types.Value_StringValue:
				resolved, ok := substitute(kind.StringValue)
				if !ok {
					keep = false
					break
				}
				node.Value = &types.Value{Kind: &types.Value_StringValue{StringValue: resolved}}
			case *types.Value_ListValue:
				for i, entry := range kind.ListValue.Values {
					value := entry.GetStringValue()
					if value == "" {
						continue
					}
					resolved, ok := substitute(value)
					if !ok {
						keep = false
						break
					}
					kind.ListValue.Values[i] = &types.Value{Kind: &types.Value_StringValue{StringValue: resolved}}
				}
			}
			if keep {
				out = append(out, node)
			}
		}
		return out
	}
	return walk(filters), warnings
}
