package full

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouteSpeller(t *testing.T) {
	speller := newRouteSpeller()
	speller.add("list_spaces", "get", "/v2/spaces")
	speller.add("list_chats", "get", "/v2/spaces/{space_id}/chats")
	speller.add("get_schema", "get", "/v2/schemas/{kind}")
	speller.add("get_op_schema", "get", "/v2/schemas/ops/{op}")
	speller.add("search_space", "post", "/v2/spaces/{space_id}/search")

	for _, tc := range []struct {
		name, in, want string
	}{
		{"a route with no placeholders", "Space id, from GET /v2/spaces", "Space id, from `list_spaces`"},
		{"a placeholder written as itself stays unbound", "Chat id, from GET /v2/spaces/{space_id}/chats", "Chat id, from `list_chats`"},
		{"a bound placeholder is an argument", "come from GET /v2/schemas/object.", "come from `get_schema` with kind: object."},
		{"the more literal template wins", "come from GET /v2/schemas/ops/{op} with its op name", "come from `get_op_schema` with its op name"},
		{"a query is an argument too", "resend GET /v2/spaces?limit=5", "resend `list_spaces` with limit: 5"},
		{"two routes in one sentence", "from POST /v2/spaces/{space_id}/search or GET /v2/spaces", "from `search_space` or `list_spaces`"},
		{"the method must match", "DELETE /v2/spaces stays", "DELETE /v2/spaces stays"},
		{"a route no tool serves is left as written", "GET /v2/members", "GET /v2/members"},
		{"prose without a route is untouched", "use list_spaces first", "use list_spaces first"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, speller.respell(tc.in))
		})
	}

	t.Run("data values in a schema are not prose", func(t *testing.T) {
		// given
		schema := map[string]any{
			"description": "from GET /v2/spaces",
			"properties": map[string]any{
				"note": map[string]any{"description": "from GET /v2/spaces", "default": "GET /v2/spaces",
					"examples": []any{"GET /v2/spaces"}, "enum": []any{"GET /v2/spaces"}},
			},
		}

		// when
		speller.respellSchema(schema)

		// then
		note := schema["properties"].(map[string]any)["note"].(map[string]any)
		assert.Equal(t, "from `list_spaces`", schema["description"])
		assert.Equal(t, "from `list_spaces`", note["description"])
		assert.Equal(t, "GET /v2/spaces", note["default"])
		assert.Equal(t, []any{"GET /v2/spaces"}, note["examples"])
		assert.Equal(t, []any{"GET /v2/spaces"}, note["enum"])
	})
}

// TestNoRouteReachesATool: the document names operations by route for HTTP
// callers; every one of them must reach a tool caller as the tool.
func TestNoRouteReachesATool(t *testing.T) {
	table := deriveReal(t)
	for _, tool := range table.Tools {
		assert.NotRegexp(t, routeToken, tool.Description, tool.Name)
		var schema any
		require.NoError(t, json.Unmarshal(tool.InputSchema, &schema))
		var walk func(node any, path string)
		walk = func(node any, path string) {
			switch n := node.(type) {
			case map[string]any:
				for k, v := range n {
					if text, ok := v.(string); ok && k == "description" {
						assert.NotRegexp(t, routeToken, text, "%s %s", tool.Name, path)
						continue
					}
					walk(v, path+"/"+k)
				}
			case []any:
				for _, v := range n {
					walk(v, path)
				}
			}
		}
		walk(schema, "")
	}
}
