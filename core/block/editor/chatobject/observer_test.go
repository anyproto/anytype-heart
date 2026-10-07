package chatobject

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
)

// The chat state manager's observer is what the API v2 space-wide chat stream
// is built on. These tests drive the real mutation paths (BeforeCreate,
// UpgradeKeyModifier, BeforeDelete) with no windowed subscription and no
// session context, which is exactly the state of a chat nobody has open.
func TestManagerObserverThroughChatObject(t *testing.T) {
	ctx := context.Background()

	t.Run("an added message reaches the observer with its state id", func(t *testing.T) {
		// given
		fx := newFixture(t)
		var changes []chatsubscription.ChatChange
		fx.subscription.Lock()
		fx.subscription.AddObserver("test", func(change chatsubscription.ChatChange) {
			changes = append(changes, change)
		})
		fx.subscription.Unlock()

		// when
		messageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage("hello"))
		require.NoError(t, err)

		// then: the counters move first (BeforeCreate updates state before Add),
		// then the message itself
		stored, err := fx.GetMessageById(ctx, messageId)
		require.NoError(t, err)
		var kinds []chatsubscription.ChatChangeKind
		var added *chatsubscription.ChatChange
		for i, change := range changes {
			kinds = append(kinds, change.Kind)
			if change.Kind == chatsubscription.ChatChangeMessageAdded {
				added = &changes[i]
			}
		}
		assert.Equal(t, []chatsubscription.ChatChangeKind{
			chatsubscription.ChatChangeStateUpdated,
			chatsubscription.ChatChangeMessageAdded,
		}, kinds)
		require.NotNil(t, added)
		assert.Equal(t, messageId, added.MessageId)
		assert.Equal(t, stored.StateId, added.Message.StateId)
		assert.Equal(t, "hello", added.Message.Message.Text)
		assert.Equal(t, stored.StateId, changes[0].State.LastStateId)
	})

	t.Run("an edit, a reaction, a pin and a deletion reach the observer", func(t *testing.T) {
		// given
		fx := newFixture(t)
		messageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage("hello"))
		require.NoError(t, err)
		var kinds []chatsubscription.ChatChangeKind
		fx.subscription.Lock()
		fx.subscription.AddObserver("test", func(change chatsubscription.ChatChange) {
			if change.Kind != chatsubscription.ChatChangeStateUpdated {
				kinds = append(kinds, change.Kind)
			}
		})
		fx.subscription.Unlock()

		// when
		require.NoError(t, fx.EditMessage(ctx, messageId, givenSimpleMessage("hello, edited")))
		_, err = fx.ToggleMessageReaction(ctx, messageId, "👍")
		require.NoError(t, err)
		require.NoError(t, fx.SetMessagePinned(ctx, messageId, true))
		require.NoError(t, fx.DeleteMessage(ctx, messageId))

		// then
		want := []chatsubscription.ChatChangeKind{
			chatsubscription.ChatChangeMessageUpdated,
			chatsubscription.ChatChangeReactionsUpdated,
			chatsubscription.ChatChangePinnedUpdated,
			chatsubscription.ChatChangeMessageDeleted,
		}
		assert.Equal(t, want, kinds)
	})

	t.Run("an observed chat is not active, so it stays evictable", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.subscription.Lock()
		fx.subscription.AddObserver("test", func(chatsubscription.ChatChange) {})
		fx.subscription.Unlock()

		// when
		fx.subscription.Lock()
		active := fx.subscription.IsActive()
		fx.subscription.Unlock()

		// then: TryClose keeps a chat loaded only while IsActive
		assert.False(t, active)
	})
}
