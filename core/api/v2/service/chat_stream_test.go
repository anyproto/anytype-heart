package v2service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// replayFromState decides what a reconnecting stream owes its client. The
// subscription hands back a sliding window of the newest messages; the
// client's Last-Event-ID is a chat state id, which every message carries and
// which sorts. Anything newer than that id is what the client missed.
//
// The case that matters is the one it CANNOT serve: an id older than the
// oldest row in the window means messages moved through the chat while the
// client was away and are no longer in the window to replay. The stream says
// so instead of sending a partial window that looks complete.
func TestReplayFromState(t *testing.T) {
	window := []*model.ChatMessage{
		{Id: "m1", StateId: "s1"},
		{Id: "m2", StateId: "s2"},
		{Id: "m3", StateId: "s3"},
	}
	ids := func(msgs []*model.ChatMessage) []string {
		out := make([]string, 0, len(msgs))
		for _, m := range msgs {
			out = append(out, m.Id)
		}
		return out
	}

	t.Run("a fresh connection replays the whole window", func(t *testing.T) {
		replay, resync := replayFromState(window, "", 3)

		assert.Equal(t, []string{"m1", "m2", "m3"}, ids(replay))
		assert.False(t, resync, "a client with no history has nothing to reconcile")
	})

	t.Run("a resume replays only what the client has not seen", func(t *testing.T) {
		replay, resync := replayFromState(window, "s2", 3)

		assert.Equal(t, []string{"m3"}, ids(replay))
		assert.False(t, resync)
	})

	t.Run("a resume from the newest state replays nothing", func(t *testing.T) {
		replay, resync := replayFromState(window, "s3", 3)

		assert.Empty(t, replay)
		assert.False(t, resync)
	})

	t.Run("a short window proves continuity by being the whole chat", func(t *testing.T) {
		// fewer rows than asked for means nothing was evicted, so there is
		// nothing between the cursor and the oldest row to have missed.
		// Reporting resync here fired on every reconnect to a small chat.
		replay, resync := replayFromState(window, "s0", 50)

		assert.Equal(t, []string{"m1", "m2", "m3"}, ids(replay))
		assert.False(t, resync)
	})

	t.Run("a full window whose rows are all newer cannot prove continuity", func(t *testing.T) {
		// the window is full AND every row postdates the cursor, so rows may
		// have been evicted between them: the client is told to reconcile
		// rather than handed a window that looks like a complete catch-up
		replay, resync := replayFromState(window, "s0", 3)

		assert.Equal(t, []string{"m1", "m2", "m3"}, ids(replay))
		assert.True(t, resync)
	})

	t.Run("an empty window cannot prove continuity for a resuming client", func(t *testing.T) {
		replay, resync := replayFromState(nil, "s1", 3)

		assert.Empty(t, replay)
		assert.True(t, resync, "the chat holds nothing; the client's copy cannot be confirmed")
	})

	t.Run("an empty window is not a resync for a fresh connection", func(t *testing.T) {
		replay, resync := replayFromState(nil, "", 3)

		assert.Empty(t, replay)
		assert.False(t, resync)
	})

	t.Run("a message with no state id never suppresses itself", func(t *testing.T) {
		// legacy rows predate stateId; they cannot be positioned against the
		// cursor, so they replay rather than vanish
		legacy := []*model.ChatMessage{{Id: "old", StateId: ""}, {Id: "m2", StateId: "s2"}}

		replay, resync := replayFromState(legacy, "s1", 3)

		assert.Equal(t, []string{"old", "m2"}, ids(replay))
		assert.True(t, resync, "an unpositionable row means the window cannot be proven contiguous")
	})
}

// withChatSub installs the stream's dependency on an already-built fixture.
// Production passes it through V2Deps at construction; a test only needs the
// one field, and setting it in-package keeps the fixture from growing a
// parameter every other test would have to pass nil for.
func (fx *v2Fixture) withChatSub(sub apicore.ChatSubscriptionService) {
	fx.chatSub = sub
}

// The stream's reason for existing in v2 is that v1's never checked anything:
// its handler took space_id from the path, used it only to enrich author
// names, and subscribed to whatever chat_id it was handed. A scoped key could
// stream any chat whose id it knew, in any space.
func TestOpenChatStream(t *testing.T) {
	ctx := context.Background()

	t.Run("a chat outside the space is a 404 and never subscribes", func(t *testing.T) {
		// given: no subscription expectation — reaching it fails the test
		fx := newV2Fixture(t)
		subMock := mock_apicore.NewMockChatSubscriptionService(t)
		fx.withChatSub(subMock)

		// when
		_, err := fx.OpenChatStream(ctx, testSpaceId, "chat-from-another-space", ChatStreamQuery{})

		// then
		requireV2Code(t, err, v2model.CodeNotFound)
	})

	t.Run("an object that is not a chat is refused", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.withChatSub(mock_apicore.NewMockChatSubscriptionService(t))
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("page1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})

		_, err := fx.OpenChatStream(ctx, testSpaceId, "page1", ChatStreamQuery{})

		requireV2Code(t, err, v2model.CodeValidationFailed)
	})

	t.Run("the limit is clamped to the advertised bound", func(t *testing.T) {
		// v1 parsed limit with a bare Atoi and passed it through, so
		// ?limit=999999 reached the subscription untouched despite the
		// document promising 1..1000
		fx := newV2Fixture(t)
		fx.addChat(t, "chat1", "General", 1)
		subMock := mock_apicore.NewMockChatSubscriptionService(t)
		var gotLimit int
		subMock.EXPECT().SubscribeLastMessages(mock.Anything, "chat1", mock.Anything, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, _ string, limit int, _ string, _ chan<- *pb.Event) ([]*model.ChatMessage, error) {
				gotLimit = limit
				return nil, nil
			})
		subMock.EXPECT().Unsubscribe("chat1", mock.Anything).Return(nil).Maybe()
		fx.withChatSub(subMock)

		stream, err := fx.OpenChatStream(ctx, testSpaceId, "chat1", ChatStreamQuery{Limit: 999999})

		require.NoError(t, err)
		defer stream.Close()
		assert.Equal(t, maxChatStreamLimit, gotLimit)
	})

	t.Run("a zero limit takes the default", func(t *testing.T) {
		fx := newV2Fixture(t)
		fx.addChat(t, "chat1", "General", 1)
		subMock := mock_apicore.NewMockChatSubscriptionService(t)
		var gotLimit int
		subMock.EXPECT().SubscribeLastMessages(mock.Anything, "chat1", mock.Anything, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, _ string, limit int, _ string, _ chan<- *pb.Event) ([]*model.ChatMessage, error) {
				gotLimit = limit
				return nil, nil
			})
		subMock.EXPECT().Unsubscribe("chat1", mock.Anything).Return(nil).Maybe()
		fx.withChatSub(subMock)

		stream, err := fx.OpenChatStream(ctx, testSpaceId, "chat1", ChatStreamQuery{})

		require.NoError(t, err)
		defer stream.Close()
		assert.Equal(t, defaultChatStreamLimit, gotLimit)
	})

	t.Run("the opening window resolves author names", func(t *testing.T) {
		// the streamed message is the same shape a paginated read returns,
		// and that includes the display name behind the raw creator identity
		fx := newV2Fixture(t)
		fx.addChat(t, "chat1", "General", 1)
		fx.addParticipant(t, "identityA", "Alice")
		subMock := mock_apicore.NewMockChatSubscriptionService(t)
		subMock.EXPECT().SubscribeLastMessages(mock.Anything, "chat1", mock.Anything, mock.Anything, mock.Anything).
			Return([]*model.ChatMessage{{Id: "m1", StateId: "s1", Creator: "identityA"}}, nil)
		subMock.EXPECT().Unsubscribe("chat1", mock.Anything).Return(nil)
		fx.withChatSub(subMock)

		stream, err := fx.OpenChatStream(ctx, testSpaceId, "chat1", ChatStreamQuery{})

		require.NoError(t, err)
		defer stream.Close()
		require.Len(t, stream.Initial, 1)
		assert.Equal(t, "Alice", stream.Initial[0].Message.Author)
	})

	t.Run("the open-stream cap refuses rather than growing without bound", func(t *testing.T) {
		// each stream pins a cloned window and a sink; nothing else bounds
		// them, since the shared limiter is writes-only and keyed on a
		// loopback address
		fx := newV2Fixture(t)
		fx.addChat(t, "chat1", "General", 1)
		subMock := mock_apicore.NewMockChatSubscriptionService(t)
		subMock.EXPECT().SubscribeLastMessages(mock.Anything, "chat1", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, nil)
		subMock.EXPECT().Unsubscribe("chat1", mock.Anything).Return(nil).Maybe()
		fx.withChatSub(subMock)

		open := make([]*ChatStream, 0, maxConcurrentChatStreams)
		for i := 0; i < maxConcurrentChatStreams; i++ {
			stream, err := fx.OpenChatStream(ctx, testSpaceId, "chat1", ChatStreamQuery{})
			require.NoError(t, err, "stream %d", i)
			open = append(open, stream)
		}

		_, err := fx.OpenChatStream(ctx, testSpaceId, "chat1", ChatStreamQuery{})

		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusTooManyRequests, apiErr.Status)
		assert.Equal(t, v2model.CodeRateLimitExceeded, apiErr.Code)

		// and a closed stream returns its slot
		open[0].Close()
		reopened, err := fx.OpenChatStream(ctx, testSpaceId, "chat1", ChatStreamQuery{})
		require.NoError(t, err)
		reopened.Close()
		for _, stream := range open[1:] {
			stream.Close()
		}
	})

	t.Run("Close unsubscribes exactly once", func(t *testing.T) {
		// the stream owns the subscription's lifetime; a leaked subscription
		// keeps a sliding window alive for a client that is gone
		fx := newV2Fixture(t)
		fx.addChat(t, "chat1", "General", 1)
		subMock := mock_apicore.NewMockChatSubscriptionService(t)
		subMock.EXPECT().SubscribeLastMessages(mock.Anything, "chat1", mock.Anything, mock.Anything, mock.Anything).
			Return(nil, nil)
		unsubscribes := 0
		subMock.EXPECT().Unsubscribe("chat1", mock.Anything).RunAndReturn(func(string, string) error {
			unsubscribes++
			return nil
		})
		fx.withChatSub(subMock)

		stream, err := fx.OpenChatStream(ctx, testSpaceId, "chat1", ChatStreamQuery{})
		require.NoError(t, err)

		stream.Close()
		stream.Close()

		assert.Equal(t, 1, unsubscribes)
	})
}
