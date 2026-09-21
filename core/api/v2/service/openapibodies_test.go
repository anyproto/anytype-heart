package v2service

import (
	"encoding/json"
	"fmt"
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
		{v2model.OpUpdateProperty, []string{`{"name":"Priority"}`, `{"name":""}`}},
		{v2model.OpCreateQuery, []string{v2SchemaKinds["query"].example,
			// an empty filter string is no filter, so it may sit beside the array or views
			`{"name":"All","type":"task","filter":"","filters":[]}`, `{"name":"All","type":"task","views":[{"name":"All"}],"filter":""}`,
			`{"name":"Open","type":"task","filters":[{"property":"severity","condition":"in","value":["High"]},{"operator":"or","filters":[{"property":"done","condition":"equal","value":false}]}]}`}},
		{v2model.OpCreateCollection, []string{v2SchemaKinds["collection"].example}},
		{v2model.OpCreateType, []string{v2SchemaKinds["type"].example, v2SchemaKinds["type_document"].example}},
		{v2model.OpUpdateType, []string{`{"name":"Plants"}`, `{"properties":{"description":"Updated"}}`, `{"properties":{"name":"Plant","description":null}}`, `{"type_settings":{"layout":"todo","property_definitions":[{"name":"Location","format":"select"}]}}`, `{"icon":{"format":"emoji","emoji":"🌱"}}`,
			`{"ops":[` + v2OpSchemas["add_property"].example + `]}`, `{"ops":[` + v2OpSchemas["insert_view"].example + `]}`}},
		{v2model.OpCreateObject, []string{v2SchemaKinds["shortcut"].example, v2SchemaKinds["object"].example}},
		{v2model.OpCreateTemplate, []string{v2SchemaKinds["template"].example}},
		{v2model.OpValidate, []string{v2SchemaKinds["object"].example, v2SchemaKinds["type_document"].example}},
		{v2model.OpPatchObject, []string{`{"ops":[` + v2OpSchemas["set_properties"].example + `,` + v2OpSchemas["insert_blocks"].example + `]}`}},
		{v2model.OpCreateWidget, []string{v2SchemaKinds["widget"].example, `{"target":"obj1","scope":"space","layout":"view","limit":10,"view_id":"v1","after":"_favorite"}`}},
		{v2model.OpUpdateWidget, []string{`{"layout":"compact_list"}`, `{"limit":30,"position":"first"}`, `{"view_id":""}`}},
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

	// and the refusals the widget bodies promise: POST needs its identity
	// members, PATCH refuses them and takes at least one member; an
	// off-list or negative limit is the service's to normalise, not the
	// schema's to refuse
	t.Run("widget refusals", func(t *testing.T) {
		create := compileOpenAPIBody(t, composed, composed.Bodies[v2model.OpCreateWidget])
		update := compileOpenAPIBody(t, composed, composed.Bodies[v2model.OpUpdateWidget])
		for _, example := range []string{`{"scope":"personal"}`, `{"target":"obj1"}`, `{"target":"obj1","scope":"shared"}`,
			`{"target":"obj1","scope":"space","after":""}`, `{"target":"obj1","scope":"space","after":"w1","position":"last"}`} {
			var v any
			require.NoError(t, json.Unmarshal([]byte(example), &v))
			assert.Error(t, create.Validate(v), example)
		}
		for _, example := range []string{`{}`, `{"target":"obj1","limit":10}`, `{"scope":"space","limit":10}`, `{"position":"middle"}`,
			`{"after":""}`, `{"before":""}`, `{"after":"w1","before":"w2"}`, `{"after":"w1","position":"first"}`, `{"before":"w1","position":"last"}`} {
			var v any
			require.NoError(t, json.Unmarshal([]byte(example), &v))
			assert.Error(t, update.Validate(v), example)
		}
		for _, example := range []string{`{"target":"obj1","scope":"space","limit":7}`, `{"target":"obj1","scope":"space","limit":-1}`} {
			var v any
			require.NoError(t, json.Unmarshal([]byte(example), &v))
			assert.NoError(t, create.Validate(v), example)
		}
		var v any
		require.NoError(t, json.Unmarshal([]byte(`{"limit":-1}`), &v))
		assert.NoError(t, update.Validate(v))
	})
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
		// the slug is identity: create-only
		{v2model.OpUpdateType, `{"api_key":"changed"}`},
		{v2model.OpUpdateType, `{"type_settings":{"api_key":"changed"}}`},
		// a filter node the served kind refuses
		{v2model.OpCreateQuery, `{"name":"Open","type":"task","filters":[{"key":"done","value":false}]}`},
		// both filter channels, and views beside a top-level channel: ambiguous_input
		{v2model.OpCreateQuery, `{"name":"Open","type":"task","filter":"done = false","filters":[]}`},
		{v2model.OpCreateQuery, `{"name":"Open","type":"task","views":[{"name":"All"}],"sorts":[{"property":"name"}]}`},
		{v2model.OpCreateQuery, `{"name":"Open","type":"task","views":[{"name":"All"}],"filter":"done = false"}`},
		{v2model.OpCreateQuery, `{"name":"Open","type":"task","views":[{"name":"All"}],"filters":[]}`},
		// the partial document writes name and description only, as strings
		{v2model.OpUpdateType, `{"properties":{"status":"Done"}}`},
		{v2model.OpUpdateType, `{"properties":{"name":123}}`},
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
	filterNode, ok := composed.Components["FilterNode"]
	assert.True(t, ok, "the filters kind's filterNode def is a component")
	assert.Contains(t, string(filterNode), `"#/components/schemas/FilterNode"`, "the recursive reference is re-aimed inside the def too")
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
	assert.Contains(t, string(composed.Bodies[v2model.OpValidate]), v2model.OpGetSchema+" with kind document", "validate checks any kind, so it points at the unnarrowed schema")
	envelope := string(composed.Bodies[v2model.OpPatchObject])
	assert.True(t, strings.Contains(envelope, v2model.OpGetOpSchema), envelope)
	assert.Contains(t, envelope, "none is applied", "the object channel is atomic")
	// the union's document branch is open by design, so the flat branch's
	// requirement is asserted on its text
	assert.Contains(t, string(composed.Bodies[v2model.OpCreateType]), `"required":["name"]`)
	typeEnvelope := string(composed.Bodies[v2model.OpUpdateType])
	assert.Contains(t, typeEnvelope, "leaves the written property lists in place", "the type channel is not, and says so")
	assert.NotContains(t, typeEnvelope, "none is applied")
	assert.Contains(t, envelope, fmt.Sprintf(`"maxItems":%d`, v2MaxOpsPerPatch))
}
