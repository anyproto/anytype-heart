package chatobject

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscription(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)

	for i := 0; i < 10; i++ {
		inputMessage := givenComplexMessage()
		inputMessage.Message.Text = fmt.Sprintf("text %d", i+1)
		messageId, err := fx.AddMessage(ctx, nil, inputMessage)
		require.NoError(t, err)
		assert.NotEmpty(t, messageId)
	}

	resp, err := fx.SubscribeLastMessages(ctx, "subId", 5, false)
	require.NoError(t, err)
	wantTexts := []string{"text 6", "text 7", "text 8", "text 9", "text 10"}
	for i, msg := range resp.Messages {
		assert.Equal(t, wantTexts[i], msg.Message.Text)
	}

	t.Run("add message", func(t *testing.T) {
		fx.events = nil

		messageId, err := fx.AddMessage(ctx, nil, givenComplexMessage())
		require.NoError(t, err)
		require.Len(t, fx.events, 2)

		ev := fx.events[0].GetChatAdd()
		require.NotNil(t, ev)
		assert.Equal(t, messageId, ev.Id)

		evState := fx.events[1].GetChatStateUpdate()
		require.NotNil(t, evState)
		assert.True(t, evState.State.DbTimestamp > 0)
	})

	t.Run("edit message", func(t *testing.T) {
		fx.events = nil

		edited := givenComplexMessage()
		edited.Message.Text = "edited text"

		err = fx.EditMessage(ctx, resp.Messages[0].Id, edited)
		require.NoError(t, err)
		require.Len(t, fx.events, 1)

		ev := fx.events[0].GetChatUpdate()
		require.NotNil(t, ev)
		assert.Equal(t, resp.Messages[0].Id, ev.Id)
		assert.Equal(t, edited.Message.Text, ev.Message.Message.Text)
	})

	t.Run("toggle message reaction", func(t *testing.T) {
		fx.events = nil

		err = fx.ToggleMessageReaction(ctx, resp.Messages[0].Id, "👍")
		require.NoError(t, err)
		require.Len(t, fx.events, 1)

		ev := fx.events[0].GetChatUpdateReactions()
		require.NotNil(t, ev)
		assert.Equal(t, resp.Messages[0].Id, ev.Id)
		_, ok := ev.Reactions.Reactions["👍"]
		assert.True(t, ok)
	})

	t.Run("delete message", func(t *testing.T) {
		fx.events = nil

		err = fx.DeleteMessage(ctx, resp.Messages[0].Id)
		require.NoError(t, err)
		require.Len(t, fx.events, 1)

		ev := fx.events[0].GetChatDelete()
		require.NotNil(t, ev)
		assert.Equal(t, resp.Messages[0].Id, ev.Id)
	})
}
