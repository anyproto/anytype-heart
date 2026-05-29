package objectstore

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

func TestDeleteSpaceIndex(t *testing.T) {
	t.Run("empty spaceId returns error", func(t *testing.T) {
		s := NewStoreFixture(t)
		err := s.DeleteSpaceIndex("")
		require.Error(t, err)
	})

	t.Run("removes space from registry, opened set and disk", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		const spaceId = "spaceToDelete"
		s.AddObjects(t, spaceId, []TestObject{
			{
				bundle.RelationKeyId:   domain.String("obj1"),
				bundle.RelationKeyName: domain.String("Object 1"),
			},
		})
		require.Contains(t, s.OpenedSpaceIds(), spaceId)
		ids, err := s.anystoreProvider.ListSpaceIdsFromFilesystem()
		require.NoError(t, err)
		require.Contains(t, ids, spaceId)

		// when
		err = s.DeleteSpaceIndex(spaceId)

		// then
		require.NoError(t, err)
		assert.NotContains(t, s.OpenedSpaceIds(), spaceId)

		s.lock.Lock()
		_, stillRegistered := s.spaceIndexes[spaceId]
		s.lock.Unlock()
		assert.False(t, stillRegistered)

		ids, err = s.anystoreProvider.ListSpaceIdsFromFilesystem()
		require.NoError(t, err)
		assert.NotContains(t, ids, spaceId, "space dir must be removed from disk")
	})

	t.Run("idempotent: deleting an absent space succeeds", func(t *testing.T) {
		s := NewStoreFixture(t)
		require.NoError(t, s.DeleteSpaceIndex("neverOpened"))
		// second call is also a no-op
		require.NoError(t, s.DeleteSpaceIndex("neverOpened"))
	})

	t.Run("tombstone is lifted after delete so the space can be re-opened fresh", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		const spaceId = "rejoinSpace"
		s.AddObjects(t, spaceId, []TestObject{
			{bundle.RelationKeyId: domain.String("o1"), bundle.RelationKeyName: domain.String("n1")},
		})

		// when
		require.NoError(t, s.DeleteSpaceIndex(spaceId))

		// then: re-opening yields a fresh, empty store (no resurrection of old data)
		store := s.SpaceIndex(spaceId)
		require.NoError(t, store.Init())
		idsAfter, err := store.ListIds()
		require.NoError(t, err)
		assert.Empty(t, idsAfter, "re-opened space must be empty, not the deleted data")
	})
}

// TestDeleteSpaceIndex_ResurrectionGuard verifies the TOCTOU guard: a
// SpaceIndex call that lands while the tombstone is set must NOT recreate the
// store/DB; it returns an invalid store.
func TestDeleteSpaceIndex_ResurrectionGuard(t *testing.T) {
	s := NewStoreFixture(t)
	const spaceId = "guarded"
	s.AddObjects(t, spaceId, []TestObject{
		{bundle.RelationKeyId: domain.String("o1"), bundle.RelationKeyName: domain.String("n1")},
	})

	// Manually set the tombstone to simulate the in-flight delete window.
	s.lock.Lock()
	s.deletedSpaceIds[spaceId] = struct{}{}
	delete(s.spaceIndexes, spaceId)
	s.lock.Unlock()

	// SpaceIndex must refuse to reopen the tombstoned space.
	store := s.SpaceIndex(spaceId)
	require.Error(t, store.Init(), "tombstoned space must yield an invalid store")

	s.lock.Lock()
	_, recreated := s.spaceIndexes[spaceId]
	s.lock.Unlock()
	assert.False(t, recreated, "tombstoned space must not be re-registered")
}

// TestMarkSpaceIndexOpened_TombstoneGuard verifies that a stale registry
// snapshot re-marking a deleted space does NOT re-add it to openedSpaceIds.
func TestMarkSpaceIndexOpened_TombstoneGuard(t *testing.T) {
	s := NewStoreFixture(t)
	const spaceId = "staleSnapshot"
	_ = s.SpaceIndex(spaceId)
	require.Contains(t, s.OpenedSpaceIds(), spaceId)

	// Delete it: openedSpaceIds loses it and the tombstone is set then lifted.
	require.NoError(t, s.DeleteSpaceIndex(spaceId))
	require.NotContains(t, s.OpenedSpaceIds(), spaceId)

	// Simulate the markSpaceIndexOpened call from a stale cross-space snapshot
	// that still references the deleted space, while the tombstone is set.
	s.lock.Lock()
	s.deletedSpaceIds[spaceId] = struct{}{}
	s.lock.Unlock()
	s.markSpaceIndexOpened(spaceId)

	assert.NotContains(t, s.OpenedSpaceIds(), spaceId,
		"a tombstoned space must not be re-added to openedSpaceIds")
}

// TestDeleteSpaceIndex_ConcurrentCrossSpace is a race-detector test: while one
// goroutine repeatedly runs cross-space / full-text iterations over the
// registry (which snapshot stores and operate on them outside the lock),
// another deletes spaces. The drain barrier + tombstone must prevent a
// use-after-close / removed-file access and any data race.
func TestDeleteSpaceIndex_ConcurrentCrossSpace(t *testing.T) {
	s := NewStoreFixture(t)

	spaceIds := []string{"s1", "s2", "s3", "s4", "s5"}
	for _, id := range spaceIds {
		s.AddObjects(t, id, []TestObject{
			{bundle.RelationKeyId: domain.String(id + "-o1"), bundle.RelationKeyName: domain.String("n")},
		})
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Cross-space readers: exercise the snapshot-then-operate-outside-lock path.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, _ = s.QueryCrossSpace(context.Background(), database.Query{})
				_ = s.EnqueueAllForFulltextIndexing(context.Background())
				_ = s.IterateSpaceIndex(context.Background(), func(store spaceindex.Store) error { return nil })
			}
		}()
	}

	// Deleter: delete each space once.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, id := range spaceIds {
			_ = s.DeleteSpaceIndex(id)
		}
	}()

	// Let the readers keep going briefly after deletions, then stop.
	wgDeleteDone := make(chan struct{})
	go func() {
		// Wait for all deletions to complete by polling the registry.
		for {
			s.lock.Lock()
			remaining := 0
			for _, id := range spaceIds {
				if _, ok := s.spaceIndexes[id]; ok {
					remaining++
				}
			}
			s.lock.Unlock()
			if remaining == 0 {
				close(wgDeleteDone)
				return
			}
		}
	}()
	<-wgDeleteDone
	close(stop)
	wg.Wait()

	// All target spaces must be gone from the registry and the opened set.
	opened := s.OpenedSpaceIds()
	for _, id := range spaceIds {
		assert.False(t, slices.Contains(opened, id), "space %s must be forgotten", id)
	}
}
