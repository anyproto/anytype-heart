package anystoreprovider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestProvider(t *testing.T) *provider {
	p, err := NewInPath(t.TempDir())
	require.NoError(t, err)
	prov := p.(*provider)
	t.Cleanup(func() {
		_ = prov.Close(context.Background())
	})
	return prov
}

func TestDeleteSpaceData(t *testing.T) {
	t.Run("closes dbs and removes the space directory", func(t *testing.T) {
		// given
		p := newTestProvider(t)
		const spaceId = "spaceToDelete"

		db, err := p.GetSpaceIndexDb(spaceId)
		require.NoError(t, err)
		require.NotNil(t, db)
		// touch crdt db as well so it gets opened
		crdtDb, err := p.GetCrdtDb(spaceId).Wait()
		require.NoError(t, err)
		require.NotNil(t, crdtDb)

		spacePath := filepath.Join(p.objectStorePath, spaceId)
		_, statErr := os.Stat(spacePath)
		require.NoError(t, statErr, "space dir must exist before deletion")

		// when
		err = p.DeleteSpaceData(spaceId)

		// then
		require.NoError(t, err)
		_, statErr = os.Stat(spacePath)
		assert.True(t, errors.Is(statErr, os.ErrNotExist), "space dir must be gone after deletion")

		// maps no longer track the space
		p.spaceIndexDbsLock.Lock()
		_, hasIndex := p.spaceIndexDbs[spaceId]
		p.spaceIndexDbsLock.Unlock()
		assert.False(t, hasIndex)

		p.crtdStoreLock.Lock()
		_, hasCrdt := p.crdtDbs[spaceId]
		p.crtdStoreLock.Unlock()
		assert.False(t, hasCrdt)
	})

	t.Run("idempotent: deleting a never-opened/absent space is a no-op", func(t *testing.T) {
		// given
		p := newTestProvider(t)

		// when
		err := p.DeleteSpaceData("neverExisted")

		// then
		require.NoError(t, err)
	})

	t.Run("tombstone is lifted after successful deletion so the space can be re-opened", func(t *testing.T) {
		// given
		p := newTestProvider(t)
		const spaceId = "rejoinSpace"
		_, err := p.GetSpaceIndexDb(spaceId)
		require.NoError(t, err)

		// when
		require.NoError(t, p.DeleteSpaceData(spaceId))

		// then: tombstone lifted, fresh open succeeds and recreates the dir
		assert.False(t, p.isSpaceDeleted(spaceId))
		db, err := p.GetSpaceIndexDb(spaceId)
		require.NoError(t, err)
		require.NotNil(t, db)
		_, statErr := os.Stat(filepath.Join(p.objectStorePath, spaceId))
		assert.NoError(t, statErr)
	})
}

// TestDeleteSpaceData_RefusesReopenWhileTombstoned verifies the TOCTOU guard:
// while a space is tombstoned, GetSpaceIndexDb / GetCrdtDb.Wait must refuse to
// reopen (and recreate the just-removed directory of) the space.
func TestDeleteSpaceData_RefusesReopenWhileTombstoned(t *testing.T) {
	t.Run("GetSpaceIndexDb refuses a tombstoned space", func(t *testing.T) {
		p := newTestProvider(t)
		const spaceId = "tombstoned"

		p.deletedSpaceIdsLock.Lock()
		p.deletedSpaceIds[spaceId] = struct{}{}
		p.deletedSpaceIdsLock.Unlock()

		db, err := p.GetSpaceIndexDb(spaceId)
		assert.Nil(t, db)
		assert.ErrorIs(t, err, ErrSpaceDeleted)
		// dir must not be recreated
		_, statErr := os.Stat(filepath.Join(p.objectStorePath, spaceId))
		assert.True(t, errors.Is(statErr, os.ErrNotExist))
	})

	t.Run("a getter handed out before deletion refuses to reopen crdt.db afterwards", func(t *testing.T) {
		p := newTestProvider(t)
		const spaceId = "crdtLeak"

		// Caller obtains the getter BEFORE deletion but has not opened the DB yet.
		getter := p.GetCrdtDb(spaceId)

		// Space is deleted while the caller still holds the getter reference.
		require.NoError(t, p.DeleteSpaceData(spaceId))

		// Calling Wait() now must NOT recreate crdt.db / the directory and must
		// not leak an untracked DB handle.
		db, err := getter.Wait()
		assert.Nil(t, db)
		assert.ErrorIs(t, err, ErrSpaceDeleted)
		_, statErr := os.Stat(filepath.Join(p.objectStorePath, spaceId))
		assert.True(t, errors.Is(statErr, os.ErrNotExist), "crdt.db dir must not be resurrected")
	})

	t.Run("GetCrdtDb during tombstone hands out an already-deleted getter", func(t *testing.T) {
		p := newTestProvider(t)
		const spaceId = "freshCrdtDuringDelete"

		p.deletedSpaceIdsLock.Lock()
		p.deletedSpaceIds[spaceId] = struct{}{}
		p.deletedSpaceIdsLock.Unlock()

		db, err := p.GetCrdtDb(spaceId).Wait()
		assert.Nil(t, db)
		assert.ErrorIs(t, err, ErrSpaceDeleted)
	})
}

// TestDeleteSpaceData_ConcurrentReopen is a race-detector test: a goroutine
// repeatedly tries to reopen the space while another deletes it. The reopen
// must either succeed cleanly (before tombstone / after lift) or be refused
// with ErrSpaceDeleted, but must never produce a data race or a leaked open DB
// handle whose files were removed.
func TestDeleteSpaceData_ConcurrentReopen(t *testing.T) {
	p := newTestProvider(t)
	const spaceId = "raceSpace"
	_, err := p.GetSpaceIndexDb(spaceId)
	require.NoError(t, err)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			db, err := p.GetSpaceIndexDb(spaceId)
			if err == nil && db != nil {
				// fine: either before deletion or after the tombstone lifted
			} else {
				assert.ErrorIs(t, err, ErrSpaceDeleted)
			}
		}
	}()

	require.NoError(t, p.DeleteSpaceData(spaceId))
	close(stop)
	wg.Wait()
}
