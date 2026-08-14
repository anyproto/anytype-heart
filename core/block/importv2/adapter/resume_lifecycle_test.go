package adapter

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

// The sweep's resume branch, driven through the service (DM spec §8.1 +
// the §13.4 harness): a run whose pass 2 completed restarts pass 3 from
// its dir; attempts are capped; a pass-2 suspend still compensates.

// makeResumableRun builds a run dir imitating a crash after pass 2: one
// minted claim with its payload, its spool row, the fetched marker.
func makeResumableRun(t *testing.T, root, name string) string {
	t.Helper()
	ctx := context.Background()
	dir := filepath.Join(root, name)
	store, err := runstore.Create(ctx, dir, runstore.Manifest{
		RunId: name, SpaceId: "space-1", Converter: "Markdown", NoCollection: true,
	})
	require.NoError(t, err)
	require.NoError(t, store.RecordClaims(ctx, []runstore.ClaimRecord{{
		SourceKey: "page-1", ObjectId: "obj-1",
		PayloadRoot: []byte("raw-obj-1"), PayloadHeads: []string{"obj-1"},
	}}))
	spool, err := store.Spool(ctx)
	require.NoError(t, err)
	require.NoError(t, spool.Append(ctx, &importv2.Object{
		SourceKey: "page-1",
		SbType:    coresb.SmartBlockTypePage,
		Payload:   &importv2.Snapshot{},
	}))
	require.NoError(t, store.MarkFetched(ctx, importv2.RootSpec{}))
	require.NoError(t, store.Close())
	return dir
}

// resumeFixture wires the real resume path over a mock space.
func resumeFixture(t *testing.T) (*lifecycleFixture, *mock_clientspace.MockSpace) {
	fx := newLifecycleFixture(t)
	spc := mock_clientspace.NewMockSpace(t)
	fx.service.spaceService = &fakeSpaceGetter{spc: spc}
	fx.service.objectStore = objectstore.NewStoreFixture(t)
	fx.service.installer = fakeInstaller{}
	fx.service.notificationsSvc = &fakeNotifications{}
	fx.service.resumeRunner = fx.service.resumeRun // the real branch
	return fx, spc
}

func TestSweepResumesFetchedRun(t *testing.T) {
	t.Run("a fetched run materializes through the sweep, headlessly, end to end", func(t *testing.T) {
		// given: a dir a crashed process left after pass 2, and a space that
		// accepts the recorded create payload
		fx, spc := resumeFixture(t)
		dir := makeResumableRun(t, runstore.RunsRoot(fx.repo), "crashed")
		var createdId string
		spc.EXPECT().CreateTreeObjectWithPayload(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, payload treestorage.TreeStorageCreatePayload, initFunc smartblock.InitFunc) (smartblock.SmartBlock, error) {
				createdId = payload.RootRawChange.Id
				sb := smarttest.New(createdId)
				if initCtx := initFunc(createdId); initCtx.State != nil {
					require.NoError(t, sb.Apply(initCtx.State))
				}
				return sb, nil
			}).Once()

		// when
		fx.service.sweepAbandoned()

		// then: the recorded id materialized (nothing re-minted — the mock
		// has no CreateTreePayload expectation), the dir is disposed, the
		// finish event delivered
		assert.Equal(t, "obj-1", createdId, "the resumed create must reuse the LEDGER's payload")
		_, err := os.Stat(dir)
		assert.True(t, os.IsNotExist(err), "a completed resume disposes the dir")
		assert.Equal(t, 1, fx.finishEvents(), "a resumed finish is delivered like any async run")
		assert.False(t, runstore.IsActive(dir))
	})
}

func TestSweepResumeAttemptCap(t *testing.T) {
	t.Run("exhausted attempts compensate instead of resuming forever", func(t *testing.T) {
		// given: a resumable dir whose attempts are spent (each BeginResume
		// moves the counter durably BEFORE the attempt — a crash loop lands
		// here after maxResumeAttempts tries however early it crashes)
		fx, _ := resumeFixture(t)
		dir := makeResumableRun(t, runstore.RunsRoot(fx.repo), "crashed")
		ctx := context.Background()
		store, err := runstore.Open(ctx, dir)
		require.NoError(t, err)
		for i := 0; i < maxResumeAttempts; i++ {
			_, err = store.BeginResume(ctx)
			require.NoError(t, err)
		}
		require.NoError(t, store.Close())
		fx.service.resumeRunner = func(context.Context, *runstore.Store, runstore.Manifest) sweepOutcome {
			t.Fatal("an exhausted run must never reach the resume branch")
			return sweepOutcome{}
		}
		deleter := fx.service.objects.(*sweepDeleter)

		// when
		fx.service.sweepAbandoned()

		// then: the claimed id is compensated (MaterializeStarted gates it
		// IN — a possible create) and the dir is gone
		assert.Equal(t, []string{"obj-1"}, deleter.deleted)
		_, err = os.Stat(dir)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestSweepSuspendSplit(t *testing.T) {
	t.Run("suspended mid-materialize resumes; suspended mid-crawl compensates", func(t *testing.T) {
		// given — the §7 split: a pass-3 suspend is restartable from the
		// spool; a pass-2 suspend is not until DM-3's crawl seam, so its dir
		// sweeps to nothing (trivially — no effects exist).
		fx, _ := resumeFixture(t)
		ctx := context.Background()
		root := runstore.RunsRoot(fx.repo)
		materialize := makeResumableRun(t, root, "susp-materialize")
		store, err := runstore.Open(ctx, materialize)
		require.NoError(t, err)
		require.NoError(t, store.SetState(ctx, runstore.StateSuspended))
		require.NoError(t, store.Close())
		crawl := filepath.Join(root, "susp-crawl")
		crawlStore, err := runstore.Create(ctx, crawl, runstore.Manifest{RunId: "susp-crawl", SpaceId: "space-1"})
		require.NoError(t, err)
		require.NoError(t, crawlStore.SetState(ctx, runstore.StateSuspended))
		require.NoError(t, crawlStore.Close())

		var mu sync.Mutex
		var resumed []string
		fx.service.resumeRunner = func(ctx context.Context, store *runstore.Store, m runstore.Manifest) sweepOutcome {
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
		assert.Equal(t, []string{"susp-materialize"}, resumed)
		_, err = os.Stat(crawl)
		assert.True(t, os.IsNotExist(err), "a pass-2 suspend sweeps away (nothing to keep)")
	})
}

func TestCloseSuspendsSweepResume(t *testing.T) {
	t.Run("Close mid-resume suspends it: dir kept, no finish event", func(t *testing.T) {
		// given: a resume whose create parks until the run context dies —
		// the shape of a slow materialization meeting a shutdown
		fx, spc := resumeFixture(t)
		dir := makeResumableRun(t, runstore.RunsRoot(fx.repo), "crashed")
		entered := make(chan struct{})
		spc.EXPECT().CreateTreeObjectWithPayload(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, payload treestorage.TreeStorageCreatePayload, initFunc smartblock.InitFunc) (smartblock.SmartBlock, error) {
				close(entered)
				<-ctx.Done()
				return nil, ctx.Err()
			}).Once()
		require.NoError(t, fx.service.Run(context.Background())) // sweep in background
		select {
		case <-entered:
		case <-time.After(10 * time.Second):
			t.Fatal("the sweep resume never reached the space")
		}

		// when
		closeStart := time.Now()
		require.NoError(t, fx.service.Close(context.Background()))

		// then: prompt close, dir kept suspended for the NEXT start, no
		// finish event (the run is not over)
		assert.Less(t, time.Since(closeStart), 10*time.Second)
		assert.Zero(t, fx.finishEvents())
		store, err := runstore.Open(context.Background(), dir)
		require.NoError(t, err, "the dir must survive for the next start's sweep")
		defer store.Close()
		manifest, err := store.Manifest(context.Background())
		require.NoError(t, err)
		assert.Equal(t, runstore.StateSuspended, manifest.State)
		assert.True(t, manifest.MaterializeStarted, "still in the resumable class")
		assert.Equal(t, 1, manifest.ResumeAttempts)
	})
}
