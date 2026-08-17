package adapter

import (
	"context"
	"os"
	"path/filepath"
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

	t.Run("a dir killed mid-crawl reports the FETCHING counters, not the materializing ones", func(t *testing.T) {
		// given: the phase a big Notion import spends hours in. The counters
		// are per phase (§15.3), so while fetching they must measure the
		// crawl — spool rows against the pass-1 claim count — and not
		// materialization, which has not started. Serving the materializing
		// column here reported "0 of 2 pages" for a crawl that had fetched
		// two thirds of the workspace.
		fx := newLifecycleFixture(t)
		ctx := context.Background()
		dir := makeCrawlRun(t, runstore.RunsRoot(fx.repo), "crawling", runstore.StateRunning)
		store, err := runstore.Open(ctx, dir)
		require.NoError(t, err)
		require.NoError(t, store.RecordClaims(ctx, []runstore.ClaimRecord{
			{SourceKey: "p2", ObjectId: "obj-p2", PayloadRoot: []byte("r"), PayloadHeads: []string{"obj-p2"}},
			{SourceKey: "p3", ObjectId: "obj-p3", PayloadRoot: []byte("r"), PayloadHeads: []string{"obj-p3"}},
		}))
		spool, err := store.Spool(ctx)
		require.NoError(t, err)
		require.NoError(t, spool.Append(ctx, &importv2.Object{
			SourceKey: "rel-status", SbType: coresb.SmartBlockTypeRelation, Payload: &importv2.Snapshot{},
		}))
		require.NoError(t, spool.Append(ctx, &importv2.Object{
			SourceKey: "img.png", SbType: coresb.SmartBlockTypeFileObject,
			Payload: &importv2.Snapshot{}, File: &importv2.FileSource{Name: "img.png", Path: "/x/img.png"},
		}))
		spillFile := filepath.Join(store.SpillDir(), "spool-123-img.png")
		require.NoError(t, os.WriteFile(spillFile, []byte("downloaded bytes"), 0o600))
		require.NoError(t, store.Close())

		// when
		run, err := fx.service.RunStatus(ctx, "crawling")

		// then
		require.NoError(t, err)
		status := run.Status
		assert.Equal(t, pb.EventImportStatistic_Fetching, status.Phase)
		assert.Equal(t, pb.EventImportStatistic_NothingToUndo, status.CancelEffect)
		assert.Equal(t, int64(3), status.PagesTotal, "the claim count is the fetch denominator")
		assert.Equal(t, int64(1), status.PagesDone, "one page spooled; the relation is a definition")
		assert.Equal(t, int64(1), status.FilesDone)
		assert.Zero(t, status.FilesTotal, "files are found by crawling; 0 means unknown")
		assert.Equal(t, int64(len("downloaded bytes")), status.BytesDone)
		assert.True(t, status.TotalsKnown, "a spool row proves pass 1 finished")
		assert.Zero(t, status.ObjectsCreated, "nothing has entered the space yet")
	})

	t.Run("a dir killed before it spooled anything admits it knows no total", func(t *testing.T) {
		// given: crashed during the /search chain — the claim batch may not
		// even have flushed, and nothing on disk can prove pass 1 ended
		fx := newLifecycleFixture(t)
		ctx := context.Background()
		dir := filepath.Join(runstore.RunsRoot(fx.repo), "scanning")
		store, err := runstore.Create(ctx, dir, runstore.Manifest{
			RunId: "scanning", SpaceId: "space-1", Converter: "Notion",
		})
		require.NoError(t, err)
		require.NoError(t, store.Close())

		// when
		run, err := fx.service.RunStatus(ctx, "scanning")

		// then
		require.NoError(t, err)
		assert.False(t, run.Status.TotalsKnown, "never a fake bar, never a division by zero")
		assert.Zero(t, run.Status.PagesDone)
	})

	t.Run("a mid-crawl dir being compensated does not claim to be finishing up", func(t *testing.T) {
		// given: the phase indicator and the cancel effect are read from
		// different places — the lifecycle label and the materialize marker
		// — and a cancelled or compensating crawl put them in contradiction:
		// "Finishing up" next to "nothing has entered your space yet", and
		// the wrong counter column with them.
		fx := newLifecycleFixture(t)
		ctx := context.Background()
		dir := makeCrawlRun(t, runstore.RunsRoot(fx.repo), "abandoned", runstore.StateCompensating)
		require.DirExists(t, dir)

		// when
		run, err := fx.service.RunStatus(ctx, "abandoned")

		// then
		require.NoError(t, err)
		assert.Equal(t, pb.EventImportStatistic_Fetching, run.Status.Phase,
			"nothing entered the space, so the run never left pass 2")
		assert.Equal(t, pb.EventImportStatistic_NothingToUndo, run.Status.CancelEffect)
		assert.Equal(t, int64(1), run.Status.PagesDone, "the crawl column, not the materialize one")
	})

	t.Run("a pass-3 dir with an empty spool admits it knows no total either", func(t *testing.T) {
		// given — the sibling of review item 12, on the pull side: the crawl
		// branch derives totalsKnown from "the spool has a row" (§15's own
		// as-built rule) and the materializing branch three lines up
		// hard-coded true. The same field then meant one thing on one side of
		// an `if` and another on the other, and a spool-less pass-3 dir
		// answered "known: 0 of 0" where its crawling sibling answers the
		// honest unknown.
		fx := newLifecycleFixture(t)
		ctx := context.Background()
		dir := filepath.Join(runstore.RunsRoot(fx.repo), "empty-pass3")
		store, err := runstore.Create(ctx, dir, runstore.Manifest{
			RunId: "empty-pass3", SpaceId: "space-1", Converter: "Markdown",
		})
		require.NoError(t, err)
		require.NoError(t, store.MarkFetched(ctx, importv2.RootSpec{}))
		require.NoError(t, store.Close())

		// when
		run, err := fx.service.RunStatus(ctx, "empty-pass3")

		// then
		require.NoError(t, err)
		require.Equal(t, pb.EventImportStatistic_Creating, run.Status.Phase)
		assert.False(t, run.Status.TotalsKnown, "never a fake bar, never a division by zero")
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

func TestRunStatusCrossVersion(t *testing.T) {
	t.Run("an older-schema dir is served from its frozen core, not dropped", func(t *testing.T) {
		// given — review P2: buildRunStatus called resume.Load
		// unconditionally, so a v1 dir errored from RunStatus and silently
		// VANISHED from RunList — the exact symptom Class E was raised to
		// fix, through a different door. §4.4 froze the manifest fields for
		// exactly this: any version can always say what a run IS.
		fx := newLifecycleFixture(t)
		dir := makeResumableRun(t, runstore.RunsRoot(fx.repo), "old-run")
		downgradeSchema(t, dir, runstore.SchemaVersion-1)

		// when
		run, err := fx.service.RunStatus(context.Background(), "old-run")

		// then: identity and lifecycle served; ledger-derived numbers absent
		require.NoError(t, err)
		assert.False(t, run.Live)
		assert.Equal(t, string(runstore.StateMaterializing), run.ManifestState)
		assert.Equal(t, "old-run", run.Status.ImportId)
		assert.False(t, run.Status.TotalsKnown)

		// and: the listing includes it
		runs, err := fx.service.RunList(context.Background())
		require.NoError(t, err)
		require.Len(t, runs, 1)
		assert.Equal(t, "old-run", runs[0].Status.ImportId)
	})
}

func TestRunListStrayDir(t *testing.T) {
	t.Run("a stray dir without a db is skipped, and NO db is created into it", func(t *testing.T) {
		// given — review P2: anystore.Open creates run.db where none
		// exists, so a listing MATERIALISED a database into any stray
		// directory under the runs root (a reader must never write).
		fx := newLifecycleFixture(t)
		stray := filepath.Join(runstore.RunsRoot(fx.repo), "stray")
		require.NoError(t, os.MkdirAll(stray, 0o700))

		// when
		runs, err := fx.service.RunList(context.Background())

		// then
		require.NoError(t, err)
		assert.Empty(t, runs)
		_, statErr := os.Stat(filepath.Join(stray, "run.db"))
		assert.True(t, os.IsNotExist(statErr), "a status read must not create a database")
	})
}

func TestRunStatusLive(t *testing.T) {
	t.Run("a running import answers live from its own statistic emitter", func(t *testing.T) {
		// given: a scripted engine parked mid-run, reporting through the seam
		// a real engine reports through. §15.5 serves a live run from the
		// registry snapshot, not from a second derivation over the ledger —
		// the push event and this answer are then the same message.
		fx := newLifecycleFixture(t)
		started := make(chan string, 1)
		barrier := make(chan struct{})
		req := fx.script(func(ctx context.Context, request importv2.Request, converter importv2.Converter, spc clientspace.Space, lc *runLifecycle, progress process.Progress) *importv2.Result {
			require.NoError(t, lc.store.RecordCreated(ctx, "page-1", "obj-1"))
			lc.stats.Phase(importv2.PhaseCreating)
			lc.stats.Discovered(importv2.KindPage, 2)
			lc.stats.Completed(importv2.KindPage, 1)
			lc.stats.Created(1)
			lc.stats.Item("Q3 Planning")
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
		assert.Equal(t, importId, run.Status.ImportId)
		assert.Equal(t, int64(1), run.Status.ObjectsCreated)
		assert.Equal(t, int64(2), run.Status.PagesTotal)
		assert.Equal(t, int64(1), run.Status.PagesDone)
		assert.Equal(t, pb.EventImportStatistic_Creating, run.Status.Phase)
		assert.Equal(t, pb.EventImportStatistic_RemovesCreated, run.Status.CancelEffect)
		assert.Equal(t, "Q3 Planning", run.Status.CurrentItem,
			"currentItem is in-memory only, so only a live run can carry it")

		// and: once the coalescing window closes, the pushed event carries
		// exactly what the poll answered. Coalescing may DELAY the stream;
		// it may never make it contradict the poll, because both are one
		// builder over one state (§15.5).
		require.Eventually(t, func() bool {
			all := fx.statistics()
			return len(all) > 0 && all[len(all)-1].String() == run.Status.String()
		}, 5*time.Second, 10*time.Millisecond)

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
