package api

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription/mock_chatsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestChatSubAdapterChatState(t *testing.T) {
	t.Run("returns the manager's state under its lock", func(t *testing.T) {
		// given
		svc := mock_chatsubscription.NewMockService(t)
		mngr := mock_chatsubscription.NewMockManager(t)
		want := &model.ChatState{
			Messages: &model.ChatStateUnreadState{Counter: 3},
			Mentions: &model.ChatStateUnreadState{Counter: 1},
		}
		svc.EXPECT().GetManager("space1", "chat1").Return(mngr, nil)
		mngr.EXPECT().Lock().Once()
		mngr.EXPECT().GetChatState().Return(want).Once()
		mngr.EXPECT().Unlock().Once()
		adapter := &chatSubAdapter{svc: svc}

		// when
		got, err := adapter.ChatState("space1", "chat1")

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("a manager that cannot be created is an error", func(t *testing.T) {
		// given
		svc := mock_chatsubscription.NewMockService(t)
		svc.EXPECT().GetManager("space1", "chat1").Return(nil, errors.New("no repository"))
		adapter := &chatSubAdapter{svc: svc}

		// when
		got, err := adapter.ChatState("space1", "chat1")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get chat manager")
		assert.Nil(t, got)
	})
}
