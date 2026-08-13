package adapter

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
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
		lc, err := s.beginRun(context.Background(), testRequest(), "Notion", 3)

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
		lc, err := s.beginRun(context.Background(), testRequest(), "Markdown", 0)

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
			{Err: importv2.Fatal(importv2.IssueStoreError, assert.AnError)},
			// P1-3's confirmed disagreement scenario: the run aborted (and
			// compensated) for its own reasons, then a Close raced in. The
			// engine's verdict says NOT suspended — the dir must be disposed,
			// never wrongly promoted backwards to "suspended".
			{Err: importv2.Fatal(importv2.IssueObjectFailed, assert.AnError), Suspended: false},
		} {
			// given
			s := &service{config: &config.Config{RepoPath: t.TempDir()}}
			lc, err := s.beginRun(context.Background(), testRequest(), "Markdown", 0)
			require.NoError(t, err)
			dir := lc.store.Dir()

			// when
			s.finishRun(lc, result)

			// then
			_, statErr := os.Stat(dir)
			assert.True(t, os.IsNotExist(statErr))
		}
	})

	t.Run("a suspended run keeps its dir, flushed, in the suspended state", func(t *testing.T) {
		// given — the verdict comes from the engine's Result, the single
		// source of truth (deriving it twice from two contexts disagreed).
		s := &service{config: &config.Config{RepoPath: t.TempDir()}}
		lc, err := s.beginRun(context.Background(), testRequest(), "Notion", 0)
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
