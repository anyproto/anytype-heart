package v2service

// schemas_ops.go is the per-op discovery surface:
// GET /v2/schemas/ops/{op} serves one tiny C13-strict schema and a minimal
// example per PATCH op, so the smallest consumers stay at the smallest schema
// surface. The multi-op composite example remains on the PATCH endpoint docs
// as the secondary illustration.
//
// The example is an INSTANCE of the schema served beside it — one op object,
// not a whole {"ops":[…]} request body (§8.32). A consumer that reads the
// pair together, which is the small consumer this route exists for, otherwise
// gets two contradictory shapes; measured, the wrapped example cost
// gemma4:e4b a missing `op` field on 9 of 60 calls, and unwrapping it took
// that to 0 of 60. TestServedOpExampleValidatesAgainstItsOwnSchema is the
// pin: a new op cannot land with an example its own schema rejects.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

// v2OpsEndpoint is the endpoint every op schema belongs to.
const v2OpsEndpoint = "PATCH /v2/spaces/{space_id}/objects/{object_id}"

// v2TypeOpsEndpoint is the endpoint the three type-op schemas belong to.
const v2TypeOpsEndpoint = "PATCH /v2/spaces/{space_id}/types/{type}"

// v2ViewOpsEndpoints is the view family's endpoint line. Those ops run on
// BOTH channels with the same body: a dataview op does not care whether the
// dataview belongs to a set, a collection or a type. A type's own document
// already serves its views (GET /types/{key} returns blocks), so the endpoint
// that shows them is the endpoint that changes them.
const v2ViewOpsEndpoints = v2OpsEndpoint + " · " + v2TypeOpsEndpoint

// The payload block comes in TWO shapes, and the split is the whole point
// (§8.30). `id` in a payload means "name an EXISTING block, keep its
// identity" — which is meaningful only where the op has existing content to
// name. In a NEW-content payload every possible value of it is an error (an
// id that resolves is a duplicate, one that does not is unresolvable), so it
// must not be advertised there: with additionalProperties:false (C13) a
// constrained decoder then cannot emit the field at all, which is the
// instrument that works against a decoder that emits what it sees.
//
// The claim only holds for slots the schema actually TYPES, so the nested
// entries are typed too (§8.31): `columns` and `rows` publish `items` defs
// that are themselves additionalProperties:false, and the id slot inside
// them appears on the existing-content shape only. What is NOT typed here is
// the interior of a cell run (a cell is string | null | object | array of
// blocks — recursive, and a strict recursive def is a real cost to a
// constrained decoder); there the runtime guard is the instrument, and the
// descriptions say so rather than implying the schema covers it.
//
// v2OpBlockIndentProp and v2OpBlockCommonProps are the fields both shapes
// share, split only so the id slot can sit in its historical position. The
// full inventory is SPEC §5 — served as GET /v2/schemas/object; these defs
// cover the fields a generated edit realistically touches.
const v2OpBlockIndentProp = `"indent":{"type":"integer","minimum":0,"maximum":32,"description":"relative: 0 = the anchor's level (after/before/replace_subtree) or the container's child level (inside)"}`

// v2OpBlockIdProp is the EXISTING-content id slot.
const v2OpBlockIdProp = `"id":{"type":"string","pattern":"^[A-Za-z0-9_-]{1,64}$","description":"optional; names an existing block of this object (full id or unique suffix), keeping its identity. Omit it to author new content: the server mints one into created_blocks. An unknown id is refused."}`

// v2OpBlockTypeProp publishes the block-type vocabulary itself (§8.32). It
// used to be a bare {"type":"string","maxLength":64} beside a description
// pointing at another fetch — and a decoder cannot fetch. Asked for a
// checkbox item, gemma4:e2b wrote {"type":"bulleted_list_item","text":"[ ]
// Follow up"} 10 times out of 10: a plausible type plus a literal markdown
// checkbox in the text, which is what inventing a vocabulary looks like. The
// names come from anyblockjson.AuthorableBlockTypeNames — the format's own
// schema enum minus the §7 structural types — never from a copy kept here.
var v2OpBlockTypeProp = `"type":{"type":"string","enum":[` +
	strings.Join(quoteAll(anyblockjson.AuthorableBlockTypeNames()), ",") + `]}`

// quoteAll JSON-quotes each name of a published vocabulary.
func quoteAll(names []string) []string {
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = strconv.Quote(name)
	}
	return out
}

var v2OpBlockCommonProps = v2OpBlockTypeProp + `,` +
	`"text":{"type":"string","maxLength":1048576,"description":"inline markup source"},` +
	`"checked":{"type":"boolean"},` +
	`"color":{"type":"string","maxLength":64},` +
	`"language":{"type":"string","maxLength":64},` +
	`"processor":{"type":"string","maxLength":64},` +
	`"url":{"type":"string","maxLength":4096},` +
	`"object_id":{"type":"string","maxLength":256},` +
	`"name":{"type":"string","maxLength":4096},` +
	`"style":{"type":"string","maxLength":64},` +
	// §2b replaced the flat icon_emoji/icon_image pair with ONE typed member
	// whose `format` selects the variant — the same shape the object icon
	// uses, minus the two variants only an object has (plainIcon). A callout
	// is the only block type that carries it. Published here because both
	// payload defs are additionalProperties:false: a variant this schema does
	// not show is one a grammar-constrained decoder cannot author at all, and
	// TestSchemaOp checks every name published here against the format's own
	// block schema.
	`"icon":{"type":"object","additionalProperties":false,"required":["format"],"description":"a callout's icon: format selects the variant — emoji needs emoji, file needs file (an image object id in this space), icon needs name","properties":{` +
	`"format":{"type":"string","enum":["emoji","file","icon","color"]},` +
	`"emoji":{"type":"string","maxLength":64},` +
	`"file":{"type":"string","maxLength":256},` +
	`"name":{"type":"string","maxLength":64},` +
	`"color":{"type":"string","maxLength":64}}},` +
	`"icon_size":{"type":"string","maxLength":32},` +
	`"property":{"type":"string","maxLength":256},` +
	`"card_style":{"type":"string","maxLength":32},` +
	`"align":{"type":"string","enum":["left","center","right","justify"]},` +
	`"background_color":{"type":"string","maxLength":64}`

// v2OpTableInnerIdProp is the EXISTING-content id slot of a table row or
// column. Its charset has no dash on purpose: a cell's id is rowId+"-"+colId
// and the editor recovers the column by splitting on the first dash (SPEC
// §6.1), so a dash in either would be unrecoverable.
const v2OpTableInnerIdProp = `"id":{"type":"string","pattern":"^[A-Za-z0-9_]{1,64}$","description":"optional; names an existing row or column (full id or unique suffix) and keeps its identity. Omit it to author a new one: the server mints the id into created_blocks under this payload path."}`

// v2OpCellDef types one table cell. The four cell forms are SPEC §6.1; the
// array form's interior is left untyped — see the file header.
const v2OpCellDef = `{"type":["string","null","object","array"],"description":"a cell: a string (paragraph shorthand), null (empty), a block object, or a flat array whose first element is the cell block. a cell block takes no id; blocks inside follow the payload block's id rule."}`

// opTableProps builds the columns/rows properties of a payload block def.
// withId decides whether the row/column entries publish an id slot — the
// same §8.30 split the block itself gets, applied one level down, because a
// decoder emits whatever the schema shows it at ANY depth.
func opTableProps(withId bool) string {
	innerId := ""
	if withId {
		innerId = v2OpTableInnerIdProp + `,`
	}
	column := `{"type":"object","additionalProperties":false,"properties":{` + innerId +
		`"width":{"type":"number","minimum":0,"maximum":10000}}}`
	row := `{"type":"object","additionalProperties":false,"properties":{` + innerId +
		`"is_header":{"type":"boolean"},` +
		`"cells":{"type":"array","maxItems":64,"items":` + v2OpCellDef + `}}}`
	return `"columns":{"type":"array","maxItems":64,"items":` + column + `,"description":"table columns"},` +
		`"rows":{"type":"array","maxItems":1024,"items":` + row + `,"description":"table rows"}`
}

// v2OpBlockDef is the EXISTING-content payload block (replace_subtree): it
// publishes the id slot — on the block and on its row/column entries —
// because naming what the op replaces is what makes echoing a read back a
// no-op instead of a rename.
var v2OpBlockDef = `{"type":"object","additionalProperties":false,"required":["type"],` +
	`"description":"a flat AnyBlock block; the full field inventory is GET /v2/schemas/object",` +
	`"properties":{` + v2OpBlockIndentProp + `,` + v2OpBlockIdProp + `,` + v2OpBlockCommonProps + `,` + opTableProps(true) + `}}`

// v2OpNewBlockDef is the NEW-content payload block (insert_blocks): no id
// slot anywhere the schema reaches — not on the block, not on its row or
// column entries. There is no `views` property on EITHER shape, so a payload
// block cannot name a dataview view at all through this channel; views are
// authored by the view-family ops and by update_block's untyped `set`.
var v2OpNewBlockDef = `{"type":"object","additionalProperties":false,"required":["type"],` +
	`"description":"a flat AnyBlock block to create. No id slot here or on its rows and columns: the server mints every id into created_blocks, keyed by payload path. Full fields: GET /v2/schemas/object.",` +
	`"properties":{` + v2OpBlockIndentProp + `,` + v2OpBlockCommonProps + `,` + opTableProps(false) + `}}`

// v2BlockRefDef is a block reference: full id (canonical) or unique suffix.
const v2BlockRefDef = `{"type":"string","minLength":1,"maxLength":64,"description":"a block id — full (canonical) or a unique suffix"}`

// v2OpMatchPropDef is the `match` locator: the alternative to `id` on the
// ops that address one existing block by
// content. One published def, because one resolution rule serves them all —
// a second spelling here is how the two halves drift apart (§8.31).
const v2OpMatchPropDef = `"match":{"type":"string","minLength":1,"maxLength":65536,"description":"alternative to id: exact text from the block, markdown markup included. Must match exactly one block (repeats in it are fine); several are refused with candidate ids. Give id or match, never both."}`

// opSchema builds one op's strict schema. The op NAME is the first argument
// because THREE things are derived from it and must not be spelled
// independently: the `op` const, the required `op` field, and — through
// v2NewContentOps (ops.go) — which payload-block def the schema publishes.
// That last one is the point: the runtime reads the same set, so an op cannot
// advertise an id slot it will only ever refuse, which is §8.30's own bug.
// v2BlockShapedPayloadOps are the ops whose payload is block-shaped but
// deliberately UNTYPED: set_cell's `value` ("a block object / array of blocks")
// and update_block's `set` ("only the named fields change") are both bounded by
// the generic any-value, so nothing in their schema states what a block's
// fields are. For them $defs/block is documentation rather than machinery, and
// it is emitted even though no $ref points at it — pruning it by reachability
// would take away the only published field vocabulary those payloads have.
var v2BlockShapedPayloadOps = map[string]bool{
	"update_block": true,
	"set_cell":     true,
}

// v2OpEnvelopeProse says, on every op schema, that the schema describes ONE
// entry of the ops array rather than a whole request body. A caller that GETs
// /v2/schemas/ops/<op> otherwise has no way to learn the wrapper exists, and
// sends the op object as the body; parsePatchRequest names that mistake, but
// only after a failed round trip.
//
// This is prose, deliberately, and the distinction is measured. The `example`
// beside it stays a bare op object: wrapping the EXAMPLE cost gemma4:e4b a
// missing `op` field on 9 of 60 calls (models copy an example verbatim, and
// copying the wrapper lost the inner field), and unwrapping it took that to 0
// of 60. A description is not a copyable instance, so it buys the same
// knowledge without the shape to copy. schemas_ops_test.go pins the example
// against a regression here.
const v2OpEnvelopeProse = `"one entry of the ops array. The request body wraps entries: {\"ops\":[ this, ... ]}, 1 to 512 of them, applied in order as one edit: if any one op is refused, none of them is applied."`

func opSchema(op string, required []string, props ...string) string {
	blockDef := v2OpBlockDef
	if v2NewContentOps[op] {
		blockDef = v2OpNewBlockDef
	}
	req := make([]string, 0, len(required)+1)
	for _, name := range append([]string{"op"}, required...) {
		req = append(req, `"`+name+`"`)
	}
	all := append([]string{`"op":{"const":"` + op + `"}`}, props...)
	body := `"description":` + v2OpEnvelopeProse + `,"type":"object","additionalProperties":false,"required":[` +
		strings.Join(req, ",") + `],"properties":{` + strings.Join(all, ",") + `}`

	// emit only the definitions this op actually uses. Every op used to carry
	// BOTH, and only 2 of the 14 reference `block` (11 reference `blockRef`,
	// 3 reference neither) — so twelve op documents shipped a `block`
	// definition nothing in them pointed at.
	defs := make([]string, 0, 2)
	if strings.Contains(body, `"#/$defs/block"`) || v2BlockShapedPayloadOps[op] {
		defs = append(defs, `"block":`+blockDef)
	}
	if strings.Contains(body, `"#/$defs/blockRef"`) {
		defs = append(defs, `"blockRef":`+v2BlockRefDef)
	}
	if len(defs) == 0 {
		return `{` + body + `}`
	}
	return `{"$defs":{` + strings.Join(defs, ",") + `},` + body + `}`
}

// v2ViewBlockPropDef is the shared dataview-block targeting property of the
// view-family ops.
const v2ViewBlockPropDef = `"block":{"$ref":"#/$defs/blockRef","description":"a dataview block — optional when the object has exactly one (types, queries and collections do)"}`

// v2ViewFieldsDef is the authorable §6.2 view-level field set — one
// spelling for the view ops' set channel and for the views a query is
// created with (round-two eval F17: the query kind typed them as anyValue,
// and a caller guessed groupBy, then groupProperty, then gave up).
const v2ViewFieldsDef = `"name":{"type":["string","null"],"maxLength":4096},` +
	`"type":{"type":["string","null"],"enum":["table","list","gallery","kanban","calendar","graph",null]},` +
	`"group_by":{"type":["string","null"],"maxLength":256,"description":"property key to group by (kanban/board views)"},` +
	`"cover_property":{"type":["string","null"],"maxLength":256},` +
	`"end_property":{"type":["string","null"],"maxLength":256},` +
	`"hide_icon":{"type":["boolean","null"]},` +
	`"card_size":{"type":["string","null"],"enum":["small","medium","large",null]},` +
	`"cover_fit":{"type":["boolean","null"]},` +
	`"colored_groups":{"type":["boolean","null"]},` +
	`"page_size":{"type":["integer","null"],"minimum":0,"maximum":1000},` +
	`"default_template_id":{"type":["string","null"],"maxLength":256},` +
	`"default_type_id":{"type":["string","null"],"maxLength":256},` +
	`"wrap_content":{"type":["boolean","null"]},` +
	`"list_size":{"type":["string","null"],"enum":["compact","regular",null]},` +
	`"alternate_rows":{"type":["boolean","null"]},` +
	`"sorts":{"type":["array","null"],"maxItems":10,"items":{"type":"object","additionalProperties":false,"required":["property"],"properties":{"property":{"type":"string","maxLength":256},"direction":{"type":"string","enum":["asc","desc","custom"]},"custom_order":{"type":"array","maxItems":128},"empty_placement":{"type":"string","enum":["start","end"]},"include_time":{"type":"boolean"},"no_collate":{"type":"boolean"},"id":{"type":"string","maxLength":64,"description":"output-only on reads; accepted back so a read sort round-trips"}}}},` +
	`"filters":{"type":["array","null"],"maxItems":32,"description":"filter nodes (schema kind filters), at most 32 at the top level (group more under and/or nodes) — recursive, so prefer filter, the compact string"}`

// v2ViewCreateFieldsDef is v2ViewFieldsDef for a view that is CREATED whole
// rather than merged into: the same members, minus the null that means
// "clear to default" on the merge channel and is refused by the document a
// created view lands in, and with the filters advice restated for a shape
// that has no compact-string alternative.
func v2ViewCreateFieldsDef() string {
	var fields map[string]map[string]any
	if err := json.Unmarshal([]byte(`{`+v2ViewFieldsDef+`}`), &fields); err != nil {
		panic(fmt.Sprintf("view field defs: %v", err))
	}
	for _, field := range fields {
		if types, ok := field["type"].([]any); ok {
			if kept := withoutNull(types); len(kept) == 1 {
				field["type"] = kept[0]
			} else {
				field["type"] = kept
			}
		}
		if enum, ok := field["enum"].([]any); ok {
			field["enum"] = withoutNull(enum)
		}
	}
	fields["filters"]["description"] = "filter nodes (schema kind filters), at most 32 at the top level (group more under and/or nodes); a created view takes nodes only — the compact string is the query's top-level filter"
	out, err := json.Marshal(fields)
	if err != nil {
		panic(fmt.Sprintf("view create field defs: %v", err))
	}
	return strings.TrimSuffix(strings.TrimPrefix(string(out), "{"), "}")
}

// withoutNull drops the null of a JSON Schema list: the type name "null" in
// a type list, the literal null in an enum.
func withoutNull(values []any) []any {
	kept := make([]any, 0, len(values))
	for _, v := range values {
		if v == nil || v == "null" {
			continue
		}
		kept = append(kept, v)
	}
	return kept
}

// v2ViewColumnsListDef is a view's column list as a whole, the shape a
// created view carries (the ops merge per column through v2ViewColumnsPropDef).
const v2ViewColumnsListDef = `"columns":{"type":"array","maxItems":64,"items":{"type":"object","additionalProperties":false,"required":["property"],"properties":{` +
	`"property":{"type":"string","maxLength":256},"hidden":{"type":"boolean"},"width":{"type":"integer","minimum":0,"maximum":10000},` +
	`"align":{"type":"string","enum":["left","center","right","justify"]},` +
	`"aggregation":{"type":"string","enum":["count","count_value","count_distinct","count_empty","count_not_empty","percent_empty","percent_not_empty","sum","average","median","min","max","range"]}}}}`

// v2ViewSetPropDef is the shared `set` channel of update_view and insert_view:
// the authorable §6.2 view-level fields, merge semantics.
const v2ViewSetPropDef = `"set":{"type":"object","maxProperties":18,"additionalProperties":false,"description":"merge: only the named fields change, null clears one to its default. sorts and filters replace whole. filter is the compact-string alternative to filters; never both. Columns use the columns channel.","properties":{` +
	v2ViewFieldsDef + `,` +
	`"filter":{"type":"string","maxLength":4096,"description":"compact filter syntax (GET /v2/schemas/filters serves the grammar); parsed server-side into filters"}}}`

// v2ViewColumnsPropDef is the shared per-column merge channel.
const v2ViewColumnsPropDef = `"columns":{"type":"object","maxProperties":64,"description":"per-column patches keyed by property key: each merges into that property's column (appending one if absent); null removes the column; unnamed columns are untouched — never resend the whole column list","additionalProperties":{"type":["object","null"],"additionalProperties":false,"properties":{` +
	`"hidden":{"type":["boolean","null"],"description":"omitted/false = visible"},` +
	`"width":{"type":["integer","null"],"minimum":0,"maximum":10000,"description":"pixels; null/omitted lets the client pick per format"},` +
	`"align":{"type":["string","null"],"enum":["left","center","right","justify",null]},` +
	`"aggregation":{"type":["string","null"],"enum":["count","count_value","count_distinct","count_empty","count_not_empty","percent_empty","percent_not_empty","sum","average","median","min","max","range",null]}}}}`

// v2ViewSetPropDefNoName is insert_view's set channel: identical, minus name
// — insert_view's name is the op's required top-level field, and a set.name
// (null included) would silently defeat it (§8.19-E).
var v2ViewSetPropDefNoName = strings.Replace(strings.Replace(v2ViewSetPropDef,
	`"name":{"type":["string","null"],"maxLength":4096},`, "", 1),
	`"maxProperties":18`, `"maxProperties":17`, 1)

// v2TypePropertyRefDef is how the type ops name a property: the api key this
// surface serves, or a display name. Everything on this surface resolves the
// same way, and a caller who has only ever been shown the served key must be
// able to use it here.
const v2TypePropertyRefDef = `"property":{"type":"string","minLength":1,"maxLength":256,"description":"the property, by the key GET /v2/spaces/{space_id}/properties serves or by its display name. A name nothing answers to creates the property, which is what format is for. To rename a property send {name} to PATCH /v2/spaces/{space_id}/properties/{key}; do not re-send property_definitions."}`

// v2TypeSectionPropDef is where on the type the property sits. Null names
// the plain field list, the same way an explicit null clears a view field to
// its default.
const v2TypeSectionPropDef = `"section":{"type":["string","null"],"enum":["featured","hidden",null],"description":"featured shows the property at the top of an object, hidden keeps it off the object entirely. Null is the plain field list. Omitted leaves a property already on the type where it is."}`

const v2TypeAfterPropDef = `"after":{"type":"string","minLength":1,"maxLength":256,"description":"place it after this property of the same section; ordering against another section changes nothing and is refused"}`

const v2TypeBeforePropDef = `"before":{"type":"string","minLength":1,"maxLength":256,"description":"place it before this property of the same section"}`

const v2TypePositionPropDef = `"position":{"type":"string","enum":["first","last"],"description":"at most one of after, before and position; first makes this the leading field of its section"}`

// v2OpSchemas maps each PATCH op to its strict schema + single-op example.
var v2OpSchemas = map[string]v2SchemaKind{
	"set_properties": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("set_properties", nil,
			`"set":{"type":"object","maxProperties":128,"additionalProperties":{"type":["string","number","boolean","array","null"]},"description":"property key → value; presence is meaningful — an empty array means present-but-empty; unknown select option names are created"}`,
			`"unset":{"type":"array","maxItems":128,"items":{"type":"string","maxLength":256},"description":"property keys to remove"}`,
			`"add":{"type":"object","maxProperties":128,"additionalProperties":{"type":"array","maxItems":128,"items":{"type":"string","maxLength":4096}},"description":"list-shaped keys only (select, multi_select, objects, files): append entries without rewriting the array — existing entries are never duplicated; unknown option NAMES are created"}`,
			`"remove":{"type":"object","maxProperties":128,"additionalProperties":{"type":"array","maxItems":128,"items":{"type":"string","maxLength":4096}},"description":"list-shaped keys only: delete matching entries — absent entries (and absent keys) are a no-op; a key may appear in only one of set/unset/add/remove"}`),
		example: `{"op":"set_properties","set":{"status":["Done"]},"add":{"tags":["Urgent"]},"unset":["due_date"]}`,
	},
	"update_block": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("update_block", []string{"set"},
			`"id":{"$ref":"#/$defs/blockRef","description":"the block to update — give this or match, never both"}`,
			v2OpMatchPropDef,
			`"set":{"type":"object","maxProperties":32,"description":"merge semantics: only the named fields change — text included only if named; null clears a field; id and indent are rejected (use move_block to re-nest)"}`),
		example: `{"op":"update_block","match":"Draft timeline","set":{"checked":true}}`,
	},
	"replace_subtree": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("replace_subtree", []string{"id", "blocks"},
			`"id":{"$ref":"#/$defs/blockRef"}`,
			`"blocks":{"type":"array","minItems":1,"maxItems":256,"items":{"$ref":"#/$defs/block"},"description":"replaces the block AND its descendants; indent 0 = the replaced block's level"}`),
		example: `{"op":"replace_subtree","id":"b7","blocks":[{"type":"bulleted_list_item","text":"a"},{"indent":1,"type":"paragraph","text":"b"}]}`,
	},
	"insert_blocks": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("insert_blocks", nil,
			`"after":{"$ref":"#/$defs/blockRef","description":"insert after this block's subtree, at its level"}`,
			`"before":{"$ref":"#/$defs/blockRef","description":"insert before this block, at its level"}`,
			`"inside":{"$ref":"#/$defs/blockRef","description":"insert as children of this block"}`,
			`"position":{"type":"string","enum":["first","last"],"description":"which end to insert at: of the inside container, or of the document itself when no targeting field is given. Refused with after/before. first goes to the start, last appends (the default either way)."}`,
			`"blocks":{"type":"array","minItems":1,"maxItems":256,"items":{"$ref":"#/$defs/block"},"description":"at most one of after/before/inside targets the run — omit all three to insert at the end of the document (position:first for the start; both work on an empty object); indent 0 = the insertion level"}`,
			`"markdown":{"type":"string","minLength":1,"maxLength":1048576,"description":"alternative to blocks; give exactly one. Parsed into flat blocks (headings, lists, checkboxes, fences, quotes, dividers, tables), at most 256. created_blocks keys read ops[i].markdown[j]."}`),
		example: `{"op":"insert_blocks","after":"b3","markdown":"- [ ] todo"}`,
	},
	"move_block": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("move_block", []string{"id"},
			`"id":{"$ref":"#/$defs/blockRef","description":"the block to move — its whole subtree moves with it; omit after/before/inside to move it to the end of the document (position:first for the start)"}`,
			`"after":{"$ref":"#/$defs/blockRef"}`,
			`"before":{"$ref":"#/$defs/blockRef"}`,
			`"inside":{"$ref":"#/$defs/blockRef","description":"moving into the moved block's own subtree is a cycle → error"}`,
			`"position":{"type":"string","enum":["first","last"],"description":"which end to move to: of the inside container, or — with NO targeting field — of the document itself, so first moves the block to the start of the document and last to its end (the default either way)"}`),
		example: `{"op":"move_block","id":"b9","inside":"b2","position":"last"}`,
	},
	"delete_block": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("delete_block", nil,
			`"id":{"$ref":"#/$defs/blockRef","description":"the block to delete — give this or match, never both"}`,
			v2OpMatchPropDef,
			`"recursive":{"type":"boolean","description":"default false — deleting a block that has descendants without it is an error naming the descendant count and the resolved block id"}`),
		example: `{"op":"delete_block","match":"Obsolete section","recursive":true}`,
	},
	"replace_text": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("replace_text", []string{"find", "replace"},
			`"id":{"$ref":"#/$defs/blockRef","description":"optional — omit it and find locates the block: the find text must appear in exactly ONE block, or the op refuses (several matching blocks → the error lists candidate ids to retry with)"}`,
			`"find":{"type":"string","minLength":1,"maxLength":65536,"description":"exact text within one block, markup included. Must match once in that block unless replace_all. A match touching markup metadata (a link destination) is rejected. Without id it must find one block."}`,
			`"replace":{"type":"string","maxLength":65536,"description":"literal replacement, applied in the parsed text-and-marks model: markup bytes cannot create or remove marks. An empty string deletes the match. The result may not exceed 1048576 UTF-16 units."}`,
			`"replace_all":{"type":"boolean","description":"default false — replaces every occurrence WITHIN the one matched block; it never widens the locator to several blocks"}`),
		example: `{"op":"replace_text","find":"Q3","replace":"Q4"}`,
	},
	"set_cell": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("set_cell", []string{"table_id", "row", "col", "value"},
			`"table_id":{"$ref":"#/$defs/blockRef","description":"a table block"}`,
			`"row":{"type":"string","minLength":1,"maxLength":64,"description":"row id (full or unique suffix), or the row's first-cell text — case-insensitive, must name exactly one row"}`,
			`"col":{"type":"string","minLength":1,"maxLength":64,"description":"column id (full or unique suffix), or the column's header text (served as header on each column in the read) — case-insensitive, must name exactly one column"}`,
			`"value":{"type":["string","null","object","array"],"description":"string = paragraph shorthand, null = clear, or a block object / array of blocks"}`),
		example: `{"op":"set_cell","table_id":"t1","row":"r2","col":"c1","value":"done"}`,
	},
	"update_view": {
		endpoint: v2ViewOpsEndpoints,
		schema: opSchema("update_view", nil,
			v2ViewBlockPropDef,
			`"view":{"type":"string","minLength":1,"maxLength":64,"description":"view id, full or unique suffix — optional when the dataview has exactly one view"}`,
			v2ViewSetPropDef,
			v2ViewColumnsPropDef),
		example: `{"op":"update_view","columns":{"status":{"hidden":false}}}`,
	},
	"insert_view": {
		endpoint: v2ViewOpsEndpoints,
		schema: opSchema("insert_view", []string{"name"},
			v2ViewBlockPropDef,
			`"name":{"type":"string","minLength":1,"maxLength":4096,"description":"the new view's name (its tab label)"}`,
			`"copy_from":{"type":"string","minLength":1,"maxLength":64,"description":"duplicate this view of the same dataview, everything but its id and name, then apply set and columns on top. Omitted: defaults, every listed property visible, sorted by last_modified_date desc."}`,
			`"after":{"type":"string","minLength":1,"maxLength":64,"description":"insert after this view (id, full or unique suffix)"}`,
			`"before":{"type":"string","minLength":1,"maxLength":64,"description":"insert before this view"}`,
			`"position":{"type":"string","enum":["first","last"],"description":"at most one of after/before/position; omitted = append; the FIRST view is the client's default tab"}`,
			v2ViewSetPropDefNoName,
			v2ViewColumnsPropDef),
		example: `{"op":"insert_view","name":"Board","copy_from":"viewAll1","set":{"type":"kanban","group_by":"status"}}`,
	},
	"move_view": {
		endpoint: v2ViewOpsEndpoints,
		schema: opSchema("move_view", []string{"view"},
			v2ViewBlockPropDef,
			`"view":{"type":"string","minLength":1,"maxLength":64,"description":"the view to move (id, full or unique suffix)"}`,
			`"after":{"type":"string","minLength":1,"maxLength":64,"description":"move after this view"}`,
			`"before":{"type":"string","minLength":1,"maxLength":64,"description":"move before this view"}`,
			`"position":{"type":"string","enum":["first","last"],"description":"give exactly one of after/before/position — a destination is required; first makes the view the client's default tab"}`),
		example: `{"op":"move_view","view":"viewBoard2","position":"first"}`,
	},
	"delete_view": {
		endpoint: v2ViewOpsEndpoints,
		schema: opSchema("delete_view", []string{"view"},
			v2ViewBlockPropDef,
			`"view":{"type":"string","minLength":1,"maxLength":64,"description":"the view to delete (id, full or unique suffix) — deleting the last view is refused; per-view editor state goes with it"}`),
		example: `{"op":"delete_view","view":"viewBoard2"}`,
	},
	//
	// ---- the type ops (PATCH types/{type}) ----
	//
	// A different endpoint and a disjoint op set, on the same route so a
	// caller learns one discovery path for both. What makes these ops worth
	// having at all is stated in each description: the whole-type body
	// REPLACES its field list, which reads as "add" and is not.
	"add_property": {
		endpoint: v2TypeOpsEndpoint,
		schema: opSchema("add_property", []string{"property"},
			v2TypePropertyRefDef,
			`"format":{"type":"string","enum":[`+v2PropertyFormatEnum+`],"description":"required when no property answers to the name yet, which is the case where this op creates one; on one that exists it must match the format it already has"}`,
			v2TypeSectionPropDef,
			`"options":{"type":"array","maxItems":`+strconv.Itoa(maxV2PropertyOptions)+`,"description":"select and multi_select only: the option names the property may take. Creating one needs create_missing_options=true.","items":{"type":"object","additionalProperties":false,"required":["name"],"properties":{"name":{"type":"string","maxLength":4096},"color":{"type":"string","maxLength":64}}}}`,
			v2TypeAfterPropDef, v2TypeBeforePropDef, v2TypePositionPropDef),
		// no `options`: creating them needs ?create_missing_options=true, and an
		// example that only works with a non-default query param is one a
		// caller copies and gets refused by.
		example: `{"op":"add_property","property":"Harvest Season","format":"select","section":"featured"}`,
	},
	"remove_property": {
		endpoint: v2TypeOpsEndpoint,
		schema: opSchema("remove_property", []string{"property"},
			`"property":{"type":"string","minLength":1,"maxLength":256,"description":"the property to take off this type, by the key GET /v2/spaces/{space_id}/properties serves or by its display name. A property this type does not list is refused, never ignored. The property itself stays in the space with its values; only this type stops declaring it."}`),
		example: `{"op":"remove_property","property":"sun_needs"}`,
	},
	"move_property": {
		endpoint: v2TypeOpsEndpoint,
		schema: opSchema("move_property", []string{"property"},
			`"property":{"type":"string","minLength":1,"maxLength":256,"description":"the property to move, by the key GET /v2/spaces/{space_id}/properties serves or by its display name. This reorders within a section; add_property with section moves it between sections."}`,
			v2TypeAfterPropDef, v2TypeBeforePropDef,
			`"position":{"type":"string","enum":["first","last"],"description":"give exactly one of after, before or position. first makes this the type's leading field, within the section it sits in."}`),
		example: `{"op":"move_property","property":"harvest_season","position":"first"}`,
	},
	"add_items": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("add_items", []string{"items"},
			`"items":{"type":"array","minItems":1,"maxItems":1000,"items":{"type":"string","maxLength":256},"description":"member object ids to add to the collection (already-present ids are ignored)"}`),
		example: `{"op":"add_items","items":["bafyreieqh63jv…"]}`,
	},
	"remove_items": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("remove_items", []string{"items"},
			`"items":{"type":"array","minItems":1,"maxItems":1000,"items":{"type":"string","maxLength":256},"description":"member object ids to remove from the collection (absent ids are ignored)"}`),
		example: `{"op":"remove_items","items":["bafyreieqh63jv…"]}`,
	},
}

// SchemaOp implements GET /v2/schemas/ops/{op}.
func (s *Service) SchemaOp(op string) (v2model.SchemaEntry, error) {
	entry, ok := v2OpSchemas[op]
	if !ok {
		return v2model.SchemaEntry{}, v2model.NotFound(
			fmt.Sprintf("unknown op %q — available ops: %s", op, strings.Join(v2AllOpNames(), ", ")))
	}
	schema, err := strictDiscoverySchema(json.RawMessage(entry.schema))
	if err != nil {
		return v2model.SchemaEntry{}, fmt.Errorf("normalize op schema %q: %w", op, err)
	}
	return v2model.SchemaEntry{
		Kind:     op,
		Endpoint: entry.endpoint,
		Schema:   schema,
		Example:  json.RawMessage(entry.example),
	}, nil
}

// v2AllOpNames is every op this service serves a schema for, object ops
// first. The two endpoints keep their own lists — advertising a type op as
// an object op would publish a field no value of which can succeed — and
// this joins them only where a caller is being shown what exists.
func v2AllOpNames() []string {
	all := make([]string, 0, len(v2OpNames)+len(v2TypeOpNames))
	seen := make(map[string]bool, len(v2OpNames)+len(v2TypeOpNames))
	for _, list := range [][]string{v2OpNames, v2TypeOpNames} {
		for _, op := range list {
			if seen[op] {
				continue // the view family is on BOTH lists, by design
			}
			seen[op] = true
			all = append(all, op)
		}
	}
	return all
}
