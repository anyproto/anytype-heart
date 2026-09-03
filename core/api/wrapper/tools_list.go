package wrapper

// tools_list.go — what `read` answers when the object is a Query or a
// Collection.
//
// A list object's document is almost entirely dataview machinery: structured
// filter nodes, view configuration, column arrays. Served verbatim it tells a
// small model what the object IS shaped like and nothing it can act on — the
// rows are not in it at all (they are computed), and the filter is in a
// vocabulary no tool on this surface accepts. So `read` answers a list in the
// two things the caller can use:
//
//   - the DEFINITION, in the writing tools' own vocabulary: the source type
//     NAME (find's `type`), the filter as the compact filter STRING (find's
//     and update_view's `filter`), the sort in update_view's `sort` grammar.
//     What a tool prints must be accepted as what a tool takes — the rendered
//     string is parsed back through the same parser the server uses before it
//     is printed, and dropped if it does not survive the round trip.
//   - the ROWS, as numbered handles, so the objects a query MATCHES can be
//     addressed by the same numbers `find` hands out. This is the
//     cross-object half of the cliff: a model that can read a list but cannot
//     address its members has to re-find every one of them.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
)

// listKind is which of the two list routes reads an object's rows.
type listKind string

const (
	listKindQuery      listKind = "query"
	listKindCollection listKind = "collection"
)

// queryTypeKey / collectionTypeKey are the STORED type keys of the two list
// kinds. The product renamed the surface noun only: a "Query" is still keyed
// `set` everywhere below the API (v2service list_read.go says the same about
// the layout), so the detection has to know both spellings — the key, and
// whatever the space's own type listing names it, because the wrapper reads
// documents in the name vocabulary (D5) and a served `type` is therefore the
// display name.
const (
	queryTypeKey      = "set"
	collectionTypeKey = "collection"
)

// listRoutes maps a kind onto its REST collection segment. Spelled out
// rather than pluralised: "query" + "s" is not the route.
var listRoutes = map[listKind]string{
	listKindQuery:      "queries",
	listKindCollection: "collections",
}

// defaultListRowsLimit is how many rows one read numbers. Each row costs a
// handle, and a page a caller cannot see the end of is worse than a stated
// truncation: the definition printed above the rows is exactly what `find`
// takes, so narrowing is one call away.
const defaultListRowsLimit = 25

// maxListSources bounds how many sources a query's definition resolves by
// name — a query over more than a couple of types is not a shape any UI
// builds, and each source costs one read.
const maxListSources = 3

//
// ---- the served document ----
//

// servedListDoc is the slice of a served document this file reads.
type servedListDoc struct {
	Id         string            `json:"id"`
	Kind       string            `json:"kind"`
	Type       string            `json:"type"`
	Properties map[string]any    `json:"properties"`
	Blocks     []servedListBlock `json:"blocks"`
}

type servedListBlock struct {
	Id           string             `json:"id"`
	Type         string             `json:"type"`
	IsCollection bool               `json:"is_collection"`
	Properties   []servedDvProperty `json:"properties"`
	Views        []servedListView   `json:"views"`
}

// servedDvProperty is one entry of the dataview's own property list — the
// FORMATS the filter renderer needs to spell a date as a date.
type servedDvProperty struct {
	Property string `json:"property"`
	Format   string `json:"format"`
}

type servedListView struct {
	Id      string         `json:"id"`
	Name    string         `json:"name"`
	Filters []servedFilter `json:"filters"`
	Sorts   []servedSort   `json:"sorts"`
}

// servedFilter is one §6.2 filter node: a group (operator + filters) or a
// leaf (property + condition + value / date_preset).
type servedFilter struct {
	Operator   string          `json:"operator"`
	Filters    []servedFilter  `json:"filters"`
	Property   string          `json:"property"`
	Condition  string          `json:"condition"`
	Value      json.RawMessage `json:"value"`
	DatePreset string          `json:"date_preset"`
}

type servedSort struct {
	Property  string `json:"property"`
	Direction string `json:"direction"`
}

// listDefinition is one list object's readable definition.
type listDefinition struct {
	Kind    listKind `json:"kind"`
	Name    string   `json:"name,omitempty"`
	Sources []string `json:"sources,omitempty"`
	// SourceIsType reports that Sources name TYPES (the ordinary case) rather
	// than properties (a query over "everything that has this property").
	SourceIsType bool     `json:"source_is_type,omitempty"`
	View         string   `json:"view,omitempty"`
	ViewNames    []string `json:"view_names,omitempty"`
	// Filter is the compact filter string, empty when the view filters
	// nothing; FilterUnspellable marks the rare filter the compact syntax
	// cannot express (see renderFilterString).
	Filter            string `json:"filter,omitempty"`
	FilterUnspellable bool   `json:"filter_unspellable,omitempty"`
	Sort              string `json:"sort,omitempty"`
}

// listReadResult is a list read's machine shape: the definition, the rows as
// numbered handles, and the document itself — the JSON channel loses nothing
// the plain read used to serve.
type listReadResult struct {
	Definition listDefinition `json:"definition"`
	Rows       []Handle       `json:"rows"`
	Total      int            `json:"total"`
	HasMore    bool           `json:"has_more"`
	Handle     int            `json:"handle"`
	Document   jsonRaw        `json:"document"`
}

//
// ---- detection ----
//

// detectedList is a served document recognised as a list: its kind, the
// decoded envelope, its dataview block, and the type-name index the
// detection had to load (kept so the row rendering below does not list the
// space's types a second time in the same call).
type detectedList struct {
	kind      listKind
	envelope  servedListDoc
	dataview  *servedListBlock
	typeNames map[string]string
	// raw is the served document, kept whole: the JSON channel still carries
	// it, so nothing the plain read used to serve is lost by rendering.
	raw []byte
}

// detectListKind reports whether a served document is a Query or a
// Collection.
//
// A dataview block is necessary but NOT sufficient — an ordinary page can
// embed one — so the type term decides. The term is tested against the
// stored key first, so a document that is already spelled `set` costs
// nothing; only a document that carries a dataview AND names its type
// otherwise pays for the type listing that maps `set` and `collection` onto
// the names this space gives them. That listing is unavoidable: the wrapper
// reads in the name vocabulary (D5), so a served `type` is the display name,
// and "Query" is a name the space owns and can change.
func (r *Runner) detectListKind(ctx context.Context, spaceId string, doc []byte) (detectedList, bool) {
	found := detectedList{raw: doc}
	if err := json.Unmarshal(doc, &found.envelope); err != nil {
		return detectedList{}, false
	}
	for i := range found.envelope.Blocks {
		if found.envelope.Blocks[i].Type == "dataview" {
			found.dataview = &found.envelope.Blocks[i]
			break
		}
	}
	if found.dataview == nil || found.envelope.Type == "" {
		return detectedList{}, false
	}
	switch found.envelope.Type {
	case queryTypeKey:
		found.kind = listKindQuery
		return found, true
	case collectionTypeKey:
		found.kind = listKindCollection
		return found, true
	}
	found.typeNames = r.typeNameIndex(ctx, spaceId)
	switch found.envelope.Type {
	case found.typeNames[queryTypeKey]:
		found.kind = listKindQuery
		return found, true
	case found.typeNames[collectionTypeKey]:
		found.kind = listKindCollection
		return found, true
	}
	return detectedList{}, false
}

//
// ---- the read ----
//

// readList renders a Query or Collection: its definition, then its rows as
// numbered handles. Returns ok=false when anything it needs could not be
// read, and the caller then serves the plain document — an enrichment must
// never be able to break the read it decorates.
//
// HANDLE NUMBERING. The rows are registered with Session.registerHandle,
// which APPENDS within the working space (create's precedent) rather than
// renumbering the way find does. Two reasons, and the second is the load
// bearing one:
//
//   - a read is not a search: it ranks nothing, so the numbers it hands out
//     have to sit beside the find that produced the list rather than replace
//     it — the object the caller addressed as 1 a moment ago is still 1.
//   - the list's OWN handle must survive the call. Renumbering would delete
//     the very number the caller just used, so the model would finish reading
//     a query and no longer be able to name it — the tool would take away
//     more addressing than it gives. When the list has no handle yet (it was
//     read by id), it is registered FIRST, so it always has one afterwards,
//     and so the rows are appended behind an entry that already fixed the
//     working space.
//
// A row that already carries a handle is registered again rather than
// deduplicated: both numbers resolve to the same object, and skipping one
// would make the printed list skip a number, which reads as a lost row.
//
// Reading a list in a space the session was not working in moves the working
// space, because a handle resolves through it — the same thing create does
// when it makes an object elsewhere. Nothing is registered until the rows
// are in hand, so a read that falls back to the plain document leaves the
// session exactly as it found it.
func (r *Runner) readList(ctx context.Context, session *Session, spaceId, objectId string, found detectedList) (*Result, bool) {
	view := ""
	if len(found.dataview.Views) > 0 {
		view = found.dataview.Views[0].Id
	}
	rows, total, hasMore, warnings, err := r.listRows(ctx, spaceId, objectId, found.kind, view)
	if err != nil {
		return nil, false
	}

	def := r.listDefinitionOf(ctx, spaceId, found)
	selfHandle, ok := session.handleFor(spaceId, objectId)
	if !ok {
		selfHandle = session.registerHandle(spaceId, Handle{Id: objectId, Name: def.Name, Type: string(found.kind)})
	}
	handles := make([]Handle, 0, len(rows))
	for _, row := range rows {
		n := session.registerHandle(spaceId, Handle{Id: row.Id, Name: row.Name, Type: row.Type})
		handles = append(handles, Handle{N: n, Id: row.Id, Name: row.Name, Type: row.Type})
	}

	// the text channel spells each row's type by display name (find's rule),
	// reusing whatever the detection already loaded
	typeNames := found.typeNames
	if typeNames == nil && len(handles) > 0 {
		typeNames = r.typeNameIndex(ctx, spaceId)
	}
	text := listText(def, selfHandle, handles, typeNames, total, hasMore, warnings)
	return &Result{
		Text: text,
		JSON: listReadResult{
			Definition: def,
			Rows:       handles,
			Total:      total,
			HasMore:    hasMore,
			Handle:     selfHandle,
			Document:   jsonRaw(found.raw),
		},
	}, true
}

// listRows runs the list and returns its member rows. The stored view is
// passed through deliberately: the definition printed above the rows is that
// view's filter and sort, and rows fetched without it would be a DIFFERENT
// set of objects under a heading that says otherwise.
func (r *Runner) listRows(ctx context.Context, spaceId, objectId string, kind listKind, view string) ([]v2model.ObjectRow, int, bool, []v2model.Issue, error) {
	query := url.Values{
		"limit": []string{strconv.Itoa(defaultListRowsLimit)},
		// the name vocabulary, like every wrapper read (D5)
		"keys": []string{"name"},
	}
	if view != "" {
		query.Set("view", view)
	}
	var resp v2model.ListResponse[v2model.ObjectRow]
	err := r.client.decode(ctx, apiRequest{
		method: "GET",
		path:   "/v2/spaces/" + seg(spaceId) + "/" + listRoutes[kind] + "/" + seg(objectId) + "/objects",
		query:  query,
	}, &resp)
	if err != nil {
		return nil, 0, false, nil, fmt.Errorf("list the objects of %s %s: %w", kind, objectId, err)
	}
	return resp.Data, resp.Total, resp.HasMore, resp.Warnings, nil
}

// listDefinitionOf assembles the definition from the served document.
func (r *Runner) listDefinitionOf(ctx context.Context, spaceId string, found detectedList) listDefinition {
	def := listDefinition{Kind: found.kind, Name: docPropertyString(found.envelope.Properties, "name")}
	if found.kind == listKindQuery {
		def.Sources, def.SourceIsType = r.listSourceLabels(ctx, spaceId, found.envelope.Properties)
	}
	dv := found.dataview
	for _, v := range dv.Views {
		def.ViewNames = append(def.ViewNames, v.Name)
	}
	if len(dv.Views) == 0 {
		return def
	}
	view := dv.Views[0]
	def.View = view.Name
	formats := map[string]string{}
	for _, p := range dv.Properties {
		formats[p.Property] = p.Format
	}
	if len(view.Filters) > 0 {
		filter, err := renderFilterString(view.Filters, formats)
		if err != nil {
			def.FilterUnspellable = true
		} else {
			def.Filter = filter
		}
	}
	def.Sort = renderSortString(view.Sorts)
	return def
}

// listSourceLabels names a query's sources — the `setOf` property, whose
// entries are the ids of the TYPES the query runs over (or, rarely, of
// properties: a query over "everything that has this property"). Each id is
// resolved to its name by reading it, because the type listing serves keys
// and names but no ids. Best-effort in both halves: an unresolvable source is
// spelled by its id rather than dropped, since a definition that silently
// omits what a query runs over is worse than one that is ugly about it.
func (r *Runner) listSourceLabels(ctx context.Context, spaceId string, props map[string]any) ([]string, bool) {
	entries := docPropertyStrings(props, "setof")
	if len(entries) > maxListSources {
		entries = entries[:maxListSources]
	}
	labels := make([]string, 0, len(entries))
	isType := true
	for _, id := range entries {
		name, kind, ok := r.objectNameAndKind(ctx, spaceId, id)
		if !ok || name == "" {
			labels = append(labels, id)
			continue
		}
		if kind != "object_type" {
			isType = false
		}
		labels = append(labels, name)
	}
	return labels, isType
}

// objectNameAndKind reads one object's name and document kind.
func (r *Runner) objectNameAndKind(ctx context.Context, spaceId, objectId string) (string, string, bool) {
	doc, err := r.client.raw(ctx, apiRequest{
		method: "GET",
		path:   "/v2/spaces/" + seg(spaceId) + "/objects/" + seg(objectId),
		// include=properties is the cheapest shape that answers this
		// question: the name lives in the properties map, and a source type's
		// whole block tree would be serialized for nothing. (?outline=true
		// would be cheaper still and is WRONG here — the outline shape drops
		// the properties map unless include=properties accompanies it.)
		query: url.Values{"keys": []string{"name"}, "include": []string{"properties"}},
	})
	if err != nil {
		return "", "", false
	}
	var envelope servedListDoc
	if err := json.Unmarshal(doc, &envelope); err != nil {
		return "", "", false
	}
	return docPropertyString(envelope.Properties, "name"), envelope.Kind, true
}

//
// ---- the definition's text ----
//

// listText renders a list read. The frame comes first (find's rule: what
// this IS, before what it holds), then the definition in the vocabulary the
// writing tools take, then the numbered rows.
func listText(def listDefinition, selfHandle int, rows []Handle, typeNames map[string]string, total int, hasMore bool, warnings []v2model.Issue) string {
	var b strings.Builder
	name := def.Name
	if name == "" {
		name = "(unnamed)"
	}
	if def.Kind == listKindQuery {
		fmt.Fprintf(&b, "%s (handle %d) — a Query: a saved search, so its rows are whatever matches; nothing is added to it by hand.\n", name, selfHandle)
		if len(def.Sources) > 0 {
			word := "type"
			if !def.SourceIsType {
				word = "source"
			}
			fmt.Fprintf(&b, "%s: %s\n", word, strings.Join(def.Sources, ", "))
		}
		switch {
		case def.FilterUnspellable:
			b.WriteString("filter: (this view's filter cannot be written in the compact filter syntax — update_view would replace it, not reproduce it)\n")
		case def.Filter != "":
			fmt.Fprintf(&b, "filter: %s\n", def.Filter)
		default:
			b.WriteString("filter: (none — every object of that type)\n")
		}
	} else {
		fmt.Fprintf(&b, "%s (handle %d) — a Collection: a manual list, so its rows were put there by hand; no filter decides them.\n", name, selfHandle)
	}
	if def.Sort != "" {
		fmt.Fprintf(&b, "sort: %s\n", def.Sort)
	}
	if len(def.ViewNames) > 1 {
		fmt.Fprintf(&b, "views: %s — showing %q\n", strings.Join(def.ViewNames, ", "), def.View)
	}

	for _, h := range rows {
		fmt.Fprintf(&b, "%d. %s (%s)\n", h.N, displayName(h), typeLabel(typeNames, h.Type))
	}
	switch {
	case total == 0 && def.Kind == listKindQuery:
		b.WriteString("no rows — nothing in the space matches this query right now")
	case total == 0:
		b.WriteString("no rows — this collection is empty")
	case hasMore:
		fmt.Fprintf(&b, "%d rows — showing the first %d, numbered above", total, len(rows))
	default:
		fmt.Fprintf(&b, "%d rows, numbered above", total)
	}
	// the closing line names what to do next, and only claims the numbers
	// address something when there is something numbered
	addressing := ""
	if len(rows) > 0 {
		addressing = "the numbers address the rows; "
	}
	if def.Kind == listKindQuery {
		b.WriteString("\n" + addressing + "find takes that type and filter to search the same objects, and update_view changes what this query shows")
	} else {
		b.WriteString("\n" + addressing + "update_view changes this collection's sort or columns")
	}
	for _, w := range warnings {
		b.WriteString("\nwarning: " + w.Message)
	}
	return b.String()
}

//
// ---- the filter string ----
//

// renderFilterString renders a view's structured filters as the compact
// filter string — the ONE filter vocabulary this surface publishes (find's
// `filter`, update_view's `filter`). The array is an implicit AND (§6.2), the
// same shape the parser emits, so the rendering is the parser's own inverse.
//
// The result is parsed back before it is returned: what a tool prints must be
// accepted as what a tool takes, and here that is checkable rather than
// merely intended — the parser this calls IS the one the server runs on the
// way in. A filter that does not survive the round trip is reported as
// unspellable instead of printed, because a filter string the caller cannot
// resend is worse than an honest gap (a property named "C++" cannot be a bare
// key in this grammar, and no escape for it exists by design).
func renderFilterString(nodes []servedFilter, formats map[string]string) (string, error) {
	rendered, err := renderFilterNodes(nodes, "AND", formats)
	if err != nil {
		return "", err
	}
	if rendered == "" {
		return "", nil
	}
	if _, err := filterstring.Parse(rendered, filterstring.Options{}); err != nil {
		return "", fmt.Errorf("the rendered filter does not parse: %w", err)
	}
	return rendered, nil
}

// renderFilterNodes joins one level of filter nodes with its operator.
func renderFilterNodes(nodes []servedFilter, op string, formats map[string]string) (string, error) {
	parts := make([]string, 0, len(nodes))
	for _, node := range nodes {
		part, err := renderFilterNode(node, formats)
		if err != nil {
			return "", err
		}
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " "+op+" "), nil
}

// renderFilterNode renders one node: a group in parentheses, or a leaf.
func renderFilterNode(node servedFilter, formats map[string]string) (string, error) {
	if len(node.Filters) > 0 {
		op := "AND"
		if strings.EqualFold(node.Operator, "or") {
			op = "OR"
		}
		inner, err := renderFilterNodes(node.Filters, op, formats)
		if err != nil {
			return "", err
		}
		if inner == "" {
			return "", nil
		}
		return "(" + inner + ")", nil
	}
	if node.Property == "" {
		return "", fmt.Errorf("a filter names no property")
	}
	key := filterKeySpelling(node.Property)
	switch node.Condition {
	case "empty":
		return key + " IS EMPTY", nil
	case "not_empty":
		return key + " IS NOT EMPTY", nil
	case "exists":
		return key + " EXISTS", nil
	}
	value, err := renderFilterValue(node, formats[node.Property])
	if err != nil {
		return "", err
	}
	switch node.Condition {
	case "equal", "":
		// a list value here is the grammar's set literal, which is spelled
		// with the same operator (it parses as exact_in)
		return key + " = " + value, nil
	case "not_equal":
		return key + " != " + value, nil
	case "greater":
		return key + " > " + value, nil
	case "less":
		return key + " < " + value, nil
	case "greater_or_equal":
		return key + " >= " + value, nil
	case "less_or_equal":
		return key + " <= " + value, nil
	case "contains":
		return key + " CONTAINS " + value, nil
	case "not_contains":
		return key + " NOT CONTAINS " + value, nil
	case "in", "exact_in":
		return key + " IN " + listLiteral(value), nil
	case "not_in", "not_exact_in":
		return key + " NOT IN " + listLiteral(value), nil
	case "all_in":
		return key + " HAS ALL " + listLiteral(value), nil
	case "not_all_in":
		return key + " NOT HAS ALL " + listLiteral(value), nil
	default:
		return "", fmt.Errorf("condition %q has no compact spelling", node.Condition)
	}
}

// listLiteral makes sure a list-taking condition gets a parenthesised list,
// even when the stored value is a single scalar (which the store allows).
func listLiteral(value string) string {
	if strings.HasPrefix(value, "(") {
		return value
	}
	return "(" + value + ")"
}

// filterKeySpelling renders a property name as the grammar's bare key: the
// compact syntax takes identifiers, so a multi-word display name is joined
// with underscores — the spelling find's and update_view's own argument
// descriptions teach ("Due_date"), which the server's fold resolves back onto
// "Due date". A name no identifier can spell survives to the parse check,
// which rejects it and turns the whole filter into an honest gap.
func filterKeySpelling(name string) string {
	return strings.Join(strings.Fields(name), "_")
}

// datePresetFunctions maps the structured §6.2 date_preset names onto the
// compact syntax's own function names. The two vocabularies are deliberately
// different (snake_case values, camelCase functions), so the map is written
// out; the parse check above is what keeps it honest.
var datePresetFunctions = map[string]string{
	"yesterday":          "yesterday",
	"today":              "today",
	"tomorrow":           "tomorrow",
	"last_week":          "lastWeek",
	"current_week":       "currentWeek",
	"next_week":          "nextWeek",
	"last_month":         "lastMonth",
	"current_month":      "currentMonth",
	"next_month":         "nextMonth",
	"last_year":          "lastYear",
	"current_year":       "currentYear",
	"next_year":          "nextYear",
	"number_of_days_ago": "daysAgo",
	"number_of_days_now": "daysFromNow",
}

// countingDatePresets take their day count from the filter's value.
var countingDatePresets = map[string]bool{"daysAgo": true, "daysFromNow": true}

// renderFilterValue renders a leaf's value: a date preset as its function
// call, a date property's stored unix seconds as the RFC 3339 string the
// compact syntax takes (the parser converts it back — the two forms are the
// same instant, and only one of them is readable), everything else as its
// literal.
func renderFilterValue(node servedFilter, format string) (string, error) {
	if fn, ok := datePresetFunctions[node.DatePreset]; ok {
		if !countingDatePresets[fn] {
			return fn + "()", nil
		}
		days := 0
		if len(node.Value) > 0 {
			var n float64
			if err := json.Unmarshal(node.Value, &n); err != nil {
				return "", fmt.Errorf("date preset %q takes a day count: %w", node.DatePreset, err)
			}
			days = int(n)
		}
		return fmt.Sprintf("%s(%d)", fn, days), nil
	}
	if node.DatePreset != "" && node.DatePreset != "exact_date" {
		return "", fmt.Errorf("date preset %q has no compact spelling", node.DatePreset)
	}
	if len(node.Value) == 0 {
		return "", fmt.Errorf("condition %q needs a value", node.Condition)
	}
	var value any
	if err := json.Unmarshal(node.Value, &value); err != nil {
		return "", fmt.Errorf("decode filter value: %w", err)
	}
	return renderFilterLiteral(value, format)
}

func renderFilterLiteral(value any, format string) (string, error) {
	switch v := value.(type) {
	case string:
		return quoteFilterValue(v), nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case float64:
		if format == "date" {
			return quoteFilterValue(time.Unix(int64(v), 0).UTC().Format(time.RFC3339)), nil
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case []any:
		parts := make([]string, 0, len(v))
		for _, entry := range v {
			part, err := renderFilterLiteral(entry, format)
			if err != nil {
				return "", err
			}
			parts = append(parts, part)
		}
		if len(parts) == 0 {
			return "", fmt.Errorf("an empty value list has no compact spelling")
		}
		return "(" + strings.Join(parts, ", ") + ")", nil
	default:
		return "", fmt.Errorf("value %v has no compact spelling", value)
	}
}

//
// ---- the sort string ----
//

// renderSortString renders a view's sorts in update_view's own `sort`
// grammar ("Due date desc, Name asc"). The direction is always explicit,
// which is what update_view's receipt does with the same list: a caller
// pasting this back gets exactly the order it just read.
func renderSortString(sorts []servedSort) string {
	parts := make([]string, 0, len(sorts))
	for _, s := range sorts {
		if s.Property == "" {
			continue
		}
		direction := "asc"
		if strings.EqualFold(s.Direction, "desc") {
			direction = "desc"
		}
		parts = append(parts, s.Property+" "+direction)
	}
	return strings.Join(parts, ", ")
}

//
// ---- document property lookup ----
//

// docPropertyValue finds a served property by its FOLD class rather than by
// one spelling: a document read in the name vocabulary spells `setOf` as
// "Set of" and `name` as "Name", the api vocabulary spells them set_of and
// name, and the fold (the format's own FoldKeyTerm) is what makes all of
// them one key — the same tool values.go's property index is built on.
func docPropertyValue(props map[string]any, foldClass string) (any, bool) {
	for key, value := range props {
		if anyblockjson.FoldKeyTerm(key) == foldClass {
			return value, true
		}
	}
	return nil, false
}

// docPropertyString reads a scalar string property.
func docPropertyString(props map[string]any, foldClass string) string {
	value, ok := docPropertyValue(props, foldClass)
	if !ok {
		return ""
	}
	s, _ := value.(string)
	return s
}

// docPropertyStrings reads a property as its list of strings, tolerating the
// single-value shape the store also stores.
func docPropertyStrings(props map[string]any, foldClass string) []string {
	value, ok := docPropertyValue(props, foldClass)
	if !ok {
		return nil
	}
	switch v := value.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, entry := range v {
			if s, ok := entry.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
