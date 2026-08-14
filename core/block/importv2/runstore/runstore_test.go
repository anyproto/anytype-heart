package runstore

import (
	"context"
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
			var violated atomic.Bool
			go func() {
				for {
					select {
					case <-stop:
						return
					default:
					}
					if _, err := os.Stat(dir); err == nil && !IsActive(dir) {
						violated.Store(true)
						return
					}
				}
			}()
			store, err := Create(context.Background(), dir, testManifest())
			require.NoError(t, err)
			close(stop)
			require.NoError(t, store.Close())
			require.False(t, violated.Load(),
				"the dir existed on disk while not registered active (iteration %d)", i)
		}
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
					if _, err := os.Stat(dir); err == nil && !IsActive(dir) {
						violated.Store(true)
						return
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

// TestFrozenCoreFixture pins §4.4's forward-compat promise: the
// compensation-critical fields written at schema v1 must stay readable
// forever. The fixture is a real store committed under testdata; generation
// (RUNSTORE_UPDATE_FIXTURE=1) REFUSES to overwrite an existing fixture —
// the pin is only ever created once per schema version, in a NEW dir.
func TestFrozenCoreFixture(t *testing.T) {
	ctx := context.Background()
	fixtureDir := filepath.Join("testdata", "frozen-v1")

	if os.Getenv("RUNSTORE_UPDATE_FIXTURE") == "1" {
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

	// The committed fixture is read-only for the test: copy it aside so
	// opening (which may write WAL/sentinel files) never dirties testdata.
	workDir := filepath.Join(t.TempDir(), "frozen-v1")
	copyDir(t, fixtureDir, workDir)

	store, err := Open(ctx, workDir)
	require.NoError(t, err)
	defer store.Close()

	manifest, err := store.Manifest(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, manifest.SchemaVersion)
	assert.Equal(t, StateCancelling, manifest.State)
	assert.Equal(t, "space-1", manifest.SpaceId)

	inputs, err := store.CompensationInputs(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"obj-2", "obj-1"}, inputs.Created)
	assert.Equal(t, []string{"file-obj-1"}, inputs.OwnedFiles)
	assert.Equal(t, []string{"obj-3"}, inputs.Updated)
}

// TestFrozenCoreRawFields pins presence AND anyenc type of every frozen
// field (§4.4) by reading dbs raw, independent of what the current reader
// happens to consume. The gap it closes is structural: CompensationInputs
// never reads entries.status, so renaming or retyping it passed the entire
// black-box suite (confirmed by review) — yet the field is frozen and phase
// B will read it. Two subjects, both required:
//   - the committed v1 fixture (the reader-forever half of the freeze);
//   - a store freshly written by TODAY'S writer (the writer half — a field
//     rename in the writer fails here immediately).
func TestFrozenCoreRawFields(t *testing.T) {
	ctx := context.Background()

	t.Run("the committed v1 fixture carries every frozen field", func(t *testing.T) {
		workDir := filepath.Join(t.TempDir(), "frozen-v1")
		copyDir(t, filepath.Join("testdata", "frozen-v1"), workDir)
		assertFrozenFields(t, filepath.Join(workDir, "run.db"))
	})

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
