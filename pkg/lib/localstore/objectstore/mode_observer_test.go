package objectstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func TestOnSpaceModeChange(t *testing.T) {
	const testSpaceId = "test-space-for-mode-change"

	t.Run("mode change to Loading initializes space index", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		// SpaceIndex returns a proxy that's not initialized yet
		store := s.SpaceIndex(testSpaceId)

		// verify it's not initialized (writes should fail)
		err := store.UpdateObjectDetails(context.Background(), "obj1", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("obj1"),
		}))
		require.ErrorIs(t, err, spaceindex.ErrSpaceNotInitialized)

		// when - simulate mode change to Loading
		s.OnSpaceModeChange(testSpaceId, spaceinfo.SpaceModeInitial, spaceinfo.SpaceModeLoading)

		// then - space index should be initialized and usable
		err = store.UpdateObjectDetails(context.Background(), "obj1", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("obj1"),
			bundle.RelationKeyName: domain.String("Test Object"),
		}))
		require.NoError(t, err)

		details, err := store.GetDetails("obj1")
		require.NoError(t, err)
		assert.Equal(t, "Test Object", details.GetString(bundle.RelationKeyName))
	})

	t.Run("mode change to Offloading closes space index", func(t *testing.T) {
		// given - initialized space with data
		s := NewStoreFixture(t)
		store := s.SpaceIndex(testSpaceId)

		// Initialize via mode change
		s.OnSpaceModeChange(testSpaceId, spaceinfo.SpaceModeInitial, spaceinfo.SpaceModeLoading)

		// Write some data
		err := store.UpdateObjectDetails(context.Background(), "obj1", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("obj1"),
			bundle.RelationKeyName: domain.String("Test Object"),
		}))
		require.NoError(t, err)

		// Verify data exists
		details, err := store.GetDetails("obj1")
		require.NoError(t, err)
		assert.Equal(t, "Test Object", details.GetString(bundle.RelationKeyName))

		// when - simulate mode change to Offloading
		s.OnSpaceModeChange(testSpaceId, spaceinfo.SpaceModeLoading, spaceinfo.SpaceModeOffloading)

		// then - space index should be closed, reads return empty, writes fail
		details, err = store.GetDetails("obj1")
		require.NoError(t, err)
		assert.True(t, details.Len() == 0, "expected empty details after close")

		err = store.UpdateObjectDetails(context.Background(), "obj2", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("obj2"),
		}))
		require.ErrorIs(t, err, spaceindex.ErrSpaceNotInitialized)
	})

	t.Run("full lifecycle: Loading -> Offloading -> Loading", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		store := s.SpaceIndex(testSpaceId)

		// Step 1: Initialize via Loading
		s.OnSpaceModeChange(testSpaceId, spaceinfo.SpaceModeInitial, spaceinfo.SpaceModeLoading)

		err := store.UpdateObjectDetails(context.Background(), "obj1", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("obj1"),
			bundle.RelationKeyName: domain.String("Test Object"),
		}))
		require.NoError(t, err)

		// Step 2: Close via Offloading
		s.OnSpaceModeChange(testSpaceId, spaceinfo.SpaceModeLoading, spaceinfo.SpaceModeOffloading)

		// Verify closed
		details, err := store.GetDetails("obj1")
		require.NoError(t, err)
		assert.True(t, details.Len() == 0)

		// Step 3: Reinitialize via Loading
		s.OnSpaceModeChange(testSpaceId, spaceinfo.SpaceModeOffloading, spaceinfo.SpaceModeLoading)

		// Data should persist (DB wasn't deleted, just closed)
		details, err = store.GetDetails("obj1")
		require.NoError(t, err)
		assert.Equal(t, "Test Object", details.GetString(bundle.RelationKeyName))
	})

	t.Run("other mode changes are no-op", func(t *testing.T) {
		// given - initialized space
		s := NewStoreFixture(t)
		store := s.SpaceIndex(testSpaceId)
		s.OnSpaceModeChange(testSpaceId, spaceinfo.SpaceModeInitial, spaceinfo.SpaceModeLoading)

		err := store.UpdateObjectDetails(context.Background(), "obj1", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("obj1"),
			bundle.RelationKeyName: domain.String("Test Object"),
		}))
		require.NoError(t, err)

		// when - mode changes to Joining (not Loading or Offloading)
		s.OnSpaceModeChange(testSpaceId, spaceinfo.SpaceModeLoading, spaceinfo.SpaceModeJoining)

		// then - store should still be usable (not closed)
		details, err := store.GetDetails("obj1")
		require.NoError(t, err)
		assert.Equal(t, "Test Object", details.GetString(bundle.RelationKeyName))
	})
}
