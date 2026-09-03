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
	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// newStreamFixture builds a fixture whose chat resolves and whose subscription
// publishes its sink on the returned channel once the handler subscribes, so a
// test can push live events into a stream it has already opened.
func newStreamFixture(t *testing.T, window []*model.ChatMessage) (*v2HandlerFixture, <-chan chan<- *pb.Event) {
	t.Helper()
	sinkCh := make(chan chan<- *pb.Event, 1)
	subMock := mock_apicore.NewMockChatSubscriptionService(t)
	subMock.EXPECT().SubscribeLastMessages(mock.Anything, "chat1", mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ string, _ int, _ string, sink chan<- *pb.Event) ([]*model.ChatMessage, error) {
			sinkCh <- sink
			return window, nil
		})
	// NOT .Maybe(): the handler owns the subscription's lifetime, and a
	// missing defer Close() is the leak the whole apparatus exists to stop.
	// Without this expectation that mutation survives every test.
	subMock.EXPECT().Unsubscribe("chat1", mock.Anything).Return(nil)

	fx := newV2HandlerFixtureWithChatSub(t, subMock)
	fx.store.AddObjects(t, "space1", []objectstore.TestObject{{
		bundle.RelationKeyId:             domain.String("chat1"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_chatDerived)),
	}})
	// the real group parses and bounds `limit` before the handler; without it
	// this fixture would test a route that does not exist
	fx.router.Use(pagination.New(pagination.Config{
		DefaultPage: 0, DefaultPageSize: 25, MinPageSize: 1, MaxPageSize: 1000,
	}))
	fx.router.GET("/v2/spaces/:space_id/chats/:chat_id/messages/stream", ChatStreamHandler(fx.svc))
	return fx, sinkCh
}

// sseFrames splits a body into SSE frames, asserting each is well formed:
// terminated by a blank line, and carrying no raw newline inside a value.
func sseFrames(t *testing.T, body string) []map[string]string {
	t.Helper()
	require.True(t, body == "" || strings.HasSuffix(body, "\n\n"),
		"an SSE body ends on a frame terminator, got %q", body)
	var frames []map[string]string
	for _, block := range strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n") {
		if block == "" {
			continue
		}
		frame := map[string]string{}
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, ":") {
				frame["comment"] = strings.TrimPrefix(line, ": ")
				continue
			}
			field, value, found := strings.Cut(line, ": ")
			require.True(t, found, "a frame line is `field: value`, got %q", line)
			frame[field] = value
		}
		frames = append(frames, frame)
	}
	return frames
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
		fx := newV2HandlerFixtureWithChatSub(t, mock_apicore.NewMockChatSubscriptionService(t))
		fx.router.GET("/v2/spaces/:space_id/chats/:chat_id/messages/stream", ChatStreamHandler(fx.svc))

		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, streamRequest("/v2/spaces/space1/chats/nope/messages/stream"))

		require.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
		assert.Contains(t, w.Body.String(), v2model.CodeNotFound)
	})

	t.Run("the opening window streams as message_added carrying its state id", func(t *testing.T) {
		fx, sinkCh := newStreamFixture(t, []*model.ChatMessage{{
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
		fx, sinkCh := newStreamFixture(t, nil)

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
		// a FULL window (one row at limit=1) every row of which postdates the
		// cursor: rows may have been evicted in between, so continuity cannot
		// be proven. A short window would prove it and must not resync.
		fx, sinkCh := newStreamFixture(t, []*model.ChatMessage{{Id: "m1", StateId: "s5"}})
		req := streamRequest(route + "?limit=1")
		req.Header.Set("Last-Event-ID", "s0")

		w := runStream(t, fx, req, sinkCh, nil)

		body := w.Body.String()
		require.Contains(t, body, "event: resync_required")
		require.Contains(t, body, "event: message_added")
		assert.Less(t, strings.Index(body, "resync_required"), strings.Index(body, "message_added"),
			"the client must learn the window is unverified before it reads the window")
	})

	t.Run("a short window is not a resync, because nothing was evicted", func(t *testing.T) {
		fx, sinkCh := newStreamFixture(t, []*model.ChatMessage{{Id: "m1", StateId: "s5"}})
		req := streamRequest(route) // default limit 25, one row: the whole chat
		req.Header.Set("Last-Event-ID", "s0")

		w := runStream(t, fx, req, sinkCh, nil)

		assert.NotContains(t, w.Body.String(), "resync_required")
	})

	t.Run("every frame is well formed and the keepalive is a comment", func(t *testing.T) {
		// the deliverable of this route IS a wire format, so something has to
		// check the wire: blank-line terminators, one field per line, and a
		// keepalive that is an SSE comment rather than an event a client has
		// to filter
		fx, sinkCh := newStreamFixture(t, []*model.ChatMessage{{
			Id: "m1", StateId: "s1",
			Message: &model.ChatMessageMessageContent{Text: "line one\nline two"},
		}})

		w := runStream(t, fx, streamRequest(route+"?heartbeat=1"), sinkCh, func(chan<- *pb.Event) {
			time.Sleep(1100 * time.Millisecond) // one heartbeat period
		})

		frames := sseFrames(t, w.Body.String())
		require.NotEmpty(t, frames)
		assert.Equal(t, "message_added", frames[0]["event"])
		assert.Equal(t, "s1", frames[0]["id"])
		// exactly id + event + data: a missing blank-line terminator would
		// fold the following frame into this one and nothing else would notice
		assert.Len(t, frames[0], 3, "a frame ends at its terminator: %v", frames[0])
		assert.Contains(t, frames[0]["data"], `\nline two`,
			"a newline inside the text is escaped by JSON, never a raw one splitting the data line")
		var keepalives int
		for _, frame := range frames {
			if frame["comment"] == "keepalive" {
				keepalives++
				assert.NotContains(t, frame, "event", "a keepalive is a comment, not an event")
			}
		}
		assert.GreaterOrEqual(t, keepalives, 1, "the heartbeat must actually fire")
	})

	t.Run("an out-of-range heartbeat falls back instead of failing", func(t *testing.T) {
		// ?heartbeat=0 would reach time.NewTicker(0), which panics. The bound
		// is what keeps a query parameter from taking the process down.
		fx, sinkCh := newStreamFixture(t, nil)

		w := runStream(t, fx, streamRequest(route+"?heartbeat=0"), sinkCh, nil)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
	})

	t.Run("a deletion carries no resumable id", func(t *testing.T) {
		// an id: line advances the client's Last-Event-ID, and a deletion has
		// no surviving row whose state it could name
		fx, sinkCh := newStreamFixture(t, nil)

		w := runStream(t, fx, streamRequest(route), sinkCh, func(sink chan<- *pb.Event) {
			sink <- &pb.Event{Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfChatDelete{ChatDelete: &pb.EventChatDelete{Id: "m9"}},
			}}}
			time.Sleep(50 * time.Millisecond)
		})

		for _, frame := range sseFrames(t, w.Body.String()) {
			if frame["event"] == "message_deleted" {
				assert.NotContains(t, frame, "id")
				return
			}
		}
		t.Fatal("no message_deleted frame")
	})

	t.Run("an evicted subscription is told to resync, not closed silently", func(t *testing.T) {
		// the producer drops a subscriber that reads too slowly by closing the
		// sink. Returning on !ok without a word looks exactly like a clean
		// shutdown, and the client would never learn its copy is incomplete.
		fx, sinkCh := newStreamFixture(t, nil)

		w := runStream(t, fx, streamRequest(route), sinkCh, func(sink chan<- *pb.Event) {
			close(sink)
			time.Sleep(50 * time.Millisecond)
		})

		assert.Contains(t, w.Body.String(), "event: resync_required")
	})

	t.Run("the trailing cursor is the highest state id, not the last row", func(t *testing.T) {
		// the window is ordered by order id; a message backfilled from an
		// offline peer carries a fresh state id at an old position, so the
		// last row emitted is not the newest state
		fx, sinkCh := newStreamFixture(t, []*model.ChatMessage{
			{Id: "m1", StateId: "s9"},
			{Id: "m2", StateId: "s2"},
		})

		w := runStream(t, fx, streamRequest(route), sinkCh, nil)

		frames := sseFrames(t, w.Body.String())
		last := frames[len(frames)-1]
		assert.Equal(t, "s9", last["id"],
			"the client's cursor must not go backwards on the first reconnect")
		assert.Len(t, last, 1, "the cursor is a frame of its own: %v", last)
	})

	t.Run("a Last-Event-ID inside the window replays nothing already seen", func(t *testing.T) {
		fx, sinkCh := newStreamFixture(t, []*model.ChatMessage{
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
