package service

// v2_schemas.go implements the §5 discovery surface for the create kinds
// and the Phase-4 search kind: GET /v2/schemas (index) and
// GET /v2/schemas/{kind} (JSON Schema + one worked example, C12). All
// generation-facing schemas are strict-mode-compatible (C13):
// additionalProperties:false, bounded, non-recursive — with the documented
// exception of the AnyBlock document schema's filter tree (served verbatim
// from the format package) and the `filters` kind itself, which is
// recursive by nature and documented as such; small models are steered to
// the compact filter string, whose grammar (EBNF + examples) is served ON
// the `filters` kind — one concept, one discovery slot (C2), the artifact
// the Phase-5 GBNF conversion consumes.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
)

// v2SchemaKind is one discoverable kind.
type v2SchemaKind struct {
	endpoint string
	schema   string // JSON Schema (compact); "" = the embedded AnyBlock document schema
	example  string
}

var v2SchemaKinds = map[string]v2SchemaKind{
	"object": {
		endpoint: "POST /v2/spaces/{spaceId}/objects",
		// the full AnyBlock document schema is served verbatim (schema: "")
		example: `{"version":1,"type":"task","properties":{"name":"Prepare the Q3 report","status":["In progress"],"dueDate":"2026-08-01T00:00:00Z"},"blocks":[{"type":"heading2","text":"Steps"},{"type":"checkbox","text":"Collect the numbers"},{"indent":1,"type":"paragraph","text":"Ask **finance** first"}]}`,
	},
	"shortcut": {
		endpoint: "POST /v2/spaces/{spaceId}/objects",
		schema: `{"type":"object","additionalProperties":false,"required":["type"],"properties":{` +
			`"type":{"type":"string","maxLength":256,"description":"type key, e.g. page or task"},` +
			`"name":{"type":"string","maxLength":4096},` +
			`"properties":{"type":"object","maxProperties":128,"additionalProperties":{"type":["string","number","boolean","array","null"]}},` +
			`"markdown":{"type":"string","maxLength":1048576,"description":"markdown body parsed into blocks server-side — part of the same single create (dry runs validate it too); at most 2048 parsed blocks"}}}`,
		example: `{"type":"task","name":"Buy milk","properties":{"dueDate":"2026-08-01T00:00:00Z"},"markdown":"- [ ] oat\n- [ ] whole"}`,
	},
	"type": {
		endpoint: "POST /v2/spaces/{spaceId}/types",
		// a type document is an AnyBlock document (kind objectType)
		example: `{"version":1,"kind":"objectType","key":"task","properties":{"name":"Task","iconEmoji":"✅","recommendedLayout":"todo"},"typeProperties":[{"key":"dueDate","name":"Due date","format":"date","section":"featured"},{"key":"status","name":"Status","format":"select"}]}`,
	},
	"template": {
		endpoint: "POST /v2/spaces/{spaceId}/templates",
		example:  `{"version":1,"type":"template","templateFor":"task","properties":{"name":"Weekly task"},"blocks":[{"type":"heading2","text":"Checklist"},{"type":"checkbox","text":"First step"}]}`,
	},
	"property": {
		endpoint: "POST /v2/spaces/{spaceId}/properties",
		schema: `{"type":"object","additionalProperties":false,"required":["name","format"],"properties":{` +
			`"key":{"type":"string","maxLength":256,"pattern":"^[a-zA-Z0-9_]+$"},` +
			`"name":{"type":"string","maxLength":4096},` +
			`"format":{"type":"string","enum":["text","number","select","multiSelect","date","files","checkbox","url","email","phone","objects"]},` +
			`"options":{"type":"array","maxItems":100,"items":{"type":"object","additionalProperties":false,"required":["name"],"properties":{` +
			`"name":{"type":"string","maxLength":4096},"color":{"type":"string","maxLength":64}}}}}}`,
		example: `{"key":"priority","name":"Priority","format":"select","options":[{"name":"High","color":"red"},{"name":"Low"}]}`,
	},
	"set": {
		endpoint: "POST /v2/spaces/{spaceId}/sets",
		schema: `{"type":"object","additionalProperties":false,"required":["name","type"],"properties":{` +
			`"name":{"type":"string","maxLength":4096},` +
			`"type":{"type":"string","maxLength":256,"description":"the queried type's key"},` +
			`"filter":{"type":"string","maxLength":4096,"description":"compact filter string (grammar on kind filters); the endpoint also accepts a recursive structured filters array, kept out of this schema so it stays strict-mode-decodable (C13) — see kind filters"},` +
			`"sorts":{"type":"array","maxItems":10,"items":{"type":"object","additionalProperties":false,"required":["property"],"properties":{` +
			`"property":{"type":"string","maxLength":256},"direction":{"type":"string","enum":["asc","desc"]},"emptyPlacement":{"type":"string","enum":["start","end"]}}}},` +
			`"views":{"type":"array","maxItems":10,"description":"full SPEC §6.2 view objects; mutually exclusive with top-level filter/sorts"}}}`,
		example: `{"name":"Open tasks","type":"task","filter":"done = false","sorts":[{"property":"dueDate","direction":"asc"}]}`,
	},
	"collection": {
		endpoint: "POST /v2/spaces/{spaceId}/collections",
		schema: `{"type":"object","additionalProperties":false,"required":["name"],"properties":{` +
			`"name":{"type":"string","maxLength":4096},` +
			`"items":{"type":"array","maxItems":1000,"items":{"type":"string","maxLength":256,"description":"member object id"}}}}`,
		example: `{"name":"Reading list","items":["bafyreieqh63jv…","bafyreidfmzjh…"]}`,
	},
	"file": {
		endpoint: "POST /v2/spaces/{spaceId}/files",
		schema: `{"type":"object","additionalProperties":false,"required":["url"],"properties":{` +
			`"url":{"type":"string","maxLength":4096,"description":"source URL; alternatively upload bytes as multipart/form-data with a file field"},` +
			`"name":{"type":"string","maxLength":4096}}}`,
		example: `{"url":"https://example.org/report.pdf"}`,
	},
	"search": {
		endpoint: "POST /v2/spaces/{spaceId}/search (and POST /v2/search global)",
		schema: `{"type":"object","additionalProperties":false,"properties":{` +
			`"query":{"type":"string","maxLength":4096,"description":"full-text query"},` +
			`"type":{"type":"string","maxLength":256,"description":"one type key; multi-type queries use the type pseudo-key in the filter channel"},` +
			`"filter":{"type":"string","maxLength":4096,"description":"compact filter string (grammar on kind filters); the endpoint also accepts a recursive structured filters array, kept out of this schema so it stays strict-mode-decodable (C13) — see kind filters"},` +
			`"sorts":{"type":"array","maxItems":10,"items":{"type":"object","additionalProperties":false,"required":["property"],"properties":{` +
			`"property":{"type":"string","maxLength":256,"description":"any property key"},"direction":{"type":"string","enum":["asc","desc"]},"emptyPlacement":{"type":"string","enum":["start","end"]}}}},` +
			`"fields":{"type":"array","maxItems":25,"items":{"type":"string","maxLength":256},"description":"property keys to include per row"}}}`,
		example: `{"query":"report","type":"task","filter":"done = false AND (dueDate < currentWeek() OR dueDate IS EMPTY)","sorts":[{"property":"dueDate","direction":"asc"}],"fields":["name","dueDate","status"]}`,
	},
	"chat": {
		endpoint: "POST /v2/spaces/{spaceId}/chats",
		schema: `{"type":"object","additionalProperties":false,"required":["name"],"properties":{` +
			`"name":{"type":"string","minLength":1,"maxLength":4096}}}`,
		example: `{"name":"Project chat"}`,
	},
	"chatMessage": {
		endpoint: "POST /v2/spaces/{spaceId}/chats/{chatId}/messages",
		schema: `{"type":"object","additionalProperties":false,"properties":{` +
			`"text":{"type":"string","maxLength":65536,"description":"inline markup SOURCE (SPEC §8): *, [, backtick and <mention objectId=\"…\"> mint real marks; escape literal specials with a backslash; required unless attachments are given"},` +
			`"replyTo":{"type":"string","maxLength":256,"description":"message id being replied to"},` +
			`"attachments":{"type":"array","maxItems":32,"items":{"type":"string","maxLength":256},"description":"object ids; the kind is inferred from each target's layout (image → image, other file layouts → file, anything else → link)"}}}`,
		example: `{"text":"can you **check** the doc?","attachments":["bafyreie6n5l5nkbjal37su54cha4coy"]}`,
	},
	"chatRead": {
		endpoint: "POST /v2/spaces/{spaceId}/chats/{chatId}/read",
		schema: `{"type":"object","additionalProperties":false,"properties":{` +
			`"upTo":{"type":"string","maxLength":256,"description":"INCLUSIVE order id to mark read up to — take it from the newest message of a GET messages read; required for scopes messages/mentions, absent for reactions"},` +
			`"lastStateId":{"type":"string","maxLength":256,"description":"race guard: the state.lastStateId from the same messages read — messages that arrived after that state stay unread"},` +
			`"scope":{"type":"string","enum":["messages","mentions","reactions"],"description":"defaults to messages; reactions marks ALL unread reactions"}}}`,
		example: `{"upTo":"00a1b2c3d4e5f6","lastStateId":"66f2a1b0c9d8e7f6a5b4c3d2","scope":"messages"}`,
	},
	"filters": {
		endpoint: "POST /v2/spaces/{spaceId}/search (filters field) · POST /v2/spaces/{spaceId}/sets (filters field)",
		// documented C13 exception: the structured filter tree is recursive
		// (SPEC §12 filterNode) and therefore not constrained-decodable
		schema: `{"$defs":{"filterNode":{"oneOf":[` +
			`{"type":"object","additionalProperties":false,"required":["operator","filters"],"properties":{` +
			`"operator":{"type":"string","enum":["and","or"]},"filters":{"type":"array","maxItems":50,"items":{"$ref":"#/$defs/filterNode"}}}},` +
			`{"type":"object","additionalProperties":false,"required":["property"],"properties":{` +
			`"property":{"type":"string","maxLength":256},` +
			`"condition":{"type":"string","enum":["equal","notEqual","greater","less","greaterOrEqual","lessOrEqual","contains","notContains","in","notIn","empty","notEmpty","allIn","notAllIn","exactIn","notExactIn","exists"]},` +
			`"value":{"description":"leaf value — select/multiSelect: option NAMES; date: unix SECONDS (RFC 3339 strings belong to the compact filter string, which converts them)"},` +
			`"datePreset":{"type":"string","enum":["yesterday","today","tomorrow","lastWeek","currentWeek","nextWeek","lastMonth","currentMonth","nextMonth","numberOfDaysAgo","numberOfDaysNow","lastYear","currentYear","nextYear"]},` +
			`"includeTime":{"type":"boolean"}}}]}},` +
			`"type":"array","maxItems":50,"items":{"$ref":"#/$defs/filterNode"},` +
			`"description":"RECURSIVE (documented C13 exception): top-level nodes combine with an implicit AND; select values are option names; date values are unix seconds"}`,
		example: `[{"property":"done","condition":"equal","value":false},{"operator":"or","filters":[{"property":"dueDate","condition":"less","datePreset":"currentWeek"},{"property":"dueDate","condition":"empty"}]}]`,
	},
}

// SchemaIndex implements GET /v2/schemas.
func (s *V2Service) SchemaIndex() apimodel.V2SchemaIndex {
	kinds := make([]string, 0, len(v2SchemaKinds))
	for kind := range v2SchemaKinds {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	index := apimodel.V2SchemaIndex{Kinds: make([]apimodel.V2SchemaIndexEntry, 0, len(kinds))}
	for _, kind := range kinds {
		index.Kinds = append(index.Kinds, apimodel.V2SchemaIndexEntry{
			Kind:     kind,
			Endpoint: v2SchemaKinds[kind].endpoint,
			Url:      "/v2/schemas/" + kind,
		})
	}
	for _, op := range v2OpNames {
		index.Ops = append(index.Ops, apimodel.V2SchemaIndexEntry{
			Kind:     op,
			Endpoint: v2OpsEndpoint,
			Url:      "/v2/schemas/ops/" + op,
		})
	}
	return index
}

// SchemaKind implements GET /v2/schemas/{kind}.
func (s *V2Service) SchemaKind(kind string) (apimodel.V2SchemaEntry, error) {
	entry, ok := v2SchemaKinds[kind]
	if !ok {
		kinds := make([]string, 0, len(v2SchemaKinds))
		for k := range v2SchemaKinds {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		return apimodel.V2SchemaEntry{}, apimodel.V2NotFound(
			fmt.Sprintf("unknown schema kind %q — available kinds: %s", kind, strings.Join(kinds, ", ")))
	}
	schema := json.RawMessage(entry.schema)
	if entry.schema == "" {
		// object/type/template bodies ARE AnyBlock documents: serve the
		// format's published schema verbatim
		schema = json.RawMessage(anyblockjson.SchemaJSON())
	}
	result := apimodel.V2SchemaEntry{
		Kind:     kind,
		Endpoint: entry.endpoint,
		Schema:   schema,
		Example:  json.RawMessage(entry.example),
	}
	if kind == "filters" {
		// the compact filter-string grammar rides the filters kind: one
		// concept, one slot (§5) — the parser pins the grammar (SPEC §6.2.1)
		result.Grammar = filterstring.EBNF
		result.GrammarExamples = filterstring.Examples
	}
	return result, nil
}
