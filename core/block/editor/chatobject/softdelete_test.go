package chatobject

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	alice = "alice"
	bob   = "bob"
)

// push applies the builder's changes as one change set authored by creator at the given ACL
// head, the way a change from another member arrives, and runs the post-commit update.
func (fx *fixture) push(creator, aclHeadId string, builder *storestate.Builder) (string, error) {
	ctx := context.Background()
	tx, err := fx.store.NewTx(ctx)
	if err != nil {
		return "", fmt.Errorf("new tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()
	changeSet := storestate.ChangeSet{
		Id:        bson.NewObjectId().Hex(),
		Order:     fx.generateOrderId(tx),
		Changes:   builder.ChangeSet,
		Creator:   creator,
		AclHeadId: aclHeadId,
		Timestamp: time.Now().Unix(),
	}
	if err = tx.ApplyChangeSetReturnAllErrors(changeSet); err != nil {
		return "", fmt.Errorf("apply change set: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	fx.appliedChangeSets = append(fx.appliedChangeSets, changeSet)
	fx.onUpdate()
	return changeSet.Id, nil
}

func (fx *fixture) pushAs(t *testing.T, creator string, builder *storestate.Builder) string {
	t.Helper()
	id, err := fx.push(creator, "", builder)
	require.NoError(t, err)
	return id
}

func (fx *fixture) createAs(t *testing.T, creator string, msg *chatmodel.Message) string {
	t.Helper()
	arena := &anyenc.Arena{}
	obj := arena.NewObject()
	msg.MarshalAnyenc(obj, arena)
	builder := &storestate.Builder{}
	require.NoError(t, builder.Create(CollectionName, storestate.IdFromChange, obj))
	return fx.pushAs(t, creator, builder)
}

func deleteBuilder(messageId string) *storestate.Builder {
	builder := &storestate.Builder{}
	builder.Delete(CollectionName, messageId)
	return builder
}

func editBuilder(t *testing.T, messageId, text string) *storestate.Builder {
	arena := &anyenc.Arena{}
	obj := arena.NewObject()
	givenSimpleMessage(text).MarshalAnyenc(obj, arena)
	builder := &storestate.Builder{}
	require.NoError(t, builder.Modify(CollectionName, messageId, []string{chatmodel.ContentKey}, pb.ModifyOp_Set, obj.Get(chatmodel.ContentKey)))
	return builder
}

func reactBuilder(t *testing.T, messageId, emoji, identity string) *storestate.Builder {
	builder := &storestate.Builder{}
	require.NoError(t, builder.Modify(CollectionName, messageId, []string{chatmodel.ReactionsKey, emoji}, pb.ModifyOp_AddToSet, (&anyenc.Arena{}).NewString(identity)))
	return builder
}

func pinBuilder(t *testing.T, messageId string) *storestate.Builder {
	builder := &storestate.Builder{}
	require.NoError(t, builder.Modify(CollectionName, messageId, []string{chatmodel.PinnedKey}, pb.ModifyOp_Set, (&anyenc.Arena{}).NewTrue()))
	return builder
}

func (fx *fixture) getMessage(t *testing.T, id string) *chatmodel.Message {
	t.Helper()
	msgs, err := fx.GetMessagesByIds(context.Background(), []string{id})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	return msgs[0]
}

func (fx *fixture) messageCount(t *testing.T) int32 {
	t.Helper()
	resp, err := fx.GetMessages(context.Background(), chatrepositoryGetAll)
	require.NoError(t, err)
	return resp.MessageCount
}

// wantTombstone is the stored form of a deleted comment, built from the message as it was
// before the delete: its id, author, time, reply target, place and sync flag stay.
func wantTombstone(before *chatmodel.Message) *model.ChatMessage {
	return &model.ChatMessage{
		Id:               before.Id,
		OrderId:          before.OrderId,
		Creator:          before.Creator,
		CreatedAt:        before.CreatedAt,
		StateId:          before.StateId,
		ReplyToMessageId: before.ReplyToMessageId,
		Message:          &model.ChatMessageMessageContent{Marks: []*model.BlockContentTextMark{}},
		Attachments:      []*model.ChatMessageAttachment{},
		Reactions:        &model.ChatMessageReactions{},
		Read:             true,
		MentionRead:      true,
		Synced:           before.Synced,
		Deleted:          true,
	}
}

// rawOrders is the stored order object (_o) of a message: the delete must leave it untouched.
func (fx *fixture) rawOrders(t *testing.T, id string) string {
	t.Helper()
	coll, err := fx.store.Collection(context.Background(), CollectionName)
	require.NoError(t, err)
	doc, err := coll.FindId(context.Background(), id)
	require.NoError(t, err)
	orders := doc.Value().Get(chatmodel.OrderKey)
	require.NotNil(t, orders)
	return orders.String()
}

func TestCreateNeverArrivesDeleted(t *testing.T) {
	t.Run("a create carrying deleted is stored as a live message", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		msg := givenSimpleMessage("hello")
		msg.Deleted = true

		// when
		id := fx.createAs(t, alice, msg)

		// then
		got := fx.getMessage(t, id)
		assert.False(t, got.Deleted)
		assert.Equal(t, "hello", got.Message.Text)
	})

	t.Run("AddMessage does not send deleted", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		msg := givenSimpleMessage("hello")
		msg.Deleted = true

		// when
		id, err := fx.AddMessage(context.Background(), nil, msg)

		// then
		require.NoError(t, err)
		assert.False(t, fx.getMessage(t, id).Deleted)
		pushed := fx.appliedChangeSets[len(fx.appliedChangeSets)-1].Changes[0].GetCreate().GetValue()
		assert.NotContains(t, pushed, chatmodel.DeletedKey)
	})
}

func TestDiscussionDeleteLeavesTombstone(t *testing.T) {
	t.Run("the deleted comment keeps its author, time, reply target and place", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		msg := givenComplexMessage()
		msg.ChatMessage.Blocks = []*model.ChatMessageMessageBlock{{Content: &model.ChatMessageMessageBlockContentOfText{
			Text: &model.ChatMessageMessageBlockText{Text: "block text"},
		}}}
		id := fx.createAs(t, alice, msg)
		fx.pushAs(t, alice, editBuilder(t, id, "edited"))
		fx.pushAs(t, alice, pinBuilder(t, id))
		before := fx.getMessage(t, id)
		ordersBefore := fx.rawOrders(t, id)
		require.Equal(t, "replyToMessageId1", before.ReplyToMessageId)
		require.NotZero(t, before.CreatedAt)

		// when
		fx.pushAs(t, alice, deleteBuilder(id))

		// then
		got := fx.getMessage(t, id)
		assert.Equal(t, wantTombstone(before), got.ChatMessage)
		assert.Equal(t, ordersBefore, fx.rawOrders(t, id))
		resp, err := fx.GetMessages(context.Background(), chatrepositoryGetAll)
		require.NoError(t, err)
		require.Len(t, resp.Messages, 1, "the tombstone stays in the list")
		assert.Equal(t, int32(0), resp.MessageCount)
		pinned, err := fx.GetPinnedMessages(context.Background())
		require.NoError(t, err)
		assert.Empty(t, pinned)
	})

	t.Run("the fulltext index drops the deleted comment", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		id := fx.createAs(t, alice, givenSimpleMessage("hello"))

		// when
		fx.pushAs(t, alice, deleteBuilder(id))

		// then
		queued, err := fx.storeFixture.ListIdsFromFullTextQueue([]string{testSpaceId}, 0)
		require.NoError(t, err)
		require.Len(t, queued, 1)
		assert.Equal(t, []string{id}, queued[0].DeletedMsgIds)
	})

	t.Run("a space chat delete still removes the message", func(t *testing.T) {
		// given
		fx := newFixture(t)
		id := fx.createAs(t, alice, givenSimpleMessage("hello"))

		// when
		fx.pushAs(t, alice, deleteBuilder(id))

		// then
		msgs, err := fx.GetMessagesByIds(context.Background(), []string{id})
		require.NoError(t, err)
		assert.Empty(t, msgs)
	})

	t.Run("a writer can't delete someone else's comment", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		id := fx.createAs(t, alice, givenSimpleMessage("hello"))

		// when
		_, err := fx.push(bob, "", deleteBuilder(id))

		// then
		require.ErrorIs(t, err, ErrDeleteForeignMessage)
		got := fx.getMessage(t, id)
		assert.False(t, got.Deleted)
		assert.Equal(t, "hello", got.Message.Text)
	})

	for _, moderator := range []string{"a", "adm"} {
		t.Run("a moderator can delete someone else's comment: "+map[string]string{"a": "Owner", "adm": "Admin"}[moderator], func(t *testing.T) {
			// given
			exec := list.NewAclExecutor("space")
			for _, cmd := range []string{"a.init::a", "a.invite::invId", "adm.join::invId", "a.approve::adm,adm", "writer.join::invId", "a.approve::writer,rw"} {
				require.NoError(t, exec.Execute(cmd), cmd)
			}
			acl := exec.ActualAccounts()["a"].Acl
			deleter := exec.ActualAccounts()[moderator].Keys.SignKey.GetPublic().Account()
			writer := exec.ActualAccounts()["writer"].Keys.SignKey.GetPublic().Account()
			fx := newFixture(t, asDiscussion("parentId"))
			fx.chatHandler.aclList = acl
			id := fx.createAs(t, writer, givenSimpleMessage("hello"))
			before := fx.getMessage(t, id)

			// when
			_, err := fx.push(deleter, acl.Head().Id, deleteBuilder(id))

			// then
			require.NoError(t, err)
			assert.Equal(t, wantTombstone(before), fx.getMessage(t, id).ChatMessage)
		})
	}

	t.Run("a second delete changes nothing", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		id := fx.createAs(t, alice, givenSimpleMessage("hello"))
		fx.pushAs(t, alice, deleteBuilder(id))
		before := fx.getMessage(t, id)
		subscribeForEvents(t, fx)
		fx.events = nil
		require.NoError(t, fx.storeFixture.ClearFullTextQueue([]string{testSpaceId}))
		var observed []chatsubscription.ChatChange
		fx.subscription.Lock()
		fx.subscription.AddObserver("test", func(change chatsubscription.ChatChange) {
			observed = append(observed, change)
		})
		fx.subscription.Unlock()

		// when
		fx.pushAs(t, bob, deleteBuilder(id))

		// then
		assert.Equal(t, before, fx.getMessage(t, id))
		for _, ev := range fx.events {
			assert.Nil(t, ev.GetChatUpdate(), "no event for a message that is already deleted")
			assert.Nil(t, ev.GetChatDelete())
		}
		for _, change := range observed {
			assert.Equal(t, chatsubscription.ChatChangeStateUpdated, change.Kind, "observers see no message change")
		}
		queued, err := fx.storeFixture.ListIdsFromFullTextQueue([]string{testSpaceId}, 0)
		require.NoError(t, err)
		for _, entry := range queued {
			assert.Empty(t, entry.DeletedMsgIds, "no fulltext work for a message that is already deleted")
		}
	})
}

// subscribeForEvents opens a subscription so the manager sends window events to fx.events.
func subscribeForEvents(t *testing.T, fx *fixture) {
	t.Helper()
	_, err := fx.chatSubscriptionService.SubscribeLastMessages(context.Background(), subscribeRequest(fx))
	require.NoError(t, err)
}

func TestDeleteWinsOverConcurrentModifies(t *testing.T) {
	modifies := map[string]func(t *testing.T, id string) (string, *storestate.Builder){
		"an edit":    func(t *testing.T, id string) (string, *storestate.Builder) { return alice, editBuilder(t, id, "edited") },
		"a reaction": func(t *testing.T, id string) (string, *storestate.Builder) { return bob, reactBuilder(t, id, "👍", bob) },
		"a pin":      func(t *testing.T, id string) (string, *storestate.Builder) { return alice, pinBuilder(t, id) },
	}
	for name, modify := range modifies {
		t.Run(name+" applied after the delete is dropped", func(t *testing.T) {
			// given
			fx := newFixture(t, asDiscussion("parentId"))
			id := fx.createAs(t, alice, givenSimpleMessage("hello"))
			before := fx.getMessage(t, id)
			ordersBefore := fx.rawOrders(t, id)
			fx.pushAs(t, alice, deleteBuilder(id))
			creator, builder := modify(t, id)

			// when
			fx.pushAs(t, creator, builder)

			// then
			assert.Equal(t, wantTombstone(before), fx.getMessage(t, id).ChatMessage)
			assert.Equal(t, ordersBefore, fx.rawOrders(t, id), "the dropped modify leaves no order behind")
		})

		t.Run(name+" applied before the delete is cleared", func(t *testing.T) {
			// given
			fx := newFixture(t, asDiscussion("parentId"))
			id := fx.createAs(t, alice, givenSimpleMessage("hello"))
			creator, builder := modify(t, id)
			fx.pushAs(t, creator, builder)
			before := fx.getMessage(t, id)

			// when
			fx.pushAs(t, alice, deleteBuilder(id))

			// then
			assert.Equal(t, wantTombstone(before), fx.getMessage(t, id).ChatMessage)
		})
	}
}

func TestReplayBuildsTombstone(t *testing.T) {
	// given
	ctx := context.Background()
	fx := newFixture(t, asDiscussion("parentId"))
	id := fx.createAs(t, alice, givenSimpleMessage("hello"))
	replyId := fx.createAs(t, bob, &chatmodel.Message{ChatMessage: &model.ChatMessage{
		ReplyToMessageId: id,
		Message:          &model.ChatMessageMessageContent{Text: "reply"},
	}})
	before := fx.getMessage(t, id)
	fx.pushAs(t, alice, deleteBuilder(id))
	coll, err := fx.store.Collection(ctx, CollectionName)
	require.NoError(t, err)
	for _, docId := range []string{id, replyId} {
		require.NoError(t, coll.DeleteId(ctx, docId))
	}

	// when
	require.NoError(t, fx.replayStoredChanges(ctx))

	// then
	got := fx.getMessage(t, id)
	want := wantTombstone(before)
	want.StateId = got.StateId // a store stamps its own state id when it stores a create
	assert.Equal(t, want, got.ChatMessage)
	assert.Equal(t, "reply", fx.getMessage(t, replyId).Message.Text)
}

func TestModifyOfMissingMessageCreatesNothing(t *testing.T) {
	// given
	fx := newFixture(t)
	id := fx.createAs(t, alice, givenSimpleMessage("hello"))
	fx.pushAs(t, alice, deleteBuilder(id))

	// when
	fx.pushAs(t, bob, reactBuilder(t, id, "👍", bob))
	fx.pushAs(t, alice, pinBuilder(t, id))

	// then
	msgs, err := fx.GetMessagesByIds(context.Background(), []string{id})
	require.NoError(t, err)
	assert.Empty(t, msgs, "a modify of a deleted message must not re-create it")
	count, err := fx.repository.CountMessages(context.Background())
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestReplyToDeletedCommentIsMention(t *testing.T) {
	for _, replyFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("reply applied before the delete: %v", replyFirst), func(t *testing.T) {
			// given
			fx := newFixture(t, asDiscussion("parentId"))
			id, err := fx.AddMessage(context.Background(), nil, givenSimpleMessage("mine"))
			require.NoError(t, err)
			reply := &chatmodel.Message{ChatMessage: &model.ChatMessage{
				ReplyToMessageId: id,
				Message:          &model.ChatMessageMessageContent{Text: "reply"},
			}}

			// when
			var replyId string
			if replyFirst {
				replyId = fx.createAs(t, bob, reply)
				fx.pushAs(t, testCreator, deleteBuilder(id))
			} else {
				fx.pushAs(t, testCreator, deleteBuilder(id))
				replyId = fx.createAs(t, bob, reply)
			}

			// then
			assert.True(t, fx.getMessage(t, replyId).HasMention)
			assert.Equal(t, int32(1), fx.subscription.GetChatState().Mentions.Counter)
		})
	}
}

func TestDeletedCommentLeavesTheCounters(t *testing.T) {
	ctx := context.Background()
	reply := func(to string) *chatmodel.Message {
		return &chatmodel.Message{ChatMessage: &model.ChatMessage{
			ReplyToMessageId: to,
			Message:          &model.ChatMessageMessageContent{Text: "reply"},
		}}
	}

	t.Run("an unread message", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		id := fx.createAs(t, bob, givenSimpleMessage("hello"))
		require.Equal(t, int32(1), fx.subscription.GetChatState().Messages.Counter)

		// when
		fx.pushAs(t, bob, deleteBuilder(id))

		// then
		assert.Equal(t, int32(0), fx.subscription.GetChatState().Messages.Counter)
		assert.Equal(t, int32(0), fx.messageCount(t))
	})

	t.Run("an unread mention", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		id := fx.createAs(t, bob, givenMessageWithMention("@me"))
		require.Equal(t, int32(1), fx.subscription.GetChatState().Mentions.Counter)

		// when
		fx.pushAs(t, bob, deleteBuilder(id))

		// then
		assert.Equal(t, int32(0), fx.subscription.GetChatState().Mentions.Counter)
	})

	t.Run("an unread reply to my message", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		mine, err := fx.AddMessage(ctx, nil, givenSimpleMessage("mine"))
		require.NoError(t, err)
		replyId := fx.createAs(t, bob, reply(mine))
		require.Equal(t, int32(1), fx.subscription.GetChatState().Mentions.Counter)

		// when
		fx.pushAs(t, bob, deleteBuilder(replyId))

		// then
		assert.Equal(t, int32(0), fx.subscription.GetChatState().Mentions.Counter)
		assert.Equal(t, int32(0), fx.subscription.GetChatState().Messages.Counter)
	})

	t.Run("unread reactions on my deleted message", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"), withReactionsCounterEpoch(0))
		mine, err := fx.AddMessage(ctx, nil, givenSimpleMessage("mine"))
		require.NoError(t, err)
		fx.pushAs(t, bob, reactBuilder(t, mine, "👍", bob))
		require.NotEmpty(t, fx.subscription.GetChatState().UnreadReactionOrderId)

		// when
		fx.pushAs(t, testCreator, deleteBuilder(mine))

		// then
		assert.Empty(t, fx.subscription.GetChatState().UnreadReactionOrderId)
	})

	t.Run("marking the chat unread leaves the tombstone read", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		first := fx.createAs(t, bob, givenSimpleMessage("first"))
		fx.createAs(t, bob, givenSimpleMessage("second"))
		require.NoError(t, fx.readAll(ctx))
		fx.pushAs(t, bob, deleteBuilder(first))

		// when
		require.NoError(t, fx.MarkMessagesAsUnread(ctx, "", chatmodel.CounterTypeMessage))

		// then
		assert.True(t, fx.getMessage(t, first).Read)
		assert.Equal(t, int32(1), fx.subscription.GetChatState().Messages.Counter)
	})

	t.Run("no flushed state counts a message deleted in the batch that created it", func(t *testing.T) {
		// Seen heads can arrive before the changes they name: a read callback then flushes
		// right after the commit, before the post-commit reload. The deleted message must
		// already be out of the counters by then.
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		subscribeForEvents(t, fx)
		tx, err := fx.store.NewTx(ctx)
		require.NoError(t, err)
		// A mentions me, so it is both an unread message and an unread mention
		createA, createB := &storestate.Builder{}, &storestate.Builder{}
		for b, msg := range map[*storestate.Builder]*chatmodel.Message{
			createA: givenMessageWithMention("@me"),
			createB: givenSimpleMessage("hello"),
		} {
			arena := &anyenc.Arena{}
			obj := arena.NewObject()
			msg.MarshalAnyenc(obj, arena)
			require.NoError(t, b.Create(CollectionName, storestate.IdFromChange, obj))
		}
		idA, idB := bson.NewObjectId().Hex(), bson.NewObjectId().Hex()
		for _, cs := range []storestate.ChangeSet{
			{Id: idA, Order: fx.generateOrderId(tx), Creator: bob, Changes: createA.ChangeSet},
			{Id: idB, Order: fx.generateOrderId(tx), Creator: bob, Changes: createB.ChangeSet},
			{Id: bson.NewObjectId().Hex(), Order: fx.generateOrderId(tx), Creator: bob, Changes: deleteBuilder(idA).ChangeSet},
		} {
			require.NoError(t, tx.ApplyChangeSetReturnAllErrors(cs))
		}
		require.NoError(t, tx.Commit())
		fx.events = nil

		// when
		require.NoError(t, fx.markReadMessages([]string{idB}, chatmodel.CounterTypeMessage))
		fx.onUpdate()

		// then
		var states int
		for _, ev := range fx.events {
			if st := ev.GetChatStateUpdate(); st != nil {
				states++
				assert.Equal(t, int32(0), st.State.Messages.Counter, "A is deleted and B is read")
				assert.Equal(t, int32(0), st.State.Mentions.Counter, "A, the mention, is deleted")
			}
			if count := ev.GetChatUpdateMessageCount(); count != nil {
				assert.Equal(t, int32(1), count.MessageCount)
			}
		}
		assert.NotZero(t, states)
	})
}

func TestDiscussionDeleteEvents(t *testing.T) {
	t.Run("the tombstone is sent as an update", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		subscribeForEvents(t, fx)
		id := fx.createAs(t, bob, givenSimpleMessage("hello"))
		fx.events = nil

		// when
		fx.pushAs(t, bob, deleteBuilder(id))

		// then
		var updated *model.ChatMessage
		for _, ev := range fx.events {
			assert.Nil(t, ev.GetChatDelete())
			if upd := ev.GetChatUpdate(); upd != nil {
				updated = upd.Message
			}
		}
		require.NotNil(t, updated)
		assert.True(t, updated.Deleted)
		assert.Empty(t, updated.Message.Text)
	})

	t.Run("observers see an update carrying the tombstone", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		id := fx.createAs(t, bob, givenSimpleMessage("hello"))
		var changes []chatsubscription.ChatChange
		fx.subscription.Lock()
		fx.subscription.AddObserver("test", func(change chatsubscription.ChatChange) {
			if change.Kind != chatsubscription.ChatChangeStateUpdated {
				changes = append(changes, change)
			}
		})
		fx.subscription.Unlock()

		// when
		fx.pushAs(t, bob, deleteBuilder(id))

		// then
		require.Len(t, changes, 1)
		assert.Equal(t, chatsubscription.ChatChangeMessageUpdated, changes[0].Kind)
		assert.True(t, changes[0].Message.Deleted)
	})

	t.Run("a create and its delete in one batch reach the window as one add carrying the tombstone", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t, asDiscussion("parentId"))
		subscribeForEvents(t, fx)
		var kinds []chatsubscription.ChatChangeKind
		fx.subscription.Lock()
		fx.subscription.AddObserver("test", func(change chatsubscription.ChatChange) {
			if change.Kind != chatsubscription.ChatChangeStateUpdated {
				kinds = append(kinds, change.Kind)
			}
		})
		fx.subscription.Unlock()
		fx.events = nil
		create := &storestate.Builder{}
		arena := &anyenc.Arena{}
		obj := arena.NewObject()
		givenSimpleMessage("hello").MarshalAnyenc(obj, arena)
		require.NoError(t, create.Create(CollectionName, storestate.IdFromChange, obj))
		id := bson.NewObjectId().Hex()

		// when
		tx, err := fx.store.NewTx(ctx)
		require.NoError(t, err)
		for _, cs := range []storestate.ChangeSet{
			{Id: id, Order: fx.generateOrderId(tx), Creator: bob, Changes: create.ChangeSet},
			{Id: bson.NewObjectId().Hex(), Order: fx.generateOrderId(tx), Creator: bob, Changes: deleteBuilder(id).ChangeSet},
		} {
			require.NoError(t, tx.ApplyChangeSetReturnAllErrors(cs))
		}
		require.NoError(t, tx.Commit())
		fx.onUpdate()

		// then
		var added []*model.ChatMessage
		for _, ev := range fx.events {
			assert.Nil(t, ev.GetChatUpdate(), "the add already carries the tombstone")
			if add := ev.GetChatAdd(); add != nil {
				added = append(added, add.Message)
			}
		}
		require.Len(t, added, 1)
		assert.True(t, added[0].Deleted)
		assert.Equal(t, []chatsubscription.ChatChangeKind{chatsubscription.ChatChangeMessageAdded, chatsubscription.ChatChangeMessageUpdated}, kinds)
	})

	t.Run("a delete outside the window still sends the tombstone", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		older := fx.createAs(t, bob, givenSimpleMessage("older"))
		fx.createAs(t, bob, givenSimpleMessage("newer"))
		req := subscribeRequest(fx)
		req.Limit = 1
		_, err := fx.chatSubscriptionService.SubscribeLastMessages(context.Background(), req)
		require.NoError(t, err)
		fx.events = nil

		// when
		fx.pushAs(t, bob, deleteBuilder(older))

		// then
		var updated []*model.ChatMessage
		for _, ev := range fx.events {
			if upd := ev.GetChatUpdate(); upd != nil {
				updated = append(updated, upd.Message)
			}
		}
		require.Len(t, updated, 1)
		assert.Equal(t, older, updated[0].Id)
		assert.True(t, updated[0].Deleted)
	})

	t.Run("a space chat delete still sends a delete", func(t *testing.T) {
		// given
		fx := newFixture(t)
		subscribeForEvents(t, fx)
		id := fx.createAs(t, bob, givenSimpleMessage("hello"))
		fx.events = nil

		// when
		fx.pushAs(t, bob, deleteBuilder(id))

		// then
		var deleted []string
		for _, ev := range fx.events {
			if del := ev.GetChatDelete(); del != nil {
				deleted = append(deleted, del.Id)
			}
		}
		assert.Equal(t, []string{id}, deleted)
	})
}

func TestSubscriptionsHoldTombstones(t *testing.T) {
	ctx := context.Background()

	t.Run("a new subscription gets the deleted post and its reply", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		post := fx.createAs(t, alice, givenSimpleMessage("post"))
		reply := fx.createAs(t, bob, &chatmodel.Message{ChatMessage: &model.ChatMessage{
			ReplyToMessageId: post,
			Message:          &model.ChatMessageMessageContent{Text: "reply"},
		}})
		fx.pushAs(t, alice, deleteBuilder(post))

		// when
		resp, err := fx.chatSubscriptionService.SubscribeLastMessages(ctx, subscribeRequest(fx))

		// then
		require.NoError(t, err)
		require.Len(t, resp.Messages, 2)
		assert.Equal(t, post, resp.Messages[0].Id)
		assert.True(t, resp.Messages[0].Deleted)
		assert.Equal(t, reply, resp.Messages[1].Id)
		assert.Equal(t, post, resp.Messages[1].ReplyToMessageId)
	})

	t.Run("a new message is placed after the newest tombstone", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		deleted := fx.createAs(t, alice, givenSimpleMessage("first"))
		fx.pushAs(t, alice, deleteBuilder(deleted))
		subscribeForEvents(t, fx)
		fx.events = nil

		// when
		added := fx.createAs(t, bob, givenSimpleMessage("second"))

		// then
		var afterOrderId string
		for _, ev := range fx.events {
			if add := ev.GetChatAdd(); add != nil && add.Id == added {
				afterOrderId = add.AfterOrderId
			}
		}
		assert.Equal(t, fx.getMessage(t, deleted).OrderId, afterOrderId)
	})
}

func TestGuardsRefuseDeletedAndMissingTargets(t *testing.T) {
	ctx := context.Background()
	// given a discussion with a live message of mine and a deleted one
	newFx := func(t *testing.T) (fx *fixture, live, deleted string) {
		fx = newFixture(t, asDiscussion("parentId"))
		var err error
		live, err = fx.AddMessage(ctx, nil, givenSimpleMessage("live"))
		require.NoError(t, err)
		deleted, err = fx.AddMessage(ctx, nil, givenSimpleMessage("deleted"))
		require.NoError(t, err)
		require.NoError(t, fx.DeleteMessage(ctx, deleted))
		return fx, live, deleted
	}
	const missing = "missingMessageId"

	targets := map[string]struct {
		id      func(live, deleted string) string
		wantErr error
	}{
		"a deleted message": {id: func(_, deleted string) string { return deleted }, wantErr: ErrMessageDeleted},
		"a missing message": {id: func(string, string) string { return missing }, wantErr: ErrMessageNotFound},
	}
	for name, target := range targets {
		t.Run("edit of "+name, func(t *testing.T) {
			fx, live, deleted := newFx(t)
			pushed := len(fx.appliedChangeSets)

			err := fx.EditMessage(ctx, target.id(live, deleted), givenSimpleMessage("edited"))

			require.ErrorIs(t, err, target.wantErr)
			assert.Len(t, fx.appliedChangeSets, pushed, "nothing is pushed")
		})

		t.Run("reaction to "+name, func(t *testing.T) {
			fx, live, deleted := newFx(t)
			pushed := len(fx.appliedChangeSets)

			_, err := fx.ToggleMessageReaction(ctx, target.id(live, deleted), "👍")

			require.ErrorIs(t, err, target.wantErr)
			assert.Len(t, fx.appliedChangeSets, pushed, "nothing is pushed")
		})

		t.Run("pin of "+name, func(t *testing.T) {
			fx, live, deleted := newFx(t)
			pushed := len(fx.appliedChangeSets)

			err := fx.SetMessagesPinned(ctx, []string{target.id(live, deleted)}, true)

			require.ErrorIs(t, err, target.wantErr)
			assert.Len(t, fx.appliedChangeSets, pushed, "nothing is pushed")
		})

		t.Run("delete of "+name+" succeeds without a change", func(t *testing.T) {
			fx, live, deleted := newFx(t)
			pushed := len(fx.appliedChangeSets)

			err := fx.DeleteMessage(ctx, target.id(live, deleted))

			require.NoError(t, err)
			assert.Len(t, fx.appliedChangeSets, pushed, "nothing is pushed")
		})
	}

	t.Run("a pin batch with a deleted message pins nothing", func(t *testing.T) {
		for position := 0; position < 3; position++ {
			// given
			fx, live, deleted := newFx(t)
			other, err := fx.AddMessage(ctx, nil, givenSimpleMessage("other"))
			require.NoError(t, err)
			batch := []string{live, other}
			batch = append(batch[:position], append([]string{deleted}, batch[position:]...)...)
			pushed := len(fx.appliedChangeSets)

			// when
			err = fx.SetMessagesPinned(ctx, batch, true)

			// then
			require.ErrorIs(t, err, ErrMessageDeleted, "deleted message at position %d", position)
			assert.Len(t, fx.appliedChangeSets, pushed, "nothing is pushed (deleted message at position %d)", position)
			assert.False(t, fx.getMessage(t, live).Pinned)
			assert.False(t, fx.getMessage(t, other).Pinned)
		}
	})

	t.Run("a pin batch of live messages pins them all", func(t *testing.T) {
		// given
		fx, live, _ := newFx(t)
		other, err := fx.AddMessage(ctx, nil, givenSimpleMessage("other"))
		require.NoError(t, err)

		// when
		err = fx.SetMessagesPinned(ctx, []string{live, other}, true)

		// then
		require.NoError(t, err)
		assert.True(t, fx.getMessage(t, live).Pinned)
		assert.True(t, fx.getMessage(t, other).Pinned)
	})

	t.Run("a reply to a deleted message is refused", func(t *testing.T) {
		// given
		fx, _, deleted := newFx(t)
		pushed := len(fx.appliedChangeSets)
		reply := givenSimpleMessage("reply")
		reply.ReplyToMessageId = deleted

		// when
		_, err := fx.AddMessage(ctx, nil, reply)

		// then
		require.ErrorIs(t, err, ErrReplyToDeletedMessage)
		assert.Len(t, fx.appliedChangeSets, pushed, "nothing is pushed")
	})

	t.Run("a reply to a live or unknown message is accepted", func(t *testing.T) {
		for _, to := range []string{"live", missing} {
			// given
			fx, live, _ := newFx(t)
			reply := givenSimpleMessage("reply")
			reply.ReplyToMessageId = map[string]string{"live": live, missing: missing}[to]

			// when
			_, err := fx.AddMessage(ctx, nil, reply)

			// then
			require.NoError(t, err, to)
		}
	})

	t.Run("edit, reaction and pin of a live message still work", func(t *testing.T) {
		// given
		fx, live, _ := newFx(t)

		// when
		require.NoError(t, fx.EditMessage(ctx, live, givenSimpleMessage("edited")))
		_, err := fx.ToggleMessageReaction(ctx, live, "👍")
		require.NoError(t, err)
		require.NoError(t, fx.SetMessagesPinned(ctx, []string{live}, true))

		// then
		got := fx.getMessage(t, live)
		assert.Equal(t, "edited", got.Message.Text)
		assert.Contains(t, got.Reactions.Reactions, "👍")
		assert.True(t, got.Pinned)
	})
}

// failingCollection refuses every update, as a full disk would.
type failingCollection struct {
	anystore.Collection
}

func (failingCollection) UpdateId(ctx context.Context, id any, mod query.Modifier) (anystore.ModifyResult, error) {
	return anystore.ModifyResult{}, errors.New("disk is full")
}

func TestTombstoneWriteFailureIsCritical(t *testing.T) {
	// given
	tombstone := givenSimpleMessage("hello")
	tombstone.MakeTombstone()

	// when
	err := writeTombstone(context.Background(), failingCollection{}, "msg1", tombstone)

	// then
	require.ErrorIs(t, err, storestate.ErrCritical, "a failed tombstone write must abort the apply, not be skipped")
}

var chatrepositoryGetAll = chatrepository.GetMessagesRequest{Limit: 1000}

func subscribeRequest(fx *fixture) chatsubscription.SubscribeLastMessagesRequest {
	return chatsubscription.SubscribeLastMessagesRequest{
		ChatObjectId: fx.Id(),
		SubId:        "sub",
		Limit:        100,
	}
}
