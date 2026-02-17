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

func (f *fixture) getMessage(t *testing.T, id string) *chatmodel.Message {
	t.Helper()
	msgs, err := f.repo.GetMessagesByIds(context.Background(), []string{id})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	return msgs[0]
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
		assert.True(t, fx.getMessage(t, "msg2").MentionRead) // was already true
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
