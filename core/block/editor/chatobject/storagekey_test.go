package chatobject

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/anyproto/any-store/v2/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// pushCreate applies the create of msg as change changeId at order, authored by creator, the way
// a change arrives from the tree; strict returns every apply error, as a local push does.
func (fx *fixture) pushCreate(t *testing.T, creator, changeId, order string, msg *chatmodel.Message, strict bool) error {
	t.Helper()
	arena := &anyenc.Arena{}
	obj := arena.NewObject()
	msg.MarshalAnyenc(obj, arena)
	builder := &storestate.Builder{}
	require.NoError(t, builder.Create(CollectionName, storestate.IdFromChange, obj))
	ctx := context.Background()
	tx, err := fx.store.NewTx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	changeSet := storestate.ChangeSet{
		Id:        changeId,
		Order:     order,
		Changes:   builder.ChangeSet,
		Creator:   creator,
		Timestamp: time.Now().Unix(),
	}
	if strict {
		err = tx.ApplyChangeSetReturnAllErrors(changeSet)
	} else {
		err = tx.ApplyChangeSet(changeSet)
	}
	if err != nil {
		return err
	}
	require.NoError(t, tx.Commit())
	fx.appliedChangeSets = append(fx.appliedChangeSets, changeSet)
	return nil
}

func (fx *fixture) storageKey(t *testing.T, messageId string) string {
	t.Helper()
	coll, err := fx.store.Collection(context.Background(), CollectionName)
	require.NoError(t, err)
	key, err := chatrepository.FindMessageKey(context.Background(), coll, messageId)
	require.NoError(t, err)
	return key
}

func TestMessagesUnderStorageKeys(t *testing.T) {
	ctx := context.Background()

	t.Run("a create is stored under its storage key", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		id := fx.createAs(t, alice, givenSimpleMessage("hello"))

		// then
		msg := fx.getMessage(t, id)
		assert.Equal(t, chatmodel.StorageKey(msg.OrderId, id), fx.storageKey(t, id))
	})

	t.Run("an edit, a reaction and a pin reach the message by id and keep its key", func(t *testing.T) {
		// given
		fx := newFixture(t)
		id := fx.createAs(t, alice, givenSimpleMessage("hello"))
		key := fx.storageKey(t, id)

		// when
		fx.pushAs(t, alice, editBuilder(t, id, "edited"))
		fx.pushAs(t, bob, reactBuilder(t, id, "👍", bob))
		fx.pushAs(t, alice, pinBuilder(t, id))

		// then
		got := fx.getMessage(t, id)
		assert.Equal(t, "edited", got.Message.Text)
		assert.Equal(t, []string{bob}, got.GetReactions().GetReactions()["👍"].GetIds())
		assert.True(t, got.Pinned)
		assert.Equal(t, key, fx.storageKey(t, id), "the key is written once")
	})

	t.Run("a delete reaches the message by id", func(t *testing.T) {
		// given
		fx := newFixture(t)
		id := fx.createAs(t, alice, givenSimpleMessage("hello"))

		// when
		fx.pushAs(t, alice, deleteBuilder(id))

		// then
		msgs, err := fx.GetMessagesByIds(ctx, []string{id})
		require.NoError(t, err)
		assert.Empty(t, msgs)
	})

	t.Run("a reply to my message mentions me: the reply target is found by id", func(t *testing.T) {
		// given
		fx := newFixture(t)
		mine := fx.createAs(t, testCreator, givenSimpleMessage("question"))

		// when
		reply := fx.createAs(t, bob, &chatmodel.Message{ChatMessage: &model.ChatMessage{
			ReplyToMessageId: mine,
			Message:          &model.ChatMessageMessageContent{Text: "answer"},
		}})

		// then
		assert.True(t, fx.getMessage(t, reply).HasMention)
	})

	t.Run("concurrent messages share an order id and keep distinct keys", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.generateOrderIdFunc = func(*storestate.StoreStateTx) string { return "!!$5" }

		// when
		first := fx.createAs(t, alice, givenSimpleMessage("one"))
		second := fx.createAs(t, bob, givenSimpleMessage("two"))

		// then
		resp, err := fx.GetMessages(ctx, chatrepositoryGetAll)
		require.NoError(t, err)
		require.Len(t, resp.Messages, 2)
		assert.ElementsMatch(t, []string{first, second}, []string{resp.Messages[0].Id, resp.Messages[1].Id})
		assert.NotEqual(t, fx.storageKey(t, first), fx.storageKey(t, second))
	})

	t.Run("a create whose key another message holds is refused, never written over it", func(t *testing.T) {
		// given: two changes at one order whose ids end alike
		fx := newFixture(t)
		tail := strings.Repeat("x", 12)
		require.NoError(t, fx.pushCreate(t, alice, "first-change-"+tail, "!!$5", givenSimpleMessage("first"), true))

		// when
		strictErr := fx.pushCreate(t, bob, "other-change-"+tail, "!!$5", givenSimpleMessage("other"), true)
		replayErr := fx.pushCreate(t, bob, "other-change-"+tail, "!!$5", givenSimpleMessage("other"), false)

		// then
		assert.ErrorIs(t, strictErr, ErrStorageKeyCollision)
		assert.NoError(t, replayErr, "a replay logs and skips it")
		assert.Equal(t, "first", fx.getMessage(t, "first-change-"+tail).Message.Text)
		msgs, err := fx.GetMessagesByIds(ctx, []string{"other-change-" + tail})
		require.NoError(t, err)
		assert.Empty(t, msgs)
	})

	t.Run("a full replay into an emptied store rebuilds every message", func(t *testing.T) {
		// given: a chat as the tree holds it
		fx := newFixture(t)
		hello := fx.createAs(t, alice, givenSimpleMessage("hello"))
		world := fx.createAs(t, bob, givenSimpleMessage("world"))
		gone := fx.createAs(t, bob, givenSimpleMessage("gone"))
		fx.pushAs(t, alice, editBuilder(t, hello, "hello, edited"))
		fx.pushAs(t, bob, reactBuilder(t, hello, "👍", bob))
		fx.pushAs(t, bob, pinBuilder(t, world))
		fx.pushAs(t, bob, deleteBuilder(gone))
		want, err := fx.GetMessages(ctx, chatrepositoryGetAll)
		require.NoError(t, err)
		// and a store emptied as a rebuild of the collection leaves it
		coll, err := fx.store.Collection(ctx, CollectionName)
		require.NoError(t, err)
		_, err = coll.Find(nil).Delete(ctx)
		require.NoError(t, err)

		// when
		require.NoError(t, fx.replayStoredChanges(ctx))

		// then
		got, err := fx.GetMessages(ctx, chatrepositoryGetAll)
		require.NoError(t, err)
		require.Len(t, got.Messages, len(want.Messages))
		for i := range want.Messages {
			w, g := want.Messages[i], got.Messages[i]
			assert.Equal(t, w.Id, g.Id)
			assert.Equal(t, w.OrderId, g.OrderId)
			assert.Equal(t, w.Message.Text, g.Message.Text)
			assert.Equal(t, w.GetReactions(), g.GetReactions())
			assert.Equal(t, w.Pinned, g.Pinned)
			assert.Equal(t, chatmodel.StorageKey(w.OrderId, w.Id), fx.storageKey(t, g.Id))
		}
	})
}
