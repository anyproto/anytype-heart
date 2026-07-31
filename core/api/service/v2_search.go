package service

// v2_search.go implements the Phase-4 query surface (APIV2.md §2 Phase 4):
// POST /v2/spaces/{spaceId}/search and POST /v2/search (global). Both filter
// forms — the compact string (SPEC §6.2.1, parsed by anyblockjson/
// filterstring) and the structured array — land on ONE internal tree via
// anyblockjson.UnmarshalFilters, then translate to a direct store query
// (database.Query): full-text via TextQuery, any-key filters with date
// presets, any-key sorts. Search is a read: no idempotency, dry_run ignored.
//
// The Phase-4 validation & resolution rules, as numbered in the spec:
//  1. key scope — a top-level type narrows keys to the type's recommended
//     set (+ name); otherwise the space's property keys; per space on global
//  2. the system-key allowlist (createdDate, lastModifiedDate, creator,
//     lastOpenedDate) always joins the reference set
//  3. option names resolve READ-ONLY — a query never creates the option it
//     names; unresolved → did-you-mean, never a silent no-match
//  4. global search resolves per space, merges by the requested sort, and
//     reports honest totals (sum of per-space store counts)
//  5. the unguarded-date-comparison hazard rides the C6 warnings channel
//  6. `type` is a filterable pseudo-key; the top-level `type` composes by AND

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/gogo/protobuf/types"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// v2SystemQueryKeys is the Phase-4 rule-2 allowlist: §3-output-only/system
// keys that appear in no type's recommended lists yet back bread-and-butter
// queries. Always part of the query-surface reference set — for search AND
// for set filters/sorts.
var v2SystemQueryKeys = []string{"createdDate", "lastModifiedDate", "creator", "lastOpenedDate"}

// V2SearchNarrowHint is the C10 truncation steering for search results.
const V2SearchNarrowHint = "narrow with filter or query, or request the next offset"

// searchPlan is one space's compiled query: the translated filter tree, the
// effective sort list (shared with the global merge comparator), and the
// warning-grade findings gathered on the way.
type searchPlan struct {
	textQuery string
	filters   []database.FilterRequest
	sorts     []database.SortRequest
	warnings  []apimodel.V2Issue
}

// validateSearchShape applies the request-shape rules that do not depend on
// a space: the filter/filters mutual exclusion (C6).
func validateSearchShape(req apimodel.V2SearchRequest) error {
	if req.Filter != "" && len(req.Filters) > 0 {
		return apimodel.V2AmbiguousInput("provide filter or filters, not both",
			apimodel.V2Issue{Path: "/filter", Message: "conflicts with filters"},
			apimodel.V2Issue{Path: "/filters", Message: "conflicts with filter"})
	}
	return nil
}

// SearchObjects implements POST /v2/spaces/{spaceId}/search.
func (s *V2Service) SearchObjects(ctx context.Context, spaceId string, req apimodel.V2SearchRequest, offset, limit int) ([]apimodel.V2ObjectRow, int, bool, []apimodel.V2Issue, error) {
	if err := s.ensureSpace(spaceId); err != nil {
		return nil, 0, false, nil, err
	}
	if err := validateSearchShape(req); err != nil {
		return nil, 0, false, nil, err
	}
	plan, err := s.buildSearchPlan(spaceId, req)
	if err != nil {
		return nil, 0, false, nil, err
	}

	records, total, err := s.runSearchQuery(spaceId, plan, offset, limit)
	if err != nil {
		return nil, 0, false, nil, err
	}

	builder, err := s.newObjectRowBuilder(spaceId, req.Fields)
	if err != nil {
		return nil, 0, false, nil, err
	}
	rows := make([]apimodel.V2ObjectRow, 0, len(records))
	for _, record := range records {
		rows = append(rows, builder.row(record))
	}
	return rows, total, offset+len(records) < total, plan.warnings, nil
}

// runSearchQuery executes one space's plan with C10 pagination. Full-text
// queries materialize the whole (candidate-bounded) result set so the total
// is the store count, not v1's total = len(fetched) approximation.
func (s *V2Service) runSearchQuery(spaceId string, plan *searchPlan, offset, limit int) ([]database.Record, int, error) {
	index := s.store.SpaceIndex(spaceId)
	if plan.textQuery != "" {
		all, err := index.Query(database.Query{
			SpaceId:   spaceId,
			TextQuery: plan.textQuery,
			Filters:   plan.filters,
			Sorts:     plan.sorts,
		})
		if err != nil {
			return nil, 0, fmt.Errorf("search space %s: %w", spaceId, err)
		}
		return pageRecords(all, offset, limit), len(all), nil
	}
	records, total, err := index.QueryAndCount(database.Query{
		Filters: plan.filters,
		Sorts:   plan.sorts,
		Offset:  offset,
		Limit:   limit,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("search space %s: %w", spaceId, err)
	}
	return records, total, nil
}

// pageRecords slices one C10 page out of a full result set.
func pageRecords(records []database.Record, offset, limit int) []database.Record {
	if offset >= len(records) {
		return nil
	}
	end := offset + limit
	if limit <= 0 || end > len(records) {
		end = len(records)
	}
	return records[offset:end]
}

//
// ---- plan building ----
//

// buildSearchPlan compiles the request against one space: reference-set
// validation (rules 1–2), both filter forms onto one tree, read-only option
// resolution (rule 3), the type pseudo-key (rule 6), and the effective sort
// list. Every failure is a C6 error with path-addressed issues.
func (s *V2Service) buildSearchPlan(spaceId string, req apimodel.V2SearchRequest) (*searchPlan, error) {
	plan := &searchPlan{textQuery: req.Query}
	index := s.store.SpaceIndex(spaceId)
	reads := storeresolver.New(index)

	// rules 1 + 2: the reference key set
	var refKeys []string
	if req.Type != "" {
		typeId, ok := s.typeIdInSpace(spaceId, req.Type)
		if !ok {
			return nil, s.unknownTypeKeyError(spaceId, req.Type, "/type")
		}
		refKeys = append(s.typePropertyKeys(spaceId, typeId), "name")
		// rule 6: the top-level type composes by AND with any type filter
		plan.filters = append(plan.filters, database.FilterRequest{
			RelationKey: bundle.RelationKeyType,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.StringList([]string{typeId}),
		})
	} else {
		refKeys = s.knownPropertyKeys(spaceId)
	}
	refKeys = appendMissing(refKeys, v2SystemQueryKeys...)
	refKeys = appendMissing(refKeys, "type")
	sort.Strings(refKeys)
	allowed := map[string]bool{}
	for _, key := range refKeys {
		allowed[key] = true
	}

	formatName := s.formatNameResolver(spaceId)
	listUrl := fmt.Sprintf("list keys with GET /v2/spaces/%s/properties", spaceId)

	// rule 1 covers field keys too
	var issues []apimodel.V2Issue
	for i, field := range req.Fields {
		if !allowed[field] {
			issues = append(issues, unknownPropertyIssue(field, fmt.Sprintf("/fields/%d", i), refKeys, listUrl))
		}
	}
	if len(issues) > 0 {
		return nil, apimodel.V2ValidationFailed("unknown property keys", issues...)
	}

	// both filter forms → one structured tree
	filtersJSON := req.Filters
	fromString := req.Filter != ""
	if fromString {
		parsed, err := filterstring.Parse(req.Filter, filterstring.Options{
			KnownKeys:     refKeys,
			ResolveFormat: formatName,
			KnownOptions: func(key string) ([]string, bool) {
				return s.propertyOptionNames(spaceId, key), true
			},
		})
		if err != nil {
			return nil, filterStringError(err)
		}
		filtersJSON = parsed
	} else if len(filtersJSON) > 0 {
		// the parser validated the string form with offsets; the structured
		// form gets the same checks path-addressed (rules 1 + 3)
		if err := s.validateStructuredFilters(spaceId, filtersJSON, allowed, refKeys, formatName, listUrl); err != nil {
			return nil, err
		}
	}

	if len(filtersJSON) > 0 {
		opts := reads.Options()
		opts.OnWarning = func(iss anyblockjson.Issue) {
			path := iss.Path
			if fromString {
				path = "/filter" // the string form has no /filters array to address
			}
			plan.warnings = append(plan.warnings, apimodel.V2Issue{Path: path, Message: iss.Message})
		}
		modelFilters, err := anyblockjson.UnmarshalFilters(filtersJSON, opts)
		if err != nil {
			return nil, mapFilterCodecError(err, fromString)
		}
		if err := s.resolveTypeLeaves(spaceId, modelFilters, filterFieldPath(fromString)); err != nil {
			return nil, err
		}
		plan.filters = append(plan.filters, database.FiltersFromProto(modelFilters)...)
	}

	// sorts: any property key (rule 1 scopes it; v1's closed enum is gone)
	if len(req.Sorts) > 0 {
		var probes []sortProbe
		if err := json.Unmarshal(req.Sorts, &probes); err != nil {
			return nil, apimodel.V2ValidationFailed("invalid sorts",
				apimodel.V2Issue{Path: "/sorts", Message: err.Error(), Hint: "sorts is the SPEC §6.2 array of sort objects"})
		}
		for i, probe := range probes {
			if probe.Property != "" && !allowed[probe.Property] {
				issues = append(issues, unknownPropertyIssue(probe.Property, fmt.Sprintf("/sorts/%d/property", i), refKeys, listUrl))
			}
		}
		if len(issues) > 0 {
			return nil, apimodel.V2ValidationFailed("unknown property keys", issues...)
		}
		modelSorts, err := anyblockjson.UnmarshalSorts(req.Sorts, reads.Options())
		if err != nil {
			return nil, mapFilterCodecError(err, false)
		}
		plan.sorts = database.SortsFromProto(modelSorts)
	}

	// the effective sort list — shared with the global merge comparator.
	// Full-text with explicit sorts gets a relevance tiebreak appended, which
	// also keeps the engine from prepending its own score sort; full-text
	// without sorts is pure relevance; no query and no sorts falls back to
	// newest-modified first (the ListObjects default).
	if plan.textQuery != "" {
		if !hasScoreSort(plan.sorts) {
			plan.sorts = append(plan.sorts, database.SortRequest{
				RelationKey: bundle.RelationKey_final_score,
				Type:        model.BlockContentDataviewSort_Desc,
			})
		}
	} else if len(plan.sorts) == 0 {
		plan.sorts = []database.SortRequest{{
			RelationKey: bundle.RelationKeyLastModifiedDate,
			Type:        model.BlockContentDataviewSort_Desc,
			IncludeTime: true,
		}}
	}

	// base row scope (ListObjects parity): object layouts, no templates, no
	// hidden objects; archived/deleted are excluded by the store's defaults
	plan.filters = append(plan.filters,
		database.FilterRequest{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.Int64List(util.LayoutsToIntArgs(util.ObjectLayouts)),
		},
		database.FilterRequest{
			RelationKey: "type.uniqueKey",
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.String(bundle.TypeKeyTemplate.URL()),
		},
		database.FilterRequest{
			RelationKey: bundle.RelationKeyIsHidden,
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.Bool(true),
		},
	)
	return plan, nil
}

// hasScoreSort reports whether the sort list already orders by relevance.
func hasScoreSort(sorts []database.SortRequest) bool {
	for _, s := range sorts {
		if s.RelationKey == bundle.RelationKey_score || s.RelationKey == bundle.RelationKey_final_score {
			return true
		}
	}
	return false
}

// appendMissing appends the entries not already present.
func appendMissing(keys []string, add ...string) []string {
	seen := map[string]bool{}
	for _, k := range keys {
		seen[k] = true
	}
	for _, k := range add {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys
}

// formatNameResolver resolves a property key to its §3 format name over one
// space (bundle first, then the store).
func (s *V2Service) formatNameResolver(spaceId string) func(key string) (string, bool) {
	resolve := storeFormatResolver(s, spaceId)
	return func(key string) (string, bool) {
		if f, ok := resolve(domain.RelationKey(key)); ok {
			return anyblockjson.FormatName(f), true
		}
		return "", false
	}
}

// propertyOptionNames lists a select/multiSelect property's existing option
// names (read-only — rule 3).
func (s *V2Service) propertyOptionNames(spaceId, key string) []string {
	options, err := s.store.SpaceIndex(spaceId).ListRelationOptions(domain.RelationKey(key))
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(options))
	for _, option := range options {
		if option != nil && option.Text != "" {
			names = append(names, option.Text)
		}
	}
	sort.Strings(names)
	return names
}

// filterStringError maps a filterstring parse error to the C6 shape: one
// issue at /filter carrying the offset-addressed message and the
// did-you-mean hint.
func filterStringError(err error) error {
	var pe *filterstring.Error
	if !errors.As(err, &pe) {
		return apimodel.V2ValidationFailed("invalid filter",
			apimodel.V2Issue{Path: "/filter", Message: err.Error()})
	}
	where := fmt.Sprintf("near %q", pe.Token)
	if pe.Token == "" {
		where = "at end of input"
	}
	return apimodel.V2ValidationFailed("invalid filter",
		apimodel.V2Issue{
			Path:    "/filter",
			Message: fmt.Sprintf("parse error at offset %d %s: %s", pe.Offset, where, pe.Message),
			Hint:    pe.Hint,
		})
}

// mapFilterCodecError converts anyblockjson.ValidationError issues from the
// filter/sort codec into the C6 shape. fromString remaps the codec's
// /filters paths onto the /filter request field.
func mapFilterCodecError(err error, fromString bool) error {
	var ve *anyblockjson.ValidationError
	if !errors.As(err, &ve) {
		return apimodel.V2ValidationFailed("invalid filters",
			apimodel.V2Issue{Path: "/filters", Message: err.Error()})
	}
	issues := make([]apimodel.V2Issue, 0, len(ve.Issues))
	for _, iss := range ve.Issues {
		path := iss.Path
		if fromString {
			path = "/filter"
		}
		issues = append(issues, apimodel.V2Issue{Path: path, Message: iss.Message})
	}
	return apimodel.V2ValidationFailed("invalid filters", issues...)
}

// filterFieldPath names the request field errors should address for the
// active filter form.
func filterFieldPath(fromString bool) string {
	if fromString {
		return "/filter"
	}
	return "/filters"
}

// searchFilterNode decodes the structured form just deep enough for
// path-addressed validation.
type searchFilterNode struct {
	Operator string             `json:"operator"`
	Filters  []searchFilterNode `json:"filters"`
	Property string             `json:"property"`
	Value    any                `json:"value"`
}

// validateStructuredFilters applies rules 1 (key scope) and 3 (read-only
// option names) to the structured filters array, path-addressed with
// did-you-mean — the same checks the string form gets offset-addressed from
// the parser.
func (s *V2Service) validateStructuredFilters(spaceId string, raw json.RawMessage, allowed map[string]bool, refKeys []string, formatName func(string) (string, bool), listUrl string) error {
	var nodes []searchFilterNode
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return apimodel.V2ValidationFailed("invalid filters",
			apimodel.V2Issue{Path: "/filters", Message: err.Error(), Hint: "filters is the SPEC §6.2 array of filter nodes"})
	}
	var issues []apimodel.V2Issue
	var walk func(nodes []searchFilterNode, path string)
	walk = func(nodes []searchFilterNode, path string) {
		for i, node := range nodes {
			nodePath := fmt.Sprintf("%s/%d", path, i)
			if len(node.Filters) > 0 || node.Operator != "" {
				walk(node.Filters, nodePath+"/filters")
				continue
			}
			if node.Property == "" {
				continue // the codec reports the missing key
			}
			if !allowed[node.Property] {
				issues = append(issues, unknownPropertyIssue(node.Property, nodePath+"/property", refKeys, listUrl))
				continue
			}
			// rule 3: option names resolve read-only — never a silent no-match
			if format, ok := formatName(node.Property); ok && (format == "select" || format == "multiSelect") {
				names := s.propertyOptionNames(spaceId, node.Property)
				for _, value := range stringValues(node.Value) {
					if !containsString(names, value) {
						issues = append(issues, apimodel.V2Issue{
							Path:    nodePath + "/value",
							Message: fmt.Sprintf("property %q has no option named %q — a query never creates options", node.Property, value),
							Hint:    didYouMean(value, names, fmt.Sprintf("list them with GET /v2/spaces/%s/properties/%s/options", spaceId, node.Property)),
						})
					}
				}
			}
		}
	}
	walk(nodes, "/filters")
	if len(issues) > 0 {
		return apimodel.V2ValidationFailed("invalid filters", issues...)
	}
	return nil
}

// stringValues extracts the string entries of a filter value (bare string or
// list).
func stringValues(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		var out []string
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func containsString(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

// resolveTypeLeaves resolves the `type` pseudo-key (rule 6): filter leaves
// on `type` carry type KEYS, resolved to the space's type object ids like
// any reference; unknown keys get the R9 did-you-mean.
func (s *V2Service) resolveTypeLeaves(spaceId string, filters []*model.BlockContentDataviewFilter, path string) error {
	for _, f := range filters {
		if f == nil {
			continue
		}
		if len(f.NestedFilters) > 0 {
			if err := s.resolveTypeLeaves(spaceId, f.NestedFilters, path); err != nil {
				return err
			}
			continue
		}
		if f.RelationKey != bundle.RelationKeyType.String() || f.Value == nil {
			continue
		}
		resolve := func(key string) (string, error) {
			id, ok := s.typeIdInSpace(spaceId, key)
			if !ok {
				return "", s.unknownTypeKeyError(spaceId, key, path)
			}
			return id, nil
		}
		switch kind := f.Value.GetKind().(type) {
		case *types.Value_StringValue:
			id, err := resolve(kind.StringValue)
			if err != nil {
				return err
			}
			f.Value = &types.Value{Kind: &types.Value_StringValue{StringValue: id}}
		case *types.Value_ListValue:
			for i, entry := range kind.ListValue.Values {
				name := entry.GetStringValue()
				if name == "" {
					continue
				}
				id, err := resolve(name)
				if err != nil {
					return err
				}
				kind.ListValue.Values[i] = &types.Value{Kind: &types.Value_StringValue{StringValue: id}}
			}
		}
	}
	return nil
}

//
// ---- global search (rule 4) ----
//

// spaceRef is one queryable space (id + display name for warnings).
type spaceRef struct {
	id   string
	name string
}

// spaceRefs enumerates the account's spaces from the tech space's views.
func (s *V2Service) spaceRefs() ([]spaceRef, error) {
	records, err := s.store.SpaceIndex(s.techSpaceId).Query(database.Query{
		Filters: []database.FilterRequest{{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_spaceView)),
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("query space views: %w", err)
	}
	var out []spaceRef
	seen := map[string]bool{}
	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyTargetSpaceId)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		name := record.Details.GetString(bundle.RelationKeyName)
		if name == "" {
			name = id
		}
		out = append(out, spaceRef{id: id, name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out, nil
}

// globalRecord is one merged result with its origin space.
type globalRecord struct {
	spaceId string
	record  database.Record
}

// GlobalSearchObjects implements POST /v2/search: the per-space loop with
// per-space name resolution, a merge by the requested sort, and honest
// totals — total is the sum of per-space store counts, has_more compares it
// against the requested page (never v1's total = len(fetched)).
func (s *V2Service) GlobalSearchObjects(ctx context.Context, req apimodel.V2SearchRequest, offset, limit int) ([]apimodel.V2ObjectRow, int, bool, []apimodel.V2Issue, error) {
	if err := validateSearchShape(req); err != nil {
		return nil, 0, false, nil, err
	}
	spaces, err := s.spaceRefs()
	if err != nil {
		return nil, 0, false, nil, err
	}

	var (
		merged     []globalRecord
		total      int
		warnings   []apimodel.V2Issue
		mergeSorts []database.SortRequest
		firstErr   error
		resolved   int
	)
	need := offset + limit
	for _, space := range spaces {
		// rule 4: type keys and option names resolve inside each space's loop
		// iteration; a reference that resolves in only some spaces queries
		// those and warns about the rest
		plan, err := s.buildSearchPlan(space.id, req)
		if err != nil {
			var v2Err *apimodel.V2Error
			if errors.As(err, &v2Err) {
				if firstErr == nil {
					firstErr = err
				}
				warnings = append(warnings, apimodel.V2Issue{
					Message: fmt.Sprintf("space %q was skipped: %s", space.name, firstIssueMessage(v2Err)),
				})
				continue
			}
			return nil, 0, false, nil, err
		}
		resolved++
		if mergeSorts == nil {
			mergeSorts = plan.sorts
		}
		records, spaceTotal, err := s.runSearchQuery(space.id, plan, 0, need)
		if err != nil {
			return nil, 0, false, nil, err
		}
		total += spaceTotal
		for _, record := range records {
			merged = append(merged, globalRecord{spaceId: space.id, record: record})
		}
		warnings = append(warnings, plan.warnings...)
	}
	if resolved == 0 && firstErr != nil {
		// the request resolved nowhere — the per-space error is the answer,
		// not an empty result
		return nil, 0, false, nil, firstErr
	}
	warnings = dedupeIssues(warnings)

	sortGlobalRecords(merged, mergeSorts)
	page := merged[minInt(offset, len(merged)):minInt(need, len(merged))]

	builders := map[string]*objectRowBuilder{}
	rows := make([]apimodel.V2ObjectRow, 0, len(page))
	for _, entry := range page {
		builder, ok := builders[entry.spaceId]
		if !ok {
			if builder, err = s.newObjectRowBuilder(entry.spaceId, req.Fields); err != nil {
				return nil, 0, false, nil, err
			}
			builder.includeSpaceId = true
			builders[entry.spaceId] = builder
		}
		rows = append(rows, builder.row(entry.record))
	}
	return rows, total, need < total, warnings, nil
}

// firstIssueMessage picks the most specific message of a C6 error for the
// per-space skip warning.
func firstIssueMessage(err *apimodel.V2Error) string {
	if len(err.Issues) > 0 {
		return err.Issues[0].Message
	}
	return err.Message
}

// dedupeIssues drops repeated (path, message) pairs — per-space loops
// produce the same warning once per space.
func dedupeIssues(issues []apimodel.V2Issue) []apimodel.V2Issue {
	seen := map[string]bool{}
	out := issues[:0]
	for _, iss := range issues {
		key := iss.Path + "\x00" + iss.Message
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, iss)
	}
	return out
}

// sortGlobalRecords merges per-space results by the effective sort list.
// The comparator approximates the store's ordering (no locale collation);
// ties break by space id then object id for determinism.
func sortGlobalRecords(records []globalRecord, sorts []database.SortRequest) {
	sort.SliceStable(records, func(i, j int) bool {
		a, b := records[i], records[j]
		for _, srt := range sorts {
			av := a.record.Details.Get(srt.RelationKey)
			bv := b.record.Details.Get(srt.RelationKey)
			comp := av.Compare(bv)
			if comp == 0 {
				continue
			}
			if srt.Type == model.BlockContentDataviewSort_Desc {
				return comp > 0
			}
			return comp < 0
		}
		if a.spaceId != b.spaceId {
			return a.spaceId < b.spaceId
		}
		return a.record.Details.GetString(bundle.RelationKeyId) < b.record.Details.GetString(bundle.RelationKeyId)
	})
}
