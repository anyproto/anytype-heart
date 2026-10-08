package full

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

// recorded is one request the stub API saw.
type recorded struct {
	Method, Path, RawQuery string
	Header                 http.Header
	Body                   string
}

// stubAPI records requests and answers from a script: one response per
// call, the last one repeating.
type stubAPI struct {
	mu        sync.Mutex
	requests  []recorded
	responses []stubResponse
}

type stubResponse struct {
	status int
	body   string
}

func (s *stubAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, recorded{Method: r.Method, Path: r.URL.EscapedPath(), RawQuery: r.URL.RawQuery, Header: r.Header.Clone(), Body: string(body)})
	resp := s.responses[0]
	if len(s.responses) > 1 {
		s.responses = s.responses[1:]
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.status)
	_, _ = io.WriteString(w, resp.body)
}

func newExecutorFixture(t *testing.T, responses ...stubResponse) (*Executor, *stubAPI) {
	t.Helper()
	if len(responses) == 0 {
		responses = []stubResponse{{200, `{"ok":true}`}}
	}
	api := &stubAPI{responses: responses}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	client := wrapper.NewClient(server.URL, "caller-key")
	client.Backoff = nil
	ex := NewExecutor(client, deriveReal(t))
	keys := 0
	ex.newKey = func() (string, error) { keys++; return "minted-" + strings.Repeat("k", keys), nil }
	return ex, api
}

func TestExecutorAssemblesRequests(t *testing.T) {
	t.Run("path, query, header and body each go where the table says", func(t *testing.T) {
		ex, api := newExecutorFixture(t)
		_, err := ex.Run(context.Background(), "patch_object", map[string]any{
			"space_id":  "bafyreiabc.28y6mgnwgodt7",
			"object_id": "obj 1",
			"dry_run":   true,
			"if_match":  `"etag-1"`,
			"ops":       []any{map[string]any{"op": "set_properties", "set": map[string]any{"done": true}}},
		})
		require.NoError(t, err)
		require.Len(t, api.requests, 1)
		got := api.requests[0]
		assert.Equal(t, http.MethodPatch, got.Method)
		assert.Equal(t, "/v2/spaces/bafyreiabc.28y6mgnwgodt7/objects/obj%201", got.Path)
		assert.Equal(t, "dry_run=true", got.RawQuery)
		assert.Equal(t, `"etag-1"`, got.Header.Get("If-Match"))
		assert.Equal(t, "Bearer caller-key", got.Header.Get("Authorization"))
		assert.JSONEq(t, `{"ops":[{"op":"set_properties","set":{"done":true}}]}`, got.Body)
		assert.NotContains(t, got.Body, "space_id")
		assert.NotContains(t, got.Body, "dry_run")
	})

	t.Run("a GET sends no body and no idempotency key", func(t *testing.T) {
		ex, api := newExecutorFixture(t)
		_, err := ex.Run(context.Background(), "list_objects", map[string]any{"space_id": "s", "limit": 5})
		require.NoError(t, err)
		got := api.requests[0]
		assert.Equal(t, "/v2/spaces/s/objects", got.Path)
		assert.Equal(t, "limit=5", got.RawQuery)
		assert.Empty(t, got.Body)
		assert.Empty(t, got.Header.Get("Idempotency-Key"))
	})

	t.Run("a mutation without a key gets one minted, reused across the client's retries", func(t *testing.T) {
		ex, api := newExecutorFixture(t, stubResponse{503, `{}`}, stubResponse{201, `{"id":"new"}`})
		result, err := ex.Run(context.Background(), "create_chat", map[string]any{"space_id": "s", "name": "General"})
		require.NoError(t, err)
		require.Len(t, api.requests, 2, "the 503 was retried")
		first := api.requests[0].Header.Get("Idempotency-Key")
		assert.Equal(t, "minted-k", first)
		assert.Equal(t, first, api.requests[1].Header.Get("Idempotency-Key"), "the retry carries the same key")
		assert.Equal(t, api.requests[0].Body, api.requests[1].Body)
		assert.Equal(t, `{"id":"new"}`, result.Text)
	})

	t.Run("an explicit idempotency key is sent as given", func(t *testing.T) {
		ex, api := newExecutorFixture(t)
		_, err := ex.Run(context.Background(), "create_chat", map[string]any{"space_id": "s", "name": "General", "idempotency_key": "mine"})
		require.NoError(t, err)
		assert.Equal(t, "mine", api.requests[0].Header.Get("Idempotency-Key"))
	})

	t.Run("an open body carries members the schema does not list", func(t *testing.T) {
		ex, api := newExecutorFixture(t)
		_, err := ex.Run(context.Background(), "create_object", map[string]any{"space_id": "s", "formatVersion": "2.0", "blocks": []any{}})
		require.NoError(t, err)
		assert.JSONEq(t, `{"formatVersion":"2.0","blocks":[]}`, api.requests[0].Body)
	})
}

func TestExecutorRefusesShapeMistakes(t *testing.T) {
	cases := []struct {
		name string
		tool string
		args map[string]any
		want string
	}{
		{"unknown tool", "no_such_tool", nil, `unknown tool "no_such_tool" — tools: `},
		{"unknown argument on a strict body", "create_chat", map[string]any{"space_id": "s", "name": "x", "colour": "red"}, `create_chat does not take "colour" — arguments: `},
		{"missing path argument", "get_object", map[string]any{"object_id": "o"}, `get_object needs "space_id"`},
		{"missing required body member", "upload_file", map[string]any{"space_id": "s"}, `upload_file needs "url"`},
		{"body on a bodiless operation", "list_spaces", map[string]any{"name": "x"}, `list_spaces does not take "name"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ex, api := newExecutorFixture(t)
			_, err := ex.Run(context.Background(), tc.tool, tc.args)
			require.Error(t, err)
			var ae wrapper.ArgumentError
			require.ErrorAs(t, err, &ae, "a shape mistake is a pre-flight refusal")
			assert.Contains(t, err.Error(), tc.want)
			assert.Empty(t, api.requests, "nothing reached the server")
		})
	}
}

// restRouteShape is the guard: no served string may carry a REST route.
var restRouteShape = regexp.MustCompile(`(?:GET|POST|PATCH|PUT|DELETE) /v[0-9]+`)

func TestExecutorRendersRefusals(t *testing.T) {
	envelope := func(message string, issues ...v2model.Issue) string {
		raw, err := json.Marshal(v2model.ValidationFailed(message, issues...))
		require.NoError(t, err)
		return string(raw)
	}
	cases := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "schema lookup reference",
			body: envelope("unknown op member",
				v2model.Issue{Path: "/ops/0", Message: `unknown key "properties"`}.Hintf("%s serves the op's schema and example", v2model.RefGetOpSchema("set_properties"))),
			want: []string{"`get_op_schema` with op: set_properties serves the op's schema"},
		},
		{
			name: "list reference with bound arguments",
			body: envelope("no such property",
				v2model.Issue{Path: "/set/colour", Message: "unknown property"}.Hintf("list keys with %s", v2model.RefListProperties("space1"))),
			want: []string{"list keys with `list_properties` with space_id: space1"},
		},
		{
			name: "resend reference",
			body: envelope("option missing",
				v2model.Issue{Path: "/set/status", Message: "no option named Done"}.Hintf("or resend with %s", v2model.Resend("create_missing_options", "true"))),
			want: []string{"or resend with this call again with create_missing_options: true"},
		},
		{
			name: "legacy hint without references",
			body: envelope("no such property",
				v2model.Issue{Path: "/set/colour", Message: "unknown property", Hint: "list keys with GET /v2/spaces/space1/properties"}),
			want: []string{"list keys with the HTTP API"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ex, _ := newExecutorFixture(t, stubResponse{400, tc.body})
			_, err := ex.Run(context.Background(), "patch_object", map[string]any{"space_id": "s", "object_id": "o", "ops": []any{}})
			require.Error(t, err)
			var te *wrapper.ToolError
			require.ErrorAs(t, err, &te)
			assert.Equal(t, 400, te.Status)
			assert.Equal(t, v2model.CodeValidationFailed, te.Code)
			for _, w := range tc.want {
				assert.Contains(t, te.Text, w)
			}
			assert.NotRegexp(t, restRouteShape, te.Text, "nothing route-shaped survives")
			for _, issue := range te.Issues {
				assert.NotRegexp(t, restRouteShape, issue.Hint)
				assert.Nil(t, issue.SeeAlso, "references are rendered, not carried")
			}
		})
	}

	t.Run("a non-envelope body degrades to its text", func(t *testing.T) {
		ex, _ := newExecutorFixture(t, stubResponse{502, `gateway down`})
		_, err := ex.Run(context.Background(), "list_spaces", nil)
		var te *wrapper.ToolError
		require.ErrorAs(t, err, &te)
		assert.Equal(t, 502, te.Status)
		assert.Equal(t, "server answered 502: gateway down", te.Text)
	})

	t.Run("a rejected key keeps its status for the delivery to tip on", func(t *testing.T) {
		ex, _ := newExecutorFixture(t, stubResponse{401, `{"code":"unauthorized","message":"invalid authorization header format","status":401}`})
		_, err := ex.Run(context.Background(), "list_spaces", nil)
		var te *wrapper.ToolError
		require.ErrorAs(t, err, &te)
		assert.Equal(t, 401, te.Status)
	})
}

func TestExecutorPassesResultsThrough(t *testing.T) {
	body := `{"data":[{"id":"o1"}],"warnings":[{"message":"stored filter ignored","hint":"pass view to apply it"}]}`
	ex, _ := newExecutorFixture(t, stubResponse{200, body})
	result, err := ex.Run(context.Background(), "get_query_objects", map[string]any{"space_id": "s", "query_id": "q"})
	require.NoError(t, err)
	assert.Equal(t, body, result.Text, "the body is the text, warnings and hints included")
	raw, ok := result.JSON.(json.RawMessage)
	require.True(t, ok)
	assert.JSONEq(t, body, string(raw))
}

func TestSpelling(t *testing.T) {
	assert.Equal(t, "`list_spaces`", Spelling(v2model.RefListSpaces()))
	assert.Equal(t, "`get_object` with object_id: o, outline: true, space_id: s", Spelling(v2model.RefGetObject("s", "o").With("outline", "true")))
	assert.Equal(t, "this call again with dry_run: true", Spelling(v2model.Resend("dry_run", "true")))
}

// TestPathArgumentsCannotChangeTheRoute: gin routes on the decoded path, so
// an escaped slash in a path argument would land on another route —
// including the excluded stream and download routes. Every path argument
// is one segment.
func TestPathArgumentsCannotChangeTheRoute(t *testing.T) {
	cases := []struct {
		tool string
		args map[string]any
	}{
		{"get_space", map[string]any{"space_id": "s/chats/c/messages/stream"}},
		{"get_object", map[string]any{"space_id": "s", "object_id": "../files/f/content"}},
		{"list_chats", map[string]any{"space_id": "s/chats/stream"}},
		{"get_object", map[string]any{"space_id": "s", "object_id": ".."}},
		{"get_object", map[string]any{"space_id": "s", "object_id": "."}},
		{"get_object", map[string]any{"space_id": "s", "object_id": ""}},
	}
	for _, tc := range cases {
		ex, api := newExecutorFixture(t)
		_, err := ex.Run(context.Background(), tc.tool, tc.args)
		var ae wrapper.ArgumentError
		require.ErrorAs(t, err, &ae, "%s %v", tc.tool, tc.args)
		assert.Contains(t, err.Error(), "one path segment")
		assert.Empty(t, api.requests, "nothing reached the server for %v", tc.args)
	}
}

// TestIdempotencyKeyArgumentIsBounded: the executor applies the server's
// bound before sending.
func TestIdempotencyKeyArgumentIsBounded(t *testing.T) {
	for _, key := range []string{strings.Repeat("k", 256), "two words"} {
		ex, api := newExecutorFixture(t)
		_, err := ex.Run(context.Background(), "create_chat", map[string]any{"space_id": "s", "name": "x", "idempotency_key": key})
		var ae wrapper.ArgumentError
		require.ErrorAs(t, err, &ae)
		assert.Contains(t, err.Error(), "at most 255 visible ASCII characters")
		assert.Empty(t, api.requests)
	}
}

// TestRetryKeyIsAHeaderOnEveryWrite: an explicit retry key goes to the
// header and never into the body — on a tool whose route documents the
// header, one whose route does not, and an open-or-document body.
func TestRetryKeyIsAHeaderOnEveryWrite(t *testing.T) {
	cases := map[string]map[string]any{
		"patch_object":  {"space_id": "s", "object_id": "o", "ops": []any{map[string]any{"op": "delete_block", "id": "b1"}}},
		"create_object": {"space_id": "s", "type": "page", "name": "x"},
		"create_chat":   {"space_id": "s", "name": "x"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			ex, api := newExecutorFixture(t)
			args["idempotency_key"] = "retry-1"
			_, err := ex.Run(context.Background(), name, args)
			require.NoError(t, err)
			assert.Equal(t, "retry-1", api.requests[0].Header.Get("Idempotency-Key"))
			assert.NotContains(t, api.requests[0].Body, "idempotency")
		})
	}
}
