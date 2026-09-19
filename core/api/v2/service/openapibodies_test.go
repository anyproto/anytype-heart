package v2service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// compileOpenAPIBody compiles a composed body with its components resolvable
// under the document's own component prefix.
func compileOpenAPIBody(t *testing.T, composed *OpenAPIBodies, body json.RawMessage) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	doc := map[string]any{"components": map[string]any{"schemas": map[string]any{}}}
	for name, schema := range composed.Components {
		var v any
		require.NoError(t, json.Unmarshal(schema, &v))
		doc["components"].(map[string]any)["schemas"].(map[string]any)[name] = v
	}
	var root map[string]any
	require.NoError(t, json.Unmarshal(body, &root))
	for k, v := range root {
		doc[k] = v
	}
	require.NoError(t, c.AddResource("openapi.json", doc))
	schema, err := c.Compile("openapi.json")
	require.NoError(t, err)
	return schema
}

func TestOpenAPIBodiesAcceptTheServedExamples(t *testing.T) {
	composed, err := ComposeOpenAPIBodies()
	require.NoError(t, err)

	// each body, with the served examples it must accept
	cases := []struct {
		op       string
		examples []string
	}{
		{v2model.OpCreateProperty, []string{v2SchemaKinds["property"].example}},
		{v2model.OpUpdateProperty, []string{`{"name":"Priority"}`}},
		{v2model.OpCreateQuery, []string{v2SchemaKinds["query"].example}},
		{v2model.OpCreateCollection, []string{v2SchemaKinds["collection"].example}},
		{v2model.OpCreateType, []string{v2SchemaKinds["type"].example, v2SchemaKinds["type_document"].example}},
		{v2model.OpUpdateType, []string{`{"name":"Plants"}`, `{"ops":[` + v2OpSchemas["add_property"].example + `]}`, `{"ops":[` + v2OpSchemas["insert_view"].example + `]}`}},
		{v2model.OpCreateObject, []string{v2SchemaKinds["shortcut"].example, v2SchemaKinds["object"].example}},
		{v2model.OpCreateTemplate, []string{v2SchemaKinds["template"].example}},
		{v2model.OpValidate, []string{v2SchemaKinds["object"].example}},
		{v2model.OpPatchObject, []string{`{"ops":[` + v2OpSchemas["set_properties"].example + `,` + v2OpSchemas["insert_blocks"].example + `]}`}},
	}
	seen := map[string]bool{}
	for _, tc := range cases {
		seen[tc.op] = true
		t.Run(tc.op, func(t *testing.T) {
			body, ok := composed.Bodies[tc.op]
			require.True(t, ok, "no composed body")
			schema := compileOpenAPIBody(t, composed, body)
			for _, example := range tc.examples {
				var v any
				require.NoError(t, json.Unmarshal([]byte(example), &v))
				assert.NoError(t, schema.Validate(v), example)
			}
		})
	}
	for op := range composed.Bodies {
		assert.True(t, seen[op], "%s is composed but has no example case here", op)
	}
}

func TestOpenAPIBodiesRefuseTheWrongShape(t *testing.T) {
	composed, err := ComposeOpenAPIBodies()
	require.NoError(t, err)
	refused := []struct{ op, body string }{
		// the eval's blind writes: `type` where the API wants `format`
		{v2model.OpCreateProperty, `{"name":"Urgency","type":"select"}`},
		// a structured filters array on the query kind
		{v2model.OpCreateQuery, `{"name":"Open","type":"task","filters":[{"key":"done","value":false}]}`},
		// an op outside the type channel
		{v2model.OpUpdateType, `{"ops":[{"op":"set_properties","set":{"name":"x"}}]}`},
		// an op outside the object channel
		{v2model.OpPatchObject, `{"ops":[{"op":"add_property","property":"x"}]}`},
		// a bare op object instead of the envelope
		{v2model.OpPatchObject, `{"op":"set_properties","set":{"name":"x"}}`},
		{v2model.OpUpdateProperty, `{"format":"text"}`},
	}
	for _, tc := range refused {
		t.Run(tc.op+" "+tc.body, func(t *testing.T) {
			schema := compileOpenAPIBody(t, composed, composed.Bodies[tc.op])
			var v any
			require.NoError(t, json.Unmarshal([]byte(tc.body), &v))
			assert.Error(t, schema.Validate(v))
		})
	}
}

func TestOpenAPIBodiesHoistDefsIntoComponents(t *testing.T) {
	composed, err := ComposeOpenAPIBodies()
	require.NoError(t, err)
	_, ok := composed.Components["AnyValue"]
	assert.True(t, ok, "the query kind's anyValue def is a component")
	for op, body := range composed.Bodies {
		assert.NotContains(t, string(body), `#/$defs/`, "%s still references a root def a flattening reader would drop", op)
		assert.NotContains(t, string(body), `"$defs"`, op)
	}
	assert.Contains(t, string(composed.Bodies[v2model.OpCreateQuery]), `"#/components/schemas/AnyValue"`)
}

func TestOpenAPIBodiesNameOperationsByOpId(t *testing.T) {
	composed, err := ComposeOpenAPIBodies()
	require.NoError(t, err)
	for op, body := range composed.Bodies {
		text := string(body)
		assert.NotContains(t, text, "/v2/", "%s spells a route", op)
		assert.NotContains(t, text, "—", "%s carries an em dash the document prose rules refuse", op)
	}
	pointer := string(composed.Bodies[v2model.OpCreateTemplate])
	assert.True(t, strings.Contains(pointer, v2model.OpGetSchema+" with kind template"), pointer)
	envelope := string(composed.Bodies[v2model.OpPatchObject])
	assert.True(t, strings.Contains(envelope, v2model.OpGetOpSchema), envelope)
}
