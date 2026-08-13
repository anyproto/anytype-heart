package runstore

import (
	"context"
	"os"
	"path/filepath"
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
// forever. The fixture is a real store committed under testdata; regenerate
// with RUNSTORE_UPDATE_FIXTURE=1 go test -run TestFrozenCoreFixture ./...
// only when creating it for a NEW schema version — never rewrite v1.
func TestFrozenCoreFixture(t *testing.T) {
	ctx := context.Background()
	fixtureDir := filepath.Join("testdata", "frozen-v1")

	if os.Getenv("RUNSTORE_UPDATE_FIXTURE") == "1" {
		require.NoError(t, os.RemoveAll(fixtureDir))
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
