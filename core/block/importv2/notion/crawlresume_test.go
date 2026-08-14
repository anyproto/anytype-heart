package notion

import (
	"context"
	"net/http"
	"net/http/httptest"
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
