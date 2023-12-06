package filestore

import (
	"fmt"
	"sync"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
)

func TestConflictResolution(t *testing.T) {
	t.Run("add same file concurrently", func(t *testing.T) {
		store := newFixture(t)
		fileInfo := givenEmptyFileInfo()
		numberOfTimes := 20

		errs := store.addSameFileConcurrently(numberOfTimes, fileInfo)

		var noErrors, alreadyExists int
		for _, err := range errs {
			if err == nil {
				noErrors++
			}
			if err == localstore.ErrDuplicateKey {
				alreadyExists++
			}
		}
		assert.Equal(t, 1, noErrors)
		assert.Equal(t, numberOfTimes-1, alreadyExists)
	})

	t.Run("add same file via AddFileVariants concurrently", func(t *testing.T) {
		store := newFixture(t)
		fileInfo := givenEmptyFileInfo()
		numberOfTimes := 20

		store.addMultiSameFileConcurrently(t, numberOfTimes, fileInfo)

		got, err := store.GetFileVariant(domain.FileContentId(fileInfo.Hash))
		assert.NoError(t, err)
		assert.Equal(t, fileInfo, got)
	})

	t.Run("add same file key concurrently", func(t *testing.T) {
		store := newFixture(t)
		fileKeys := domain.FileEncryptionKeys{
			FileId: "target",
			EncryptionKeys: map[string]string{
				"foo": "bar",
			},
		}
		numberOfTimes := 20

		store.addSameFileKeyConcurrently(t, numberOfTimes, fileKeys)

		got, err := store.GetFileKeys(fileKeys.FileId)
		assert.NoError(t, err)
		assert.Equal(t, fileKeys.EncryptionKeys, got)
	})

	t.Run("add multiple files concurrently", func(t *testing.T) {
		store := newFixture(t)
		fileInfo := store.givenEmptyInfoAddedToStore(t)

		wantTargets := givenTargets(100)
		store.addTargetsConcurrently(t, fileInfo, wantTargets)

		got, err := store.GetFileVariant(domain.FileContentId(fileInfo.Hash))
		require.NoError(t, err)
		assert.ElementsMatch(t, wantTargets, got.Targets)
	})

	t.Run("delete all files concurrently", func(t *testing.T) {
		store := newFixture(t)
		fileInfo := store.givenFileWithTargets(t, 100)

		store.deleteTargetsConcurrently(t, fileInfo.Targets)

		_, err := store.GetFileVariant(domain.FileContentId(fileInfo.Hash))
		assert.Equal(t, localstore.ErrNotFound, err)
	})

	t.Run("delete some files concurrently", func(t *testing.T) {
		store := newFixture(t)
		fileInfo := store.givenFileWithTargets(t, 100)

		var fileIdsToDelete, fileIdsToKeep []string
		for i, fileId := range fileInfo.Targets {
			if i%2 == 0 {
				fileIdsToDelete = append(fileIdsToDelete, fileId)
			} else {
				fileIdsToKeep = append(fileIdsToKeep, fileId)
			}
		}
		store.deleteTargetsConcurrently(t, fileIdsToDelete)

		got, err := store.GetFileVariant(domain.FileContentId(fileInfo.Hash))
		assert.NoError(t, err)
		assert.ElementsMatch(t, fileIdsToKeep, got.Targets)
	})

	t.Run("add files and get info concurrently", func(t *testing.T) {
		store := newFixture(t)
		fileInfo := store.givenEmptyInfoAddedToStore(t)

		wantTargets := givenTargets(100)

		var wg sync.WaitGroup

		wg.Add(1)
		go func() {
			defer wg.Done()
			store.addTargetsConcurrently(t, fileInfo, wantTargets)
		}()

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := store.GetFileVariant(domain.FileContentId(fileInfo.Hash))
				assert.NoError(t, err)
			}()
		}
		wg.Wait()
	})
}

func (fx *fixture) addSameFileConcurrently(n int, fileInfo *storage.FileInfo) []error {
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			errs[i] = fx.AddFileVariant(fileInfo)
		}(i)
	}
	wg.Wait()
	return errs
}

func (fx *fixture) addMultiSameFileConcurrently(t *testing.T, n int, fileInfo *storage.FileInfo) {
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			err := fx.AddFileVariants(false, fileInfo)
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()
}

func (fx *fixture) addSameFileKeyConcurrently(t *testing.T, n int, fileKeys domain.FileEncryptionKeys) {
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := fx.AddFileKeys(fileKeys)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}

type fixture struct {
	*dsFileStore
}

func newFixture(t *testing.T) *fixture {
	db, err := badger.Open(badger.DefaultOptions(t.TempDir()))
	require.NoError(t, err)

	store := &dsFileStore{
		db: db,
	}

	return &fixture{
		dsFileStore: store,
	}
}

func (fx *fixture) givenEmptyInfoAddedToStore(t *testing.T) *storage.FileInfo {
	fileInfo := givenEmptyFileInfo()

	err := fx.AddFileVariant(fileInfo)
	require.NoError(t, err)

	return fileInfo
}

func givenEmptyFileInfo() *storage.FileInfo {
	fileInfo := &storage.FileInfo{
		Hash:    "fileId1",
		Key:     "secret1",
		Targets: nil,
	}
	return fileInfo
}

func (fx *fixture) givenFileWithTargets(t *testing.T, numberOfTargets int) *storage.FileInfo {
	fileIds := givenTargets(numberOfTargets)

	for _, fileId := range fileIds {
		err := fx.AddFileKeys(domain.FileEncryptionKeys{FileId: domain.FileId(fileId), EncryptionKeys: map[string]string{"foo": "bar"}})
		assert.NoError(t, err)
	}

	fileInfo := &storage.FileInfo{
		Hash:    "fileId1",
		Key:     "secret1",
		Targets: fileIds,
	}

	err := fx.AddFileVariant(fileInfo)
	require.NoError(t, err)

	return fileInfo
}

func (fx *fixture) addTargetsConcurrently(t *testing.T, fileInfo *storage.FileInfo, fileIds []string) {
	var wg sync.WaitGroup
	for _, fileId := range fileIds {
		wg.Add(1)
		go func(fileId string) {
			defer wg.Done()
			err := fx.AddFileKeys(domain.FileEncryptionKeys{FileId: domain.FileId(fileId)})
			assert.NoError(t, err)

			err = fx.LinkFileVariantToFile(domain.FileId(fileId), domain.FileContentId(fileInfo.Hash))
			assert.NoError(t, err)
		}(fileId)
	}
	wg.Wait()
}

func (fx *fixture) deleteTargetsConcurrently(t *testing.T, fileIds []string) {
	var wg sync.WaitGroup
	for _, fileId := range fileIds {
		wg.Add(1)
		go func(fileId string) {
			defer wg.Done()

			err := fx.DeleteFile(domain.FileId(fileId))
			assert.NoError(t, err)
		}(fileId)
	}
	wg.Wait()
}

func givenTargets(n int) []string {
	var fileIds []string
	for i := 0; i < n; i++ {
		fileIds = append(fileIds, fmt.Sprintf("target%d", i))
	}
	return fileIds
}
