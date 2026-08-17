package notion

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/domain"
)

// The ResumableConverter seam (08-13 §6.3 / DM spec §8.3): on a resumed
// crawl the engine hands the converter the spool's key set, and every
// skipped page saves the ~2 requests (page + block tree) that make an
// interrupted Notion crawl expensive to redo. The re-search itself stays —
// ~1 request per 100 entities — which is the whole cost of resuming.

// recordingHandler wraps a workspace handler, recording every request line.
type recordingHandler struct {
	inner http.HandlerFunc
	mu    sync.Mutex
	seen  []string
}

func (h *recordingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.seen = append(h.seen, r.Method+" "+r.URL.Path)
	h.mu.Unlock()
	h.inner(w, r)
}

func (h *recordingHandler) requests() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.seen...)
}

func TestSetSkipAvoidsRecordedFetches(t *testing.T) {
	t.Run("a recorded page is neither fetched nor re-emitted; the rest converts", func(t *testing.T) {
		// given: p1 (and its child n1) were recorded by a previous
		// incarnation; the resumed crawl re-searches and must spend zero
		// further requests on them
		handler := &recordingHandler{inner: scriptedWorkspace(t)}
		server := httptest.NewServer(handler)
		t.Cleanup(server.Close)
		apiClient := client.NewClient("token",
			client.WithBaseURL(server.URL),
			client.WithRateLimit(1000),
			client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second}),
		)
		converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir())
		recorded := map[string]struct{}{"p1": {}, "n1": {}}
		converter.SetSkip(func(sourceKey string) bool {
			_, ok := recorded[sourceKey]
			return ok
		})

		// when
		require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error {
			return nil
		}))
		sink := &recordingSink{}
		_, err := converter.Convert(context.Background(), sink)
		require.NoError(t, err)

		// then: no fetch of any skipped entity — page object or block tree
		for _, request := range handler.requests() {
			assert.NotContains(t, []string{
				"GET /pages/p1", "GET /blocks/p1/children",
				"GET /pages/n1", "GET /blocks/n1/children",
			}, request, "a recorded page must cost zero requests on resume")
		}
		assert.Nil(t, sink.byKey("p1"), "a skipped page is not re-emitted (the replay serves it)")
		assert.Nil(t, sink.byKey("n1"))
		assert.NotNil(t, sink.byKey("p2"), "unrecorded pages must still convert")
		assert.NotNil(t, sink.byKey("db1"), "databases re-convert regardless: rows need their property mappings")
	})
}

func TestPlanReuse(t *testing.T) {
	newScriptedConverter := func(t *testing.T, opts ...Option) *Converter {
		t.Helper()
		server := httptest.NewServer(scriptedWorkspace(t))
		t.Cleanup(server.Close)
		apiClient := client.NewClient("token",
			client.WithBaseURL(server.URL),
			client.WithRateLimit(1000),
			client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second}),
		)
		return New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir(), opts...)
	}
	drive := func(t *testing.T, converter *Converter) *recordingSink {
		t.Helper()
		require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error {
			return nil
		}))
		sink := &recordingSink{}
		_, err := converter.Convert(context.Background(), sink)
		require.NoError(t, err)
		return sink
	}

	t.Run("a preset plan is reused verbatim: the planner is never called", func(t *testing.T) {
		// given — 08-13 §6.3: LLM output is not deterministic across calls; a
		// resumed crawl replanning would mint divergent identities for the
		// run's second half. The recorded plan is the only legal input.
		poison := schemaplan.PlannerFunc(func(context.Context, []schemaplan.ContainerSchema) (schemaplan.Plan, error) {
			t.Error("a resumed crawl must never recompute the plan")
			return schemaplan.Plan{}, nil
		})
		preset := schemaplan.Plan{Containers: map[string]schemaplan.ContainerPlan{
			"db1": {TypeKey: domain.TypeKey("task"), Reason: "recorded"},
		}}
		converter := newScriptedConverter(t,
			WithPlanner(poison),
			WithPlanReuse(schemaplan.Reuse{Preset: &preset}),
		)

		// when
		drive(t, converter)

		// then: the preset drove conversion (db1's rows typed per the plan)
		assert.Equal(t, domain.TypeKey("task"), converter.plan.Containers["db1"].TypeKey)
	})

	t.Run("a fresh run records the sanitized plan for a possible resume", func(t *testing.T) {
		// given
		var recordedPlan *schemaplan.Plan
		converter := newScriptedConverter(t, WithPlanReuse(schemaplan.Reuse{
			Record: func(plan schemaplan.Plan) error {
				recordedPlan = &plan
				return nil
			},
		}))

		// when
		drive(t, converter)

		// then: the recorder saw exactly the plan the run converted under
		require.NotNil(t, recordedPlan, "the plan must be recorded before any emission")
		assert.Equal(t, converter.plan, *recordedPlan)
	})
}

// apiResponse scripts one route, failures included — the recovery
// classification depends on exact status shapes.
type apiResponse struct {
	status int
	body   string
}

// recoveryWorkspace is a tiny scripted API for the claim-recovery tests:
// /search returns the given results; every other route must be scripted,
// scripted failures included.
func recoveryWorkspace(t *testing.T, search string, routes map[string]apiResponse) *recordingHandler {
	t.Helper()
	handler := &recordingHandler{}
	handler.inner = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/search" {
			fmt.Fprint(w, search)
			return
		}
		if response, ok := routes[r.Method+" "+r.URL.Path]; ok {
			if response.status != 0 {
				w.WriteHeader(response.status)
			}
			fmt.Fprint(w, response.body)
			return
		}
		t.Errorf("unexpected api call: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
	return handler
}

func recoveryConverter(t *testing.T, handler http.Handler) *Converter {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	apiClient := client.NewClient("token",
		client.WithBaseURL(server.URL),
		client.WithRateLimit(1000),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second}),
	)
	return New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir())
}

func driveConverter(t *testing.T, converter *Converter) *recordingSink {
	t.Helper()
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error {
		return nil
	}))
	sink := &recordingSink{}
	_, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)
	return sink
}

// The recovery half of the seam (review P0-A): a previous incarnation's
// claim with no spool row belongs to an entity /search never returns — it
// was found through a parent's block tree, and on resume the recorded
// parent is skipped, so the block tree is never re-walked. The claim key IS
// the Notion id: recovery re-fetches it directly.
func TestSetRecoverRefetchesUnrecordedClaims(t *testing.T) {
	searchP1 := `{"results":[
		{"object":"page","id":"p1","parent":{"type":"workspace","workspace":true},
		 "properties":{"Name":{"type":"title","title":[{"plain_text":"One","type":"text"}]}}}
	],"has_more":false,"next_cursor":null}`
	childPage := `{"id":"c1","archived":false,
		"created_time":"2024-02-01T10:00:00.000Z","last_edited_time":"2024-02-02T10:00:00.000Z",
		"parent":{"type":"page_id","page_id":"p1"},
		"properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"Child","type":"text"}]}}}`
	notFound := apiResponse{status: http.StatusNotFound, body: `{"code":"object_not_found","message":"gone"}`}

	t.Run("a live unrecorded claim is re-fetched directly and converted — no drift report", func(t *testing.T) {
		// given: p1 recorded (skipped — its block tree, which references c1,
		// is never re-walked), c1 claimed but never spooled
		handler := recoveryWorkspace(t, searchP1, map[string]apiResponse{
			"GET /pages/c1":           {body: childPage},
			"GET /blocks/c1/children": {body: `{"results":[],"has_more":false,"next_cursor":null}`},
		})
		converter := recoveryConverter(t, handler)
		converter.SetSkip(func(sourceKey string) bool { return sourceKey == "p1" })
		converter.SetRecover([]string{"c1", "p1"}) // p1 re-enumerated: filtered by the converter

		// when
		sink := driveConverter(t, converter)

		// then: the page imported; nothing was misreported as loss
		require.NotNil(t, sink.byKey("c1"), "the recovered page must convert: it still exists in the source")
		assert.Empty(t, sink.issues, "recovering a live page is not an issue of any kind")
		var claimed []string
		for _, claim := range sink.claims {
			claimed = append(claimed, claim.SourceKey)
		}
		assert.Contains(t, claimed, "c1", "the recovered entity is re-claimed (absorbed as a reuse by identity)")
		for _, request := range handler.requests() {
			assert.NotContains(t, []string{"GET /pages/p1", "GET /blocks/p1/children"}, request,
				"the recorded parent still costs zero requests")
		}
	})

	t.Run("a positively-gone claim warns dataLoss once — the API's answer, not a guess", func(t *testing.T) {
		// given: every fetch shape answers not-found — the entity is gone (or
		// the integration lost access; either way the source no longer
		// offers it)
		handler := recoveryWorkspace(t, searchP1, map[string]apiResponse{
			"GET /pages/p1":           {body: `{"id":"p1","archived":false,"created_time":"2024-02-01T10:00:00.000Z","last_edited_time":"2024-02-02T10:00:00.000Z","properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"One","type":"text"}]}}}`},
			"GET /blocks/p1/children": {body: `{"results":[],"has_more":false,"next_cursor":null}`},
			"GET /pages/gone":         notFound,
			"GET /data_sources/gone":  notFound,
			"GET /databases/gone":     notFound,
		})
		converter := recoveryConverter(t, handler)
		converter.SetRecover([]string{"gone"})

		// when
		sink := driveConverter(t, converter)

		// then
		require.Len(t, sink.issues, 1)
		assert.Equal(t, importv2.SeverityWarning, sink.issues[0].Severity)
		assert.Equal(t, importv2.IssueDataLoss, sink.issues[0].Code)
		assert.Equal(t, "gone", sink.issues[0].SourceKey)
		assert.Nil(t, sink.byKey("gone"))
	})

	t.Run("a transient recovery failure is a loud object error, never drift", func(t *testing.T) {
		// given: the API is misbehaving — the entity may well still exist,
		// so classifying this as dataLoss would assert a deletion nobody
		// established
		unavailable := apiResponse{status: http.StatusServiceUnavailable, body: `{"code":"service_unavailable"}`}
		handler := recoveryWorkspace(t, searchP1, map[string]apiResponse{
			"GET /pages/p1":           {body: `{"id":"p1","archived":false,"created_time":"2024-02-01T10:00:00.000Z","last_edited_time":"2024-02-02T10:00:00.000Z","properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"One","type":"text"}]}}}`},
			"GET /blocks/p1/children": {body: `{"results":[],"has_more":false,"next_cursor":null}`},
			"GET /pages/flaky":        unavailable,
			"GET /data_sources/flaky": unavailable,
			"GET /databases/flaky":    unavailable,
		})
		converter := recoveryConverter(t, handler)
		converter.SetRecover([]string{"flaky"})

		// when
		sink := driveConverter(t, converter)

		// then: loud, retryable-shaped (so an all-or-nothing abort keeps the
		// dir via the transient-keep rule), and NOT a drift claim
		require.Len(t, sink.issues, 1)
		assert.Equal(t, importv2.SeverityObjectError, sink.issues[0].Severity)
		assert.Equal(t, importv2.IssueObjectFailed, sink.issues[0].Code)
		assert.Equal(t, "flaky", sink.issues[0].SourceKey)
		assert.True(t, client.IsRetryable(sink.issues[0].Err),
			"the wrapped cause must keep its retryable shape for the dir-keep classification")
	})

	t.Run("a data-source claim recovers through its owning database", func(t *testing.T) {
		// given: ds1 was adopted late (a child_database block in a page now
		// recorded); its id is not a page, so the ladder goes
		// /pages (404) → /data_sources → /databases/{owner}, which adopts
		// every data source with proper parents through the proven
		// discovery path.
		handler := recoveryWorkspace(t, searchP1, map[string]apiResponse{
			"GET /pages/p1":           {body: `{"id":"p1","archived":false,"created_time":"2024-02-01T10:00:00.000Z","last_edited_time":"2024-02-02T10:00:00.000Z","properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"One","type":"text"}]}}}`},
			"GET /blocks/p1/children": {body: `{"results":[],"has_more":false,"next_cursor":null}`},
			"GET /pages/ds1":          notFound,
			"GET /data_sources/ds1": {body: `{"id":"ds1","title":[{"plain_text":"Tasks","type":"text"}],
				"parent":{"type":"database_id","database_id":"realdb"},
				"created_time":"2024-01-01T10:00:00.000Z","last_edited_time":"2024-01-02T10:00:00.000Z",
				"properties":{"Name":{"id":"title","type":"title","name":"Name"}}}`},
			"GET /databases/realdb": {body: `{"id":"realdb","title":[{"plain_text":"Tasks","type":"text"}],
				"parent":{"type":"workspace","workspace":true},
				"data_sources":[{"id":"ds1","name":"Tasks"}]}`},
		})
		converter := recoveryConverter(t, handler)
		converter.SetRecover([]string{"ds1"})

		// when
		sink := driveConverter(t, converter)

		// then: the collection converted (schema fetched, rows mappable);
		// nothing warning-or-worse (the type-suggestor info line is the
		// normal late-database path speaking)
		require.NotNil(t, sink.byKey("ds1"), "the recovered data source must convert as a collection")
		for _, issue := range sink.issues {
			assert.Less(t, issue.Severity, importv2.SeverityWarning, "unexpected issue: %v", issue)
		}
	})
}

func TestSkipCarveOutForCollections(t *testing.T) {
	t.Run("a RECORDED data source re-discovered late still re-converts — rows need its property mappings", func(t *testing.T) {
		// given — the deliberate carve-out in the pending drain (review P2:
		// removing it broke no test in the tree, so this one exists): the
		// skip set spares recorded PAGES their fetches, but a collection-like
		// discovery must re-convert regardless, because its schema fetch
		// rebuilds the property mappings THIS incarnation's row conversions
		// need — converter memory a crash lost. Here p-live's link_to_page
		// re-discovers realdb whose data source ds1 is already recorded.
		search := `{"results":[
			{"object":"page","id":"p-live","parent":{"type":"workspace","workspace":true},
			 "properties":{"Name":{"type":"title","title":[{"plain_text":"Live","type":"text"}]}}}
		],"has_more":false,"next_cursor":null}`
		handler := recoveryWorkspace(t, search, map[string]apiResponse{
			"GET /pages/p-live": {body: `{"id":"p-live","archived":false,
				"created_time":"2024-02-01T10:00:00.000Z","last_edited_time":"2024-02-02T10:00:00.000Z",
				"properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"Live","type":"text"}]}}}`},
			"GET /blocks/p-live/children": {body: `{"results":[
				{"id":"lb1","type":"link_to_page","has_children":false,"link_to_page":{"type":"database_id","database_id":"realdb"}}
			],"has_more":false,"next_cursor":null}`},
			"GET /databases/realdb": {body: `{"id":"realdb","title":[{"plain_text":"Tasks","type":"text"}],
				"parent":{"type":"workspace","workspace":true},
				"data_sources":[{"id":"ds1","name":"Tasks"}]}`},
			"GET /data_sources/ds1": {body: `{"id":"ds1","title":[{"plain_text":"Tasks","type":"text"}],
				"created_time":"2024-01-01T10:00:00.000Z","last_edited_time":"2024-01-02T10:00:00.000Z",
				"properties":{"Name":{"id":"title","type":"title","name":"Name"}}}`},
		})
		converter := recoveryConverter(t, handler)
		converter.SetSkip(func(sourceKey string) bool { return sourceKey == "ds1" }) // ds1 recorded

		// when
		sink := driveConverter(t, converter)

		// then: the schema was re-fetched and the collection re-emitted (the
		// engine's sink backstop absorbs the duplicate row; the converter's
		// job is the property mappings)
		assert.Contains(t, handler.requests(), "GET /data_sources/ds1",
			"the recorded data source's schema must be re-fetched: row mappings live in converter memory")
		assert.NotNil(t, sink.byKey("ds1"),
			"a collection-like discovery re-converts even when recorded (the drain carve-out)")
	})
}

// TestRecoveryConsultsTheStop and TestRecoveryProbesEveryIdShape cover the
// two halves of the recovery ladder the review found broken: it classified
// from the error's SHAPE without asking whether the run was still alive, and
// it probed two of the three id shapes pass 1 can claim.
func TestRecoveryConsultsTheStop(t *testing.T) {
	searchP1 := `{"results":[
		{"object":"page","id":"p1","parent":{"type":"workspace","workspace":true},
		 "properties":{"Name":{"type":"title","title":[{"plain_text":"One","type":"text"}]}}}
	],"has_more":false,"next_cursor":null}`
	livePage := `{"id":"p1","archived":false,
		"created_time":"2024-02-01T10:00:00.000Z","last_edited_time":"2024-02-02T10:00:00.000Z",
		"properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"One","type":"text"}]}}}`

	t.Run("a recovery the user's cancel interrupts is the stop, not an object failure", func(t *testing.T) {
		// given — review item 1, the second direction: recoverOne issued
		// IssueObjectFailed for a cancellation without ever consulting
		// ctx.Err(), so a cancelled import reported a retryable-shaped
		// failure — and the settlement above kept its dir, token intact, and
		// silently re-ran it on the next start.
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		handler := recoveryWorkspace(t, searchP1, map[string]apiResponse{
			"GET /pages/p1":           {body: livePage},
			"GET /blocks/p1/children": {body: `{"results":[],"has_more":false,"next_cursor":null}`},
		})
		inner := handler.inner
		handler.inner = func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/pages/c1" {
				cancel()             // the user cancels while the probe is in flight
				<-r.Context().Done() // hold until the client abandons it
				return
			}
			inner(w, r)
		}
		converter := recoveryConverter(t, handler)
		converter.SetRecover([]string{"c1"})
		require.NoError(t, converter.EnumerateIdentities(context.Background(),
			func(importv2.IdentityClaim) error { return nil }))

		// when
		sink := &recordingSink{}
		_, err := converter.Convert(ctx, sink)

		// then: the crawl stops and says so; no verdict is invented about c1
		require.Error(t, err, "a cancelled crawl must abort, not carry on reporting failures")
		assert.ErrorIs(t, err, context.Canceled)
		for _, issue := range sink.issues {
			assert.NotEqual(t, "c1", issue.SourceKey,
				"a cancellation is not evidence about the entity: %v", issue)
		}
	})
}

func TestRecoveryProbesEveryIdShape(t *testing.T) {
	searchP1 := `{"results":[
		{"object":"page","id":"p1","parent":{"type":"workspace","workspace":true},
		 "properties":{"Name":{"type":"title","title":[{"plain_text":"One","type":"text"}]}}}
	],"has_more":false,"next_cursor":null}`
	livePage := `{"id":"p1","archived":false,
		"created_time":"2024-02-01T10:00:00.000Z","last_edited_time":"2024-02-02T10:00:00.000Z",
		"properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"One","type":"text"}]}}}`
	notFound := apiResponse{status: http.StatusNotFound, body: `{"code":"object_not_found","message":"gone"}`}

	t.Run("a bare DATABASE claim recovers instead of being reported deleted", func(t *testing.T) {
		// given — review item 2: /search yields three object shapes and
		// EnumerateIdentities claims all of them, but the ladder probed only
		// /pages and /data_sources. A live database 404s on both and was
		// reported as deleted — verbatim the P0-A symptom this round removed.
		handler := recoveryWorkspace(t, searchP1, map[string]apiResponse{
			"GET /pages/p1":           {body: livePage},
			"GET /blocks/p1/children": {body: `{"results":[],"has_more":false,"next_cursor":null}`},
			"GET /pages/db1":          notFound,
			"GET /data_sources/db1":   notFound,
			"GET /databases/db1": {body: `{"id":"db1","title":[{"plain_text":"Tasks","type":"text"}],
				"parent":{"type":"workspace","workspace":true},
				"data_sources":[{"id":"ds1","name":"Tasks"}]}`},
			"GET /data_sources/ds1": {body: `{"id":"ds1","title":[{"plain_text":"Tasks","type":"text"}],
				"created_time":"2024-01-01T10:00:00.000Z","last_edited_time":"2024-01-02T10:00:00.000Z",
				"properties":{"Name":{"id":"title","type":"title","name":"Name"}}}`},
		})
		converter := recoveryConverter(t, handler)
		converter.SetRecover([]string{"db1"})

		// when
		sink := driveConverter(t, converter)

		// then: the claim key itself converts, and nothing claims it is gone
		require.NotNil(t, sink.byKey("db1"),
			"a live database must import under the id pass 1 claimed")
		for _, issue := range sink.issues {
			assert.NotEqual(t, importv2.IssueDataLoss, issue.Code,
				"a live database must never be reported as deleted: %v", issue)
		}
	})

	t.Run("recovery stops probing at its budget and says so once", func(t *testing.T) {
		// given — review item 15: a vanished claim costs a full id ladder,
		// three GETs and every one of them a 404, and the loop was bounded
		// only by how many claims the previous session made. A workspace
		// whose 10,000 claimed pages were then deleted spent hours issuing
		// silent not-founds at the pacer's ~3 rps before pass 2 could start.
		// lateDiscoveryCap bounds the sibling seam for exactly this reason.
		gone := make([]string, 0, 8)
		routes := map[string]apiResponse{
			"GET /pages/p1":           {body: livePage},
			"GET /blocks/p1/children": {body: `{"results":[],"has_more":false,"next_cursor":null}`},
		}
		for i := 0; i < 8; i++ {
			key := fmt.Sprintf("gone-%d", i)
			gone = append(gone, key)
			routes["GET /pages/"+key] = notFound
			routes["GET /data_sources/"+key] = notFound
			routes["GET /databases/"+key] = notFound
		}
		handler := recoveryWorkspace(t, searchP1, routes)
		converter := recoveryConverter(t, handler)
		converter.recoverBudget = 3
		converter.SetRecover(gone)

		// when
		sink := driveConverter(t, converter)

		// then: exactly three claims were probed, one ladder each
		probed := map[string]bool{}
		for _, request := range handler.requests() {
			for _, key := range gone {
				if strings.HasSuffix(request, "/"+key) {
					probed[key] = true
				}
			}
		}
		assert.Len(t, probed, 3, "the budget bounds the probing, not the claim count")

		// and: the three probed claims report their own drift, and the five
		// unprobed ones are accounted for ONCE rather than guessed at
		var drift, capped int
		for _, issue := range sink.issues {
			require.Equal(t, importv2.IssueDataLoss, issue.Code, "%v", issue)
			if issue.SourceKey == "" {
				capped++
				assert.Contains(t, issue.Message, "5")
				continue
			}
			drift++
		}
		assert.Equal(t, 3, drift)
		assert.Equal(t, 1, capped, "the remainder is one honest sentence, never silence")
	})

	t.Run("a positively-gone claim still warns once, all three shapes consulted", func(t *testing.T) {
		// given: the third rung must not turn honest drift into a loud failure
		handler := recoveryWorkspace(t, searchP1, map[string]apiResponse{
			"GET /pages/p1":           {body: livePage},
			"GET /blocks/p1/children": {body: `{"results":[],"has_more":false,"next_cursor":null}`},
			"GET /pages/gone":         notFound,
			"GET /data_sources/gone":  notFound,
			"GET /databases/gone":     notFound,
		})
		converter := recoveryConverter(t, handler)
		converter.SetRecover([]string{"gone"})

		// when
		sink := driveConverter(t, converter)

		// then
		require.Len(t, sink.issues, 1)
		assert.Equal(t, importv2.IssueDataLoss, sink.issues[0].Code)
		assert.Equal(t, "gone", sink.issues[0].SourceKey)
	})
}
