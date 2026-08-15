package adapter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func testRequest() importv2.Request {
	return importv2.Request{
		SpaceID:        "space-1",
		Origin:         objectorigin.Import(model.Import_Notion),
		Mode:           importv2.ModeAllOrNothing,
		UpdateExisting: true,
	}
}

func TestBeginRun(t *testing.T) {
	t.Run("durable mode creates a run dir with a running manifest", func(t *testing.T) {
		// given
		repo := t.TempDir()
		s := &service{config: &config.Config{RepoPath: repo}}

		// when
		lc, err := s.beginRun(context.Background(), testRequest(), &pb.RpcObjectImportRequest{}, "Notion", 3, process.NewNoOp())

		// then
		require.NoError(t, err)
		require.NotNil(t, lc.store)
		defer lc.store.Close()
		assert.DirExists(t, lc.spillDir)
		assert.Equal(t, runstore.RunsRoot(repo), filepath.Dir(lc.store.Dir()))
		manifest, err := lc.store.Manifest(context.Background())
		require.NoError(t, err)
		assert.Equal(t, runstore.StateRunning, manifest.State)
		assert.Equal(t, "space-1", manifest.SpaceId)
		assert.Equal(t, int64(model.Import_Notion), manifest.ImportType)
		assert.Equal(t, "Notion", manifest.Converter)
		assert.Equal(t, 3, manifest.PathIndex)
		assert.True(t, manifest.UpdateExisting)
		assert.Equal(t, manifest.RunId, filepath.Base(lc.store.Dir()))
	})

	t.Run("volatile mode without a repo path falls back to a temp spill dir", func(t *testing.T) {
		// given
		s := &service{config: &config.Config{}}

		// when
		lc, err := s.beginRun(context.Background(), testRequest(), &pb.RpcObjectImportRequest{}, "Markdown", 0, process.NewNoOp())

		// then
		require.NoError(t, err)
		assert.Nil(t, lc.store)
		assert.DirExists(t, lc.spillDir)

		// and: finishing cleans the temp dir
		s.finishRun(lc, &importv2.Result{})
		_, statErr := os.Stat(lc.spillDir)
		assert.True(t, os.IsNotExist(statErr))
	})
}

func TestFinishRun(t *testing.T) {
	t.Run("a finished run is disposed whole — success, failure, or an abort that merely raced a Close", func(t *testing.T) {
		for _, result := range []*importv2.Result{
			{},
			// failures model the engine contract: a non-suspend fatal always
			// ran compensation before returning (the disposal invariant keeps
			// the dir otherwise — see the test below)
			{Err: importv2.Fatal(importv2.IssueStoreError, assert.AnError), CompensationRan: true},
			// P1-3's confirmed disagreement scenario: the run aborted (and
			// compensated) for its own reasons, then a Close raced in. The
			// engine's verdict says NOT suspended — the dir must be disposed,
			// never wrongly promoted backwards to "suspended".
			{Err: importv2.Fatal(importv2.IssueObjectFailed, assert.AnError), Suspended: false, CompensationRan: true},
		} {
			// given
			s := &service{config: &config.Config{RepoPath: t.TempDir()}}
			lc, err := s.beginRun(context.Background(), testRequest(), &pb.RpcObjectImportRequest{}, "Markdown", 0, process.NewNoOp())
			require.NoError(t, err)
			dir := lc.store.Dir()

			// when
			s.finishRun(lc, result)

			// then
			_, statErr := os.Stat(dir)
			assert.True(t, os.IsNotExist(statErr))
		}
	})

	t.Run("a failure whose effects no compensation covered keeps the dir", func(t *testing.T) {
		// given — review Class A, the invariant half: a result that never ran
		// compensation (a prologue failure, the engine's nil-spool guard, a
		// gated-out cleanup) must not destroy the dir — its ledger is the
		// only record of what was created.
		s := &service{config: &config.Config{RepoPath: t.TempDir()}}
		lc, err := s.beginRun(context.Background(), testRequest(), &pb.RpcObjectImportRequest{}, "Markdown", 0, process.NewNoOp())
		require.NoError(t, err)
		require.NoError(t, lc.store.RecordCreated(context.Background(), "page-1", "obj-1"))
		dir := lc.store.Dir()

		// when
		s.finishRun(lc, &importv2.Result{
			Err: importv2.Fatal(importv2.IssueStoreError, assert.AnError),
			// CompensationRan false: nothing was undone
		})

		// then: the dir survives, state untouched (the sweep decides), and
		// the store handle is released
		reopened, err := runstore.Open(context.Background(), dir)
		require.NoError(t, err, "the dir must survive a failure that compensated nothing")
		defer reopened.Close()
		manifest, err := reopened.Manifest(context.Background())
		require.NoError(t, err)
		assert.Equal(t, runstore.StateRunning, manifest.State,
			"no state is forced: the sweep chooses resume or compensate")
		inputs, err := reopened.CompensationInputs(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-1"}, inputs.Created, "the ledger's record must be intact")
	})

	t.Run("a mid-crawl failure with nothing to undo keeps the crawl artifact", func(t *testing.T) {
		// given — review P0-B: an abort with an empty journal has nothing to
		// compensate, and the dir IS the crawl artifact DM-3 exists to keep.
		// This is the structural rule, not the Notion-retryability allowlist:
		// noObjects on an eventually-consistent /search, a rotated token, a
		// markdown store failure — all keep the artifact for the sweep's
		// attempts-capped retry.
		s := &service{config: &config.Config{RepoPath: t.TempDir()}}
		lc, err := s.beginRun(context.Background(), testRequest(),
			&pb.RpcObjectImportRequest{SpaceId: "space-1", Type: model.Import_Notion}, "Notion", 0, process.NewNoOp())
		require.NoError(t, err)
		dir := lc.store.Dir()

		// when
		s.finishRun(lc, &importv2.Result{
			Err:           importv2.Fatal(importv2.IssueSourceInvalid, assert.AnError),
			NothingToUndo: true,
		})

		// then: dir kept, state untouched, request intact — still crawl-resumable
		reopened, err := runstore.Open(context.Background(), dir)
		require.NoError(t, err, "a mid-crawl failure must not destroy the crawl artifact")
		defer reopened.Close()
		manifest, err := reopened.Manifest(context.Background())
		require.NoError(t, err)
		assert.Equal(t, runstore.StateRunning, manifest.State)
		assert.NotEmpty(t, manifest.Request, "the request must survive for the crawl resume")
	})

	t.Run("a user cancel with nothing to undo disposes the dir", func(t *testing.T) {
		// given — the cancel carve-out (review P0-C's disposal half): the
		// user discarded the import, nothing is in the space, so keeping the
		// dir would silently resurrect a cancelled import on the next start.
		s := &service{config: &config.Config{RepoPath: t.TempDir()}}
		lc, err := s.beginRun(context.Background(), testRequest(), &pb.RpcObjectImportRequest{}, "Notion", 0, process.NewNoOp())
		require.NoError(t, err)
		dir := lc.store.Dir()

		// when
		s.finishRun(lc, &importv2.Result{
			Err:           importv2.Fatal(importv2.IssueCancelled, context.Canceled),
			Cancelled:     true,
			NothingToUndo: true,
		})

		// then
		_, statErr := os.Stat(dir)
		assert.True(t, os.IsNotExist(statErr), "a cancelled import must not survive as a resumable dir")
	})

	t.Run("a transport timeout wearing the cancel's code keeps the crawl artifact", func(t *testing.T) {
		// given — review item 1, the destructive direction: the Notion
		// client's own http.Client{Timeout: time.Minute} fires on a server
		// hang, classifyFatal painted anything wrapping DeadlineExceeded
		// IssueCancelled, and this disposal read that CODE as the user's
		// intent. A 60-second hang then deleted a two-hour crawl. The stop
		// source, not the shape, decides: nobody cancelled here.
		s := &service{config: &config.Config{RepoPath: t.TempDir()}}
		lc, err := s.beginRun(context.Background(), testRequest(),
			&pb.RpcObjectImportRequest{SpaceId: "space-1", Type: model.Import_Notion}, "Notion", 0, process.NewNoOp())
		require.NoError(t, err)
		dir := lc.store.Dir()

		// when
		s.finishRun(lc, &importv2.Result{
			Err:           importv2.Fatal(importv2.IssueCancelled, fmt.Errorf("search: %w", context.DeadlineExceeded)),
			NothingToUndo: true,
			// Cancelled false: the run context is alive, nobody stopped it
		})

		// then
		reopened, err := runstore.Open(context.Background(), dir)
		require.NoError(t, err, "a network hang must not destroy the crawl artifact")
		defer reopened.Close()
		manifest, err := reopened.Manifest(context.Background())
		require.NoError(t, err)
		assert.NotEmpty(t, manifest.Request, "the dir stays crawl-resumable")
	})

	t.Run("a user cancel with nothing to undo keeps the dir once pass 3 has begun", func(t *testing.T) {
		// given — review item 3: NothingToUndo is an IN-MEMORY oracle (the
		// engine's journal), but the DURABLE compensation scope is the
		// manifest's sticky MaterializeStarted marker. Past it a still-claimed
		// row IS the crash window of a possible create, and
		// runstore.CompensationInputs deletes it — those rows are the only
		// attribution the hollow trees an interrupted pass 3 leaves behind
		// will ever have. A cancel early in pass 3 tears up to workerCount
		// in-flight creates and still finds an empty journal, so the carve-out
		// dropped the dir and the attribution with it.
		ctx := context.Background()
		s := &service{config: &config.Config{RepoPath: t.TempDir()}}
		lc, err := s.beginRun(ctx, testRequest(), &pb.RpcObjectImportRequest{}, "Notion", 0, process.NewNoOp())
		require.NoError(t, err)
		require.NoError(t, lc.store.RecordClaims(ctx, []runstore.ClaimRecord{
			{SourceKey: "p1", ObjectId: "obj-p1", PayloadRoot: []byte("raw-p1"), PayloadHeads: []string{"obj-p1"}},
		}))
		require.NoError(t, lc.store.MarkFetched(ctx, importv2.RootSpec{}))
		dir := lc.store.Dir()

		// when
		s.finishRun(lc, &importv2.Result{
			Err:           importv2.Fatal(importv2.IssueCancelled, context.Canceled),
			Cancelled:     true,
			NothingToUndo: true,
		})

		// then: the dir survives with its claim rows still in the delete set
		reopened, err := runstore.Open(ctx, dir)
		require.NoError(t, err, "an interrupted pass 3 keeps the only record of what it may have created")
		defer reopened.Close()
		inputs, err := reopened.CompensationInputs(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-p1"}, inputs.Created,
			"past MaterializeStarted a claimed row is a possible create, and must stay compensable")
	})

	t.Run("a user cancel whose compensation was GATED still keeps the dir", func(t *testing.T) {
		// given: effects exist (non-empty journal) but the compensating
		// marker could not be written — the disposal invariant outranks the
		// cancel carve-out, because the ledger is the only record of what
		// was created.
		s := &service{config: &config.Config{RepoPath: t.TempDir()}}
		lc, err := s.beginRun(context.Background(), testRequest(), &pb.RpcObjectImportRequest{}, "Notion", 0, process.NewNoOp())
		require.NoError(t, err)
		require.NoError(t, lc.store.RecordCreated(context.Background(), "page-1", "obj-1"))
		dir := lc.store.Dir()

		// when: cancelled, CompensationRan false, NothingToUndo false
		s.finishRun(lc, &importv2.Result{
			Err:       importv2.Fatal(importv2.IssueCancelled, context.Canceled),
			Cancelled: true,
		})

		// then
		reopened, err := runstore.Open(context.Background(), dir)
		require.NoError(t, err, "uncompensated effects outrank the cancel: the dir is the only record")
		defer reopened.Close()
	})

	t.Run("a suspended run keeps its dir, flushed, in the suspended state", func(t *testing.T) {
		// given — the verdict comes from the engine's Result, the single
		// source of truth (deriving it twice from two contexts disagreed).
		s := &service{config: &config.Config{RepoPath: t.TempDir()}}
		lc, err := s.beginRun(context.Background(), testRequest(), &pb.RpcObjectImportRequest{}, "Notion", 0, process.NewNoOp())
		require.NoError(t, err)
		require.NoError(t, lc.store.RecordCreated(context.Background(), "page-1", "obj-1"))
		dir := lc.store.Dir()

		// when
		s.finishRun(lc, &importv2.Result{
			Err:       importv2.Fatal(importv2.IssueCancelled, importv2.ErrSuspended),
			Suspended: true,
		})

		// then: the dir survives and reopens as suspended with effects intact
		reopened, err := runstore.Open(context.Background(), dir)
		require.NoError(t, err)
		defer reopened.Close()
		manifest, err := reopened.Manifest(context.Background())
		require.NoError(t, err)
		assert.Equal(t, runstore.StateSuspended, manifest.State)
		inputs, err := reopened.CompensationInputs(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-1"}, inputs.Created)
	})
}

func TestSuspendRuns(t *testing.T) {
	t.Run("suspendRuns cancels every registered run with the suspend cause", func(t *testing.T) {
		// given
		s := &service{}
		ctx, cancel := context.WithCancelCause(context.Background())
		handle := s.registerRun(cancel)
		defer s.unregisterRun(handle)

		// when
		s.suspendRuns()

		// then
		require.Error(t, ctx.Err())
		assert.ErrorIs(t, context.Cause(ctx), importv2.ErrSuspended)
	})
}
