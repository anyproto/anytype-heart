package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// TestV2RequestBodiesMatchTheComposition pins the embedded document's
// request bodies to core/api/v2/service/openapibodies.go: an edit to a served
// discovery schema, or to the recipe table, that is not followed by
// `make openapi` fails here. It also refuses the bare `{"type":"object"}`
// body swag emits for an annotation the table does not cover, so a new
// operation cannot ship an opaque body unnoticed.
func TestV2RequestBodiesMatchTheComposition(t *testing.T) {
	var doc struct {
		Paths map[string]map[string]struct {
			OperationId string `json:"operationId"`
			RequestBody *struct {
				Content map[string]struct {
					Schema json.RawMessage `json:"schema"`
				} `json:"content"`
			} `json:"requestBody"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(openapiV2JSON, &doc))
	composed, err := v2service.ComposeOpenAPIBodies()
	require.NoError(t, err)

	canonical := func(raw json.RawMessage) any {
		var v any
		require.NoError(t, json.Unmarshal(raw, &v))
		return v
	}
	seen := map[string]bool{}
	for path, item := range doc.Paths {
		for method, operation := range item {
			if operation.RequestBody == nil {
				continue
			}
			body, ok := operation.RequestBody.Content["application/json"]
			if !ok {
				continue
			}
			want, composedHere := composed.Bodies[operation.OperationId]
			if composedHere {
				seen[operation.OperationId] = true
				assert.Equal(t, canonical(want), canonical(body.Schema),
					"%s %s (%s): the document's body differs from the composition; run make openapi", method, path, operation.OperationId)
				continue
			}
			assert.NotEqual(t, map[string]any{"type": "object"}, canonical(body.Schema),
				"%s %s (%s) publishes an opaque body; add a recipe to openAPIBodyRecipes", method, path, operation.OperationId)
		}
	}
	for op := range composed.Bodies {
		assert.True(t, seen[op], "%s is composed but the document has no such operation with a JSON body", op)
	}
	for name, want := range composed.Components {
		got, ok := doc.Components.Schemas[name]
		require.True(t, ok, "component %s missing from the document; run make openapi", name)
		assert.Equal(t, canonical(want), canonical(got), "component %s", name)
	}
}
