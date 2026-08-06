package service

// v2_schemas_ops.go is the Phase-3 per-op discovery (APIV2.md §5):
// GET /v2/schemas/ops/{op} serves one tiny C13-strict schema and a single-op
// minimal example per PATCH op, so the smallest consumers stay at the
// smallest schema surface. The multi-op composite example remains on the
// PATCH endpoint docs as the secondary illustration.

import (
	"encoding/json"
	"fmt"
	"strings"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
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
func (s *V2Service) SchemaOp(op string) (apimodel.V2SchemaEntry, error) {
	entry, ok := v2OpSchemas[op]
	if !ok {
		return apimodel.V2SchemaEntry{}, apimodel.V2NotFound(
			fmt.Sprintf("unknown op %q — available ops: %s", op, strings.Join(v2OpNames, ", ")))
	}
	return apimodel.V2SchemaEntry{
		Kind:     op,
		Endpoint: entry.endpoint,
		Schema:   json.RawMessage(entry.schema),
		Example:  json.RawMessage(entry.example),
	}, nil
}
