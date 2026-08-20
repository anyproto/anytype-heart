package chatrepository

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type fixture struct {
	repo *repository
	db   anystore.DB
}

func newFixture(t *testing.T) *fixture {
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "store.db"), nil)
	require.NoError(t, err)

	coll, err := db.CreateCollection(ctx, "testchats")
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	return &fixture{
		repo: &repository{
			collection: coll,
			arenaPool:  &anyenc.ArenaPool{},
		},
		db: db,
	}
}

func (f *fixture) addMessage(t *testing.T, id, orderId string, read, hasMention, mentionRead bool) {
	t.Helper()
	msg := &chatmodel.Message{
		ChatMessage: &model.ChatMessage{
			Id:      id,
			OrderId: orderId,
			Message: &model.ChatMessageMessageContent{
				Text: "test",
			},
			Read:        read,
			HasMention:  hasMention,
			MentionRead: mentionRead,
		},
	}
	err := f.repo.AddTestMessage(context.Background(), msg)
	require.NoError(t, err)
}

func (f *fixture) addMessageWithSynced(t *testing.T, id, orderId string, synced bool) {
	t.Helper()
	msg := &chatmodel.Message{
		ChatMessage: &model.ChatMessage{
			Id:      id,
			OrderId: orderId,
			Message: &model.ChatMessageMessageContent{
				Text: "test",
			},
			Synced: synced,
		},
	}
	err := f.repo.AddTestMessage(context.Background(), msg)
	require.NoError(t, err)
}

func (f *fixture) getMessage(t *testing.T, id string) *chatmodel.Message {
	t.Helper()
	msgs, err := f.repo.GetMessagesByIds(context.Background(), []string{id})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	return msgs[0]
}

func (f *fixture) addMessageWithUnreadReactions(t *testing.T, id, orderId string, reactions map[string]map[string]chatmodel.ReactionChangeEntry) {
	t.Helper()
	msg := &chatmodel.Message{
		ChatMessage: &model.ChatMessage{
			Id:      id,
			OrderId: orderId,
			Message: &model.ChatMessageMessageContent{
				Text: "test",
			},
		},
		UnreadReactionIds: reactions,
	}
	err := f.repo.AddTestMessage(context.Background(), msg)
	require.NoError(t, err)
}

// addMessageWithOrders inserts a message with an explicit _o order object so
// tests can model a content edit whose order differs from the creation order.
func (f *fixture) addMessageWithOrders(t *testing.T, id, orderId, contentOrderId string) {
	t.Helper()
	arena := &anyenc.Arena{}
	msg := &chatmodel.Message{
		ChatMessage: &model.ChatMessage{
			Id:      id,
			OrderId: orderId,
			Message: &model.ChatMessageMessageContent{
				Text: "test",
			},
		},
	}
	val := arena.NewObject()
	msg.MarshalAnyenc(val, arena)

	orderObj := arena.NewObject()
	orderObj.Set("id", arena.NewString(orderId))
	if contentOrderId != "" {
		orderObj.Set("content", arena.NewString(contentOrderId))
	}
	val.Set(chatmodel.OrderKey, orderObj)

	err := f.repo.collection.Insert(context.Background(), val)
	require.NoError(t, err)
}

func TestIterateMessagesForIndexing(t *testing.T) {
	collectIds := func(fx *fixture, afterOrderId string) ([]string, error) {
		var ids []string
		err := fx.repo.IterateMessagesForIndexing(context.Background(), afterOrderId, func(msg *chatmodel.Message) error {
			ids = append(ids, msg.Id)
			return nil
		})
		return ids, err
	}

	t.Run("all messages when afterOrderId is empty", func(t *testing.T) {
		// given
		fx := newFixture(t)
		want := make([]string, 0, 5)
		for i := 1; i <= 5; i++ {
			id := fmt.Sprintf("msg%d", i)
			fx.addMessage(t, id, fmt.Sprintf("o%d", i), false, false, false)
			want = append(want, id)
		}

		// when
		got, err := collectIds(fx, "")

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("messages at or after afterOrderId", func(t *testing.T) {
		// given
		fx := newFixture(t)
		for i := 1; i <= 5; i++ {
			fx.addMessage(t, fmt.Sprintf("msg%d", i), fmt.Sprintf("o%d", i), false, false, false)
		}
		want := []string{"msg3", "msg4", "msg5"}

		// when
		got, err := collectIds(fx, "o3")

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("message with older order id but newer content edit is included", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.addMessageWithOrders(t, "msgEdited", "o1", "o9")
		fx.addMessageWithOrders(t, "msgOld", "o2", "")
		fx.addMessageWithOrders(t, "msgNew", "o5", "")
		want := []string{"msgEdited", "msgNew"}

		// when
		got, err := collectIds(fx, "o4")

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("proc error stops iteration and propagates", func(t *testing.T) {
		// given
		fx := newFixture(t)
		for i := 1; i <= 3; i++ {
			fx.addMessage(t, fmt.Sprintf("msg%d", i), fmt.Sprintf("o%d", i), false, false, false)
		}
		wantErr := fmt.Errorf("sink failed")

		// when
		var seen int
		err := fx.repo.IterateMessagesForIndexing(context.Background(), "", func(msg *chatmodel.Message) error {
			seen++
			return wantErr
		})

		// then
		require.ErrorIs(t, err, wantErr)
		assert.Equal(t, 1, seen)
	})
}

func TestCountMessages(t *testing.T) {
	t.Run("zero on empty collection", func(t *testing.T) {
		fx := newFixture(t)
		count, err := fx.repo.CountMessages(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("counts all messages including read, unread and mentioned", func(t *testing.T) {
		fx := newFixture(t)
		fx.addMessage(t, "msg1", "order1", true, false, false)  // read
		fx.addMessage(t, "msg2", "order2", false, false, false) // unread
		fx.addMessage(t, "msg3", "order3", true, true, true)    // read with mention

		count, err := fx.repo.CountMessages(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 3, count)

		state, err := fx.repo.LoadChatState(context.Background())
		require.NoError(t, err)
		// sanity: pre-existing unread counter still reflects only unread
		assert.Equal(t, int32(1), state.Messages.Counter)
	})
}

func TestSetReadFlag(t *testing.T) {
	chatObjectId := "chatObj1"

	t.Run("mark unread messages as read", func(t *testing.T) {
		fx := newFixture(t)
		fx.addMessage(t, "msg1", "order1", false, false, false)
		fx.addMessage(t, "msg2", "order2", false, false, false)
		fx.addMessage(t, "msg3", "order3", false, false, false)

		modified, err := fx.repo.SetReadFlag(context.Background(), chatObjectId, []string{"msg1", "msg2", "msg3"}, chatmodel.CounterTypeMessage, true)
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"msg1", "msg2", "msg3"}, modified)
		assert.True(t, fx.getMessage(t, "msg1").Read)
		assert.True(t, fx.getMessage(t, "msg2").Read)
		assert.True(t, fx.getMessage(t, "msg3").Read)
	})

	t.Run("mark read messages as unread", func(t *testing.T) {
		fx := newFixture(t)
		fx.addMessage(t, "msg1", "order1", true, false, false)
		fx.addMessage(t, "msg2", "order2", true, false, false)

		modified, err := fx.repo.SetReadFlag(context.Background(), chatObjectId, []string{"msg1", "msg2"}, chatmodel.CounterTypeMessage, false)
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"msg1", "msg2"}, modified)
		assert.False(t, fx.getMessage(t, "msg1").Read)
		assert.False(t, fx.getMessage(t, "msg2").Read)
	})

	t.Run("skip already read messages", func(t *testing.T) {
		fx := newFixture(t)
		fx.addMessage(t, "msg1", "order1", true, false, false)
		fx.addMessage(t, "msg2", "order2", false, false, false)

		modified, err := fx.repo.SetReadFlag(context.Background(), chatObjectId, []string{"msg1", "msg2"}, chatmodel.CounterTypeMessage, true)
		require.NoError(t, err)

		assert.Equal(t, []string{"msg2"}, modified)
	})

	t.Run("skip already unread messages", func(t *testing.T) {
		fx := newFixture(t)
		fx.addMessage(t, "msg1", "order1", false, false, false)
		fx.addMessage(t, "msg2", "order2", true, false, false)

		modified, err := fx.repo.SetReadFlag(context.Background(), chatObjectId, []string{"msg1", "msg2"}, chatmodel.CounterTypeMessage, false)
		require.NoError(t, err)

		assert.Equal(t, []string{"msg2"}, modified)
	})

	t.Run("skip missing messages", func(t *testing.T) {
		fx := newFixture(t)
		fx.addMessage(t, "msg1", "order1", false, false, false)

		modified, err := fx.repo.SetReadFlag(context.Background(), chatObjectId, []string{"msg1", "nonexistent"}, chatmodel.CounterTypeMessage, true)
		require.NoError(t, err)

		assert.Equal(t, []string{"msg1"}, modified)
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		fx := newFixture(t)

		modified, err := fx.repo.SetReadFlag(context.Background(), chatObjectId, nil, chatmodel.CounterTypeMessage, true)
		require.NoError(t, err)

		assert.Nil(t, modified)
	})

	t.Run("mention counter type", func(t *testing.T) {
		fx := newFixture(t)
		fx.addMessage(t, "msg1", "order1", false, true, false)
		fx.addMessage(t, "msg2", "order2", false, true, true)
		fx.addMessage(t, "msg3", "order3", false, false, false) // no mention

		modified, err := fx.repo.SetReadFlag(context.Background(), chatObjectId, []string{"msg1", "msg2", "msg3"}, chatmodel.CounterTypeMention, true)
		require.NoError(t, err)

		assert.Equal(t, []string{"msg1"}, modified)
		assert.True(t, fx.getMessage(t, "msg1").MentionRead)
		assert.True(t, fx.getMessage(t, "msg2").MentionRead)  // was already true
		assert.False(t, fx.getMessage(t, "msg3").MentionRead) // no mention, not modified
	})

	t.Run("more than 100 messages are chunked", func(t *testing.T) {
		fx := newFixture(t)
		var ids []string
		for i := range 150 {
			id := fmt.Sprintf("msg%03d", i)
			fx.addMessage(t, id, fmt.Sprintf("order%03d", i), false, false, false)
			ids = append(ids, id)
		}

		modified, err := fx.repo.SetReadFlag(context.Background(), chatObjectId, ids, chatmodel.CounterTypeMessage, true)
		require.NoError(t, err)

		assert.Len(t, modified, 150)
		for _, id := range ids {
			assert.True(t, fx.getMessage(t, id).Read, id)
		}
	})

	t.Run("updates are committed", func(t *testing.T) {
		fx := newFixture(t)
		fx.addMessage(t, "msg1", "order1", false, false, false)
		fx.addMessage(t, "msg2", "order2", false, false, false)

		modified, err := fx.repo.SetReadFlag(context.Background(), chatObjectId, []string{"msg1", "msg2"}, chatmodel.CounterTypeMessage, true)
		require.NoError(t, err)
		assert.Len(t, modified, 2)

		assert.True(t, fx.getMessage(t, "msg1").Read)
		assert.True(t, fx.getMessage(t, "msg2").Read)
	})
}

func TestSetSyncedByMaxOrderId(t *testing.T) {
	t.Run("mark unsynced messages up to max order id", func(t *testing.T) {
		fx := newFixture(t)
		fx.addMessageWithSynced(t, "msg1", "order1", false)
		fx.addMessageWithSynced(t, "msg2", "order2", false)
		fx.addMessageWithSynced(t, "msg3", "order3", false)

		modified, err := fx.repo.SetSyncedByMaxOrderId(context.Background(), "order2")
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"msg1", "msg2"}, modified)
		assert.True(t, fx.getMessage(t, "msg1").Synced)
		assert.True(t, fx.getMessage(t, "msg2").Synced)
		assert.False(t, fx.getMessage(t, "msg3").Synced)
	})

	t.Run("skip already synced messages", func(t *testing.T) {
		fx := newFixture(t)
		fx.addMessageWithSynced(t, "msg1", "order1", true)
		fx.addMessageWithSynced(t, "msg2", "order2", false)

		modified, err := fx.repo.SetSyncedByMaxOrderId(context.Background(), "order2")
		require.NoError(t, err)

		assert.Equal(t, []string{"msg2"}, modified)
	})

	t.Run("empty max order id returns nil", func(t *testing.T) {
		fx := newFixture(t)

		modified, err := fx.repo.SetSyncedByMaxOrderId(context.Background(), "")
		require.NoError(t, err)

		assert.Nil(t, modified)
	})

	t.Run("no unsynced messages in range", func(t *testing.T) {
		fx := newFixture(t)
		fx.addMessageWithSynced(t, "msg1", "order1", true)
		fx.addMessageWithSynced(t, "msg2", "order2", true)

		modified, err := fx.repo.SetSyncedByMaxOrderId(context.Background(), "order2")
		require.NoError(t, err)

		assert.Empty(t, modified)
	})
}

func TestClearUnreadReactions(t *testing.T) {
	t.Run("empty maxOrderId clears all unread reactions", func(t *testing.T) {
		fx := newFixture(t)
		fx.addMessageWithUnreadReactions(t, "msg1", "order1", map[string]map[string]chatmodel.ReactionChangeEntry{
			"👍": {"user1": {ChangeId: "ch1", OrderId: "order1"}},
		})
		fx.addMessageWithUnreadReactions(t, "msg2", "order2", map[string]map[string]chatmodel.ReactionChangeEntry{
			"❤️": {"user2": {ChangeId: "ch2", OrderId: "order2"}},
		})

		modified, err := fx.repo.ClearUnreadReactions(context.Background(), "")
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"msg1", "msg2"}, modified)
		assert.False(t, fx.getMessage(t, "msg1").UnreadReaction)
		assert.False(t, fx.getMessage(t, "msg2").UnreadReaction)
	})

	t.Run("maxOrderId clears only messages with orderId <= maxOrderId", func(t *testing.T) {
		fx := newFixture(t)
		fx.addMessageWithUnreadReactions(t, "msg1", "order1", map[string]map[string]chatmodel.ReactionChangeEntry{
			"👍": {"user1": {ChangeId: "ch1", OrderId: "order1"}},
		})
		fx.addMessageWithUnreadReactions(t, "msg2", "order2", map[string]map[string]chatmodel.ReactionChangeEntry{
			"❤️": {"user2": {ChangeId: "ch2", OrderId: "order2"}},
		})
		fx.addMessageWithUnreadReactions(t, "msg3", "order3", map[string]map[string]chatmodel.ReactionChangeEntry{
			"🎉": {"user3": {ChangeId: "ch3", OrderId: "order3"}},
		})

		modified, err := fx.repo.ClearUnreadReactions(context.Background(), "order2")
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"msg1", "msg2"}, modified)
		assert.False(t, fx.getMessage(t, "msg1").UnreadReaction)
		assert.False(t, fx.getMessage(t, "msg2").UnreadReaction)
		assert.True(t, fx.getMessage(t, "msg3").UnreadReaction)
	})

	t.Run("partial clear removes only entries with orderId <= maxOrderId within a message", func(t *testing.T) {
		fx := newFixture(t)
		// Message has two reactions with different order IDs
		fx.addMessageWithUnreadReactions(t, "msg1", "order2", map[string]map[string]chatmodel.ReactionChangeEntry{
			"👍":  {"user1": {ChangeId: "ch1", OrderId: "order1"}},
			"❤️": {"user2": {ChangeId: "ch2", OrderId: "order3"}},
		})

		// maxOrderId < message's rUnreadOrdId, so partial clear applies
		modified, err := fx.repo.ClearUnreadReactions(context.Background(), "order1")
		require.NoError(t, err)

		assert.Equal(t, []string{"msg1"}, modified)
		msg := fx.getMessage(t, "msg1")
		// The reaction from user2 with order3 > order1 should remain
		assert.True(t, msg.UnreadReaction)
		assert.Nil(t, msg.UnreadReactionIds["👍"])
		require.Len(t, msg.UnreadReactionIds["❤️"], 1)
		assert.Equal(t, "ch2", msg.UnreadReactionIds["❤️"]["user2"].ChangeId)
	})

	t.Run("no unread reactions returns empty", func(t *testing.T) {
		fx := newFixture(t)
		fx.addMessage(t, "msg1", "order1", false, false, false)

		modified, err := fx.repo.ClearUnreadReactions(context.Background(), "")
		require.NoError(t, err)

		assert.Empty(t, modified)
	})

	t.Run("messages without unread reactions are not affected", func(t *testing.T) {
		fx := newFixture(t)
		fx.addMessage(t, "msg1", "order1", false, false, false)
		fx.addMessageWithUnreadReactions(t, "msg2", "order2", map[string]map[string]chatmodel.ReactionChangeEntry{
			"👍": {"user1": {ChangeId: "ch1", OrderId: "order2"}},
		})

		modified, err := fx.repo.ClearUnreadReactions(context.Background(), "")
		require.NoError(t, err)

		assert.Equal(t, []string{"msg2"}, modified)
		assert.False(t, fx.getMessage(t, "msg2").UnreadReaction)
	})

	t.Run("more than 100 messages are batched", func(t *testing.T) {
		fx := newFixture(t)
		for i := range 150 {
			id := fmt.Sprintf("msg%03d", i)
			fx.addMessageWithUnreadReactions(t, id, fmt.Sprintf("order%03d", i), map[string]map[string]chatmodel.ReactionChangeEntry{
				"👍": {"user1": {ChangeId: fmt.Sprintf("ch%03d", i), OrderId: fmt.Sprintf("order%03d", i)}},
			})
		}

		modified, err := fx.repo.ClearUnreadReactions(context.Background(), "")
		require.NoError(t, err)

		assert.Len(t, modified, 150)
		for i := range 150 {
			id := fmt.Sprintf("msg%03d", i)
			assert.False(t, fx.getMessage(t, id).UnreadReaction, id)
		}
	})
}

func TestGetLastMessagesByCreators(t *testing.T) {
	// given
	fx := newFixture(t)
	ctx := context.Background()
	addMsg := func(id, orderId, creator string) {
		require.NoError(t, fx.repo.AddTestMessage(ctx, &chatmodel.Message{
			ChatMessage: &model.ChatMessage{
				Id:      id,
				OrderId: orderId,
				Creator: creator,
				Message: &model.ChatMessageMessageContent{Text: "test"},
			},
		}))
	}
	// identities are case-sensitive: A5alice vs a5alice are different authors
	addMsg("msg1", "o1", "A5alice")
	addMsg("msg2", "o2", "B7bob")
	addMsg("msg3", "o3", "A5alice")
	addMsg("msg4", "o4", "a5alice")

	t.Run("single creator: newest selected, ascending order like GetLastMessages", func(t *testing.T) {
		// when
		got, err := fx.repo.GetLastMessagesByCreators(ctx, []string{"A5alice"}, 10)

		// then
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "msg1", got[0].Id)
		assert.Equal(t, "msg3", got[1].Id)
	})

	t.Run("multiple creators with limit selects the newest", func(t *testing.T) {
		// when: limit 2 must keep the newest two of alice+bob (msg2, msg3)
		got, err := fx.repo.GetLastMessagesByCreators(ctx, []string{"A5alice", "B7bob"}, 2)

		// then
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "msg2", got[0].Id)
		assert.Equal(t, "msg3", got[1].Id)
	})

	t.Run("unknown creator returns empty", func(t *testing.T) {
		// when
		got, err := fx.repo.GetLastMessagesByCreators(ctx, []string{"nobody"}, 10)

		// then
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}
