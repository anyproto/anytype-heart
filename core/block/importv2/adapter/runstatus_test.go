package adapter

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

// The §15 pull surface: dormant runs served from manifest + ledger alone
// (the same reading the pass-3 restart is built on), live runs from the
// running run's own store handle.

func TestRunStatusDormant(t *testing.T) {
	t.Run("a crashed run's dir answers a status poll with ledger-exact numbers", func(t *testing.T) {
		// given: a dir killed mid-materialize — one page done, one file
		// uploaded, one warning recorded, one page pending
		fx := newLifecycleFixture(t)
		ctx := context.Background()
		dir := makeResumableRun(t, runstore.RunsRoot(fx.repo), "crashed")
		store, err := runstore.Open(ctx, dir)
		require.NoError(t, err)
		spool, err := store.Spool(ctx)
		require.NoError(t, err)
		require.NoError(t, spool.Append(ctx, &importv2.Object{
			SourceKey: "pending-page", SbType: coresb.SmartBlockTypePage, Payload: &importv2.Snapshot{},
		}))
		require.NoError(t, spool.Append(ctx, &importv2.Object{
			SourceKey: "img.png", SbType: coresb.SmartBlockTypeFileObject,
			Payload: &importv2.Snapshot{}, File: &importv2.FileSource{Name: "img.png", Path: "/x/img.png"},
		}))
		require.NoError(t, store.RecordClaims(ctx, []runstore.ClaimRecord{{
			SourceKey: "pending-page", ObjectId: "obj-2", PayloadRoot: []byte("r"), PayloadHeads: []string{"obj-2"},
		}}))
		require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-1"))
		require.NoError(t, store.RecordFile(ctx, "img.png", "file-1", false))
		require.NoError(t, store.AppendIssue(ctx, runstore.IssueRecord{
			Severity: int(importv2.SeverityWarning), Code: string(importv2.IssueDataLoss), SourceKey: "page-1",
		}))
		require.NoError(t, store.Close())

		// when
		run, err := fx.service.RunStatus(ctx, "crashed")

		// then: every ledger-backed field is exact (§15.4's dormant column)
		require.NoError(t, err)
		assert.False(t, run.Live)
		assert.Equal(t, string(runstore.StateMaterializing), run.ManifestState)
		status := run.Status
		assert.Equal(t, "crashed", status.ImportId)
		assert.Equal(t, pb.EventImportStatistic_Creating, status.Phase)
		assert.Equal(t, pb.EventImportStatistic_RemovesCreated, status.CancelEffect)
		assert.True(t, status.SafeToClose, "pass-3 restart exists from DM-2: closing loses nothing")
		assert.True(t, status.TotalsKnown)
		assert.Equal(t, int64(2), status.PagesTotal, "the fetched spool fixes the totals")
		assert.Equal(t, int64(1), status.FilesTotal)
		assert.Equal(t, int64(1), status.PagesDone)
		assert.Equal(t, int64(1), status.FilesDone)
		assert.Equal(t, int64(2), status.ObjectsCreated)
		assert.Equal(t, int64(1), status.WarningCount)
		assert.Zero(t, status.ErrorCount)
		assert.False(t, runstore.IsActive(dir), "the status read must release its hold")
	})

	t.Run("an unknown importId is not found", func(t *testing.T) {
		// given
		fx := newLifecycleFixture(t)

		// when
		_, err := fx.service.RunStatus(context.Background(), "no-such-run")

		// then
		assert.ErrorIs(t, err, ErrRunNotFound)
	})
}

func TestRunStatusLive(t *testing.T) {
	t.Run("a running import answers live from its own store handle", func(t *testing.T) {
		// given: a scripted engine parked mid-run with effects recorded
		fx := newLifecycleFixture(t)
		started := make(chan string, 1)
		barrier := make(chan struct{})
		req := fx.script(func(ctx context.Context, request importv2.Request, converter importv2.Converter, spc clientspace.Space, lc *runLifecycle, progress process.Progress) *importv2.Result {
			require.NoError(t, lc.store.RecordCreated(ctx, "page-1", "obj-1"))
			started <- runstore.RunIdOfDir(lc.store.Dir())
			<-barrier
			return &importv2.Result{Created: 1}
		})
		fx.service.Import(req)
		var importId string
		select {
		case importId = <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("import never started")
		}

		// when
		run, err := fx.service.RunStatus(context.Background(), importId)

		// then
		require.NoError(t, err)
		assert.True(t, run.Live)
		assert.Equal(t, model.Import_Markdown, run.Status.ImportType)
		assert.Equal(t, int64(1), run.Status.ObjectsCreated)

		// and: after the run finishes and disposes its dir, the id is gone
		close(barrier)
		fx.waitRuns()
		_, err = fx.service.RunStatus(context.Background(), importId)
		assert.ErrorIs(t, err, ErrRunNotFound)
	})
}

func TestRunList(t *testing.T) {
	t.Run("the listing enumerates dormant dirs", func(t *testing.T) {
		// given
		fx := newLifecycleFixture(t)
		makeResumableRun(t, runstore.RunsRoot(fx.repo), "run-a")
		makeResumableRun(t, runstore.RunsRoot(fx.repo), "run-b")

		// when
		runs, err := fx.service.RunList(context.Background())

		// then
		require.NoError(t, err)
		require.Len(t, runs, 2)
		ids := []string{runs[0].Status.ImportId, runs[1].Status.ImportId}
		assert.ElementsMatch(t, []string{"run-a", "run-b"}, ids)
		for _, run := range runs {
			assert.False(t, run.Live)
			assert.Equal(t, string(runstore.StateMaterializing), run.ManifestState)
		}
	})

	t.Run("an empty root lists nothing", func(t *testing.T) {
		// given
		fx := newLifecycleFixture(t)

		// when
		runs, err := fx.service.RunList(context.Background())

		// then
		require.NoError(t, err)
		assert.Empty(t, runs)
		// and no dir was left marked active
		_, statErr := os.Stat(runstore.RunsRoot(fx.repo))
		assert.True(t, os.IsNotExist(statErr))
	})
}
