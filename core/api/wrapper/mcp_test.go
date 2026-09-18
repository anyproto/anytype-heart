package wrapper

// mcp_test.go — the MCP delivery (§8.20): wire framing, tier-filtered
// tools/list, and — most importantly — the repair loop: a table of
// realistic wrong calls asserting the EXACT tip the model gets back, then
// the corrected call succeeding in the same session.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// rpc renders one JSON-RPC request line.
func rpc(t *testing.T, id any, method string, params any) string {
	t.Helper()
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if id != nil {
		msg["id"] = id
	}
	if params != nil {
		msg["params"] = params
	}
	data, err := json.Marshal(msg)
	require.NoError(t, err)
	return string(data)
}

// call renders a tools/call request line.
func call(t *testing.T, id int, tool string, args map[string]any) string {
	params := map[string]any{"name": tool}
	if args != nil {
		params["arguments"] = args
	}
	return rpc(t, id, "tools/call", params)
}

// mcpSession drives scripted lines through one server over the fixture's
// runner and returns the decoded response lines in order.
func (fx *fixture) mcpSession(t *testing.T, tier Tier, lines ...string) []map[string]any {
	t.Helper()
	server := NewMCPServer(fx.Runner, tier)
	var out strings.Builder
	err := server.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out)
	require.NoError(t, err)
	var responses []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m), "response line: %s", line)
		responses = append(responses, m)
	}
	return responses
}

// callText extracts (text, isError) from a tools/call response.
func callText(t *testing.T, resp map[string]any) (string, bool) {
	t.Helper()
	require.Nil(t, resp["error"], "expected a result, got JSON-RPC error: %v", resp["error"])
	result, ok := resp["result"].(map[string]any)
	require.True(t, ok, "no result in %v", resp)
	content, ok := result["content"].([]any)
	require.True(t, ok, "no content in %v", result)
	require.Len(t, content, 1)
	block := content[0].(map[string]any)
	assert.Equal(t, "text", block["type"])
	isErr, _ := result["isError"].(bool)
	return block["text"].(string), isErr
}

func TestMCPInitialize(t *testing.T) {
	t.Run("echoes a known protocol version", func(t *testing.T) {
		fx := newFixture(t)
		resps := fx.mcpSession(t, TierLarge,
			rpc(t, 1, "initialize", map[string]any{"protocolVersion": "2024-11-05"}))
		require.Len(t, resps, 1)
		result := resps[0]["result"].(map[string]any)
		assert.Equal(t, "2024-11-05", result["protocolVersion"])
		info := result["serverInfo"].(map[string]any)
		assert.Equal(t, "anytype", info["name"])
		caps := result["capabilities"].(map[string]any)
		assert.Contains(t, caps, "tools")
	})

	t.Run("answers an unknown version with the latest supported", func(t *testing.T) {
		fx := newFixture(t)
		resps := fx.mcpSession(t, TierLarge,
			rpc(t, 1, "initialize", map[string]any{"protocolVersion": "1999-01-01"}))
		result := resps[0]["result"].(map[string]any)
		assert.Equal(t, mcpLatestVersion, result["protocolVersion"])
	})

	t.Run("instructions steer the workflow, tier-aware", func(t *testing.T) {
		fx := newFixture(t)
		for _, tc := range []struct {
			tier      Tier
			wantTable bool
		}{{TierSmall, false}, {TierLarge, true}} {
			resps := fx.mcpSession(t, tc.tier, rpc(t, 1, "initialize", nil))
			result := resps[0]["result"].(map[string]any)
			instructions := result["instructions"].(string)
			assert.Contains(t, instructions, "find")
			assert.Contains(t, instructions, "describe a type BEFORE create")
			assert.Contains(t, instructions, "retry once")
			assert.Contains(t, instructions, "edit_text alone can skip it",
				"the instructions must not steer the model back into read-before-edit_text (§8.21)")
			assert.Equal(t, tc.wantTable, strings.Contains(instructions, "set_cell's row and col"),
				"the set_cell steering follows the tool into its tier (%s)", tc.tier)
		}
	})
}

func TestMCPToolsList(t *testing.T) {
	for _, tier := range []Tier{TierSmall, TierLarge} {
		t.Run(string(tier), func(t *testing.T) {
			fx := newFixture(t)
			resps := fx.mcpSession(t, tier, rpc(t, 1, "tools/list", nil))
			require.Len(t, resps, 1)
			result := resps[0]["result"].(map[string]any)
			toolList := result["tools"].([]any)
			var names []string
			for _, raw := range toolList {
				entry := raw.(map[string]any)
				names = append(names, entry["name"].(string))
				assert.NotEmpty(t, entry["description"])
				schema := entry["inputSchema"].(map[string]any)
				assert.Equal(t, "object", schema["type"], "inputSchema is the served C13 schema")
			}
			assert.Equal(t, ToolNamesForTier(tier), names)
		})
	}

	t.Run("read-only annotations", func(t *testing.T) {
		fx := newFixture(t)
		resps := fx.mcpSession(t, TierLarge, rpc(t, 1, "tools/list", nil))
		result := resps[0]["result"].(map[string]any)
		for _, raw := range result["tools"].([]any) {
			entry := raw.(map[string]any)
			name := entry["name"].(string)
			readOnly := map[string]bool{"spaces": true, "find": true, "read": true, "describe": true}[name]
			if readOnly {
				ann, ok := entry["annotations"].(map[string]any)
				require.True(t, ok, "%s must carry annotations", name)
				assert.Equal(t, true, ann["readOnlyHint"])
			} else {
				assert.Nil(t, entry["annotations"], "%s is a write tool — no readOnlyHint", name)
			}
		}
	})
}

func TestMCPToolsCall(t *testing.T) {
	t.Run("success returns the text channel", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("GET /v2/spaces", 200, `{"data":[{"id":"bafyspace1","name":"Work"}],"total":1,"offset":0,"limit":25,"has_more":false}`)
		resps := fx.mcpSession(t, TierSmall, call(t, 1, "spaces", nil))
		text, isErr := callText(t, resps[0])
		assert.False(t, isErr)
		assert.Contains(t, text, "Work — bafyspace1")
	})

	t.Run("a tool outside the tier is a protocol error listing the tier's tools", func(t *testing.T) {
		fx := newFixture(t)
		resps := fx.mcpSession(t, TierSmall, call(t, 1, "delete_block", map[string]any{"object": "1", "block": "e0002"}))
		rpcErr := resps[0]["error"].(map[string]any)
		assert.Equal(t, float64(mcpInvalidParams), rpcErr["code"])
		msg := rpcErr["message"].(string)
		assert.Contains(t, msg, `unknown tool "delete_block"`)
		assert.Contains(t, msg, "spaces, find, read, describe, create, set_properties, add_blocks, edit_text")
		assert.NotContains(t, msg, "delete_block,", "the small tier never advertises large-tier tools")
	})

	t.Run("the same tool exists on the large tier", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false, v2model.ObjectRow{Id: "bafyobj1", Name: "Doc", Type: "task"}))
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		resps := fx.mcpSession(t, TierLarge,
			call(t, 1, "find", map[string]any{"space": "space1", "query": "doc"}),
			call(t, 2, "delete_block", map[string]any{"object": "1", "block": "e0002"}))
		text, isErr := callText(t, resps[1])
		assert.False(t, isErr)
		assert.Contains(t, text, `ok — "Doc": 1 changed`)
	})
}

// TestMCPRepairLoop is the §8.20 loop test: each case scripts a realistic
// WRONG call, asserts the exact tip that comes back in-band (isError), then
// runs the corrected call in the same session and asserts it succeeds. If a
// tip regresses into a raw error, this fails.
func TestMCPRepairLoop(t *testing.T) {
	t.Run("handle before find → run find first", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false, v2model.ObjectRow{Id: "bafyobj1", Name: "Groceries", Type: "page"}))
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, testFullDoc)
		resps := fx.mcpSession(t, TierSmall,
			call(t, 1, "read", map[string]any{"object": "1"}),
			call(t, 2, "find", map[string]any{"space": "space1", "query": "groceries"}),
			call(t, 3, "read", map[string]any{"object": "1"}))

		tip, isErr := callText(t, resps[0])
		assert.True(t, isErr)
		assert.Equal(t, "no working session for handle 1 — run find first: it numbers the results (1, 2, …) and sets the working space", tip)

		text, isErr := callText(t, resps[2])
		assert.False(t, isErr)
		assert.Contains(t, text, `"e0002"`, "the corrected read returns the relabeled document")
	})

	t.Run("missing required argument → the arg and its meaning", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false, v2model.ObjectRow{Id: "bafyobj1", Name: "Doc", Type: "task"}))
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		resps := fx.mcpSession(t, TierSmall,
			call(t, 1, "find", map[string]any{"space": "space1", "query": "doc"}),
			call(t, 2, "edit_text", map[string]any{"object": "1", "block": "e0002", "find": "body"}),
			call(t, 3, "edit_text", map[string]any{"object": "1", "block": "e0002", "find": "body", "replace": "the body"}))

		tip, isErr := callText(t, resps[1])
		assert.True(t, isErr)
		assert.Equal(t, `edit_text needs "replace" — the new text — empty deletes the found text`, tip)

		text, isErr := callText(t, resps[2])
		assert.False(t, isErr)
		assert.Contains(t, text, `ok — "Doc": 1 changed`)
	})

	t.Run("after and under together → pick one", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false, v2model.ObjectRow{Id: "bafyobj1", Name: "Doc", Type: "task"}))
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		resps := fx.mcpSession(t, TierSmall,
			call(t, 1, "find", map[string]any{"space": "space1", "query": "doc"}),
			call(t, 2, "add_blocks", map[string]any{"object": "1", "markdown": "- [ ] x", "after": "e0001", "under": "e0002"}),
			call(t, 3, "add_blocks", map[string]any{"object": "1", "markdown": "- [ ] x", "after": "e0001"}))

		tip, isErr := callText(t, resps[1])
		assert.True(t, isErr)
		assert.Equal(t, "give after or under, not both — after inserts next to the block, under inserts into it", tip)

		_, isErr = callText(t, resps[2])
		assert.False(t, isErr)
	})

	t.Run("bad enum value → the allowed values", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false, v2model.ObjectRow{Id: "bafyobj1", Name: "Doc", Type: "task"}))
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, `{"outline":[{"indent":0,"id":"e0001","type":"heading_1","text":"Section"}]}`)
		resps := fx.mcpSession(t, TierSmall,
			call(t, 1, "find", map[string]any{"space": "space1", "query": "doc"}),
			call(t, 2, "read", map[string]any{"object": "1", "mode": "tree"}),
			call(t, 3, "read", map[string]any{"object": "1", "mode": "outline"}))

		tip, isErr := callText(t, resps[1])
		assert.True(t, isErr)
		assert.Equal(t, `read: "mode" must be one of full, outline`, tip)

		_, isErr = callText(t, resps[2])
		assert.False(t, isErr)
	})

	t.Run("server C6 error arrives in tool vocabulary with the hint", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false, v2model.ObjectRow{Id: "bafyobj1", Name: "Doc", Type: "task"}))
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 400,
			`{"status":400,"code":"validation_failed","message":"validation failed","issues":[{"path":"ops[0].inside","message":"block \"zzzzz\" not found","hint":"GET the object with ?outline=true to list them"}]}`)
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, testFullDoc)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		resps := fx.mcpSession(t, TierLarge,
			call(t, 1, "find", map[string]any{"space": "space1", "query": "doc"}),
			call(t, 2, "move_block", map[string]any{"object": "1", "block": "e0002", "under": "zzzzz"}),
			call(t, 3, "read", map[string]any{"object": "1"}),
			call(t, 4, "move_block", map[string]any{"object": "1", "block": "e0002", "under": "e0001"}))

		tip, isErr := callText(t, resps[1])
		assert.True(t, isErr)
		assert.Contains(t, tip, "under: block \"zzzzz\" not found")
		assert.Contains(t, tip, "run read (the default mode=full) to see each block's text")
		assert.NotContains(t, tip, "ops[0]", "the op-path vocabulary must not reach the model")
		assert.NotContains(t, tip, "inside", "the REST op vocabulary must not reach the model")

		text, isErr := callText(t, resps[3])
		assert.False(t, isErr)
		assert.Contains(t, text, `ok — "Doc": 1 changed`)
	})

	t.Run("rejected API key → ask the user, not a retry", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("GET /v2/spaces", 401, `{"status":401,"code":"unauthorized","message":"invalid api key"}`)
		resps := fx.mcpSession(t, TierSmall, call(t, 1, "spaces", nil))
		tip, isErr := callText(t, resps[0])
		assert.True(t, isErr)
		assert.Contains(t, tip, "invalid api key")
		assert.Contains(t, tip, "ask the user to check ANYTYPE_API_KEY")
		assert.Contains(t, tip, "no change to the call will help")
	})

	t.Run("API server unreachable → ask the user to start the app", func(t *testing.T) {
		client := NewClient("http://127.0.0.1:1", "key")
		client.Backoff = func(int) time.Duration { return 0 }
		runner := NewRunner(client, NewMemoryStore())
		server := NewMCPServer(runner, TierSmall)
		var out strings.Builder
		require.NoError(t, server.Serve(context.Background(),
			strings.NewReader(call(t, 1, "spaces", nil)+"\n"), &out))
		var resp map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(out.String())), &resp))
		tip, isErr := callText(t, resp)
		assert.True(t, isErr)
		assert.Contains(t, tip, "cannot reach the local Anytype API at http://127.0.0.1:1")
		assert.Contains(t, tip, "ask the user to start the Anytype app")
	})
}

func TestMCPFraming(t *testing.T) {
	t.Run("notifications get no response", func(t *testing.T) {
		fx := newFixture(t)
		resps := fx.mcpSession(t, TierLarge,
			rpc(t, 1, "initialize", nil),
			rpc(t, nil, "notifications/initialized", nil),
			rpc(t, 2, "ping", nil))
		require.Len(t, resps, 2, "the notification must not be answered")
		assert.Equal(t, float64(1), resps[0]["id"])
		assert.Equal(t, float64(2), resps[1]["id"])
		assert.NotNil(t, resps[1]["result"], "ping answers an empty result")
	})

	t.Run("unknown method → -32601", func(t *testing.T) {
		fx := newFixture(t)
		resps := fx.mcpSession(t, TierLarge, rpc(t, 7, "resources/list", nil))
		rpcErr := resps[0]["error"].(map[string]any)
		assert.Equal(t, float64(mcpMethodNotFound), rpcErr["code"])
	})

	t.Run("parse error → -32700 with null id", func(t *testing.T) {
		fx := newFixture(t)
		resps := fx.mcpSession(t, TierLarge, `{oops`)
		rpcErr := resps[0]["error"].(map[string]any)
		assert.Equal(t, float64(mcpParseError), rpcErr["code"])
		assert.Nil(t, resps[0]["id"])
	})

	t.Run("batch → -32600", func(t *testing.T) {
		fx := newFixture(t)
		resps := fx.mcpSession(t, TierLarge, `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`)
		rpcErr := resps[0]["error"].(map[string]any)
		assert.Equal(t, float64(mcpInvalidRequest), rpcErr["code"])
		assert.Contains(t, rpcErr["message"], "batching is not supported")
	})

	t.Run("string ids round-trip", func(t *testing.T) {
		fx := newFixture(t)
		resps := fx.mcpSession(t, TierLarge, rpc(t, "req-1", "ping", nil))
		assert.Equal(t, "req-1", resps[0]["id"])
	})

	t.Run("responses are one line each", func(t *testing.T) {
		fx := newFixture(t)
		server := NewMCPServer(fx.Runner, TierLarge)
		var out strings.Builder
		require.NoError(t, server.Serve(context.Background(),
			strings.NewReader(rpc(t, 1, "tools/list", nil)+"\n"), &out))
		raw := out.String()
		assert.Equal(t, 1, strings.Count(raw, "\n"), "one message, one newline-terminated line")
		assert.True(t, strings.HasSuffix(raw, "\n"))
	})
}

// TestMCPSessionStateSpansCalls pins the long-lived state story (§7.4): the
// MCP delivery holds handle state in memory, so a find in one call resolves
// handles in the next — unlike the CLI, no file is involved.
func TestMCPSessionStateSpansCalls(t *testing.T) {
	fx := newFixture(t)
	fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(2, false,
		v2model.ObjectRow{Id: "bafyobj1", Name: "First", Type: "page"},
		v2model.ObjectRow{Id: "bafyobj2", Name: "Second", Type: "page"}))
	fx.stub("GET /v2/spaces/space1/objects/bafyobj2", 200, testFullDoc)
	resps := fx.mcpSession(t, TierSmall,
		call(t, 1, "find", map[string]any{"space": "space1", "query": "doc"}),
		call(t, 2, "read", map[string]any{"object": "2"}))
	require.Len(t, resps, 2)
	_, isErr := callText(t, resps[1])
	assert.False(t, isErr)
	sent := fx.sent("GET /v2/spaces/space1/objects/bafyobj2")
	require.Len(t, sent, 1, "handle 2 resolved to the second find result")
}
