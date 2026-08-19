package v2service

// schemas_ops.go is the Phase-3 per-op discovery (APIV2.md §5):
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
const v2OpBlockIdProp = `"id":{"type":"string","pattern":"^[A-Za-z0-9_-]{1,64}$","description":"optional; when present it must name an EXISTING block of this object — full id or unique suffix, resolved like every other id slot — and the payload keeps that block's identity. Omit it to author new content: the server mints an id and returns it in created_blocks under this payload path. An id that matches nothing is refused, never minted over."}`

// v2OpBlockTypeProp publishes the block-type vocabulary itself (§8.32). It
// used to be a bare {"type":"string","maxLength":64} beside a description
// pointing at another fetch — and a decoder cannot fetch. Asked for a
// checkbox item, gemma4:e2b wrote {"type":"bulletedListItem","text":"[ ]
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
	`"text":{"type":"string","maxLength":1048576,"description":"inline markup per SPEC §8"},` +
	`"checked":{"type":"boolean"},` +
	`"color":{"type":"string","maxLength":64},` +
	`"language":{"type":"string","maxLength":64},` +
	`"processor":{"type":"string","maxLength":64},` +
	`"url":{"type":"string","maxLength":4096},` +
	`"object_id":{"type":"string","maxLength":256},` +
	`"name":{"type":"string","maxLength":4096},` +
	`"style":{"type":"string","maxLength":64},` +
	// block ATTRIBUTE names are not key slots — §7.5a-4 excludes them by name,
	// and the format's own schema (served verbatim as GET /v2/schemas/object)
	// declares icon_emoji/icon_image. Re-spelled here, both defs being
	// additionalProperties:false meant a grammar-constrained decoder could not
	// author a callout icon at all, and two served schemas contradicted each
	// other one request apart. TestOpBlockPropsExistInTheFormatSchema is the
	// structural guard that keeps the exclusion out of prose.
	`"icon_emoji":{"type":"string","maxLength":64},` +
	`"icon_image":{"type":"string","maxLength":256},` +
	`"key":{"type":"string","maxLength":256},` +
	`"card_style":{"type":"string","maxLength":32},` +
	`"align":{"type":"string","enum":["left","center","right","justify"]},` +
	`"background_color":{"type":"string","maxLength":64}`

// v2OpTableInnerIdProp is the EXISTING-content id slot of a table row or
// column. Its charset has no dash on purpose: a cell's id is rowId+"-"+colId
// and the editor recovers the column by splitting on the first dash (SPEC
// §6.1), so a dash in either would be unrecoverable.
const v2OpTableInnerIdProp = `"id":{"type":"string","pattern":"^[A-Za-z0-9_]{1,64}$","description":"optional; names an EXISTING row/column of this table (full id or unique suffix) and keeps its identity. Omit it to author a new one — the server mints the id and returns it in created_blocks under this payload path."}`

// v2OpCellDef types one table cell. The four cell forms are SPEC §6.1; the
// array form's interior is left untyped — see the file header.
const v2OpCellDef = `{"type":["string","null","object","array"],"description":"a cell: a string (paragraph shorthand), null (empty), a block object, or a flat array of blocks whose first element is the cell block itself (SPEC §6.1). A cell block never carries an id — cell ids are derived rowId-colId; ids on the blocks INSIDE a cell run follow the same rule as the payload block's own id, enforced at runtime"}`

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
		`"isHeader":{"type":"boolean"},` +
		`"cells":{"type":"array","maxItems":64,"items":` + v2OpCellDef + `}}}`
	return `"columns":{"type":"array","maxItems":64,"items":` + column + `,"description":"table columns (SPEC §6.1)"},` +
		`"rows":{"type":"array","maxItems":1024,"items":` + row + `,"description":"table rows (SPEC §6.1)"}`
}

// v2OpBlockDef is the EXISTING-content payload block (replace_subtree): it
// publishes the id slot — on the block and on its row/column entries —
// because naming what the op replaces is what makes echoing a read back a
// no-op instead of a rename.
var v2OpBlockDef = `{"type":"object","additionalProperties":false,"required":["type"],` +
	`"description":"a flat AnyBlock block; the full field inventory is GET /v2/schemas/object (SPEC §5)",` +
	`"properties":{` + v2OpBlockIndentProp + `,` + v2OpBlockIdProp + `,` + v2OpBlockCommonProps + `,` + opTableProps(true) + `}}`

// v2OpNewBlockDef is the NEW-content payload block (insert_blocks): no id
// slot anywhere the schema reaches — not on the block, not on its row or
// column entries. There is no `views` property on EITHER shape, so a payload
// block cannot name a dataview view at all through this channel; views are
// authored by the view-family ops and by update_block's untyped `set`.
var v2OpNewBlockDef = `{"type":"object","additionalProperties":false,"required":["type"],` +
	`"description":"a flat AnyBlock block to CREATE. There is no id slot — not here and not on its rows or columns: this op only ever makes new content, so the server mints every id and returns it in created_blocks keyed by the payload path that produced it (a table's row and column ids included). Ids name EXISTING blocks, which is what the other ops address. The full field inventory is GET /v2/schemas/object (SPEC §5)",` +
	`"properties":{` + v2OpBlockIndentProp + `,` + v2OpBlockCommonProps + `,` + opTableProps(false) + `}}`

// v2BlockRefDef is a block reference: full id (canonical) or unique suffix.
const v2BlockRefDef = `{"type":"string","minLength":1,"maxLength":64,"description":"a block id — full (canonical) or a unique suffix"}`

// v2OpMatchPropDef is the `match` locator (Wave 2.1b, §8.45): the
// alternative to `id` on the ops that address one existing block by
// content. One published def, because one resolution rule serves them all —
// a second spelling here is how the two halves drift apart (§8.31).
const v2OpMatchPropDef = `"match":{"type":"string","minLength":1,"maxLength":65536,"description":"alternative to id: exact text from the block (inline markup included — text is markdown source), which must appear in exactly ONE block or the op refuses (several matching blocks → the error lists candidate ids to retry with; zero → read the outline). Give id or match, never both. Repeats WITHIN the one matched block are fine — this addresses the block, not an occurrence"}`

// opSchema builds one op's strict schema. The op NAME is the first argument
// because THREE things are derived from it and must not be spelled
// independently: the `op` const, the required `op` field, and — through
// v2NewContentOps (ops.go) — which payload-block def the schema publishes.
// That last one is the point: the runtime reads the same set, so an op cannot
// advertise an id slot it will only ever refuse, which is §8.30's own bug.
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
	return `{"$defs":{"block":` + blockDef + `,"blockRef":` + v2BlockRefDef + `},` +
		`"type":"object","additionalProperties":false,"required":[` + strings.Join(req, ",") + `],"properties":{` +
		strings.Join(all, ",") + `}}`
}

// v2ViewBlockPropDef is the shared dataview-block targeting property of the
// view-family ops.
const v2ViewBlockPropDef = `"block":{"$ref":"#/$defs/blockRef","description":"a dataview block — optional when the object has exactly one (types, sets and collections do)"}`

// v2ViewSetPropDef is the shared `set` channel of update_view and insert_view:
// the authorable §6.2 view-level fields, merge semantics.
const v2ViewSetPropDef = `"set":{"type":"object","maxProperties":18,"additionalProperties":false,"description":"merge semantics: only the named view fields change, null clears one back to its default; sorts and filters replace whole (small ordered lists); filter is the compact-string alternative to filters (give at most one of the two); columns are NOT set here — use the columns channel","properties":{` +
	`"name":{"type":["string","null"],"maxLength":4096},` +
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
	`"filters":{"type":["array","null"],"maxItems":32,"description":"SPEC §6.2 filter nodes (GET /v2/schemas/filters), at most 32 at the top level (group more under and/or nodes) — recursive, so small models should prefer filter, the compact string"},` +
	`"filter":{"type":"string","maxLength":4096,"description":"compact filter syntax (GET /v2/schemas/filters serves the grammar); parsed server-side into filters"}}}`

// v2ViewColumnsPropDef is the shared per-column merge channel.
const v2ViewColumnsPropDef = `"columns":{"type":"object","maxProperties":64,"description":"per-column patches keyed by property key: each merges into that property's column (appending one if absent); null removes the column; unnamed columns are untouched — never resend the whole column list","additionalProperties":{"type":["object","null"],"additionalProperties":false,"properties":{` +
	`"hidden":{"type":["boolean","null"],"description":"omitted/false = visible"},` +
	`"width":{"type":["integer","null"],"minimum":0,"maximum":10000,"description":"pixels; null/omitted lets the client pick per format (SPEC §6.2)"},` +
	`"align":{"type":["string","null"],"enum":["left","center","right","justify",null]},` +
	`"aggregation":{"type":["string","null"],"enum":["count","count_value","count_distinct","count_empty","count_not_empty","percent_empty","percent_not_empty","sum","average","median","min","max","range",null]}}}}`

// v2ViewSetPropDefNoName is insert_view's set channel: identical, minus name
// — insert_view's name is the op's required top-level field, and a set.name
// (null included) would silently defeat it (§8.19-E).
var v2ViewSetPropDefNoName = strings.Replace(strings.Replace(v2ViewSetPropDef,
	`"name":{"type":["string","null"],"maxLength":4096},`, "", 1),
	`"maxProperties":18`, `"maxProperties":17`, 1)

// v2OpSchemas maps each PATCH op to its strict schema + single-op example.
var v2OpSchemas = map[string]v2SchemaKind{
	"set_properties": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("set_properties", nil,
			`"set":{"type":"object","maxProperties":128,"additionalProperties":{"type":["string","number","boolean","array","null"]},"description":"property key → value; presence is meaningful — an empty array means present-but-empty (SPEC §3); unknown select option NAMES are created"}`,
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
		example: `{"op":"replace_subtree","id":"b7","blocks":[{"type":"bulletedListItem","text":"a"},{"indent":1,"type":"paragraph","text":"b"}]}`,
	},
	"insert_blocks": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("insert_blocks", nil,
			`"after":{"$ref":"#/$defs/blockRef","description":"insert after this block's subtree, at its level"}`,
			`"before":{"$ref":"#/$defs/blockRef","description":"insert before this block, at its level"}`,
			`"inside":{"$ref":"#/$defs/blockRef","description":"insert as children of this block"}`,
			`"position":{"type":"string","enum":["first","last"],"description":"which end to insert at: of the inside container, or — with NO targeting field — of the document itself, so first inserts at the start of the document and last appends at its end (the default either way)"}`,
			`"blocks":{"type":"array","minItems":1,"maxItems":256,"items":{"$ref":"#/$defs/block"},"description":"at most one of after/before/inside targets the run — omit all three to insert at the end of the document (position:first for the start; both work on an empty object); indent 0 = the insertion level"}`,
			`"markdown":{"type":"string","minLength":1,"maxLength":1048576,"description":"authoring alternative to blocks (give exactly one): the server parses markdown into flat blocks — headings, lists, checkboxes, fences, quotes, dividers, tables; same targeting; at most 256 parsed blocks per op (the blocks channel's cap); created_blocks keys read markdown[j] for the j-th parsed block"}`),
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
			`"find":{"type":"string","minLength":1,"maxLength":65536,"description":"exact text within one block's text (inline markup included) — must match exactly once in that block unless replace_all; with id omitted it is also the locator and must identify exactly one block"}`,
			`"replace":{"type":"string","maxLength":65536}`,
			`"replace_all":{"type":"boolean","description":"default false — replaces every occurrence WITHIN the one matched block; it never widens the locator to several blocks"}`),
		example: `{"op":"replace_text","find":"Q3","replace":"Q4"}`,
	},
	"set_cell": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("set_cell", []string{"table_id", "row", "col", "value"},
			`"table_id":{"$ref":"#/$defs/blockRef","description":"a table block"}`,
			`"row":{"type":"string","minLength":1,"maxLength":64,"description":"row id — full or unique suffix"}`,
			`"col":{"type":"string","minLength":1,"maxLength":64,"description":"column id — full or unique suffix"}`,
			`"value":{"type":["string","null","object","array"],"description":"string = paragraph shorthand, null = clear, or a block object / array of blocks (SPEC §6.1 cell forms)"}`),
		example: `{"op":"set_cell","table_id":"t1","row":"r2","col":"c1","value":"done"}`,
	},
	"update_view": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("update_view", nil,
			v2ViewBlockPropDef,
			`"view":{"type":"string","minLength":1,"maxLength":64,"description":"view id, full or unique suffix — optional when the dataview has exactly one view"}`,
			v2ViewSetPropDef,
			v2ViewColumnsPropDef),
		example: `{"op":"update_view","columns":{"status":{"hidden":false}}}`,
	},
	"insert_view": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("insert_view", []string{"name"},
			v2ViewBlockPropDef,
			`"name":{"type":"string","minLength":1,"maxLength":4096,"description":"the new view's name (its tab label)"}`,
			`"copy_from":{"type":"string","minLength":1,"maxLength":64,"description":"duplicate this view of the same dataview (columns, sorts, filters, type — everything but id and name), then apply set/columns on top; omitted = defaults (every listed property visible, sorted by last_modified_date desc)"}`,
			`"after":{"type":"string","minLength":1,"maxLength":64,"description":"insert after this view (id, full or unique suffix)"}`,
			`"before":{"type":"string","minLength":1,"maxLength":64,"description":"insert before this view"}`,
			`"position":{"type":"string","enum":["first","last"],"description":"at most one of after/before/position; omitted = append; the FIRST view is the client's default tab"}`,
			v2ViewSetPropDefNoName,
			v2ViewColumnsPropDef),
		example: `{"op":"insert_view","name":"Board","copy_from":"viewAll1","set":{"type":"kanban","group_by":"status"}}`,
	},
	"move_view": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("move_view", []string{"view"},
			v2ViewBlockPropDef,
			`"view":{"type":"string","minLength":1,"maxLength":64,"description":"the view to move (id, full or unique suffix)"}`,
			`"after":{"type":"string","minLength":1,"maxLength":64,"description":"move after this view"}`,
			`"before":{"type":"string","minLength":1,"maxLength":64,"description":"move before this view"}`,
			`"position":{"type":"string","enum":["first","last"],"description":"give exactly one of after/before/position — a destination is required; first makes the view the client's default tab"}`),
		example: `{"op":"move_view","view":"viewBoard2","position":"first"}`,
	},
	"delete_view": {
		endpoint: v2OpsEndpoint,
		schema: opSchema("delete_view", []string{"view"},
			v2ViewBlockPropDef,
			`"view":{"type":"string","minLength":1,"maxLength":64,"description":"the view to delete (id, full or unique suffix) — deleting the last view is refused; per-view editor state goes with it"}`),
		example: `{"op":"delete_view","view":"viewBoard2"}`,
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
			fmt.Sprintf("unknown op %q — available ops: %s", op, strings.Join(v2OpNames, ", ")))
	}
	return v2model.SchemaEntry{
		Kind:     op,
		Endpoint: entry.endpoint,
		Schema:   json.RawMessage(entry.schema),
		Example:  json.RawMessage(entry.example),
	}, nil
}
