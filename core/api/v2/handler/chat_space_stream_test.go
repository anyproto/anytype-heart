package v2handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const spaceStreamRoute = "/v2/spaces/space1/chats/stream"

// fakeHubSubscription is the hub port's subscription, fed by the test with
// frames the service's own renderer produced.
type fakeHubSubscription struct {
	snapshot []apicore.SpaceChatFrame
	ready    chan struct{}

	mu     sync.Mutex
	live   []apicore.SpaceChatFrame
	closes int
}

func (f *fakeHubSubscription) Snapshot() []apicore.SpaceChatFrame { return f.snapshot }
func (f *fakeHubSubscription) Ready() <-chan struct{}             { return f.ready }

func (f *fakeHubSubscription) Drain() []apicore.SpaceChatFrame {
	f.mu.Lock()
	defer f.mu.Unlock()
	live := f.live
	f.live = nil
	return live
}

func (f *fakeHubSubscription) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
}

func (f *fakeHubSubscription) push(frames ...apicore.SpaceChatFrame) {
	f.mu.Lock()
	f.live = append(f.live, frames...)
	f.mu.Unlock()
	select {
	case f.ready <- struct{}{}:
	default:
	}
}

func (f *fakeHubSubscription) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closes
}

// spaceStreamHub records each open the handler makes and answers it with a
// subscription whose snapshot is one chat, rendered by the service.
type spaceStreamHub struct {
	opened chan apicore.SpaceChatOpen
	subs   chan *fakeHubSubscription
}

func newSpaceStreamFixture(t *testing.T) (*v2HandlerFixture, *spaceStreamHub) {
	t.Helper()
	hub := &spaceStreamHub{opened: make(chan apicore.SpaceChatOpen, 32), subs: make(chan *fakeHubSubscription, 32)}
	subMock := mock_apicore.NewMockChatSubscriptionService(t)
	subMock.EXPECT().OpenSpaceChats(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, req apicore.SpaceChatOpen) (apicore.SpaceChatSubscription, error) {
			added, err := req.Render(apicore.SpaceChatChange{Type: apicore.SpaceChatAdded, SpaceId: req.SpaceId,
				Chat: apicore.SpaceChat{Id: "chat1", Name: "General"}, State: &model.ChatState{LastStateId: "s3"}})
			require.NoError(t, err)
			complete, err := req.Render(apicore.SpaceChatChange{Type: apicore.SpaceChatSnapshotComplete, SpaceId: req.SpaceId})
			require.NoError(t, err)
			sub := &fakeHubSubscription{snapshot: []apicore.SpaceChatFrame{added, complete}, ready: make(chan struct{}, 1)}
			hub.opened <- req
			hub.subs <- sub
			return sub, nil
		}).Maybe()
	fx := newV2HandlerFixtureWithChatSub(t, subMock)
	fx.router.GET("/v2/spaces/:space_id/chats/stream", SpaceChatStreamHandler(fx.svc))
	return fx, hub
}

// runSpaceStream drives the handler until feed returns, then hangs up the
// way a client would: the stream itself never ends on its own.
func runSpaceStream(t *testing.T, fx *v2HandlerFixture, hub *spaceStreamHub, req *http.Request,
	feed func(sub *fakeHubSubscription, render apicore.SpaceChatRender)) (*httptest.ResponseRecorder, *fakeHubSubscription) {
	t.Helper()
	ctx, cancel := context.WithCancel(req.Context())
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); fx.router.ServeHTTP(w, req.WithContext(ctx)) }()

	var sub *fakeHubSubscription
	select {
	case open := <-hub.opened:
		sub = <-hub.subs
		if feed != nil {
			feed(sub, open.Render)
		}
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("the stream never opened")
	}
	select {
	case <-done:
		t.Fatal("the stream ended on its own; only the client may end it")
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the stream did not close when the client hung up")
	}
	return w, sub
}

func decodeSpaceEvent(t *testing.T, data string) v2model.SpaceChatEvent {
	t.Helper()
	var event v2model.SpaceChatEvent
	require.NoError(t, json.Unmarshal([]byte(data), &event))
	return event
}

func TestSpaceChatStreamHandler(t *testing.T) {
	t.Run("the snapshot, snapshot_complete and live events stream as SSE frames with no id", func(t *testing.T) {
		// given
		fx, hub := newSpaceStreamFixture(t)

		// when
		w, sub := runSpaceStream(t, fx, hub, streamRequest(spaceStreamRoute), func(sub *fakeHubSubscription, render apicore.SpaceChatRender) {
			frame, err := render(apicore.SpaceChatChange{Type: apicore.SpaceChatMessageAdded, SpaceId: "space1",
				Chat: apicore.SpaceChat{Id: "disc1", Name: "Plan", Discussion: true, ParentId: "page1"}, MessageId: "m1",
				Message: &model.ChatMessage{Id: "m1", StateId: "s4", OrderId: "o1", Message: &model.ChatMessageMessageContent{Text: "hi"}}})
			require.NoError(t, err)
			sub.push(frame)
			time.Sleep(50 * time.Millisecond) // let the loop drain before the hangup
		})

		// then
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
		frames := sseFrames(t, w.Body.String())
		require.Len(t, frames, 3)
		var types []string
		for _, frame := range frames {
			assert.Len(t, frame, 2, "exactly event + data, and no id: the space stream has no replay: %v", frame)
			event := decodeSpaceEvent(t, frame["data"])
			assert.Equal(t, frame["event"], event.Type)
			assert.Equal(t, "space1", event.SpaceId, "every event carries space_id")
			types = append(types, event.Type)
		}
		assert.Equal(t, []string{"chat_added", "snapshot_complete", "message_added"}, types)
		message := decodeSpaceEvent(t, frames[2]["data"])
		want := v2model.SpaceChatEvent{Type: "message_added", SpaceId: "space1", ChatId: "disc1", Kind: "discussion", ParentId: "page1",
			Message: &v2model.ChatMessage{Id: "m1", StateId: "s4", Order: "o1", Text: "hi"}}
		assert.Equal(t, want, message)
		assert.Equal(t, 1, sub.closeCount(), "leaving the stream leaves the hub")
	})

	t.Run("include defaults to discussions and none narrows to chats", func(t *testing.T) {
		for query, want := range map[string]bool{"": true, "?include=discussions": true, "?include=none": false} {
			// given
			fx, hub := newSpaceStreamFixture(t)
			var got apicore.SpaceChatOpen
			recording := &spaceStreamHub{opened: make(chan apicore.SpaceChatOpen, 1), subs: hub.subs}
			go func() {
				open := <-hub.opened
				got = open
				recording.opened <- open
			}()

			// when
			runSpaceStream(t, fx, recording, streamRequest(spaceStreamRoute+query), nil)

			// then
			assert.Equal(t, want, got.IncludeDiscussions, "query %q", query)
		}
	})

	t.Run("an unknown include is a 400 naming the parameter before anything opens", func(t *testing.T) {
		// given
		fx, _ := newSpaceStreamFixture(t)
		w := httptest.NewRecorder()

		// when
		fx.router.ServeHTTP(w, streamRequest(spaceStreamRoute+"?include=all"))

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		var got v2model.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, v2model.CodeValidationFailed, got.Code)
		require.Len(t, got.Issues, 1)
		assert.Equal(t, "include", got.Issues[0].Path)
		assert.Contains(t, got.Issues[0].Hint, "discussions")
		assert.Contains(t, got.Issues[0].Hint, "none")
	})

	t.Run("a space outside the key's grant refuses in the C6 envelope before the first byte", func(t *testing.T) {
		// given
		fx, _ := newSpaceStreamFixture(t)
		req := streamRequest(spaceStreamRoute)
		req = req.WithContext(util.CtxWithApiGrant(req.Context(), &util.ApiGrant{Spaces: []string{"other"}, Perms: util.GrantPermsRead}))
		w := httptest.NewRecorder()

		// when
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
		assert.Contains(t, w.Body.String(), v2model.CodeSpaceNotGranted)
	})

	t.Run("the heartbeat query parameter sets the keepalive cadence", func(t *testing.T) {
		// given
		fx, hub := newSpaceStreamFixture(t)

		// when
		w, _ := runSpaceStream(t, fx, hub, streamRequest(spaceStreamRoute+"?heartbeat=1"), func(*fakeHubSubscription, apicore.SpaceChatRender) {
			time.Sleep(1100 * time.Millisecond) // one heartbeat period
		})

		// then
		var keepalives int
		for _, frame := range sseFrames(t, w.Body.String()) {
			if frame["comment"] == "keepalive" {
				keepalives++
				assert.NotContains(t, frame, "event", "a keepalive is a comment, not an event")
			}
		}
		assert.GreaterOrEqual(t, keepalives, 1)
	})

	t.Run("an out-of-range heartbeat falls back instead of failing", func(t *testing.T) {
		// given
		fx, hub := newSpaceStreamFixture(t)

		// when
		w, _ := runSpaceStream(t, fx, hub, streamRequest(spaceStreamRoute+"?heartbeat=0"), nil)

		// then
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("the cap answers 429 too_many_streams in the C6 envelope", func(t *testing.T) {
		// given: every slot is held
		fx, hub := newSpaceStreamFixture(t)
		for i := 0; i < 16; i++ {
			stream, err := fx.svc.OpenSpaceChatStream(context.Background(), "space1", v2service.SpaceChatStreamQuery{})
			require.NoError(t, err)
			<-hub.opened
			<-hub.subs
			defer stream.Close()
		}
		w := httptest.NewRecorder()

		// when
		fx.router.ServeHTTP(w, streamRequest(spaceStreamRoute))

		// then
		require.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Contains(t, w.Body.String(), v2model.CodeTooManyStreams)
		assert.Contains(t, w.Body.String(), "space chat streams")
	})

	t.Run("a failing write ends the stream and still leaves the hub", func(t *testing.T) {
		// given: the peer is gone but never sent a FIN, so only the write
		// error can end the stream
		fx, hub := newSpaceStreamFixture(t)
		var failing *failingWriter
		fx.router.Use(func(c *gin.Context) {
			failing = &failingWriter{ResponseWriter: c.Writer, remaining: 0}
			c.Writer = failing
			c.Next()
		})
		// re-register so the middleware above is in the chain for this route
		fx.router.GET("/v2/spaces/:space_id/chats/stream2", SpaceChatStreamHandler(fx.svc))

		// when
		done := make(chan struct{})
		go func() {
			defer close(done)
			fx.router.ServeHTTP(httptest.NewRecorder(), streamRequest("/v2/spaces/space1/chats/stream2"))
		}()
		<-hub.opened
		sub := <-hub.subs

		// then
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("the handler kept going after its writes started failing")
		}
		assert.Equal(t, 1, failing.writes, "the stream stops at the first failed write")
		assert.Equal(t, 1, sub.closeCount(), "and still leaves the hub")
	})
}
