package wrapper

// wrapper_test.go — the fixture: a stub HTTP server recording every request
// the tools send, so each test asserts the exact wire shape (path, query,
// headers, body) and the channel behaviors (handles resolved, labels
// applied, markdown passed through).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
)

type recordedRequest struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
}

type stubResponse struct {
	status int
	body   string
}

type fixture struct {
	*Runner
	t        *testing.T
	server   *httptest.Server
	store    *MemoryStore
	mu       sync.Mutex
	requests []recordedRequest
	// stubs maps "METHOD path" to queued responses (popped in order; the
	// last one repeats).
	stubs map[string][]stubResponse
	now   time.Time
}

func newFixture(t *testing.T) *fixture {
	fx := &fixture{
		t:     t,
		store: NewMemoryStore(),
		stubs: map[string][]stubResponse{},
		now:   time.Date(2026, 8, 6, 15, 0, 0, 0, time.UTC),
	}
	fx.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 0)
		if r.Body != nil {
			data := make([]byte, 1<<20)
			n, _ := r.Body.Read(data)
			body = data[:n]
		}
		fx.mu.Lock()
		fx.requests = append(fx.requests, recordedRequest{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Header: r.Header.Clone(), Body: body,
		})
		key := r.Method + " " + r.URL.Path
		queue := fx.stubs[key]
		fx.mu.Unlock()
		if len(queue) == 0 {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"status":404,"code":"not_found","message":"no stub for %s"}`, key)
			return
		}
		resp := queue[0]
		if len(queue) > 1 {
			fx.mu.Lock()
			fx.stubs[key] = queue[1:]
			fx.mu.Unlock()
		}
		w.WriteHeader(resp.status)
		fmt.Fprint(w, resp.body)
	}))
	t.Cleanup(fx.server.Close)

	client := NewClient(fx.server.URL, "test-key")
	client.Backoff = func(int) time.Duration { return 0 }
	fx.Runner = NewRunner(client, fx.store)
	fx.Runner.now = func() time.Time { return fx.now }
	return fx
}

// stub queues one response for "METHOD path".
func (fx *fixture) stub(methodPath string, status int, body string) {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	parts := methodPath
	fx.stubs[parts] = append(fx.stubs[parts], stubResponse{status: status, body: body})
}

// sent returns the recorded requests matching "METHOD path".
func (fx *fixture) sent(methodPath string) []recordedRequest {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	var out []recordedRequest
	for _, r := range fx.requests {
		if r.Method+" "+r.Path == methodPath {
			out = append(out, r)
		}
	}
	return out
}

// bodyJSON decodes a recorded request body.
func bodyJSON(t *testing.T, r recordedRequest) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(r.Body, &m))
	return m
}

// firstOp digs the single PATCH op out of a recorded request.
func firstOp(t *testing.T, r recordedRequest) map[string]any {
	t.Helper()
	body := bodyJSON(t, r)
	ops, ok := body["ops"].([]any)
	require.True(t, ok, "PATCH body has ops")
	require.Len(t, ops, 1, "the wrapper sends single-op PATCHes")
	op, ok := ops[0].(map[string]any)
	require.True(t, ok)
	return op
}

// seedSession installs a working session (as if find ran).
func (fx *fixture) seedSession(space string, handles ...Handle) *Session {
	s := &Session{Space: space, Handles: handles}
	require.NoError(fx.t, fx.store.Save(s))
	return s
}

// searchResponse renders a stub search result.
func searchResponse(total int, hasMore bool, rows ...apimodel.V2ObjectRow) string {
	resp := apimodel.V2ListResponse[apimodel.V2ObjectRow]{
		Data: rows, Total: total, HasMore: hasMore, Limit: 10,
	}
	data, _ := json.Marshal(resp)
	return string(data)
}

const editOKBody = `{"diffStats":{"blocksAdded":0,"blocksRemoved":0,"blocksChanged":1,"blocksMoved":0,"propertiesChanged":0}}`

func TestRunValidation(t *testing.T) {
	t.Run("unknown tool lists the tool set", func(t *testing.T) {
		fx := newFixture(t)
		_, err := fx.Run(context.Background(), "archive", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unknown tool "archive"`)
		assert.Contains(t, err.Error(), "find, read, describe, create")
	})

	t.Run("unknown argument names the allowed ones", func(t *testing.T) {
		fx := newFixture(t)
		_, err := fx.Run(context.Background(), "find", map[string]any{"space": "s", "sort": "name"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `find does not take "sort"`)
		assert.Contains(t, err.Error(), "space, query, type, filter, limit")
	})

	t.Run("missing required argument carries its description", func(t *testing.T) {
		fx := newFixture(t)
		_, err := fx.Run(context.Background(), "find", map[string]any{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `find needs "space"`)
	})

	t.Run("enum violation lists the values", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "obj1"})
		_, err := fx.Run(context.Background(), "read", map[string]any{"object": "1", "mode": "verbose"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"mode" must be one of full, outline`)
	})
}

func TestFind(t *testing.T) {
	ctx := context.Background()

	t.Run("numbers results and saves the session", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(2, false,
			apimodel.V2ObjectRow{Id: "bafyobj1", Name: "Q3 report", Type: "task"},
			apimodel.V2ObjectRow{Id: "bafyobj2", Name: "Q3 plan", Type: "page"},
		))

		// when
		result, err := fx.Run(ctx, "find", map[string]any{"space": "space1", "type": "task", "filter": "done = false"})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, "1. Q3 report (task)")
		assert.Contains(t, result.Text, "2. Q3 plan (page)")
		assert.Contains(t, result.Text, "2 matches")

		sent := fx.sent("POST /v2/spaces/space1/search")
		require.Len(t, sent, 1)
		assert.Equal(t, "10", sent[0].Query.Get("limit"), "default limit")
		assert.Equal(t, "Bearer test-key", sent[0].Header.Get("Authorization"))
		assert.Empty(t, sent[0].Header.Get("Idempotency-Key"), "search is a read — no idempotency key")
		body := bodyJSON(t, sent[0])
		assert.Equal(t, "task", body["type"])
		assert.Equal(t, "done = false", body["filter"])
		assert.NotContains(t, body, "query", "unset args stay off the wire")

		session, _ := fx.store.Load()
		assert.Equal(t, "space1", session.Space)
		require.Len(t, session.Handles, 2)
		assert.Equal(t, Handle{N: 1, Id: "bafyobj1", Name: "Q3 report", Type: "task"}, session.Handles[0])
	})

	t.Run("each find renumbers and prunes stale labels", func(t *testing.T) {
		fx := newFixture(t)
		s := fx.seedSession("space1", Handle{N: 1, Id: "old1"})
		s.Labels = map[string]map[string]string{
			"old1":     {"ab123": "0123456789abcdef0000ab123"},
			"bafyobj2": {"cd456": "0123456789abcdef0000cd456"},
		}
		require.NoError(t, fx.store.Save(s))
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false,
			apimodel.V2ObjectRow{Id: "bafyobj2", Name: "Plan", Type: "page"}))

		_, err := fx.Run(ctx, "find", map[string]any{"space": "space1"})

		require.NoError(t, err)
		session, _ := fx.store.Load()
		require.Len(t, session.Handles, 1)
		assert.Equal(t, 1, session.Handles[0].N)
		assert.Equal(t, "bafyobj2", session.Handles[0].Id)
		assert.Contains(t, session.Labels, "bafyobj2", "labels of still-referenced objects survive")
		assert.NotContains(t, session.Labels, "old1", "labels of dropped objects are pruned")
	})

	t.Run("truncation steers", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(312, true,
			apimodel.V2ObjectRow{Id: "o1", Name: "A", Type: "task"}))

		result, err := fx.Run(ctx, "find", map[string]any{"space": "space1", "limit": 1})

		require.NoError(t, err)
		assert.Contains(t, result.Text, "312 matches — showing 1; narrow with filter or query, or raise limit")
	})

	t.Run("@me in the filter resolves through members/me", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/members/me", 200, `{"id":"_participant_space1_acc","name":"Me","role":"owner"}`)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(0, false))

		_, err := fx.Run(ctx, "find", map[string]any{"space": "space1", "filter": `assignee = "@me"`})

		require.NoError(t, err)
		sent := fx.sent("POST /v2/spaces/space1/search")
		require.Len(t, sent, 1)
		assert.Equal(t, `assignee = "_participant_space1_acc"`, bodyJSON(t, sent[0])["filter"])
		session, _ := fx.store.Load()
		assert.Equal(t, "_participant_space1_acc", session.Me["space1"], "@me is cached per space")
	})

	t.Run("server error text passes through with issues", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 400, `{"status":400,"code":"validation_failed","message":"parse error at offset 17","issues":[{"path":"/filter","message":"unknown property key \"dueDat\"","hint":"did you mean dueDate?"}]}`)

		_, err := fx.Run(ctx, "find", map[string]any{"space": "space1", "filter": "dueDat < today()"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse error at offset 17")
		assert.Contains(t, err.Error(), `/filter: unknown property key "dueDat" (did you mean dueDate?)`)
	})
}

const testFullDoc = `{"version":1,"etag":"abcd1234","type":"task","properties":{"name":"Doc"},"blocks":[{"id":"aaaabbbbccccddddeeee0001","type":"heading1","text":"Section"},{"id":"aaaabbbbccccddddeeee0002","type":"paragraph","text":"body"}]}`

func TestRead(t *testing.T) {
	ctx := context.Background()

	t.Run("full read relabels block ids and retains the map", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1", Name: "Doc", Type: "task"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, testFullDoc)

		// when: the object argument is a handle from find
		result, err := fx.Run(ctx, "read", map[string]any{"object": "1"})

		// then
		require.NoError(t, err)
		assert.NotContains(t, result.Text, "aaaabbbbccccddddeeee0001", "full ids never reach the model")
		assert.Contains(t, result.Text, `"e0001"`, "block ids become 5-char suffix labels")
		assert.Contains(t, result.Text, `"e0002"`)

		session, _ := fx.store.Load()
		assert.Equal(t, "aaaabbbbccccddddeeee0001", session.Labels["bafyobj1"]["e0001"])
		assert.Equal(t, "aaaabbbbccccddddeeee0002", session.Labels["bafyobj1"]["e0002"])
	})

	t.Run("outline mode passes through the server shape", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, `{"outline":[{"indent":0,"id":"e0001","type":"heading1","text":"Section"}]}`)

		result, err := fx.Run(ctx, "read", map[string]any{"object": "1", "mode": "outline"})

		require.NoError(t, err)
		sent := fx.sent("GET /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 1)
		assert.Equal(t, "true", sent[0].Query.Get("outline"))
		assert.Contains(t, result.Text, `"outline"`)
	})

	t.Run("raw object id resolves against the working space", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1")
		fx.stub("GET /v2/spaces/space1/objects/bafyzzz", 200, `{"version":1,"type":"page"}`)

		_, err := fx.Run(ctx, "read", map[string]any{"object": "bafyzzz"})

		require.NoError(t, err)
	})

	t.Run("no session steers to find", func(t *testing.T) {
		fx := newFixture(t)
		_, err := fx.Run(ctx, "read", map[string]any{"object": "1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "run find first")
	})

	t.Run("stale handle names the live range", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "a"}, Handle{N: 2, Id: "b"})
		_, err := fx.Run(ctx, "read", map[string]any{"object": "7"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no handle 7 — the last find returned 2 results")
	})
}

func TestDescribe(t *testing.T) {
	fx := newFixture(t)
	fx.stub("GET /v2/spaces/space1/types/task", 200,
		`{"version":1,"kind":"objectType","key":"task","properties":{"name":"Task"},"typeProperties":[{"key":"dueDate","name":"Due date","format":"date"},{"key":"status","name":"Status","format":"select"}]}`)
	fx.stub("GET /v2/spaces/space1/properties/status/options", 200,
		`{"data":[{"name":"Backlog"},{"name":"In progress"},{"name":"Done"}],"total":3,"offset":0,"limit":25,"has_more":false}`)

	result, err := fx.Run(context.Background(), "describe", map[string]any{"space": "space1", "type": "task"})

	require.NoError(t, err)
	assert.Contains(t, result.Text, "type task — Task")
	assert.Contains(t, result.Text, "dueDate  date")
	assert.Contains(t, result.Text, "status  select  options: Backlog, In progress, Done")
	assert.Contains(t, result.Text, "use these exact property keys and option names")

	js, ok := result.JSON.(describeResult)
	require.True(t, ok)
	require.Len(t, js.Properties, 2)
	assert.Equal(t, []string{"Backlog", "In progress", "Done"}, js.Properties[1].Options)
}
