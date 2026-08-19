package v2service

// viewops.go implements the update_view PATCH op (APIV2.md §8.17): the
// targeted edit of ONE dataview view — the write path for view and column
// configuration that GET …/views could read but nothing could change. The
// op speaks SPEC §6.2's vocabulary throughout: the merge happens on the
// block's exported JSON form and the result re-imports through the format
// codec, exactly the set_cell pattern for tables — untouched views, columns
// and editor state (groups, objectOrders) round-trip unchanged.
//
// Two merge channels, both scoped so one flip never rewrites an array (the
// documented small-model trap):
//   - `set` merges §6.2 view-level fields (update_block semantics: named
//     fields change, explicit null clears one; sorts/filters replace whole —
//     they are small ordered lists; `filter` is the compact-string
//     alternative to `filters`);
//   - `columns` merges per column, keyed by property key (a null removes the
//     column, a key without a column appends one).
//
// Everything is validated against a private copy FIRST — an op that fails
// leaves neither the state nor the applier's document view touched.

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// maxV2ViewColumns bounds one update_view's columns map — the maxProperties
// the served op schema advertises (M6: advertised bounds are enforced).
const maxV2ViewColumns = 64

// maxV2ViewPageSize bounds pageSize — the maximum the served schema
// advertises.
const maxV2ViewPageSize = 1000

// maxV2ColumnWidth bounds a column width in pixels (SPEC §6.2: the editor's
// own drag-resize stays within 54…1000; the schema advertises this cap).
const maxV2ColumnWidth = 10000

// maxV2ViewFilterNodes bounds the TOP-LEVEL nodes of a set.filters array —
// the maxItems the served schema advertises (M6). Nesting stays unbounded:
// the recursive tree is the documented C13 exception.
const maxV2ViewFilterNodes = 32

// maxV2CustomOrderValues bounds one sort's customOrder — the maxItems the
// served schema always advertised (M6: advertised = enforced).
const maxV2CustomOrderValues = 128

// v2ViewFieldKinds maps each authorable §6.2 view-level field to its value
// kind. groups and objectOrders are output-only editor state (§4a) and id is
// immutable — all three get targeted rejections; anything else unknown gets
// the allowed-field listing.
var v2ViewFieldKinds = map[string]string{
	"name":                "name",
	"type":                "viewType",
	"group_by":            "propertyKey",
	"cover_property":      "propertyKey",
	"end_property":        "propertyKey",
	"hide_icon":           "bool",
	"card_size":           "cardSize",
	"cover_fit":           "bool",
	"colored_groups":      "bool",
	"page_size":           "int",
	"default_template_id": "id",
	"default_type_id":     "id",
	"wrap_content":        "bool",
	"list_size":           "listSize",
	"alternate_rows":      "bool",
	"sorts":               "sorts",
	"filters":             "filters",
	"filter":              "filterString",
}

// v2ViewSetFieldList renders the allowed `set` fields for error text, in a
// stable order.
func v2ViewSetFieldList() string {
	fields := make([]string, 0, len(v2ViewFieldKinds))
	for f := range v2ViewFieldKinds {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	return strings.Join(fields, ", ")
}

// v2ColumnFields is the column-patch vocabulary (§6.2 Column, minus the
// output-only/derived pieces).
var v2ColumnFields = map[string]bool{
	"hidden": true, "width": true, "align": true, "aggregation": true,
}

// viewKeyUse records one property key an update_view references, with the
// issue path a rejection should carry.
type viewKeyUse struct {
	key  string
	path string
}

func (a *v2StateApplier) applyUpdateView(op opUpdateView, opPath string) error {
	if len(op.Set) == 0 && len(op.Columns) == 0 {
		return v2model.ValidationFailed("update_view needs set and/or columns",
			v2model.Issue{Path: opPath, Message: "set (view-level fields) and columns (per-column patches) are both empty",
				Hint: "GET /v2/schemas/ops/update_view for the op's schema and example"})
	}
	doc, err := a.doc()
	if err != nil {
		return err
	}
	idx, err := a.resolveDataviewBlock(doc, op.Block, opPath)
	if err != nil {
		return err
	}
	fullId := blockId(doc.blocks[idx])

	// operate on a private deep copy: every validation below may bail, and a
	// failing op must leave the applier's document view untouched
	edited, err := deepCopyBlock(doc.blocks[idx])
	if err != nil {
		return err
	}
	delete(edited, "indent")
	views, _ := edited["views"].([]any)
	vi, err := resolveViewIndex(views, op.View, opPath)
	if err != nil {
		return err
	}
	view, ok := views[vi].(map[string]any)
	if !ok {
		return fmt.Errorf("block %s: view %d is not an object", fullId, vi)
	}

	var issues []v2model.Issue
	var keyUses []viewKeyUse
	// membership is captured BEFORE the merges: a key the op itself just
	// introduced must not vouch for its own existence
	preKnown := dataviewMembership(edited)

	if err := a.applyViewSet(op.Set, edited, view, opPath, false, &issues, &keyUses); err != nil {
		return err
	}
	if err := a.applyViewColumns(op.Columns, view, opPath, &issues, &keyUses); err != nil {
		return err
	}

	// referenced property keys must resolve — in the dataview itself or in the
	// space (did-you-mean otherwise); resolvable keys missing from the
	// dataview's properties list are appended so the codec can rehydrate their
	// formats (sorts/filters carry no cached format, SPEC §6.2)
	a.validateViewKeys(edited, preKnown, keyUses, &issues)

	if len(issues) > 0 {
		return v2model.ValidationFailed("update_view rejected", issues...)
	}

	views[vi] = view
	edited["views"] = views
	viewId, _ := view["id"].(string)
	return a.commitDataviewBlock(edited, fullId, opPath, viewCommitPlan{
		authored: map[string]viewAuthored{viewId: {
			restoreFrom: viewId,
			sorts:       setNames(op.Set, "sorts"),
			filters:     setNames(op.Set, "filters") || setNames(op.Set, "filter"),
		}},
	})
}

// setNames reports whether the set channel names a field (null included —
// a null clears the field, which is authoring it).
func setNames(set map[string]json.RawMessage, field string) bool {
	_, ok := set[field]
	return ok
}

// viewCommitPlan tells commitDataviewBlock which parts of the imported
// dataview the op actually AUTHORED. Everything else is restored from the
// live proto after the codec round-trip (§8.19-A): the JSON form carries
// option values as NAMES, so re-importing content the op never touched
// re-resolves name→id — a dangling reference round-trips into a freshly
// minted option, twins sharing a name repoint by store listing order, and
// the creates fire under the object lock past both halves of M5. Restoring
// unauthored content makes the codec round-trip a no-op for it.
type viewCommitPlan struct {
	// authored maps a view id to what the op wrote there. A view id absent
	// from the map was not touched at all: it is restored wholly (move_view
	// and delete_view author nothing; update_view and insert_view author one
	// view).
	authored map[string]viewAuthored
}

// viewAuthored describes the op's writes within one view. Fields the op did
// not author restore from the live view restoreFrom names — the view's own
// id normally, the copy_from source for a fresh copy, empty when there is no
// live source (a bare insert_view, whose constructed content carries no
// resolvable values).
type viewAuthored struct {
	restoreFrom string
	sorts       bool
	filters     bool
}

// commitDataviewBlock re-imports one edited dataview block through the
// format codec, restores everything the op did not author from the live
// proto, and lands the block in the state — the shared tail of every view
// op. The import runs with NO-CREATE option resolution (commitImportOptions):
// op-authored option names were resolved by the pre-lock prewarm, so a miss
// here is content the op has no business minting for — it passes through
// verbatim instead of creating under the lock (§8.19-A).
func (a *v2StateApplier) commitDataviewBlock(edited map[string]any, fullId, opPath string, plan viewCommitPlan) error {
	raw, err := json.Marshal(edited)
	if err != nil {
		return fmt.Errorf("encode edited dataview: %w", err)
	}
	blocks, err := anyblockjson.UnmarshalBlock(raw, fullId, a.commitImportOptions())
	if err != nil {
		return invalidDocError(err)
	}
	if len(blocks) > 0 {
		if dv := blocks[0].GetDataview(); dv != nil {
			a.restoreUnauthoredViews(dv, fullId, plan)
		}
	}
	// the op replaces the dataview block whole, so the ids it may reuse are
	// the block's own subtree AND the view ids that block currently holds —
	// collectSubtreeIds carries both since §8.31, which is what lets the
	// surviving views keep their identities through the re-import
	if err := a.claimPayloadIds(blocks, collectSubtreeIds(a.st, fullId), func(string) string { return opPath }); err != nil {
		return err
	}
	a.replaceLive(false, blocks)
	return nil
}

// restoreUnauthoredViews overwrites imported views (or their unauthored
// sorts/filters) with clones of the live proto content, per the plan.
func (a *v2StateApplier) restoreUnauthoredViews(dv *model.BlockContentDataview, fullId string, plan viewCommitPlan) {
	live := a.st.Pick(fullId)
	if live == nil {
		return
	}
	liveDv := live.Model().GetDataview()
	if liveDv == nil {
		return
	}
	liveById := make(map[string]*model.BlockContentDataviewView, len(liveDv.Views))
	for _, v := range liveDv.Views {
		if v != nil {
			liveById[v.Id] = v
		}
	}
	for i, v := range dv.Views {
		if v == nil {
			continue
		}
		auth, isAuthored := plan.authored[v.Id]
		if !isAuthored {
			if lv := liveById[v.Id]; lv != nil {
				dv.Views[i] = cloneDataviewView(lv)
			}
			continue
		}
		if auth.restoreFrom == "" {
			continue // no live source — everything is op-constructed
		}
		lv := liveById[auth.restoreFrom]
		if lv == nil {
			continue
		}
		if !auth.sorts {
			dv.Views[i].Sorts = cloneDataviewView(lv).Sorts
		}
		if !auth.filters {
			dv.Views[i].Filters = cloneDataviewView(lv).Filters
		}
	}
}

// cloneDataviewView deep-copies one view proto (the pbtypes.CopyBlock
// marshal/unmarshal pattern).
func cloneDataviewView(in *model.BlockContentDataviewView) *model.BlockContentDataviewView {
	data, err := in.Marshal()
	if err != nil {
		return in
	}
	out := &model.BlockContentDataviewView{}
	if err := out.Unmarshal(data); err != nil {
		return in
	}
	return out
}

// applyViewSet validates and merges the view-level `set` channel into the
// view's JSON form. nameViaOp marks insert_view, whose name rides the op's
// required top-level field — set.name there would silently override it (or,
// as null, defeat the requirement), so it is rejected with the steer.
func (a *v2StateApplier) applyViewSet(set map[string]json.RawMessage, edited, view map[string]any, opPath string, nameViaOp bool, issues *[]v2model.Issue, keyUses *[]viewKeyUse) error {
	if len(set) == 0 {
		return nil
	}
	if _, hasString := set["filter"]; hasString {
		if _, hasStructured := set["filters"]; hasStructured {
			return v2model.AmbiguousInput("provide filter or filters, not both",
				v2model.Issue{Path: opPath + ".set.filter", Message: "conflicts with filters"},
				v2model.Issue{Path: opPath + ".set.filters", Message: "conflicts with filter"})
		}
	}
	for _, field := range sortedKeys(set) {
		path := opPath + ".set." + field
		raw := set[field]
		if nameViaOp && field == "name" {
			*issues = append(*issues, v2model.Issue{Path: path,
				Message: "insert_view's name is the op's required top-level field — drop set.name"})
			continue
		}
		switch field {
		case "id":
			*issues = append(*issues, v2model.Issue{Path: path,
				Message: "a view's id is immutable — address the view through the op's view field"})
			continue
		case "columns":
			*issues = append(*issues, v2model.Issue{Path: path,
				Message: "columns are not set wholesale — use the op's columns channel, keyed by property key, so one change never rewrites the array"})
			continue
		case "groups", "objectOrders":
			*issues = append(*issues, v2model.Issue{Path: path,
				Message: fmt.Sprintf("%q is output-only editor state (SPEC §4a) — export writes it, writes must not", field)})
			continue
		}
		kind, known := v2ViewFieldKinds[field]
		if !known {
			*issues = append(*issues, v2model.Issue{Path: path,
				Message: fmt.Sprintf("unknown view field %q", field),
				Hint:    "allowed: " + v2ViewSetFieldList()})
			continue
		}
		if isJSONNull(raw) {
			if field == "filter" {
				*issues = append(*issues, v2model.Issue{Path: path,
					Message: "filter is the compact-string input form, not a stored field — to clear the view's filters set filters to null"})
				continue
			}
			delete(view, field)
			continue
		}
		if err := a.applyViewSetField(field, kind, raw, edited, view, path, issues, keyUses); err != nil {
			return err
		}
	}
	return nil
}

// canonicalViewKey maps an inbound property key to the spelling the VIEW
// DOCUMENT uses (APIV2_ADDRESSING.md §7.5a). The view ops merge into exported
// JSON, which spells slugs, so a key that arrives as a stored key — or as a
// folded spelling, or as a bundled slug — has to be translated once, on the
// way in, or it addresses a column that is not there and writes a filter the
// store can never match.
//
// This closes the §8.22/§8.23 deferral ("key slots inside view-op set
// channels accept stored keys only"): every spelling the rest of the API
// accepts now works here too, and the one the listings advertise is the one
// the merge sees. Ambiguity returns candidates; a miss passes through
// verbatim so validateViewKeys owns the did-you-mean refusal in the caller's
// own spelling.
func (a *v2StateApplier) canonicalViewKey(input string) (string, []string) {
	if input == "" {
		return input, nil
	}
	entries, err := a.propEntries()
	if err != nil {
		return input, nil // the load error surfaces in validateViewKeys
	}
	entry, ok, ambiguous := a.s.resolvePropertyInput(input, entries)
	if len(ambiguous) > 0 {
		return input, ambiguous
	}
	if !ok || entry.Key == "" {
		return input, nil
	}
	keyTaken, slugHolders := servedPropertyKeySets(entries)
	return servedKey(entry.Key, entry.Slug, keyTaken, slugHolders), nil
}

// canonicalizeDecodedKeySlots rewrites every `property` slot in a decoded
// sorts/filters/columns array to the document spelling, recursively through
// filter groups. Ambiguous terms are left alone and reported.
func (a *v2StateApplier) canonicalizeDecodedKeySlots(nodes []any, path string, issues *[]v2model.Issue) {
	for i, raw := range nodes {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if nested, ok := node["filters"].([]any); ok {
			a.canonicalizeDecodedKeySlots(nested, fmt.Sprintf("%s[%d].filters", path, i), issues)
			continue
		}
		key, ok := node["property"].(string)
		if !ok || key == "" {
			continue
		}
		canonical, ambiguous := a.canonicalViewKey(key)
		if len(ambiguous) > 0 {
			*issues = append(*issues, v2model.Issue{Path: fmt.Sprintf("%s[%d].property", path, i),
				Message: fmt.Sprintf("%q matches %s", key, strings.Join(ambiguous, " and ")),
				Hint:    "address the intended one by its exact key"})
			continue
		}
		node["property"] = canonical
	}
}

// applyViewSetField validates one non-null `set` field per its kind and
// writes it into the view JSON. Kind failures append issues; only transport
// errors return.
func (a *v2StateApplier) applyViewSetField(field, kind string, raw json.RawMessage, edited, view map[string]any, path string, issues *[]v2model.Issue, keyUses *[]viewKeyUse) error {
	badType := func(want string) {
		*issues = append(*issues, v2model.Issue{Path: path,
			Message: fmt.Sprintf("%s takes %s", field, want)})
	}
	switch kind {
	case "name", "id", "propertyKey":
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			badType("a string")
			return nil
		}
		max := maxV2NameLength
		if kind != "name" {
			max = maxV2KeyLength
		}
		if length := utf8.RuneCountInString(s); length > max {
			*issues = append(*issues, v2model.Issue{Path: path,
				Message: fmt.Sprintf("%d characters — the cap is %d (the advertised maxLength)", length, max)})
			return nil
		}
		if kind == "propertyKey" {
			canonical, ambiguous := a.canonicalViewKey(s)
			if len(ambiguous) > 0 {
				*issues = append(*issues, v2model.Issue{Path: path,
					Message: fmt.Sprintf("%q matches %s", s, strings.Join(ambiguous, " and ")),
					Hint:    "address the intended one by its exact key"})
				return nil
			}
			s = canonical
			*keyUses = append(*keyUses, viewKeyUse{key: s, path: path})
		}
		view[field] = s
	case "bool":
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			badType("a boolean")
			return nil
		}
		view[field] = b
	case "int":
		var n float64
		if err := json.Unmarshal(raw, &n); err != nil || n != float64(int(n)) || n < 0 || n > maxV2ViewPageSize {
			badType(fmt.Sprintf("an integer between 0 and %d", maxV2ViewPageSize))
			return nil
		}
		view[field] = n
	case "viewType", "cardSize", "listSize":
		allowed := map[string][]string{
			"viewType": anyblockjson.ViewTypeNames(),
			"cardSize": anyblockjson.ViewCardSizeNames(),
			"listSize": anyblockjson.ViewListSizeNames(),
		}[kind]
		var s string
		if err := json.Unmarshal(raw, &s); err != nil || !slices.Contains(allowed, s) {
			badType("one of: " + strings.Join(allowed, ", "))
			return nil
		}
		view[field] = s
	case "sorts":
		return a.applyViewSorts(raw, view, path, issues, keyUses)
	case "filters":
		return a.applyViewFilters(raw, view, path, issues, keyUses)
	case "filterString":
		return a.applyViewFilterString(raw, edited, view, path, issues)
	}
	return nil
}

// applyViewSorts validates a §6.2 sorts array (vocabulary through the
// format's own fragment codec) and replaces the view's sorts.
func (a *v2StateApplier) applyViewSorts(raw json.RawMessage, view map[string]any, path string, issues *[]v2model.Issue, keyUses *[]viewKeyUse) error {
	var probes []sortProbe
	if err := json.Unmarshal(raw, &probes); err != nil {
		*issues = append(*issues, v2model.Issue{Path: path,
			Message: "sorts takes the SPEC §6.2 array of sort objects: " + err.Error()})
		return nil
	}
	if len(probes) > maxV2SetSorts {
		*issues = append(*issues, v2model.Issue{Path: path,
			Message: fmt.Sprintf("%d sorts — the cap is %d (the advertised maxItems)", len(probes), maxV2SetSorts)})
		return nil
	}
	// the advertised custom_order bound, enforced (M6: advertised = enforced)
	for i, probe := range probes {
		if len(probe.CustomOrder) > maxV2CustomOrderValues {
			*issues = append(*issues, v2model.Issue{
				Path:    fmt.Sprintf("%s[%d].custom_order", path, i),
				Message: fmt.Sprintf("%d entries — the cap is %d (the advertised maxItems)", len(probe.CustomOrder), maxV2CustomOrderValues)})
			return nil
		}
	}
	// the codec's own vocabulary validation (direction, emptyPlacement, the
	// missing-property rule) — resolution is READ-ONLY here; the actual
	// import happens on the block re-import
	if _, err := anyblockjson.UnmarshalSorts(raw, a.marshalOptions()); err != nil {
		appendCodecIssues(issues, err, path, "/sorts")
		return nil
	}
	var decoded []any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("re-decode sorts: %w", err)
	}
	a.canonicalizeDecodedKeySlots(decoded, path, issues)
	for i, node := range decoded {
		m, _ := node.(map[string]any)
		if key, _ := m["property"].(string); key != "" {
			*keyUses = append(*keyUses, viewKeyUse{key: key, path: fmt.Sprintf("%s[%d].property", path, i)})
		}
	}
	view["sorts"] = decoded
	return nil
}

// applyViewFilters validates a §6.2 structured filters array — the M3 shape
// gate first (a set PERSISTS its filter, so a malformed shape is not a bad
// query but a view that quietly matches everything, for good), then the
// codec's vocabulary and date-semantics checks — and replaces the view's
// filters. The §6.2 unguarded-date-comparison finding rides the C11 warnings
// channel, exactly as on document import.
func (a *v2StateApplier) applyViewFilters(raw json.RawMessage, view map[string]any, path string, issues *[]v2model.Issue, keyUses *[]viewKeyUse) error {
	nodes, err := decodeFilterNodes(raw, "")
	if err != nil {
		appendCodecIssues(issues, err, path, "")
		return nil
	}
	// the advertised top-level node bound, enforced (M6); nesting is the
	// documented C13 recursion exception, so only the top level is counted
	if len(nodes) > maxV2ViewFilterNodes {
		*issues = append(*issues, v2model.Issue{Path: path,
			Message: fmt.Sprintf("%d top-level filter nodes — the cap is %d (the advertised maxItems); group conditions under and/or nodes", len(nodes), maxV2ViewFilterNodes)})
		return nil
	}
	opts := a.marshalOptions()
	opts.OnWarning = func(iss anyblockjson.Issue) {
		a.warnings = append(a.warnings, v2model.Issue{
			Path:    rebaseSlashPath(path, strings.TrimPrefix(iss.Path, "/filters")),
			Message: iss.Message,
		})
	}
	if _, err := anyblockjson.UnmarshalFilters(raw, opts); err != nil {
		appendCodecIssues(issues, err, path, "/filters")
		return nil
	}
	var decoded []any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("re-decode filters: %w", err)
	}
	a.canonicalizeDecodedKeySlots(decoded, path, issues)
	collectDecodedKeys(decoded, path, keyUses)
	// §11 canonical form: empty/notEmpty/exists leaves carry no value —
	// stripping it here keeps the in-lock import from resolving (and possibly
	// creating) an option the view never uses, exactly mirroring what the
	// prewarm pass skips (resolver.go prewarmViewOptionValues)
	stripValuelessConditionValues(decoded)
	view["filters"] = decoded
	return nil
}

// stripValuelessConditionValues removes `value` from empty/notEmpty/exists
// leaves, recursively (§11 canonical form).
func stripValuelessConditionValues(nodes []any) {
	for _, raw := range nodes {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if nested, ok := node["filters"].([]any); ok {
			stripValuelessConditionValues(nested)
			continue
		}
		switch node["condition"] {
		case "empty", "notEmpty", "exists":
			delete(node, "value")
		}
	}
}

// applyViewFilterString parses the compact filter string (SPEC §6.2.1) into
// the structured array and stores it as the view's filters — the same split
// POST /sets makes. The parser's reference set is the WHOLE dataview's keys
// (properties list plus every view's columns — the same membership the
// structured channel validates against, so the two forms accept the same
// keys) plus the space's, so an existing column is always addressable even
// when the queried type does not recommend it.
func (a *v2StateApplier) applyViewFilterString(raw json.RawMessage, edited, view map[string]any, path string, issues *[]v2model.Issue) error {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		*issues = append(*issues, v2model.Issue{Path: path, Message: "filter takes a string (the compact filter syntax — GET /v2/schemas/filters serves the grammar)"})
		return nil
	}
	if length := utf8.RuneCountInString(s); length > maxV2FilterLength {
		*issues = append(*issues, v2model.Issue{Path: path,
			Message: fmt.Sprintf("%d characters — the cap is %d (the advertised maxLength)", length, maxV2FilterLength)})
		return nil
	}
	refKeys := appendMissing(a.s.knownPropertyKeys(a.spaceId), "name")
	refKeys = appendMissing(refKeys, v2SystemQueryKeys...)
	for key := range dataviewMembership(edited) {
		refKeys = appendMissing(refKeys, key)
	}
	sort.Strings(refKeys)
	parsed, err := filterstring.Parse(s, filterstring.Options{
		KnownKeys:     refKeys,
		ResolveFormat: a.s.formatNameResolver(a.spaceId),
	})
	if err != nil {
		fsErr := filterStringError(err)
		var v2Err *v2model.Error
		if errors.As(fsErr, &v2Err) {
			for i := range v2Err.Issues {
				v2Err.Issues[i].Path = path
			}
			*issues = append(*issues, v2Err.Issues...)
			return nil
		}
		*issues = append(*issues, v2model.Issue{Path: path, Message: err.Error()})
		return nil
	}
	var decoded []any
	if err := json.Unmarshal(parsed, &decoded); err != nil {
		return fmt.Errorf("decode parsed filter: %w", err)
	}
	// the parser accepts the SERVED spellings (its KnownKeys are those), and
	// the compact string is fold-strict only in what it accepts — what it
	// EMITS still has to be the document's spelling, like every other channel
	a.canonicalizeDecodedKeySlots(decoded, path, issues)
	view["filters"] = decoded
	return nil
}

// applyViewColumns validates and merges the per-column channel.
func (a *v2StateApplier) applyViewColumns(patches map[string]json.RawMessage, view map[string]any, opPath string, issues *[]v2model.Issue, keyUses *[]viewKeyUse) error {
	if len(patches) == 0 {
		return nil
	}
	if len(patches) > maxV2ViewColumns {
		return v2model.ValidationFailed("too many columns in one update_view",
			v2model.Issue{Path: opPath + ".columns",
				Message: fmt.Sprintf("%d column patches — the cap is %d (the advertised maxProperties)", len(patches), maxV2ViewColumns)})
	}
	columns, _ := view["columns"].([]any)
	indexOf := func(key string) int {
		for i, raw := range columns {
			if col, ok := raw.(map[string]any); ok {
				if prop, _ := col["property"].(string); prop == key {
					return i
				}
			}
		}
		return -1
	}
	for _, rawKey := range sortedKeys(patches) {
		path := opPath + ".columns." + rawKey
		raw := patches[rawKey]
		key, ambiguous := a.canonicalViewKey(rawKey)
		if len(ambiguous) > 0 {
			*issues = append(*issues, v2model.Issue{Path: path,
				Message: fmt.Sprintf("%q matches %s", rawKey, strings.Join(ambiguous, " and ")),
				Hint:    "address the intended one by its exact key"})
			continue
		}
		ci := indexOf(key)
		if isJSONNull(raw) {
			// remove the column; removing an absent one is a no-op (the unset
			// precedent) — a stale column for a deleted property must stay
			// removable, so removal is NOT key-validated against the space
			if ci >= 0 {
				columns = append(columns[:ci], columns[ci+1:]...)
			}
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			*issues = append(*issues, v2model.Issue{Path: path,
				Message: "a column patch is an object ({hidden, width, align, aggregation}) or null to remove the column"})
			continue
		}
		var col map[string]any
		if ci >= 0 {
			col, _ = columns[ci].(map[string]any)
		} else {
			// a new column references its property — validated with the rest
			col = map[string]any{"property": key}
			*keyUses = append(*keyUses, viewKeyUse{key: key, path: path})
		}
		if col == nil {
			return fmt.Errorf("column %d is not an object", ci)
		}
		ok := true
		for _, field := range sortedKeys(fields) {
			if !a.applyColumnField(col, field, fields[field], path+"."+field, issues) {
				ok = false
			}
		}
		if !ok {
			continue
		}
		if ci >= 0 {
			columns[ci] = col
		} else {
			columns = append(columns, col)
		}
	}
	if columns == nil {
		// a view with no columns key that only saw removal no-ops: writing an
		// explicit null would make the re-import reject the block
		delete(view, "columns")
	} else {
		view["columns"] = columns
	}
	return nil
}

// applyColumnField validates one column-patch field and merges it (explicit
// null clears the field back to its default). Returns false when the field
// was rejected.
func (a *v2StateApplier) applyColumnField(col map[string]any, field string, raw json.RawMessage, path string, issues *[]v2model.Issue) bool {
	reject := func(msg string) bool {
		*issues = append(*issues, v2model.Issue{Path: path, Message: msg})
		return false
	}
	if !v2ColumnFields[field] {
		return reject(fmt.Sprintf("unknown column field %q — allowed: hidden, width, align, aggregation", field))
	}
	if isJSONNull(raw) {
		delete(col, field)
		return true
	}
	switch field {
	case "hidden":
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return reject("hidden takes a boolean")
		}
		col[field] = b
	case "width":
		var n float64
		if err := json.Unmarshal(raw, &n); err != nil || n != float64(int(n)) || n < 0 || n > maxV2ColumnWidth {
			return reject(fmt.Sprintf("width takes an integer between 0 and %d (pixels; omit it to let the client pick per format — SPEC §6.2)", maxV2ColumnWidth))
		}
		col[field] = n
	case "align":
		var s string
		if err := json.Unmarshal(raw, &s); err != nil || !slices.Contains(anyblockjson.ColumnAlignNames(), s) {
			return reject("align takes one of: " + strings.Join(anyblockjson.ColumnAlignNames(), ", "))
		}
		col[field] = s
	case "aggregation":
		var s string
		if err := json.Unmarshal(raw, &s); err != nil || !slices.Contains(anyblockjson.ColumnAggregationNames(), s) {
			return reject("aggregation takes one of: " + strings.Join(anyblockjson.ColumnAggregationNames(), ", "))
		}
		col[field] = s
	}
	return true
}

// dataviewMembership collects the property keys a dataview block already
// knows: its properties list plus every view's columns.
func dataviewMembership(edited map[string]any) map[string]bool {
	known := map[string]bool{}
	props, _ := edited["properties"].([]any)
	for _, raw := range props {
		if p, ok := raw.(map[string]any); ok {
			if key, _ := p["key"].(string); key != "" {
				known[key] = true
			}
		}
	}
	allViews, _ := edited["views"].([]any)
	for _, rawView := range allViews {
		view, ok := rawView.(map[string]any)
		if !ok {
			continue
		}
		cols, _ := view["columns"].([]any)
		for _, raw := range cols {
			if col, ok := raw.(map[string]any); ok {
				if key, _ := col["property"].(string); key != "" {
					known[key] = true
				}
			}
		}
	}
	return known
}

// validateViewKeys checks every referenced property key against the
// dataview's pre-merge membership and the space (did-you-mean otherwise),
// and appends resolvable keys missing from the dataview's properties list so
// formats rehydrate on import. Deliberately NOT tightened to the queried
// type's recommended keys (the POST /sets R9 rule): generated views already
// carry columns outside that set (backlinks, lastModifiedBy, …), and an edit
// of an existing surface must not reject what the surface already shows
// (§8.17).
func (a *v2StateApplier) validateViewKeys(edited map[string]any, preKnown map[string]bool, keyUses []viewKeyUse, issues *[]v2model.Issue) {
	if len(keyUses) == 0 {
		return
	}
	props, _ := edited["properties"].([]any)
	entries, err := a.propEntries() // primed once per PATCH (§7.5a-2)
	if err != nil {
		*issues = append(*issues, v2model.Issue{Message: err.Error()})
		return
	}
	var known []string
	seen := map[string]bool{}
	for _, use := range keyUses {
		if seen[use.key] {
			continue
		}
		seen[use.key] = true
		if preKnown[use.key] {
			continue
		}
		if propertyKeyExistsIn(entries, use.key) {
			if !propertyKeyInstalledIn(entries, use.key) {
				// the key exists only through the bundled table — the same
				// removal gate every other write channel runs (§8.41). Before
				// this, only the slug≠key bundled class was refused (as an
				// accidental unknown-key), while `tag`, `status` and the other
				// 41 keys whose derived slug equals the key sailed straight
				// into columns, groupBy, filters and sorts of a property the
				// user had deleted.
				refused, err := a.refusesRemovedBundled(entries, use.key)
				if err != nil {
					*issues = append(*issues, v2model.Issue{Path: use.path, Message: err.Error()})
					continue
				}
				if refused {
					*issues = append(*issues, removedPropertyIssue(a.spaceId, use.key, use.key, use.path))
					continue
				}
			}
			props = append(props, map[string]any{
				"key":    use.key,
				"format": anyblockjson.FormatName(a.propertyFormat(use.key)),
			})
			continue
		}
		// a bundled DERIVED slug that no longer resolves may be the removed
		// relation's view spelling — say "removed", not "unknown key" with a
		// did-you-mean pointing somewhere else (§8.41-10)
		if stored, ok := bundle.RelationKeyByApiSlug(use.key); ok {
			refused, err := a.refusesRemovedBundled(entries, string(stored))
			if err == nil && refused {
				*issues = append(*issues, removedPropertyIssue(a.spaceId, string(stored), use.key, use.path))
				continue
			}
		}
		if known == nil {
			known = knownPropertyKeysIn(entries)
		}
		*issues = append(*issues, unknownPropertyIssue(use.key, use.path, known,
			fmt.Sprintf("list all with GET /v2/spaces/%s/properties, or create it with POST /v2/spaces/%s/properties", a.spaceId, a.spaceId)))
	}
	edited["properties"] = props
}

// resolveDataviewBlock resolves the op's dataview target: an explicit block
// reference (which must be a dataview), or — omitted — the object's only
// dataview block.
func (a *v2StateApplier) resolveDataviewBlock(doc *v2EditDoc, ref, opPath string) (int, error) {
	if ref != "" {
		idx, err := a.resolveRef(doc, ref, opPath+".block")
		if err != nil {
			return -1, err
		}
		if typ := blockType(doc.blocks[idx]); typ != "dataview" {
			return -1, v2model.ValidationFailed(
				fmt.Sprintf("block %q is a %q block, not a dataview", ref, typ),
				v2model.Issue{Path: opPath + ".block", Message: "update_view addresses a dataview block (SPEC §6.2)"})
		}
		return idx, nil
	}
	var found []int
	for i, b := range doc.blocks {
		if blockType(b) == "dataview" {
			found = append(found, i)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return -1, v2model.ValidationFailed("this object has no dataview block",
			v2model.Issue{Path: opPath,
				Message: "views live in dataview blocks — types, sets and collections carry one",
				Hint:    "GET the object (?outline=true) to inspect its blocks"})
	default:
		ids := make([]string, len(found))
		for i, bi := range found {
			ids[i] = blockId(doc.blocks[bi])
		}
		return -1, v2model.AmbiguousInput(
			fmt.Sprintf("this object has %d dataview blocks — name one with the op's block field: %s", len(found), strings.Join(ids, ", ")),
			v2model.Issue{Path: opPath + ".block", Message: "block is required when the object has more than one dataview"})
	}
}

// viewIdList extracts view ids and the `id ("name")` listing every view
// error carries, so a repair needs no second read.
func viewIdList(views []any) (ids, listed []string) {
	ids = make([]string, len(views))
	listed = make([]string, len(views))
	for i, raw := range views {
		view, _ := raw.(map[string]any)
		id, _ := view["id"].(string)
		name, _ := view["name"].(string)
		ids[i] = id
		listed[i] = fmt.Sprintf("%s (%q)", id, name)
	}
	return ids, listed
}

// matchViewRef resolves a NON-EMPTY view reference by full id or unique
// suffix (resolveViewRef's rule on the read surface). path names the field
// the reference came from.
func matchViewRef(views []any, ref, path string) (int, error) {
	ids, listed := viewIdList(views)
	idx, matches := matchBlockRef(ids, ref)
	switch {
	case matches == 1:
		return idx, nil
	case matches > 1:
		return -1, v2model.AmbiguousInput(
			fmt.Sprintf("view reference %q matches more than one view — use the full view id", ref),
			v2model.Issue{Path: path, Message: "the reference is a suffix of several view ids"})
	default:
		return -1, v2model.NotFound(
			fmt.Sprintf("view %q not found — views: %s", ref, strings.Join(listed, ", ")))
	}
}

// resolveViewIndex resolves the op's view target within one dataview's
// views: an explicit id (full or unique suffix), or — omitted — the
// dataview's only view.
func resolveViewIndex(views []any, ref, opPath string) (int, error) {
	if len(views) == 0 {
		return -1, v2model.NotFound("this dataview has no views")
	}
	if ref == "" {
		if len(views) == 1 {
			return 0, nil
		}
		_, listed := viewIdList(views)
		return -1, v2model.AmbiguousInput(
			fmt.Sprintf("this dataview has %d views — name one with the op's view field: %s", len(views), strings.Join(listed, ", ")),
			v2model.Issue{Path: opPath + ".view", Message: "view is required when the dataview has more than one view"})
	}
	return matchViewRef(views, ref, opPath+".view")
}

// resolveViewListTarget maps the after/before/position targeting vocabulary
// to an insertion index in the views list. Views are a flat ordered list —
// there is no `inside` — and the FIRST view is the client's default tab, so
// position "first" is the "make this the default" verb. Omitting all three
// appends (the move_block root-append precedent).
func (a *v2StateApplier) resolveViewListTarget(views []any, after, before, position, opPath string) (int, error) {
	var fields []string
	if after != "" {
		fields = append(fields, "after")
	}
	if before != "" {
		fields = append(fields, "before")
	}
	if position != "" {
		fields = append(fields, "position")
	}
	if len(fields) > 1 {
		return -1, v2model.AmbiguousInput("at most one of after, before, position is allowed",
			v2model.Issue{Path: opPath, Message: fmt.Sprintf("got %d targeting fields (%s)", len(fields), strings.Join(fields, ", "))})
	}
	switch {
	case after != "":
		idx, err := matchViewRef(views, after, opPath+".after")
		if err != nil {
			return -1, err
		}
		return idx + 1, nil
	case before != "":
		idx, err := matchViewRef(views, before, opPath+".before")
		if err != nil {
			return -1, err
		}
		return idx, nil
	case position == "first":
		return 0, nil
	case position == "last", position == "":
		return len(views), nil
	default:
		return -1, v2model.ValidationFailed("invalid position",
			v2model.Issue{Path: opPath + ".position", Message: fmt.Sprintf("unknown position %q", position), Hint: "allowed: first, last"})
	}
}

// collectNodeKeys walks a filter tree for leaf property keys.
// collectDecodedKeys is collectNodeKeys over the DECODED (and by then
// canonicalized) filter array — the keys the merge will actually write.
func collectDecodedKeys(nodes []any, path string, keyUses *[]viewKeyUse) {
	for i, raw := range nodes {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		nodePath := fmt.Sprintf("%s[%d]", path, i)
		if nested, ok := node["filters"].([]any); ok {
			collectDecodedKeys(nested, nodePath+".filters", keyUses)
			continue
		}
		if key, _ := node["property"].(string); key != "" {
			*keyUses = append(*keyUses, viewKeyUse{key: key, path: nodePath + ".property"})
		}
	}
}

func collectNodeKeys(nodes []searchFilterNode, path string, keyUses *[]viewKeyUse) {
	for i, node := range nodes {
		nodePath := fmt.Sprintf("%s[%d]", path, i)
		if node.isGroup() {
			collectNodeKeys(node.Filters, nodePath+".filters", keyUses)
			continue
		}
		if node.Property != "" {
			*keyUses = append(*keyUses, viewKeyUse{key: node.Property, path: nodePath + ".property"})
		}
	}
}

// appendCodecIssues rebases a codec/gate error's issues onto the op's field
// path and appends them. stripPrefix removes the codec's own root segment
// ("/filters", "/sorts") before rebasing.
func appendCodecIssues(issues *[]v2model.Issue, err error, base, stripPrefix string) {
	var v2Err *v2model.Error
	if errors.As(err, &v2Err) {
		for _, iss := range v2Err.Issues {
			iss.Path = rebaseSlashPath(base, strings.TrimPrefix(iss.Path, stripPrefix))
			*issues = append(*issues, iss)
		}
		return
	}
	var ve *anyblockjson.ValidationError
	if errors.As(err, &ve) {
		for _, iss := range ve.Issues {
			*issues = append(*issues, v2model.Issue{
				Path:    rebaseSlashPath(base, strings.TrimPrefix(iss.Path, stripPrefix)),
				Message: iss.Message,
			})
		}
		return
	}
	*issues = append(*issues, v2model.Issue{Path: base, Message: err.Error()})
}

// rebaseSlashPath maps a slash-addressed sub-path ("/0/condition") onto an
// op-style base ("ops[2].set.filters" → "ops[2].set.filters[0].condition").
// Numeric segments become indexes, the rest dots.
func rebaseSlashPath(base, path string) string {
	out := base
	for _, seg := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if seg == "" {
			continue
		}
		if isAllDigits(seg) {
			out += "[" + seg + "]"
		} else {
			out += "." + seg
		}
	}
	return out
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// deepCopyBlock deep-copies one view-doc block through JSON.
func deepCopyBlock(block map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(block)
	if err != nil {
		return nil, fmt.Errorf("copy block: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("copy block: %w", err)
	}
	return out, nil
}

// isJSONNull reports an explicit JSON null.
func isJSONNull(raw json.RawMessage) bool {
	return string(raw) == "null"
}

//
// ---- insert_view / move_view / delete_view (§8.18) ----
//

func (a *v2StateApplier) applyInsertView(op opInsertView, opPath string) error {
	if op.Name == "" {
		return v2model.ValidationFailed("a view needs a name",
			v2model.Issue{Path: opPath + ".name", Message: "name is required — a view is a named tab",
				Hint: "GET /v2/schemas/ops/insert_view for the op's schema and example"})
	}
	if length := utf8.RuneCountInString(op.Name); length > maxV2NameLength {
		return v2model.ValidationFailed("name is too long",
			v2model.Issue{Path: opPath + ".name",
				Message: fmt.Sprintf("%d characters — the cap is %d (the advertised maxLength)", length, maxV2NameLength)})
	}
	doc, err := a.doc()
	if err != nil {
		return err
	}
	idx, err := a.resolveDataviewBlock(doc, op.Block, opPath)
	if err != nil {
		return err
	}
	fullId := blockId(doc.blocks[idx])
	edited, err := deepCopyBlock(doc.blocks[idx])
	if err != nil {
		return err
	}
	delete(edited, "indent")
	views, _ := edited["views"].([]any)
	insertAt, err := a.resolveViewListTarget(views, op.After, op.Before, op.Position, opPath)
	if err != nil {
		return err
	}

	var issues []v2model.Issue
	var keyUses []viewKeyUse
	preKnown := dataviewMembership(edited)

	var view map[string]any
	var copySourceId string
	if op.CopyFrom != "" {
		si, err := matchViewRef(views, op.CopyFrom, opPath+".copy_from")
		if err != nil {
			return err
		}
		src, ok := views[si].(map[string]any)
		if !ok {
			return fmt.Errorf("block %s: view %d is not an object", fullId, si)
		}
		copySourceId, _ = src["id"].(string)
		// duplicate everything but the identity: columns, sorts, filters,
		// type, groupBy, card options, even the per-view editor state — the
		// §6.2 form nests groups/objectOrders per view, so they re-key to the
		// new view id on import
		if view, err = deepCopyBlock(src); err != nil {
			return err
		}
		delete(view, "id")
	} else {
		// the bare default: every property the dataview lists, VISIBLE (the
		// GO-5969 lesson — the native CreateView default hides everything but
		// name), ordered latest-first like the native default sort.
		// last_modified_date is bundled and rides NO keyUse: the properties-list
		// top-up would make the bare default self-modifying (a second bare
		// insert in the same batch would grow an extra column), and the sort's
		// format resolves from the store/bundle without a list entry.
		props, _ := edited["properties"].([]any)
		columns := make([]any, 0, len(props))
		for _, raw := range props {
			if p, ok := raw.(map[string]any); ok {
				if key, _ := p["key"].(string); key != "" {
					columns = append(columns, map[string]any{"property": key})
				}
			}
		}
		if len(columns) == 0 {
			columns = []any{map[string]any{"property": "name"}}
		}
		view = map[string]any{
			"columns": columns,
			// the document vocabulary is slugs (§7.5a) — this literal is
			// written straight into the view JSON, so it is spelled the way
			// the export spells it
			"sorts": []any{map[string]any{"property": bundle.ApiSlug(bundle.RelationKeyLastModifiedDate.String()), "direction": "desc"}},
		}
	}
	view["name"] = op.Name

	// set/columns merge on top of the base — update_view's exact channels, so
	// create is "update_view aimed at a fresh view" (name rides the op's own
	// required field, so set.name is rejected — nameViaOp)
	if err := a.applyViewSet(op.Set, edited, view, opPath, true, &issues, &keyUses); err != nil {
		return err
	}
	if err := a.applyViewColumns(op.Columns, view, opPath, &issues, &keyUses); err != nil {
		return err
	}
	a.validateViewKeys(edited, preKnown, keyUses, &issues)
	if len(issues) > 0 {
		return v2model.ValidationFailed("insert_view rejected", issues...)
	}

	newId := a.mintViewId(views)
	view["id"] = newId
	views = slices.Insert(views, insertAt, any(view))
	edited["views"] = views
	if err := a.commitDataviewBlock(edited, fullId, opPath, viewCommitPlan{
		authored: map[string]viewAuthored{newId: {
			restoreFrom: copySourceId, // "" for a bare insert — nothing live to restore
			sorts:       setNames(op.Set, "sorts") || copySourceId == "",
			filters:     setNames(op.Set, "filters") || setNames(op.Set, "filter") || copySourceId == "",
		}},
	}); err != nil {
		return err
	}
	a.createdViews[opPath] = newId
	return nil
}

func (a *v2StateApplier) applyMoveView(op opMoveView, opPath string) error {
	if op.View == "" {
		return v2model.ValidationFailed("view is required",
			v2model.Issue{Path: opPath + ".view", Message: "move_view moves one view — name it by id (full or unique suffix)"})
	}
	doc, err := a.doc()
	if err != nil {
		return err
	}
	idx, err := a.resolveDataviewBlock(doc, op.Block, opPath)
	if err != nil {
		return err
	}
	fullId := blockId(doc.blocks[idx])
	edited, err := deepCopyBlock(doc.blocks[idx])
	if err != nil {
		return err
	}
	delete(edited, "indent")
	views, _ := edited["views"].([]any)
	mi, err := matchViewRef(views, op.View, opPath+".view")
	if err != nil {
		return err
	}
	// a destination is required (unlike insert_view's append default and
	// move_block's end default): the end of the views list has no special
	// meaning while its FRONT is the default tab, so a target-less move_view
	// is far more likely a forgotten field than an intent — and it would
	// silently change which view a fresh client opens (§8.19)
	if op.After == "" && op.Before == "" && op.Position == "" {
		return v2model.ValidationFailed("move_view needs a destination",
			v2model.Issue{Path: opPath,
				Message: "give one of after, before, position",
				Hint:    `position "first" makes the view the default tab; "last" moves it to the end`})
	}
	target, err := a.resolveViewListTarget(views, op.After, op.Before, op.Position, opPath)
	if err != nil {
		return err
	}
	moved := views[mi]
	views = slices.Delete(views, mi, mi+1)
	if target > mi {
		target--
	}
	views = slices.Insert(views, target, moved)
	edited["views"] = views
	// move_view authors nothing inside any view — every view is restored from
	// the live proto; only the ORDER comes from the splice
	return a.commitDataviewBlock(edited, fullId, opPath, viewCommitPlan{})
}

func (a *v2StateApplier) applyDeleteView(op opDeleteView, opPath string) error {
	if op.View == "" {
		return v2model.ValidationFailed("view is required",
			v2model.Issue{Path: opPath + ".view", Message: "delete_view deletes one view — name it by id (full or unique suffix)"})
	}
	doc, err := a.doc()
	if err != nil {
		return err
	}
	idx, err := a.resolveDataviewBlock(doc, op.Block, opPath)
	if err != nil {
		return err
	}
	fullId := blockId(doc.blocks[idx])
	edited, err := deepCopyBlock(doc.blocks[idx])
	if err != nil {
		return err
	}
	delete(edited, "indent")
	views, _ := edited["views"].([]any)
	vi, err := matchViewRef(views, op.View, opPath+".view")
	if err != nil {
		return err
	}
	// the native DeleteView guard, as a clean C6 refusal: a dataview with
	// zero views is a corrupt surface (the editor would regenerate a default
	// on open, sync permitting — do not rely on it)
	if len(views) <= 1 {
		return v2model.ValidationFailed("cannot delete the last view",
			v2model.Issue{Path: opPath + ".view",
				Message: "a dataview needs at least one view",
				Hint:    "insert_view a replacement first, or update_view to fix this one in place"})
	}
	// per-view editor state (groups, objectOrders) nests inside the view in
	// the §6.2 form, so it vanishes with it — no orphaned group orders. A
	// client whose locally stored active view this was falls back to the
	// first view (activeView is local UI state, §6.2).
	views = slices.Delete(views, vi, vi+1)
	edited["views"] = views
	// delete_view authors nothing inside the surviving views — all restored
	// from the live proto; only the membership comes from the splice
	return a.commitDataviewBlock(edited, fullId, opPath, viewCommitPlan{})
}

// mintViewId mints a view id unused by the state, this PATCH, and the given
// views list.
func (a *v2StateApplier) mintViewId(views []any) string {
	ids, _ := viewIdList(views)
	existing := make(map[string]bool, len(ids))
	for _, id := range ids {
		existing[id] = true
	}
	for {
		id := a.mintBlockId()
		if !existing[id] {
			return id
		}
	}
}
