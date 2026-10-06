package chatobject

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
)

// The unread counters a chat stores as local details: on itself for every chat without a parent
// (the space chat and the other chatDerived chats), on its parent for a discussion. A background
// worker writes them, so every test runs the chat's calls under the object lock, as the RPC
// layer does, and waits for the stored value.
func TestUnreadCountersStoredAsDetails(t *testing.T) {
	ctx := context.Background()

	t.Run("the space chat stores its counters on itself after a message and after a read", func(t *testing.T) {
		// given
		fx := newFixture(t, withSpace(t, chatId))
		fx.chatHandler.forceNotRead = true

		// when
		fx.locked(t, func() error {
			return fx.addMessages(ctx, givenSimpleMessage("hello"), givenMessageWithMention("hello, me"))
		})

		// then
		requireStoredUnreadCounters(t, fx.storeObject, unreadCounters{Stored: true, Messages: 2, Mentions: 1})
		fx.Lock()
		isMainChat := fx.LocalDetails().GetBool(bundle.RelationKeyIsMainChat)
		fx.Unlock()
		assert.True(t, isMainChat, "precondition: the fixture chat is the space chat")

		// when
		fx.locked(t, func() error { return fx.readAll(ctx) })

		// then
		requireStoredUnreadCounters(t, fx.storeObject, unreadCounters{Stored: true})
	})

	t.Run("a chat that is not the space chat stores its counters on itself too", func(t *testing.T) {
		// given
		fx := newFixture(t, withSpace(t, "anotherChatId"))
		fx.chatHandler.forceNotRead = true

		// when
		fx.locked(t, func() error {
			return fx.addMessages(ctx, givenMessageWithMention("hello, me"))
		})

		// then
		requireStoredUnreadCounters(t, fx.storeObject, unreadCounters{Stored: true, Messages: 1, Mentions: 1})
		fx.Lock()
		hasIsMainChat := fx.LocalDetails().Has(bundle.RelationKeyIsMainChat)
		fx.Unlock()
		assert.False(t, hasIsMainChat, "precondition: the fixture chat is not the space chat")
	})

	t.Run("a discussion stores its counters on its parent, never on itself", func(t *testing.T) {
		// given
		parent := smarttest.New("parentId")
		fx := newFixture(t, asDiscussion("parentId"), withSpace(t, "spaceChatId", parent))
		fx.chatHandler.forceNotRead = true

		// when
		fx.locked(t, func() error {
			return fx.addMessages(ctx, givenSimpleMessage("hello"), givenMessageWithMention("hello, me"))
		})

		// then
		requireStoredUnreadCounters(t, parent, unreadCounters{Stored: true, Messages: 2, Mentions: 1})
		assert.Equal(t, unreadCounters{}, storedUnreadCounters(fx.storeObject))
		assert.Equal(t, []string{"parentId"}, fx.spaceObjects.applyTargets(), "every counters write goes to the parent")
	})

	t.Run("the chat's counters are local details, applied without pushing a change", func(t *testing.T) {
		// given
		fx := newFixture(t, withSpace(t, chatId))
		fx.chatHandler.forceNotRead = true

		// when
		fx.locked(t, func() error {
			return fx.addMessages(ctx, givenMessageWithMention("hello, me"))
		})

		// then
		requireStoredUnreadCounters(t, fx.storeObject, unreadCounters{Stored: true, Messages: 1, Mentions: 1})
		applies := fx.spaceObjects.appliesOn(chatId)
		require.NotEmpty(t, applies)
		for _, flags := range applies {
			assert.Contains(t, flags, smartblock.NotPushChanges, "a counters write never pushes a change")
		}
		fx.Lock()
		defer fx.Unlock()
		assert.False(t, fx.Details().Has(bundle.RelationKeyUnreadMessageCount), "not a synced detail")
		assert.False(t, fx.Details().Has(bundle.RelationKeyUnreadMentionCount), "not a synced detail")
		assert.Len(t, fx.appliedChangeSets, 1, "the message is the only change pushed to the store")
	})

	t.Run("a reindex reopen that replays the stored messages does not inflate the stored counters", func(t *testing.T) {
		// given: a fully read chat, its counters stored as zero
		fx := newFixture(t, withSpace(t, chatId))
		fx.chatHandler.forceNotRead = true
		const n = 5
		for i := 0; i < n; i++ {
			fx.locked(t, func() error {
				return fx.addMessages(ctx, givenSimpleMessage(fmt.Sprintf("message %d", i+1)))
			})
		}
		requireStoredUnreadCounters(t, fx.storeObject, unreadCounters{Stored: true, Messages: n})
		fx.locked(t, func() error { return fx.readAll(ctx) })
		requireStoredUnreadCounters(t, fx.storeObject, unreadCounters{Stored: true})

		// when: a reindex reopens the chat — storeApply's one-time full pass re-applies every
		// stored message change (as in TestReindexReopenDoesNotResetUnreadCounter), then the open
		// hook runs — and one more unread message arrives, so the next stored value reflects
		// everything before it
		fx.locked(t, func() error {
			if err := fx.replayStoredChanges(ctx); err != nil {
				return fmt.Errorf("replay stored changes: %w", err)
			}
			fx.onInit(&smartblock.InitContext{Ctx: ctx, State: fx.NewState()})
			return nil
		})
		fx.locked(t, func() error {
			return fx.addMessages(ctx, givenSimpleMessage("after the reopen"))
		})

		// then: one unread message, not the replayed ones on top of it
		requireStoredUnreadCounters(t, fx.storeObject, unreadCounters{Stored: true, Messages: 1})
	})

	t.Run("a chat marked deleted stores nothing more", func(t *testing.T) {
		// given: the write the open triggers has landed
		fx := newFixture(t, withSpace(t, chatId))
		fx.chatHandler.forceNotRead = true
		requireFinishedWrites(t, fx, 1)
		fx.locked(t, func() error {
			fx.SetIsDeleted()
			return nil
		})

		// when
		fx.locked(t, func() error {
			return fx.addMessages(ctx, givenSimpleMessage("hello"))
		})

		// then: the worker handles the message's trigger without applying anything
		requireFinishedWrites(t, fx, 2)
		assert.Len(t, fx.spaceObjects.appliesOn(chatId), 1, "only the open's write was applied")
		assert.Equal(t, unreadCounters{Stored: true}, storedUnreadCounters(fx.storeObject))
	})
}

func TestUnreadCountersWorkerLifetime(t *testing.T) {
	t.Run("closing the chat stops its counters worker", func(t *testing.T) {
		// given
		fx := newFixture(t, withSpace(t, chatId))
		requireFinishedWrites(t, fx, 1)

		// when
		require.NoError(t, fx.Close())

		// then: the worker is gone, and a trigger after the close neither blocks nor writes
		requireWorkerStopped(t, fx)
		fx.triggerUnreadCountersUpdate()
		fx.triggerUnreadCountersUpdate()
		assert.Equal(t, 1, fx.spaceObjects.finishedDos(), "nothing is written after the close")
	})

	t.Run("a chat the cache evicts through TryClose stops its counters worker", func(t *testing.T) {
		// given: the cache does not call Close after a TryClose that succeeded
		fx := newFixture(t, withSpace(t, chatId), withEvictableSmartBlock())
		requireFinishedWrites(t, fx, 1)

		// when
		closed, err := fx.TryClose(time.Minute)

		// then
		require.NoError(t, err)
		require.True(t, closed)
		requireWorkerStopped(t, fx)
	})

	t.Run("a chat the cache keeps open keeps its counters worker", func(t *testing.T) {
		// given: smarttest's TryClose always declines, as a chat with an open session does
		fx := newFixture(t, withSpace(t, chatId))
		requireFinishedWrites(t, fx, 1)

		// when
		closed, err := fx.TryClose(time.Minute)
		require.NoError(t, err)
		require.False(t, closed)
		fx.chatHandler.forceNotRead = true
		fx.locked(t, func() error {
			return fx.addMessages(context.Background(), givenSimpleMessage("still open"))
		})

		// then
		requireStoredUnreadCounters(t, fx.storeObject, unreadCounters{Stored: true, Messages: 1})
	})
}

// requireWorkerStopped waits for the unread counters worker to return.
func requireWorkerStopped(t *testing.T, fx *fixture) {
	t.Helper()
	select {
	case <-fx.unreadCountersDone:
	case <-time.After(10 * time.Second):
		t.Fatal("the unread counters worker is still running")
	}
}

// evictableSmartTest lets the cache's TryClose succeed, which smarttest's never does.
type evictableSmartTest struct {
	*smarttest.SmartTest
}

func (e *evictableSmartTest) TryClose(time.Duration) (bool, error) {
	return true, nil
}

func withEvictableSmartBlock() fixtureOption {
	return func(fx *fixture) {
		fx.storeObject.SmartBlock = &evictableSmartTest{SmartTest: fx.sb}
	}
}

// unreadCounters is what an object stores as the two local details; Stored is false while
// neither is set.
type unreadCounters struct {
	Stored   bool
	Messages int64
	Mentions int64
}

// storedUnreadCounters reads the counters under the object's lock, the lock their writer holds.
func storedUnreadCounters(sb smartblock.SmartBlock) unreadCounters {
	sb.Lock()
	defer sb.Unlock()
	details := sb.LocalDetails()
	return unreadCounters{
		Stored:   details.Has(bundle.RelationKeyUnreadMessageCount) || details.Has(bundle.RelationKeyUnreadMentionCount),
		Messages: details.GetInt64(bundle.RelationKeyUnreadMessageCount),
		Mentions: details.GetInt64(bundle.RelationKeyUnreadMentionCount),
	}
}

// requireStoredUnreadCounters waits for the worker to store want on the object.
func requireStoredUnreadCounters(t *testing.T, sb smartblock.SmartBlock, want unreadCounters) {
	t.Helper()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, want, storedUnreadCounters(sb))
	}, 10*time.Second, time.Millisecond)
}

// requireFinishedWrites waits until Space().Do has returned at least n times.
func requireFinishedWrites(t *testing.T, fx *fixture, n int) {
	t.Helper()
	require.Eventually(t, func() bool { return fx.spaceObjects.finishedDos() >= n }, 10*time.Second, time.Millisecond)
}

// locked runs f under the object lock, as the RPC layer runs every call on a chat.
func (fx *fixture) locked(t *testing.T, f func() error) {
	t.Helper()
	fx.Lock()
	defer fx.Unlock()
	require.NoError(t, f())
}

func (fx *fixture) addMessages(ctx context.Context, messages ...*chatmodel.Message) error {
	for _, message := range messages {
		if _, err := fx.AddMessage(ctx, nil, message); err != nil {
			return fmt.Errorf("add message: %w", err)
		}
	}
	return nil
}

func (fx *fixture) readAll(ctx context.Context) error {
	for _, counterType := range []chatmodel.CounterType{chatmodel.CounterTypeMessage, chatmodel.CounterTypeMention} {
		if _, err := fx.MarkReadMessages(ctx, ReadMessagesRequest{All: true, CounterType: counterType}); err != nil {
			return fmt.Errorf("mark read: %w", err)
		}
	}
	return nil
}

// replayStoredChanges re-applies every change set the store holds, as storeApply does on the
// one-time full pass of a reopen.
func (fx *fixture) replayStoredChanges(ctx context.Context) error {
	tx, err := fx.store.NewTx(ctx)
	if err != nil {
		return fmt.Errorf("new tx: %w", err)
	}
	for _, cs := range fx.appliedChangeSets {
		if err := tx.ApplyChangeSet(cs); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply change set: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// asDiscussion makes the fixture chat a discussion of parentId.
func asDiscussion(parentId string) fixtureOption {
	return func(fx *fixture) {
		fx.typeKey = bundle.TypeKeyDiscussion
		fx.layout = model.ObjectType_discussion
		fx.sb.SetTree(&stubTree{parentId: parentId})
	}
}

// withSpace places the chat in a space whose main chat is spaceChatId. Its Space().Do reaches the
// chat itself and the given objects.
func withSpace(t *testing.T, spaceChatId string, others ...smartblock.SmartBlock) fixtureOption {
	return func(fx *fixture) {
		objects := &spaceObjects{objects: map[string]smartblock.SmartBlock{fx.Id(): fx.storeObject}}
		for _, other := range others {
			objects.objects[other.Id()] = other
		}
		spc := smartblock.NewMockSpace(t)
		spc.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{SpaceChat: spaceChatId}).Maybe()
		spc.EXPECT().Do(mock.Anything, mock.Anything).RunAndReturn(objects.do).Maybe()
		fx.spaceObjects = objects
		fx.sb.SetSpace(spc)
	}
}

// spaceObjects is what a test space's Do reaches: each object is locked as the object cache locks
// it, and every Apply made through Do is recorded.
type spaceObjects struct {
	objects map[string]smartblock.SmartBlock

	mu       sync.Mutex
	finished int
	applies  []recordedApply
}

type recordedApply struct {
	targetId string
	flags    []smartblock.ApplyFlag
}

func (o *spaceObjects) do(objectId string, apply func(smartblock.SmartBlock) error) error {
	defer func() {
		o.mu.Lock()
		o.finished++
		o.mu.Unlock()
	}()
	sb, ok := o.objects[objectId]
	if !ok {
		return fmt.Errorf("get object %s: %w", objectId, treestorage.ErrUnknownTreeId)
	}
	sb.Lock()
	defer sb.Unlock()
	return apply(&recordingObject{SmartBlock: sb, objects: o})
}

func (o *spaceObjects) finishedDos() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.finished
}

func (o *spaceObjects) appliesOn(objectId string) [][]smartblock.ApplyFlag {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out [][]smartblock.ApplyFlag
	for _, a := range o.applies {
		if a.targetId == objectId {
			out = append(out, a.flags)
		}
	}
	return out
}

func (o *spaceObjects) applyTargets() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []string
	for _, a := range o.applies {
		if !slices.Contains(out, a.targetId) {
			out = append(out, a.targetId)
		}
	}
	return out
}

// recordingObject is the object Do hands to its callback: the object itself, recording each Apply.
type recordingObject struct {
	smartblock.SmartBlock
	objects *spaceObjects
}

func (r *recordingObject) Apply(st *state.State, flags ...smartblock.ApplyFlag) error {
	r.objects.mu.Lock()
	r.objects.applies = append(r.objects.applies, recordedApply{targetId: r.Id(), flags: flags})
	r.objects.mu.Unlock()
	return r.SmartBlock.Apply(st, flags...)
}
