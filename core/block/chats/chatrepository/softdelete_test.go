package chatrepository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (f *fixture) addAuthoredMessage(t *testing.T, id, orderId, creator string, deleted bool) {
	t.Helper()
	msg := &chatmodel.Message{
		ChatMessage: &model.ChatMessage{
			Id:      id,
			OrderId: orderId,
			Creator: creator,
			Message: &model.ChatMessageMessageContent{Text: "test"},
		},
	}
	if deleted {
		msg.MakeTombstone()
	}
	require.NoError(t, f.repo.AddTestMessage(context.Background(), msg))
}

func messageIds(msgs []*chatmodel.Message) []string {
	ids := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		ids = append(ids, msg.Id)
	}
	return ids
}

func TestTombstonesInRepository(t *testing.T) {
	ctx := context.Background()
	newFx := func(t *testing.T) *fixture {
		fx := newFixture(t)
		fx.addAuthoredMessage(t, "msg1", "o1", "alice", false)
		fx.addAuthoredMessage(t, "msg2", "o2", "bob", false)
		fx.addAuthoredMessage(t, "msg3", "o3", "alice", true)
		fx.addAuthoredMessage(t, "msg4", "o4", "bob", true)
		return fx
	}

	t.Run("CountMessages excludes tombstones", func(t *testing.T) {
		// given
		fx := newFx(t)

		// when
		count, err := fx.repo.CountMessages(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("marking unread skips tombstones", func(t *testing.T) {
		// given
		fx := newFx(t)
		_, err := fx.repo.SetReadFlag(ctx, "chat", []string{"msg1"}, chatmodel.CounterTypeMessage, true)
		require.NoError(t, err)

		// when
		modified, err := fx.repo.SetReadFlag(ctx, "chat", []string{"msg1", "msg3"}, chatmodel.CounterTypeMessage, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"msg1"}, modified)
		assert.True(t, fx.getMessage(t, "msg3").Read)
		state, err := fx.repo.LoadChatState(ctx)
		require.NoError(t, err)
		assert.Equal(t, int32(2), state.Messages.Counter, "msg1 and msg2 are unread, the tombstones never are")
	})

	t.Run("IterateMessagesForIndexing skips tombstones", func(t *testing.T) {
		for _, afterOrderId := range []string{"", "o2"} {
			// given
			fx := newFx(t)
			var got []string

			// when
			err := fx.repo.IterateMessagesForIndexing(ctx, afterOrderId, func(msg *chatmodel.Message) error {
				got = append(got, msg.Id)
				return nil
			})

			// then
			require.NoError(t, err)
			assert.NotContains(t, got, "msg3", "afterOrderId %q", afterOrderId)
			assert.NotContains(t, got, "msg4", "afterOrderId %q", afterOrderId)
			assert.Contains(t, got, "msg2", "afterOrderId %q", afterOrderId)
		}
	})

	t.Run("GetLastMessages keeps tombstones", func(t *testing.T) {
		// given
		fx := newFx(t)

		// when
		got, err := fx.repo.GetLastMessages(ctx, 2)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"msg3", "msg4"}, messageIds(got))
	})

	t.Run("GetLastLiveMessages returns the newest live messages", func(t *testing.T) {
		// given
		fx := newFx(t)

		// when
		got, err := fx.repo.GetLastLiveMessages(ctx, 2)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"msg1", "msg2"}, messageIds(got))
	})

	t.Run("GetLastMessagesByCreators returns the newest live messages", func(t *testing.T) {
		// given
		fx := newFx(t)

		// when
		got, err := fx.repo.GetLastMessagesByCreators(ctx, []string{"alice", "bob"}, 2)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"msg1", "msg2"}, messageIds(got))
	})
}

// The live-only reads add a residual "not deleted" filter; it must not cost them their index or
// their natural-order walk, and the tombstone count must come from the sparse index rather than a
// collection scan.
func TestTombstoneQueriesUseIndexes(t *testing.T) {
	fx := newPopulatedFixture(t)

	t.Run("tombstone count", func(t *testing.T) {
		assert.Equal(t, chatmodel.DeletedKey, usedIndex(explain(t, fx.repo.deletedMessagesQuery())))
	})

	t.Run("live messages by creator", func(t *testing.T) {
		ex := explain(t, fx.repo.lastLiveMessagesByCreatorQuery("creator1", 10))
		assert.Equal(t, chatmodel.CreatorKey+","+chatmodel.StorageKeyField, usedIndex(ex), ex.Sql)
	})

	t.Run("newest live messages", func(t *testing.T) {
		ex := explain(t, fx.repo.lastLiveMessagesQuery(10))
		assert.Equal(t, "FullScan(reverse)(filtered) -> Limit(10)", ex.Sql, "the collection walked from the newest end")
	})
}
