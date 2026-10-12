package chatobject

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-store/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
)

// pushAs applies the builder's changes as one change set authored by creator, the way a change
// from another member arrives.
func (fx *fixture) pushAs(t *testing.T, creator string, builder *storestate.Builder) string {
	t.Helper()
	prev := fx.sourceCreator
	fx.sourceCreator = creator
	defer func() { fx.sourceCreator = prev }()
	id, err := fx.applyToStore(context.Background(), source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   fx.store,
		Time:    time.Now(),
	})
	require.NoError(t, err)
	return id
}

func (fx *fixture) getMessage(t *testing.T, id string) *chatmodel.Message {
	t.Helper()
	msgs, err := fx.GetMessagesByIds(context.Background(), []string{id})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	return msgs[0]
}

func TestCreateNeverArrivesDeleted(t *testing.T) {
	t.Run("a create carrying deleted is stored as a live message", func(t *testing.T) {
		// given
		fx := newFixture(t, asDiscussion("parentId"))
		msg := givenSimpleMessage("hello")
		msg.Deleted = true
		arena := &anyenc.Arena{}
		obj := arena.NewObject()
		msg.MarshalAnyenc(obj, arena)
		builder := &storestate.Builder{}
		require.NoError(t, builder.Create(CollectionName, storestate.IdFromChange, obj))

		// when
		id := fx.pushAs(t, "alice", builder)

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
