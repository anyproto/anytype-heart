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
