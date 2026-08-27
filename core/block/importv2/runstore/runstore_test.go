package runstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testManifest() Manifest {
	return Manifest{
		RunId:          "run-1",
		SpaceId:        "space-1",
		ImportType:     3,
		Mode:           1,
		UpdateExisting: true,
		NoCollection:   false,
		PathIndex:      2,
		Converter:      "Notion",
		AppVersion:     "v0.test",
	}
}

func createStore(t *testing.T, dir string) *Store {
	t.Helper()
	store, err := Create(context.Background(), dir, testManifest())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestCreateOpen(t *testing.T) {
	t.Run("create writes a running manifest and open reads it back", func(t *testing.T) {
		// given
		dir := filepath.Join(t.TempDir(), "run-1")
		store := createStore(t, dir)
		require.NoError(t, store.Close())

		// when
		reopened, err := Open(context.Background(), dir)

		// then
		require.NoError(t, err)
		defer reopened.Close()
		manifest, err := reopened.Manifest(context.Background())
		require.NoError(t, err)
		want := testManifest()
		want.SchemaVersion = SchemaVersion
		want.State = StateRunning
		want.Incarnation = 1
		manifest.CreatedAt = time.Time{}
		manifest.UpdatedAt = time.Time{}
		assert.Equal(t, want, manifest)
	})

	t.Run("create makes the spill dir inside the run dir", func(t *testing.T) {
		// given
		dir := filepath.Join(t.TempDir(), "run-1")

		// when
		store := createStore(t, dir)

		// then
		info, err := os.Stat(store.SpillDir())
		require.NoError(t, err)
		assert.True(t, info.IsDir())
		assert.Equal(t, dir, filepath.Dir(store.SpillDir()))
	})

	t.Run("open without a manifest fails", func(t *testing.T) {
		// given: a db file that is a valid store but carries no manifest doc
		dir := filepath.Join(t.TempDir(), "run-1")
		require.NoError(t, os.MkdirAll(dir, 0o700))
		db, err := anystore.Open(context.Background(), filepath.Join(dir, "run.db"), nil)
		require.NoError(t, err)
		require.NoError(t, db.Close())

		// when
		_, err = Open(context.Background(), dir)

		// then
		require.Error(t, err)
	})

	t.Run("open on a corrupted db is recognizable", func(t *testing.T) {
		// given
		dir := filepath.Join(t.TempDir(), "run-1")
		require.NoError(t, os.MkdirAll(dir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "run.db"), []byte("this is not a database"), 0o600))

		// when
		_, err := Open(context.Background(), dir)

		// then
		require.Error(t, err)
		assert.True(t, IsCorrupted(err))
	})

	t.Run("an unreadable but intact db is NOT corrupted", func(t *testing.T) {
		// given — P0-3: SQLITE_CANTOPEN also means EACCES, fd exhaustion and
		// some disk-full paths. None of those is a damaged file, and the
		// sweep answers "corrupted" by DELETING the ledger — so a permission
		// hiccup must never classify as corruption.
		if os.Geteuid() == 0 {
			t.Skip("chmod-based denial does not bind as root")
		}
		dir := filepath.Join(t.TempDir(), "run-1")
		store := createStore(t, dir)
		require.NoError(t, store.Close())
		require.NoError(t, os.Chmod(filepath.Join(dir, "run.db"), 0o000))
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "run.db"), 0o600) })

		// when
		_, err := Open(context.Background(), dir)

		// then
		require.Error(t, err)
		assert.False(t, IsCorrupted(err),
			"an intact ledger behind a transient open failure must survive the sweep")
	})
}

func TestSetState(t *testing.T) {
	t.Run("state transition persists and bumps updatedAt", func(t *testing.T) {
		// given
		dir := filepath.Join(t.TempDir(), "run-1")
		store := createStore(t, dir)
		before, err := store.Manifest(context.Background())
		require.NoError(t, err)

		// when
		time.Sleep(1100 * time.Millisecond) // unix-second resolution
		require.NoError(t, store.SetState(context.Background(), StateSuspended))

		// then
		manifest, err := store.Manifest(context.Background())
		require.NoError(t, err)
		assert.Equal(t, StateSuspended, manifest.State)
		assert.True(t, manifest.UpdatedAt.After(before.UpdatedAt))
		assert.Equal(t, before.CreatedAt, manifest.CreatedAt)
	})
}

func TestEffectLedger(t *testing.T) {
	t.Run("compensation inputs return created newest first, owned files only, updated separately", func(t *testing.T) {
		// given
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-1"))
		require.NoError(t, store.RecordCreated(ctx, "page-2", "obj-2"))
		require.NoError(t, store.RecordUpdated(ctx, "page-3", "obj-3"))
		require.NoError(t, store.RecordFile(ctx, "file-1", "file-obj-1", false))
		require.NoError(t, store.RecordFile(ctx, "file-2", "file-obj-2", true)) // pre-existing: never deleted
		require.NoError(t, store.RecordCreated(ctx, "page-4", "obj-4"))
		require.NoError(t, store.RecordFile(ctx, "file-3", "file-obj-3", false))

		// when
		inputs, err := store.CompensationInputs(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-4", "obj-2", "obj-1"}, inputs.Created)
		assert.Equal(t, []string{"file-obj-3", "file-obj-1"}, inputs.OwnedFiles)
		assert.Equal(t, []string{"obj-3"}, inputs.Updated)
	})

	t.Run("effects survive close and reopen", func(t *testing.T) {
		// given
		ctx := context.Background()
		dir := filepath.Join(t.TempDir(), "run-1")
		store := createStore(t, dir)
		require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-1"))
		require.NoError(t, store.RecordFile(ctx, "file-1", "file-obj-1", false))
		require.NoError(t, store.Close())

		// when
		reopened, err := Open(ctx, dir)
		require.NoError(t, err)
		defer reopened.Close()
		inputs, err := reopened.CompensationInputs(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-1"}, inputs.Created)
		assert.Equal(t, []string{"file-obj-1"}, inputs.OwnedFiles)
	})

	t.Run("recording after reopen keeps ordering ahead of prior effects", func(t *testing.T) {
		// given
		ctx := context.Background()
		dir := filepath.Join(t.TempDir(), "run-1")
		store := createStore(t, dir)
		require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-1"))
		require.NoError(t, store.Close())
		reopened, err := Open(ctx, dir)
		require.NoError(t, err)
		defer reopened.Close()

		// when
		require.NoError(t, reopened.RecordCreated(ctx, "page-2", "obj-2"))
		inputs, err := reopened.CompensationInputs(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-2", "obj-1"}, inputs.Created)
	})

	t.Run("an undecodable entries row is skipped, not fatal", func(t *testing.T) {
		// given
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-1"))
		arena := &anyenc.Arena{}
		garbage := arena.NewObject()
		garbage.Set("id", arena.NewString("garbage-row"))
		garbage.Set("mode", arena.NewNumberInt(42)) // mode must be a string
		require.NoError(t, store.entries.UpsertOne(ctx, garbage))

		// when
		inputs, err := store.CompensationInputs(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-1"}, inputs.Created)
	})

	t.Run("a run-created object never leaves the delete set", func(t *testing.T) {
		// given — P1-4 (confirmed): a later effect under the same source key
		// clobbered the created row wholesale, so the object silently left
		// the delete set and would orphan on abort. minted is sticky.
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-1"))

		// when
		require.NoError(t, store.RecordUpdated(ctx, "page-1", "obj-1"))
		inputs, err := store.CompensationInputs(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-1"}, inputs.Created, "minted must be sticky")
		assert.Empty(t, inputs.Updated, "a run-created object is deleted, never reported as an uncovered update")
	})

	t.Run("rank is first-record order and never changes", func(t *testing.T) {
		// given
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordCreated(ctx, "page-a", "obj-a"))
		require.NoError(t, store.RecordCreated(ctx, "page-b", "obj-b"))

		// when: a re-record of page-a must not move it to the front
		require.NoError(t, store.RecordUpdated(ctx, "page-a", "obj-a"))
		inputs, err := store.CompensationInputs(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-b", "obj-a"}, inputs.Created)
	})

	t.Run("a file's first classification wins", func(t *testing.T) {
		// given — a re-recorded file (dedup makes the second upload look
		// pre-existing because the first one indexed it) must keep its
		// honest first classification: owned by this run.
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordFile(ctx, "file-1", "file-obj-1", false))

		// when
		require.NoError(t, store.RecordFile(ctx, "file-1", "file-obj-1", true))
		inputs, err := store.CompensationInputs(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"file-obj-1"}, inputs.OwnedFiles)
	})

	t.Run("a same-key effect with a DIFFERENT objectId is never silently dropped", func(t *testing.T) {
		// given — Invariant 3 (review): the merge rules must not discard an
		// id. A conflicting record indicates an identity violation upstream:
		// keep BOTH ids in the ledger (the displaced one under a synthetic
		// key, preserving its mode so matched ids stay undeletable).
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-a"))

		// when: a second create arrives under the same key with another id
		require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-b"))
		inputs, err := store.CompensationInputs(ctx)

		// then: both ids stay deletable
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"obj-a", "obj-b"}, inputs.Created)

		// and: a matched id displaced by a minted one keeps its matched mode
		require.NoError(t, store.RecordUpdated(ctx, "page-2", "obj-c"))
		require.NoError(t, store.RecordCreated(ctx, "page-2", "obj-d"))
		inputs, err = store.CompensationInputs(ctx)
		require.NoError(t, err)
		assert.Contains(t, inputs.Created, "obj-d")
		assert.Contains(t, inputs.Updated, "obj-c", "the displaced matched id must never become deletable")
	})

	t.Run("an unknown entries mode is deletable to this reader", func(t *testing.T) {
		// given — §4.4 frozen-core reader rule: a mode this binary does not
		// know (a phase-B "derived" read by an older binary) is treated as
		// deletable; a future non-deletable mode must bump schemaVersion.
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		arena := &anyenc.Arena{}
		row := arena.NewObject()
		row.Set("id", arena.NewString("derived-1"))
		row.Set("objectId", arena.NewString("obj-derived"))
		row.Set("mode", arena.NewString("derived"))
		row.Set("status", arena.NewString("persisted"))
		row.Set("rank", arena.NewNumberInt(7))
		require.NoError(t, store.entries.UpsertOne(ctx, row))

		// when
		inputs, err := store.CompensationInputs(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-derived"}, inputs.Created)
		assert.Empty(t, inputs.Updated)
	})
}

func TestActiveRegistry(t *testing.T) {
	t.Run("the registry refcounts holders, and Close is idempotent", func(t *testing.T) {
		// given — Invariant 3: a set-not-refcount registry is disarmed for
		// the still-live holder by any double open (confirmed by review).
		ctx := context.Background()
		dir := filepath.Join(t.TempDir(), "run-1")
		first := createStore(t, dir)
		second, err := Open(ctx, dir)
		require.NoError(t, err)

		// when / then
		assert.True(t, IsActive(dir))
		require.NoError(t, second.Close())
		assert.True(t, IsActive(dir), "the first holder is still live")
		require.NoError(t, first.Close())
		require.NoError(t, first.Close()) // double close must not underflow
		assert.False(t, IsActive(dir))
	})
}

func TestCreateGuardsItsDir(t *testing.T) {
	t.Run("a run dir is active from the moment it exists on disk", func(t *testing.T) {
		// given — C2 (CONFIRMED, 156/200 probe): markActive fired at the END
		// of open, after the dir and db files existed, so a concurrent sweep
		// could unlink a dir beginRun was still creating — the run then
		// wrote its ledger into an unlinked db.
		for i := 0; i < 20; i++ {
			dir := filepath.Join(t.TempDir(), "run-1")
			stop := make(chan struct{})
			watcherDone := make(chan struct{})
			var violated atomic.Bool
			go func() {
				defer close(watcherDone)
				for {
					select {
					case <-stop:
						return
					default:
					}
					// IsActive first: markActive strictly precedes MkdirAll
					// in Create, so an existing-but-inactive observation
					// during the create IS the C2 violation. DOUBLE-CHECKED
					// (a 1-in-60 flake under full-machine load): the watcher
					// can be descheduled between IsActive and Stat long
					// enough for markActive+MkdirAll to BOTH run — the
					// dir-exists then reflects that gap, not a guard hole.
					// The recheck disambiguates soundly: the guard is never
					// released before this goroutine joins, so a dir that
					// exists while STILL inactive is a genuine violation.
					if !IsActive(dir) {
						if _, err := os.Stat(dir); err == nil && !IsActive(dir) {
							violated.Store(true)
							return
						}
					}
				}
			}()
			store, err := Create(context.Background(), dir, testManifest())
			require.NoError(t, err)
			// JOIN the watcher before Close (review Class H TOCTOU): Close
			// legitimately releases the guard while the dir still exists, so
			// an in-flight iteration past its stop check would read that
			// valid end state as a violation.
			close(stop)
			<-watcherDone
			require.NoError(t, store.Close())
			require.False(t, violated.Load(),
				"the dir existed on disk while not registered active (iteration %d)", i)
		}
	})

	t.Run("a failed create leaves no dir behind", func(t *testing.T) {
		// given — CONFIRMED: any post-MkdirAll failure leaked the created
		// run dir (sweepable garbage forever). Make open fail: run.db as a
		// directory.
		dir := filepath.Join(t.TempDir(), "run-1")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "run.db"), 0o700))

		// when
		_, err := Create(context.Background(), dir, testManifest())

		// then
		require.Error(t, err)
		_, statErr := os.Stat(dir)
		assert.True(t, os.IsNotExist(statErr), "Create owns the dir and must remove it on failure")
		assert.False(t, IsActive(dir))
	})

	t.Run("a failed create never leaks an active mark", func(t *testing.T) {
		// given: a parent that is a file, so MkdirAll fails
		parent := filepath.Join(t.TempDir(), "not-a-dir")
		require.NoError(t, os.WriteFile(parent, []byte("x"), 0o600))
		dir := filepath.Join(parent, "run-1")

		// when
		_, err := Create(context.Background(), dir, testManifest())

		// then
		require.Error(t, err)
		assert.False(t, IsActive(dir))
	})
}

func TestOpenExclusive(t *testing.T) {
	t.Run("OpenExclusive refuses while any holder is live, atomically", func(t *testing.T) {
		// given — the sweep's IsActive-then-Open pair is not atomic; DM-2's
		// resume is the first thing that can slip into the gap.
		ctx := context.Background()
		dir := filepath.Join(t.TempDir(), "run-1")
		holder := createStore(t, dir)

		// when / then
		_, err := OpenExclusive(ctx, dir)
		require.ErrorIs(t, err, ErrActive)
		require.NoError(t, holder.Close())
		store, err := OpenExclusive(ctx, dir)
		require.NoError(t, err)
		require.NoError(t, store.Close())
	})
}

func TestIsCorruptedStopShapes(t *testing.T) {
	t.Run("a quick check that died OF the stop is not corruption", func(t *testing.T) {
		// given — review P0-A (reproduced end to end by the reviewers): the
		// quick check runs ONLY on dirty-sentinel dirs — crashed runs, the
		// ones whose ledgers hold uncompensated effects — and any-store
		// wraps EVERY quick-check failure as ErrQuickCheckFailed, including
		// a shutdown's cancellation and its own five-minute cap. Classifying
		// those as corruption made the sweep unlink exactly the ledgers that
		// matter most. The wrap shape below is any-store's own
		// (db.go: fmt.Errorf("%w: %w", ErrQuickCheckFailed, err)).
		for _, tc := range []struct {
			name string
			err  error
			want bool
		}{
			{"cancelled quick check", fmt.Errorf("%w: %w", anystore.ErrQuickCheckFailed, context.Canceled), false},
			{"deadline-capped quick check", fmt.Errorf("%w: %w", anystore.ErrQuickCheckFailed, context.DeadlineExceeded), false},
			{"genuinely failed quick check", fmt.Errorf("%w: %w", anystore.ErrQuickCheckFailed, errors.New("integrity check failed")), true},
			{"plain cancellation", context.Canceled, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				assert.Equal(t, tc.want, IsCorrupted(tc.err))
			})
		}
	})
}

func TestWriteTxSurvivesPanics(t *testing.T) {
	t.Run("a panic mid-transaction releases the write connection", func(t *testing.T) {
		// given — the db has a SINGLE write connection: a transaction leaked
		// by a panic between WriteTx and Commit wedges every later durable
		// write (suspend markers, refunds, effect rows) into an unbounded
		// Background wait — the exact 30s-Close signature of the review's
		// blocker. The ledger writers must release on EVERY exit.
		ctx := context.Background()
		store := createStore(t, filepath.Join(t.TempDir(), "run-1"))
		func() {
			defer func() { require.NotNil(t, recover(), "the injected panic must fire") }()
			_ = store.withWriteTx(ctx, func(txCtx context.Context) error {
				panic("injected mid-tx panic")
			})
		}()

		// when: a later write must acquire the write connection promptly
		writeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		err := store.RecordCreated(writeCtx, "page-1", "obj-1")

		// then
		require.NoError(t, err, "a panicked transaction must not wedge the store")
	})
}

func TestDropGuardOrder(t *testing.T) {
	t.Run("the guard outlives the unlink", func(t *testing.T) {
		// given — C3 (CONFIRMED regression): Close's deferred release fired
		// before RemoveAll, so the dir was briefly unguarded while being
		// deleted. Pin the ordering via a watcher: the dir must never be
		// observable as existing-but-inactive during Drop.
		for i := 0; i < 20; i++ {
			dir := filepath.Join(t.TempDir(), "run-1")
			store := createStore(t, dir)
			stop := make(chan struct{})
			var violated atomic.Bool
			go func() {
				for {
					select {
					case <-stop:
						return
					default:
					}
					// Read IsActive FIRST (review Class H: this test was the
					// phase's second flake — Stat-then-IsActive raced benignly,
					// Drop completing between the two reads). Once observed
					// inactive, correct code has already unlinked (release
					// strictly follows RemoveAll), so a dir that still Stats is
					// a REAL violation — and the broken release-before-unlink
					// shape is exactly what this ordering fires on.
					if !IsActive(dir) {
						if _, err := os.Stat(dir); err == nil {
							violated.Store(true)
							return
						}
					}
				}
			}()
			require.NoError(t, store.Drop())
			close(stop)
			require.False(t, violated.Load(),
				"the dir was existing-but-unguarded during Drop (iteration %d)", i)
		}
	})
}

func TestDrop(t *testing.T) {
	t.Run("drop removes the whole run dir", func(t *testing.T) {
		// given
		dir := filepath.Join(t.TempDir(), "run-1")
		store := createStore(t, dir)
		require.NoError(t, os.WriteFile(filepath.Join(store.SpillDir(), "spilled.bin"), []byte("x"), 0o600))

		// when
		require.NoError(t, store.Drop())

		// then
		_, err := os.Stat(dir)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestListRunDirs(t *testing.T) {
	t.Run("lists run dirs, ignores files, tolerates a missing root", func(t *testing.T) {
		// given
		root := t.TempDir()
		createStore(t, filepath.Join(root, "run-a"))
		createStore(t, filepath.Join(root, "run-b"))
		require.NoError(t, os.WriteFile(filepath.Join(root, "stray-file"), []byte("x"), 0o600))

		// when
		dirs, err := ListRunDirs(root)

		// then
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{
			filepath.Join(root, "run-a"),
			filepath.Join(root, "run-b"),
		}, dirs)

		// and: a root that does not exist yet lists empty, no error
		dirs, err = ListRunDirs(filepath.Join(root, "does-not-exist"))
		require.NoError(t, err)
		assert.Empty(t, dirs)
	})
}

func TestRunsRoot(t *testing.T) {
	assert.Equal(t, filepath.Join("repo", "importv2", "runs"), RunsRoot("repo"))
}

// TestFrozenCoreFixture pins the forward-compat promise: the
// compensation-critical fields written at schema v1 must stay readable
// forever. The fixture is a real store committed under testdata; generation
// (RUNSTORE_UPDATE_FIXTURE=1) REFUSES to overwrite an existing fixture —
// the pin is only ever created once per schema version, in a NEW dir.
// frozenFixtureVersions lists every committed freeze pin. Each schema
// version gets its OWN dir, created once and never regenerated (v2:
// SchemaVersion bumped because derived-claimed rows must not be deleted by
// v1 readers — the writer obligation, honoured mechanically).
var frozenFixtureVersions = []int{1, 2}

func TestFrozenCoreFixture(t *testing.T) {
	ctx := context.Background()

	if os.Getenv("RUNSTORE_UPDATE_FIXTURE") == "1" {
		fixtureDir := filepath.Join("testdata", fmt.Sprintf("frozen-v%d", SchemaVersion))
		if _, err := os.Stat(fixtureDir); err == nil {
			t.Fatalf("refusing to overwrite %s: the freeze pin must never be regenerated in place — a failing freeze test means the CODE broke the freeze; a new schema version gets a NEW fixture dir", fixtureDir)
		}
		store, err := Create(ctx, fixtureDir, testManifest())
		require.NoError(t, err)
		require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-1"))
		require.NoError(t, store.RecordCreated(ctx, "page-2", "obj-2"))
		require.NoError(t, store.RecordUpdated(ctx, "page-3", "obj-3"))
		require.NoError(t, store.RecordFile(ctx, "file-1", "file-obj-1", false))
		require.NoError(t, store.RecordFile(ctx, "file-2", "file-obj-2", true))
		require.NoError(t, store.SetState(ctx, StateCancelling))
		require.NoError(t, store.Flush(ctx))
		require.NoError(t, store.Close())
		// the spill dir holds nothing; drop it so the fixture is just the db
		require.NoError(t, os.RemoveAll(filepath.Join(fixtureDir, "spill")))
	}

	for _, version := range frozenFixtureVersions {
		t.Run(fmt.Sprintf("v%d stays compensable", version), func(t *testing.T) {
			// The committed fixture is read-only for the test: copy it aside
			// so opening (which may write WAL/sentinel files) never dirties
			// testdata.
			name := fmt.Sprintf("frozen-v%d", version)
			workDir := filepath.Join(t.TempDir(), name)
			copyDir(t, filepath.Join("testdata", name), workDir)

			store, err := Open(ctx, workDir)
			require.NoError(t, err)
			defer store.Close()

			manifest, err := store.Manifest(ctx)
			require.NoError(t, err)
			assert.Equal(t, version, manifest.SchemaVersion)
			assert.Equal(t, StateCancelling, manifest.State)
			assert.Equal(t, "space-1", manifest.SpaceId)

			inputs, err := store.CompensationInputs(ctx)
			require.NoError(t, err)
			assert.Equal(t, []string{"obj-2", "obj-1"}, inputs.Created)
			assert.Equal(t, []string{"file-obj-1"}, inputs.OwnedFiles)
			assert.Equal(t, []string{"obj-3"}, inputs.Updated)
		})
	}
}

// TestFrozenCoreRawFields pins presence AND anyenc type of every frozen
// field by reading dbs raw, independent of what the current reader
// happens to consume. The gap it closes is structural: CompensationInputs
// never reads entries.status, so renaming or retyping it passed the entire
// black-box suite (confirmed by review) — yet the field is frozen and phase
// B will read it. Two subjects, both required:
//   - the committed v1 fixture (the reader-forever half of the freeze);
//   - a store freshly written by TODAY'S writer (the writer half — a field
//     rename in the writer fails here immediately).
func TestFrozenCoreRawFields(t *testing.T) {
	ctx := context.Background()

	for _, version := range frozenFixtureVersions {
		t.Run(fmt.Sprintf("the committed v%d fixture carries every frozen field", version), func(t *testing.T) {
			name := fmt.Sprintf("frozen-v%d", version)
			workDir := filepath.Join(t.TempDir(), name)
			copyDir(t, filepath.Join("testdata", name), workDir)
			assertFrozenFields(t, filepath.Join(workDir, "run.db"))
		})
	}

	t.Run("today's writer still writes every frozen field", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "run-1")
		store, err := Create(ctx, dir, testManifest())
		require.NoError(t, err)
		require.NoError(t, store.RecordCreated(ctx, "page-1", "obj-1"))
		require.NoError(t, store.RecordUpdated(ctx, "page-2", "obj-2"))
		require.NoError(t, store.RecordFile(ctx, "file-1", "file-obj-1", false))
		require.NoError(t, store.RecordFile(ctx, "file-2", "file-obj-2", true))
		require.NoError(t, store.Close())
		assertFrozenFields(t, filepath.Join(dir, "run.db"))
	})
}

func assertFrozenFields(t *testing.T, dbPath string) {
	t.Helper()
	ctx := context.Background()
	db, err := anystore.Open(ctx, dbPath, nil)
	require.NoError(t, err)
	defer db.Close()

	boolTypes := []string{"true", "false"}
	frozen := map[string]map[string][]string{
		"manifest": {
			"schemaVersion": {"number"},
			"state":         {"string"},
			"spaceId":       {"string"},
		},
		"entries": {
			"id":       {"string"},
			"objectId": {"string"},
			"mode":     {"string"},
			"status":   {"string"},
			"rank":     {"number"},
		},
		"files": {
			"id":          {"string"},
			"objectId":    {"string"},
			"status":      {"string"},
			"preExisting": boolTypes,
			"rank":        {"number"},
		},
	}
	for collName, fields := range frozen {
		coll, err := db.OpenCollection(ctx, collName)
		require.NoError(t, err, "frozen collection %q must exist", collName)
		iter, err := coll.Find(nil).Iter(ctx)
		require.NoError(t, err)
		rows := 0
		for iter.Next() {
			doc, err := iter.Doc()
			require.NoError(t, err)
			rows++
			for field, allowedTypes := range fields {
				value := doc.Value().Get(field)
				require.NotNil(t, value, "frozen field %s.%s missing", collName, field)
				assert.Contains(t, allowedTypes, value.Type().String(),
					"frozen field %s.%s changed type", collName, field)
			}
		}
		require.NoError(t, iter.Close())
		require.Positive(t, rows, "db must carry %s rows", collName)
	}
}

func copyDir(t *testing.T, from, to string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(to, 0o700))
	dirEntries, err := os.ReadDir(from)
	require.NoError(t, err)
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() {
			copyDir(t, filepath.Join(from, dirEntry.Name()), filepath.Join(to, dirEntry.Name()))
			continue
		}
		data, err := os.ReadFile(filepath.Join(from, dirEntry.Name()))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(to, dirEntry.Name()), data, 0o600))
	}
}

func TestOpCtxBudget(t *testing.T) {
	t.Run("a nested op SHARES the enclosing budget instead of minting a fresh one", func(t *testing.T) {
		// given — review P1: opCtx used WithoutCancel, which drops the
		// deadline along with the cancellation, so every NESTED op got a
		// fresh full dbOpTimeout. SetState (Manifest + writeManifest) alone
		// compounded to 3x; the crawl-resume suspend settlement compounded
		// to ~75s of cancellation-immune work against a 30s close grace.
		// The dbOpTimeout comment ("deliberately UNDER the close grace")
		// was true per-op, false per-settlement.
		outer, outerDone := context.WithTimeout(context.Background(), 3*time.Second)
		defer outerDone()
		outerDeadline, _ := outer.Deadline()

		// when
		nested, nestedDone := opCtx(outer)
		defer nestedDone()

		// then
		nestedDeadline, ok := nested.Deadline()
		require.True(t, ok)
		assert.Equal(t, outerDeadline, nestedDeadline,
			"a nested op must inherit the enclosing budget, not extend it")
	})

	t.Run("cancellation stays stripped whatever the deadline does", func(t *testing.T) {
		// given — the connection-leak rule this machinery exists for: the
		// operation itself runs undisturbed; cancellation is honored only
		// BETWEEN operations. A deadline must never smuggle the parent's
		// cancel back in (a status poll's dead RPC ctx would wedge the live
		// run's single read connection).
		parent, cancel := context.WithTimeout(context.Background(), 3*time.Second)

		// when
		op, opDone := opCtx(parent)
		defer opDone()
		cancel()

		// then
		assert.NoError(t, op.Err(), "the parent's cancellation must not reach the op")
	})

	t.Run("no enclosing deadline mints the standard per-op budget", func(t *testing.T) {
		// when
		op, opDone := opCtx(context.Background())
		defer opDone()

		// then
		deadline, ok := op.Deadline()
		require.True(t, ok)
		remaining := time.Until(deadline)
		assert.Greater(t, remaining, dbOpTimeout-time.Second)
		assert.LessOrEqual(t, remaining, dbOpTimeout)
	})

	t.Run("an ALREADY-SPENT enclosing budget mints a fresh one, never a born-dead ctx", func(t *testing.T) {
		// given — review item 10: the inherit test is `time.Until(deadline) <
		// dbOpTimeout`, which is true for NEGATIVE remainders too, so an
		// enclosing budget a composite's earlier nested op had already spent
		// was re-imposed verbatim. The op then ran on a context that was dead
		// before its first statement — exactly the input any-store v0.4.7
		// leaks a read connection on, which is the whole reason this
		// detachment exists.
		expired, expiredDone := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer expiredDone()
		require.Error(t, expired.Err(), "premise: the enclosing budget is spent")

		// when
		op, opDone := opCtx(expired)
		defer opDone()

		// then
		require.NoError(t, op.Err(), "an op must never begin on a dead context")
		deadline, ok := op.Deadline()
		require.True(t, ok)
		assert.Positive(t, time.Until(deadline))
		assert.LessOrEqual(t, time.Until(deadline), dbOpTimeout)
	})

	t.Run("an enclosing deadline LOOSER than the op budget is capped at the op budget", func(t *testing.T) {
		// given: an outside caller with a long budget (an RPC with a
		// 10-minute deadline) must not stretch one db op to match it.
		outer, outerDone := context.WithTimeout(context.Background(), 10*time.Minute)
		defer outerDone()

		// when
		op, opDone := opCtx(outer)
		defer opDone()

		// then
		deadline, ok := op.Deadline()
		require.True(t, ok)
		assert.LessOrEqual(t, time.Until(deadline), dbOpTimeout)
	})
}
