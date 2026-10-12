package chatsubscription

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/pb"
)

func tombstoneOf(msg *chatmodel.Message) *chatmodel.Message {
	tombstone := msg.Clone()
	tombstone.MakeTombstone()
	return tombstone
}

// windowFixture opens a manager whose subscription window (limit 2) holds a and b.
func windowFixture(t *testing.T, a, b *chatmodel.Message) (*fixture, Manager) {
	t.Helper()
	ctx := context.Background()
	fx := newFixture(t)
	repo, err := fx.repo.Repository(testSpaceId, "chatId1")
	require.NoError(t, err)
	require.NoError(t, repo.AddTestMessage(ctx, a))
	require.NoError(t, repo.AddTestMessage(ctx, b))
	mngr, err := fx.GetManager(testSpaceId, "chatId1")
	require.NoError(t, err)
	_, err = fx.SubscribeLastMessages(ctx, SubscribeLastMessagesRequest{ChatObjectId: "chatId1", SubId: "sub", Limit: 2})
	require.NoError(t, err)
	return fx, mngr
}

func (fx *fixture) eventMessages() []*pb.EventMessage {
	fx.lock.Lock()
	defer fx.lock.Unlock()
	var out []*pb.EventMessage
	for _, ev := range fx.events {
		out = append(out, ev.Messages...)
	}
	return out
}

func TestEvictionKeepsPendingEvents(t *testing.T) {
	t.Run("a tombstone update survives the eviction of its message", func(t *testing.T) {
		// given
		a := givenSimpleMessage("a", "first", "o1")
		b := givenSimpleMessage("b", "second", "o2")
		fx, mngr := windowFixture(t, a, b)
		mngr.Lock()
		defer mngr.Unlock()

		// when: one batch deletes a, then adds c, which pushes a out of the full window
		mngr.SoftDelete(a, tombstoneOf(a))
		mngr.Add("o2", givenSimpleMessage("c", "third", "o3"))
		mngr.Flush(false)

		// then
		var updated, added []string
		for _, ev := range fx.eventMessages() {
			if upd := ev.GetChatUpdate(); upd != nil {
				updated = append(updated, upd.Id)
				assert.True(t, upd.Message.Deleted)
			}
			if add := ev.GetChatAdd(); add != nil {
				added = append(added, add.Id)
			}
		}
		assert.Equal(t, []string{"a"}, updated)
		assert.Equal(t, []string{"c"}, added)
	})

	t.Run("an edit survives the eviction of its message", func(t *testing.T) {
		// given
		a := givenSimpleMessage("a", "first", "o1")
		b := givenSimpleMessage("b", "second", "o2")
		fx, mngr := windowFixture(t, a, b)
		mngr.Lock()
		defer mngr.Unlock()
		edited := givenSimpleMessage("a", "edited", "o1")

		// when
		mngr.UpdateFull(edited)
		mngr.Add("o2", givenSimpleMessage("c", "third", "o3"))
		mngr.Flush(false)

		// then
		var updatedText []string
		for _, ev := range fx.eventMessages() {
			if upd := ev.GetChatUpdate(); upd != nil {
				updatedText = append(updatedText, upd.Message.Message.Text)
			}
		}
		assert.Equal(t, []string{"edited"}, updatedText)
	})

	t.Run("an unflushed add of an evicted message is dropped", func(t *testing.T) {
		// given
		a := givenSimpleMessage("a", "first", "o1")
		b := givenSimpleMessage("b", "second", "o3")
		fx, mngr := windowFixture(t, a, b)
		mngr.Lock()
		defer mngr.Unlock()

		// when: c lands between a and b and evicts a; d then evicts c before any flush
		mngr.Add("o1", givenSimpleMessage("c", "between", "o2"))
		mngr.Add("o3", givenSimpleMessage("d", "newest", "o4"))
		mngr.Flush(false)

		// then
		var added []string
		for _, ev := range fx.eventMessages() {
			if add := ev.GetChatAdd(); add != nil {
				added = append(added, add.Id)
			}
			if upd := ev.GetChatUpdate(); upd != nil {
				assert.NotEqual(t, "c", upd.Id)
			}
		}
		assert.Equal(t, []string{"d"}, added)
	})
}

func TestGetLastMessageSkipsTombstones(t *testing.T) {
	t.Run("in the subscription window", func(t *testing.T) {
		// given
		live := givenSimpleMessage("a", "live", "o1")
		deleted := tombstoneOf(givenSimpleMessage("b", "deleted", "o2"))
		_, mngr := windowFixture(t, live, deleted)
		mngr.Lock()
		defer mngr.Unlock()

		// when
		got, ok, err := mngr.GetLastMessage()

		// then
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "a", got.Id)
	})

	t.Run("in the repository fallback", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		repo, err := fx.repo.Repository(testSpaceId, "chatId1")
		require.NoError(t, err)
		require.NoError(t, repo.AddTestMessage(ctx, givenSimpleMessage("a", "live", "o1")))
		require.NoError(t, repo.AddTestMessage(ctx, tombstoneOf(givenSimpleMessage("b", "deleted", "o2"))))
		mngr, err := fx.GetManager(testSpaceId, "chatId1")
		require.NoError(t, err)
		mngr.Lock()
		defer mngr.Unlock()

		// when
		got, ok, err := mngr.GetLastMessage()

		// then
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "a", got.Id)
	})

	t.Run("when every message is deleted", func(t *testing.T) {
		// given
		_, mngr := windowFixture(t,
			tombstoneOf(givenSimpleMessage("a", "deleted", "o1")),
			tombstoneOf(givenSimpleMessage("b", "deleted", "o2")))
		mngr.Lock()
		defer mngr.Unlock()

		// when
		_, ok, err := mngr.GetLastMessage()

		// then
		require.NoError(t, err)
		assert.False(t, ok)
	})
}
