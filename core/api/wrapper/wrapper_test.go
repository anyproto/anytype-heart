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
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
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
//
// A stubbed body that IS an AnyBlock document is validated against the
// format before it is queued. These stubs stand in for what the API serves,
// and a stale one does not fail — it agrees with equally stale decoding code
// and both drift away from production together. That is exactly how
// describe's type decode went on reading a pre-§2a document (envelope `key`,
// top-level `typeProperties`) long after the API stopped serving one:
// json.Unmarshal leaves absent members zero, so the tool answered "this type
// has no properties" and every test agreed with it.
func (fx *fixture) stub(methodPath string, status int, body string) {
	fx.t.Helper()
	if doc, ok := anyBlockDocumentStub(body); ok {
		if err := anyblockjson.Validate(doc); err != nil {
			fx.t.Fatalf("stubbed AnyBlock document for %q is not valid — a stub the API could never serve "+
				"tests nothing: %v", methodPath, err)
		}
	}
	fx.mu.Lock()
	defer fx.mu.Unlock()
	parts := methodPath
	fx.stubs[parts] = append(fx.stubs[parts], stubResponse{status: status, body: body})
}

// anyBlockDocumentStub reports whether a stubbed body is a document read,
// and returns it with v2's own envelope additions removed so the FORMAT can
// judge the rest.
//
// A read envelope is an AnyBlock document plus what v2 adds to it — `etag`
// (C7), `warnings` (C11), and the `?outline=`/`?format=md`/`?block=` shapes.
// Those are v2's, not the format's, and validating them as if they were the
// format's would fail on the API's own contract. The strip list is the one
// normalizeCreateBody applies for the same reason on the way in.
func anyBlockDocumentStub(body string) ([]byte, bool) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &probe); err != nil {
		return nil, false
	}
	if _, ok := probe["version"]; !ok {
		return nil, false
	}
	// a partial read is a fragment of a document, not one — the format
	// cannot judge it whole
	if _, partial := probe["subtree"]; partial {
		return nil, false
	}
	for _, added := range []string{"etag", "warnings", "outline", "markdown"} {
		delete(probe, added)
	}
	doc, err := json.Marshal(probe)
	if err != nil {
		return nil, false
	}
	return doc, true
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
	ops := patchOpsSent(t, r)
	require.Len(t, ops, 1, "every tool but delete_block's batch form sends single-op PATCHes")
	return ops[0]
}

// patchOpsSent digs all PATCH ops out of a recorded request (delete_block's
// multi-reference batch).
func patchOpsSent(t *testing.T, r recordedRequest) []map[string]any {
	t.Helper()
	body := bodyJSON(t, r)
	raw, ok := body["ops"].([]any)
	require.True(t, ok, "PATCH body has ops")
	ops := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		op, ok := entry.(map[string]any)
		require.True(t, ok)
		ops = append(ops, op)
	}
	return ops
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

const editOKBody = `{"diff_stats":{"blocks_added":0,"blocks_removed":0,"blocks_changed":1,"blocks_moved":0,"properties_changed":0}}`

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

	t.Run("the text channel spells each result's type by display name", func(t *testing.T) {
		// the product example: users call the `set` type "Query" — the key
		// is an internal identifier find must not teach, because whatever
		// find prints is what the model hands back to describe and create
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(2, false,
			v2model.ObjectRow{Id: "bafyobj1", Name: "Open tasks", Type: "set"},
			v2model.ObjectRow{Id: "bafyobj2", Name: "Q3 report", Type: "task"},
		))
		fx.stub("GET /v2/spaces/space1/types", 200,
			`{"data":[{"key":"set","name":"Query"},{"key":"task","name":"Task"}],"total":2,"offset":0,"limit":500,"has_more":false}`)

		result, err := fx.Run(ctx, "find", map[string]any{"space": "space1", "query": "q"})

		require.NoError(t, err)
		assert.Contains(t, result.Text, "1. Open tasks (Query)")
		assert.Contains(t, result.Text, "2. Q3 report (Task)")
		assert.NotContains(t, result.Text, "(set)", "the internal key stays off the prompt")

		js, ok := result.JSON.(findResult)
		require.True(t, ok)
		assert.Equal(t, "set", js.Handles[0].Type,
			"the machine channel keeps the key — it is the type's identity, and programmatic callers speak it")
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

	t.Run("server error text passes through with issues — verbatim, in the name vocabulary", func(t *testing.T) {
		// find sends ?keys=name, so the server's refusal arrives already
		// spelled in names (§4.3) — the wrapper serves it untouched (zero
		// translation, APIV2_VOCABULARY.md layer 3)
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 400, `{"status":400,"code":"validation_failed","message":"parse error at offset 17","issues":[{"path":"/filter","message":"unknown property \"due_dat\"","hint":"did you mean Due date?"}]}`)

		_, err := fx.Run(ctx, "find", map[string]any{"space": "space1", "filter": "dueDat < today()"})

		require.Error(t, err)
		sent := fx.sent("POST /v2/spaces/space1/search")
		require.Len(t, sent, 1)
		assert.Equal(t, "name", sent[0].Query.Get("keys"),
			"search asks for the name vocabulary — the server then refuses in names")
		assert.Contains(t, err.Error(), "parse error at offset 17")
		assert.Contains(t, err.Error(), `/filter: unknown property "due_dat" (did you mean Due date?)`)
	})
}

// testFullDoc is what the server now serves on a default read: minted block
// ids already relabeled server-side (Wave 0.2), meaningful ids in full.
const testFullDoc = `{"formatVersion":"2.0","etag":"abcd1234","type":"task","properties":{"name":"Doc"},"blocks":[{"id":"e0001","type":"heading_1","text":"Section"},{"id":"e0002","type":"paragraph","text":"body"},{"id":"quarterly-goals-1","type":"paragraph","text":"tail"}]}`

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
		sent := fx.sent("GET /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 1)
		assert.Equal(t, "name", sent[0].Query.Get("keys"),
			"read asks for the name vocabulary (D5) — the wrapper teaches names, so its reads must spell them")
	})

	t.Run("outline mode passes through the server shape", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, `{"outline":[{"indent":0,"id":"e0001","type":"heading_1","text":"Section"}]}`)

		result, err := fx.Run(ctx, "read", map[string]any{"object": "1", "mode": "outline"})

		require.NoError(t, err)
		sent := fx.sent("GET /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 1)
		assert.Equal(t, "true", sent[0].Query.Get("outline"))
		assert.Equal(t, "name", sent[0].Query.Get("keys"), "the vocabulary rides outline reads too")
		assert.Contains(t, result.Text, `"outline"`)
	})

	t.Run("raw object id resolves against the working space", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1")
		fx.stub("GET /v2/spaces/space1/objects/bafyzzz", 200, `{"formatVersion":"2.0","type":"page"}`)

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
	// the type document as the server serves it under ?keys=name (D5): the
	// definitions spell display names, internal_key carries the stored key
	fx := newFixture(t)
	fx.stub("GET /v2/spaces/space1/types/task", 200,
		`{"formatVersion":"2.0","kind":"object_type","properties":{"Name":"Task"},"type_settings":{"api_key":"task","property_definitions":[{"property":"Due date","internal_key":"dueDate","name":"Due date","format":"date"},{"property":"Status","internal_key":"status","name":"Status","format":"select"}]}}`)
	fx.stub("GET /v2/spaces/space1/properties/status/options", 200,
		`{"data":[{"name":"Backlog"},{"name":"In progress"},{"name":"Done"}],"total":3,"offset":0,"limit":25,"has_more":false}`)
	fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(
		v2model.PropertyRow{Key: "due_date", Name: "Due date", Format: "date"},
		v2model.PropertyRow{Key: "status", Name: "Status", Format: "select"},
		v2model.PropertyRow{Key: "description", Name: "Description", Format: "text"},
	))

	result, err := fx.Run(context.Background(), "describe", map[string]any{"space": "space1", "type": "task"})

	require.NoError(t, err)
	sent := fx.sent("GET /v2/spaces/space1/types/task")
	require.Len(t, sent, 1)
	assert.Equal(t, "name", sent[0].Query.Get("keys"), "describe reads the name vocabulary (D5)")
	assert.Contains(t, result.Text, "type Task")
	assert.NotContains(t, result.Text, "type task", "the internal key stays off the prompt — the name IS the vocabulary")
	assert.Contains(t, result.Text, "Due date  date")
	assert.Contains(t, result.Text, "Status  select  options: Backlog, In progress, Done")
	assert.Contains(t, result.Text, "use these exact property names and option names")
	// the option route takes the api key, never the display name
	assert.Len(t, fx.sent("GET /v2/spaces/space1/properties/status/options"), 1)

	js, ok := result.JSON.(describeResult)
	require.True(t, ok)
	// the type's two, then the space's remainder (Description) and the
	// always-settable Name — the type's list is a section, not the bound;
	// the definitions and the /properties rows spell one property
	// differently now, and the identity dedup must not list it twice
	require.Len(t, js.Properties, 4)
	assert.Equal(t, []string{"Backlog", "In progress", "Done"}, js.Properties[1].Options)
	assert.True(t, js.Properties[0].OnType)
	assert.Equal(t, "Due date", js.Properties[0].Key, "the served spelling is the document's — the name")
	assert.Equal(t, []string{"Description", "Name"}, []string{js.Properties[2].Key, js.Properties[3].Key})
	assert.False(t, js.Properties[2].OnType)
}

// TestDescribeSpeaksNamesOverSlugDocument pins the name vocabulary against
// an older server whose type document still states slugs in
// property_definitions (`property: "due_date"`): the definition's `name`
// field carries the display name, and THAT is what describe must render —
// a slug shown under a footer saying "use these exact property names" is
// the contradiction this surface existed to avoid.
func TestDescribeSpeaksNamesOverSlugDocument(t *testing.T) {
	fx := newFixture(t)
	fx.stub("GET /v2/spaces/space1/types/task", 200,
		`{"formatVersion":"2.0","kind":"object_type","properties":{"name":"Task"},"type_settings":{"api_key":"task","property_definitions":[{"property":"due_date","internal_key":"dueDate","name":"Due date","format":"date"}]}}`)
	fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(
		v2model.PropertyRow{Key: "due_date", Name: "Due date", Format: "date"},
	))

	result, err := fx.Run(context.Background(), "describe", map[string]any{"space": "space1", "type": "task"})

	require.NoError(t, err)
	assert.Contains(t, result.Text, "type Task")
	assert.Contains(t, result.Text, "\n  Due date  date",
		"the definition stated the slug, the name field names it — the name is served")
	assert.NotContains(t, result.Text, "due_date", "the slug never reaches the prompt")

	js, ok := result.JSON.(describeResult)
	require.True(t, ok)
	assert.Equal(t, "Due date", js.Properties[0].Key, "the machine row's served spelling is the name too")
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
	// the live `page` type under ?keys=name: it recommends output-only and
	// derived keys — spelled as display names now, with the stored key in
	// internal_key — and recommends neither Name nor Description
	fx.stub("GET /v2/spaces/space1/types/page", 200,
		`{"formatVersion":"2.0","kind":"object_type","properties":{"Name":"Page"},"type_settings":{"api_key":"page","property_definitions":[
			{"property":"Tag","internal_key":"tag","name":"Tag","format":"multi_select"},
			{"property":"Creation date","internal_key":"createdDate","name":"Creation date","format":"date"},
			{"property":"Created by","internal_key":"creator","name":"Created by","format":"objects"}]}}`)
	fx.stub("GET /v2/spaces/space1/properties/tag/options", 200,
		`{"data":[{"name":"Urgent"}],"total":1,"offset":0,"limit":25,"has_more":false}`)
	fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(
		v2model.PropertyRow{Key: "description", Name: "Description", Format: "text"},
		v2model.PropertyRow{Key: "due_date", Name: "Due date", Format: "date"},
		v2model.PropertyRow{Key: "created_date", Name: "Creation date", Format: "date"},
	))

	result, err := fx.Run(context.Background(), "describe", map[string]any{"space": "space1", "type": "page"})

	require.NoError(t, err)
	assert.Contains(t, result.Text, "\n  Description  text",
		"a bundled settable property the type does not recommend must still be shown")
	assert.Contains(t, result.Text, "\n  Name  text",
		"name is hidden from GET /properties and recommended by no type — describe is the only place it can come from")
	assert.Contains(t, result.Text, "\n  Due date  date",
		"any of the space's properties is settable on any object")
	assert.Contains(t, result.Text, "read-only — read serves these, set_properties refuses them: Creation date, Created by",
		"output-only keys are named as such, not offered as settable and not silently dropped — the "+
			"read-only predicate resolves through the definition's row/internal_key, not the display name")
	assert.NotContains(t, result.Text, "\n  Creation date  date",
		"an output-only key must never appear in a settable section — nor twice: its definition and its "+
			"/properties row are ONE property under two spellings")
	assert.Len(t, fx.sent("GET /v2/spaces/space1/properties/tag/options"), 1,
		"a definition with no /properties row lists options through its internal_key, never the display name")

	js, ok := result.JSON.(describeResult)
	require.True(t, ok)
	settable := map[string]bool{}
	for _, p := range js.Properties {
		if !p.ReadOnly {
			settable[p.Key] = true
		}
	}
	assert.Equal(t, map[string]bool{"Tag": true, "Description": true, "Due date": true, "Name": true}, settable)
}
