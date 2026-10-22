package chatobject

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TestReindexReopenDoesNotResetUnreadCounter guards against the chat unread-counter "reset"
// that a bump of ForceInvalidateObjectsIndexCounter used to cause (fixed by the existence
// check in ChatHandler.BeforeCreate).
//
// The whole flow (core/indexer/reindex.go):
//   - Bumping ForceInvalidateObjectsIndexCounter sets flags.invalidateObjectsIndex, which
//     ClearHeadsState()s the objectstore heads collection and then reindexOutdatedObjects
//     re-opens EVERY object in the space via space.Do(). The iterator over HeadStorage has
//     no smartblock-type filter, so chat/discussion derived objects are re-opened too.
//   - Opening a chat runs source.ReadStoreDoc -> storeApply.Apply. For a chat whose store
//     predates the `_meta.r` (fullyReplayed) marker, Apply walks the WHOLE tree with
//     IterateRoot (core/block/source/sourceimpl/store_apply.go), i.e. it re-applies every
//     already-stored message change.
//   - storestate.applyCreate calls ChatHandler.BeforeCreate for every such change and only
//     THEN Insert()s it. Already-stored messages hit ErrDocExists -> ErrIgnore, so the
//     persisted read-flags in crdt.db are preserved — but BeforeCreate has already bumped
//     the in-memory unread counter (subscription.UpdateChatState). Nothing sets
//     needReloadState on a create-only replay (only Delete does), so Flush never reloads
//     the true count from the DB, and the inflated counter is what the client sees.
//
// Net effect: a fully-read chat comes back with its unread counter equal to the number of
// not-own messages, even though every message is still marked read in the DB.
//
// The test drives the store through a faithful stand-in for that one-time full replay: it
// re-applies the exact change sets the store already holds, which is precisely what
// storeApply.applyChange does for each change IterateRoot yields on reopen. No object tree
// is needed to exhibit the counter divergence.
func TestReindexReopenDoesNotResetUnreadCounter(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)
	// forceNotRead makes every applied message land as unread, standing in for messages
	// authored by other participants (Creator != currentIdentity) — the case that inflates
	// the counter on replay.
	fx.chatHandler.forceNotRead = true

	const n = 5
	for i := 0; i < n; i++ {
		_, err := fx.AddMessage(ctx, nil, givenSimpleMessage(fmt.Sprintf("message %d", i+1)))
		require.NoError(t, err)
	}

	// The user reads the whole chat: counter goes to zero and every message is read in the DB.
	_, err := fx.MarkReadMessages(ctx, ReadMessagesRequest{
		All:         true,
		CounterType: chatmodel.CounterTypeMessage,
	})
	require.NoError(t, err)

	require.Equal(t, int32(0), fx.subscription.GetChatState().Messages.Counter,
		"precondition: a fully-read chat must have a zero unread counter")

	// Reindex reopen: replay the store's own changes, exactly as storeApply.Apply does on
	// the one-time full IterateRoot pass for a chat without the fullyReplayed marker.
	tx, err := fx.store.NewTx(ctx)
	require.NoError(t, err)
	for _, cs := range fx.appliedChangeSets {
		// Already-stored creates hit ErrDocExists, which applyChangeSet swallows as ErrIgnore.
		require.NoError(t, tx.ApplyChangeSet(cs))
	}
	require.NoError(t, tx.Commit())

	// The persisted state is untouched: every message is still read (the Insert was ignored).
	dbState, err := fx.repository.LoadChatState(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(0), dbState.Messages.Counter,
		"replay must not change the persisted read-flags")

	// Before the fix the replay's BeforeCreate calls inflated the in-memory counter the
	// client sees, resurrecting every already-read message as unread.
	got := fx.subscription.GetChatState().Messages.Counter
	assert.Equal(t, int32(0), got,
		"reopening a fully-read chat during reindex must not resurrect %d unread messages", n)
}

// TestReconcileChatStateHealsDriftedCounters covers the self-healing backstop: whatever
// manages to desynchronize the cached manager's in-memory counters from the persisted
// read-flags (a rolled-back apply after the counters were bumped, a past inflation shipped
// before the BeforeCreate fix), the reconcile run on every object open restores DB truth.
func TestReconcileChatStateHealsDriftedCounters(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)
	fx.chatHandler.forceNotRead = true

	const n = 3
	for i := 0; i < n; i++ {
		_, err := fx.AddMessage(ctx, nil, givenSimpleMessage(fmt.Sprintf("message %d", i+1)))
		require.NoError(t, err)
	}

	// Drift the in-memory state away from the DB truth (n unread, oldest = first message).
	fx.subscription.Lock()
	fx.subscription.UpdateChatState(func(state *model.ChatState) *model.ChatState {
		state.Messages.Counter += 100
		state.Messages.OldestOrderId = ""
		return state
	})
	fx.subscription.UpdateMessageCount(100)
	fx.subscription.Unlock()

	dbState, err := fx.repository.LoadChatState(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(n), dbState.Messages.Counter, "precondition: DB truth is untouched")

	// Reopen path: onInit reconciles under lock before flushing.
	fx.subscription.Lock()
	fx.subscription.ReconcileChatState()
	fx.subscription.Unlock()

	assert.Equal(t, int32(n), fx.subscription.GetChatState().Messages.Counter,
		"reconcile must restore the unread counter from the persisted read-flags")
	assert.Equal(t, dbState.Messages.OldestOrderId, fx.subscription.GetChatState().Messages.OldestOrderId,
		"reconcile must restore the oldest unread order id")
	assert.Equal(t, int32(n), fx.subscription.GetMessageCount(),
		"reconcile must restore the message count")
}
