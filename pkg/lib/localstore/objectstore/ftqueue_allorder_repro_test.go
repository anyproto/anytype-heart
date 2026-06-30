package objectstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

// TestFtAllOrderId_QueueMergePrecedence guards the chat-fulltext backfill fix
// from GO-7316.
//
// reindexChatMessagesFulltext enqueues every chat with FtAllOrderId ("_all") to
// reindex the FULL message history. Real order ids start with '!' (0x21) and
// "_all" starts with '_' (0x5F), so every real order id sorts BEFORE "_all".
// The original merge kept the existing order id whenever `currentOrderId <
// orderId`, so an incoming "_all" lost to any pending real order id (a live
// message the backed-up queue hadn't processed yet) — and only messages from
// that order id forward got indexed, never the full history.
//
// Observed in real data: a chat with 299 messages had only its most-recent 32
// (everything >= one real order id "!%wK") indexed; the older 267 were invisible
// to chat search.
func TestFtAllOrderId_QueueMergePrecedence(t *testing.T) {
	ctx := context.Background()

	queuedOrderId := func(t *testing.T, s *StoreFixture, chatId domain.FullID) string {
		t.Helper()
		ids, err := s.ListIdsFromFullTextQueue([]string{chatId.SpaceID}, 10)
		require.NoError(t, err)
		for i := range ids {
			if ids[i].ObjectId == chatId.ObjectID {
				return ids[i].MsgOrderId
			}
		}
		t.Fatalf("chat %s not queued", chatId.ObjectID)
		return ""
	}

	t.Run("incoming _all overrides a pending real order id", func(t *testing.T) {
		s := NewStoreFixture(t)
		chatId := domain.FullID{ObjectID: "chatAllWins", SpaceID: "space1"}

		// a live message enqueued a real order id (still pending in a backed-up queue)
		require.NoError(t, s.AddChatMessageToIndexQueue(ctx, chatId, "!%wK"))
		// the reindex backfill asks to index the WHOLE history
		require.NoError(t, s.AddChatMessageToIndexQueue(ctx, chatId, FtAllOrderId))

		assert.Equal(t, FtAllOrderId, queuedOrderId(t, s, chatId),
			"_all backfill must win over a pending real order id, else chat history is never fully indexed")
	})

	t.Run("a real order id does not downgrade a pending _all", func(t *testing.T) {
		s := NewStoreFixture(t)
		chatId := domain.FullID{ObjectID: "chatAllKept", SpaceID: "space1"}

		require.NoError(t, s.AddChatMessageToIndexQueue(ctx, chatId, FtAllOrderId))
		require.NoError(t, s.AddChatMessageToIndexQueue(ctx, chatId, "!%wK"))

		assert.Equal(t, FtAllOrderId, queuedOrderId(t, s, chatId),
			"a real order id must not shrink a pending full-history reindex")
	})

	t.Run("smaller real order id still wins over a larger one", func(t *testing.T) {
		s := NewStoreFixture(t)
		chatId := domain.FullID{ObjectID: "chatLowerWins", SpaceID: "space1"}

		// real order ids start with '!' (0x21); keep the lower one (covers more)
		require.NoError(t, s.AddChatMessageToIndexQueue(ctx, chatId, "!bbb"))
		require.NoError(t, s.AddChatMessageToIndexQueue(ctx, chatId, "!aaa"))

		assert.Equal(t, "!aaa", queuedOrderId(t, s, chatId),
			"the lower real order id covers more messages and must be kept")
	})

	t.Run("incoming _all preserves a pending deletion list", func(t *testing.T) {
		// the fix routes an incoming _all through the overwrite path; the merge
		// must still carry over a pending deletion list so queued message deletes
		// are not silently dropped by a full-history backfill.
		s := NewStoreFixture(t)
		chatId := domain.FullID{ObjectID: "chatDelKept", SpaceID: "space1"}

		require.NoError(t, s.AddChatMessageDeleteToIndexQueue(ctx, chatId, "deletedMsg1"))
		require.NoError(t, s.AddChatMessageToIndexQueue(ctx, chatId, FtAllOrderId))

		ids, err := s.ListIdsFromFullTextQueue([]string{chatId.SpaceID}, 10)
		require.NoError(t, err)
		var got *domain.FullTextQueuedObject
		for i := range ids {
			if ids[i].ObjectId == chatId.ObjectID {
				got = &ids[i]
				break
			}
		}
		require.NotNil(t, got, "chat should be queued")
		assert.Equal(t, FtAllOrderId, got.MsgOrderId, "_all must win")
		assert.Contains(t, got.DeletedMsgIds, "deletedMsg1",
			"pending message deletion must survive the _all overwrite")
	})
}
