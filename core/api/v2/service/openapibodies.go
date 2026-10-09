package v2service

// openapibodies.go composes the request-body schemas the v2 OpenAPI document
// publishes for the operations swag cannot describe: the bodies whose shape
// lives in the served discovery schemas (schemas.go) rather than in a Go
// struct. swag emits a bare `{"type":"object"}` for each of them, and every
// reader of the document (the external MCP bridge derives its tool arguments
// from it) saw an opaque body: the four-round eval measured a 6-in-10 blind
// invalid write on create_property against 0 with the body typed.
//
// `make openapi` runs cmd/openapibodies, which prints what this file
// composes, and scripts/fix_openapi_v2.py splices it into the generated
// document. core/api/openapibodies_test.go pins the embedded document to this
// composition, so an edit to a served schema that is not followed by
// `make openapi` fails there.
//
// Two shapes, by size. A kind that is small is spliced whole, so a tool
// declaration carries it. A body that is an AnyBlock document stays an open
// object with a description that names the discovery operation to read: the
// document schema is 44 KB, and four copies of it in a tool listing is the
// wrong trade; the eval's pointer description sent 9 of 10 callers to the
// schema before writing. An ops envelope publishes its op vocabulary and
// points at get_op_schema for each op's members, for the same reason.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// OpenAPIBodies is the composition: one request-body schema per operationId,
// plus the component schemas the bodies reference (a served schema's `$defs`,
// hoisted, because a reader that flattens a body into arguments drops its
// root and with it any `#/$defs/…` target).
type OpenAPIBodies struct {
	Bodies     map[string]json.RawMessage
	Components map[string]json.RawMessage
}

// openAPIBodyRecipes names, per operationId, how its body is composed. Every
// operation whose swag annotation declares `body object` must have a row:
// core/api/openapibodies_test.go refuses a bare object body in the document.
var openAPIBodyRecipes = map[string]func(c *openAPIBodyComposer) (json.RawMessage, error){
	v2model.OpPublishChatStatus: func(c *openAPIBodyComposer) (json.RawMessage, error) { return c.kind("chatStatus") },
	v2model.OpCreateProperty:    func(c *openAPIBodyComposer) (json.RawMessage, error) { return c.kind("property") },
	v2model.OpUpdateProperty:    func(c *openAPIBodyComposer) (json.RawMessage, error) { return c.literal(openAPIUpdatePropertyBody) },
	v2model.OpCreateCollection:  func(c *openAPIBodyComposer) (json.RawMessage, error) { return c.kind("collection") },
	v2model.OpCreateQuery: func(c *openAPIBodyComposer) (json.RawMessage, error) {
		// the served kind leaves the recursive structured filter tree out so
		// it stays simple to decode; the document is the contract and
		// CreateQueryRequest takes `filters`, so the body declares it
		filters, err := c.kind("filters")
		if err != nil {
			return nil, fmt.Errorf("compose filters member: %w", err)
		}
		return c.kindWith("query", func(root map[string]any) error {
			var node map[string]any
			if err := json.Unmarshal(filters, &node); err != nil {
				return fmt.Errorf("decode filters kind: %w", err)
			}
			node["description"] = "structured filter nodes (schema kind filters), an alternative to the compact filter string; a body sends at most one of the two"
			props := root["properties"].(map[string]any)
			props["filters"] = node
			props["filter"].(map[string]any)["description"] = "compact filter string (grammar on kind filters), an alternative to the structured filters array; a body sends at most one of the two"
			// CreateQuery refuses both filter channels at once, and views
			// beside any top-level filter or sort. An empty filter string
			// is not a filter; an array, even empty, is
			nonEmptyFilter := map[string]any{"filter": map[string]any{"minLength": 1}}
			root["allOf"] = []any{
				map[string]any{"not": map[string]any{"required": []string{"filter", "filters"}, "properties": nonEmptyFilter}},
				map[string]any{"not": map[string]any{"required": []string{"views", "filter"}, "properties": nonEmptyFilter}},
				map[string]any{"not": map[string]any{"required": []string{"views", "filters"}}},
				map[string]any{"not": map[string]any{"required": []string{"views", "sorts"}}},
			}
			return nil
		})
	},
	v2model.OpSearchSpace:       func(c *openAPIBodyComposer) (json.RawMessage, error) { return c.searchBody() },
	v2model.OpSearchGlobal:      func(c *openAPIBodyComposer) (json.RawMessage, error) { return c.searchBody() },
	v2model.OpStreamSpaceSearch: func(c *openAPIBodyComposer) (json.RawMessage, error) { return c.searchBody() },
	v2model.OpCreateType: func(c *openAPIBodyComposer) (json.RawMessage, error) {
		return c.anyOf("the flat type body, or the interchange document with kind object_type",
			func() (json.RawMessage, error) {
				return c.kindWith("type", func(root map[string]any) error {
					root["required"] = []string{"name"}
					return nil
				})
			},
			func() (json.RawMessage, error) {
				return c.pointer("type_document", "the type as an AnyBlock document: formatVersion 2.0, kind object_type, the definition under type_settings")
			})
	},
	v2model.OpUpdateType: func(c *openAPIBodyComposer) (json.RawMessage, error) {
		return c.anyOf("the flat type body with the fields to change, a partial type document, or an ops envelope that edits the property list and the views one op at a time",
			func() (json.RawMessage, error) {
				// the same flat body as create, minus api_key: the slug is
				// identity and the patch refuses it
				return c.kindWith("type", func(root map[string]any) error {
					delete(root["properties"].(map[string]any), "api_key")
					root["description"] = "the flat body, every member optional; at least one"
					return nil
				})
			},
			func() (json.RawMessage, error) { return c.typePatchDocument() },
			func() (json.RawMessage, error) {
				return c.envelope(v2TypeOpNames, "planned and refused whole before any write, then written in order: property ops first, view ops after, and a view op refused leaves the written property lists in place")
			})
	},
	v2model.OpCreateObject: func(c *openAPIBodyComposer) (json.RawMessage, error) {
		return c.anyOf("the shortcut body, or a full AnyBlock document; formatVersion or blocks picks the document form",
			func() (json.RawMessage, error) { return c.kind("shortcut") },
			func() (json.RawMessage, error) {
				return c.pointer("object", "a full AnyBlock document: formatVersion 2.0, properties, and blocks in preorder. It takes the shortcut's template member too")
			})
	},
	v2model.OpCreateTemplate: func(c *openAPIBodyComposer) (json.RawMessage, error) {
		return c.pointer("template", "the template as an AnyBlock document: formatVersion 2.0, kind template, template_for naming the type")
	},
	v2model.OpValidate: func(c *openAPIBodyComposer) (json.RawMessage, error) {
		return c.pointer(apiV2ValidateKind, "an AnyBlock document of any kind, checked without being stored")
	},
	v2model.OpCreateWidget: func(c *openAPIBodyComposer) (json.RawMessage, error) {
		return c.kindWith("widget", func(root map[string]any) error {
			root["required"] = []string{"target", "scope"}
			return nil
		})
	},
	v2model.OpUpdateWidget: func(c *openAPIBodyComposer) (json.RawMessage, error) {
		// the same body minus the two identity members: the target and the
		// scope are what the widget IS, and the patch refuses them
		return c.kindWith("widget", func(root map[string]any) error {
			props := root["properties"].(map[string]any)
			delete(props, "target")
			delete(props, "scope")
			root["description"] = "the members to change, every one optional; at least one. The target and the scope are the widget's identity and are refused here"
			root["minProperties"] = 1
			// the create defaults do not apply to a patch: an omitted member
			// keeps what is stored, and an omitted placement keeps the position
			props["limit"].(map[string]any)["description"] = "how many entries a listing shows, from the app's own pick-list: 6, 10, 14, 30 or 50, or 4, 6, 8, 30, 50 for the list layout; any other value is stored as the smallest, with a warning. Omitted, the stored limit stays, re-fitted to the list of a new layout when that list does not hold it. With the link layout the stored limit stays untouched and a sent one is not stored"
			props["view_id"].(map[string]any)["description"] = "which of the target's views a view widget shows, by view id; an empty string returns to the target's first view. Only a query, collection or type target has views, and only a space widget keeps one"
			props["after"].(map[string]any)["description"] = "id or target of the sidebar widget to move this one after. At most one of after, before and position; omitted, the widget keeps its position"
			props["before"].(map[string]any)["description"] = "id or target of the sidebar widget to move this one before"
			props["position"].(map[string]any)["description"] = "first or last in the sidebar, last meaning before the bin widget when that is last"
			props["layout"].(map[string]any)["description"] = "link, tree, list, compact_list or view. The layout and the limit are re-validated as a pair whenever either is sent: a layout the target cannot render is replaced with the one it can, with a warning, and a stored layout the target cannot render is replaced the same way on a limit change"
			return nil
		})
	},
	v2model.OpPatchObject: func(c *openAPIBodyComposer) (json.RawMessage, error) {
		return c.envelope(v2OpNames, "applied in order as one edit, and if any one is refused none is applied")
	},
}

// openAPIUpdatePropertyBody is PATCH properties/{key}: the one member
// UpdatePropertyRequest takes. The key is identity and does not change.
const openAPIUpdatePropertyBody = `{"type":"object","additionalProperties":false,"required":["name"],"properties":{` +
	`"name":{"type":"string","maxLength":4096,"description":"the new display name; the key is identity and does not change"}}}`

// ComposeOpenAPIBodies builds every request body in the recipe table.
func ComposeOpenAPIBodies() (*OpenAPIBodies, error) {
	c := &openAPIBodyComposer{components: map[string]json.RawMessage{}}
	out := &OpenAPIBodies{Bodies: make(map[string]json.RawMessage, len(openAPIBodyRecipes))}
	ops := make([]string, 0, len(openAPIBodyRecipes))
	for op := range openAPIBodyRecipes {
		ops = append(ops, op)
	}
	sort.Strings(ops) // deterministic component collision reports
	for _, op := range ops {
		body, err := openAPIBodyRecipes[op](c)
		if err != nil {
			return nil, fmt.Errorf("compose %s body: %w", op, err)
		}
		out.Bodies[op] = body
	}
	out.Components = c.components
	return out, nil
}

// openAPIBodyComposer accumulates the hoisted components across bodies.
type openAPIBodyComposer struct {
	components map[string]json.RawMessage
}

// kind is a served discovery schema, spliced whole, its `$defs` hoisted into
// components and every `#/$defs/x` reference re-aimed at them.
func (c *openAPIBodyComposer) kind(kind string) (json.RawMessage, error) {
	return c.kindWith(kind, nil)
}

// kindWith is kind with one edit to the decoded root before the hoist: the
// document publishes a served schema where an operation's body differs from
// the kind by a member or a requirement.
func (c *openAPIBodyComposer) kindWith(kind string, edit func(root map[string]any) error) (json.RawMessage, error) {
	entry, err := schemaKind(kind)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(entry.Schema, &root); err != nil {
		return nil, fmt.Errorf("decode served schema %q: %w", kind, err)
	}
	if edit != nil {
		if err := edit(root); err != nil {
			return nil, fmt.Errorf("edit served schema %q: %w", kind, err)
		}
	}
	defs, _ := root["$defs"].(map[string]any)
	delete(root, "$defs")
	names := make(map[string]string, len(defs))
	for def := range defs {
		names[def] = openAPIComponentName(def)
	}
	rewriteDefRefs(root, names)
	for def, schema := range defs {
		rewriteDefRefs(schema, names)
		raw, err := json.Marshal(schema)
		if err != nil {
			return nil, fmt.Errorf("encode def %q of %q: %w", def, kind, err)
		}
		name := names[def]
		if have, ok := c.components[name]; ok && string(have) != string(raw) {
			return nil, fmt.Errorf("component %s: kind %q defines it differently from an earlier body", name, kind)
		}
		c.components[name] = raw
	}
	return json.Marshal(root)
}

// typePatchDocument is the partial type document PATCH types/{type} takes
// (v2TypePatch): the type's own details under properties, the patchable
// settings under type_settings, and the envelope icon. The member schemas
// are the flat kind's, so the two cannot drift.
func (c *openAPIBodyComposer) typePatchDocument() (json.RawMessage, error) {
	flat, err := c.kind("type")
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(flat, &root); err != nil {
		return nil, fmt.Errorf("decode type kind: %w", err)
	}
	members := root["properties"].(map[string]any)
	settings := map[string]any{}
	for name := range typeSettingsPatchKeys {
		settings[name] = members[name]
	}
	settings["property_definitions"] = members["property_definitions"]
	// the type's own details the patch writes: updatableTypeDetailKeys,
	// each a string, null to clear
	details := map[string]any{}
	for key := range updatableTypeDetailKeys {
		details[key] = map[string]any{"type": []string{"string", "null"}, "maxLength": 4096}
	}
	out, err := json.Marshal(map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"description":          "a partial type document: properties carries the type's own details, type_settings the settings to change",
		"properties": map[string]any{
			"properties": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"description":          "the type's own details, name and description only",
				"properties":           details,
			},
			"type_settings": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           settings,
			},
			"icon": members["icon"],
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode type patch document: %w", err)
	}
	return out, nil
}

// searchBody is the search request (v2model.SearchRequest): the query's
// type, filter channels and sorts, the very members create_query takes, so a
// caller writes a search and a saved query alike, plus the full-text query
// and the row fields.
func (c *openAPIBodyComposer) searchBody() (json.RawMessage, error) {
	filters, err := c.kind("filters")
	if err != nil {
		return nil, fmt.Errorf("compose filters member: %w", err)
	}
	query, err := c.kind("query")
	if err != nil {
		return nil, fmt.Errorf("compose query members: %w", err)
	}
	var filtersNode, queryRoot map[string]any
	if err := json.Unmarshal(filters, &filtersNode); err != nil {
		return nil, fmt.Errorf("decode filters kind: %w", err)
	}
	if err := json.Unmarshal(query, &queryRoot); err != nil {
		return nil, fmt.Errorf("decode query kind: %w", err)
	}
	members := queryRoot["properties"].(map[string]any)
	filtersNode["description"] = "structured filter nodes (schema kind filters), an alternative to the compact filter string; a body sends at most one of the two"
	filter := members["filter"].(map[string]any)
	filter["description"] = "compact filter string (grammar on kind filters), an alternative to the structured filters array; a body sends at most one of the two"
	typ := members["type"].(map[string]any)
	typ["description"] = "a type key: only objects of this type"
	return json.Marshal(map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"query":   map[string]any{"type": "string", "description": "full-text query over object names and indexed content"},
			"type":    typ,
			"filter":  filter,
			"filters": filtersNode,
			"sorts":   members["sorts"],
			"fields": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": `property keys to include per row, as an array such as ["status","due_date"]; the list reads take the same array`,
			},
		},
		"not": map[string]any{"required": []string{"filter", "filters"}, "properties": map[string]any{"filter": map[string]any{"minLength": 1}}},
	})
}

// literal is a hand-written body, validated as JSON.
func (c *openAPIBodyComposer) literal(schema string) (json.RawMessage, error) {
	if !json.Valid([]byte(schema)) {
		return nil, fmt.Errorf("literal body is not valid JSON")
	}
	return json.RawMessage(schema), nil
}

// pointer is an open object whose description names the discovery schema to
// read first. The vocabulary is the operation id, which every wrapper
// re-spells into its own tool name (the same rule as a hint's see_also).
//
// The object stays open, but its container members are declared by JSON
// kind alone (see kindSkeleton): with no member typed, a host's model sent
// blocks and properties as strings holding JSON.
func (c *openAPIBodyComposer) pointer(kind, what string) (json.RawMessage, error) {
	skeleton, required, err := kindSkeleton(kind)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"type":                 "object",
		"description":          what + "; its schema and a worked example come from " + v2model.OpGetSchema + " with kind " + kind,
		"properties":           skeleton,
		"additionalProperties": true,
	}
	if len(required) > 0 {
		body["required"] = required
	}
	out, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode pointer body for kind %q: %w", kind, err)
	}
	return out, nil
}

// maxSkeletonEnum bounds the enums a skeleton repeats: a short one tells the
// caller the value to send, a long one (the generic document's every kind)
// is the lookup's to list.
const maxSkeletonEnum = 4

// kindSkeleton is a served kind's top-level members that hold an object, an
// array or a fixed value, each reduced to that JSON kind (and its const or
// enum): enough for a caller to send the right shapes, nothing the lookup
// does not repeat in full. Output-only members are left out.
//
// A required member of another kind (a template's template_for string) is
// listed with its type and description and returned in required: left to
// the lookup, a host's model put it under properties.
func kindSkeleton(kind string) (map[string]any, []string, error) {
	entry, err := schemaKind(kind)
	if err != nil {
		return nil, nil, err
	}
	var root struct {
		Properties map[string]map[string]any `json:"properties"`
		Defs       map[string]map[string]any `json:"$defs"`
		Required   []string                  `json:"required"`
	}
	if err := json.Unmarshal(entry.Schema, &root); err != nil {
		return nil, nil, fmt.Errorf("decode served schema %q: %w", kind, err)
	}
	resolve := func(member map[string]any) map[string]any {
		if ref, ok := member["$ref"].(string); ok {
			if def, ok := root.Defs[strings.TrimPrefix(ref, "#/$defs/")]; ok {
				return def
			}
		}
		return member
	}
	skeleton := map[string]any{}
	for name, member := range root.Properties {
		if out, _ := member["x-output-only"].(bool); out {
			continue
		}
		if v, ok := member["const"]; ok {
			skeleton[name] = map[string]any{"const": v}
			continue
		}
		if v, ok := member["enum"].([]any); ok && len(v) <= maxSkeletonEnum {
			skeleton[name] = map[string]any{"enum": v}
			continue
		}
		switch resolve(member)["type"] {
		case "object":
			skeleton[name] = map[string]any{"type": "object"}
		case "array":
			items := map[string]any{}
			if it, ok := member["items"].(map[string]any); ok {
				if t, ok := resolve(it)["type"]; ok {
					items["type"] = t
				}
			}
			skeleton[name] = map[string]any{"type": "array", "items": items}
		}
	}
	var required []string
	for _, name := range root.Required {
		member, ok := root.Properties[name]
		if !ok {
			continue
		}
		required = append(required, name)
		if _, done := skeleton[name]; done {
			continue
		}
		listed := map[string]any{}
		if t, ok := resolve(member)["type"]; ok {
			listed["type"] = t
		}
		if d, ok := member["description"].(string); ok && d != "" {
			listed["description"] = d
		}
		skeleton[name] = listed
	}
	return skeleton, required, nil
}

// envelope is `{"ops":[…]}` with the op vocabulary closed and each op's
// members left to its own served schema. `order` states the channel's
// write semantics, which differ: the object channel is atomic, the type
// channel is planned whole but written list by list.
func (c *openAPIBodyComposer) envelope(opNames []string, order string) (json.RawMessage, error) {
	names := append([]string(nil), opNames...)
	return json.Marshal(map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"ops"},
		"properties": map[string]any{
			"ops": map[string]any{
				"type":        "array",
				"minItems":    1,
				"maxItems":    v2MaxOpsPerPatch,
				"description": order + ". Each op's members, schema and example come from " + v2model.OpGetOpSchema + " with its op name",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"op"},
					"properties": map[string]any{
						"op": map[string]any{"type": "string", "enum": names},
					},
				},
			},
		},
	})
}

// anyOf joins alternative bodies. A failure in any branch is the result.
func (c *openAPIBodyComposer) anyOf(description string, branches ...func() (json.RawMessage, error)) (json.RawMessage, error) {
	schemas := make([]json.RawMessage, 0, len(branches))
	for _, branch := range branches {
		schema, err := branch()
		if err != nil {
			return nil, err
		}
		schemas = append(schemas, schema)
	}
	return json.Marshal(map[string]any{"anyOf": schemas, "description": description})
}

// openAPIComponentName spells a `$defs` name the way the document's
// components are spelled: anyValue becomes AnyValue.
func openAPIComponentName(def string) string {
	if def == "" {
		return def
	}
	return strings.ToUpper(def[:1]) + def[1:]
}

// rewriteDefRefs re-aims every `#/$defs/<def>` reference at its component.
func rewriteDefRefs(node any, names map[string]string) {
	switch n := node.(type) {
	case map[string]any:
		if ref, ok := n["$ref"].(string); ok && strings.HasPrefix(ref, "#/$defs/") {
			if name, known := names[strings.TrimPrefix(ref, "#/$defs/")]; known {
				n["$ref"] = "#/components/schemas/" + name
			}
		}
		for _, child := range n {
			rewriteDefRefs(child, names)
		}
	case []any:
		for _, child := range n {
			rewriteDefRefs(child, names)
		}
	}
}
