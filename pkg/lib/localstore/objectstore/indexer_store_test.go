package objectstore

import (
	"context"
	"fmt"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-store/syncpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func listIdsFromFullTextQueueAll(ftqueue anystore.Collection, spaceIds []string, limit uint) ([]domain.FullID, error) {
	if len(spaceIds) == 0 {
		return nil, fmt.Errorf("at least one space must be provided")
	}

	filters := query.And{}
	filters = append(filters, ftQueueFilterSpaceIds(spaceIds))
	// filters = append(filters, ftQueueFilterSeq(0, query.CompOpLte))
	iter, err := ftqueue.Find(filters).Limit(limit).Iter(context.Background())
	if err != nil {
		return nil, fmt.Errorf("create iterator: %w", err)
	}
	defer iter.Close()

	var ids []domain.FullID
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("read doc: %w", err)
		}
		id := doc.Value().GetString(idKey)
		spaceId := doc.Value().GetString(spaceIdKey)
		ids = append(ids, domain.FullID{ObjectID: id, SpaceID: spaceId})
	}
	return ids, nil
}

func TestDsObjectStore_IndexQueue(t *testing.T) {
	s := NewStoreFixture(t)

	ctx := context.Background()

	t.Run("add to queue", func(t *testing.T) {
		_, _, err := s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "one", SpaceID: "id1"})
		require.NoError(t, err)
		_, _, err = s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "one", SpaceID: "id1"})
		require.NoError(t, err)
		_, _, err = s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "two", SpaceID: "id1"})
		require.NoError(t, err)

		ids, err := s.ListIdsFromFullTextQueue([]string{"id1"}, 0)
		require.NoError(t, err)

		assert.ElementsMatch(t, []domain.FullTextQueuedObject{{ObjectId: "one", SpaceId: "id1"}, {ObjectId: "two", SpaceId: "id1"}}, ids)
	})

	t.Run("reconcile", func(t *testing.T) {
		_, _, err := s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "x", SpaceID: "id2"})
		require.NoError(t, err)
		_, _, err = s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "y", SpaceID: "id2"})
		require.NoError(t, err)
		_, _, err = s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "z", SpaceID: "id2"})
		require.NoError(t, err)
		ids, err := s.ListIdsFromFullTextQueue([]string{"id2"}, 0)
		require.NoError(t, err)
		require.Len(t, ids, 3)

		require.NoError(t, s.FtQueueMarkAsIndexed([]domain.FullID{{ObjectID: "x", SpaceID: "id2"}}, 1))
		require.NoError(t, s.FtQueueMarkAsIndexed([]domain.FullID{{ObjectID: "y", SpaceID: "id2"}}, 2))
		require.NoError(t, s.FtQueueMarkAsIndexed([]domain.FullID{{ObjectID: "z", SpaceID: "id2"}}, 3))

		ids, err = s.ListIdsFromFullTextQueue([]string{"id2"}, 0)
		require.NoError(t, err)
		require.Len(t, ids, 0)

		err = s.FtQueueReconcileWithSeq(context.Background(), 1)
		require.NoError(t, err)

		ids, err = s.ListIdsFromFullTextQueue([]string{"id2"}, 0)
		require.NoError(t, err)
		require.Len(t, ids, 2)
	})
}

func Test_ftSeq(t *testing.T) {
	arena := &anyenc.Arena{}

	seq0 := ftSeq(uint64(0), arena)
	seq1 := ftSeq(uint64(1), arena)
	seq2 := ftSeq(uint64(2), arena)

	val := arena.NewObject()

	docBuf := &syncpool.DocBuffer{}
	filterGt1 := ftQueueFilterSeq(1, query.CompOpGt, arena)
	val.Set(ftSequenceKey, seq0)
	assert.False(t, filterGt1.Ok(val, docBuf))

	val.Set(ftSequenceKey, seq1)
	assert.False(t, filterGt1.Ok(val, docBuf))

	val.Set(ftSequenceKey, seq2)
	assert.True(t, filterGt1.Ok(val, docBuf))

	filterGt0 := ftQueueFilterSeq(0, query.CompOpGt, arena)
	assert.True(t, filterGt0.Ok(val, docBuf))

	emptyBufferVal := arena.NewBinary(emptyBuffer)
	val.Set(ftSequenceKey, emptyBufferVal)
	filterLte0 := ftQueueFilterSeq(0, query.CompOpLte, arena)
	assert.True(t, filterLte0.Ok(val, docBuf))
}

func TestIndexerBatch(t *testing.T) {
	s := NewStoreFixture(t)
	ctx := context.Background()

	t.Run("batch - no more than limit", func(t *testing.T) {
		_, _, err := s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "one", SpaceID: "id1"})
		require.NoError(t, err)
		_, _, err = s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "two", SpaceID: "id1"})
		require.NoError(t, err)
		_, _, err = s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "three", SpaceID: "id1"})
		require.NoError(t, err)
		var batches [][]domain.FullTextQueuedObject
		err = s.BatchProcessFullTextQueue(
			func() []string { return []string{"id1"} },
			2,
			func(ids []domain.FullTextQueuedObject) ([]domain.FullID, uint64, error) {
				batches = append(batches, ids)
				fullIds := make([]domain.FullID, len(ids))
				for i, id := range ids {
					fullIds[i] = id.FullId()
				}
				return fullIds, 1, nil
			})
		require.NoError(t, err)
		require.Len(t, batches, 2)

		// Collect all processed IDs
		var allProcessed []domain.FullTextQueuedObject
		for _, batch := range batches {
			allProcessed = append(allProcessed, batch...)
		}

		// Verify all items were processed
		assert.ElementsMatch(t, []domain.FullTextQueuedObject{
			{ObjectId: "one", SpaceId: "id1"},
			{ObjectId: "two", SpaceId: "id1"},
			{ObjectId: "three", SpaceId: "id1"},
		}, allProcessed)

		// Verify batch sizes
		assert.LessOrEqual(t, len(batches[0]), 2)
		assert.LessOrEqual(t, len(batches[1]), 2)
	})
}

func TestIndexerChecksums(t *testing.T) {
	t.Run("save and load checksums", func(t *testing.T) {
		s := NewStoreFixture(t)

		want := &model.ObjectStoreChecksums{
			BundledObjectTypes:               "hash1",
			BundledRelations:                 "hash2",
			BundledLayouts:                   "hash3",
			ObjectsForceReindexCounter:       1,
			FilesForceReindexCounter:         2,
			IdxRebuildCounter:                3,
			BundledTemplates:                 "hash4",
			BundledObjects:                   5,
			FilestoreKeysForceReindexCounter: 6,
		}

		require.NoError(t, s.SaveChecksums("spaceX", want))

		got, err := s.GetChecksums("spaceX")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
}

func TestAddChatMessageToIndexQueue(t *testing.T) {
	s := NewStoreFixture(t)
	ctx := context.Background()

	t.Run("add message with orderId", func(t *testing.T) {
		chatId := domain.FullID{ObjectID: "chat1", SpaceID: "space1"}
		err := s.AddChatMessageToIndexQueue(ctx, chatId, "order1")
		require.NoError(t, err)

		// Verify message was added to queue
		ids, err := s.ListIdsFromFullTextQueue([]string{"space1"}, 10)
		require.NoError(t, err)
		require.Len(t, ids, 1)
		assert.Equal(t, "chat1", ids[0].ObjectId)
		assert.Equal(t, "space1", ids[0].SpaceId)
		assert.Equal(t, "order1", ids[0].MsgOrderId)
	})

	t.Run("update with older orderId", func(t *testing.T) {
		chatId := domain.FullID{ObjectID: "chat2", SpaceID: "space1"}

		// Add with orderId "order1"
		err := s.AddChatMessageToIndexQueue(ctx, chatId, "order3")
		require.NoError(t, err)

		// Update with newer orderId "order3"
		err = s.AddChatMessageToIndexQueue(ctx, chatId, "order2")
		require.NoError(t, err)

		ids, err := s.ListIdsFromFullTextQueue([]string{"space1"}, 10)
		require.NoError(t, err)

		// Find chat2 in results
		var found *domain.FullTextQueuedObject
		for _, id := range ids {
			if id.ObjectId == "chat2" {
				found = &id
				break
			}
		}
		require.NotNil(t, found)
		assert.Equal(t, "order2", found.MsgOrderId)
	})

	t.Run("ignore newer orderId", func(t *testing.T) {
		chatId := domain.FullID{ObjectID: "chat3", SpaceID: "space1"}

		// Add with orderId "order5"
		err := s.AddChatMessageToIndexQueue(ctx, chatId, "order2")
		require.NoError(t, err)

		// Try to update with older orderId "order2" - should be ignored
		err = s.AddChatMessageToIndexQueue(ctx, chatId, "order5")
		require.NoError(t, err)

		ids, err := s.ListIdsFromFullTextQueue([]string{"space1"}, 10)
		require.NoError(t, err)

		// Find chat3 in results
		var found *domain.FullTextQueuedObject
		for _, id := range ids {
			if id.ObjectId == "chat3" {
				found = &id
				break
			}
		}
		require.NotNil(t, found)
		// Should still have the older orderId
		assert.Equal(t, "order2", found.MsgOrderId)
	})

	t.Run("preserve deleted message IDs", func(t *testing.T) {
		chatId := domain.FullID{ObjectID: "chat4", SpaceID: "space1"}

		// First add a deleted message
		err := s.AddChatMessageDeleteToIndexQueue(ctx, chatId, "deletedMsg1")
		require.NoError(t, err)

		// Then add a message update
		err = s.AddChatMessageToIndexQueue(ctx, chatId, "order1")
		require.NoError(t, err)

		ids, err := s.ListIdsFromFullTextQueue([]string{"space1"}, 10)
		require.NoError(t, err)

		// Find chat4 in results
		var found *domain.FullTextQueuedObject
		for _, id := range ids {
			if id.ObjectId == "chat4" {
				found = &id
				break
			}
		}
		require.NotNil(t, found)
		// Should preserve deleted message IDs
		assert.Contains(t, found.DeletedMsgIds, "deletedMsg1")
		assert.Equal(t, "order1", found.MsgOrderId)
	})

	t.Run("FtAllOrderId takes precedence", func(t *testing.T) {
		chatId := domain.FullID{ObjectID: "chat5", SpaceID: "space1"}

		// Add with FtAllOrderId
		err := s.AddChatMessageToIndexQueue(ctx, chatId, FtAllOrderId)
		require.NoError(t, err)

		// Try to update with normal orderId - should be ignored
		err = s.AddChatMessageToIndexQueue(ctx, chatId, "order1")
		require.NoError(t, err)

		ids, err := s.ListIdsFromFullTextQueue([]string{"space1"}, 10)
		require.NoError(t, err)

		// Find chat5 in results
		var found *domain.FullTextQueuedObject
		for _, id := range ids {
			if id.ObjectId == "chat5" {
				found = &id
				break
			}
		}
		require.NotNil(t, found)
		// Should keep FtAllOrderId
		assert.Equal(t, FtAllOrderId, found.MsgOrderId)
	})
}

func TestAddChatMessageDeleteToIndexQueue(t *testing.T) {
	s := NewStoreFixture(t)
	ctx := context.Background()

	t.Run("add single deleted message", func(t *testing.T) {
		chatId := domain.FullID{ObjectID: "chat1", SpaceID: "space1"}
		err := s.AddChatMessageDeleteToIndexQueue(ctx, chatId, "msg1")
		require.NoError(t, err)

		// Verify message was added to queue
		ids, err := s.ListIdsFromFullTextQueue([]string{"space1"}, 10)
		require.NoError(t, err)
		require.Len(t, ids, 1)
		assert.Equal(t, "chat1", ids[0].ObjectId)
		assert.Equal(t, "space1", ids[0].SpaceId)
		assert.Contains(t, ids[0].DeletedMsgIds, "msg1")
	})

	t.Run("add multiple deleted messages", func(t *testing.T) {
		chatId := domain.FullID{ObjectID: "chat2", SpaceID: "space1"}

		// Add first deleted message
		err := s.AddChatMessageDeleteToIndexQueue(ctx, chatId, "msg1")
		require.NoError(t, err)

		// Add second deleted message
		err = s.AddChatMessageDeleteToIndexQueue(ctx, chatId, "msg2")
		require.NoError(t, err)

		// Add third deleted message
		err = s.AddChatMessageDeleteToIndexQueue(ctx, chatId, "msg3")
		require.NoError(t, err)

		ids, err := s.ListIdsFromFullTextQueue([]string{"space1"}, 10)
		require.NoError(t, err)

		// Find chat2 in results
		var found *domain.FullTextQueuedObject
		for _, id := range ids {
			if id.ObjectId == "chat2" {
				found = &id
				break
			}
		}
		require.NotNil(t, found)
		// Should have all deleted messages
		assert.Len(t, found.DeletedMsgIds, 3)
		assert.Contains(t, found.DeletedMsgIds, "msg1")
		assert.Contains(t, found.DeletedMsgIds, "msg2")
		assert.Contains(t, found.DeletedMsgIds, "msg3")
	})

	t.Run("preserve existing orderId", func(t *testing.T) {
		chatId := domain.FullID{ObjectID: "chat3", SpaceID: "space1"}

		// First add with orderId
		err := s.AddChatMessageToIndexQueue(ctx, chatId, "order1")
		require.NoError(t, err)

		// Then add deleted message
		err = s.AddChatMessageDeleteToIndexQueue(ctx, chatId, "deletedMsg1")
		require.NoError(t, err)

		ids, err := s.ListIdsFromFullTextQueue([]string{"space1"}, 10)
		require.NoError(t, err)

		// Find chat3 in results
		var found *domain.FullTextQueuedObject
		for _, id := range ids {
			if id.ObjectId == "chat3" {
				found = &id
				break
			}
		}
		require.NotNil(t, found)
		// Should preserve orderId
		assert.Equal(t, "order1", found.MsgOrderId)
		assert.Contains(t, found.DeletedMsgIds, "deletedMsg1")
	})

	t.Run("works without existing orderId", func(t *testing.T) {
		chatId := domain.FullID{ObjectID: "chat4", SpaceID: "space1"}

		// Add deleted message without prior orderId
		err := s.AddChatMessageDeleteToIndexQueue(ctx, chatId, "msg1")
		require.NoError(t, err)

		ids, err := s.ListIdsFromFullTextQueue([]string{"space1"}, 10)
		require.NoError(t, err)

		// Find chat4 in results
		var found *domain.FullTextQueuedObject
		for _, id := range ids {
			if id.ObjectId == "chat4" {
				found = &id
				break
			}
		}
		require.NotNil(t, found)
		// MsgOrderId should be empty
		assert.Empty(t, found.MsgOrderId)
		assert.Contains(t, found.DeletedMsgIds, "msg1")
	})
}
