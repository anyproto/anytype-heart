package spaceindex

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestListIdsWithoutSyncDetails(t *testing.T) {
	t.Run("returns only ids missing at least one sync relation", func(t *testing.T) {
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{
			// Complete: has all three sync relations, with the zero-valued
			// "Synced"/"Null" enums explicitly present. Presence (not value)
			// must keep it out of the result, otherwise every synced object
			// would be reprocessed forever.
			{
				bundle.RelationKeyId:         domain.String("complete"),
				bundle.RelationKeySyncStatus: domain.Int64(0),
				bundle.RelationKeySyncDate:   domain.Int64(1700000000),
				bundle.RelationKeySyncError:  domain.Int64(0),
			},
			// Missing SyncStatus only.
			{
				bundle.RelationKeyId:        domain.String("missingStatus"),
				bundle.RelationKeySyncDate:  domain.Int64(1700000000),
				bundle.RelationKeySyncError: domain.Int64(0),
			},
			// Missing everything.
			{
				bundle.RelationKeyId:   domain.String("missingAll"),
				bundle.RelationKeyName: domain.String("foo"),
			},
		})

		ids, err := s.ListIdsWithoutSyncDetails(context.Background())
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"missingStatus", "missingAll"}, ids)
	})

	t.Run("returns empty when all objects already have sync relations", func(t *testing.T) {
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{
			{
				bundle.RelationKeyId:         domain.String("a"),
				bundle.RelationKeySyncStatus: domain.Int64(0),
				bundle.RelationKeySyncDate:   domain.Int64(1700000000),
				bundle.RelationKeySyncError:  domain.Int64(0),
			},
		})

		ids, err := s.ListIdsWithoutSyncDetails(context.Background())
		require.NoError(t, err)
		assert.Empty(t, ids)
	})
}
