package v2service

// search.go implements the Phase-4 query surface (APIV2.md §2 Phase 4):
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

	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
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

// maxGlobalSearchOffset bounds how deep the global merge pages: the k-way
// merge materializes up to offset+limit records PER SPACE, so an unbounded
// offset lets one request decode the whole account into memory. Deeper
// enumeration belongs on the space-scoped search, which pushes offset/limit
// into the store.
const maxGlobalSearchOffset = 2000

// searchPlan is one space's compiled query: the translated filter tree, the
// effective sort list (shared with the global merge comparator), and the
// warning-grade findings gathered on the way.
type searchPlan struct {
	textQuery string
	filters   []database.FilterRequest
	sorts     []database.SortRequest
	warnings  []v2model.Issue
	// includeFileLayouts widens the base row scope to ObjectAndFileLayouts
	// (the Phase-7 file-discovery opt-in): set when the type CHANNEL names a
	// file type — top-level type or a positive (=, IN) type filter leaf —
	// reproducing v1's prepareBaseFilters(includeFileLayouts) opt-in without
	// a new parameter. Without it a pure-v2 agent could upload a file and
	// never find it again (file layouts are excluded from ObjectLayouts).
	includeFileLayouts bool
}

// validateSearchShape applies the request-shape rules that do not depend on
// a space: the filter/filters mutual exclusion (C6).
func validateSearchShape(req v2model.SearchRequest) error {
	if req.Filter != "" && len(req.Filters) > 0 {
		return v2model.AmbiguousInput("provide filter or filters, not both",
			v2model.Issue{Path: "/filter", Message: "conflicts with filters"},
			v2model.Issue{Path: "/filters", Message: "conflicts with filter"})
	}
	return nil
}

// SearchObjects implements POST /v2/spaces/{spaceId}/search.
func (s *V2Service) SearchObjects(ctx context.Context, spaceId string, req v2model.SearchRequest, offset, limit int) ([]v2model.ObjectRow, int, bool, []v2model.Issue, error) {
	if err := s.ensureSpace(ctx, spaceId); err != nil {
		return nil, 0, false, nil, err
	}
	if err := validateSearchShape(req); err != nil {
		return nil, 0, false, nil, err
	}
	plan, err := s.buildSearchPlan(spaceId, req, true)
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
	rows := make([]v2model.ObjectRow, 0, len(records))
	for _, record := range records {
		rows = append(rows, builder.row(record))
	}
	return rows, total, offset+len(records) < total, plan.warnings, nil
}

// runSearchQuery executes one space's plan with C10 pagination. Full-text
// queries push the requested page into the store as Limit = offset+limit+1
// so the engine's candidate-budget escalation sees it (with Limit 0 the
// budget froze at the 100-doc floor and every full-text search silently
// capped at ~100 matches, ending pagination at page 4); the one extra
// record distinguishes an exhausted result from a clipped one, so the
// returned total is exact when the store had fewer matches and a lower
// bound (offset+limit+1 → has_more true) when it clipped — honest steering
// either way, within the engine's documented candidate budget.
func (s *V2Service) runSearchQuery(spaceId string, plan *searchPlan, offset, limit int) ([]database.Record, int, error) {
	index := s.store.SpaceIndex(spaceId)
	if plan.textQuery != "" {
		all, err := index.Query(database.Query{
			SpaceId:   spaceId,
			TextQuery: plan.textQuery,
			Filters:   plan.filters,
			Sorts:     plan.sorts,
			Limit:     offset + limit + 1,
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
//
// strictFields picks how an unknown `fields` entry is handled. fields is a
// DISPLAY concern, not scope: on the space-scoped search a 400 with
// did-you-mean is the right repair signal (strict), but on the global
// fan-out a hard error would silently drop the whole space from results
// and total — a column request must never narrow the search — so the
// global caller passes lenient and the key degrades to a per-space warning.
func (s *V2Service) buildSearchPlan(spaceId string, req v2model.SearchRequest, strictFields bool) (*searchPlan, error) {
	plan := &searchPlan{textQuery: req.Query}
	index := s.store.SpaceIndex(spaceId)
	reads := storeresolver.New(index)

	// the reference sets are only consulted by fields/filters/sorts (and the
	// type scope); the commonest global call — a bare full-text query — must
	// not pay a full relation-listing store query per space for nothing
	needRefs := req.Type != "" || len(req.Fields) > 0 || req.Filter != "" || len(req.Filters) > 0 || len(req.Sorts) > 0
	if !needRefs {
		plan.sorts = defaultSearchSorts(plan.textQuery, nil)
		plan.filters = appendBaseRowScope(plan.filters, false)
		return plan, nil
	}

	// rules 1 + 2: the reference key set
	var refKeys []string
	if req.Type != "" {
		// live lookup, slug-aware: a corpse type is not a query scope — the
		// did-you-mean steers to live keys (§7.5-2 corpse policy)
		typeEntries, err := s.liveTypes(spaceId)
		if err != nil {
			return nil, err
		}
		entry, ok, ambiguous := s.resolveTypeInput(req.Type, typeEntries)
		if len(ambiguous) > 0 {
			return nil, ambiguousKeyError("type key", req.Type, "/type", ambiguous)
		}
		if !ok || entry.Id == "" {
			return nil, s.unknownTypeKeyError(spaceId, req.Type, "/type")
		}
		typeId := entry.Id
		refKeys = append(s.typePropertyKeys(spaceId, typeId), "name")
		// rule 6: the top-level type composes by AND with any type filter
		plan.filters = append(plan.filters, database.FilterRequest{
			RelationKey: bundle.RelationKeyType,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.StringList([]string{typeId}),
		})
		// the Phase-7 file opt-in: a top-level file type widens the row scope
		// (entry.Key — the canonical spelling — in case a slug named it)
		if util.IsFileTypeKey(entry.Key) {
			plan.includeFileLayouts = true
		}
	}
	// kc canonicalizes every concrete property input of this request through
	// the one chain (file aliases + §7.5a-5): the listings advertise served
	// spellings, so the query channels must accept them and translate to the
	// stored keys the store binds (review cause 3)
	kc, err := s.newKeyCanon(spaceId)
	if err != nil {
		return nil, err
	}
	if req.Type == "" {
		// no type scope: the whole space's STORED keys are the base set
		// (withServedSpellings widens them below)
		stored := make([]string, 0, len(kc.entries))
		for _, entry := range kc.entries {
			stored = append(stored, entry.Key)
		}
		refKeys = sortedDistinct(stored)
	}
	// membership accepts BOTH spellings (stored keys stay valid — GET
	// documents still emit them); candidate lists speak the served spelling
	// only (never advertise what a channel rejects)
	acceptKeys := kc.withServedSpellings(refKeys)
	refKeys = kc.servedSpellings(refKeys)
	for _, extra := range [][]string{v2SystemQueryKeys, {"type"}} {
		refKeys = appendMissing(refKeys, extra...)
		acceptKeys = appendMissing(acceptKeys, extra...)
	}
	// the Phase-7 file aliases join the reference set when active (no real
	// live property claims the spelling): mimeType/size are live in EVERY
	// channel — fields, filters and sorts — translated to the backing store
	// relation, so the one advertised spelling works everywhere (C2)
	aliases := kc.aliases
	for alias := range aliases {
		refKeys = appendMissing(refKeys, alias)
		acceptKeys = appendMissing(acceptKeys, alias)
	}
	sort.Strings(refKeys)
	sort.Strings(acceptKeys)
	allowed := map[string]bool{}
	for _, key := range acceptKeys {
		allowed[key] = true
	}

	formatName := canonFormatName(aliasedFormatName(s.formatNameResolver(spaceId), aliases), kc)
	listUrl := fmt.Sprintf("list keys with GET /v2/spaces/%s/properties", spaceId)

	// rule 1 covers field keys too — hard on the space search, warning-grade
	// on the global fan-out (see the strictFields contract above)
	var issues []v2model.Issue
	for i, field := range req.Fields {
		if canonical, ambiguous := kc.canon(field); len(ambiguous) > 0 {
			issues = append(issues, ambiguousInputIssue("property key", field, fmt.Sprintf("/fields/%d", i), ambiguous))
			continue
		} else if allowed[field] || allowed[canonical] {
			continue
		}
		if strictFields {
			issues = append(issues, unknownPropertyIssue(field, fmt.Sprintf("/fields/%d", i), refKeys, listUrl))
		} else {
			plan.warnings = append(plan.warnings, v2model.Issue{
				Path:    fmt.Sprintf("/fields/%d", i),
				Message: fmt.Sprintf("field %q is not a property of space %q — omitted from those rows", field, spaceId),
			})
		}
	}
	if len(issues) > 0 {
		return nil, v2model.ValidationFailed("unknown property keys", issues...)
	}

	// both filter forms → one structured tree
	filtersJSON := req.Filters
	fromString := req.Filter != ""
	if fromString {
		parsed, err := filterstring.Parse(req.Filter, filterstring.Options{
			KnownKeys:     acceptKeys,
			ResolveFormat: formatName,
			KnownOptions: func(key string) ([]string, bool) {
				// ok=false when the store could not list the options: the
				// check is skipped rather than asserting "no such option"
				// about data the code never saw
				key, _ = kc.canon(key)
				return s.propertyOptionNames(spaceId, key)
			},
		})
		if err != nil {
			return nil, filterStringError(err)
		}
		filtersJSON = parsed
	}
	if len(filtersJSON) > 0 {
		// canonicalize every property leaf to its stored spelling BEFORE
		// validation and the store query — a served slug in a filter would
		// otherwise bind a RelationKey the store never matches, silently
		canonical, err := kc.canonicalizeRawChannel(filtersJSON, "filters", filterFieldPath(fromString))
		if err != nil {
			return nil, err
		}
		filtersJSON = canonical
	}
	if !fromString && len(filtersJSON) > 0 {
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
			plan.warnings = append(plan.warnings, v2model.Issue{Path: path, Message: iss.Message})
		}
		modelFilters, err := anyblockjson.UnmarshalFilters(filtersJSON, opts)
		if err != nil {
			return nil, mapFilterCodecError(err, fromString)
		}
		namedFileType, err := s.resolveTypeLeaves(spaceId, modelFilters, filterFieldPath(fromString))
		if err != nil {
			return nil, err
		}
		if namedFileType {
			plan.includeFileLayouts = true
		}
		rewriteAliasLeaves(modelFilters, aliases)
		plan.filters = append(plan.filters, database.FiltersFromProto(modelFilters)...)
	}

	// sorts: any property key (rule 1 scopes it; v1's closed enum is gone)
	if len(req.Sorts) > 0 {
		var probes []sortProbe
		if err := json.Unmarshal(req.Sorts, &probes); err != nil {
			return nil, v2model.ValidationFailed("invalid sorts",
				v2model.Issue{Path: "/sorts", Message: err.Error(), Hint: "sorts is the SPEC §6.2 array of sort objects"})
		}
		for i, probe := range probes {
			if probe.Property == "" {
				continue
			}
			if canonical, ambiguous := kc.canon(probe.Property); len(ambiguous) > 0 {
				issues = append(issues, ambiguousInputIssue("property key", probe.Property, fmt.Sprintf("/sorts/%d/property", i), ambiguous))
			} else if !allowed[probe.Property] && !allowed[canonical] {
				issues = append(issues, unknownPropertyIssue(probe.Property, fmt.Sprintf("/sorts/%d/property", i), refKeys, listUrl))
			}
		}
		if len(issues) > 0 {
			return nil, v2model.ValidationFailed("unknown property keys", issues...)
		}
		modelSorts, err := anyblockjson.UnmarshalSorts(req.Sorts, reads.Options())
		if err != nil {
			return nil, mapFilterCodecError(err, false)
		}
		plan.sorts = database.SortsFromProto(modelSorts)
		// every spelling translates in the sort channel too (file aliases +
		// served slugs — C2: the one advertised spelling works everywhere)
		for i := range plan.sorts {
			if canonical, ambiguous := kc.canon(string(plan.sorts[i].RelationKey)); len(ambiguous) == 0 {
				plan.sorts[i].RelationKey = domain.RelationKey(canonical)
			}
		}
		// a date sort the request left includeTime-less defaults to second
		// granularity, matching the default lastModifiedDate sort. Without
		// this the granularity depended on whether the engine appended the
		// full-text tiebreak: a single date sort got seconds via the store's
		// isSingleDateSort compensation, the same sort + `query` got days.
		for i := range plan.sorts {
			if i >= len(probes) || probes[i].IncludeTime != nil || plan.sorts[i].IncludeTime {
				continue
			}
			if name, ok := formatName(string(plan.sorts[i].RelationKey)); ok && name == "date" {
				plan.sorts[i].IncludeTime = true
			}
		}
	}

	plan.sorts = defaultSearchSorts(plan.textQuery, plan.sorts)
	plan.filters = appendBaseRowScope(plan.filters, plan.includeFileLayouts)
	return plan, nil
}

// defaultSearchSorts builds the effective sort list — shared with the global
// merge comparator. Full-text with explicit sorts gets a relevance tiebreak
// appended, which also keeps the engine from prepending its own score sort;
// full-text without sorts is pure relevance; no query and no sorts falls
// back to newest-modified first (the ListObjects default).
func defaultSearchSorts(textQuery string, sorts []database.SortRequest) []database.SortRequest {
	if textQuery != "" {
		if !hasScoreSort(sorts) {
			sorts = append(sorts, database.SortRequest{
				RelationKey: bundle.RelationKey_final_score,
				Type:        model.BlockContentDataviewSort_Desc,
			})
		}
		return sorts
	}
	if len(sorts) == 0 {
		return []database.SortRequest{{
			RelationKey: bundle.RelationKeyLastModifiedDate,
			Type:        model.BlockContentDataviewSort_Desc,
			IncludeTime: true,
		}}
	}
	return sorts
}

// appendBaseRowScope appends the base row scope (ListObjects parity): object
// layouts, no templates, no hidden objects; archived/deleted are excluded by
// the store's defaults. includeFileLayouts is the Phase-7 opt-in (a file
// type named in the type channel): it widens the layout list to
// ObjectAndFileLayouts, mirroring v1's prepareBaseFilters.
func appendBaseRowScope(filters []database.FilterRequest, includeFileLayouts bool) []database.FilterRequest {
	layouts := util.ObjectLayouts
	if includeFileLayouts {
		layouts = util.ObjectAndFileLayouts
	}
	return append(filters,
		database.FilterRequest{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.Int64List(util.LayoutsToIntArgs(layouts)),
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

// aliasedFormatName resolves an active file alias to its backing relation's
// format before the base lookup — `size > 5` must get number semantics from
// sizeInBytes, not an unknown-key miss.
func aliasedFormatName(base func(string) (string, bool), aliases map[string]domain.RelationKey) func(string) (string, bool) {
	if len(aliases) == 0 {
		return base
	}
	return func(key string) (string, bool) {
		if backing, ok := aliases[key]; ok {
			key = string(backing)
		}
		return base(key)
	}
}

// rewriteAliasLeaves rewrites filter leaves on an active file alias to the
// backing store relation — the store query only knows fileMimeType and
// sizeInBytes; the caller speaks mimeType and size.
func rewriteAliasLeaves(filters []*model.BlockContentDataviewFilter, aliases map[string]domain.RelationKey) {
	if len(aliases) == 0 {
		return
	}
	for _, f := range filters {
		if f == nil {
			continue
		}
		if len(f.NestedFilters) > 0 {
			rewriteAliasLeaves(f.NestedFilters, aliases)
		}
		if backing, ok := aliases[f.RelationKey]; ok {
			f.RelationKey = string(backing)
		}
	}
}

// propertyOptionNames lists a select/multiSelect property's existing option
// names (read-only — rule 3). ok is false when the store lookup failed —
// callers must then SKIP the option check instead of reporting a confident
// "no such option" about data the code never actually listed.
func (s *V2Service) propertyOptionNames(spaceId, key string) ([]string, bool) {
	options, err := s.store.SpaceIndex(spaceId).ListRelationOptions(domain.RelationKey(key))
	if err != nil {
		return nil, false
	}
	names := make([]string, 0, len(options))
	for _, option := range options {
		if option != nil && option.Text != "" {
			names = append(names, option.Text)
		}
	}
	sort.Strings(names)
	return names, true
}

// filterStringError maps a filterstring parse error to the C6 shape: one
// issue at /filter carrying the offset-addressed message and the
// did-you-mean hint.
func filterStringError(err error) error {
	var pe *filterstring.Error
	if !errors.As(err, &pe) {
		return v2model.ValidationFailed("invalid filter",
			v2model.Issue{Path: "/filter", Message: err.Error()})
	}
	where := fmt.Sprintf("near %q", pe.Token)
	if pe.Token == "" {
		where = "at end of input"
	}
	return v2model.ValidationFailed("invalid filter",
		v2model.Issue{
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
		return v2model.ValidationFailed("invalid filters",
			v2model.Issue{Path: "/filters", Message: err.Error()})
	}
	issues := make([]v2model.Issue, 0, len(ve.Issues))
	for _, iss := range ve.Issues {
		path := iss.Path
		if fromString {
			path = "/filter"
		}
		issues = append(issues, v2model.Issue{Path: path, Message: iss.Message})
	}
	return v2model.ValidationFailed("invalid filters", issues...)
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
	Operator   string             `json:"operator"`
	Filters    []searchFilterNode `json:"filters"`
	Property   string             `json:"property"`
	Condition  string             `json:"condition"`
	Value      any                `json:"value"`
	DatePreset string             `json:"datePreset"`
}

// isGroup reports whether the node is shaped like a group. A node carrying
// BOTH group and leaf fields is ambiguous and rejected by
// validateFilterStructure before anything asks this.
func (n searchFilterNode) isGroup() bool { return n.Operator != "" || len(n.Filters) > 0 }

// hasLeafFields reports whether the node carries any leaf-only field.
func (n searchFilterNode) hasLeafFields() bool {
	return n.Property != "" || n.Condition != "" || n.Value != nil || n.DatePreset != ""
}

// validateFilterStructure enforces the SHAPE of the §6.2 filter tree — the
// structure the served `filters` schema already describes as a two-armed
// oneOf, which nothing enforced (surface review M3). Every violation below
// used to degrade into MATCH EVERYTHING, silently and with no warning, which
// inverts the surface's central promise ("unresolved → did-you-mean, never a
// silent no-match") in the most damaging possible direction:
//
//   - a node carrying both arms, e.g. {"operator":"and","property":"severity",
//     "condition":"equal","value":"High"} — both the codec and the semantic
//     gate treat it as a GROUP, ignore the leaf fields entirely, and emit an
//     AND with no children. An empty AND is true.
//   - a group with an empty `filters` array — the same empty AND, reached
//     directly.
//   - a leaf with no condition, e.g. {"property":"severity","value":"High"},
//     which a typo'd key ("conditon") also produces. It reaches the store as
//     Condition_None and database.FiltersFromProto DROPS it.
//
// This is the ONE input channel with no GBNF grammar (a documented C13
// exception, the tree being recursive), so it is exactly where a small model
// is most likely to emit these shapes. It runs on the query path AND on
// POST /sets, where a malformed filter would otherwise be persisted into the
// set's dataview and match everything for good.
func validateFilterStructure(nodes []searchFilterNode, path string) []v2model.Issue {
	var issues []v2model.Issue
	for i, node := range nodes {
		nodePath := fmt.Sprintf("%s/%d", path, i)
		switch {
		case node.isGroup() && node.hasLeafFields():
			issues = append(issues, v2model.Issue{
				Path:    nodePath,
				Message: "a filter node is either a group (operator + filters) or a leaf (property + condition), not both",
				Hint: `to combine conditions write {"operator":"and","filters":[{"property":"…","condition":"…","value":…}]}; ` +
					`a node with both is read as an empty group, which matches everything`,
			})
		case node.isGroup() && len(node.Filters) == 0:
			issues = append(issues, v2model.Issue{
				Path:    nodePath + "/filters",
				Message: fmt.Sprintf("group %q has no filters", node.Operator),
				Hint:    "an empty group matches every object — remove it, or give it at least one leaf",
			})
		case node.isGroup():
			issues = append(issues, validateFilterStructure(node.Filters, nodePath+"/filters")...)
		case node.Condition == "":
			// the codec reports a missing property separately; a leaf that
			// names one but no condition is the typo case
			if node.Property != "" {
				issues = append(issues, v2model.Issue{
					Path:    nodePath + "/condition",
					Message: fmt.Sprintf("filter on %q has no condition", node.Property),
					Hint: "a leaf needs a condition (equal, notEqual, contains, in, empty, …); " +
						"without one the filter is dropped and every object matches",
				})
			}
		}
	}
	return issues
}

// decodeFilterNodes decodes the §6.2 array and checks its shape. Both v2
// entry points that accept the structured form go through here.
func decodeFilterNodes(raw json.RawMessage, path string) ([]searchFilterNode, error) {
	var nodes []searchFilterNode
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return nil, v2model.ValidationFailed("invalid filters",
			v2model.Issue{Path: path, Message: err.Error(), Hint: "filters is the SPEC §6.2 array of filter nodes"})
	}
	if issues := validateFilterStructure(nodes, path); len(issues) > 0 {
		return nil, v2model.ValidationFailed("invalid filter structure", issues...)
	}
	return nodes, nil
}

// validateStructuredFilters applies rules 1 (key scope) and 3 (read-only
// option names) to the structured filters array, path-addressed with
// did-you-mean — the same checks the string form gets offset-addressed from
// the parser.
func (s *V2Service) validateStructuredFilters(spaceId string, raw json.RawMessage, allowed map[string]bool, refKeys []string, formatName func(string) (string, bool), listUrl string) error {
	nodes, err := decodeFilterNodes(raw, "/filters")
	if err != nil {
		return err
	}
	var issues []v2model.Issue
	var walk func(nodes []searchFilterNode, path string)
	walk = func(nodes []searchFilterNode, path string) {
		for i, node := range nodes {
			nodePath := fmt.Sprintf("%s/%d", path, i)
			// a node carrying both arms was rejected above, so this branch is
			// now unambiguous
			if node.isGroup() {
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
			format, formatKnown := formatName(node.Property)
			// a date property takes unix seconds in the structured form
			// (SPEC §6.2) — an RFC 3339 string would survive to the store,
			// compare string-against-int64 and silently match nothing (or,
			// through the quick-option day-range transform, everything).
			// The string form converts RFC 3339 at parse time; here the
			// mistake is rejected with the conversion spelled out.
			if formatKnown && format == "date" {
				for _, value := range stringValues(node.Value) {
					issue := v2model.Issue{
						Path: nodePath + "/value",
						Hint: fmt.Sprintf(`RFC 3339 dates belong to the compact filter string, e.g. "filter": "%s > \"%s\"" — or use a datePreset`, node.Property, value),
					}
					if sec, ok := filterstring.ParseDate(value); ok {
						issue.Message = fmt.Sprintf("property %q is a date — the structured form takes unix seconds (%d), not %q", node.Property, sec, value)
					} else {
						issue.Message = fmt.Sprintf("property %q is a date — the structured form takes unix seconds, and %q is not a date", node.Property, value)
					}
					issues = append(issues, issue)
				}
			}
			// rule 3: option names resolve read-only — never a silent no-match
			if formatKnown && (format == "select" || format == "multiSelect") {
				names, ok := s.propertyOptionNames(spaceId, node.Property)
				if !ok {
					continue // the store could not list the options — no check
				}
				for _, value := range stringValues(node.Value) {
					if !containsString(names, value) {
						issues = append(issues, v2model.Issue{
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
		return v2model.ValidationFailed("invalid filters", issues...)
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
// any reference; unknown keys get the R9 did-you-mean. The returned bool
// reports whether a POSITIVE leaf named a file type — the Phase-7
// file-layout opt-in trigger. Positive is decided by EXCLUSION: any
// condition except the negated family (!=, NOT IN, notAllIn, notExactIn,
// notContains) asks for the type rather than ruling it out — the review
// caught `allIn` (the compact string's HAS ALL) silently returning zero
// file rows under the earlier =/IN allowlist. Negated leaves exclude a
// type, so they never widen the scope.
func (s *V2Service) resolveTypeLeaves(spaceId string, filters []*model.BlockContentDataviewFilter, path string) (namedFileType bool, err error) {
	// one live snapshot for every leaf of this tree — chain-resolved and
	// corpse-aware like the top-level type scope (review cause 2: the same
	// spelling worked at top level and 400'd one level down, and a
	// UI-deleted type was a usable query scope)
	typeEntries, err := s.liveTypes(spaceId)
	if err != nil {
		return false, err
	}
	return s.resolveTypeLeavesIn(spaceId, filters, path, typeEntries)
}

func (s *V2Service) resolveTypeLeavesIn(spaceId string, filters []*model.BlockContentDataviewFilter, path string, typeEntries []typeEntry) (namedFileType bool, err error) {
	for _, f := range filters {
		if f == nil {
			continue
		}
		if len(f.NestedFilters) > 0 {
			nested, err := s.resolveTypeLeavesIn(spaceId, f.NestedFilters, path, typeEntries)
			if err != nil {
				return false, err
			}
			namedFileType = namedFileType || nested
			continue
		}
		if f.RelationKey != bundle.RelationKeyType.String() || f.Value == nil {
			continue
		}
		positive := !negatedFilterConditions[f.Condition]
		resolve := func(key string) (string, error) {
			entry, ok, ambiguous := s.resolveTypeInput(key, typeEntries)
			if len(ambiguous) > 0 {
				return "", ambiguousKeyError("type key", key, path, ambiguous)
			}
			if !ok || entry.Id == "" {
				return "", s.unknownTypeKeyError(spaceId, key, path)
			}
			if positive && util.IsFileTypeKey(entry.Key) {
				namedFileType = true
			}
			return entry.Id, nil
		}
		switch kind := f.Value.GetKind().(type) {
		case *types.Value_StringValue:
			id, err := resolve(kind.StringValue)
			if err != nil {
				return false, err
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
					return false, err
				}
				kind.ListValue.Values[i] = &types.Value{Kind: &types.Value_StringValue{StringValue: id}}
			}
		}
	}
	return namedFileType, nil
}

// negatedFilterConditions is the condition family that EXCLUDES the named
// value — the only leaves that never trigger the file-layout opt-in. Kept
// as a set (not an =/IN allowlist) so a new positive condition widens by
// default instead of silently returning zero file rows.
var negatedFilterConditions = map[model.BlockContentDataviewFilterCondition]bool{
	model.BlockContentDataviewFilter_NotEqual:   true,
	model.BlockContentDataviewFilter_NotIn:      true,
	model.BlockContentDataviewFilter_NotAllIn:   true,
	model.BlockContentDataviewFilter_NotExactIn: true,
	model.BlockContentDataviewFilter_NotLike:    true,
}

//
// ---- global search (rule 4) ----
//

// spaceRef is one queryable space (id + display name for warnings).
type spaceRef struct {
	id   string
	name string
}

// spaceRefs enumerates the account's spaces from the tech space's views,
// filtered to the live ones (isLiveSpaceView — the predicate shared with
// the v2 spaces list and GET-one): global search calls SpaceIndex on every
// ref, which MINTS an index for the id, so a removed or never-loaded space
// must not get one materialized as a search side effect.
//
// The ctx grant intersects the INPUT set here, before any per-space work —
// not the output rows: a non-granted space must never enter the fan-out
// loop, where a per-space failure or warning would disclose that it exists.
func (s *V2Service) spaceRefs(ctx context.Context) ([]spaceRef, error) {
	grant := util.ApiGrantFromCtx(ctx)
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
		if !isLiveSpaceView(record.Details) {
			continue
		}
		if grant != nil && !grant.AllowsSpace(id) {
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
func (s *V2Service) GlobalSearchObjects(ctx context.Context, req v2model.SearchRequest, offset, limit int) ([]v2model.ObjectRow, int, bool, []v2model.Issue, error) {
	if err := validateSearchShape(req); err != nil {
		return nil, 0, false, nil, err
	}
	if offset > maxGlobalSearchOffset {
		return nil, 0, false, nil, v2model.ValidationFailed(
			fmt.Sprintf("global search pages at most %d rows deep", maxGlobalSearchOffset),
			v2model.Issue{
				Path:    "offset",
				Message: fmt.Sprintf("offset %d exceeds the global-search maximum of %d — the cross-space merge materializes offset+limit rows per space", offset, maxGlobalSearchOffset),
				Hint:    "narrow with filter, type or query, or page one space with POST /v2/spaces/{spaceId}/search",
			})
	}
	spaces, err := s.spaceRefs(ctx)
	if err != nil {
		return nil, 0, false, nil, err
	}

	var (
		merged     []globalRecord
		total      int
		warnings   []v2model.Issue
		mergeSorts []database.SortRequest
		firstErr   error
		resolved   int
	)
	need := offset + limit
	for _, space := range spaces {
		// rule 4: type keys and option names resolve inside each space's loop
		// iteration; a reference that resolves in only some spaces queries
		// those and warns about the rest. Fields are lenient here — a display
		// column a space lacks must not remove that space from scope/total.
		plan, err := s.buildSearchPlan(space.id, req, false)
		if err != nil {
			var v2Err *v2model.Error
			if errors.As(err, &v2Err) {
				if firstErr == nil {
					firstErr = err
				}
				warnings = append(warnings, v2model.Issue{
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
	rows := make([]v2model.ObjectRow, 0, len(page))
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
func firstIssueMessage(err *v2model.Error) string {
	if len(err.Issues) > 0 {
		return err.Issues[0].Message
	}
	return err.Message
}

// dedupeIssues drops repeated (path, message) pairs — per-space loops
// produce the same warning once per space.
func dedupeIssues(issues []v2model.Issue) []v2model.Issue {
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
