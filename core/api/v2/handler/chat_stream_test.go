package v2handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// addStreamChat registers a resolvable chat and a subscription whose sink is
// published on the returned channel once the handler subscribes, so a test
// can push live events into a stream it has already opened.
func addStreamChat(t *testing.T, fx *v2HandlerFixture, window []*model.ChatMessage) <-chan chan<- *pb.Event {
	t.Helper()
	fx.store.AddObjects(t, "space1", []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String("chat1"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_chatDerived)),
	}})
	sinkCh := make(chan chan<- *pb.Event, 1)
	subMock := mock_apicore.NewMockChatSubscriptionService(t)
	subMock.EXPECT().SubscribeLastMessages(mock.Anything, "chat1", mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, _ int, _ string, sink chan<- *pb.Event) ([]*model.ChatMessage, error) {
			sinkCh <- sink
			return window, nil
		})
	subMock.EXPECT().Unsubscribe("chat1", mock.Anything).Return(nil).Maybe()
	fx.svc.SetChatSubscription(subMock)
	fx.router.GET("/v2/spaces/:space_id/chats/:chat_id/messages/stream", ChatStreamHandler(fx.svc))
	return sinkCh
}

// runStream drives the handler, lets the caller feed the live sink, then
// hangs up the way a client would. The handler blocks on its event loop, so
// the disconnect is what ends it.
func runStream(t *testing.T, fx *v2HandlerFixture, req *http.Request,
	sinkCh <-chan chan<- *pb.Event, feed func(sink chan<- *pb.Event)) *httptest.ResponseRecorder {
	t.Helper()
	ctx, cancel := context.WithCancel(req.Context())
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); fx.router.ServeHTTP(w, req.WithContext(ctx)) }()

	select {
	case sink := <-sinkCh:
		if feed != nil {
			feed(sink)
		}
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("the stream never subscribed")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the stream did not close when the client hung up")
	}
	return w
}

func streamRequest(target string) *http.Request {
	return httptest.NewRequest(http.MethodGet, target, nil)
}

func TestChatStreamHandler(t *testing.T) {
	const route = "/v2/spaces/space1/chats/chat1/messages/stream"

	t.Run("a chat outside the space refuses in the C6 envelope, not a stream", func(t *testing.T) {
		// the refusal precedes the first byte of the stream, so it can still
		// carry a status and a body — once the stream opens neither can move
		fx := newV2HandlerFixture(t)
		fx.svc.SetChatSubscription(mock_apicore.NewMockChatSubscriptionService(t))
		fx.router.GET("/v2/spaces/:space_id/chats/:chat_id/messages/stream", ChatStreamHandler(fx.svc))

		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, streamRequest("/v2/spaces/space1/chats/nope/messages/stream"))

		require.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
		assert.Contains(t, w.Body.String(), v2model.CodeNotFound)
	})

	t.Run("the opening window streams as message_added carrying its state id", func(t *testing.T) {
		fx := newV2HandlerFixture(t)
		sinkCh := addStreamChat(t, fx, []*model.ChatMessage{{
			Id: "m1", OrderId: "o1", StateId: "s1",
			Message: &model.ChatMessageMessageContent{Text: "hello"},
		}})

		w := runStream(t, fx, streamRequest(route), sinkCh, nil)

		body := w.Body.String()
		assert.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
		assert.Contains(t, body, "event: message_added")
		assert.Contains(t, body, "id: s1")
		assert.Contains(t, body, `"text":"hello"`)
	})

	t.Run("live events reach the client", func(t *testing.T) {
		fx := newV2HandlerFixture(t)
		sinkCh := addStreamChat(t, fx, nil)

		w := runStream(t, fx, streamRequest(route), sinkCh, func(sink chan<- *pb.Event) {
			sink <- &pb.Event{Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfChatDelete{ChatDelete: &pb.EventChatDelete{Id: "m9"}},
			}}}
			time.Sleep(50 * time.Millisecond) // let the loop drain before the hangup
		})

		body := w.Body.String()
		assert.Contains(t, body, "event: message_deleted")
		assert.Contains(t, body, `"message_id":"m9"`)
	})

	t.Run("a Last-Event-ID the window cannot cover emits resync_required first", func(t *testing.T) {
		fx := newV2HandlerFixture(t)
		sinkCh := addStreamChat(t, fx, []*model.ChatMessage{{Id: "m1", StateId: "s5"}})
		req := streamRequest(route)
		req.Header.Set("Last-Event-ID", "s0")

		w := runStream(t, fx, req, sinkCh, nil)

		body := w.Body.String()
		require.Contains(t, body, "event: resync_required")
		require.Contains(t, body, "event: message_added")
		assert.Less(t, strings.Index(body, "resync_required"), strings.Index(body, "message_added"),
			"the client must learn the window is unverified before it reads the window")
	})

	t.Run("a Last-Event-ID inside the window replays nothing already seen", func(t *testing.T) {
		fx := newV2HandlerFixture(t)
		sinkCh := addStreamChat(t, fx, []*model.ChatMessage{
			{Id: "m1", StateId: "s1"},
			{Id: "m2", StateId: "s2"},
		})
		req := streamRequest(route)
		req.Header.Set("Last-Event-ID", "s1")

		w := runStream(t, fx, req, sinkCh, nil)

		body := w.Body.String()
		assert.NotContains(t, body, "resync_required")
		assert.Contains(t, body, `"id":"m2"`)
		assert.NotContains(t, body, `"id":"m1"`, "the client already has it")
	})
}
