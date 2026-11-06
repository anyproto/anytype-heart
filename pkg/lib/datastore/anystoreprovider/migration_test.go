package anystoreprovider

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	anystorev1 "github.com/anyproto/any-store"
	v1enc "github.com/anyproto/any-store/anyenc"
	anystore "github.com/anyproto/any-store/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoremigrate"
)

const migrationSpaceId = "space1"

// seedV1Store writes an object store as a v1 build left it: the common objects.db and a space's
// objects.db and crdt.db, each a v1 database with one collection of n documents.
func seedV1Store(t *testing.T, root string, n int) (objectStorePath string) {
	t.Helper()
	objectStorePath = filepath.Join(root, "objectstore")
	for _, path := range []string{
		filepath.Join(objectStorePath, "objects.db"),
		filepath.Join(objectStorePath, migrationSpaceId, "objects.db"),
		filepath.Join(objectStorePath, migrationSpaceId, "crdt.db"),
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		db, err := anystorev1.Open(context.Background(), path, nil)
		require.NoError(t, err)
		coll, err := db.CreateCollection(context.Background(), "docs")
		require.NoError(t, err)
		arena := &v1enc.Arena{}
		for i := range n {
			arena.Reset()
			doc := arena.NewObject()
			doc.Set("id", arena.NewNumberInt(i))
			doc.Set("value", arena.NewString(path))
			require.NoError(t, coll.Insert(context.Background(), doc))
		}
		require.NoError(t, db.Close())
	}
	return objectStorePath
}

func withFreeSpace(t *testing.T, free uint64) {
	t.Helper()
	was := freeDiskSpace
	freeDiskSpace = func(string) (uint64, error) { return free, nil }
	t.Cleanup(func() { freeDiskSpace = was })
}

func countDocs(t *testing.T, _ *provider, get func() (anystore.DB, error)) int {
	t.Helper()
	db, err := get()
	require.NoError(t, err)
	coll, err := db.OpenCollection(context.Background(), "docs")
	require.NoError(t, err)
	count, err := coll.Count(context.Background())
	require.NoError(t, err)
	return count
}

func TestV1StoreMigratesAtFirstOpen(t *testing.T) {
	// given
	root := t.TempDir()
	objectStorePath := seedV1Store(t, root, 5)
	withFreeSpace(t, 1<<40)

	// when: the start migrates the common database, the space ones migrate as they are opened
	p, err := NewInPath(root)
	require.NoError(t, err)
	prov := p.(*provider)

	// then
	assert.Equal(t, 5, countDocs(t, prov, func() (anystore.DB, error) { return prov.GetCommonDb(), nil }))
	spaceIndex := filepath.Join(objectStorePath, migrationSpaceId, "objects.db")
	isV1, err := anystoremigrate.IsV1File(spaceIndex)
	require.NoError(t, err)
	assert.True(t, isV1, "a space database migrates at its first open, not at the start")

	t.Run("concurrent openers of one space wait for one migration", func(t *testing.T) {
		var wg sync.WaitGroup
		counts := make([]int, 4)
		for i := range counts {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if i%2 == 0 {
					counts[i] = countDocs(t, prov, func() (anystore.DB, error) { return prov.GetSpaceIndexDb(migrationSpaceId) })
				} else {
					counts[i] = countDocs(t, prov, func() (anystore.DB, error) { return prov.GetCrdtDb(migrationSpaceId).Wait() })
				}
			}()
		}
		wg.Wait()
		assert.Equal(t, []int{5, 5, 5, 5}, counts)
	})

	t.Run("the v1 files are kept beside the migrated ones", func(t *testing.T) {
		for _, path := range []string{
			filepath.Join(objectStorePath, "objects.db"),
			filepath.Join(objectStorePath, migrationSpaceId, "objects.db"),
			filepath.Join(objectStorePath, migrationSpaceId, "crdt.db"),
		} {
			isV1, err := anystoremigrate.IsV1File(path + anystoremigrate.V1BackupSuffix)
			require.NoError(t, err)
			assert.True(t, isV1, path)
		}
	})

	t.Run("a start with nothing left to migrate marks the store", func(t *testing.T) {
		require.NoError(t, prov.Close(context.Background()))
		_, err := os.Stat(filepath.Join(objectStorePath, v2Marker))
		require.ErrorIs(t, err, os.ErrNotExist, "the first start still had v1 databases")

		// when
		reopened, err := NewInPath(root)
		require.NoError(t, err)
		defer reopened.Close(context.Background())

		// then
		_, err = os.Stat(filepath.Join(objectStorePath, v2Marker))
		assert.NoError(t, err)
	})
}

func TestV1StoreWithoutEnoughSpaceFailsTheStart(t *testing.T) {
	// given
	root := t.TempDir()
	objectStorePath := seedV1Store(t, root, 5)
	withFreeSpace(t, 1<<20)

	// when
	_, err := NewInPath(root)

	// then
	require.ErrorIs(t, err, anystoremigrate.ErrNotEnoughSpace)
	for _, path := range []string{
		filepath.Join(objectStorePath, "objects.db"),
		filepath.Join(objectStorePath, migrationSpaceId, "objects.db"),
		filepath.Join(objectStorePath, migrationSpaceId, "crdt.db"),
	} {
		isV1, err := anystoremigrate.IsV1File(path)
		require.NoError(t, err)
		assert.True(t, isV1, "nothing is migrated: %s", path)
		_, err = os.Stat(path + anystoremigrate.V1BackupSuffix)
		assert.ErrorIs(t, err, os.ErrNotExist)
	}
}
