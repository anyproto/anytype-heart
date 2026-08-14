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
	deleted  []string
	failIds  map[string]error
	panicIds map[string]bool
}

func (d *sweepDeleter) GetObject(ctx context.Context, objectId string) (smartblock.SmartBlock, error) {
	return nil, errors.New("not used")
}

func (d *sweepDeleter) GetObjectByFullID(ctx context.Context, id domain.FullID) (smartblock.SmartBlock, error) {
	return nil, errors.New("not used")
}

func (d *sweepDeleter) DeleteObject(objectId string) error {
	if d.panicIds[objectId] {
		panic("injected sweep delete panic: " + objectId)
	}
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
		outcomes := sweepRuns(ctx, root, deleter, alwaysOK, nil, nil)

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
			// the DM-1 pass-boundary states sweep the same way until DM-2
			// brings the resume branch
			runstore.StateFetched, runstore.StateMaterializing,
		} {
			// given
			root := t.TempDir()
			dir := makeRun(t, root, "run", state, true)
			deleter := &sweepDeleter{}

			// when
			outcomes := sweepRuns(ctx, root, deleter, alwaysOK, nil, nil)

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
		outcomes := sweepRuns(ctx, root, deleter, alwaysOK, nil, nil)

		// then
		require.Len(t, outcomes, 1)
		assert.Equal(t, sweepDeletedCorrupt, outcomes[0].Action)
		assert.Empty(t, deleter.deleted)
		_, err := os.Stat(dir)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("an unreadable run db is kept for the next start, not deleted", func(t *testing.T) {
		// given — P0-3: a permission hiccup (or fd exhaustion, disk full)
		// must not answer as corruption; the sweep would delete the ledger
		// of a run whose db is perfectly intact.
		if os.Geteuid() == 0 {
			t.Skip("chmod-based denial does not bind as root")
		}
		root := t.TempDir()
		dir := makeRun(t, root, "unreadable", runstore.StateRunning, true)
		require.NoError(t, os.Chmod(filepath.Join(dir, "run.db"), 0o000))
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "run.db"), 0o600) })
		deleter := &sweepDeleter{}

		// when
		outcomes := sweepRuns(ctx, root, deleter, alwaysOK, nil, nil)

		// then
		require.Len(t, outcomes, 1)
		assert.Equal(t, sweepSkippedError, outcomes[0].Action)
		assert.Empty(t, deleter.deleted)
		assert.DirExists(t, dir)
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
		outcomes := sweepRuns(ctx, root, &sweepDeleter{}, alwaysOK, nil, nil)

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
		outcomes := sweepRuns(ctx, root, deleter, alwaysOK, nil, nil)

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
			func(context.Context, string) spaceStatus { return spaceGone }, nil, nil)

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
			func(context.Context, string) spaceStatus { return spaceUnknown }, nil, nil)

		// then
		require.Len(t, outcomes, 1)
		assert.Equal(t, sweepSkippedSpaceUnavailable, outcomes[0].Action)
		assert.Empty(t, deleter.deleted)
		assert.DirExists(t, dir)
	})

	t.Run("a leaked delete keeps the dir so the next start retries", func(t *testing.T) {
		// given — P1-1: compensation is idempotent, so retrying next start
		// is free; dropping the dir would turn a retryable leak into a
		// permanent orphan.
		root := t.TempDir()
		dir := makeRun(t, root, "leaky", runstore.StateRunning, true)
		deleter := &sweepDeleter{failIds: map[string]error{"obj-1": assert.AnError}}

		// when
		outcomes := sweepRuns(ctx, root, deleter, alwaysOK, nil, nil)

		// then: partial result, dir kept in the compensating state
		require.Len(t, outcomes, 1)
		assert.Equal(t, sweepCompensatedPartially, outcomes[0].Action)
		assert.Equal(t, 2, outcomes[0].Result.Compensated)
		assert.Equal(t, 1, outcomes[0].Result.Leaked)
		assert.DirExists(t, dir)
		store, err := runstore.Open(ctx, dir)
		require.NoError(t, err)
		manifest, err := store.Manifest(ctx)
		require.NoError(t, err)
		require.NoError(t, store.Close())
		assert.Equal(t, runstore.StateCompensating, manifest.State)

		// and: once the failure clears, the next sweep finishes the job
		// (already-deleted objects count compensated, not leaked)
		deleter.failIds = nil
		outcomes = sweepRuns(ctx, root, deleter, alwaysOK, nil, nil)
		require.Len(t, outcomes, 1)
		assert.Equal(t, sweepCompensated, outcomes[0].Action)
		assert.Zero(t, outcomes[0].Result.Leaked)
		_, statErr := os.Stat(dir)
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("an active run's dir is never touched", func(t *testing.T) {
		// given — P1-2 (confirmed): a second Open of a live run's db
		// succeeds (the .lock is a dirty sentinel, not a mutex) and Drop
		// unlinks the dir under the live writer. Reachable via Close()'s
		// 30s give-up plus a same-process account restart.
		root := t.TempDir()
		dir := filepath.Join(root, "live")
		store, err := runstore.Create(ctx, dir, runstore.Manifest{RunId: "live", SpaceId: "space-1"})
		require.NoError(t, err)
		require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-1"))
		deleter := &sweepDeleter{}

		// when: swept while the run still holds its store open
		outcomes := sweepRuns(ctx, root, deleter, alwaysOK, nil, nil)

		// then
		require.Len(t, outcomes, 1)
		assert.Equal(t, sweepSkippedActive, outcomes[0].Action)
		assert.Empty(t, deleter.deleted)
		assert.DirExists(t, dir)

		// and: once the run lets go, the sweep settles it normally
		require.NoError(t, store.Close())
		outcomes = sweepRuns(ctx, root, deleter, alwaysOK, nil, nil)
		require.Len(t, outcomes, 1)
		assert.Equal(t, sweepCompensated, outcomes[0].Action)
		_, statErr := os.Stat(dir)
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("a dead component context stops the sweep before it touches anything", func(t *testing.T) {
		// given — P1-5: an account stop mid-sweep must not keep deleting
		// through a closing service (every delete would fail and every
		// remaining dir would be dropped anyway).
		root := t.TempDir()
		dir := makeRun(t, root, "run", runstore.StateRunning, true)
		deleter := &sweepDeleter{}
		dead, cancel := context.WithCancel(context.Background())
		cancel()

		// when
		outcomes := sweepRuns(dead, root, deleter, alwaysOK, nil, nil)

		// then
		assert.Empty(t, outcomes)
		assert.Empty(t, deleter.deleted)
		assert.DirExists(t, dir)
	})
}
