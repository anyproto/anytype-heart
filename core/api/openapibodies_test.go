package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// TestV2RequestBodiesMatchTheComposition pins the embedded document's
// request bodies to core/api/v2/service/openapibodies.go: an edit to a served
// discovery schema, or to the recipe table, that is not followed by
// `make openapi` fails here, on the JSON document and on its YAML twin. It
// also refuses the bare `{"type":"object"}` body swag emits for an
// annotation the table does not cover, and one a recipe would compose, so
// no operation can ship an opaque body unnoticed.
func TestV2RequestBodiesMatchTheComposition(t *testing.T) {
	composed, err := v2service.ComposeOpenAPIBodies()
	require.NoError(t, err)

	var jsonDoc any
	require.NoError(t, json.Unmarshal(openapiV2JSON, &jsonDoc))
	var yamlDoc any
	require.NoError(t, yaml.Unmarshal(openapiV2YAML, &yamlDoc))

	for name, doc := range map[string]any{"openapi.json": jsonDoc, "openapi.yaml": canonicalYAML(t, yamlDoc)} {
		t.Run(name, func(t *testing.T) {
			assertRequestBodiesMatch(t, doc.(map[string]any), composed)
		})
	}
}

// canonicalYAML takes a decoded YAML document through JSON so it compares
// like the JSON document (string keys, float64 numbers).
func canonicalYAML(t *testing.T, doc any) any {
	t.Helper()
	raw, err := json.Marshal(doc)
	require.NoError(t, err)
	var out any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func assertRequestBodiesMatch(t *testing.T, doc map[string]any, composed *v2service.OpenAPIBodies) {
	t.Helper()
	canonical := func(raw json.RawMessage) any {
		var v any
		require.NoError(t, json.Unmarshal(raw, &v))
		return v
	}
	seen := map[string]bool{}
	for path, item := range doc["paths"].(map[string]any) {
		for method, raw := range item.(map[string]any) {
			operation, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			operationId, _ := operation["operationId"].(string)
			requestBody, _ := operation["requestBody"].(map[string]any)
			content, _ := requestBody["content"].(map[string]any)
			media, ok := content["application/json"].(map[string]any)
			if !ok {
				continue
			}
			schema := media["schema"]
			// the opacity guard runs first, on every body: a recipe that
			// composes an unconstrained object is as opaque as no recipe
			assert.False(t, opaqueBody(schema),
				"%s %s (%s) publishes an opaque body: give it a shape, or a description naming the discovery operation to read", method, path, operationId)
			if want, composedHere := composed.Bodies[operationId]; composedHere {
				seen[operationId] = true
				assert.Equal(t, canonical(want), schema,
					"%s %s (%s): the document's body differs from the composition; run make openapi", method, path, operationId)
			}
		}
	}
	for op := range composed.Bodies {
		assert.True(t, seen[op], "%s is composed but the document has no such operation with a JSON body", op)
	}
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	for name, want := range composed.Components {
		got, ok := schemas[name]
		require.True(t, ok, "component %s missing from the document; run make openapi", name)
		assert.Equal(t, canonical(want), got, "component %s", name)
	}
}

// opaqueBody reports whether a body schema constrains nothing and says
// nothing about where its shape is published: an object (or untyped)
// schema with no effective constraint, whose description does not point at
// a discovery operation with its argument.
func opaqueBody(schema any) bool {
	node, ok := schema.(map[string]any)
	if !ok {
		return false
	}
	if typ, has := node["type"]; has && typ != "object" {
		return false // an array or scalar body is a shape of its own
	}
	if pointsAtDiscovery(node) {
		return false
	}
	return !constrainsObject(node)
}

// pointsAtDiscovery reports whether a description names a discovery
// operation together with what to ask it for.
func pointsAtDiscovery(node map[string]any) bool {
	description, _ := node["description"].(string)
	return strings.Contains(description, v2model.OpGetSchema+" with kind ") ||
		strings.Contains(description, v2model.OpGetOpSchema+" with ")
}

// constrainsObject reports whether a schema node narrows an object at all.
func constrainsObject(node map[string]any) bool {
	if props, ok := node["properties"].(map[string]any); ok && len(props) > 0 {
		return true
	}
	if required, ok := node["required"].([]any); ok && len(required) > 0 {
		return true
	}
	for _, keyword := range []string{"anyOf", "oneOf", "allOf"} {
		if branches, ok := node[keyword].([]any); ok {
			for _, branch := range branches {
				if b, ok := branch.(map[string]any); ok && (constrainsObject(b) || pointsAtDiscovery(b)) {
					return true
				}
			}
		}
	}
	for _, keyword := range []string{"$ref", "not", "if", "enum", "const", "propertyNames", "maxProperties", "minProperties"} {
		if _, has := node[keyword]; has {
			return true
		}
	}
	if patterns, ok := node["patternProperties"].(map[string]any); ok && len(patterns) > 0 {
		return true
	}
	switch extra := node["additionalProperties"].(type) {
	case bool:
		return !extra
	case map[string]any:
		return constrainsObject(extra) || extra["type"] != nil
	}
	return false
}

func TestOpaqueBodyGuard(t *testing.T) {
	cases := []struct {
		name   string
		schema string
		opaque bool
	}{
		{"bare object", `{"type":"object"}`, true},
		{"open map", `{"type":"object","additionalProperties":true}`, true},
		{"empty properties", `{"type":"object","properties":{}}`, true},
		{"empty allOf branch", `{"type":"object","allOf":[{}]}`, true},
		{"described extra without type", `{"type":"object","additionalProperties":{"description":"anything"}}`, true},
		{"incidental mention", `{"type":"object","description":"a body; get_schema is elsewhere"}`, true},
		{"pointer with kind", `{"type":"object","description":"a document; its schema comes from get_schema with kind object"}`, false},
		{"pointer with op", `{"type":"object","description":"each op comes from get_op_schema with its op name"}`, false},
		{"members", `{"type":"object","properties":{"name":{"type":"string"}}}`, false},
		{"closed empty object", `{"type":"object","additionalProperties":false}`, false},
		{"typed map", `{"type":"object","additionalProperties":{"type":"string"}}`, false},
		{"union with a pointer branch", `{"anyOf":[{"type":"object","description":"see get_schema with kind object"},{"type":"object"}]}`, false},
		{"reference", `{"$ref":"#/components/schemas/X"}`, false},
		{"array body", `{"type":"array","items":{"type":"string"}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var schema any
			require.NoError(t, json.Unmarshal([]byte(tc.schema), &schema))
			assert.Equal(t, tc.opaque, opaqueBody(schema))
		})
	}
}
