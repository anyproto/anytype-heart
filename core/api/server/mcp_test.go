package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
	"github.com/anyproto/anytype-heart/core/api/wrapper/full"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	testTableOnce sync.Once
	testTable     *full.Table
	testTableErr  error
)

// testFullTable derives the full table from the document on disk, the way
// the composition root does from the embedded bytes.
func testFullTable(t *testing.T) func() (*full.Table, error) {
	t.Helper()
	return func() (*full.Table, error) {
		testTableOnce.Do(func() {
			doc, err := os.ReadFile(filepath.Join("..", "docs", "v2", "openapi.json"))
			if err != nil {
				testTableErr = err
				return
			}
			served, err := v2service.ServedOpSchemas()
			if err != nil {
				testTableErr = err
				return
			}
			ops := make(map[string]full.OpSchema, len(served))
			for op, s := range served {
				ops[op] = full.OpSchema{Schema: s.Schema, Example: s.Example, Channels: s.Channels}
			}
			testTable, testTableErr = full.Derive(full.Inputs{OpenAPI: doc, Ops: ops})
		})
		return testTable, testTableErr
	}
}

// mcpFixture is a server with two full-scope keys.
func mcpFixture(t *testing.T) *fixture {
	t.Helper()
	fx := newV2ServerFixture(t)
	fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
	fx.KeyToToken = map[string]ApiSessionEntry{
		"keyA":       {Token: "tokA", AppName: "host A", Scope: model.AccountAuth_Full},
		"keyB":       {Token: "tokB", AppName: "host B", Scope: model.AccountAuth_JsonAPI},
		"limitedKey": {Token: "tokL", AppName: "clipper", Scope: model.AccountAuth_Limited},
	}
	return fx
}

type mcpCall struct {
	method  string
	tier    string
	key     string
	session string
	headers map[string]string
	body    string
}

func (fx *fixture) mcpDo(t *testing.T, call mcpCall) *httptest.ResponseRecorder {
	t.Helper()
	if call.method == "" {
		call.method = http.MethodPost
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(call.method, "/mcp/"+call.tier, strings.NewReader(call.body))
	req.Host = localApiHost
	req.Header.Set("Content-Type", "application/json")
	if call.key != "" {
		req.Header.Set("Authorization", "Bearer "+call.key)
	}
	if call.session != "" {
		req.Header.Set(mcpSessionHeader, call.session)
	}
	for k, v := range call.headers {
		req.Header.Set(k, v)
	}
	fx.Engine().ServeHTTP(w, req)
	return w
}

func rpcLine(t *testing.T, id any, method string, params any) string {
	t.Helper()
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if id != nil {
		msg["id"] = id
	}
	if params != nil {
		msg["params"] = params
	}
	raw, err := json.Marshal(msg)
	require.NoError(t, err)
	return string(raw)
}

func toolCall(t *testing.T, id int, name string, args map[string]any) string {
	return rpcLine(t, id, "tools/call", map[string]any{"name": name, "arguments": args})
}

func rpcResult(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Nil(t, resp["error"], "JSON-RPC error: %v", resp["error"])
	result, ok := resp["result"].(map[string]any)
	require.True(t, ok, w.Body.String())
	return result
}

func TestMCPRouteGates(t *testing.T) {
	fx := mcpFixture(t)
	init := rpcLine(t, 1, "initialize", map[string]any{"protocolVersion": "2025-06-18"})

	t.Run("no key is 401 at the HTTP layer, never in-band", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "full", body: init})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.NotContains(t, w.Body.String(), "jsonrpc")
	})
	t.Run("a Limited key is refused by the scope gate", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "full", key: "limitedKey", body: init})
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
	t.Run("unknown tier", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "huge", key: "keyA", body: init})
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "small, large, full")
	})
	t.Run("GET is 405 with Allow", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{method: http.MethodGet, tier: "full", key: "keyA"})
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Equal(t, "POST, DELETE", w.Header().Get("Allow"))
	})
	t.Run("an unsupported protocol version header is 400", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "full", key: "keyA", body: init, headers: map[string]string{mcpProtocolHeader: "2024-11-05"}})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "2025-06-18, 2025-11-25")
		w = fx.mcpDo(t, mcpCall{tier: "full", key: "keyA", body: init, headers: map[string]string{mcpProtocolHeader: "2025-11-25"}})
		assert.Equal(t, http.StatusOK, w.Code)
	})
	t.Run("a batch is refused", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "full", key: "keyA", body: "[" + init + "]"})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), `"code":-32600`)
	})
	t.Run("a notification is 202 with no body and runs nothing", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "full", key: "keyA", body: rpcLine(t, nil, "notifications/initialized", nil)})
		assert.Equal(t, http.StatusAccepted, w.Code)
		assert.Empty(t, w.Body.String())
		w = fx.mcpDo(t, mcpCall{tier: "full", key: "keyA", body: rpcLine(t, nil, "tools/call", map[string]any{"name": "list_spaces"})})
		assert.Equal(t, http.StatusAccepted, w.Code, "an id-less call is a notification")
	})
	t.Run("a body past the limit is 413", func(t *testing.T) {
		big := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"pad":"` + strings.Repeat("x", 9<<20) + `"}}`
		w := fx.mcpDo(t, mcpCall{tier: "full", key: "keyA", body: big})
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	})
}

func TestMCPFullTier(t *testing.T) {
	fx := mcpFixture(t)

	t.Run("initialize issues no session and serves the full instructions", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "full", key: "keyA", body: rpcLine(t, 1, "initialize", map[string]any{"protocolVersion": "2025-06-18"})})
		result := rpcResult(t, w)
		assert.Empty(t, w.Header().Get(mcpSessionHeader))
		assert.Equal(t, "2025-06-18", result["protocolVersion"])
		assert.Contains(t, result["instructions"], "op envelopes")
	})
	t.Run("tools/list is the derived table", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "full", key: "keyA", body: rpcLine(t, 2, "tools/list", nil)})
		result := rpcResult(t, w)
		table, err := testFullTable(t)()
		require.NoError(t, err)
		assert.Len(t, result["tools"], len(table.Tools))
	})
	t.Run("a call runs as the caller and answers with structured content", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "full", key: "keyA", body: toolCall(t, 3, "list_spaces", map[string]any{"limit": 5})})
		result := rpcResult(t, w)
		assert.Nil(t, result["isError"], "the inner /v2 call was authenticated as the caller: %v", result)
		require.NotNil(t, result["structuredContent"])
		content := result["content"].([]any)[0].(map[string]any)
		assert.Contains(t, content["text"], `"data"`)
	})
	t.Run("a refusal is in-band", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "full", key: "keyA", body: toolCall(t, 4, "get_object", map[string]any{"space_id": "s"})})
		result := rpcResult(t, w)
		assert.Equal(t, true, result["isError"])
		content := result["content"].([]any)[0].(map[string]any)
		assert.Contains(t, content["text"], `get_object needs "object_id"`)
	})
	t.Run("DELETE has nothing to end", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{method: http.MethodDelete, tier: "full", key: "keyA", session: "x"})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestMCPCuratedSessions(t *testing.T) {
	fx := mcpFixture(t)
	init := rpcLine(t, 1, "initialize", map[string]any{"protocolVersion": "2025-06-18"})
	list := rpcLine(t, 2, "tools/list", nil)

	t.Run("a request before initialize is 400", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "small", key: "keyA", body: list})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), mcpSessionHeader)
	})

	w := fx.mcpDo(t, mcpCall{tier: "small", key: "keyA", body: init})
	result := rpcResult(t, w)
	sid := w.Header().Get(mcpSessionHeader)
	require.Len(t, sid, 32, "a 128-bit id")
	assert.Contains(t, result["instructions"], "find")

	t.Run("the session serves its tier", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "small", key: "keyA", session: sid, body: list})
		result := rpcResult(t, w)
		assert.Len(t, result["tools"], 8)
	})
	t.Run("a call runs as the caller", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "small", key: "keyA", session: sid, body: toolCall(t, 3, "spaces", nil)})
		result := rpcResult(t, w)
		assert.Nil(t, result["isError"], "%v", result)
	})
	t.Run("another key cannot use it", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "small", key: "keyB", session: sid, body: list})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
	t.Run("another tier cannot use it", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "large", key: "keyA", session: sid, body: list})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
	t.Run("an unknown id is 404", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "small", key: "keyA", session: strings.Repeat("0", 32), body: list})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
	t.Run("DELETE ends it, by its owner only", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{method: http.MethodDelete, tier: "small", key: "keyB", session: sid})
		assert.Equal(t, http.StatusNotFound, w.Code)
		w = fx.mcpDo(t, mcpCall{method: http.MethodDelete, tier: "small", key: "keyA"})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		w = fx.mcpDo(t, mcpCall{method: http.MethodDelete, tier: "small", key: "keyA", session: sid})
		assert.Equal(t, http.StatusNoContent, w.Code)
		w = fx.mcpDo(t, mcpCall{tier: "small", key: "keyA", session: sid, body: list})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
	t.Run("an idle session expires", func(t *testing.T) {
		w := fx.mcpDo(t, mcpCall{tier: "large", key: "keyA", body: init})
		expiring := w.Header().Get(mcpSessionHeader)
		require.NotEmpty(t, expiring)
		fx.Server.mcp.now = func() time.Time { return time.Now().Add(mcpSessionIdle + time.Minute) }
		defer func() { fx.Server.mcp.now = time.Now }()
		w = fx.mcpDo(t, mcpCall{tier: "large", key: "keyA", session: expiring, body: list})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
	t.Run("caps refuse new sessions with 429", func(t *testing.T) {
		fx.Server.mcp.maxPerKey = 1
		defer func() { fx.Server.mcp.maxPerKey = mcpMaxSessionsPerKey }()
		w := fx.mcpDo(t, mcpCall{tier: "small", key: "keyB", body: init})
		require.Equal(t, http.StatusOK, w.Code)
		w = fx.mcpDo(t, mcpCall{tier: "small", key: "keyB", body: init})
		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		fx.Server.mcp.maxTotal = 0
		defer func() { fx.Server.mcp.maxTotal = mcpMaxSessions }()
		w = fx.mcpDo(t, mcpCall{tier: "small", key: "keyA", body: init})
		assert.Equal(t, http.StatusTooManyRequests, w.Code)
	})
}

// TestMCPInnerCallsCarryTheCallersKey is the security property of §3.3:
// the inner /v2 call is the caller's own. A key without the JSON-API scope
// cannot reach /mcp at all (the gate), and a valid key's call succeeds
// only because the transport passed that key through. Forcing a different
// bearer in the transport fails this test and the transport's own.
func TestMCPInnerCallsCarryTheCallersKey(t *testing.T) {
	fx := mcpFixture(t)
	w := fx.mcpDo(t, mcpCall{tier: "full", key: "keyB", body: toolCall(t, 1, "list_spaces", nil)})
	result := rpcResult(t, w)
	assert.Nil(t, result["isError"], "%v", result)

	// the same request through a client that carries no key at all is
	// refused inside: proof the outer key is what authenticated the call
	client := fx.inProcessClient("")
	resp, err := client.HTTP.Get(InProcessBaseURL + "/v2/spaces")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
