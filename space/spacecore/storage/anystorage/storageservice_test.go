package anystorage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestService(t *testing.T) *storageService {
	return New(t.TempDir(), &anystore.Config{})
}

// makeUninitializedStore creates the state observed in GO-7393: a valid,
// openable store.db whose mandatory collections were never created (a space
// storage create interrupted by process kill or power loss before the WAL
// was durably synced).
func makeUninitializedStore(t *testing.T, s *storageService, spaceId string) string {
	dirPath := filepath.Join(s.rootPath, spaceId)
	require.NoError(t, os.MkdirAll(dirPath, 0755))
	db, err := anystore.Open(context.Background(), filepath.Join(dirPath, "store.db"), s.anyStoreConfig())
	require.NoError(t, err)
	require.NoError(t, db.Close())
	return dirPath
}

func TestWaitSpaceStorage_Uninitialized(t *testing.T) {
	t.Run("valid but uninitialized store is treated as missing and backed up", func(t *testing.T) {
		// given
		s := newTestService(t)
		const spaceId = "space1"
		dirPath := makeUninitializedStore(t, s, spaceId)

		// when
		st, err := s.WaitSpaceStorage(context.Background(), spaceId)

		// then
		require.ErrorIs(t, err, spacestorage.ErrSpaceStorageMissing)
		require.Nil(t, st)
		// the broken dir is moved aside so a re-download can recreate the space cleanly
		_, statErr := os.Stat(dirPath)
		assert.True(t, os.IsNotExist(statErr), "uninitialized space dir must be moved to backup")
		backups := s.ListCorruptedBackups()
		require.Len(t, backups, 1)
		assert.Equal(t, spaceId, backups[0].SpaceId)
		_, statErr = os.Stat(backups[0].BackupPath)
		assert.NoError(t, statErr, "backup dir must exist")
		// the space now reads as absent, so the caller takes the remote-download path
		assert.False(t, s.SpaceExists(spaceId))
	})

	t.Run("second load attempt returns missing without another backup", func(t *testing.T) {
		// given
		s := newTestService(t)
		const spaceId = "space1"
		makeUninitializedStore(t, s, spaceId)
		_, err := s.WaitSpaceStorage(context.Background(), spaceId)
		require.ErrorIs(t, err, spacestorage.ErrSpaceStorageMissing)

		// when
		_, err = s.WaitSpaceStorage(context.Background(), spaceId)

		// then
		require.ErrorIs(t, err, spacestorage.ErrSpaceStorageMissing)
		assert.Len(t, s.ListCorruptedBackups(), 1)
	})

	t.Run("missing dir still returns missing without backup", func(t *testing.T) {
		// given
		s := newTestService(t)

		// when
		_, err := s.WaitSpaceStorage(context.Background(), "neverExisted")

		// then
		require.ErrorIs(t, err, spacestorage.ErrSpaceStorageMissing)
		assert.Empty(t, s.ListCorruptedBackups())
	})
}

// The backup list is a copy: deleting each backup while ranging over it deletes every one.
func TestDeleteEveryListedBackup(t *testing.T) {
	// given
	root := t.TempDir()
	s := New(root, nil)
	for _, spaceId := range []string{"space1", "space2", "space3"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, spaceId), 0o755))
		_, err := s.backupCorruptedSpace(spaceId)
		require.NoError(t, err)
	}

	// when
	for _, backup := range s.ListCorruptedBackups() {
		require.NoError(t, s.DeleteBackup(backup.BackupPath))
	}

	// then
	assert.Empty(t, s.ListCorruptedBackups())
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestAllSpaceIdsListsSpaceDirectories(t *testing.T) {
	t.Run("directories are spaces, regular files, hidden entries and backups are not", func(t *testing.T) {
		// given
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "space1"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "desktop.ini"), []byte("x"), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(root, ".hidden"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "space3_backup1795086958"), 0o755))
		s := New(root, nil)

		// when
		ids, err := s.AllSpaceIds()

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"space1"}, ids)
	})

	t.Run("a link is a space, whatever it points to", func(t *testing.T) {
		// given: a space moved to another volume and linked back, and a link whose target is gone
		// (an unmounted volume, say): counted, so that nothing takes the space for absent
		root := t.TempDir()
		elsewhere := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(elsewhere, "space2"), 0o755))
		if err := os.Symlink(filepath.Join(elsewhere, "space2"), filepath.Join(root, "space2")); err != nil {
			t.Skipf("links cannot be made here: %v", err)
		}
		require.NoError(t, os.Symlink(filepath.Join(elsewhere, "gone"), filepath.Join(root, "space4")))
		s := New(root, nil)

		// when
		ids, err := s.AllSpaceIds()

		// then
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"space2", "space4"}, ids)
	})
}
