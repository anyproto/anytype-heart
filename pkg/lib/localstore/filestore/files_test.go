package filestore

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dsbadgerv3 "github.com/textileio/go-ds-badger3"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore/noctxds"
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

	t.Run("add same file via AddMulti concurrently", func(t *testing.T) {
		store := newFixture(t)
		fileInfo := givenEmptyFileInfo()
		numberOfTimes := 20

		store.addMultiSameFileConcurrently(t, numberOfTimes, fileInfo)

		got, err := store.GetByHash(fileInfo.Hash)
		assert.NoError(t, err)
		assert.Equal(t, fileInfo, got)
	})

	t.Run("add same file key concurrently", func(t *testing.T) {
		store := newFixture(t)
		fileKeys := FileKeys{
			Hash: "target",
			Keys: map[string]string{
				"foo": "bar",
			},
		}
		numberOfTimes := 20

		store.addSameFileKeyConcurrently(t, numberOfTimes, fileKeys)

		got, err := store.GetFileKeys(fileKeys.Hash)
		assert.NoError(t, err)
		assert.Equal(t, fileKeys.Keys, got)
	})

	t.Run("add multiple targets concurrently", func(t *testing.T) {
		store := newFixture(t)
		fileInfo := store.givenEmptyInfoAddedToStore(t)

		wantTargets := givenTargets(100)
		store.addTargetsConcurrently(t, fileInfo, wantTargets)

		got, err := store.GetByHash(fileInfo.Hash)
		require.NoError(t, err)
		assert.ElementsMatch(t, wantTargets, got.Targets)
	})

	t.Run("delete all targets concurrently", func(t *testing.T) {
		store := newFixture(t)
		fileInfo := store.givenFileWithTargets(t, 100)

		store.deleteTargetsConcurrently(t, fileInfo.Targets)

		_, err := store.GetByHash(fileInfo.Hash)
		assert.Equal(t, localstore.ErrNotFound, err)
	})

	t.Run("delete some targets concurrently", func(t *testing.T) {
		store := newFixture(t)
		fileInfo := store.givenFileWithTargets(t, 100)

		var targetsToDelete, targetsToKeep []string
		for i, targetID := range fileInfo.Targets {
			if i%2 == 0 {
				targetsToDelete = append(targetsToDelete, targetID)
			} else {
				targetsToKeep = append(targetsToKeep, targetID)
			}
		}
		store.deleteTargetsConcurrently(t, targetsToDelete)

		got, err := store.GetByHash(fileInfo.Hash)
		assert.NoError(t, err)
		assert.ElementsMatch(t, targetsToKeep, got.Targets)
	})

	t.Run("add targets and get info concurrently", func(t *testing.T) {
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
				_, err := store.GetByHash(fileInfo.Hash)
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

			errs[i] = fx.Add(fileInfo)
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

			err := fx.AddMulti(false, fileInfo)
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()
}

func (fx *fixture) addSameFileKeyConcurrently(t *testing.T, n int, fileKeys FileKeys) {
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
	ds, err := dsbadgerv3.NewDatastore(t.TempDir(), nil)

	require.NoError(t, err)

	store := &dsFileStore{
		ds: noctxds.New(ds),
	}

	return &fixture{
		dsFileStore: store,
	}
}

func (fx *fixture) givenEmptyInfoAddedToStore(t *testing.T) *storage.FileInfo {
	fileInfo := givenEmptyFileInfo()

	err := fx.Add(fileInfo)
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
	targets := givenTargets(numberOfTargets)

	for _, targetID := range targets {
		err := fx.AddFileKeys(FileKeys{Hash: targetID, Keys: map[string]string{"foo": "bar"}})
		assert.NoError(t, err)
	}

	fileInfo := &storage.FileInfo{
		Hash:    "fileId1",
		Key:     "secret1",
		Targets: targets,
	}

	err := fx.Add(fileInfo)
	require.NoError(t, err)

	return fileInfo
}

func (fx *fixture) addTargetsConcurrently(t *testing.T, fileInfo *storage.FileInfo, targets []string) {
	var wg sync.WaitGroup
	for _, targetID := range targets {
		wg.Add(1)
		go func(targetID string) {
			defer wg.Done()
			err := fx.AddFileKeys(FileKeys{Hash: targetID})
			assert.NoError(t, err)

			err = fx.AddTarget(fileInfo.Hash, targetID)
			assert.NoError(t, err)
		}(targetID)
	}
	wg.Wait()
}

func (fx *fixture) deleteTargetsConcurrently(t *testing.T, targets []string) {
	var wg sync.WaitGroup
	for _, targetID := range targets {
		wg.Add(1)
		go func(targetID string) {
			defer wg.Done()

			err := fx.DeleteFile(targetID)
			assert.NoError(t, err)
		}(targetID)
	}
	wg.Wait()
}

func givenTargets(n int) []string {
	var targets []string
	for i := 0; i < n; i++ {
		targets = append(targets, fmt.Sprintf("target%d", i))
	}
	return targets
}
