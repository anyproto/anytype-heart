package v2model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOperationsMatchOpenAPI pins the hand-copied operation table to the
// generated document: same ids, same method, same path. A route renamed in
// a handler annotation fails here until the table follows.
func TestOperationsMatchOpenAPI(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "v2", "openapi.json"))
	require.NoError(t, err)
	var doc struct {
		Paths map[string]map[string]struct {
			OperationId string `json:"operationId"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))

	documented := map[string]operation{}
	for path, methods := range doc.Paths {
		for method, op := range methods {
			require.NotEmpty(t, op.OperationId, "%s %s has no operationId", method, path)
			documented[op.OperationId] = operation{method: strings.ToUpper(method), path: path}
		}
	}
	assert.Equal(t, documented, operations, "operation table must equal the OpenAPI document")
}

// TestRefHelpersMatchOperations checks every helper binds only parameters
// its operation's path declares, so a rendered route never keeps a stray
// placeholder or drops a bound value.
func TestRefHelpersMatchOperations(t *testing.T) {
	refs := []Ref{
		RefListSpaces(),
		RefListTypes("s"), RefGetType("s", "t"), RefCreateType("s"), RefUpdateType("s", "t"), RefDeleteType("s", "t"),
		RefListProperties("s"), RefCreateProperty("s"), RefUpdateProperty("s", "k"), RefDeleteProperty("s", "k"), RefListPropertyOptions("s", "k"),
		RefListMembers("s"), RefListChats("s"), RefGetChatMessages("s", "c"),
		RefListObjects("s"), RefGetObject("s", "o"), RefPatchObject("s", "o"), RefSearchSpace("s"),
		RefCreateCollection("s"), RefCreateQuery("s"), RefUploadFile("s"),
		RefGetCollectionObjects("s", "c"), RefGetQueryObjects("s", "q"),
		RefGetSchema("k"), RefGetOpSchema("op"),
		RefListWidgets("s"), RefCreateWidget("s"),
	}
	placeholder := regexp.MustCompile(`\{[a-z_]+\}`)
	for _, r := range refs {
		o, ok := operations[r.Op]
		require.True(t, ok, "helper names unknown op %q", r.Op)
		for name := range r.Params {
			assert.Contains(t, o.path, "{"+name+"}", "%s binds %q, which its path does not declare", r.Op, name)
		}
		assert.Equal(t, len(placeholder.FindAllString(o.path, -1)), len(r.Params),
			"%s: every path parameter should be bound by its helper", r.Op)
		assert.NotContains(t, r.String(), "{", "%s renders with a placeholder left: %s", r.Op, r)
	}
}

func TestRefString(t *testing.T) {
	cases := []struct {
		name string
		ref  Ref
		want string
	}{
		{"bare op", RefListSpaces(), "GET /v2/spaces"},
		{"bound params", RefListPropertyOptions("sp1.abc", "status"), "GET /v2/spaces/sp1.abc/properties/status/options"},
		{"unbound param keeps its placeholder", NewRef(OpDeleteProperty, "space_id", "sp1"), "DELETE /v2/spaces/sp1/properties/{key}"},
		{"fully unbound", NewRef(OpPatchObject), "PATCH /v2/spaces/{space_id}/objects/{object_id}"},
		{"query", RefGetObject("sp1", "o1").With("outline", "true"), "GET /v2/spaces/sp1/objects/o1?outline=true"},
		{"resend", Resend("create_missing_options", "true"), "?create_missing_options=true"},
		{"query keys sorted", NewRef(OpListSpaces).With("limit", "5").With("after", "x"), "GET /v2/spaces?after=x&limit=5"},
		{"a bound value that looks like a placeholder is opaque", RefListPropertyOptions("s", "{space_id}"), "GET /v2/spaces/s/properties/{space_id}/options"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.ref.String())
		})
	}
}

func TestHintf(t *testing.T) {
	t.Run("renders each ref in REST and keeps it typed", func(t *testing.T) {
		issue := Issue{Path: "/type", Message: "needs a type key"}.
			Hintf("list keys with %s, or create one with %s", RefListTypes("sp1"), RefCreateType("sp1"))

		assert.Equal(t, "list keys with GET /v2/spaces/sp1/types, or create one with POST /v2/spaces/sp1/types", issue.Hint)
		assert.Equal(t, []Ref{RefListTypes("sp1"), RefCreateType("sp1")}, issue.SeeAlso)
	})

	t.Run("wire shape", func(t *testing.T) {
		issue := Issue{Message: "m"}.Hintf("see %s", RefGetOpSchema("set_properties"))
		raw, err := json.Marshal(issue)
		require.NoError(t, err)
		assert.JSONEq(t, `{"message":"m","hint":"see GET /v2/schemas/ops/set_properties","see_also":[{"op":"get_op_schema","params":{"op":"set_properties"}}]}`, string(raw))
	})

	t.Run("a plain hint carries no see_also", func(t *testing.T) {
		issue := Issue{Message: "m"}.WithHint(Plain("drop the id"))
		raw, err := json.Marshal(issue)
		require.NoError(t, err)
		assert.JSONEq(t, `{"message":"m","hint":"drop the id"}`, string(raw))
	})

	t.Run("resend shape", func(t *testing.T) {
		raw, err := json.Marshal(Resend("create_missing_options", "true"))
		require.NoError(t, err)
		assert.JSONEq(t, `{"query":{"create_missing_options":"true"}}`, string(raw))
	})
}
