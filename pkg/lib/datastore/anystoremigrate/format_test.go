package anystoremigrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	anystore "github.com/anyproto/any-store/v2"
	"github.com/anyproto/any-store/v2/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// format5Fixture is a database any-store v2.4.3 wrote, in schema format 5: collection "docs" with
// documents {id: 0..4, value: "format 5 document <id>"} and an index on value.
const format5Fixture = "testdata/format5.db"

// openFormat5 copies the format 5 fixture to path and opens it.
func openFormat5(t *testing.T, path string) anystore.DB {
	t.Helper()
	data, err := os.ReadFile(format5Fixture)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	db, err := anystore.Open(ctx, path, nil)
	require.NoError(t, err)
	format, err := anystore.SchemaFormat(db)
	require.NoError(t, err)
	require.Equal(t, 5, format)
	return db
}

func schemaFormat(t *testing.T, db anystore.DB) int {
	t.Helper()
	format, err := anystore.SchemaFormat(db)
	require.NoError(t, err)
	return format
}

// upgradeLeftovers are the files of dir an upgrade makes besides the database's own.
func upgradeLeftovers(t *testing.T, dir string) []string {
	t.Helper()
	var left []string
	for _, name := range dirNames(t, dir) {
		if strings.Contains(name, formatTempSuffix) || strings.Contains(name, ".copy-") {
			left = append(left, name)
		}
	}
	return left
}

func TestUpgradeFormatCopiesTheDatabase(t *testing.T) {
	// given: a format 5 database, and a copy an earlier run left
	dir := t.TempDir()
	path := filepath.Join(dir, "objects.db")
	db := openFormat5(t, path)
	require.NoError(t, os.WriteFile(path+formatTempSuffix, []byte("an older copy"), 0o600))
	m := New(Options{FreeSpace: func(string) (uint64, error) { return 1 << 40, nil }})

	// when
	got, err := m.UpgradeFormat(ctx, db, path, nil, false)

	// then
	require.NoError(t, err)
	require.NotNil(t, got)
	t.Cleanup(func() { require.NoError(t, got.Close()) })
	assert.Equal(t, anystore.LatestSchemaFormat, schemaFormat(t, got))
	coll, err := got.OpenCollection(ctx, "docs")
	require.NoError(t, err)
	count, err := coll.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, count)
	doc, err := coll.FindId(ctx, 3)
	require.NoError(t, err)
	assert.Equal(t, "format 5 document 3", doc.Value().GetString("value"))
	assert.Equal(t, []string{"value"}, indexNames(coll))
	assert.Empty(t, upgradeLeftovers(t, dir))
}

func TestUpgradeFormatResetsTheDatabase(t *testing.T) {
	// given
	dir := t.TempDir()
	path := filepath.Join(dir, "crdt.db")
	db := openFormat5(t, path)
	m := New(Options{FreeSpace: func(string) (uint64, error) { return 1 << 40, nil }})

	// when
	got, err := m.UpgradeFormat(ctx, db, path, nil, true)

	// then: an empty database in the latest format
	require.NoError(t, err)
	require.NotNil(t, got)
	t.Cleanup(func() { require.NoError(t, got.Close()) })
	assert.Equal(t, anystore.LatestSchemaFormat, schemaFormat(t, got))
	names, err := got.GetCollectionNames(ctx)
	require.NoError(t, err)
	assert.Empty(t, names)
	assert.Empty(t, upgradeLeftovers(t, dir))
}

func TestUpgradeFormatOfTheLatestFormatIsANoOp(t *testing.T) {
	for _, reset := range []bool{false, true} {
		// given
		path := filepath.Join(t.TempDir(), "objects.db")
		db, err := anystore.Open(ctx, path, nil)
		require.NoError(t, err)
		coll, err := db.CreateCollection(ctx, "docs")
		require.NoError(t, err)
		require.NoError(t, coll.Insert(ctx, anyenc.MustParseJson(`{"id":"kept"}`)))
		m := New(Options{})

		// when
		got, err := m.UpgradeFormat(ctx, db, path, nil, reset)

		// then: the same handle, with its data
		require.NoError(t, err)
		assert.Same(t, db, got)
		_, err = coll.FindId(ctx, "kept")
		assert.NoError(t, err)
		require.NoError(t, db.Close())
	}
}

func TestUpgradeFormatWithoutEnoughSpace(t *testing.T) {
	// given
	dir := t.TempDir()
	path := filepath.Join(dir, "objects.db")
	db := openFormat5(t, path)
	m := New(Options{FreeSpace: func(string) (uint64, error) { return 1 << 20, nil }})

	// when
	got, err := m.UpgradeFormat(ctx, db, path, nil, false)

	// then: the database goes on, open and in its old format
	require.ErrorIs(t, err, ErrNotEnoughSpace)
	assert.Same(t, db, got)
	t.Cleanup(func() { require.NoError(t, got.Close()) })
	assert.Equal(t, 5, schemaFormat(t, got))
	coll, err := got.OpenCollection(ctx, "docs")
	require.NoError(t, err)
	count, err := coll.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, count)
	assert.Empty(t, upgradeLeftovers(t, dir))
}

func indexNames(coll anystore.Collection) []string {
	var names []string
	for _, idx := range coll.GetIndexes() {
		names = append(names, idx.Info().Name)
	}
	return names
}

// A copy killed after it wrote much leaves files that take the space the next copy needs: they go
// before the space is counted.
func TestUpgradeFormatReclaimsLeftoversBeforeTheSpaceCheck(t *testing.T) {
	// given: a completed copy and a copy's staging files an earlier run left (sparse, they take no
	// real disk), and a volume that has room only once they are gone
	dir := t.TempDir()
	path := filepath.Join(dir, "objects.db")
	db := openFormat5(t, path)
	tmp := path + formatTempSuffix
	leftovers := []string{tmp, tmp + ".copy-staging", tmp + ".copy-staging-wal", tmp + ".copy-lock"}
	for _, f := range leftovers {
		require.NoError(t, os.WriteFile(f, nil, 0o600))
		require.NoError(t, os.Truncate(f, 1<<30))
	}
	// room for the copy (about 200 MB for this small database) only without the 4 GB of leftovers
	const volume = 4<<30 + 100<<20
	m := New(Options{FreeSpace: func(string) (uint64, error) {
		used, err := filesSize(leftovers...)
		require.NoError(t, err)
		return volume - uint64(used), nil
	}})

	// when
	got, err := m.UpgradeFormat(ctx, db, path, nil, false)

	// then
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, got.Close()) })
	assert.Equal(t, anystore.LatestSchemaFormat, schemaFormat(t, got))
	assert.Empty(t, upgradeLeftovers(t, dir))
}

// Documents committed after the last checkpoint are only in the log when the copy starts.
func TestUpgradeFormatCopiesWhatOnlyTheLogHolds(t *testing.T) {
	// given
	dir := t.TempDir()
	path := filepath.Join(dir, "objects.db")
	db := openFormat5(t, path)
	coll, err := db.OpenCollection(ctx, "docs")
	require.NoError(t, err)
	require.NoError(t, coll.Insert(ctx, anyenc.MustParseJson(`{"id":"late","value":"in the log"}`)))
	m := New(Options{FreeSpace: func(string) (uint64, error) { return 1 << 40, nil }})

	// when
	got, err := m.UpgradeFormat(ctx, db, path, nil, false)

	// then
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, got.Close()) })
	coll, err = got.OpenCollection(ctx, "docs")
	require.NoError(t, err)
	doc, err := coll.FindId(ctx, "late")
	require.NoError(t, err)
	assert.Equal(t, "in the log", doc.Value().GetString("value"))
	count, err := coll.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 6, count)
}

// A cancelled upgrade (the provider is closing) replaces nothing: the database goes on as it was.
func TestUpgradeFormatStopsWhenCancelled(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(fmt.Sprintf("reset=%v", reset), func(t *testing.T) {
			// given
			dir := t.TempDir()
			path := filepath.Join(dir, "objects.db")
			db := openFormat5(t, path)
			m := New(Options{FreeSpace: func(string) (uint64, error) { return 1 << 40, nil }})
			cancelled, cancel := context.WithCancel(ctx)
			cancel()

			// when
			got, err := m.UpgradeFormat(cancelled, db, path, nil, reset)

			// then
			require.ErrorIs(t, err, context.Canceled)
			assert.Same(t, db, got)
			t.Cleanup(func() { require.NoError(t, got.Close()) })
			assert.Equal(t, 5, schemaFormat(t, got))
			coll, err := got.OpenCollection(ctx, "docs")
			require.NoError(t, err)
			count, err := coll.Count(ctx)
			require.NoError(t, err)
			assert.Equal(t, 5, count)
			assert.Empty(t, upgradeLeftovers(t, dir))
		})
	}
}

// A cancellation that comes while the upgrade runs stops it at its next check: after the
// checkpoint the database goes on open and unchanged; after the close the files are left as they
// are, the old database checkpointed with every commit, for the next open.
func TestUpgradeFormatCancelledWhileItRuns(t *testing.T) {
	for _, tc := range []struct {
		step         string
		reset        bool
		wantReturned bool
	}{
		{step: formatStepCheckpointed, reset: false, wantReturned: true},
		{step: formatStepCheckpointed, reset: true, wantReturned: true},
		{step: formatStepClosed, reset: false, wantReturned: false},
		{step: formatStepClosed, reset: true, wantReturned: false},
	} {
		t.Run(fmt.Sprintf("%s reset=%v", tc.step, tc.reset), func(t *testing.T) {
			// given: a commit only the log holds
			dir := t.TempDir()
			path := filepath.Join(dir, "objects.db")
			db := openFormat5(t, path)
			coll, err := db.OpenCollection(ctx, "docs")
			require.NoError(t, err)
			require.NoError(t, coll.Insert(ctx, anyenc.MustParseJson(`{"id":"late","value":"in the log"}`)))
			running, cancel := context.WithCancel(ctx)
			defer cancel()
			lateInMainFile := false
			m := New(Options{
				FreeSpace: func(string) (uint64, error) { return 1 << 40, nil },
				formatStep: func(step string) {
					if step == formatStepCheckpointed {
						// the main file alone holds the commit: the checkpoint, not a later close
						lateInMainFile = mainFileHolds(t, path, "late")
					}
					if step == tc.step {
						cancel()
					}
				},
			})

			// when
			got, err := m.UpgradeFormat(running, db, path, nil, tc.reset)

			// then
			require.ErrorIs(t, err, context.Canceled)
			assert.True(t, lateInMainFile, "checkpointed before the close")
			if tc.wantReturned {
				assert.Same(t, db, got, "the database goes on open")
				require.NoError(t, got.Close())
			} else {
				assert.Nil(t, got, "the database was closed and is opened by the next open")
			}
			reopened, err := anystore.Open(ctx, path, nil)
			require.NoError(t, err)
			defer func() { require.NoError(t, reopened.Close()) }()
			assert.Equal(t, 5, schemaFormat(t, reopened), "nothing was replaced")
			coll, err = reopened.OpenCollection(ctx, "docs")
			require.NoError(t, err)
			count, err := coll.Count(ctx)
			require.NoError(t, err)
			assert.Equal(t, 6, count)
			assert.Empty(t, upgradeLeftovers(t, dir))
		})
	}
}

// mainFileHolds reports whether the main file of the database at path, read without its log,
// holds the document id of collection "docs".
func mainFileHolds(t *testing.T, path, id string) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	copyPath := filepath.Join(t.TempDir(), "main-only.db")
	require.NoError(t, os.WriteFile(copyPath, data, 0o600))
	db, err := anystore.Open(ctx, copyPath, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	coll, err := db.OpenCollection(ctx, "docs")
	require.NoError(t, err)
	_, err = coll.FindId(ctx, id)
	return err == nil
}
