package chatsubscription

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// observerRecorder collects what a manager hands its observer. The manager
// calls it under its lock, so a plain slice is enough.
type observerRecorder struct {
	changes []ChatChange
}

func (r *observerRecorder) observe(change ChatChange) {
	r.changes = append(r.changes, change)
}

func newObservedManager(t *testing.T) (Manager, *observerRecorder) {
	t.Helper()
	fx := newFixture(t)
	mngr, err := fx.GetManager(testSpaceId, "chatId1")
	require.NoError(t, err)
	rec := &observerRecorder{}
	mngr.Lock()
	mngr.AddObserver("observer1", rec.observe)
	mngr.Unlock()
	return mngr, rec
}

func TestManagerObservers(t *testing.T) {
	t.Run("every mutation path reaches the observer without a window or a session context", func(t *testing.T) {
		// given: no windowed subscription and no session context, so canSend()
		// is false and the window paths drop everything
		mngr, rec := newObservedManager(t)
		added := givenSimpleMessage("msg1", "hello", "o1")
		edited := givenSimpleMessage("msg1", "hello, edited", "o1")
		reacted := givenComplexMessage("msg1", "hello, edited", "o1")
		pinned := givenSimpleMessage("msg1", "hello, edited", "o1")
		pinned.Pinned = true
		want := []ChatChange{
			{Kind: ChatChangeMessageAdded, MessageId: "msg1", Message: added.Clone()},
			{Kind: ChatChangeMessageUpdated, MessageId: "msg1", Message: edited.Clone()},
			{Kind: ChatChangeReactionsUpdated, MessageId: "msg1", Message: reacted.Clone()},
			{Kind: ChatChangePinnedUpdated, MessageId: "msg1", Message: pinned.Clone()},
			{Kind: ChatChangeMessageDeleted, MessageId: "msg1"},
		}

		// when
		mngr.Lock()
		mngr.Add("", added)
		mngr.UpdateFull(edited)
		mngr.UpdateReactions(reacted)
		mngr.UpdatePinned(pinned)
		mngr.Delete("msg1")
		mngr.Unlock()

		// then
		assert.Equal(t, want, rec.changes)
	})

	t.Run("a chat state update reaches the observer as an owned copy", func(t *testing.T) {
		// given
		mngr, rec := newObservedManager(t)

		// when
		mngr.Lock()
		mngr.UpdateChatState(func(state *model.ChatState) *model.ChatState {
			state.Messages.Counter = 2
			state.Mentions.Counter = 1
			state.LastStateId = "state2"
			return state
		})
		live := mngr.GetChatState()
		mngr.UpdateChatState(func(state *model.ChatState) *model.ChatState {
			state.Messages.Counter = 5
			return state
		})
		mngr.Unlock()

		// then
		require.Len(t, rec.changes, 2)
		want := ChatChange{Kind: ChatChangeStateUpdated, State: live}
		assert.Equal(t, want, rec.changes[0], "a later update must not reach an event already handed off")
		assert.Equal(t, int32(5), rec.changes[1].State.GetMessages().GetCounter())
	})

	t.Run("the message payload is an owned copy", func(t *testing.T) {
		// given: a windowed subscription holds the very pointer Add receives,
		// and the window keeps mutating it (read, sync and reaction fields)
		fx := newFixture(t)
		_, err := fx.SubscribeLastMessages(context.Background(), SubscribeLastMessagesRequest{ChatObjectId: "chatId1", SubId: "window", Limit: 10})
		require.NoError(t, err)
		mngr, err := fx.GetManager(testSpaceId, "chatId1")
		require.NoError(t, err)
		rec := &observerRecorder{}
		msg := givenComplexMessage("msg1", "original", "o1")
		want := msg.Clone()

		// when
		mngr.Lock()
		mngr.AddObserver("observer1", rec.observe)
		mngr.Add("", msg)
		msg.Message.Text = "mutated"
		msg.Read = false
		msg.Reactions.Reactions["🥰"].Ids = append(msg.Reactions.Reactions["🥰"].Ids, "identity9")
		mngr.Unlock()

		// then
		require.Len(t, rec.changes, 1)
		assert.Equal(t, want, rec.changes[0].Message)
	})

	t.Run("observers do not make the manager active, so an observed chat stays evictable", func(t *testing.T) {
		// given
		mngr, _ := newObservedManager(t)

		// when
		mngr.Lock()
		active := mngr.IsActive()
		mngr.Unlock()

		// then: IsActive decides object retention (TryClose); counting
		// observers there would keep every watched chat loaded forever
		assert.False(t, active)
	})

	t.Run("RemoveObserver stops delivery and an unknown id is ignored", func(t *testing.T) {
		// given
		mngr, rec := newObservedManager(t)

		// when
		mngr.Lock()
		mngr.Add("", givenSimpleMessage("msg1", "seen", "o1"))
		mngr.RemoveObserver("observer1")
		mngr.RemoveObserver("never-added")
		mngr.Add("", givenSimpleMessage("msg2", "unseen", "o2"))
		mngr.Unlock()

		// then
		require.Len(t, rec.changes, 1)
		assert.Equal(t, "msg1", rec.changes[0].MessageId)
	})

	t.Run("every observer receives each change", func(t *testing.T) {
		// given
		mngr, first := newObservedManager(t)
		second := &observerRecorder{}
		mngr.Lock()
		mngr.AddObserver("observer2", second.observe)
		mngr.Unlock()

		// when
		mngr.Lock()
		mngr.Delete("msg1")
		mngr.Unlock()

		// then
		want := []ChatChange{{Kind: ChatChangeMessageDeleted, MessageId: "msg1"}}
		assert.Equal(t, want, first.changes)
		assert.Equal(t, want, second.changes)
	})
}

// the windowed paths keep working beside an observer: the observer is an
// addition, not a replacement of the existing window subscriptions
func TestManagerObserversBesideWindow(t *testing.T) {
	// given
	fx := newFixture(t)
	ctx := context.Background()
	_, err := fx.SubscribeLastMessages(ctx, SubscribeLastMessagesRequest{ChatObjectId: "chatId1", SubId: "window", Limit: 10})
	require.NoError(t, err)
	mngr, err := fx.GetManager(testSpaceId, "chatId1")
	require.NoError(t, err)
	rec := &observerRecorder{}

	// when
	mngr.Lock()
	mngr.AddObserver("observer1", rec.observe)
	mngr.Add("", givenSimpleMessage("msg1", "hello", "o1"))
	mngr.Flush(false)
	mngr.Unlock()

	// then
	require.Len(t, rec.changes, 1)
	fx.lock.Lock()
	defer fx.lock.Unlock()
	require.Len(t, fx.events, 1, "the window subscription still gets its flushed event")
	assert.Equal(t, "msg1", fx.events[0].Messages[0].GetChatAdd().GetId())
}
