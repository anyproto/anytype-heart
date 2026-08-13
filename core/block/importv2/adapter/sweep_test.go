package adapter

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/core/domain"
)

type sweepDeleter struct {
	deleted []string
	failIds map[string]error
}

func (d *sweepDeleter) GetObject(ctx context.Context, objectId string) (smartblock.SmartBlock, error) {
	return nil, errors.New("not used")
}

func (d *sweepDeleter) GetObjectByFullID(ctx context.Context, id domain.FullID) (smartblock.SmartBlock, error) {
	return nil, errors.New("not used")
}

func (d *sweepDeleter) DeleteObject(objectId string) error {
	if err, ok := d.failIds[objectId]; ok {
		return err
	}
	d.deleted = append(d.deleted, objectId)
	return nil
}

func alwaysOK(context.Context, string) spaceStatus { return spaceOK }

// makeRun builds a closed run dir in the given state, optionally with
// recorded effects.
func makeRun(t *testing.T, root, name string, state runstore.State, withEffects bool) string {
	t.Helper()
	ctx := context.Background()
	dir := filepath.Join(root, name)
	store, err := runstore.Create(ctx, dir, runstore.Manifest{RunId: name, SpaceId: "space-1"})
	require.NoError(t, err)
	if withEffects {
		require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-1"))
		require.NoError(t, store.RecordCreated(ctx, "page-2", "obj-2"))
		require.NoError(t, store.RecordUpdated(ctx, "page-3", "obj-3"))
		require.NoError(t, store.RecordFile(ctx, "file-1", "file-obj-1", false))
		require.NoError(t, store.RecordFile(ctx, "file-2", "file-obj-2", true))
	}
	if state != runstore.StateRunning {
		require.NoError(t, store.SetState(ctx, state))
	}
	require.NoError(t, store.Close())
	return dir
}

func TestSweepRuns(t *testing.T) {
	ctx := context.Background()

	t.Run("terminal runs are deleted without touching the space", func(t *testing.T) {
		// given
		root := t.TempDir()
		makeRun(t, root, "done", runstore.StateCompleted, true)
		makeRun(t, root, "lost", runstore.StateFailed, true)
		deleter := &sweepDeleter{}

		// when
		outcomes := sweepRuns(ctx, root, deleter, alwaysOK)

		// then
		require.Len(t, outcomes, 2)
		assert.Empty(t, deleter.deleted)
		for _, outcome := range outcomes {
			assert.Equal(t, sweepDeletedTerminal, outcome.Action)
			_, err := os.Stat(outcome.Dir)
			assert.True(t, os.IsNotExist(err))
		}
	})

	t.Run("crashed, suspended and cancelling runs are compensated newest-first and deleted", func(t *testing.T) {
		for _, state := range []runstore.State{
			runstore.StateRunning, runstore.StateSuspended,
			runstore.StateCancelling, runstore.StateCompensating,
		} {
			// given
			root := t.TempDir()
			dir := makeRun(t, root, "run", state, true)
			deleter := &sweepDeleter{}

			// when
			outcomes := sweepRuns(ctx, root, deleter, alwaysOK)

			// then
			require.Len(t, outcomes, 1, state)
			assert.Equal(t, sweepCompensated, outcomes[0].Action, state)
			assert.Equal(t, []string{"obj-2", "obj-1", "file-obj-1"}, deleter.deleted,
				"created newest-first, then owned files; pre-existing and updated never deleted (%s)", state)
			assert.Equal(t, 3, outcomes[0].Result.Compensated)
			assert.Equal(t, []string{"obj-3"}, outcomes[0].Result.Uncovered)
			_, err := os.Stat(dir)
			assert.True(t, os.IsNotExist(err), state)
		}
	})

	t.Run("a corrupted run db is deleted, loudly, without deletes", func(t *testing.T) {
		// given
		root := t.TempDir()
		dir := filepath.Join(root, "corrupt")
		require.NoError(t, os.MkdirAll(dir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "run.db"), []byte("not a database"), 0o600))
		deleter := &sweepDeleter{}

		// when
		outcomes := sweepRuns(ctx, root, deleter, alwaysOK)

		// then
		require.Len(t, outcomes, 1)
		assert.Equal(t, sweepDeletedCorrupt, outcomes[0].Action)
		assert.Empty(t, deleter.deleted)
		_, err := os.Stat(dir)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("a dir that never got its manifest is deleted", func(t *testing.T) {
		// given: crash between store creation and the manifest write — no
		// manifest means no recorded effects, so the dir is plain garbage.
		root := t.TempDir()
		dir := filepath.Join(root, "half-created")
		require.NoError(t, os.MkdirAll(dir, 0o700))
		db, err := anystore.Open(ctx, filepath.Join(dir, "run.db"), nil)
		require.NoError(t, err)
		require.NoError(t, db.Close())

		// when
		outcomes := sweepRuns(ctx, root, &sweepDeleter{}, alwaysOK)

		// then
		require.Len(t, outcomes, 1)
		assert.Equal(t, sweepDeletedEmpty, outcomes[0].Action)
		_, err = os.Stat(dir)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("a run written by a newer schema is left alone", func(t *testing.T) {
		// given
		root := t.TempDir()
		dir := makeRun(t, root, "future", runstore.StateRunning, true)
		db, err := anystore.Open(ctx, filepath.Join(dir, "run.db"), nil)
		require.NoError(t, err)
		coll, err := db.Collection(ctx, "manifest")
		require.NoError(t, err)
		_, err = coll.UpdateId(ctx, "manifest", query.ModifyFunc(
			func(a *anyenc.Arena, v *anyenc.Value) (*anyenc.Value, bool, error) {
				v.Set("schemaVersion", a.NewNumberInt(runstore.SchemaVersion+1))
				return v, true, nil
			}))
		require.NoError(t, err)
		require.NoError(t, db.Close())
		deleter := &sweepDeleter{}

		// when
		outcomes := sweepRuns(ctx, root, deleter, alwaysOK)

		// then
		require.Len(t, outcomes, 1)
		assert.Equal(t, sweepSkippedNewerSchema, outcomes[0].Action)
		assert.Empty(t, deleter.deleted)
		assert.DirExists(t, dir)
	})

	t.Run("a run whose space is gone is deleted without deletes", func(t *testing.T) {
		// given
		root := t.TempDir()
		dir := makeRun(t, root, "orphan", runstore.StateRunning, true)
		deleter := &sweepDeleter{}

		// when
		outcomes := sweepRuns(ctx, root, deleter,
			func(context.Context, string) spaceStatus { return spaceGone })

		// then
		require.Len(t, outcomes, 1)
		assert.Equal(t, sweepDeletedSpaceGone, outcomes[0].Action)
		assert.Empty(t, deleter.deleted)
		_, err := os.Stat(dir)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("a transient space error keeps the dir for the next start", func(t *testing.T) {
		// given
		root := t.TempDir()
		dir := makeRun(t, root, "flaky", runstore.StateRunning, true)
		deleter := &sweepDeleter{}

		// when
		outcomes := sweepRuns(ctx, root, deleter,
			func(context.Context, string) spaceStatus { return spaceUnknown })

		// then
		require.Len(t, outcomes, 1)
		assert.Equal(t, sweepSkippedSpaceUnavailable, outcomes[0].Action)
		assert.Empty(t, deleter.deleted)
		assert.DirExists(t, dir)
	})

	t.Run("a failed delete leaks loudly but the dir is still removed", func(t *testing.T) {
		// given
		root := t.TempDir()
		dir := makeRun(t, root, "leaky", runstore.StateRunning, true)
		deleter := &sweepDeleter{failIds: map[string]error{"obj-1": assert.AnError}}

		// when
		outcomes := sweepRuns(ctx, root, deleter, alwaysOK)

		// then
		require.Len(t, outcomes, 1)
		assert.Equal(t, sweepCompensated, outcomes[0].Action)
		assert.Equal(t, 2, outcomes[0].Result.Compensated)
		assert.Equal(t, 1, outcomes[0].Result.Leaked)
		_, err := os.Stat(dir)
		assert.True(t, os.IsNotExist(err))
	})
}
