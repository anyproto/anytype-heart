package subscription

import (
	"context"
	"testing"

	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

func Test_newCollectionObserver(t *testing.T) {
	spaceId := "spaceId"
	t.Run("removed ids are fetched from cache, added ids from store", func(t *testing.T) {
		// given
		collectionService := NewMockCollectionService(t)
		collectionID := "collectionId"
		subId := "subId"
		ch := make(chan []string)
		collectionService.EXPECT().SubscribeForCollection(collectionID, subId).Return([]string{"id0"}, ch, nil)
		store := spaceindex.NewStoreFixture(t)
		// id0 is part of the collection, so its (projected) entry sits in the cache;
		// it may be already deleted from the store, but its removal must still be observed
		cache := newCache()
		cache.Set(&entry{id: "id0", data: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String("id0"),
		})})
		// added objects are read from the store: cached entries are projected to other
		// subscriptions' keys and could lack keys this subscription filters by
		store.AddObjects(t, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:      domain.String("id2"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
		})
		batcher := mb.New[database.Record](0)
		c := &spaceSubscriptions{
			collectionService: collectionService,
			objectStore:       store,
			recBatch:          batcher,
			cache:             cache,
		}

		// when
		observer, err := c.newCollectionObserver(spaceId, collectionID, subId)

		// then
		assert.NoError(t, err)
		ch <- []string{"id1", "id2"}
		close(observer.closeCh)
		msgs, err := batcher.NewCond().WithMin(3).Wait(context.Background())
		assert.NoError(t, err)

		var receivedIds []string
		for _, msg := range msgs {
			id := msg.Details.GetString(bundle.RelationKeyId)
			receivedIds = append(receivedIds, id)
		}
		assert.Equal(t, []string{"id0", "id1", "id2"}, receivedIds)
		err = batcher.Close()
		assert.NoError(t, err)
	})
	t.Run("fetch entries from object store", func(t *testing.T) {
		// given
		collectionService := NewMockCollectionService(t)
		collectionID := "collectionId"
		subId := "subId"
		ch := make(chan []string)
		collectionService.EXPECT().SubscribeForCollection(collectionID, subId).Return([]string{"id"}, ch, nil)
		store := spaceindex.NewStoreFixture(t)

		store.AddObjects(t, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:      domain.String("id2"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
		})
		batcher := mb.New[database.Record](0)
		c := &spaceSubscriptions{
			collectionService: collectionService,
			objectStore:       store,
			recBatch:          batcher,
			cache:             newCache(),
		}

		// when
		observer, err := c.newCollectionObserver(spaceId, collectionID, subId)

		// then
		assert.NoError(t, err)
		expectedIds := []string{"id1", "id2"}
		ch <- expectedIds
		close(observer.closeCh)
		msgs, err := batcher.NewCond().WithMin(2).Wait(context.Background())
		assert.NoError(t, err)

		var receivedIds []string
		for _, msg := range msgs {
			id := msg.Details.GetString(bundle.RelationKeyId)
			receivedIds = append(receivedIds, id)
		}
		assert.Equal(t, expectedIds, receivedIds)
		err = batcher.Close()
		assert.NoError(t, err)
	})
}
