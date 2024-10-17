package subscription

import (
	"testing"

	"github.com/cheggaaa/mb"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func Test_newCollectionObserver(t *testing.T) {
	spaceId := "spaceId"
	t.Run("fetch entries from cache", func(t *testing.T) {
		// given
		collectionService := NewMockCollectionService(t)
		collectionID := "collectionId"
		subId := "subId"
		ch := make(chan []string)
		collectionService.EXPECT().SubscribeForCollection(collectionID, subId).Return([]string{"id"}, ch, nil)
		store := spaceindex.NewStoreFixture(t)
		cache := newCache()
		cache.Set(&entry{id: "id1", data: &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyId.String(): pbtypes.String("id1"),
		}}})
		cache.Set(&entry{id: "id2", data: &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyId.String(): pbtypes.String("id2"),
		}}})
		batcher := mb.New(0)
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
		expectedIds := []string{"id1", "id2"}
		ch <- expectedIds
		close(observer.closeCh)
		msgs := batcher.Wait()

		var receivedIds []string
		for _, msg := range msgs {
			id := pbtypes.GetString(msg.(database.Record).Details, "id")
			receivedIds = append(receivedIds, id)
		}
		assert.Equal(t, expectedIds, receivedIds)
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
				bundle.RelationKeyId:      pbtypes.String("id1"),
				bundle.RelationKeySpaceId: pbtypes.String(spaceId),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("id2"),
				bundle.RelationKeySpaceId: pbtypes.String(spaceId),
			},
		})
		batcher := mb.New(0)
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
		msgs := batcher.Wait()

		var receivedIds []string
		for _, msg := range msgs {
			id := pbtypes.GetString(msg.(database.Record).Details, "id")
			receivedIds = append(receivedIds, id)
		}
		assert.Equal(t, expectedIds, receivedIds)
		err = batcher.Close()
		assert.NoError(t, err)
	})
}
