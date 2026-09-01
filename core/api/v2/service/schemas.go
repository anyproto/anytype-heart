package v2service

// schemas.go implements the §5 discovery surface for the create kinds
// and the search kind: GET /v2/schemas (index) and
// GET /v2/schemas/{kind} (JSON Schema + one worked example, C12). All
// generation-facing schemas are strict-mode-compatible (C13):
// additionalProperties:false, bounded, non-recursive — with the documented
// exception of the AnyBlock document schema's filter tree and the `filters`
// kind itself, which is
// recursive by nature and documented as such; small models are steered to
// the compact filter string, whose grammar (EBNF + examples) is served ON
// the `filters` kind — one concept, one discovery slot (C2), the artifact
// the GBNF conversion consumes.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
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
		endpoint: "POST /v2/spaces/{space_id}/objects",
		// the full AnyBlock document schema is served verbatim (schema: "")
		example: `{"formatVersion":"2.0","type":"task","properties":{"name":"Prepare the Q3 report","status":["In progress"],"due_date":"2026-08-01T00:00:00Z"},"blocks":[{"type":"heading_2","text":"Steps"},{"type":"checkbox","text":"Collect the numbers"},{"indent":1,"type":"paragraph","text":"Ask **finance** first"}]}`,
	},
	"shortcut": {
		endpoint: "POST /v2/spaces/{space_id}/objects",
		schema: `{"type":"object","additionalProperties":false,"required":["type"],"properties":{` +
			`"type":{"type":"string","maxLength":256,"description":"type key, e.g. page or task"},` +
			`"name":{"type":"string","maxLength":4096},` +
			`"properties":{"type":"object","maxProperties":128,"additionalProperties":{"type":["string","number","boolean","array","null"]}},` +
			`"markdown":{"type":"string","maxLength":1048576,"description":"markdown body parsed into blocks server-side — part of the same single create (dry runs validate it too); at most 2048 parsed blocks"}}}`,
		example: `{"type":"task","name":"Buy milk","properties":{"due_date":"2026-08-01T00:00:00Z"},"markdown":"- [ ] oat\n- [ ] whole"}`,
	},
	"type": {
		endpoint: "POST /v2/spaces/{space_id}/types",
		// a type document is an AnyBlock document (kind object_type)
		example: `{"formatVersion":"2.0","kind":"object_type","icon":{"format":"emoji","emoji":"✅"},"properties":{"name":"Task"},"type_settings":{"api_key":"task","layout":"todo","plural_name":"Tasks","property_definitions":[{"property":"due_date","name":"Due date","format":"date","section":"featured"},{"property":"status","name":"Status","format":"select"}]}}`,
	},
	"template": {
		endpoint: "POST /v2/spaces/{space_id}/templates",
		example:  `{"formatVersion":"2.0","kind":"template","type":"template","template_for":"task","properties":{"name":"Weekly task"},"blocks":[{"type":"heading_2","text":"Checklist"},{"type":"checkbox","text":"First step"}]}`,
	},
	"property": {
		endpoint: "POST /v2/spaces/{space_id}/properties",
		schema: `{"type":"object","additionalProperties":false,"required":["name","format"],"properties":{` +
			`"key":{"type":"string","maxLength":256,"pattern":"^[a-zA-Z0-9_]+$"},` +
			`"name":{"type":"string","maxLength":4096},` +
			`"format":{"type":"string","enum":["text","number","select","multi_select","date","files","checkbox","url","email","phone","objects"]},` +
			`"options":{"type":"array","maxItems":100,"items":{"type":"object","additionalProperties":false,"required":["name"],"properties":{` +
			`"name":{"type":"string","maxLength":4096},"color":{"type":"string","maxLength":64}}}}}}`,
		example: `{"key":"priority","name":"Priority","format":"select","options":[{"name":"High","color":"red"},{"name":"Low"}]}`,
	},
	"set": {
		endpoint: "POST /v2/spaces/{space_id}/sets",
		schema: `{"type":"object","additionalProperties":false,"required":["name","type"],"properties":{` +
			`"name":{"type":"string","maxLength":4096},` +
			`"type":{"type":"string","maxLength":256,"description":"the queried type's key"},` +
			`"filter":{"type":"string","maxLength":4096,"description":"compact filter string (grammar on kind filters); the endpoint also accepts a recursive structured filters array, kept out of this schema so it stays strict-mode-decodable (C13) — see kind filters"},` +
			`"sorts":{"type":"array","maxItems":10,"items":{"type":"object","additionalProperties":false,"required":["property"],"properties":{` +
			`"property":{"type":"string","maxLength":256},"direction":{"type":"string","enum":["asc","desc"]},"empty_placement":{"type":"string","enum":["start","end"]}}}},` +
			`"views":{"type":"array","maxItems":10,"description":"full SPEC §6.2 view objects; mutually exclusive with top-level filter/sorts"}}}`,
		example: `{"name":"Open tasks","type":"task","filter":"done = false","sorts":[{"property":"due_date","direction":"asc"}]}`,
	},
	"collection": {
		endpoint: "POST /v2/spaces/{space_id}/collections",
		schema: `{"type":"object","additionalProperties":false,"required":["name"],"properties":{` +
			`"name":{"type":"string","maxLength":4096},` +
			`"items":{"type":"array","maxItems":1000,"items":{"type":"string","maxLength":256,"description":"member object id"}}}}`,
		example: `{"name":"Reading list","items":["bafyreieqh63jv…","bafyreidfmzjh…"]}`,
	},
	"file": {
		endpoint: "POST /v2/spaces/{space_id}/files",
		schema: `{"type":"object","additionalProperties":false,"required":["url"],"properties":{` +
			`"url":{"type":"string","maxLength":4096,"description":"source URL; alternatively upload bytes as multipart/form-data with a file field"},` +
			`"name":{"type":"string","maxLength":4096}}}`,
		example: `{"url":"https://example.org/report.pdf"}`,
	},
	"search": {
		endpoint: "POST /v2/spaces/{space_id}/search (and POST /v2/search global)",
		schema: `{"type":"object","additionalProperties":false,"properties":{` +
			`"query":{"type":"string","maxLength":4096,"description":"full-text query"},` +
			`"type":{"type":"string","maxLength":256,"description":"one type key; multi-type queries use the type pseudo-key in the filter channel; naming a file type (file, image, video, audio) opts file objects into the results — they are excluded otherwise"},` +
			`"filter":{"type":"string","maxLength":4096,"description":"compact filter string (grammar on kind filters); the endpoint also accepts a recursive structured filters array, kept out of this schema so it stays strict-mode-decodable (C13) — see kind filters"},` +
			`"sorts":{"type":"array","maxItems":10,"items":{"type":"object","additionalProperties":false,"required":["property"],"properties":{` +
			`"property":{"type":"string","maxLength":256,"description":"any property key"},"direction":{"type":"string","enum":["asc","desc"]},"empty_placement":{"type":"string","enum":["start","end"]}}}},` +
			`"fields":{"type":"array","maxItems":25,"items":{"type":"string","maxLength":256},"description":"property keys to include per row; file rows additionally take mimeType and size — also valid filter and sort keys (they translate to the store's fileMimeType/sizeInBytes); file rows enter scope only when the type channel names a file type"}}}`,
		example: `{"query":"report","type":"task","filter":"done = false AND (due_date < currentWeek() OR due_date IS EMPTY)","sorts":[{"property":"due_date","direction":"asc"}],"fields":["name","due_date","status"]}`,
	},
	"space": {
		endpoint: "POST /v2/spaces (PATCH /v2/spaces/{space_id} takes the same fields, both optional — at least one)",
		schema: `{"type":"object","additionalProperties":false,"required":["name"],"properties":{` +
			`"name":{"type":"string","minLength":1,"maxLength":4096},` +
			`"description":{"type":"string","maxLength":4096}}}`,
		example: `{"name":"Research","description":"Scratch space for the Q3 analysis"}`,
	},
	"chat": {
		endpoint: "POST /v2/spaces/{space_id}/chats",
		schema: `{"type":"object","additionalProperties":false,"required":["name"],"properties":{` +
			`"name":{"type":"string","minLength":1,"maxLength":4096}}}`,
		example: `{"name":"Project chat"}`,
	},
	"chatMessage": {
		endpoint: "POST /v2/spaces/{space_id}/chats/{chat_id}/messages",
		// text maxLength mirrors chatmodel.MaxMessageLength (8000 UTF-16
		// units, the STORE's cap) — advertising more turns schema-obedient
		// callers into rejected requests; a drift test pins the two together
		schema: `{"type":"object","additionalProperties":false,"properties":{` +
			`"text":{"type":"string","maxLength":8000,"description":"inline markup SOURCE (SPEC §8): *, [, backtick and <mention object_id=\"…\"> mint real marks; escape literal specials with a backslash; at most 8000 UTF-16 code units (an emoji counts 2+); required unless attachments are given"},` +
			`"reply_to":{"type":"string","maxLength":256,"description":"message id being replied to"},` +
			`"attachments":{"type":"array","maxItems":32,"items":{"type":"string","maxLength":256},"description":"object ids, at most 32 (enforced); the kind is inferred from each target's layout (image → image, other file layouts → file, anything else → link)"}}}`,
		example: `{"text":"can you **check** the doc?","attachments":["bafyreie6n5l5nkbjal37su54cha4coy"]}`,
	},
	"chatMessageEdit": {
		endpoint: "PATCH /v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id}",
		schema: `{"type":"object","additionalProperties":false,"required":["text"],"properties":{` +
			`"text":{"type":"string","maxLength":8000,"description":"replacement inline markup SOURCE (SPEC §8), at most 8000 UTF-16 code units; ALL marks are re-derived from this string (the D′1 caveat bites hardest here — escape literal specials); the message's attachments, reply target, style and blocks are preserved"}}}`,
		example: `{"text":"updated: can you **check** the doc?"}`,
	},
	"chatReaction": {
		endpoint: "POST /v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id}/reactions",
		schema: `{"type":"object","additionalProperties":false,"required":["emoji"],"properties":{` +
			`"emoji":{"type":"string","minLength":1,"maxLength":64,"description":"the reaction emoji to toggle, e.g. 👍 — the response's added reports whether it was added or removed"}}}`,
		example: `{"emoji":"👍"}`,
	},
	"chatRead": {
		endpoint: "POST /v2/spaces/{space_id}/chats/{chat_id}/read",
		schema: `{"type":"object","additionalProperties":false,"properties":{` +
			`"up_to":{"type":"string","maxLength":256,"description":"INCLUSIVE order id to mark read up to — take it from the newest message of a GET messages read; REQUIRED for scopes messages/mentions, absent for reactions"},` +
			`"last_state_id":{"type":"string","maxLength":256,"description":"race guard, REQUIRED for scopes messages/mentions (absent for reactions): the state.last_state_id from the same messages read — messages that arrived after that state stay unread; an empty guard would silently mark nothing"},` +
			`"scope":{"type":"string","enum":["messages","mentions","reactions"],"description":"defaults to messages; reactions marks ALL unread reactions"}}}`,
		example: `{"up_to":"00a1b2c3d4e5f6","last_state_id":"66f2a1b0c9d8e7f6a5b4c3d2","scope":"messages"}`,
	},
	"filters": {
		endpoint: "POST /v2/spaces/{space_id}/search (filters field) · POST /v2/spaces/{space_id}/sets (filters field)",
		// documented C13 exception: the structured filter tree is recursive
		// (SPEC §12 filterNode) and therefore not constrained-decodable
		schema: `{"$defs":{"filterNode":{"oneOf":[` +
			`{"type":"object","additionalProperties":false,"required":["operator","filters"],"properties":{` +
			`"operator":{"type":"string","enum":["and","or"]},"filters":{"type":"array","minItems":1,"maxItems":50,"items":{"$ref":"#/$defs/filterNode"}}}},` +
			`{"type":"object","additionalProperties":false,"required":["property","condition"],"properties":{` +
			`"property":{"type":"string","maxLength":256},` +
			`"condition":{"type":"string","enum":["equal","not_equal","greater","less","greater_or_equal","less_or_equal","contains","not_contains","in","not_in","empty","not_empty","all_in","not_all_in","exact_in","not_exact_in","exists"]},` +
			`"value":{"description":"leaf value — select/multi_select: option NAMES; date: unix SECONDS (RFC 3339 strings belong to the compact filter string, which converts them)"},` +
			`"date_preset":{"type":"string","enum":["yesterday","today","tomorrow","last_week","current_week","next_week","last_month","current_month","next_month","number_of_days_ago","number_of_days_now","last_year","current_year","next_year"]},` +
			`"include_time":{"type":"boolean"}}}]}},` +
			`"type":"array","maxItems":50,"items":{"$ref":"#/$defs/filterNode"},` +
			`"description":"RECURSIVE (documented C13 exception): top-level nodes combine with an implicit AND; select values are option names; date values are unix seconds"}`,
		example: `[{"property":"done","condition":"equal","value":false},{"operator":"or","filters":[{"property":"due_date","condition":"less","date_preset":"current_week"},{"property":"due_date","condition":"empty"}]}]`,
	},
}

// SchemaIndex implements GET /v2/schemas.
func (s *Service) SchemaIndex() v2model.SchemaIndex {
	kinds := make([]string, 0, len(v2SchemaKinds))
	for kind := range v2SchemaKinds {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	index := v2model.SchemaIndex{Kinds: make([]v2model.SchemaIndexEntry, 0, len(kinds))}
	for _, kind := range kinds {
		index.Kinds = append(index.Kinds, v2model.SchemaIndexEntry{
			Kind:     kind,
			Endpoint: v2SchemaKinds[kind].endpoint,
			Url:      "/v2/schemas/" + kind,
		})
	}
	for _, op := range v2OpNames {
		index.Ops = append(index.Ops, v2model.SchemaIndexEntry{
			Kind:     op,
			Endpoint: v2OpsEndpoint,
			Url:      "/v2/schemas/ops/" + op,
		})
	}
	return index
}

// SchemaKind implements GET /v2/schemas/{kind}.
func (s *Service) SchemaKind(kind string) (v2model.SchemaEntry, error) {
	entry, ok := v2SchemaKinds[kind]
	if !ok {
		kinds := make([]string, 0, len(v2SchemaKinds))
		for k := range v2SchemaKinds {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		return v2model.SchemaEntry{}, v2model.NotFound(
			fmt.Sprintf("unknown schema kind %q — available kinds: %s", kind, strings.Join(kinds, ", ")))
	}
	schema := json.RawMessage(entry.schema)
	if entry.schema == "" {
		// object/type/template bodies ARE AnyBlock documents: serve the
		// format's complete published schema, normalized with the same C13
		// finite bounds as the hand-built discovery schemas
		schema = json.RawMessage(anyblockjson.SchemaJSON())
	}
	schema, err := strictDiscoverySchema(schema)
	if err != nil {
		return v2model.SchemaEntry{}, fmt.Errorf("normalize schema %q: %w", kind, err)
	}
	result := v2model.SchemaEntry{
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

const (
	strictSchemaDefaultMaxLength = 1 << 20
	strictSchemaDefaultMaxItems  = 2048
)

// strictDiscoverySchema closes and finitely bounds every typed schema node
// before it reaches generation-facing discovery. The format schema is also a
// validation artifact and intentionally keeps some authorable fields broadly
// typed; discovery has the stronger C13 promise needed by constrained
// decoders. unevaluatedProperties is used for composed object nodes because
// additionalProperties:false inside one allOf branch would reject properties
// declared by its siblings.
func strictDiscoverySchema(raw json.RawMessage) (json.RawMessage, error) {
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	if root, ok := schema.(map[string]any); ok {
		flattenDiscoveryObjectBases(root)
	}
	strictDiscoveryNode(schema)
	return json.Marshal(schema)
}

// flattenDiscoveryObjectBases inlines the deliberately-open object bases
// used through allOf by the format schema. Closing propertyDefinition or
// iconVariants in place would otherwise make their derived shapes reject the
// extra fields they add (for example typeProperty.section). Once inherited
// properties/requirements/conditions are in the derived node, both the base
// and the derived shape can be closed independently without changing meaning.
func flattenDiscoveryObjectBases(root map[string]any) {
	defs, _ := root["$defs"].(map[string]any)
	if len(defs) == 0 {
		return
	}
	var walk func(any)
	walk = func(value any) {
		switch node := value.(type) {
		case map[string]any:
			if allOf, ok := node["allOf"].([]any); ok {
				kept := make([]any, 0, len(allOf))
				inheritedConditions := make([]any, 0)
				for _, clause := range allOf {
					clauseMap, _ := clause.(map[string]any)
					ref, _ := clauseMap["$ref"].(string)
					const prefix = "#/$defs/"
					base, _ := defs[strings.TrimPrefix(ref, prefix)].(map[string]any)
					_, baseProps := base["properties"].(map[string]any)
					if !strings.HasPrefix(ref, prefix) || !schemaNodeSupports(base, "object") || !baseProps || base["additionalProperties"] != nil || base["unevaluatedProperties"] != nil {
						kept = append(kept, clause)
						continue
					}
					mergeDiscoveryObjectBase(node, base)
					if conditions, ok := base["allOf"].([]any); ok {
						inheritedConditions = append(inheritedConditions, conditions...)
					}
				}
				kept = append(inheritedConditions, kept...)
				if len(kept) == 0 {
					delete(node, "allOf")
				} else {
					node["allOf"] = kept
				}
			}
			for _, child := range node {
				walk(child)
			}
		case []any:
			for _, child := range node {
				walk(child)
			}
		}
	}
	walk(root)
}

func mergeDiscoveryObjectBase(dst, base map[string]any) {
	if _, ok := dst["type"]; !ok {
		dst["type"] = "object"
	}
	merged := map[string]any{}
	if properties, ok := base["properties"].(map[string]any); ok {
		for key, value := range properties {
			merged[key] = value
		}
	}
	if properties, ok := dst["properties"].(map[string]any); ok {
		for key, value := range properties {
			if inherited, overlaps := merged[key]; overlaps {
				// A derived property narrows its inherited definition; it does
				// not replace it. typeProperty.object_types, for example, rules
				// out null while propertyDefinition still supplies string items.
				// Keep both constraints when flattening the open base, or the
				// discovery schema becomes weaker than the AnyBlock schema.
				merged[key] = map[string]any{"allOf": []any{inherited, value}}
			} else {
				merged[key] = value
			}
		}
	}
	dst["properties"] = merged

	required := map[string]bool{}
	var names []any
	for _, source := range []any{base["required"], dst["required"]} {
		for _, raw := range anySlice(source) {
			name, _ := raw.(string)
			if name != "" && !required[name] {
				required[name] = true
				names = append(names, name)
			}
		}
	}
	if len(names) > 0 {
		dst["required"] = names
	}
}

func anySlice(value any) []any {
	items, _ := value.([]any)
	return items
}

func strictDiscoveryNode(value any) {
	switch node := value.(type) {
	case map[string]any:
		if schemaNodeSupports(node, "object") {
			if _, fixedShape := node["properties"].(map[string]any); fixedShape {
				additional, hasAdditional := node["additionalProperties"]
				_, boundedMap := additional.(map[string]any)
				_, hasMaxProperties := positiveJSONNumber(node["maxProperties"])
				if (!hasAdditional || additional == true) && !(boundedMap && hasMaxProperties) {
					node["unevaluatedProperties"] = false
				}
			} else {
				additional, isMap := node["additionalProperties"].(map[string]any)
				_, hasMaxProperties := positiveJSONNumber(node["maxProperties"])
				closed := node["additionalProperties"] == false || node["unevaluatedProperties"] == false
				if !closed && !(isMap && len(additional) > 0 && hasMaxProperties) {
					node["maxProperties"] = 128
					node["additionalProperties"] = boundedDiscoveryValue(3)
				}
			}
		}
		if schemaNodeSupports(node, "string") && !schemaStringIsBounded(node) {
			node["maxLength"] = strictSchemaDefaultMaxLength
		}
		if schemaNodeSupports(node, "array") {
			if _, ok := positiveJSONNumber(node["maxItems"]); !ok {
				node["maxItems"] = strictSchemaDefaultMaxItems
			}
			if _, ok := node["items"]; !ok {
				node["items"] = boundedDiscoveryValue(3)
			}
		}
		for _, child := range node {
			strictDiscoveryNode(child)
		}
	case []any:
		for _, child := range node {
			strictDiscoveryNode(child)
		}
	}
}

func boundedDiscoveryValue(depth int) map[string]any {
	variants := []any{
		map[string]any{"type": "string", "maxLength": strictSchemaDefaultMaxLength},
		map[string]any{"type": "number"},
		map[string]any{"type": "boolean"},
		map[string]any{"type": "null"},
	}
	if depth > 0 {
		variants = append(variants,
			map[string]any{"type": "array", "maxItems": strictSchemaDefaultMaxItems, "items": boundedDiscoveryValue(depth - 1)},
			map[string]any{"type": "object", "maxProperties": 128, "additionalProperties": boundedDiscoveryValue(depth - 1)},
		)
	}
	return map[string]any{"oneOf": variants}
}

func schemaNodeSupports(node map[string]any, wanted string) bool {
	switch value := node["type"].(type) {
	case string:
		return value == wanted
	case []any:
		for _, item := range value {
			if item == wanted {
				return true
			}
		}
	}
	return false
}

func positiveJSONNumber(value any) (float64, bool) {
	var n float64
	switch value := value.(type) {
	case float64:
		n = value
	case float32:
		n = float64(value)
	case int:
		n = float64(value)
	case int32:
		n = float64(value)
	case int64:
		n = float64(value)
	case uint:
		n = float64(value)
	case uint32:
		n = float64(value)
	case uint64:
		n = float64(value)
	default:
		return 0, false
	}
	return n, n > 0
}

func schemaStringIsBounded(node map[string]any) bool {
	if _, ok := positiveJSONNumber(node["maxLength"]); ok {
		return true
	}
	if enum, ok := node["enum"].([]any); ok && len(enum) > 0 {
		return true
	}
	if _, ok := node["const"].(string); ok {
		return true
	}
	return false
}
