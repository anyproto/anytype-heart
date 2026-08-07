package v2service

// viewops.go implements the updateView PATCH op (APIV2.md §8.17): the
// targeted edit of ONE dataview view — the write path for view and column
// configuration that GET …/views could read but nothing could change. The
// op speaks SPEC §6.2's vocabulary throughout: the merge happens on the
// block's exported JSON form and the result re-imports through the format
// codec, exactly the setCell pattern for tables — untouched views, columns
// and editor state (groups, objectOrders) round-trip unchanged.
//
// Two merge channels, both scoped so one flip never rewrites an array (the
// documented small-model trap):
//   - `set` merges §6.2 view-level fields (updateBlock semantics: named
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
)

// maxV2ViewColumns bounds one updateView's columns map — the maxProperties
// the served op schema advertises (M6: advertised bounds are enforced).
const maxV2ViewColumns = 64

// maxV2ViewPageSize bounds pageSize — the maximum the served schema
// advertises.
const maxV2ViewPageSize = 1000

// maxV2ColumnWidth bounds a column width in pixels (SPEC §6.2: the editor's
// own drag-resize stays within 54…1000; the schema advertises this cap).
const maxV2ColumnWidth = 10000

// v2ViewFieldKinds maps each authorable §6.2 view-level field to its value
// kind. groups and objectOrders are output-only editor state (§4a) and id is
// immutable — all three get targeted rejections; anything else unknown gets
// the allowed-field listing.
var v2ViewFieldKinds = map[string]string{
	"name":              "name",
	"type":              "viewType",
	"groupBy":           "propertyKey",
	"coverProperty":     "propertyKey",
	"endProperty":       "propertyKey",
	"hideIcon":          "bool",
	"cardSize":          "cardSize",
	"coverFit":          "bool",
	"coloredGroups":     "bool",
	"pageSize":          "int",
	"defaultTemplateId": "id",
	"defaultTypeId":     "id",
	"wrapContent":       "bool",
	"listSize":          "listSize",
	"alternateRows":     "bool",
	"sorts":             "sorts",
	"filters":           "filters",
	"filter":            "filterString",
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

// viewKeyUse records one property key an updateView references, with the
// issue path a rejection should carry.
type viewKeyUse struct {
	key  string
	path string
}

func (a *v2StateApplier) applyUpdateView(op opUpdateView, opPath string) error {
	if len(op.Set) == 0 && len(op.Columns) == 0 {
		return v2model.ValidationFailed("updateView needs set and/or columns",
			v2model.Issue{Path: opPath, Message: "set (view-level fields) and columns (per-column patches) are both empty",
				Hint: "GET /v2/schemas/ops/updateView for the op's schema and example"})
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

	if err := a.applyViewSet(op.Set, view, opPath, &issues, &keyUses); err != nil {
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
		return v2model.ValidationFailed("updateView rejected", issues...)
	}

	views[vi] = view
	edited["views"] = views
	raw, err := json.Marshal(edited)
	if err != nil {
		return fmt.Errorf("encode edited dataview: %w", err)
	}
	blocks, err := anyblockjson.UnmarshalBlock(raw, fullId, a.importOptions())
	if err != nil {
		return invalidDocError(err)
	}
	if err := a.checkFreshIds(blocks, map[string]bool{fullId: true}, func(string) string { return opPath }); err != nil {
		return err
	}
	a.replaceLive(false, blocks)
	return nil
}

// applyViewSet validates and merges the view-level `set` channel into the
// view's JSON form.
func (a *v2StateApplier) applyViewSet(set map[string]json.RawMessage, view map[string]any, opPath string, issues *[]v2model.Issue, keyUses *[]viewKeyUse) error {
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
		if err := a.applyViewSetField(field, kind, raw, view, path, issues, keyUses); err != nil {
			return err
		}
	}
	return nil
}

// applyViewSetField validates one non-null `set` field per its kind and
// writes it into the view JSON. Kind failures append issues; only transport
// errors return.
func (a *v2StateApplier) applyViewSetField(field, kind string, raw json.RawMessage, view map[string]any, path string, issues *[]v2model.Issue, keyUses *[]viewKeyUse) error {
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
		return a.applyViewFilterString(raw, view, path, issues)
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
	// the codec's own vocabulary validation (direction, emptyPlacement, the
	// missing-property rule) — resolution is READ-ONLY here; the actual
	// import happens on the block re-import with the creating resolvers
	if _, err := anyblockjson.UnmarshalSorts(raw, a.marshalOptions()); err != nil {
		appendCodecIssues(issues, err, path, "/sorts")
		return nil
	}
	for i, probe := range probes {
		if probe.Property != "" {
			*keyUses = append(*keyUses, viewKeyUse{key: probe.Property, path: fmt.Sprintf("%s[%d].property", path, i)})
		}
	}
	var decoded []any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("re-decode sorts: %w", err)
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
	collectNodeKeys(nodes, path, keyUses)
	var decoded []any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return fmt.Errorf("re-decode filters: %w", err)
	}
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
// POST /sets makes. The parser's reference set is the dataview's own keys
// plus the space's, so an existing column is always addressable even when
// the queried type does not recommend it.
func (a *v2StateApplier) applyViewFilterString(raw json.RawMessage, view map[string]any, path string, issues *[]v2model.Issue) error {
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
	refKeys = appendMissing(refKeys, a.dataviewKeys(view)...)
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
	view["filters"] = decoded
	return nil
}

// applyViewColumns validates and merges the per-column channel.
func (a *v2StateApplier) applyViewColumns(patches map[string]json.RawMessage, view map[string]any, opPath string, issues *[]v2model.Issue, keyUses *[]viewKeyUse) error {
	if len(patches) == 0 {
		return nil
	}
	if len(patches) > maxV2ViewColumns {
		return v2model.ValidationFailed("too many columns in one updateView",
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
	for _, key := range sortedKeys(patches) {
		path := opPath + ".columns." + key
		raw := patches[key]
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
	view["columns"] = columns
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
		if a.s.propertyKeyExists(a.spaceId, use.key) {
			props = append(props, map[string]any{
				"key":    use.key,
				"format": anyblockjson.FormatName(a.propertyFormat(use.key)),
			})
			continue
		}
		if known == nil {
			known = a.s.knownPropertyKeys(a.spaceId)
		}
		*issues = append(*issues, unknownPropertyIssue(use.key, use.path, known,
			fmt.Sprintf("list all with GET /v2/spaces/%s/properties, or create it with POST /v2/spaces/%s/properties", a.spaceId, a.spaceId)))
	}
	edited["properties"] = props
}

// dataviewKeys lists the property keys the dataview itself knows: its
// properties list plus this view's columns.
func (a *v2StateApplier) dataviewKeys(view map[string]any) []string {
	var keys []string
	seen := map[string]bool{}
	add := func(key string) {
		if key != "" && !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	cols, _ := view["columns"].([]any)
	for _, raw := range cols {
		if col, ok := raw.(map[string]any); ok {
			key, _ := col["property"].(string)
			add(key)
		}
	}
	return keys
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
				v2model.Issue{Path: opPath + ".block", Message: "updateView addresses a dataview block (SPEC §6.2)"})
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

// resolveViewIndex resolves the op's view target within one dataview's
// views: an explicit id (full or unique suffix — resolveViewRef's rule on
// the read surface), or — omitted — the dataview's only view. Errors list
// every view as `id ("name")` so the repair needs no second read.
func resolveViewIndex(views []any, ref, opPath string) (int, error) {
	if len(views) == 0 {
		return -1, v2model.NotFound("this dataview has no views")
	}
	listed := make([]string, len(views))
	ids := make([]string, len(views))
	for i, raw := range views {
		view, _ := raw.(map[string]any)
		id, _ := view["id"].(string)
		name, _ := view["name"].(string)
		ids[i] = id
		listed[i] = fmt.Sprintf("%s (%q)", id, name)
	}
	if ref == "" {
		if len(views) == 1 {
			return 0, nil
		}
		return -1, v2model.AmbiguousInput(
			fmt.Sprintf("this dataview has %d views — name one with the op's view field: %s", len(views), strings.Join(listed, ", ")),
			v2model.Issue{Path: opPath + ".view", Message: "view is required when the dataview has more than one view"})
	}
	idx, matches := matchBlockRef(ids, ref)
	switch {
	case matches == 1:
		return idx, nil
	case matches > 1:
		return -1, v2model.AmbiguousInput(
			fmt.Sprintf("view reference %q matches more than one view — use the full view id", ref),
			v2model.Issue{Path: opPath + ".view", Message: "the reference is a suffix of several view ids"})
	default:
		return -1, v2model.NotFound(
			fmt.Sprintf("view %q not found — views: %s", ref, strings.Join(listed, ", ")))
	}
}

// collectNodeKeys walks a filter tree for leaf property keys.
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
