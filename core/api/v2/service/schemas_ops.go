package v2service

// schemas_ops.go is the Phase-3 per-op discovery (APIV2.md §5):
// GET /v2/schemas/ops/{op} serves one tiny C13-strict schema and a single-op
// minimal example per PATCH op, so the smallest consumers stay at the
// smallest schema surface. The multi-op composite example remains on the
// PATCH endpoint docs as the secondary illustration.

import (
	"encoding/json"
	"fmt"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// v2OpsEndpoint is the endpoint every op schema belongs to.
const v2OpsEndpoint = "PATCH /v2/spaces/{spaceId}/objects/{objectId}"

// v2OpBlockDef is the shared payload-block definition: the common AnyBlock
// block fields, strict and non-recursive (C13). The full inventory is
// SPEC §5 — served as GET /v2/schemas/object; this def covers the fields a
// generated edit realistically touches.
const v2OpBlockDef = `{"type":"object","additionalProperties":false,"required":["type"],` +
	`"description":"a flat AnyBlock block; the full field inventory is GET /v2/schemas/object (SPEC §5)",` +
	`"properties":{` +
	`"indent":{"type":"integer","minimum":0,"maximum":32,"description":"relative: 0 = the anchor's level (after/before/replaceSubtree) or the container's child level (inside)"},` +
	`"id":{"type":"string","pattern":"^[A-Za-z0-9_-]{1,64}$","description":"optional; omitted ids are minted and returned in createdBlocks"},` +
	`"type":{"type":"string","maxLength":64},` +
	`"text":{"type":"string","maxLength":1048576,"description":"inline markup per SPEC §8"},` +
	`"checked":{"type":"boolean"},` +
	`"color":{"type":"string","maxLength":64},` +
	`"language":{"type":"string","maxLength":64},` +
	`"processor":{"type":"string","maxLength":64},` +
	`"url":{"type":"string","maxLength":4096},` +
	`"objectId":{"type":"string","maxLength":256},` +
	`"name":{"type":"string","maxLength":4096},` +
	`"style":{"type":"string","maxLength":64},` +
	`"iconEmoji":{"type":"string","maxLength":64},` +
	`"iconImage":{"type":"string","maxLength":256},` +
	`"key":{"type":"string","maxLength":256},` +
	`"cardStyle":{"type":"string","maxLength":32},` +
	`"align":{"type":"string","enum":["left","center","right","justify"]},` +
	`"backgroundColor":{"type":"string","maxLength":64},` +
	`"columns":{"type":"array","maxItems":64,"description":"table columns (SPEC §6.1)"},` +
	`"rows":{"type":"array","maxItems":1024,"description":"table rows (SPEC §6.1)"}}}`

// v2BlockRefDef is a block reference: full id (canonical) or unique suffix.
const v2BlockRefDef = `{"type":"string","minLength":1,"maxLength":64,"description":"a block id — full (canonical) or a unique suffix"}`

func opSchema(required string, props ...string) string {
	return `{"$defs":{"block":` + v2OpBlockDef + `,"blockRef":` + v2BlockRefDef + `},` +
		`"type":"object","additionalProperties":false,"required":[` + required + `],"properties":{` +
		strings.Join(props, ",") + `}}`
}

// v2ViewBlockPropDef is the shared dataview-block targeting property of the
// view-family ops.
const v2ViewBlockPropDef = `"block":{"$ref":"#/$defs/blockRef","description":"a dataview block — optional when the object has exactly one (types, sets and collections do)"}`

// v2ViewSetPropDef is the shared `set` channel of updateView and insertView:
// the authorable §6.2 view-level fields, merge semantics.
const v2ViewSetPropDef = `"set":{"type":"object","maxProperties":18,"additionalProperties":false,"description":"merge semantics: only the named view fields change, null clears one back to its default; sorts and filters replace whole (small ordered lists); filter is the compact-string alternative to filters (give at most one of the two); columns are NOT set here — use the columns channel","properties":{` +
	`"name":{"type":["string","null"],"maxLength":4096},` +
	`"type":{"type":["string","null"],"enum":["table","list","gallery","kanban","calendar","graph",null]},` +
	`"groupBy":{"type":["string","null"],"maxLength":256,"description":"property key to group by (kanban/board views)"},` +
	`"coverProperty":{"type":["string","null"],"maxLength":256},` +
	`"endProperty":{"type":["string","null"],"maxLength":256},` +
	`"hideIcon":{"type":["boolean","null"]},` +
	`"cardSize":{"type":["string","null"],"enum":["small","medium","large",null]},` +
	`"coverFit":{"type":["boolean","null"]},` +
	`"coloredGroups":{"type":["boolean","null"]},` +
	`"pageSize":{"type":["integer","null"],"minimum":0,"maximum":1000},` +
	`"defaultTemplateId":{"type":["string","null"],"maxLength":256},` +
	`"defaultTypeId":{"type":["string","null"],"maxLength":256},` +
	`"wrapContent":{"type":["boolean","null"]},` +
	`"listSize":{"type":["string","null"],"enum":["compact","regular",null]},` +
	`"alternateRows":{"type":["boolean","null"]},` +
	`"sorts":{"type":["array","null"],"maxItems":10,"items":{"type":"object","additionalProperties":false,"required":["property"],"properties":{"property":{"type":"string","maxLength":256},"direction":{"type":"string","enum":["asc","desc","custom"]},"customOrder":{"type":"array","maxItems":128},"emptyPlacement":{"type":"string","enum":["start","end"]},"includeTime":{"type":"boolean"},"noCollate":{"type":"boolean"}}}},` +
	`"filters":{"type":["array","null"],"description":"SPEC §6.2 filter nodes (GET /v2/schemas/filters) — recursive, so small models should prefer filter, the compact string"},` +
	`"filter":{"type":"string","maxLength":4096,"description":"compact filter syntax (GET /v2/schemas/filters serves the grammar); parsed server-side into filters"}}}`

// v2ViewColumnsPropDef is the shared per-column merge channel.
const v2ViewColumnsPropDef = `"columns":{"type":"object","maxProperties":64,"description":"per-column patches keyed by property key: each merges into that property's column (appending one if absent); null removes the column; unnamed columns are untouched — never resend the whole column list","additionalProperties":{"type":["object","null"],"additionalProperties":false,"properties":{` +
	`"hidden":{"type":["boolean","null"],"description":"omitted/false = visible"},` +
	`"width":{"type":["integer","null"],"minimum":0,"maximum":10000,"description":"pixels; null/omitted lets the client pick per format (SPEC §6.2)"},` +
	`"align":{"type":["string","null"],"enum":["left","center","right","justify",null]},` +
	`"aggregation":{"type":["string","null"],"enum":["count","countValue","countDistinct","countEmpty","countNotEmpty","percentEmpty","percentNotEmpty","sum","average","median","min","max","range",null]}}}}`

// v2OpSchemas maps each PATCH op to its strict schema + single-op example.
var v2OpSchemas = map[string]v2SchemaKind{
	"setProperties": {
		endpoint: v2OpsEndpoint,
		schema: opSchema(`"op"`,
			`"op":{"const":"setProperties"}`,
			`"set":{"type":"object","maxProperties":128,"additionalProperties":{"type":["string","number","boolean","array","null"]},"description":"property key → value; presence is meaningful — an empty array means present-but-empty (SPEC §3); unknown select option NAMES are created"}`,
			`"unset":{"type":"array","maxItems":128,"items":{"type":"string","maxLength":256},"description":"property keys to remove"}`,
			`"add":{"type":"object","maxProperties":128,"additionalProperties":{"type":"array","maxItems":128,"items":{"type":"string","maxLength":4096}},"description":"list-shaped keys only (select, multiSelect, objects, files): append entries without rewriting the array — existing entries are never duplicated; unknown option NAMES are created"}`,
			`"remove":{"type":"object","maxProperties":128,"additionalProperties":{"type":"array","maxItems":128,"items":{"type":"string","maxLength":4096}},"description":"list-shaped keys only: delete matching entries — absent entries (and absent keys) are a no-op; a key may appear in only one of set/unset/add/remove"}`),
		example: `{"ops":[{"op":"setProperties","set":{"status":["Done"]},"add":{"tags":["Urgent"]},"unset":["dueDate"]}]}`,
	},
	"updateBlock": {
		endpoint: v2OpsEndpoint,
		schema: opSchema(`"op","id","set"`,
			`"op":{"const":"updateBlock"}`,
			`"id":{"$ref":"#/$defs/blockRef"}`,
			`"set":{"type":"object","maxProperties":32,"description":"merge semantics: only the named fields change — text included only if named; null clears a field; id and indent are rejected (use moveBlock to re-nest)"}`),
		example: `{"ops":[{"op":"updateBlock","id":"b5","set":{"checked":true}}]}`,
	},
	"replaceSubtree": {
		endpoint: v2OpsEndpoint,
		schema: opSchema(`"op","id","blocks"`,
			`"op":{"const":"replaceSubtree"}`,
			`"id":{"$ref":"#/$defs/blockRef"}`,
			`"blocks":{"type":"array","minItems":1,"maxItems":256,"items":{"$ref":"#/$defs/block"},"description":"replaces the block AND its descendants; indent 0 = the replaced block's level"}`),
		example: `{"ops":[{"op":"replaceSubtree","id":"b7","blocks":[{"type":"bulletedListItem","text":"a"},{"indent":1,"type":"paragraph","text":"b"}]}]}`,
	},
	"insertBlocks": {
		endpoint: v2OpsEndpoint,
		schema: opSchema(`"op"`,
			`"op":{"const":"insertBlocks"}`,
			`"after":{"$ref":"#/$defs/blockRef","description":"insert after this block's subtree, at its level"}`,
			`"before":{"$ref":"#/$defs/blockRef","description":"insert before this block, at its level"}`,
			`"inside":{"$ref":"#/$defs/blockRef","description":"insert as children of this block"}`,
			`"position":{"type":"string","enum":["first","last"],"description":"with inside only; default last"}`,
			`"blocks":{"type":"array","minItems":1,"maxItems":256,"items":{"$ref":"#/$defs/block"},"description":"at most one of after/before/inside targets the run — omit all three to append at the end of the document (works on an empty object too); indent 0 = the insertion level"}`,
			`"markdown":{"type":"string","minLength":1,"maxLength":1048576,"description":"authoring alternative to blocks (give exactly one): the server parses markdown into flat blocks — headings, lists, checkboxes, fences, quotes, dividers, tables; same targeting; at most 256 parsed blocks per op (the blocks channel's cap); createdBlocks keys read markdown[j] for the j-th parsed block"}`),
		example: `{"ops":[{"op":"insertBlocks","after":"b3","markdown":"- [ ] todo"}]}`,
	},
	"moveBlock": {
		endpoint: v2OpsEndpoint,
		schema: opSchema(`"op","id"`,
			`"op":{"const":"moveBlock"}`,
			`"id":{"$ref":"#/$defs/blockRef","description":"the block to move — its whole subtree moves with it; omit after/before/inside to move it to the end of the document"}`,
			`"after":{"$ref":"#/$defs/blockRef"}`,
			`"before":{"$ref":"#/$defs/blockRef"}`,
			`"inside":{"$ref":"#/$defs/blockRef","description":"moving into the moved block's own subtree is a cycle → error"}`,
			`"position":{"type":"string","enum":["first","last"],"description":"with inside only; default last"}`),
		example: `{"ops":[{"op":"moveBlock","id":"b9","inside":"b2","position":"last"}]}`,
	},
	"deleteBlock": {
		endpoint: v2OpsEndpoint,
		schema: opSchema(`"op","id"`,
			`"op":{"const":"deleteBlock"}`,
			`"id":{"$ref":"#/$defs/blockRef"}`,
			`"recursive":{"type":"boolean","description":"default false — deleting a block that has descendants without it is an error naming the descendant count"}`),
		example: `{"ops":[{"op":"deleteBlock","id":"b4","recursive":true}]}`,
	},
	"replaceText": {
		endpoint: v2OpsEndpoint,
		schema: opSchema(`"op","id","find","replace"`,
			`"op":{"const":"replaceText"}`,
			`"id":{"$ref":"#/$defs/blockRef"}`,
			`"find":{"type":"string","minLength":1,"maxLength":65536,"description":"exact text within this one block's text (inline markup included) — must match exactly once unless replace_all"}`,
			`"replace":{"type":"string","maxLength":65536}`,
			`"replace_all":{"type":"boolean","description":"default false"}`),
		example: `{"ops":[{"op":"replaceText","id":"b2","find":"Q3","replace":"Q4"}]}`,
	},
	"setCell": {
		endpoint: v2OpsEndpoint,
		schema: opSchema(`"op","tableId","row","col","value"`,
			`"op":{"const":"setCell"}`,
			`"tableId":{"$ref":"#/$defs/blockRef","description":"a table block"}`,
			`"row":{"type":"string","minLength":1,"maxLength":64,"description":"row id — full or unique suffix"}`,
			`"col":{"type":"string","minLength":1,"maxLength":64,"description":"column id — full or unique suffix"}`,
			`"value":{"type":["string","null","object","array"],"description":"string = paragraph shorthand, null = clear, or a block object / array of blocks (SPEC §6.1 cell forms)"}`),
		example: `{"ops":[{"op":"setCell","tableId":"t1","row":"r2","col":"c1","value":"done"}]}`,
	},
	"updateView": {
		endpoint: v2OpsEndpoint,
		schema: opSchema(`"op"`,
			`"op":{"const":"updateView"}`,
			v2ViewBlockPropDef,
			`"view":{"type":"string","minLength":1,"maxLength":64,"description":"view id, full or unique suffix — optional when the dataview has exactly one view"}`,
			v2ViewSetPropDef,
			v2ViewColumnsPropDef),
		example: `{"ops":[{"op":"updateView","columns":{"status":{"hidden":false}}}]}`,
	},
	"insertView": {
		endpoint: v2OpsEndpoint,
		schema: opSchema(`"op","name"`,
			`"op":{"const":"insertView"}`,
			v2ViewBlockPropDef,
			`"name":{"type":"string","minLength":1,"maxLength":4096,"description":"the new view's name (its tab label)"}`,
			`"copyFrom":{"type":"string","minLength":1,"maxLength":64,"description":"duplicate this view of the same dataview (columns, sorts, filters, type — everything but id and name), then apply set/columns on top; omitted = defaults (every listed property visible, sorted by lastModifiedDate desc)"}`,
			`"after":{"type":"string","minLength":1,"maxLength":64,"description":"insert after this view (id, full or unique suffix)"}`,
			`"before":{"type":"string","minLength":1,"maxLength":64,"description":"insert before this view"}`,
			`"position":{"type":"string","enum":["first","last"],"description":"at most one of after/before/position; omitted = append; the FIRST view is the client's default tab"}`,
			v2ViewSetPropDef,
			v2ViewColumnsPropDef),
		example: `{"ops":[{"op":"insertView","name":"Board","copyFrom":"viewAll1","set":{"type":"kanban","groupBy":"status"}}]}`,
	},
	"moveView": {
		endpoint: v2OpsEndpoint,
		schema: opSchema(`"op","view"`,
			`"op":{"const":"moveView"}`,
			v2ViewBlockPropDef,
			`"view":{"type":"string","minLength":1,"maxLength":64,"description":"the view to move (id, full or unique suffix)"}`,
			`"after":{"type":"string","minLength":1,"maxLength":64,"description":"move after this view"}`,
			`"before":{"type":"string","minLength":1,"maxLength":64,"description":"move before this view"}`,
			`"position":{"type":"string","enum":["first","last"],"description":"at most one of after/before/position; omitted = move to the end; first makes the view the client's default tab"}`),
		example: `{"ops":[{"op":"moveView","view":"viewBoard2","position":"first"}]}`,
	},
	"deleteView": {
		endpoint: v2OpsEndpoint,
		schema: opSchema(`"op","view"`,
			`"op":{"const":"deleteView"}`,
			v2ViewBlockPropDef,
			`"view":{"type":"string","minLength":1,"maxLength":64,"description":"the view to delete (id, full or unique suffix) — deleting the last view is refused; per-view editor state goes with it"}`),
		example: `{"ops":[{"op":"deleteView","view":"viewBoard2"}]}`,
	},
	"addItems": {
		endpoint: v2OpsEndpoint,
		schema: opSchema(`"op","items"`,
			`"op":{"const":"addItems"}`,
			`"items":{"type":"array","minItems":1,"maxItems":1000,"items":{"type":"string","maxLength":256},"description":"member object ids to add to the collection (already-present ids are ignored)"}`),
		example: `{"ops":[{"op":"addItems","items":["bafyreieqh63jv…"]}]}`,
	},
	"removeItems": {
		endpoint: v2OpsEndpoint,
		schema: opSchema(`"op","items"`,
			`"op":{"const":"removeItems"}`,
			`"items":{"type":"array","minItems":1,"maxItems":1000,"items":{"type":"string","maxLength":256},"description":"member object ids to remove from the collection (absent ids are ignored)"}`),
		example: `{"ops":[{"op":"removeItems","items":["bafyreieqh63jv…"]}]}`,
	},
}

// SchemaOp implements GET /v2/schemas/ops/{op}.
func (s *V2Service) SchemaOp(op string) (v2model.SchemaEntry, error) {
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
