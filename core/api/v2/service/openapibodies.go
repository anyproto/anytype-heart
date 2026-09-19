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
	v2model.OpCreateProperty:   func(c *openAPIBodyComposer) (json.RawMessage, error) { return c.kind("property") },
	v2model.OpUpdateProperty:   func(c *openAPIBodyComposer) (json.RawMessage, error) { return c.literal(openAPIUpdatePropertyBody) },
	v2model.OpCreateQuery:      func(c *openAPIBodyComposer) (json.RawMessage, error) { return c.kind("query") },
	v2model.OpCreateCollection: func(c *openAPIBodyComposer) (json.RawMessage, error) { return c.kind("collection") },
	v2model.OpCreateType: func(c *openAPIBodyComposer) (json.RawMessage, error) {
		return c.anyOf("the flat type body, or the interchange document with kind object_type",
			func() (json.RawMessage, error) { return c.kind("type") },
			func() (json.RawMessage, error) {
				return c.pointer("type_document", "the type as an AnyBlock document: formatVersion 2.0, kind object_type, the definition under type_settings")
			})
	},
	v2model.OpUpdateType: func(c *openAPIBodyComposer) (json.RawMessage, error) {
		return c.anyOf("the fields to change, or an ops envelope that edits the property list and the views one op at a time",
			func() (json.RawMessage, error) { return c.kind("type") },
			func() (json.RawMessage, error) {
				return c.envelope(v2TypeOpNames, "property ops are written first, view ops after")
			})
	},
	v2model.OpCreateObject: func(c *openAPIBodyComposer) (json.RawMessage, error) {
		return c.anyOf("the shortcut body, or a full AnyBlock document; formatVersion or blocks picks the document form",
			func() (json.RawMessage, error) { return c.kind("shortcut") },
			func() (json.RawMessage, error) {
				return c.pointer("object", "a full AnyBlock document: formatVersion 2.0, properties, and blocks in preorder")
			})
	},
	v2model.OpCreateTemplate: func(c *openAPIBodyComposer) (json.RawMessage, error) {
		return c.pointer("template", "the template as an AnyBlock document: formatVersion 2.0, kind template, template_for naming the type")
	},
	v2model.OpValidate: func(c *openAPIBodyComposer) (json.RawMessage, error) {
		return c.pointer("object", "an AnyBlock document of any kind, checked without being stored")
	},
	v2model.OpPatchObject: func(c *openAPIBodyComposer) (json.RawMessage, error) {
		return c.envelope(v2OpNames, "if any one is refused, none is applied")
	},
}

// openAPIUpdatePropertyBody is PATCH properties/{key}: the one member
// UpdatePropertyRequest takes. The key is identity and does not change.
const openAPIUpdatePropertyBody = `{"type":"object","additionalProperties":false,"required":["name"],"properties":{` +
	`"name":{"type":"string","minLength":1,"maxLength":4096,"description":"the new display name; the key is identity and does not change"}}}`

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
	entry, err := schemaKind(kind)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(entry.Schema, &root); err != nil {
		return nil, fmt.Errorf("decode served schema %q: %w", kind, err)
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
func (c *openAPIBodyComposer) pointer(kind, what string) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"type":        "object",
		"description": what + "; its schema and a worked example come from " + v2model.OpGetSchema + " with kind " + kind,
	})
}

// envelope is `{"ops":[…]}` with the op vocabulary closed and each op's
// members left to its own served schema.
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
				"maxItems":    512,
				"description": "applied in order as one edit; " + order + ". Each op's members, schema and example come from " + v2model.OpGetOpSchema + " with its op name",
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
