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

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
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
func searchResponse(total int, hasMore bool, rows ...v2model.ObjectRow) string {
	resp := v2model.ListResponse[v2model.ObjectRow]{
		Data: rows, Total: total, HasMore: hasMore, Limit: 10,
	}
	data, _ := json.Marshal(resp)
	return string(data)
}

// propertiesResponse renders a stub property index (the settable universe
// describe reads since §8.33).
func propertiesResponse(rows ...v2model.PropertyRow) string {
	resp := v2model.ListResponse[v2model.PropertyRow]{
		Data: rows, Total: len(rows), Limit: propertyFormatsPageSize,
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
			v2model.ObjectRow{Id: "bafyobj1", Name: "Q3 report", Type: "task"},
			v2model.ObjectRow{Id: "bafyobj2", Name: "Q3 plan", Type: "page"},
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

	t.Run("each find renumbers", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "old1"})
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false,
			v2model.ObjectRow{Id: "bafyobj2", Name: "Plan", Type: "page"}))

		_, err := fx.Run(ctx, "find", map[string]any{"space": "space1", "query": "plan"})

		require.NoError(t, err)
		session, _ := fx.store.Load()
		require.Len(t, session.Handles, 1)
		assert.Equal(t, 1, session.Handles[0].N)
		assert.Equal(t, "bafyobj2", session.Handles[0].Id)
	})

	t.Run("truncation steers", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(312, true,
			v2model.ObjectRow{Id: "o1", Name: "A", Type: "task"}))

		result, err := fx.Run(ctx, "find", map[string]any{"space": "space1", "type": "task", "limit": 1})

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

	// §8.33 defect 1: a find whose only argument is the space matched
	// nothing, yet rendered as "N matches" with numbered handles. A small
	// model that dropped its query read handle 1 as the note it was looking
	// for and wrote three blocks into an unrelated object.
	t.Run("a find with no criteria lists the space and numbers nothing", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "stale1"})
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(78, true,
			v2model.ObjectRow{Id: "bafyobj1", Name: "Kimubabe", Type: "page"},
			v2model.ObjectRow{Id: "bafyobj2", Name: "Zejeseso", Type: "page"},
		))

		// when
		result, err := fx.Run(ctx, "find", map[string]any{"space": "space1"})

		// then
		require.NoError(t, err)
		assert.NotContains(t, result.Text, "matches", "a listing must never render as a match")
		assert.NotContains(t, result.Text, "78 matches")
		assert.NotContains(t, result.Text, "1. Kimubabe", "a listing must not number its rows")
		assert.Contains(t, result.Text, "nothing was searched for")
		assert.Contains(t, result.Text, "Kimubabe (page)", "the browse intent is still served")
		assert.Contains(t, result.Text, "78 objects — showing 2")
		assert.Contains(t, result.Text, "run find again with query")

		session, _ := fx.store.Load()
		assert.Equal(t, "space1", session.Space, "the working space is still set")
		assert.Empty(t, session.Handles,
			"no handle exists, so no tool can address a listed object — the mis-address is unreachable, not discouraged")

		js, ok := result.JSON.(findResult)
		require.True(t, ok)
		assert.True(t, js.Listing)
		assert.Empty(t, js.Handles)
		require.Len(t, js.Rows, 2)
		assert.Zero(t, js.Rows[0].N, "a listed row carries no handle number")
	})

	t.Run("a handle after a listing names the repair, not a missing session", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false,
			v2model.ObjectRow{Id: "bafyobj1", Name: "Kimubabe", Type: "page"}))

		_, err := fx.Run(ctx, "find", map[string]any{"space": "space1"})
		require.NoError(t, err)
		_, err = fx.Run(ctx, "read", map[string]any{"object": "1"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "the last find numbered nothing")
		assert.Contains(t, err.Error(), "Run find with query, type or filter")
		assert.NotContains(t, err.Error(), "run find first",
			"find HAS run — a repair the caller already performed reads as a dead end")
	})

	t.Run("any one criterion makes it a search again", func(t *testing.T) {
		for _, criterion := range []map[string]any{
			{"space": "space1", "query": "kimu"},
			{"space": "space1", "type": "page"},
			{"space": "space1", "filter": `done = false`},
		} {
			fx := newFixture(t)
			fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false,
				v2model.ObjectRow{Id: "bafyobj1", Name: "Kimubabe", Type: "page"}))

			result, err := fx.Run(ctx, "find", criterion)

			require.NoError(t, err)
			assert.Contains(t, result.Text, "1. Kimubabe (page)")
			assert.Contains(t, result.Text, "1 matches")
			session, _ := fx.store.Load()
			require.Len(t, session.Handles, 1, "a criterion was given: this is a search and it numbers")
		}
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

// testFullDoc is what the server now serves on a default read: minted block
// ids already relabeled server-side (Wave 0.2), meaningful ids in full.
const testFullDoc = `{"version":1,"etag":"abcd1234","type":"task","properties":{"name":"Doc"},"blocks":[{"id":"e0001","type":"heading1","text":"Section"},{"id":"e0002","type":"paragraph","text":"body"},{"id":"quarterly-goals-1","type":"paragraph","text":"tail"}]}`

func TestRead(t *testing.T) {
	ctx := context.Background()

	t.Run("read serves the server-labeled document verbatim", func(t *testing.T) {
		// given: the server labels minted ids itself now — the wrapper's
		// client-side relabeling and its session label map are retired
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1", Name: "Doc", Type: "task"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, testFullDoc)

		// when: the object argument is a handle from find
		result, err := fx.Run(ctx, "read", map[string]any{"object": "1"})

		// then
		require.NoError(t, err)
		assert.Equal(t, testFullDoc, result.Text, "the served bytes pass through untouched")
	})

	t.Run("a stale pre-upgrade label map cannot rewrite refs", func(t *testing.T) {
		// reproduced before the retirement: a session file from a previous
		// wrapper version carried labels for an OLD document version; the
		// `if labels != nil` guard meant a fresh read never cleared it, and a
		// write rewrote the label the model just read into an outdated full
		// id — which the server accepted as an exact match. Now the field
		// does not exist: an old session file still loads (unknown JSON
		// fields are ignored) and refs pass through as the model spoke them.
		fx := newFixture(t)
		staleSession := `{"space":"space1","handles":[{"n":1,"id":"bafyobj1","name":"Doc","type":"task"}],` +
			`"labels":{"bafyobj1":{"e0001":"aaaabbbbccccddddeee9dead"}}}`
		fx.store.data = []byte(staleSession) // a session file written by the previous version
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "check_item", map[string]any{"object": "1", "block": "e0001", "checked": true})

		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		assert.Equal(t, "e0001", op["id"],
			"the ref the model spoke goes to the server — never a stale map's full id")
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

// TestObjectRefSteering pins §8.33's defect 2. The refusal for a
// block-shaped `object` was `object "767cb" not found in space "bafyrei…"`
// — true, and naming no repair: a small model sent that call three times
// byte-identically and then abandoned the task. The shape is recognisable
// and its repair is known, so the wrapper names it — on Run's error path,
// which is why every tool taking `object` gets it and not just edit_text.
func TestObjectRefSteering(t *testing.T) {
	ctx := context.Background()
	notFound := func(ref string) string {
		return `{"status":404,"code":"not_found","message":"object \"` + ref + `\" not found in space \"space1\""}`
	}

	t.Run("a block reference in object says where a block goes", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		// edit_text with no block locates it from the snippet: the object read
		// is the call that 404s
		fx.stub("GET /v2/spaces/space1/objects/767cb", 404, notFound("767cb"))

		_, err := fx.Run(ctx, "edit_text", map[string]any{
			"object": "767cb", "find": "Q3", "replace": "Q4",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `object "767cb" not found in space "space1"`,
			"the server's fact stays — the hint is added to it, never instead of it")
		assert.Contains(t, err.Error(), "that is a block reference: read serves those, and they go in `block`")
		assert.Contains(t, err.Error(), "handle number from the last find")
	})

	t.Run("a full minted block id is the same mistake", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/62d5c4a1b9e04f3a8c7d1e2b", 404, notFound("62d5c4a1b9e04f3a8c7d1e2b"))

		_, err := fx.Run(ctx, "read", map[string]any{"object": "62d5c4a1b9e04f3a8c7d1e2b"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "a block reference belongs in a block argument, not in `object`",
			"read takes no block argument, so the repair names the category and not a slot read does not have")
	})

	t.Run("the space id in object is named as the space id", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(
			v2model.PropertyRow{Key: "done", Name: "Done", Format: "checkbox"}))
		fx.stub("PATCH /v2/spaces/space1/objects/space1", 404, notFound("space1"))

		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "space1", "set": map[string]any{"done": true},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "that is the space id, not an object",
			"the steer reaches every tool taking object, including ones with no block argument")
	})

	t.Run("a name in object points at find", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/Kimubabe", 404, notFound("Kimubabe"))

		_, err := fx.Run(ctx, "read", map[string]any{"object": "Kimubabe"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "run find with query naming it")
	})

	t.Run("a not-found about anything else is left alone", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 404,
			`{"status":404,"code":"not_found","message":"property \"assignee\" not found in space \"space1\""}`)

		_, err := fx.Run(ctx, "check_item", map[string]any{"object": "1", "block": "e0002", "checked": true})

		require.Error(t, err)
		assert.NotContains(t, err.Error(), "handle number from the last find",
			"the object resolved — a 404 about something else must not be re-explained as a bad object")
	})
}

func TestDescribe(t *testing.T) {
	fx := newFixture(t)
	fx.stub("GET /v2/spaces/space1/types/task", 200,
		`{"version":1,"kind":"objectType","key":"task","properties":{"name":"Task"},"typeProperties":[{"key":"dueDate","name":"Due date","format":"date"},{"key":"status","name":"Status","format":"select"}]}`)
	fx.stub("GET /v2/spaces/space1/properties/status/options", 200,
		`{"data":[{"name":"Backlog"},{"name":"In progress"},{"name":"Done"}],"total":3,"offset":0,"limit":25,"has_more":false}`)
	fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(
		v2model.PropertyRow{Key: "dueDate", Name: "Due date", Format: "date"},
		v2model.PropertyRow{Key: "status", Name: "Status", Format: "select"},
		v2model.PropertyRow{Key: "description", Name: "Description", Format: "text"},
	))

	result, err := fx.Run(context.Background(), "describe", map[string]any{"space": "space1", "type": "task"})

	require.NoError(t, err)
	assert.Contains(t, result.Text, "type task — Task")
	assert.Contains(t, result.Text, "dueDate  date")
	assert.Contains(t, result.Text, "status  select  options: Backlog, In progress, Done")
	assert.Contains(t, result.Text, "use these exact property keys and option names")

	js, ok := result.JSON.(describeResult)
	require.True(t, ok)
	// the type's two, then the space's remainder (description) and the
	// always-settable name — the type's list is a section, not the bound
	require.Len(t, js.Properties, 4)
	assert.Equal(t, []string{"Backlog", "In progress", "Done"}, js.Properties[1].Options)
	assert.True(t, js.Properties[0].OnType)
	assert.Equal(t, []string{"description", "name"}, []string{js.Properties[2].Key, js.Properties[3].Key})
	assert.False(t, js.Properties[2].OnType)
}

// TestDescribeReportsWhatIsSettable pins §8.33's defect 3: describe read the
// type's RECOMMENDED lists and served them under "use these keys in
// set_properties". A small model asked to set a description read that,
// found none, and reported the task impossible — `description` is a
// bundled property the API sets in one call. `name` is hidden from GET
// /properties as well, so both of the two properties every object has were
// invisible on the one tool that answers "what can I set".
func TestDescribeReportsWhatIsSettable(t *testing.T) {
	fx := newFixture(t)
	// the live `page` type: it recommends output-only and derived keys, and
	// recommends neither name nor description
	fx.stub("GET /v2/spaces/space1/types/page", 200,
		`{"version":1,"kind":"objectType","key":"page","properties":{"name":"Page"},"typeProperties":[
			{"key":"tag","name":"Tag","format":"multiSelect"},
			{"key":"createdDate","name":"Creation date","format":"date"},
			{"key":"creator","name":"Created by","format":"objects"}]}`)
	fx.stub("GET /v2/spaces/space1/properties/tag/options", 200,
		`{"data":[{"name":"Urgent"}],"total":1,"offset":0,"limit":25,"has_more":false}`)
	fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(
		v2model.PropertyRow{Key: "description", Name: "Description", Format: "text"},
		v2model.PropertyRow{Key: "dueDate", Name: "Due date", Format: "date"},
		v2model.PropertyRow{Key: "createdDate", Name: "Creation date", Format: "date"},
	))

	result, err := fx.Run(context.Background(), "describe", map[string]any{"space": "space1", "type": "page"})

	require.NoError(t, err)
	assert.Contains(t, result.Text, "\n  description  text",
		"a bundled settable property the type does not recommend must still be shown")
	assert.Contains(t, result.Text, "\n  name  text",
		"name is hidden from GET /properties and recommended by no type — describe is the only place it can come from")
	assert.Contains(t, result.Text, "\n  dueDate  date",
		"any of the space's properties is settable on any object")
	assert.Contains(t, result.Text, "read-only — read serves these, set_properties refuses them: createdDate, creator",
		"output-only keys are named as such, not offered as settable and not silently dropped")
	assert.NotContains(t, result.Text, "\n  createdDate  date",
		"an output-only key must never appear in a settable section")

	js, ok := result.JSON.(describeResult)
	require.True(t, ok)
	settable := map[string]bool{}
	for _, p := range js.Properties {
		if !p.ReadOnly {
			settable[p.Key] = true
		}
	}
	assert.Equal(t, map[string]bool{"tag": true, "description": true, "dueDate": true, "name": true}, settable)
}
