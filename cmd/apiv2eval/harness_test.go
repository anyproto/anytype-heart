package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/wrapper"
)

// stubAPI is a minimal stand-in for the local /v2 server: enough of the
// surface for one read→edit loop, with a scripted refusal so the harness's
// error capture can be tested without a live app. It is NOT a model of the
// API — the real runs use the real server; this only exercises the harness.
type stubAPI struct {
	mu      sync.Mutex
	patches []json.RawMessage
	// refuseFirstPatch answers the first PATCH with a C6 400 naming a path.
	refuseFirstPatch bool
	doc              string
	// patchQueue scripts PATCH answers in order (e.g. the server's locator
	// refusals, §8.43); when empty, the editOK default answers.
	patchQueue []stubResponse
}

// stubResponse is one scripted HTTP answer.
type stubResponse struct {
	status int
	body   string
}

func newStubAPI(doc string) *httptest.Server {
	s := &stubAPI{doc: doc}
	return httptest.NewServer(s)
}

func (s *stubAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/v2/auth/whoami":
		fmt.Fprint(w, `{"grant":{"scoped":false}}`)
	case strings.HasSuffix(r.URL.Path, "/search"):
		fmt.Fprint(w, `{"data":[{"id":"obj1","name":"Quarterly plan ab12","type":"page"}],"total":1,"has_more":false}`)
	case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/objects/"):
		fmt.Fprint(w, s.doc)
	case r.Method == http.MethodPatch:
		var body struct {
			Ops []json.RawMessage `json:"ops"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		first := len(s.patches) == 0
		s.patches = append(s.patches, body.Ops...)
		var scripted *stubResponse
		if len(s.patchQueue) > 0 {
			scripted, s.patchQueue = &s.patchQueue[0], s.patchQueue[1:]
		}
		s.mu.Unlock()
		if scripted != nil {
			w.WriteHeader(scripted.status)
			fmt.Fprint(w, scripted.body)
			return
		}
		if first && s.refuseFirstPatch {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"status":400,"code":"invalid_input","message":"block \"zz\" not found",`+
				`"issues":[{"path":"ops[0].id","message":"no block matches \"zz\"","hint":"GET the object with ?outline=true to list them"}]}`)
			return
		}
		fmt.Fprint(w, `{"etag":"e1","diff_stats":{"blocks_changed":1}}`)
	case strings.HasPrefix(r.URL.Path, "/v2/schemas/ops/"):
		op := strings.TrimPrefix(r.URL.Path, "/v2/schemas/ops/")
		fmt.Fprintf(w, `{"kind":%q,"endpoint":"PATCH","schema":{"type":"object","x-op":%q},"example":{"ops":[]}}`, op, op)
	default:
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"status":404,"code":"not_found","message":"stub has no route for `+r.URL.Path+`"}`)
	}
}

// scriptedModel serves the OpenAI chat-completions shape, replaying one
// scripted assistant message per call.
type scriptedModel struct {
	mu    sync.Mutex
	turns []string // raw JSON for choices[0].message
	seen  []map[string]any
}

func newScriptedModel(turns ...string) (*httptest.Server, *scriptedModel) {
	m := &scriptedModel{turns: turns}
	return httptest.NewServer(m), m
}

func (m *scriptedModel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if strings.HasSuffix(r.URL.Path, "/models") {
		fmt.Fprint(w, `{"data":[{"id":"stub"}]}`)
		return
	}
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	m.mu.Lock()
	m.seen = append(m.seen, req)
	var message string
	if len(m.turns) > 0 {
		message, m.turns = m.turns[0], m.turns[1:]
	} else {
		message = `{"role":"assistant","content":"done"}`
	}
	m.mu.Unlock()
	fmt.Fprintf(w, `{"choices":[{"index":0,"message":%s,"finish_reason":"stop"}],`+
		`"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`, message)
}

func toolCallTurn(name, args string) string {
	encoded, err := json.Marshal(args)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf(`{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function",`+
		`"function":{"name":%q,"arguments":%s}}]}`, name, encoded)
}

func TestWrapperArmDrivesTheProductMCPServer(t *testing.T) {
	// given
	api := newStubAPI(servedDoc)
	defer api.Close()
	rec := &recorder{}
	client := wrapper.NewClient(api.URL, "key")
	client.HTTP = &http.Client{Transport: &recordingTransport{base: http.DefaultTransport, rec: rec}}
	runner := wrapper.NewRunner(client, wrapper.NewMemoryStore())

	// when
	ts, err := newMCPToolset(context.Background(), runner, wrapper.TierSmall)

	// then
	require.NoError(t, err)
	defer ts.close()

	names := make([]string, 0, len(ts.tools()))
	for _, spec := range ts.tools() {
		names = append(names, spec.Name)
		assert.NotEmpty(t, spec.Parameters, "tool %s has no schema", spec.Name)
	}
	assert.Equal(t, wrapper.ToolNamesForTier(wrapper.TierSmall), names,
		"the arm must serve the tier's published set, not a set the harness wrote")
	assert.Contains(t, ts.instructions(), "find", "the system prompt is the product's own instructions")

	out := ts.call(context.Background(), "find", map[string]any{"space": "space1", "query": "Quarterly"})
	assert.False(t, out.IsError)
	assert.Contains(t, out.Text, "1. Quarterly plan ab12")

	out = ts.call(context.Background(), "nope", map[string]any{})
	assert.True(t, out.IsError)
	assert.Contains(t, out.Text, "unknown tool")
}

// The locator refusals are produced by the SERVER now (§8.43 —
// v2/service/locator.go, re-spelled into the tool register by the
// wrapper's opsVocab/restVocab), so this drives the real wrapper over
// stubbed server refusals whose bodies mirror the server's wire shape.
// The classifier phrases must match the text a model actually sees; the
// end-to-end wording pins live where each half is produced —
// v2/service/edit_test.go for the server texts, wrapper's
// tools_smallmodel_test.go for the translation.
func TestSnippetRefusalsAreCountedFromTheProductsOwnText(t *testing.T) {
	// given — the server's two locator refusals, as locator.go serves them
	// for a document where "Q3" is in two blocks and "Q9" in none
	const ambiguousBody = `{"status":400,"code":"ambiguous_input",` +
		`"message":"\"Q3\" appears in 2 blocks — retry with id naming one of:\n  block a1b2c (paragraph): \"Revenue target for Q3 is 1.2M.\"\n  block d4e5f (paragraph): \"Q3 review is due.\"",` +
		`"issues":[{"path":"ops[0].find","message":"the find text appears in 2 blocks — a locator must identify exactly one","hint":"add surrounding text to find until it appears in one block only, or give the block id"}]}`
	const noMatchBody = `{"status":404,"code":"not_found",` +
		`"message":"no block contains \"Q9\" — copy the find text exactly, including inline markup (text is markdown source: ** [ ] etc. count)",` +
		`"issues":[{"path":"ops[0].find","message":"the find text must appear in exactly one block for the locator to resolve","hint":"GET the object with ?outline=true to list them, then copy the text exactly as a read serves it — or give the block id"}]}`
	stub := &stubAPI{doc: servedDoc, patchQueue: []stubResponse{
		{status: http.StatusBadRequest, body: ambiguousBody},
		{status: http.StatusNotFound, body: noMatchBody},
	}}
	client := wrapper.NewClient("http://stub", "key")
	client.HTTP = &http.Client{Transport: &stubTransport{handler: stub}}
	ts, err := newMCPToolset(context.Background(), wrapper.NewRunner(client, wrapper.NewMemoryStore()),
		wrapper.TierSmall)
	require.NoError(t, err)
	defer ts.close()
	_ = ts.call(context.Background(), "find", map[string]any{"space": "space1", "query": "Quarterly"})

	// when
	ambiguous := ts.call(context.Background(), "edit_text",
		map[string]any{"object": "1", "find": "Q3", "replace": "Q4"})
	missing := ts.call(context.Background(), "edit_text",
		map[string]any{"object": "1", "find": "Q9", "replace": "Q4"})

	// then
	require.True(t, ambiguous.IsError)
	require.True(t, missing.IsError)
	got := analyze([]callRecord{
		{Turn: 0, Tool: "edit_text", Args: json.RawMessage(`{"object":"1","find":"Q3","replace":"Q4"}`),
			ResultText: ambiguous.Text, IsError: true},
		{Turn: 1, Tool: "edit_text", Args: json.RawMessage(`{"object":"1","find":"Q9","replace":"Q4"}`),
			ResultText: missing.Text, IsError: true},
	})
	assert.Equal(t, 1, got.SnippetAmbiguous, "refusal was: %s", ambiguous.Text)
	assert.Equal(t, 1, got.SnippetNoMatch, "refusal was: %s", missing.Text)
	assert.Equal(t, 2, got.EditTextCalls)
	assert.Equal(t, 0, got.EditTextWithBlock)
}

func TestAgentLoopRecordsCallsErrorsAndTokens(t *testing.T) {
	// given
	stub := &stubAPI{doc: servedDoc, refuseFirstPatch: true}
	api := httptest.NewServer(stub)
	defer api.Close()
	model, script := newScriptedModel(
		toolCallTurn("find", `{"space":"space1","query":"Quarterly plan"}`),
		toolCallTurn("edit_text", `{"object":"1","find":"Q3","replace":"Q4","block":"zz"}`),
		toolCallTurn("edit_text", `{"object":"1","find":"Q3","replace":"Q4","block":"d4e5f"}`),
		`{"role":"assistant","content":"Changed Q3 to Q4."}`,
	)
	defer model.Close()

	rec := &recorder{}
	client := wrapper.NewClient(api.URL, "key")
	client.HTTP = &http.Client{Transport: &recordingTransport{base: http.DefaultTransport, rec: rec}}
	client.Backoff = func(int) time.Duration { return 0 }
	runner := wrapper.NewRunner(client, wrapper.NewMemoryStore())
	ts, err := newMCPToolset(context.Background(), runner, wrapper.TierSmall)
	require.NoError(t, err)
	defer ts.close()

	// when
	tr, err := runAgent(context.Background(), agentConfig{
		chat:     newChatClient(model.URL, "", 30*time.Second),
		model:    "stub",
		maxTurns: 6,
		rec:      rec,
	}, ts, ts.instructions(), "change Q3 to Q4")

	// then
	require.NoError(t, err)
	assert.Equal(t, "model_done", tr.StoppedBy)
	assert.Equal(t, "Changed Q3 to Q4.", tr.FinalContent)
	require.Len(t, tr.Calls, 3)
	assert.Equal(t, 40, tr.PromptTokens)
	assert.Equal(t, 20, tr.CompletionTokens)

	refused := tr.Calls[1]
	assert.True(t, refused.IsError)
	require.NotEmpty(t, refused.Exchanges, "the refusal must carry its structured facts")
	last := refused.Exchanges[len(refused.Exchanges)-1]
	assert.Equal(t, 400, last.Status)
	assert.Equal(t, "invalid_input", last.Code)
	require.Len(t, last.Issues, 1)
	assert.Equal(t, "ops[0].id", last.Issues[0].Path)
	assert.Contains(t, refused.ResultText, `block "zz" not found`,
		"the model sees the server's own text, translated to the tool vocabulary")
	assert.NotContains(t, refused.ResultText, "ops[0]", "the op vocabulary must not leak to the model")

	// the tool definitions the model was handed came from the product
	require.NotEmpty(t, script.seen)
	tools, _ := json.Marshal(script.seen[0]["tools"])
	assert.Contains(t, string(tools), `"edit_text"`)

	sig := analyze(tr.Calls)
	require.Len(t, sig.Repairs, 1)
	assert.Equal(t, repairFixedNamed, sig.Repairs[0].Class)
	assert.Equal(t, "block", sig.Repairs[0].NamedField)
	assert.Equal(t, 400, sig.Repairs[0].Status)
}

func TestOpsArmServesThePublishedOpSchemasVerbatim(t *testing.T) {
	// given
	api := newStubAPI(servedDoc)
	defer api.Close()
	client := newAPIClient(api.URL, "key", &recordingTransport{base: http.DefaultTransport, rec: &recorder{}})

	// when
	ts, err := newOpsToolset(context.Background(), client, "space1", "obj1")

	// then
	require.NoError(t, err)
	require.Len(t, ts.tools(), len(opsArmOps)+1)
	assert.Equal(t, "read_object", ts.tools()[0].Name)
	for i, op := range opsArmOps {
		spec := ts.tools()[i+1]
		assert.Equal(t, op, spec.Name)
		assert.JSONEq(t, fmt.Sprintf(`{"type":"object","x-op":%q}`, op), string(spec.Parameters),
			"the arm must pass the served schema through unchanged")
	}

	out := ts.call(context.Background(), "read_object", map[string]any{})
	assert.False(t, out.IsError)
	assert.Contains(t, out.Text, `"a1b2c"`)
}

func TestOpsArmSendsThePayloadAsOneOpAndSurfacesTheRefusal(t *testing.T) {
	// given
	stub := &stubAPI{doc: servedDoc, refuseFirstPatch: true}
	api := httptest.NewServer(stub)
	defer api.Close()
	client := newAPIClient(api.URL, "key", &recordingTransport{base: http.DefaultTransport, rec: &recorder{}})
	ts, err := newOpsToolset(context.Background(), client, "space1", "obj1")
	require.NoError(t, err)

	// when
	refused := ts.call(context.Background(), "insert_blocks", map[string]any{"markdown": "## Risks"})
	ok := ts.call(context.Background(), "insert_blocks", map[string]any{"markdown": "## Risks"})

	// then
	assert.True(t, refused.IsError)
	assert.Contains(t, refused.Text, "400 invalid_input")
	assert.Contains(t, refused.Text, "ops[0].id:", "the raw arm shows the path-addressed issue as served")
	assert.False(t, ok.IsError)

	stub.mu.Lock()
	defer stub.mu.Unlock()
	require.Len(t, stub.patches, 2)
	assert.JSONEq(t, `{"op":"insert_blocks","markdown":"## Risks"}`, string(stub.patches[0]),
		"the op name comes from the tool name when the model omits the const")
}

func TestAgentLoopStopsAtTheTurnBudget(t *testing.T) {
	// given
	model, _ := newScriptedModel(
		toolCallTurn("read_object", `{}`),
		toolCallTurn("read_object", `{}`),
		toolCallTurn("read_object", `{}`),
	)
	defer model.Close()
	api := newStubAPI(servedDoc)
	defer api.Close()
	client := newAPIClient(api.URL, "key", &recordingTransport{base: http.DefaultTransport, rec: &recorder{}})
	ts, err := newOpsToolset(context.Background(), client, "space1", "obj1")
	require.NoError(t, err)

	// when
	tr, err := runAgent(context.Background(), agentConfig{
		chat: newChatClient(model.URL, "", 30*time.Second), model: "stub", maxTurns: 2,
	}, ts, "sys", "user")

	// then
	require.NoError(t, err)
	assert.Equal(t, "turn_budget", tr.StoppedBy)
	assert.Len(t, tr.Turns, 2)
}

func TestModelFailureIsAnEnvironmentFailureNotATaskFailure(t *testing.T) {
	// given
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer dead.Close()
	api := newStubAPI(servedDoc)
	defer api.Close()
	client := newAPIClient(api.URL, "key", &recordingTransport{base: http.DefaultTransport, rec: &recorder{}})
	ts, err := newOpsToolset(context.Background(), client, "space1", "obj1")
	require.NoError(t, err)

	// when
	_, err = runAgent(context.Background(), agentConfig{
		chat: newChatClient(dead.URL, "", 5*time.Second), model: "stub", maxTurns: 2,
	}, ts, "sys", "user")

	// then
	require.Error(t, err)
	assert.ErrorIs(t, err, errEnvironment)
}

func TestToolArgumentsDecodeFromBothWireShapes(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want map[string]any
	}{
		{"json-encoded string (OpenAI, Ollama)", `"{\"object\":\"1\"}"`, map[string]any{"object": "1"}},
		{"inline object", `{"object":"1"}`, map[string]any{"object": "1"}},
		{"empty string", `""`, map[string]any{}},
		{"absent", ``, map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			var c toolCall
			c.Function.Arguments = json.RawMessage(tt.raw)

			// when
			got, err := c.argsMap()

			// then
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOpsArmSetsTheDiscriminatorFromTheToolName(t *testing.T) {
	// given — a model that writes a positional word into the const field.
	// The arm splits the ops into separate tools, which a raw HTTP caller
	// does not have, so the const must not decide the outcome here.
	stub := &stubAPI{doc: servedDoc}
	api := httptest.NewServer(stub)
	defer api.Close()
	client := newAPIClient(api.URL, "key", &recordingTransport{base: http.DefaultTransport, rec: &recorder{}})
	ts, err := newOpsToolset(context.Background(), client, "space1", "obj1")
	require.NoError(t, err)

	// when
	out := ts.call(context.Background(), "insert_blocks", map[string]any{"op": "last", "markdown": "## Risks"})

	// then
	assert.False(t, out.IsError)
	stub.mu.Lock()
	defer stub.mu.Unlock()
	require.Len(t, stub.patches, 1)
	assert.JSONEq(t, `{"op":"insert_blocks","markdown":"## Risks"}`, string(stub.patches[0]))
}

func TestWaitSearchableReturnsWhenTheIndexCatchesUp(t *testing.T) {
	// given — the fixture is invisible to search on the first poll and
	// visible on the second, which is what an asynchronous full-text index
	// looks like from the outside
	var polls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		polls++
		if polls == 1 {
			fmt.Fprint(w, `{"data":[],"total":0}`)
			return
		}
		fmt.Fprint(w, `{"data":[{"id":"obj1"}],"total":1}`)
	}))
	defer srv.Close()
	client := newAPIClient(srv.URL, "key", &recordingTransport{base: http.DefaultTransport, rec: &recorder{}})

	// when
	ok, took, err := client.waitSearchable(context.Background(), "space1", "Quarterly plan ab12", "obj1", 5*time.Second)

	// then
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 2, polls)
	assert.Greater(t, took, time.Duration(0))
}

func TestWaitSearchableGivesUpWithoutClaimingSuccess(t *testing.T) {
	// given
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[],"total":0}`)
	}))
	defer srv.Close()
	client := newAPIClient(srv.URL, "key", &recordingTransport{base: http.DefaultTransport, rec: &recorder{}})

	// when
	ok, _, err := client.waitSearchable(context.Background(), "space1", "missing", "obj1", 100*time.Millisecond)

	// then
	require.NoError(t, err)
	assert.False(t, ok, "a timeout is an environment fact, never a silent pass")
}

// stubTransport serves the stub API in-process, with no listening socket:
// http.RoundTripper straight into the handler. The live smoke tests use it
// because a test binary that binds a local port can be treated differently
// by a sandbox's network policy than one that does not, and a smoke test
// that fails for that reason tells you nothing about the model.
type stubTransport struct{ handler http.Handler }

func (t *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	t.handler.ServeHTTP(w, req)
	resp := w.Result()
	resp.Request = req
	return resp, nil
}
