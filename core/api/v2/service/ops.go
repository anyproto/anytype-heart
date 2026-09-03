package v2service

// ops.go holds the PATCH op vocabulary:
// the closed, id-addressed op set, its strict decoding, and the read-only
// document view the ops address blocks through. The view is the live
// state's flat AnyBlock rendering — the same shape agents read, so
// references (full ids, unique suffixes), indents and error texts all speak
// the document language. The actual mutations happen on the object's child
// state (stateops.go); nothing here writes the view.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// v2OpNames is the closed op set, in documentation order.
var v2OpNames = []string{
	"set_properties", "update_block", "replace_subtree",
	"insert_blocks", "move_block", "delete_block", "replace_text", "set_cell",
	"update_view", "insert_view", "move_view", "delete_view",
	"add_items", "remove_items",
}

// v2OpEditNeeds maps each op to the object-level restriction axes it
// touches, so the edit gate is per-op rather than per-request (surface
// review M1). Sets and collections carry Restrictions_Blocks but NOT
// Restrictions_Details, so demanding both of every batch made renaming a set
// impossible and left add_items/remove_items — the only v2 route into an
// existing collection — permanently refused.
//
// Item ops need NEITHER axis: they mutate the collection store
// (template.CollectionStoreKey), which no object restriction governs. That
// matches v1, whose ObjectCollectionAdd/Remove is likewise ungated.
//
// The VIEW FAMILY (update_view, insert_view, move_view, delete_view) needs
// NEITHER axis either, and getting this wrong recreates the M1 bug exactly:
// sets, collections AND object types all carry Restrictions_Blocks
// (restriction/object.go objRestrictEdit / objRestrictEditAndTemplate) —
// the three object classes that HAVE dataviews — so classifying a view op
// as a block op would refuse it on precisely the objects it exists to edit.
// The Blocks axis governs document content (basic.CreateBlock, tables,
// clipboard all check it); view configuration is not gated by it: the
// native dataview surface (sdataview.UpdateView / CreateView / DeleteView,
// i.e. v1's BlockDataviewView* RPCs) checks no object-level restriction,
// which is how the app edits views on a set at all.
// objectmutateadapter_test.go pins these facts against the live restriction
// table; viewops_test.go pins the whole family's classification through
// PatchObject.
var v2OpEditNeeds = map[string]apicore.EditNeeds{
	"set_properties":  {Details: true},
	"update_block":    {Blocks: true},
	"replace_subtree": {Blocks: true},
	"insert_blocks":   {Blocks: true},
	"move_block":      {Blocks: true},
	"delete_block":    {Blocks: true},
	"replace_text":    {Blocks: true},
	"set_cell":        {Blocks: true},
	"update_view":     {},
	"insert_view":     {},
	"move_view":       {},
	"delete_view":     {},
	"add_items":       {},
	"remove_items":    {},
}

// editNeedsForOps unions the restriction axes a batch touches and refuses the
// batch if the object forbids one of them, addressing the FIRST op that needs
// the forbidden axis (C6) rather than the request as a whole — so a caller
// mixing a legal rename with an illegal block edit on a set is told which op
// is the problem.
//
// An op the applier does not recognise contributes no needs here; it fails
// later, in the applier, with its own path-addressed error. This function
// must never be the thing that reports an unknown op.
func editNeedsForOps(ops []json.RawMessage, cur apicore.ObjectRead) (apicore.EditNeeds, error) {
	var union apicore.EditNeeds
	for i, raw := range ops {
		var probe struct {
			Op string `json:"op"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		needs, known := v2OpEditNeeds[probe.Op]
		if !known {
			continue
		}
		opPath := fmt.Sprintf("/ops/%d", i)
		if needs.Blocks {
			if cur.BlocksRefused != nil {
				return union, restrictionRefusal(cur.BlocksRefused, probe.Op, opPath, "blocks")
			}
			union.Blocks = true
		}
		if needs.Details {
			if cur.DetailsRefused != nil {
				return union, restrictionRefusal(cur.DetailsRefused, probe.Op, opPath, "properties")
			}
			union.Details = true
		}
	}
	return union, nil
}

// restrictionRefusal is the 403 for an op the object's own restrictions
// forbid: the verdict is produced HERE, where it is made, as the C6 error —
// left bare it fell through RespondError's 500 fallback, dressing a
// PERMANENT refusal as a retryable fault and retry-looping the agent
// (surface review M2a). The mutator path's in-lock re-check is classified
// by mapWriteError on the same restriction.ErrRestricted sentinel.
func restrictionRefusal(refused error, op, opPath, axis string) error {
	return v2model.NewError(http.StatusForbidden, v2model.CodeForbidden,
		fmt.Sprintf("%s (op %q at %s edits %s)", refused.Error(), op, opPath, axis),
		v2model.Issue{
			Path:    opPath,
			Message: fmt.Sprintf("the %q op edits this object's %s, which its restrictions forbid", op, axis),
			Hint:    "this refusal is permanent for this object — do not retry the same request",
		})
}

// v2NewContentOps is the set of ops whose block payload can only ever
// CREATE, so no value of an id slot in it can succeed (§8.30): an id that
// resolves is a duplicate, one that does not is unresolvable, and there is no
// third value.
//
// ONE set, read by BOTH halves that have to agree about it. The runtime
// (decodePayloadRun) refuses the field for the ops listed here; the served op
// schema (schemas_ops.go opSchema) publishes the id-less payload-block def
// for exactly the same ops. Runtime-without-schema is merely strict;
// schema-without-runtime re-creates §8.30's own bug — an op advertising a
// field no value of which can succeed — so the two literals that used to
// state this independently are gone.
var v2NewContentOps = map[string]bool{
	"insert_blocks": true,
}

// v2OpRebuildsView marks the ops whose apply invalidates the document view
// (stateops.go), forcing the NEXT op to re-marshal the whole document under
// the object lock. The render-work bound (edit.go patchRenderWork, surface
// review M7) counts exactly these; an op listed false here must keep the
// view valid in place. Unknown op names never reach a rebuild — the batch
// fails at dispatch first.
var v2OpRebuildsView = map[string]bool{
	"set_properties":  true,
	"update_block":    true,
	"replace_subtree": true,
	"insert_blocks":   true,
	"move_block":      true,
	"delete_block":    true,
	// replace_text maintains the view in place (stateops.go textEdited): it
	// changes exactly one exported field of one block, and it writes the
	// canonical rendering a re-marshal would emit. It is also the one op that
	// inherently arrives many-per-batch (one find/replace each), so exempting
	// it is what lets an agent batch hundreds of text edits on a large
	// document without tripping the render-work bound.
	"replace_text": false,
	"set_cell":     true,
	"update_view":  true,
	"insert_view":  true,
	"move_view":    true,
	"delete_view":  true,
	"add_items":    true,
	"remove_items": true,
}

// v2OutputOnlyPropertyKeys reports whether a key is one of the SPEC §4a
// output-only property keys a set_properties must reject. The set itself
// lives in v2model (the leaf both layers share): the wrapper's describe
// must not advertise an output-only key as settable, and a second copy of
// the list there would be the §8.31 drift class.
func v2OutputOnlyPropertyKeys(key string) bool { return v2model.IsOutputOnlyProperty(key) }

// v2ListShapedFormats are the property formats whose SPEC §3 value encoding
// is a list — the only formats set_properties add/remove apply to.
var v2ListShapedFormats = map[model.RelationFormat]bool{
	model.RelationFormat_status: true, // select
	model.RelationFormat_tag:    true, // multiSelect
	model.RelationFormat_object: true, // objects
	model.RelationFormat_file:   true, // files
}

// v2ListShapedFormatNames is the agent-facing list for the rejection text.
const v2ListShapedFormatNames = "select, multi_select, objects, files"

//
// ---- the document view ----
//

// v2EditDoc is the decoded flat AnyBlock document the ops read: the
// addressing view (ids, types, indents) and the reference sets the
// validations consult.
type v2EditDoc struct {
	fields     map[string]json.RawMessage
	properties map[string]json.RawMessage
	items      []string
	blocks     []map[string]any
}

func parseEditDoc(body []byte) (*v2EditDoc, error) {
	fields, err := parseEnvelope(body)
	if err != nil {
		return nil, err
	}
	doc := &v2EditDoc{fields: fields, properties: map[string]json.RawMessage{}}
	if raw, ok := fields["properties"]; ok {
		if err := json.Unmarshal(raw, &doc.properties); err != nil {
			return nil, fmt.Errorf("decode properties: %w", err)
		}
	}
	if raw, ok := fields["items"]; ok {
		if err := json.Unmarshal(raw, &doc.items); err != nil {
			return nil, fmt.Errorf("decode items: %w", err)
		}
	}
	if raw, ok := fields["blocks"]; ok {
		if err := decodeJSONUseNumber(raw, &doc.blocks); err != nil {
			return nil, fmt.Errorf("decode blocks: %w", err)
		}
	}
	return doc, nil
}

func blockIndent(b map[string]any) int {
	if v, ok := jsonFloat64(b["indent"]); ok {
		return int(v)
	}
	return 0
}

func blockId(b map[string]any) string {
	s, _ := b["id"].(string)
	return s
}

func blockType(b map[string]any) string {
	s, _ := b["type"].(string)
	return s
}

func (d *v2EditDoc) blockIds() []string {
	ids := make([]string, len(d.blocks))
	for i, b := range d.blocks {
		ids[i] = blockId(b)
	}
	return ids
}

// localIds collects every doc-local id the document carries in an id slot —
// block ids, table column and row ids, dataview view ids, and the ids of
// cell DESCENDANTS: the same id domain compact relabeling covers (§9a). The
// cell block itself carries no id in the flat form (derived, SPEC §6.1), but
// a cell with descendants is the F10 array form, whose elements past the
// first are ordinary flat blocks WITH ids in the relabel pool. Ids stay in
// document order, deduplicated.
//
// This is the id vocabulary two guards share: the create path's
// label-shaped-id warning (docLocalIds) and the PATCH payload resolver
// (payloadids.go). One walker, so neither can drift into covering a slot the
// other misses.
func (d *v2EditDoc) localIds() []string {
	var ids []string
	seen := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for i, b := range d.blocks {
		// The payload walker is also the authoritative id-slot census. Its
		// visitor sees ordinary block/row/column/view ids while deliberately
		// skipping only the derived root id of a table-cell block.
		_ = walkPayloadIdSlots(b, fmt.Sprintf("blocks[%d]", i), func(slot payloadIdSlot) error {
			add(blockId(slot.m))
			return nil
		})
	}
	return ids
}

// subtreeEnd returns the index just past block i's contiguous descendant run.
func (d *v2EditDoc) subtreeEnd(i int) int {
	base := blockIndent(d.blocks[i])
	j := i + 1
	for j < len(d.blocks) && blockIndent(d.blocks[j]) > base {
		j++
	}
	return j
}

// docType returns the envelope type key.
func (d *v2EditDoc) docType() string {
	var t string
	if raw, ok := d.fields["type"]; ok {
		_ = json.Unmarshal(raw, &t)
	}
	return t
}

//
// ---- op decoding ----
//

type opSetProperties struct {
	Op    string                     `json:"op"`
	Set   map[string]json.RawMessage `json:"set"`
	Unset []string                   `json:"unset"`
	// Add/Remove are the per-key list edits (v0.3.5): append entries to /
	// delete entries from a list-shaped property without rewriting the whole
	// array. Values are arrays of entries (option names or object/file ids).
	Add    map[string]json.RawMessage `json:"add"`
	Remove map[string]json.RawMessage `json:"remove"`
}

// opUpdateBlock addresses its block through EITHER Id or Match. Match is an
// exact substring of the block's text that must identify
// exactly one block. Giving both is refused rather than ranked — see
// resolveSubject.
type opUpdateBlock struct {
	Op    string                     `json:"op"`
	Id    string                     `json:"id"`
	Match string                     `json:"match"`
	Set   map[string]json.RawMessage `json:"set"`
}

type opReplaceSubtree struct {
	Op     string            `json:"op"`
	Id     string            `json:"id"`
	Blocks []json.RawMessage `json:"blocks"`
}

type opInsertBlocks struct {
	Op       string            `json:"op"`
	After    string            `json:"after"`
	Before   string            `json:"before"`
	Inside   string            `json:"inside"`
	Position string            `json:"position"`
	Blocks   []json.RawMessage `json:"blocks"`
	// Markdown is the authoring-channel alternative to Blocks (§7.1, v0.4):
	// the server parses it into a flat run (anyblockjson.ParseMarkdownBlocks)
	// and the op proceeds exactly as if that run had been supplied as Blocks —
	// same targeting (incl. root-append), validation, created_blocks and
	// diff_stats. Mutually exclusive with Blocks.
	Markdown string `json:"markdown"`
}

type opMoveBlock struct {
	Op       string `json:"op"`
	Id       string `json:"id"`
	After    string `json:"after"`
	Before   string `json:"before"`
	Inside   string `json:"inside"`
	Position string `json:"position"`
}

// opDeleteBlock addresses its block the same two ways as opUpdateBlock. This
// is the op §5.1 calls out as the one where one-match-or-refuse is
// load-bearing: a locator that guessed here would delete the wrong subtree.
type opDeleteBlock struct {
	Op        string `json:"op"`
	Id        string `json:"id"`
	Match     string `json:"match"`
	Recursive bool   `json:"recursive"`
}

type opReplaceText struct {
	Op         string `json:"op"`
	Id         string `json:"id"`
	Find       string `json:"find"`
	Replace    string `json:"replace"`
	ReplaceAll bool   `json:"replace_all"`
}

type opSetCell struct {
	Op      string          `json:"op"`
	TableId string          `json:"table_id"`
	Row     string          `json:"row"`
	Col     string          `json:"col"`
	Value   json.RawMessage `json:"value"`
}

type opUpdateView struct {
	Op string `json:"op"`
	// Block references a dataview block (full id or unique suffix). Optional:
	// omitted, the op targets the object's only dataview block — the common
	// case; types, sets and collections carry exactly one, at the fixed id
	// "dataview".
	Block string `json:"block"`
	// View references a view by id (full or unique suffix — the same C4
	// leniency as resolveViewRef on the read surface). Optional when the
	// dataview has exactly one view.
	View string `json:"view"`
	// Set merges §6.2 view-level fields (update_block's merge semantics: only
	// named fields change, explicit null clears one). sorts and filters
	// replace whole when named — they are small ordered lists; filter is the
	// compact-string alternative to filters. columns is NOT accepted here —
	// the Columns channel below edits per column so one flip never rewrites
	// the array.
	Set map[string]json.RawMessage `json:"set"`
	// Columns merges per column, keyed by property key: a patch object
	// ({hidden, width, align, aggregation}) merges into the existing column
	// (appending a new column for a key that has none), an explicit null
	// removes the column.
	Columns map[string]json.RawMessage `json:"columns"`
}

// opInsertView creates ONE view. Singular where insert_blocks is plural, on
// purpose: a blocks payload is a structured run (indent-nested, ordered);
// views have no internal structure, and several views are several ops in
// the already-atomic batch. The base view is either sensible defaults
// (every property of the dataview visible, lastModifiedDate-desc sort —
// the native CreateView default, minus its everything-hidden columns) or a
// duplicate of CopyFrom; Set/Columns then merge on top with update_view's
// exact semantics, so create is "update_view aimed at a fresh view".
type opInsertView struct {
	Op    string `json:"op"`
	Block string `json:"block"`
	// Name is required — a view is a named tab.
	Name string `json:"name"`
	// CopyFrom duplicates an existing view of the same dataview (columns,
	// sorts, filters, type, groupBy, editor state — everything but id and
	// name); "like that one, but…" is the common intent.
	CopyFrom string `json:"copy_from"`
	// Targeting within the views list (at most one): after/before a view
	// ref, or position "first"|"last". Omitted = append. The FIRST view is
	// the client's default tab, so position "first" is "make this the
	// default".
	After    string                     `json:"after"`
	Before   string                     `json:"before"`
	Position string                     `json:"position"`
	Set      map[string]json.RawMessage `json:"set"`
	Columns  map[string]json.RawMessage `json:"columns"`
}

type opMoveView struct {
	Op       string `json:"op"`
	Block    string `json:"block"`
	View     string `json:"view"`
	After    string `json:"after"`
	Before   string `json:"before"`
	Position string `json:"position"`
}

type opDeleteView struct {
	Op    string `json:"op"`
	Block string `json:"block"`
	View  string `json:"view"`
}

type opItems struct {
	Op    string   `json:"op"`
	Items []string `json:"items"`
}

// decodeStrictOp decodes one op body into its typed struct, rejecting
// unknown fields with a schema pointer.
func decodeStrictOp(raw json.RawMessage, opName, opPath string, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return v2model.ValidationFailed(fmt.Sprintf("invalid %s op", opName),
			v2model.Issue{
				Path:    opPath,
				Message: err.Error(),
				Hint:    fmt.Sprintf("GET /v2/schemas/ops/%s for the op's schema and example", opName),
			})
	}
	return nil
}

func decodeJSONUseNumber(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	return dec.Decode(v)
}

func jsonFloat64(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case json.Number:
		v, err := value.Float64()
		return v, err == nil
	default:
		return 0, false
	}
}

//
// ---- shared op helpers ----
//

// countBlocks renders "1 descendant block" / "3 descendant blocks".
func countBlocks(n int) string {
	if n == 1 {
		return "1 descendant block"
	}
	return fmt.Sprintf("%d descendant blocks", n)
}

// leafWithDescendantsError names the descendant count when a type change
// would turn a parent into a leaf (R5).
func leafWithDescendantsError(id, newType string, descendants int, path string) error {
	return v2model.ValidationFailed(
		fmt.Sprintf("cannot change block %q to %q — it has %s; %q blocks cannot have children", id, newType, countBlocks(descendants), newType),
		v2model.Issue{Path: path, Message: "move or delete the descendants first, or use replace_subtree"})
}

// resolveTablePart resolves a row/column reference within one table: exact
// id or unique suffix first (the same C4 leniency as block refs), then —
// only when no id answers — by the text a read shows for the part: a
// column's header-row cell, a row's FIRST cell (case-insensitive, trimmed).
// The fallback exists because column ids are the one doc-local id compact
// relabeling can never shorten (every derived <rowId>-<colId> cell id
// shares the column id's suffix), so callers naturally send the header word
// the read shows instead of a 24-hex id. The rule is matchBlockRef's
// contract on the text axis: exactly one match resolves, several refuse
// with the candidates listed, zero is a 404 — never silently pick one.
func resolveTablePart(table map[string]any, kind, ref, tableRef, path string) (int, error) {
	entries, _ := table[kind].([]any)
	ids := make([]string, len(entries))
	for i, e := range entries {
		if m, ok := e.(map[string]any); ok {
			ids[i], _ = m["id"].(string)
		}
	}
	part := strings.TrimSuffix(kind, "s")
	idx, matches := matchBlockRef(ids, ref)
	switch {
	case matches == 1:
		return idx, nil
	case matches > 1:
		// an id-suffix AMBIGUITY stays a refusal — falling through to the
		// text axis would silently pick one of the parts the 400 exists to
		// make the caller disambiguate (the filterBlockSubtree rule)
		return -1, v2model.AmbiguousInput(
			fmt.Sprintf("%s reference %q matches more than one %s in table %q — use the full id", part, ref, part, tableRef),
			v2model.Issue{Path: path, Message: "the reference is a suffix of several ids"})
	}

	labels := tablePartLabels(table, kind, len(entries))
	labelName := "first-cell text"
	if kind == "columns" {
		labelName = "header text"
	}
	var textMatches []int
	want := strings.TrimSpace(ref)
	for i, label := range labels {
		if label != "" && want != "" && strings.EqualFold(label, want) {
			textMatches = append(textMatches, i)
		}
	}
	switch len(textMatches) {
	case 1:
		return textMatches[0], nil
	case 0:
		listed := make([]string, 0, len(ids))
		for i, id := range ids {
			if i == maxListedKeys {
				break
			}
			if labels[i] != "" {
				listed = append(listed, fmt.Sprintf("%s (%q)", id, labels[i]))
			} else {
				listed = append(listed, id)
			}
		}
		return -1, v2model.NotFound(
			fmt.Sprintf("%s %q not found in table %q — %s, in cell order: %s", part, ref, tableRef, kind, strings.Join(listed, ", ")))
	default:
		cands := make([]string, 0, len(textMatches))
		for _, i := range textMatches {
			cands = append(cands, fmt.Sprintf("%s (%q)", ids[i], labels[i]))
		}
		return -1, v2model.AmbiguousInput(
			fmt.Sprintf("%s %q matches the %s of more than one %s in table %q — use the id: %s", part, ref, labelName, part, tableRef, strings.Join(cands, ", ")),
			v2model.Issue{Path: path, Message: fmt.Sprintf("several %s share that %s", kind, labelName)})
	}
}

// tablePartLabels names each entry of one table axis the way a read shows
// it, index-aligned with the entries: a column's label is the header row's
// cell text at the same index (the header row is first in the marshaled
// shape — no header row, no column labels), a row's label is its first
// cell's text. Labels are trimmed; entries without one stay "".
func tablePartLabels(table map[string]any, kind string, n int) []string {
	labels := make([]string, n)
	rows, _ := table["rows"].([]any)
	switch kind {
	case "columns":
		for i, cell := range tableHeaderCells(rows) {
			if i == n {
				break
			}
			labels[i] = strings.TrimSpace(tableCellText(cell))
		}
	case "rows":
		for i := range labels {
			if i >= len(rows) {
				break
			}
			if row, ok := rows[i].(map[string]any); ok {
				if cells, _ := row["cells"].([]any); len(cells) > 0 {
					labels[i] = strings.TrimSpace(tableCellText(cells[0]))
				}
			}
		}
	}
	return labels
}

// tableHeaderCells returns the header row's cells, or nil when the table
// has none. Header rows sort first in the marshaled shape (§6.1), so only
// rows[0] can be one.
func tableHeaderCells(rows []any) []any {
	if len(rows) == 0 {
		return nil
	}
	row, ok := rows[0].(map[string]any)
	if !ok {
		return nil
	}
	if isHeader, _ := row["is_header"].(bool); !isHeader {
		return nil
	}
	cells, _ := row["cells"].([]any)
	return cells
}

// tableCellText extracts the text of one rendered cell across the §6.1 cell
// forms: the string shorthand, a block object's text, the first block of
// the flat-descendants array. A nil (empty) cell has none.
func tableCellText(cell any) string {
	switch c := cell.(type) {
	case string:
		return c
	case map[string]any:
		text, _ := c["text"].(string)
		return text
	case []any:
		if len(c) > 0 {
			return tableCellText(c[0])
		}
	}
	return ""
}

// decodeOpBlock decodes one payload block object.
func decodeOpBlock(raw json.RawMessage, path string) (map[string]any, error) {
	var block map[string]any
	if err := decodeJSONUseNumber(raw, &block); err != nil {
		return nil, v2model.ValidationFailed("a payload block must be a JSON object",
			v2model.Issue{Path: path, Message: err.Error()})
	}
	if typ := blockType(block); typ == "" {
		return nil, v2model.ValidationFailed("a payload block needs a type",
			v2model.Issue{Path: path + ".type", Message: "type is required (SPEC §5 lists the inventory)"})
	}
	return block, nil
}

//
// ---- space lookups ----
//

// isCollectionType reports whether a type key is the collection type or a
// custom type with the collection layout.
func (s *Service) isCollectionType(spaceId, typeKey string) bool {
	if typeKey == string(bundle.TypeKeyCollection) {
		return true
	}
	typeId, ok := s.typeIdInSpace(spaceId, typeKey)
	if !ok {
		return false
	}
	details, err := s.store.SpaceIndex(spaceId).GetDetails(typeId)
	if err != nil {
		return false
	}
	return details.GetInt64(bundle.RelationKeyRecommendedLayout) == int64(model.ObjectType_collection)
}
