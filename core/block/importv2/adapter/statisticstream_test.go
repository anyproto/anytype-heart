package adapter

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

// The §15 push producer, end to end over the REAL engine: a whole import
// must leave a coherent statistic trail — the phases in order, the
// materialize counters reaching their denominator, and a terminal event
// that is not stuck behind the coalescing window.

func TestImportStatisticStream(t *testing.T) {
	t.Run("a real import pushes phases in order and finishes at its total", func(t *testing.T) {
		// given: the real runEngine over a mock space, one markdown page
		fx := newLifecycleFixture(t)
		spc := mock_clientspace.NewMockSpace(t)
		var minted atomic.Int64
		spc.EXPECT().CreateTreePayload(mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, _ payloadcreator.PayloadCreationParams) (treestorage.TreeStorageCreatePayload, error) {
				return treestorage.TreeStorageCreatePayload{RootRawChange: &treechangeproto.RawTreeChangeWithId{
					Id: fmt.Sprintf("obj-%03d", minted.Add(1)), RawChange: []byte("raw"),
				}}, nil
			}).Maybe()
		spc.EXPECT().DeriveTreePayload(mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, params payloadcreator.PayloadDerivationParams) (treestorage.TreeStorageCreatePayload, error) {
				return treestorage.TreeStorageCreatePayload{RootRawChange: &treechangeproto.RawTreeChangeWithId{
					Id: "drv-" + params.Key.Marshal(), RawChange: []byte("raw"),
				}}, nil
			}).Maybe()
		spc.EXPECT().CreateTreeObjectWithPayload(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, payload treestorage.TreeStorageCreatePayload, initFunc smartblock.InitFunc) (smartblock.SmartBlock, error) {
				id := payload.RootRawChange.Id
				sb := smarttest.New(id)
				if initCtx := initFunc(id); initCtx.State != nil {
					require.NoError(t, sb.Apply(initCtx.State))
				}
				return sb, nil
			}).Maybe()
		fx.service.spaceService = &fakeSpaceGetter{spc: spc}
		fx.service.objectStore = objectstore.NewStoreFixture(t)
		fx.service.installer = fakeInstaller{}
		fx.service.engineRunner = fx.service.runEngine

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "page.md"), []byte("# Hello"), 0o600))
		req := &pb.RpcObjectImportRequest{
			SpaceId:    "space-1",
			Type:       model.Import_Markdown,
			NoProgress: true,
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{
					Path: []string{dir}, NoCollection: true,
				},
			},
		}

		// when
		fx.service.Import(req)
		fx.waitRuns()

		// then: the phase trail is monotone and complete
		events := fx.statistics()
		require.NotEmpty(t, events)
		var phases []pb.EventImportStatisticPhase
		for _, e := range events {
			if len(phases) == 0 || phases[len(phases)-1] != e.Phase {
				phases = append(phases, e.Phase)
			}
		}
		assert.Equal(t, []pb.EventImportStatisticPhase{
			pb.EventImportStatistic_Scanning,
			pb.EventImportStatistic_Fetching,
			pb.EventImportStatistic_Analyzing,
			pb.EventImportStatistic_Fetching,
			pb.EventImportStatistic_Creating,
			pb.EventImportStatistic_Finalizing,
		}, phases)

		// and: the terminal event is flushed past the window, complete, and
		// says cancel would now remove what was created
		last := events[len(events)-1]
		assert.Equal(t, int64(1), last.PagesTotal, "the spool census fixes pass 3's denominator")
		assert.Equal(t, int64(1), last.PagesDone, "materialization reached it")
		assert.Equal(t, int64(1), last.ObjectsCreated)
		assert.True(t, last.TotalsKnown)
		assert.Equal(t, pb.EventImportStatistic_RemovesCreated, last.CancelEffect)
		assert.Equal(t, model.Import_Markdown, last.ImportType)
		assert.NotEmpty(t, last.ImportId, "a durable run is pollable by its id")

		// and: the SCANNING events are the only ones that admit no total —
		// §15.3's count-up, never a fake bar
		for _, e := range events {
			assert.Equal(t, e.Phase != pb.EventImportStatistic_Scanning, e.TotalsKnown,
				"totals become known at the pass-1/pass-2 boundary and stay known")
			assert.LessOrEqual(t, e.PagesDone, e.PagesTotal,
				"a definition must never push done past a denominator that is the claim count")
		}
	})
}

func TestImportStatisticThreeState(t *testing.T) {
	t.Run("a retried Notion request reaches the stream as a state, not an error", func(t *testing.T) {
		// given: a real Notion crawl whose first /search meets a 503.
		// Transient pushback and rate limiting are NORMAL operation for a
		// multi-hour import (§15.2) — the run must report a state, keep
		// going, and never look broken.
		inner := crawlWorkspace(t)
		var flaked atomic.Bool
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/search" && flaked.CompareAndSwap(false, true) {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			inner.ServeHTTP(w, r)
		})
		fx, spc := crawlFixture(t, handler)
		spc.EXPECT().CreateTreePayload(mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, _ payloadcreator.PayloadCreationParams) (treestorage.TreeStorageCreatePayload, error) {
				return treestorage.TreeStorageCreatePayload{RootRawChange: &treechangeproto.RawTreeChangeWithId{
					Id: "obj-p2", RawChange: []byte("raw-p2"),
				}, Heads: []string{"obj-p2"}}, nil
			}).Maybe()
		spc.EXPECT().CreateTreeObjectWithPayload(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, payload treestorage.TreeStorageCreatePayload, initFunc smartblock.InitFunc) (smartblock.SmartBlock, error) {
				sb := smarttest.New(payload.RootRawChange.Id)
				if initCtx := initFunc(payload.RootRawChange.Id); initCtx.State != nil {
					require.NoError(t, sb.Apply(initCtx.State))
				}
				return sb, nil
			}).Maybe()
		makeCrawlRun(t, runstore.RunsRoot(fx.repo), "flaky-crawl", runstore.StateSuspended)

		// when
		fx.service.sweepAbandoned()

		// then: the transient failure travelled as RETRYING with its bounded
		// attempt count, and the run reported healthy again afterwards
		var sawRetrying, recoveredAfter bool
		for _, e := range fx.statistics() {
			if e.State == pb.EventImportStatistic_Retrying {
				sawRetrying = true
				assert.Equal(t, int32(1), e.Attempt)
				assert.Equal(t, int32(2), e.AttemptsMax)
			}
			if sawRetrying && e.State == pb.EventImportStatistic_Running {
				recoveredAfter = true
			}
			assert.NotEqual(t, pb.EventImportStatistic_Error, e.State,
				"a retried request is not the import being wrong")
		}
		assert.True(t, sawRetrying, "the client's retry hook must reach the event stream")
		assert.True(t, recoveredAfter, "recovery is an edge too — the badge has to come off")
	})
}
