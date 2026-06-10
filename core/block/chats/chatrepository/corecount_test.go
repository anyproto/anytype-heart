package chatrepository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const coreMe = "me"

func addCoreMsg(t *testing.T, fx *fixture, id, orderId, creator string, hasMention bool) {
	t.Helper()
	msg := &chatmodel.Message{ChatMessage: &model.ChatMessage{
		Id:      id,
		OrderId: orderId,
		Creator: creator,
		Message: &model.ChatMessageMessageContent{Text: id},
	}}
	msg.HasMention = hasMention
	require.NoError(t, fx.repo.AddTestMessage(context.Background(), msg))
}

// CountCoreUnread implements the CORE decomposition's repository half:
// tail = counted live messages with _o.id > maxF (indexed), band = counted
// live messages among the walk's candidates. Counted = peer + counter filter.
// No read flags are consulted anywhere.
func TestCountCoreUnread(t *testing.T) {
	ctx := context.Background()

	t.Run("tail plus band, own excluded everywhere", func(t *testing.T) {
		fx := newFixture(t)
		addCoreMsg(t, fx, "p1", "o01", "alice", false) // below cut, in band (concurrent)
		addCoreMsg(t, fx, "p2", "o02", "bob", false)   // below cut, covered (not in band)
		addCoreMsg(t, fx, "p3", "o03", "carol", false) // past cut -> tail
		addCoreMsg(t, fx, "own", "o04", coreMe, false) // own -> never counted

		got, err := fx.repo.CountCoreUnread(ctx, chatmodel.CounterTypeMessage, "o02", []string{"p1"}, coreMe)
		require.NoError(t, err)
		assert.Equal(t, 2, got, "tail{p3} + band{p1}; own past the cut excluded")

		// an own message in the band candidates must not be counted either
		got, err = fx.repo.CountCoreUnread(ctx, chatmodel.CounterTypeMessage, "o04", []string{"p1", "own"}, coreMe)
		require.NoError(t, err)
		assert.Equal(t, 1, got, "band{p1}; own filtered, no tail past o04")
	})

	t.Run("band candidates that are not live messages are ignored", func(t *testing.T) {
		fx := newFixture(t)
		addCoreMsg(t, fx, "p1", "o01", "alice", false)
		// "sysChange" is a band candidate from the DAG walk that has no message
		// row (system/non-message change, or a deleted message) -> not counted.
		got, err := fx.repo.CountCoreUnread(ctx, chatmodel.CounterTypeMessage, "o05", []string{"p1", "sysChange"}, coreMe)
		require.NoError(t, err)
		assert.Equal(t, 1, got)
	})

	t.Run("mention counter only sees hasMention rows", func(t *testing.T) {
		fx := newFixture(t)
		addCoreMsg(t, fx, "m1", "o01", "alice", true)  // mention below cut, in band
		addCoreMsg(t, fx, "p1", "o02", "alice", false) // plain below cut, in band
		addCoreMsg(t, fx, "m2", "o03", "bob", true)    // mention in tail
		addCoreMsg(t, fx, "p2", "o04", "bob", false)   // plain in tail
		addCoreMsg(t, fx, "mo", "o05", coreMe, true)   // own mention -> excluded

		got, err := fx.repo.CountCoreUnread(ctx, chatmodel.CounterTypeMention, "o02", []string{"m1", "p1"}, coreMe)
		require.NoError(t, err)
		assert.Equal(t, 2, got, "mention tail{m2} + mention band{m1}; p1/p2/own-mention excluded")

		got, err = fx.repo.CountCoreUnread(ctx, chatmodel.CounterTypeMessage, "o02", []string{"m1", "p1"}, coreMe)
		require.NoError(t, err)
		assert.Equal(t, 4, got, "message counter sees mentions and plain alike")
	})

	t.Run("no resolved frontier counts every peer message", func(t *testing.T) {
		fx := newFixture(t)
		addCoreMsg(t, fx, "p1", "o01", "alice", false)
		addCoreMsg(t, fx, "p2", "o02", "bob", false)
		addCoreMsg(t, fx, "own", "o03", coreMe, false)

		// maxF == "": no cut -> everything counted is unread; band must be
		// ignored even if (incorrectly) provided.
		got, err := fx.repo.CountCoreUnread(ctx, chatmodel.CounterTypeMessage, "", []string{"p1"}, coreMe)
		require.NoError(t, err)
		assert.Equal(t, 2, got)
	})
}
