package v2service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// fakeSpaceChatSubscription is what the hub port hands back: a snapshot and
// a live queue. It counts closes, so a test can see the stream release it.
type fakeSpaceChatSubscription struct {
	snapshot []apicore.SpaceChatFrame
	ready    chan struct{}

	mu     sync.Mutex
	live   []apicore.SpaceChatFrame
	closes int
}

func newFakeSpaceChatSubscription(snapshot ...apicore.SpaceChatFrame) *fakeSpaceChatSubscription {
	return &fakeSpaceChatSubscription{snapshot: snapshot, ready: make(chan struct{}, 1)}
}

func (f *fakeSpaceChatSubscription) Snapshot() []apicore.SpaceChatFrame { return f.snapshot }
func (f *fakeSpaceChatSubscription) Ready() <-chan struct{}             { return f.ready }

func (f *fakeSpaceChatSubscription) Drain() []apicore.SpaceChatFrame {
	f.mu.Lock()
	defer f.mu.Unlock()
	live := f.live
	f.live = nil
	return live
}

func (f *fakeSpaceChatSubscription) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
}

func (f *fakeSpaceChatSubscription) Push(frames ...apicore.SpaceChatFrame) {
	f.mu.Lock()
	f.live = append(f.live, frames...)
	f.mu.Unlock()
	select {
	case f.ready <- struct{}{}:
	default:
	}
}

func (f *fakeSpaceChatSubscription) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closes
}

func TestOpenSpaceChatStream(t *testing.T) {
	ctx := context.Background()

	t.Run("a space outside the key's grant is refused before anything opens", func(t *testing.T) {
		// given: no hub expectation, so reaching the port fails the test
		fx := newV2Fixture(t)
		fx.withChatSub(mock_apicore.NewMockChatSubscriptionService(t))
		granted := util.CtxWithApiGrant(ctx, &util.ApiGrant{Spaces: []string{"other-space"}, Perms: util.GrantPermsRead})

		// when
		_, err := fx.OpenSpaceChatStream(granted, testSpaceId, SpaceChatStreamQuery{IncludeDiscussions: true})

		// then
		requireV2Code(t, err, v2model.CodeSpaceNotGranted)
	})

	t.Run("an unknown space is a 404 before anything opens", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.withChatSub(mock_apicore.NewMockChatSubscriptionService(t))

		// when
		_, err := fx.OpenSpaceChatStream(ctx, "no-such-space", SpaceChatStreamQuery{})

		// then
		requireV2Code(t, err, v2model.CodeNotFound)
	})

	t.Run("the open reaches the hub with the space, the include predicate and a renderer", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		subMock := mock_apicore.NewMockChatSubscriptionService(t)
		var got apicore.SpaceChatOpen
		hubSub := newFakeSpaceChatSubscription(apicore.SpaceChatFrame{Type: v2model.SpaceChatEventSnapshotComplete})
		subMock.EXPECT().OpenSpaceChats(mock.Anything, mock.Anything).RunAndReturn(
			func(_ context.Context, req apicore.SpaceChatOpen) (apicore.SpaceChatSubscription, error) {
				got = req
				return hubSub, nil
			})
		fx.withChatSub(subMock)

		// when
		stream, err := fx.OpenSpaceChatStream(ctx, testSpaceId, SpaceChatStreamQuery{IncludeDiscussions: true})

		// then
		require.NoError(t, err)
		defer stream.Close()
		assert.Equal(t, testSpaceId, got.SpaceId)
		assert.True(t, got.IncludeDiscussions)
		assert.NotNil(t, got.Render)
		assert.Equal(t, hubSub.snapshot, stream.Snapshot())
	})

	t.Run("a chat that cannot attach fails the open with a 500 naming the chat", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		subMock := mock_apicore.NewMockChatSubscriptionService(t)
		subMock.EXPECT().OpenSpaceChats(mock.Anything, mock.Anything).Return(nil,
			fmt.Errorf("open space chats: %w", &apicore.SpaceChatAttachError{ChatId: "chat7", Err: errors.New("init chat state: corrupted")}))
		fx.withChatSub(subMock)

		// when
		_, err := fx.OpenSpaceChatStream(ctx, testSpaceId, SpaceChatStreamQuery{})

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusInternalServerError, apiErr.Status)
		assert.Equal(t, v2model.CodeInternalError, apiErr.Code)
		assert.Contains(t, apiErr.Message, "chat7")
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].Message, "corrupted")
	})

	t.Run("the open-stream cap refuses with 429, independently of the per-chat cap", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addChat(t, "chat1", "General", 1)
		subMock := mock_apicore.NewMockChatSubscriptionService(t)
		subMock.EXPECT().OpenSpaceChats(mock.Anything, mock.Anything).RunAndReturn(
			func(context.Context, apicore.SpaceChatOpen) (apicore.SpaceChatSubscription, error) {
				return newFakeSpaceChatSubscription(), nil
			})
		subMock.EXPECT().SubscribeLastMessages(mock.Anything, "chat1", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
		subMock.EXPECT().Unsubscribe("chat1", mock.Anything).Return(nil)
		fx.withChatSub(subMock)
		open := make([]*SpaceChatStream, 0, maxConcurrentSpaceChatStreams)
		for i := 0; i < maxConcurrentSpaceChatStreams; i++ {
			stream, err := fx.OpenSpaceChatStream(ctx, testSpaceId, SpaceChatStreamQuery{})
			require.NoError(t, err, "stream %d", i)
			open = append(open, stream)
		}

		// when
		_, err := fx.OpenSpaceChatStream(ctx, testSpaceId, SpaceChatStreamQuery{})

		// then
		apiErr := v2Err(t, err)
		assert.Equal(t, http.StatusTooManyRequests, apiErr.Status)
		assert.Equal(t, v2model.CodeTooManyStreams, apiErr.Code)
		assert.Contains(t, apiErr.Message, "space chat streams")
		chatStream, err := fx.OpenChatStream(ctx, testSpaceId, "chat1", ChatStreamQuery{})
		require.NoError(t, err, "the per-chat cap is separate")
		chatStream.Close()

		// and a closed stream returns its slot
		open[0].Close()
		reopened, err := fx.OpenSpaceChatStream(ctx, testSpaceId, SpaceChatStreamQuery{})
		require.NoError(t, err)
		reopened.Close()
		for _, stream := range open[1:] {
			stream.Close()
		}
	})

	t.Run("Close leaves the hub exactly once", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		subMock := mock_apicore.NewMockChatSubscriptionService(t)
		hubSub := newFakeSpaceChatSubscription()
		subMock.EXPECT().OpenSpaceChats(mock.Anything, mock.Anything).Return(hubSub, nil)
		fx.withChatSub(subMock)
		stream, err := fx.OpenSpaceChatStream(ctx, testSpaceId, SpaceChatStreamQuery{})
		require.NoError(t, err)

		// when
		stream.Close()
		stream.Close()

		// then
		assert.Equal(t, 1, hubSub.closeCount())
	})
}

// The renderer is the whole wire contract of the space stream: one envelope
// per event, space_id on every one, the members the event table names and no
// others.
func TestSpaceChatRenderer(t *testing.T) {
	discussion := apicore.SpaceChat{Id: "disc1", Name: "Plan", Discussion: true, ParentId: "page1"}
	chat := apicore.SpaceChat{Id: "chat1", Name: "General"}
	state := &model.ChatState{
		Messages:    &model.ChatStateUnreadState{Counter: 2},
		Mentions:    &model.ChatStateUnreadState{Counter: 1},
		LastStateId: "s9",
	}
	pinned := false

	render := func(t *testing.T, fx *v2Fixture, change apicore.SpaceChatChange) (string, v2model.SpaceChatEvent) {
		t.Helper()
		change.SpaceId = testSpaceId
		frame, err := fx.spaceChatRenderer(testSpaceId)(change)
		require.NoError(t, err)
		var got v2model.SpaceChatEvent
		require.NoError(t, json.Unmarshal(frame.Data, &got))
		assert.Equal(t, frame.Type, got.Type, "the SSE event name is the body's type")
		return string(frame.Data), got
	}

	for _, tc := range []struct {
		name   string
		change apicore.SpaceChatChange
		want   v2model.SpaceChatEvent
	}{
		{
			name:   "chat_added carries the list row plus last_state_id",
			change: apicore.SpaceChatChange{Type: apicore.SpaceChatAdded, Chat: discussion, State: state},
			want: v2model.SpaceChatEvent{Type: "chat_added", SpaceId: testSpaceId, Chat: &v2model.StreamChatRow{
				Id: "disc1", Name: "Plan", Kind: "discussion", ParentId: "page1", UnreadMessages: 2, UnreadMentions: 1, LastStateId: "s9",
			}},
		},
		{
			name:   "snapshot_complete carries nothing else",
			change: apicore.SpaceChatChange{Type: apicore.SpaceChatSnapshotComplete},
			want:   v2model.SpaceChatEvent{Type: "snapshot_complete", SpaceId: testSpaceId},
		},
		{
			name:   "chat_updated carries the row",
			change: apicore.SpaceChatChange{Type: apicore.SpaceChatUpdated, Chat: chat, State: state},
			want: v2model.SpaceChatEvent{Type: "chat_updated", SpaceId: testSpaceId, Chat: &v2model.StreamChatRow{
				Id: "chat1", Name: "General", Kind: "chat", UnreadMessages: 2, UnreadMentions: 1, LastStateId: "s9",
			}},
		},
		{
			name:   "chat_removed names the chat",
			change: apicore.SpaceChatChange{Type: apicore.SpaceChatRemoved, Chat: discussion},
			want:   v2model.SpaceChatEvent{Type: "chat_removed", SpaceId: testSpaceId, ChatId: "disc1"},
		},
		{
			name:   "state_updated carries the counters",
			change: apicore.SpaceChatChange{Type: apicore.SpaceChatStateUpdated, Chat: chat, State: state},
			want: v2model.SpaceChatEvent{Type: "state_updated", SpaceId: testSpaceId, ChatId: "chat1",
				State: &v2model.ChatCounters{UnreadMessages: 2, UnreadMentions: 1, LastStateId: "s9"}},
		},
		{
			name:   "message_deleted names the message, the chat's kind and its parent",
			change: apicore.SpaceChatChange{Type: apicore.SpaceChatMessageDeleted, Chat: discussion, MessageId: "m1"},
			want: v2model.SpaceChatEvent{Type: "message_deleted", SpaceId: testSpaceId, ChatId: "disc1",
				Kind: "discussion", ParentId: "page1", MessageId: "m1"},
		},
		{
			name: "reactions_updated carries counts",
			change: apicore.SpaceChatChange{Type: apicore.SpaceChatReactionsUpdated, Chat: chat, MessageId: "m1",
				Message: &model.ChatMessage{Id: "m1", Reactions: &model.ChatMessageReactions{Reactions: map[string]*model.ChatMessageReactionsIdentityList{
					"👍": {Ids: []string{"a", "b"}},
				}}}},
			want: v2model.SpaceChatEvent{Type: "reactions_updated", SpaceId: testSpaceId, ChatId: "chat1",
				MessageId: "m1", Reactions: map[string]int{"👍": 2}},
		},
		{
			name: "pinned_updated carries the flag, false included",
			change: apicore.SpaceChatChange{Type: apicore.SpaceChatPinnedUpdated, Chat: chat, MessageId: "m1",
				Message: &model.ChatMessage{Id: "m1", Pinned: false}},
			want: v2model.SpaceChatEvent{Type: "pinned_updated", SpaceId: testSpaceId, ChatId: "chat1",
				MessageId: "m1", Pinned: &pinned},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			fx := newV2Fixture(t)

			// when
			_, got := render(t, fx, tc.change)

			// then
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("message_added carries the full message with its state id", func(t *testing.T) {
		// given
		fx := newV2Fixture(t)
		fx.addParticipant(t, testIdentity, "Alice")
		change := apicore.SpaceChatChange{Type: apicore.SpaceChatMessageAdded, Chat: discussion, MessageId: "m1",
			Message: &model.ChatMessage{Id: "m1", OrderId: "o1", StateId: "s5", Creator: testIdentity,
				Message: &model.ChatMessageMessageContent{Text: "hello"}}}
		want := v2model.SpaceChatEvent{Type: "message_added", SpaceId: testSpaceId, ChatId: "disc1",
			Kind: "discussion", ParentId: "page1", Message: &v2model.ChatMessage{
				Id: "m1", StateId: "s5", Order: "o1", Text: "hello", Author: "Alice",
				AuthorId: domain.NewParticipantId(testSpaceId, testIdentity),
			}}

		// when
		raw, got := render(t, fx, change)

		// then
		assert.Equal(t, want, got)
		assert.Contains(t, raw, `"state_id":"s5"`)
	})

	t.Run("an author unknown at first resolves once the participant appears", func(t *testing.T) {
		// given: the renderer lives as long as the space's hub, so an empty
		// lookup must not be remembered
		fx := newV2Fixture(t)
		renderer := fx.spaceChatRenderer(testSpaceId)
		change := apicore.SpaceChatChange{Type: apicore.SpaceChatMessageAdded, SpaceId: testSpaceId, Chat: chat, MessageId: "m1",
			Message: &model.ChatMessage{Id: "m1", Creator: "identityB", Message: &model.ChatMessageMessageContent{Text: "hi"}}}
		authorOf := func() string {
			frame, err := renderer(change)
			require.NoError(t, err)
			var got v2model.SpaceChatEvent
			require.NoError(t, json.Unmarshal(frame.Data, &got))
			return got.Message.Author
		}
		require.Empty(t, authorOf())

		// when
		fx.addParticipant(t, "identityB", "Bob")

		// then
		assert.Equal(t, "Bob", authorOf())
	})
}
