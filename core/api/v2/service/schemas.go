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
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
)

// v2SchemaKind is one discoverable kind.
type v2SchemaKind struct {
	endpoint string
	schema   string // JSON Schema (compact); "" = the embedded AnyBlock document schema
	example  string
}

// v2PropertyFormatEnum is the authorable property-format vocabulary, published
// by both the `property` kind and the `type` kind's property_definitions. One
// constant because two spellings of this list drift: core/api/wrapper's
// create_type reads it to keep its own list in step.
//
// Deliberately NARROWER than the server takes: POST /properties also accepts
// `emoji`, `properties` and `map`. They are left unpublished as a vocabulary
// choice, not because the server refuses them, so widening this list is a
// product decision rather than a bug fix.
const v2PropertyFormatEnum = `"text","number","select","multi_select","date","files","checkbox","url","email","phone","objects"`

// v2IconColorProp is the icon palette, shared by all three icon shapes. The
// format layer also accepts a raw palette number for a stored colour the names
// do not cover; an author never writes one, so it is not advertised here.
const v2IconColorProp = `"type":"string","enum":["grey","yellow","orange","red","pink","purple","blue","ice","teal","lime"]`

var v2SchemaKinds = map[string]v2SchemaKind{
	"object": {
		endpoint: "POST /v2/spaces/{space_id}/objects",
		// the full AnyBlock document schema is served verbatim (schema: "")
		//
		// No select value here. `status` carried ["In progress"], and options
		// are per-space objects that no bundled property ships with — so the
		// published example needed ?create_missing_options=true and was
		// refused at default settings. The `property` kind is where the option
		// vocabulary is demonstrated, and there it needs no flag.
		example: `{"formatVersion":"2.0","type":"task","properties":{"name":"Prepare the Q3 report","due_date":"2026-08-01T00:00:00Z"},"blocks":[{"type":"heading_2","text":"Steps"},{"type":"checkbox","text":"Collect the numbers"},{"indent":1,"type":"paragraph","text":"Ask **finance** first"}]}`,
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
	// The flat body, not the AnyBlock document. `type` is the kind an agent
	// reaches for before it has read anything, and serving the interchange
	// document here was a measured failure: that document puts the display name
	// inside `properties`, the settings inside `type_settings` and the field
	// list inside `type_settings.property_definitions`, and callers reliably
	// sent a flat body anyway and got a refusal. The document stays available,
	// under `type_document`, and both bodies reach the same endpoint.
	"type": {
		endpoint: "POST /v2/spaces/{space_id}/types",
		// NO root `required`. One schema serves POST and PATCH, and a
		// grammar-constrained decoder emits every required member — so
		// `required:["name"]` made it invent a name on each PATCH, silently
		// renaming the type. The requirement moves into `name`'s own
		// description, where it informs without compelling.
		schema: `{"type":"object","additionalProperties":false,"description":"the body POST takes. PATCH takes the same members, all optional, except api_key, which is create-only","properties":{` +
			`"name":{"type":"string","minLength":1,"maxLength":4096,"description":"display name, singular. Required on POST; on PATCH send it only to rename"},` +
			`"plural_name":{"type":"string","maxLength":4096},` +
			`"icon":{"type":"object","additionalProperties":false,"description":"give format plus its own member: emoji, name or file","properties":{` +
			`"format":{"type":"string","enum":["emoji","icon","file"]},` +
			`"emoji":{"type":"string","minLength":1,"maxLength":32},` +
			`"name":{"type":"string","minLength":1,"maxLength":256,"description":"a built-in icon name, with format icon"},` +
			`"file":{"type":"string","minLength":1,"maxLength":4096,"description":"id of an image object in this space, with format file"},` +
			`"color":{` + v2IconColorProp + `}},` +
			`"oneOf":[{"required":["format","emoji"],"properties":{"format":{"const":"emoji"}}},` +
			`{"required":["format","name"],"properties":{"format":{"const":"icon"}}},` +
			`{"required":["format","file"],"properties":{"format":{"const":"file"}}}]},` +
			`"layout":{"type":"string","enum":["basic","note","todo","profile","bookmark","set","collection"],"description":"the layout instances of this type get; basic unless you need another"},` +
			`"api_key":{"type":"string","maxLength":256,"pattern":"^[a-zA-Z0-9_]+$","description":"the key object bodies name this type by; derived from the name when omitted"},` +
			`"default_view":{"type":"string","enum":["table","list","gallery","kanban","calendar","graph"],"description":"how a set or collection of this type opens; applies to ones created after the change"},` +
			`"default_template":{"type":"string","maxLength":256,"description":"id of the template new objects of this type start from; empty string clears it"},` +
			`"property_definitions":{"type":"array","maxItems":128,"description":"the type's whole field list; an unknown name mints a property. On PATCH it replaces the list, so to change one field send an ops envelope with add_property to PATCH /v2/spaces/{space_id}/types/{type} instead, and to rename a property send {name} to PATCH /v2/spaces/{space_id}/properties/{key}","items":{` +
			`"type":"object","additionalProperties":false,"description":"names its property by name or by property, one of the two","properties":{` +
			`"name":{"type":"string","minLength":1,"maxLength":128,"description":"the property's display name, e.g. Due date; an unknown one is created"},` +
			`"property":{"type":"string","minLength":1,"maxLength":256,"description":"the property by the key the space serves, for one that exists"},` +
			`"format":{"type":"string","enum":[` + v2PropertyFormatEnum + `],"description":"the new property's format; omit it and an unknown name is created as text"},` +
			`"section":{"type":"string","enum":["featured","hidden"],"description":"featured shows the property on the object itself"},` +
			`"options":{"type":"array","maxItems":100,"description":"select and multi_select only: the option vocabulary; creating options here needs ?create_missing_options=true","items":{` +
			`"type":"object","additionalProperties":false,"required":["name"],"properties":{` +
			`"name":{"type":"string","minLength":1,"maxLength":4096},"color":{"type":"string","maxLength":64}}}}}}}}}`,
		// no `options` here on purpose: declaring them needs
		// ?create_missing_options=true, and an example that only works with a
		// non-default query param is one a caller copies and gets refused.
		// The `property` kind demonstrates options, where they need no flag.
		example: `{"name":"Plant","plural_name":"Plants","icon":{"format":"emoji","emoji":"🌱"},"layout":"basic","property_definitions":[{"name":"Location","format":"select"},{"name":"Watered","format":"date","section":"featured"}]}`,
	},
	"type_document": {
		endpoint: "POST /v2/spaces/{space_id}/types (the interchange document; kind `type` is the body to author one)",
		// the interchange document (kind object_type), served verbatim: what an
		// export writes and an import reads. Prefer `type` to author one.
		//
		// The same type as the `type` example, in the other form, so the two
		// read side by side: `name` moves into `properties`, everything else
		// into `type_settings`. The key is `plant`, not the `task` this example
		// carried before — `task` is a bundled type, so the endpoint refused
		// its own published example with "type key is reserved".
		example: `{"formatVersion":"2.0","kind":"object_type","icon":{"format":"emoji","emoji":"🌱"},"properties":{"name":"Plant"},"type_settings":{"api_key":"plant","layout":"basic","plural_name":"Plants","property_definitions":[{"property":"Location","format":"select"},{"property":"Watered","format":"date","section":"featured"}]}}`,
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
			`"format":{"type":"string","enum":[` + v2PropertyFormatEnum + `]},` +
			`"options":{"type":"array","maxItems":100,"items":{"type":"object","additionalProperties":false,"required":["name"],"properties":{` +
			`"name":{"type":"string","maxLength":4096},"color":{"type":"string","maxLength":64}}}}}}`,
		// `urgency`, not the `priority` this carried before: `priority` is a
		// bundled relation, so the endpoint answered its own published example
		// with "property key already exists" in every space.
		example: `{"key":"urgency","name":"Urgency","format":"select","options":[{"name":"High","color":"red"},{"name":"Low"}]}`,
	},
	"query": {
		endpoint: "POST /v2/spaces/{space_id}/queries",
		schema: `{"type":"object","additionalProperties":false,"required":["name","type"],"properties":{` +
			`"name":{"type":"string","maxLength":4096},` +
			`"type":{"type":"string","maxLength":256,"description":"the queried type's key"},` +
			`"filter":{"type":"string","maxLength":4096,"description":"compact filter string (grammar on kind filters); the endpoint also accepts a recursive structured filters array, kept out of this schema so it stays simple to decode — see kind filters"},` +
			`"sorts":{"type":"array","maxItems":10,"items":{"type":"object","additionalProperties":false,"required":["property"],"properties":{` +
			`"property":{"type":"string","maxLength":256},"direction":{"type":"string","enum":["asc","desc"]},"empty_placement":{"type":"string","enum":["start","end"]}}}},` +
			`"views":{"type":"array","maxItems":10,"description":"the query's views, each whole: the fields the insert_view op's set takes (except filter — write a view's filters as nodes here), plus its columns. Mutually exclusive with top-level filter/sorts, which build one view named All","items":{"type":"object","required":["name"],"properties":{` + v2ViewCreateFieldsDef() + `,` + v2ViewColumnsListDef + `}}}}}`,
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
			`"name":{"type":"string","maxLength":4096,"description":"names the stored object; omit to keep the name the source gives"}}}`,
		example: `{"url":"https://example.org/report.pdf","name":"Q3 report"}`,
	},
	"search": {
		endpoint: "POST /v2/spaces/{space_id}/search (and POST /v2/search global)",
		schema: `{"type":"object","additionalProperties":false,"properties":{` +
			`"query":{"type":"string","maxLength":4096,"description":"full-text query"},` +
			`"type":{"type":"string","maxLength":256,"description":"one type key; multi-type queries use the type pseudo-key in the filter channel; naming a file type (file, image, video, audio) opts file objects into the results — they are excluded otherwise"},` +
			`"filter":{"type":"string","maxLength":4096,"description":"compact filter string (grammar on kind filters); the endpoint also accepts a recursive structured filters array, kept out of this schema so it stays simple to decode — see kind filters"},` +
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
			`"text":{"type":"string","maxLength":8000,"description":"inline markup source: *, [, backtick and <mention object_id=\"…\"> mint real marks; escape literal specials with a backslash; at most 8000 UTF-16 code units (an emoji counts 2+); required unless attachments are given"},` +
			`"reply_to":{"type":"string","maxLength":256,"description":"message id being replied to"},` +
			`"attachments":{"type":"array","maxItems":32,"items":{"type":"string","maxLength":256},"description":"object ids, at most 32 (enforced); the kind is inferred from each target's layout (image → image, other file layouts → file, anything else → link)"}}}`,
		example: `{"text":"can you **check** the doc?","attachments":["bafyreie6n5l5nkbjal37su54cha4coy"]}`,
	},
	"chatMessageEdit": {
		endpoint: "PATCH /v2/spaces/{space_id}/chats/{chat_id}/messages/{message_id}",
		schema: `{"type":"object","additionalProperties":false,"required":["text"],"properties":{` +
			`"text":{"type":"string","maxLength":8000,"description":"replacement inline markup source, at most 8000 UTF-16 code units; every mark is re-derived from this string (escape literal specials); the message's attachments, reply target, style and blocks are preserved"}}}`,
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
		endpoint: "POST /v2/spaces/{space_id}/search (filters field) · POST /v2/spaces/{space_id}/queries (filters field)",
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
			`"description":"Recursive: top-level nodes combine with an implicit AND; select values are option names; date values are unix seconds"}`,
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
	for _, op := range v2AllOpNames() {
		index.Ops = append(index.Ops, v2model.SchemaIndexEntry{
			Kind:     op,
			Endpoint: v2OpSchemas[op].endpoint,
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
		// finite bounds as the hand-built discovery schemas.
		//
		// apiV2DocumentSchema, not the format's full schema and not the
		// authoring subset: see apiv2schema.go for why this surface needs its
		// own, and for the member list that is the whole difference.
		schema = json.RawMessage(apiV2DocumentSchema())
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
	// the bound an open map gets when nothing narrower is declared
	strictSchemaDefaultMaxProperties = 128
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
	if root, ok := schema.(map[string]any); ok {
		hoistAnyValue(root)
	}
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
				// An author who wrote `additionalProperties` already said what
				// a value may be. Only the COUNT is unbounded, so only the
				// count is added: replacing their schema with the any-value
				// would WIDEN a slot they narrowed, and discovery would then
				// advertise values the reader refuses. This is how
				// property_internal_keys came to be published as any-JSON when
				// the format declares it a bounded string.
				switch {
				case closed:
					// nothing open to bound
				case isMap && len(additional) > 0:
					if !hasMaxProperties {
						node["maxProperties"] = strictSchemaDefaultMaxProperties
					}
				default:
					if !hasMaxProperties {
						node["maxProperties"] = strictSchemaDefaultMaxProperties
					}
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

// anyValueDef is where the hoisted any-value schema lives, and the pointer
// every inlined copy is replaced by.
const (
	anyValueDef = "anyValue"
	anyValueRef = "#/$defs/" + anyValueDef
)

// hoistAnyValue replaces every inlined any-value blob with one $ref to a
// single $defs entry. strictDiscoveryNode stamps that blob wherever an open
// map or an untyped array needs a finite value schema, and it is ~2.2KB each
// time — by far the largest repeated structure in the served payload.
//
// The definition must NOT reference itself. The blob is recursive by
// construction (depth 3: an object whose values are depth-2 blobs, and so on),
// so the walk replaces only the OUTERMOST occurrence and never descends into
// one. $defs/anyValue therefore keeps its inner levels literal, and the
// published schema stays acyclic — which is the whole property a constrained
// decoder needs and the reason strictDiscoverySchema exists.
func hoistAnyValue(root map[string]any) {
	blob := boundedDiscoveryValue(3)
	want, err := json.Marshal(blob)
	if err != nil {
		return
	}

	replaced := 0
	var walk func(node any)
	walk = func(node any) {
		switch n := node.(type) {
		case map[string]any:
			for key, child := range n {
				if isAnyValueBlob(child, want) {
					n[key] = map[string]any{"$ref": anyValueRef}
					replaced++
					continue // never descend into a blob: that is what would make the def cyclic
				}
				walk(child)
			}
		case []any:
			for i, child := range n {
				if isAnyValueBlob(child, want) {
					n[i] = map[string]any{"$ref": anyValueRef}
					replaced++
					continue
				}
				walk(child)
			}
		}
	}
	walk(root)
	if replaced == 0 {
		return
	}

	defs, ok := root["$defs"].(map[string]any)
	if !ok {
		defs = map[string]any{}
		root["$defs"] = defs
	}
	if _, taken := defs[anyValueDef]; taken {
		return // a definition already owns the name; leave the schema alone
	}
	defs[anyValueDef] = blob
}

// isAnyValueBlob reports whether a node is exactly the stamped any-value
// schema. Compared by marshaled bytes so a hand-written lookalike that
// differs in any way is left alone.
func isAnyValueBlob(node any, want []byte) bool {
	candidate, ok := node.(map[string]any)
	if !ok {
		return false
	}
	if _, isOneOf := candidate["oneOf"]; !isOneOf {
		return false
	}
	got, err := json.Marshal(candidate)
	return err == nil && bytes.Equal(got, want)
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
			map[string]any{"type": "object", "maxProperties": strictSchemaDefaultMaxProperties, "additionalProperties": boundedDiscoveryValue(depth - 1)},
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
