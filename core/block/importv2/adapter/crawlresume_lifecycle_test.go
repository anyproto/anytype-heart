package adapter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	notionclient "github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

// The sweep's crawl-resume branch, driven through the service (DM spec
// §8.3 + the §13.4 harness): a run interrupted mid-crawl re-runs the crawl
// from the manifest's stored request, spends zero requests on recorded
// pages, and materializes the whole recording.

const testNotionToken = "secret-notion-token-bytes"

// crawlWorkspace is a minimal two-page scripted Notion API: p1 was recorded
// by incarnation 1 (fetching it again fails the test — its routes are
// absent on purpose), p2 is the crawl's remainder.
func crawlWorkspace(t *testing.T) *recordingCrawlHandler {
	t.Helper()
	search := `{"results":[
		{"object":"page","id":"p1","parent":{"type":"workspace","workspace":true},
		 "properties":{"Name":{"type":"title","title":[{"plain_text":"One","type":"text"}]}}},
		{"object":"page","id":"p2","parent":{"type":"workspace","workspace":true},
		 "properties":{"Name":{"type":"title","title":[{"plain_text":"Two","type":"text"}]}}}
	],"has_more":false,"next_cursor":null}`
	routes := map[string]string{
		"GET /pages/p2": `{"id":"p2","archived":false,
			"created_time":"2024-02-01T10:00:00.000Z","last_edited_time":"2024-02-02T10:00:00.000Z",
			"properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"Two","type":"text"}]}}}`,
		"GET /blocks/p2/children": `{"results":[],"has_more":false,"next_cursor":null}`,
	}
	handler := &recordingCrawlHandler{}
	handler.inner = func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/search" {
			fmt.Fprint(w, search)
			return
		}
		if response, ok := routes[r.Method+" "+r.URL.Path]; ok {
			fmt.Fprint(w, response)
			return
		}
		t.Errorf("unexpected api call on a resumed crawl: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
	return handler
}

type recordingCrawlHandler struct {
	inner http.HandlerFunc
	mu    sync.Mutex
	seen  []string
}

func (h *recordingCrawlHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.seen = append(h.seen, r.Method+" "+r.URL.Path)
	h.mu.Unlock()
	h.inner(w, r)
}

func (h *recordingCrawlHandler) requests() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.seen...)
}

func notionWireRequest() *pb.RpcObjectImportRequest {
	return &pb.RpcObjectImportRequest{
		SpaceId:    "space-1",
		Type:       model.Import_Notion,
		NoProgress: true,
		Params: &pb.RpcObjectImportRequestParamsOfNotionParams{
			NotionParams: &pb.RpcObjectImportRequestNotionParams{ApiKey: testNotionToken},
		},
	}
}

// makeCrawlRun builds a run dir imitating a kill mid-crawl: one claim with
// its payload, its spool row, NO fetched marker, the request stored.
func makeCrawlRun(t *testing.T, root, name string, state runstore.State) string {
	t.Helper()
	ctx := context.Background()
	requestBlob, err := notionWireRequest().Marshal()
	require.NoError(t, err)
	dir := filepath.Join(root, name)
	store, err := runstore.Create(ctx, dir, runstore.Manifest{
		RunId: name, SpaceId: "space-1", Converter: "Notion",
		ImportType:   int64(model.Import_Notion),
		NoCollection: true,
		Request:      requestBlob,
	})
	require.NoError(t, err)
	require.NoError(t, store.RecordClaims(ctx, []runstore.ClaimRecord{{
		SourceKey: "p1", ObjectId: "obj-p1",
		PayloadRoot: []byte("raw-p1"), PayloadHeads: []string{"obj-p1"},
	}}))
	spool, err := store.Spool(ctx)
	require.NoError(t, err)
	require.NoError(t, spool.Append(ctx, &importv2.Object{
		SourceKey: "p1",
		SbType:    coresb.SmartBlockTypePage,
		Payload:   &importv2.Snapshot{},
	}))
	if state != runstore.StateRunning {
		require.NoError(t, store.SetState(ctx, state))
	}
	require.NoError(t, store.Close())
	return dir
}

// crawlFixture wires the real crawl-resume path over a mock space and the
// scripted API.
func crawlFixture(t *testing.T, handler http.Handler) (*lifecycleFixture, *mock_clientspace.MockSpace) {
	fx, spc := resumeFixture(t)
	fx.service.crawlResumeRunner = fx.service.resumeCrawlRun // the real branch
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	fx.service.notionClientOpts = []notionclient.Option{
		notionclient.WithBaseURL(server.URL),
		notionclient.WithRateLimit(1000),
		notionclient.WithRetryPolicy(notionclient.RetryPolicy{
			MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second,
		}),
	}
	return fx, spc
}

func TestSweepResumesCrawl(t *testing.T) {
	t.Run("a suspended crawl re-crawls from the stored request and converges, fetching nothing twice", func(t *testing.T) {
		// given: a dir a mid-crawl Close left behind, and the live source
		handler := crawlWorkspace(t)
		fx, spc := crawlFixture(t, handler)
		dir := makeCrawlRun(t, runstore.RunsRoot(fx.repo), "susp-crawl", runstore.StateSuspended)
		var createdIds []string
		var mu sync.Mutex
		spc.EXPECT().CreateTreePayload(mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, _ payloadcreator.PayloadCreationParams) (treestorage.TreeStorageCreatePayload, error) {
				return treestorage.TreeStorageCreatePayload{RootRawChange: &treechangeproto.RawTreeChangeWithId{
					Id: "obj-p2", RawChange: []byte("raw-p2"),
				}, Heads: []string{"obj-p2"}}, nil
			}).Once() // exactly ONE mint: the recorded claim must not re-mint
		spc.EXPECT().CreateTreeObjectWithPayload(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, payload treestorage.TreeStorageCreatePayload, initFunc smartblock.InitFunc) (smartblock.SmartBlock, error) {
				mu.Lock()
				createdIds = append(createdIds, payload.RootRawChange.Id)
				mu.Unlock()
				sb := smarttest.New(payload.RootRawChange.Id)
				if initCtx := initFunc(payload.RootRawChange.Id); initCtx.State != nil {
					require.NoError(t, sb.Apply(initCtx.State))
				}
				return sb, nil
			}).Times(2)

		// when
		fx.service.sweepAbandoned()

		// then: the recorded page cost zero requests and kept its LEDGER id
		for _, request := range handler.requests() {
			assert.NotContains(t, []string{"GET /pages/p1", "GET /blocks/p1/children"}, request,
				"a recorded page must not be fetched on resume")
		}
		assert.ElementsMatch(t, []string{"obj-p1", "obj-p2"}, createdIds,
			"the recorded claim's id materializes; only the new page mints")
		_, err := os.Stat(dir)
		assert.True(t, os.IsNotExist(err), "a completed crawl resume disposes the dir")
		assert.Equal(t, 1, fx.finishEvents(), "the resumed finish is delivered like any async run")
		assert.False(t, runstore.IsActive(dir))
	})

	t.Run("a CRASHED crawl (state running) resumes the same way", func(t *testing.T) {
		// given — §8.3 covers crash and suspend alike: running at sweep time
		// IS the crash detector.
		fx, _ := resumeFixture(t)
		dir := makeCrawlRun(t, runstore.RunsRoot(fx.repo), "crashed-crawl", runstore.StateRunning)
		var resumed []string
		var mu sync.Mutex
		fx.service.crawlResumeRunner = func(ctx context.Context, store *runstore.Store, m runstore.Manifest) sweepOutcome {
			mu.Lock()
			resumed = append(resumed, m.RunId)
			mu.Unlock()
			outcome := sweepOutcome{Dir: store.Dir(), Action: sweepResumedCompleted}
			require.NoError(t, store.Drop())
			return outcome
		}

		// when
		fx.service.sweepAbandoned()

		// then
		assert.Equal(t, []string{"crashed-crawl"}, resumed)
		_, err := os.Stat(dir)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("exhausted attempts compensate instead of crawling forever", func(t *testing.T) {
		// given
		fx, _ := resumeFixture(t)
		dir := makeCrawlRun(t, runstore.RunsRoot(fx.repo), "worn-out", runstore.StateSuspended)
		ctx := context.Background()
		store, err := runstore.Open(ctx, dir)
		require.NoError(t, err)
		for i := 0; i < maxResumeAttempts; i++ {
			_, err = store.BeginCrawlResume(ctx)
			require.NoError(t, err)
		}
		require.NoError(t, store.SetState(ctx, runstore.StateSuspended))
		require.NoError(t, store.Close())
		fx.service.crawlResumeRunner = func(context.Context, *runstore.Store, runstore.Manifest) sweepOutcome {
			t.Fatal("an exhausted run must never reach the crawl-resume branch")
			return sweepOutcome{}
		}

		// when
		fx.service.sweepAbandoned()

		// then: compensated to nothing (pass 2 touched no space) and gone
		_, err = os.Stat(dir)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestCrawlResumeTransientKeep(t *testing.T) {
	t.Run("an offline restart keeps the crawl artifact instead of destroying it", func(t *testing.T) {
		// given: the source is unreachable at start — the exact shape of a
		// laptop reopened without wifi after a mid-import quit
		unavailable := &recordingCrawlHandler{}
		unavailable.inner = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		fx, _ := crawlFixture(t, unavailable)
		dir := makeCrawlRun(t, runstore.RunsRoot(fx.repo), "offline", runstore.StateSuspended)

		// when
		fx.service.sweepAbandoned()

		// then: the dir survives, still crawl-resumable, one attempt spent
		store, err := runstore.Open(context.Background(), dir)
		require.NoError(t, err, "the crawl artifact must survive a transient failure")
		defer store.Close()
		manifest, err := store.Manifest(context.Background())
		require.NoError(t, err)
		assert.Equal(t, runstore.StateRunning, manifest.State)
		assert.NotEmpty(t, manifest.Request, "the request must survive for the next attempt")
		assert.Equal(t, 1, manifest.ResumeAttempts, "the attempt stays spent: the cap bounds a never-healing dir")
		assert.Zero(t, fx.finishEvents(), "the import is not over — no finish event")
	})
}

func TestCrawlResumeNonTransientKeep(t *testing.T) {
	t.Run("a NON-retryable mid-crawl failure also keeps the artifact — the rule is structural, not an allowlist", func(t *testing.T) {
		// given — review P0-B's confirmed asymmetry: a rotated token (401)
		// is not in the Notion client's retryable set, and everything
		// outside that set used to destroy the crawl artifact. The rule is
		// the empty journal, not the error's provider shape: nothing is in
		// the space, so the dir survives for the attempts-capped retry —
		// loudly (a finish notification), unlike the quiet transient keep.
		unauthorized := &recordingCrawlHandler{}
		unauthorized.inner = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}
		fx, _ := crawlFixture(t, unauthorized)
		dir := makeCrawlRun(t, runstore.RunsRoot(fx.repo), "rotated-token", runstore.StateSuspended)

		// when
		fx.service.sweepAbandoned()

		// then: dir kept, still crawl-resumable, and the failure was delivered
		store, err := runstore.Open(context.Background(), dir)
		require.NoError(t, err, "a mid-crawl failure must keep the crawl artifact whatever its shape")
		defer store.Close()
		manifest, err := store.Manifest(context.Background())
		require.NoError(t, err)
		assert.Equal(t, runstore.StateRunning, manifest.State)
		assert.NotEmpty(t, manifest.Request, "the request must survive for the next attempt")
		assert.Equal(t, 1, fx.finishEvents(), "a non-transient failure settles loudly, unlike the quiet transient keep")
	})
}

func TestTransientCrawlFailureConsultsTheStop(t *testing.T) {
	t.Run("a user cancel is never transient, however retryable its wrap looks", func(t *testing.T) {
		// given — review P0-C, the exact proven shape: a Notion call
		// abandoned by the user's cancel fails as 'retries exhausted'
		// wrapping a transport context.Canceled, which the retryability
		// rule matches. Produce it with a REAL client whose in-flight
		// request the cancel kills.
		ctx, cancel := context.WithCancel(context.Background())
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cancel()              // the user cancels while the call is in flight
			<-r.Context().Done() // hold until the client abandons the request
		}))
		t.Cleanup(server.Close)
		apiClient := notionclient.NewClient("token",
			notionclient.WithBaseURL(server.URL),
			notionclient.WithRateLimit(1000),
			notionclient.WithRetryPolicy(notionclient.RetryPolicy{
				MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second,
			}))
		callErr := apiClient.Request(ctx, http.MethodPost, "/search", nil, nil)
		require.Error(t, callErr)
		require.True(t, notionclient.IsRetryable(callErr),
			"premise: the cancelled call must be retryable-SHAPED, or this test pins nothing")

		// and a dir still mid-crawl
		fx, _ := resumeFixture(t)
		dir := makeCrawlRun(t, runstore.RunsRoot(fx.repo), "cancelled-crawl", runstore.StateRunning)
		store, err := runstore.Open(context.Background(), dir)
		require.NoError(t, err)
		defer store.Close()

		// when / then: the cancel decides BEFORE the retryability shape
		cancelled := &importv2.Result{Err: importv2.Fatal(importv2.IssueCancelled, callErr)}
		assert.False(t, fx.service.transientCrawlFailure(store, cancelled),
			"a cancelled import must never be kept for a silent restart")

		// and the SAME wrap on a non-cancel fatal stays transient-keepable
		failed := &importv2.Result{Err: importv2.Fatal(importv2.IssueSourceInvalid, callErr)}
		assert.True(t, fx.service.transientCrawlFailure(store, failed))

		// and a suspend is not the user's cancel (Suspended is consulted first)
		suspended := &importv2.Result{Err: importv2.Fatal(importv2.IssueCancelled, callErr), Suspended: true}
		assert.False(t, userCancelled(suspended))
	})
}

func TestCrawlRunStatusSurface(t *testing.T) {
	t.Run("a mid-crawl run is safeToClose and its projection never carries the token", func(t *testing.T) {
		// given
		fx, _ := resumeFixture(t)
		makeCrawlRun(t, runstore.RunsRoot(fx.repo), "mid-crawl", runstore.StateSuspended)

		// when
		run, err := fx.service.RunStatus(context.Background(), "mid-crawl")
		require.NoError(t, err)

		// then
		assert.True(t, run.Status.SafeToClose,
			"DM-3 is what makes closing mid-crawl lossless — the field must say so")
		raw, err := run.Marshal()
		require.NoError(t, err)
		assert.NotContains(t, string(raw), testNotionToken,
			"the request blob must not leak into any projection (OQ2 mitigation 2)")

		// and the same for the list surface
		runs, err := fx.service.RunList(context.Background())
		require.NoError(t, err)
		require.Len(t, runs, 1)
		listRaw, err := runs[0].Marshal()
		require.NoError(t, err)
		assert.NotContains(t, string(listRaw), testNotionToken)
	})

	t.Run("a pre-DM-3 dir without a request stays honestly unsafe to close mid-crawl", func(t *testing.T) {
		// given: the DM-2 dir shape — no stored request, crawl not finished
		fx, _ := resumeFixture(t)
		ctx := context.Background()
		dir := filepath.Join(runstore.RunsRoot(fx.repo), "old-crawl")
		store, err := runstore.Create(ctx, dir, runstore.Manifest{
			RunId: "old-crawl", SpaceId: "space-1", Converter: "Notion",
		})
		require.NoError(t, err)
		_, err = store.Spool(ctx) // every real run opens its spool at engine start
		require.NoError(t, err)
		require.NoError(t, store.SetState(ctx, runstore.StateSuspended))
		require.NoError(t, store.Close())

		// when
		run, err := fx.service.RunStatus(ctx, "old-crawl")
		require.NoError(t, err)

		// then
		assert.False(t, run.Status.SafeToClose,
			"no resume class covers it, so closing still loses the crawl — say so")
	})
}

// TestStoredRequestRoundTrip pins beginRun's half of the contract: what the
// wire delivered is what the manifest carries, byte-exact.
func TestStoredRequestRoundTrip(t *testing.T) {
	t.Run("a fresh import's manifest carries its exact wire request", func(t *testing.T) {
		// given
		fx := newLifecycleFixture(t)
		var stored []byte
		req := fx.script(func(ctx context.Context, request importv2.Request, converter importv2.Converter, spc clientspace.Space, lc *runLifecycle, progress process.Progress) *importv2.Result {
			m, err := lc.store.Manifest(ctx)
			require.NoError(t, err)
			stored = m.Request
			return &importv2.Result{Created: 1}
		})

		// when
		fx.service.Import(req)
		fx.waitRuns()

		// then
		want, err := req.Marshal()
		require.NoError(t, err)
		assert.Equal(t, want, stored)
	})
}
