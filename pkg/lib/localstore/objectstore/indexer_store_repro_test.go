package objectstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

// Repro for: FtQueueMarkAsIndexed overwrites the whole queue document with
// {id, spaceId, seq}, erasing fields written concurrently while the batch was
// being processed. A chat message deletion (or new message orderId) queued
// between ListIdsFromFullTextQueue and FtQueueMarkAsIndexed is silently lost.
func TestFtQueueMarkAsIndexedPreservesConcurrentUpdates(t *testing.T) {
	ctx := context.Background()
	chatId := domain.FullID{ObjectID: "chat1", SpaceID: "space1"}

	t.Run("message deletion queued during processing is preserved", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		require.NoError(t, s.AddChatMessageToIndexQueue(ctx, chatId, "orderA"))

		// the indexer picks up the entry for processing
		objs, err := s.ListIdsFromFullTextQueue([]string{"space1"}, 0)
		require.NoError(t, err)
		require.Len(t, objs, 1)
		require.Equal(t, "orderA", objs[0].MsgOrderId)

		// while the batch is processed, a message deletion is queued
		require.NoError(t, s.AddChatMessageDeleteToIndexQueue(ctx, chatId, "msg1"))

		// when: the indexer finishes the batch it started earlier
		require.NoError(t, s.FtQueueMarkAsIndexed(objs, 42))

		// then: the deletion queued mid-batch must still be pending
		objs, err = s.ListIdsFromFullTextQueue([]string{"space1"}, 0)
		require.NoError(t, err)
		require.Len(t, objs, 1, "entry with a pending message deletion must stay in the queue")
		assert.Equal(t, []string{"msg1"}, objs[0].DeletedMsgIds)
	})

	t.Run("message enqueued after a successful mark re-pends the entry", func(t *testing.T) {
		// given: a chat batch that was listed, processed and marked
		s := NewStoreFixture(t)
		require.NoError(t, s.AddChatMessageToIndexQueue(ctx, chatId, "orderA"))
		objs, err := s.ListIdsFromFullTextQueue([]string{"space1"}, 0)
		require.NoError(t, err)
		require.Len(t, objs, 1)
		require.NoError(t, s.FtQueueMarkAsIndexed(objs, 42))

		// when: a newer message (larger orderId) arrives
		require.NoError(t, s.AddChatMessageToIndexQueue(ctx, chatId, "orderB"))

		// then: the entry must be pending again with the new order id
		objs, err = s.ListIdsFromFullTextQueue([]string{"space1"}, 0)
		require.NoError(t, err)
		require.Len(t, objs, 1, "a message arriving after the mark must re-pend the entry")
		assert.Equal(t, "orderB", objs[0].MsgOrderId)
		assert.Empty(t, objs[0].DeletedMsgIds, "processed deletion list must not survive the mark")
	})

	t.Run("object re-enqueued during processing stays pending", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		objId := domain.FullID{ObjectID: "obj1", SpaceID: "space1"}
		_, _, err := s.AddToIndexQueue(ctx, objId)
		require.NoError(t, err)

		// the indexer picks up the entry for processing
		objs, err := s.ListIdsFromFullTextQueue([]string{"space1"}, 0)
		require.NoError(t, err)
		require.Len(t, objs, 1)

		// while the batch is processed, the object is modified and re-enqueued
		_, _, err = s.AddToIndexQueue(ctx, objId)
		require.NoError(t, err)

		// when: the indexer finishes the batch it started earlier
		require.NoError(t, s.FtQueueMarkAsIndexed(objs, 42))

		// then: the re-enqueued object must still be pending
		objs, err = s.ListIdsFromFullTextQueue([]string{"space1"}, 0)
		require.NoError(t, err)
		assert.Len(t, objs, 1, "object re-enqueued mid-batch must stay in the queue")
	})
}
