package v2handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

const spacesStreamRoute = "/v2/spaces/stream"

// Two space ids in the production shape, with different tails: the list
// would serve both by their short reference, so a frame carrying the full id
// shows the stream does not shorten.
const (
	shapedSpaceWork  = "bafyreiay4rdeleruyuy6x575hvhtifedmjq4g3ojpffvpbgmuackq3oake.28y6mgnwgodt7"
	shapedSpaceNotes = "bafyreia4znhhjvxek2iux7enfzilekj5vwlgddgogpfgnx5qnim7bugaxa.28y6mgnwgodt7"
)

// stubKeyWatch is the key watch a handler test hands the stream.
type stubKeyWatch struct {
	changed chan struct{}
	closing chan struct{}

	mu     sync.Mutex
	err    error
	closes int
}

func newStubKeyWatch() *stubKeyWatch {
	return &stubKeyWatch{changed: make(chan struct{}, 1), closing: make(chan struct{})}
}

func (w *stubKeyWatch) State() v2service.KeyState { return v2service.KeyState{} }
func (w *stubKeyWatch) Changed() <-chan struct{}  { return w.changed }
func (w *stubKeyWatch) Closing() <-chan struct{}  { return w.closing }

func (w *stubKeyWatch) Current(context.Context) (v2service.KeyState, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return v2service.KeyState{}, w.err
}

func (w *stubKeyWatch) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closes++
}

func (w *stubKeyWatch) revoke() {
	w.mu.Lock()
	w.err = v2service.ErrKeyRevoked
	w.mu.Unlock()
	w.changed <- struct{}{}
}

func (w *stubKeyWatch) closeCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closes
}

// refuseKeyInTest stands in for the server's refusal of a key: it answers
// 401 naming the error it was handed.
func refuseKeyInTest(c *gin.Context, err error) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"refused": err.Error()})
}

func handlerSpaceView(viewId, target, name string) *domain.Details {
	return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:            domain.String(viewId),
		bundle.RelationKeyTargetSpaceId: domain.String(target),
		bundle.RelationKeyName:          domain.String(name),
	})
}

// frameCounter counts the SSE frames (events and comments) a handler wrote,
// so a test waits for the frames it expects instead of sleeping.
type frameCounter struct {
	gin.ResponseWriter
	mu     sync.Mutex
	frames int
}

func (f *frameCounter) Write(p []byte) (int, error) {
	f.mu.Lock()
	f.frames += strings.Count(string(p), "\n\n")
	f.mu.Unlock()
	return f.ResponseWriter.Write(p)
}

func (f *frameCounter) WriteString(s string) (int, error) { return f.Write([]byte(s)) }

func (f *frameCounter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.frames
}

// spacesStreamFixture serves the spaces stream over a fake subscription that
// holds two space views. watches receives every key watch the handler asked
// for; subs every subscription it opened.
type spacesStreamFixture struct {
	*v2HandlerFixture
	watches  chan *stubKeyWatch
	subs     chan *fakeLiveSearch
	counters chan *frameCounter
}

func newSpacesStreamFixture(t *testing.T) *spacesStreamFixture {
	t.Helper()
	sf := &spacesStreamFixture{
		watches:  make(chan *stubKeyWatch, 64),
		subs:     make(chan *fakeLiveSearch, 64),
		counters: make(chan *frameCounter, 64),
	}
	search := mock_apicore.NewMockObjectSearchService(t)
	search.EXPECT().OpenObjectSearch(mock.Anything, mock.Anything).RunAndReturn(
		func(context.Context, apicore.ObjectSearchOpen) (apicore.ObjectSearchSubscription, error) {
			sub := &fakeLiveSearch{
				snapshot: []apicore.ObjectSearchChange{
					{Type: apicore.ObjectSearchAdded, Id: "view-work", Details: handlerSpaceView("view-work", shapedSpaceWork, "Work")},
					{Type: apicore.ObjectSearchAdded, Id: "view-notes", Details: handlerSpaceView("view-notes", shapedSpaceNotes, "Notes")},
					{Type: apicore.ObjectSearchSnapshotComplete},
				},
				ready: make(chan struct{}, 1),
			}
			sf.subs <- sub
			return sub, nil
		}).Maybe()
	sf.v2HandlerFixture = newV2HandlerFixtureWithObjectSearch(t, search)
	sf.router.Use(func(c *gin.Context) {
		counter := &frameCounter{ResponseWriter: c.Writer}
		c.Writer = counter
		sf.counters <- counter
		c.Next()
	})
	sf.router.GET(spacesStreamRoute, SpacesStreamHandler(sf.svc, func(*gin.Context) (v2service.KeyWatch, bool) {
		watch := newStubKeyWatch()
		sf.watches <- watch
		return watch, true
	}, refuseKeyInTest))
	return sf
}

// run drives the handler: it waits for the stream to open, calls feed, and
// waits until the handler wrote wantFrames frames. Then, when the stream is
// expected to end on its own, it waits for that; otherwise it checks the
// stream is still open and hangs up the way a client would. Every wait has a
// deadline.
func (sf *spacesStreamFixture) run(t *testing.T, target string, wantFrames int, endsItself bool, feed func(watch *stubKeyWatch, sub *fakeLiveSearch)) (*httptest.ResponseRecorder, *stubKeyWatch) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := httptest.NewRecorder()
	done := make(chan struct{})
	req := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
	go func() { defer close(done); sf.router.ServeHTTP(w, req) }()

	var watch *stubKeyWatch
	select {
	case sub := <-sf.subs:
		watch = <-sf.watches
		if feed != nil {
			feed(watch, sub)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stream never opened")
	}
	counter := <-sf.counters
	deadline := time.Now().Add(5 * time.Second)
	for counter.count() < wantFrames {
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatalf("the stream wrote %d frames, want %d: %s", counter.count(), wantFrames, w.Body.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if endsItself {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the stream stayed open; the server must end it")
		}
		return w, watch
	}
	select {
	case <-done:
		t.Fatal("the stream ended on its own; only the client, the key or the server ends it")
	default:
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream did not close when the client hung up")
	}
	return w, watch
}

func decodeSpaceStreamFrames(t *testing.T, body string) []v2model.SpaceStreamEvent {
	t.Helper()
	var out []v2model.SpaceStreamEvent
	for _, frame := range sseFrames(t, body) {
		if _, comment := frame["comment"]; comment {
			continue
		}
		assert.Len(t, frame, 2, "exactly event + data, and no id: the spaces stream has no replay: %v", frame)
		var event v2model.SpaceStreamEvent
		require.NoError(t, json.Unmarshal([]byte(frame["data"]), &event))
		assert.Equal(t, frame["event"], event.Type, "the SSE event name is the body's type")
		out = append(out, event)
	}
	return out
}

func TestSpacesStreamHandler(t *testing.T) {
	flag := func(v bool) *bool { return &v }
	snapshot := []v2model.SpaceStreamEvent{
		{Type: "space_added", Space: &v2model.SpaceRow{Id: shapedSpaceNotes, Name: "Notes"}},
		{Type: "space_added", Space: &v2model.SpaceRow{Id: shapedSpaceWork, Name: "Work"}},
		{Type: "snapshot_complete", HasNotGrantedSpaces: flag(false)},
	}

	t.Run("the snapshot and live events stream as SSE frames with no id, ids in full", func(t *testing.T) {
		// given
		sf := newSpacesStreamFixture(t)
		want := append(append([]v2model.SpaceStreamEvent{}, snapshot...),
			// one batch, so its events come in full-id order: Notes sorts first
			v2model.SpaceStreamEvent{Type: "space_removed", SpaceId: shapedSpaceNotes, Reason: "gone"},
			v2model.SpaceStreamEvent{Type: "space_updated", Space: &v2model.SpaceRow{Id: shapedSpaceWork, Name: "Work 2"}},
		)

		// when
		w, watch := sf.run(t, spacesStreamRoute, len(want), false, func(_ *stubKeyWatch, sub *fakeLiveSearch) {
			sub.push(
				apicore.ObjectSearchChange{Type: apicore.ObjectSearchUpdated, Id: "view-work", Details: handlerSpaceView("view-work", shapedSpaceWork, "Work 2")},
				apicore.ObjectSearchChange{Type: apicore.ObjectSearchRemoved, Id: "view-notes"},
			)
		})

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
		assert.Equal(t, want, decodeSpaceStreamFrames(t, w.Body.String()))
		assert.Equal(t, 1, watch.closeCount(), "leaving the stream closes the key watch")
	})

	t.Run("ids=full is accepted and changes nothing", func(t *testing.T) {
		// given
		sf := newSpacesStreamFixture(t)

		// when
		w, _ := sf.run(t, spacesStreamRoute+"?ids=full", len(snapshot), false, nil)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, snapshot, decodeSpaceStreamFrames(t, w.Body.String()))
	})

	t.Run("ids=compact is a 400 at ids before anything opens", func(t *testing.T) {
		// given: neither a subscription nor a key watch may be asked for
		fx := newV2HandlerFixtureWithObjectSearch(t, mock_apicore.NewMockObjectSearchService(t))
		fx.router.GET(spacesStreamRoute, SpacesStreamHandler(fx.svc, func(*gin.Context) (v2service.KeyWatch, bool) {
			t.Error("a refused request must not register a key watch")
			return nil, false
		}, refuseKeyInTest))
		w := httptest.NewRecorder()

		// when
		fx.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, spacesStreamRoute+"?ids=compact", nil))

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		var got v2model.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, v2model.CodeValidationFailed, got.Code)
		require.Len(t, got.Issues, 1)
		assert.Equal(t, "ids", got.Issues[0].Path)
	})

	t.Run("a key the watch refuses opens nothing and keeps the refusal's response", func(t *testing.T) {
		// given
		fx := newV2HandlerFixtureWithObjectSearch(t, mock_apicore.NewMockObjectSearchService(t))
		fx.router.GET(spacesStreamRoute, SpacesStreamHandler(fx.svc, func(c *gin.Context) (v2service.KeyWatch, bool) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "unauthorized"})
			return nil, false
		}, refuseKeyInTest))
		w := httptest.NewRecorder()

		// when
		fx.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, spacesStreamRoute, nil))

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
		assert.JSONEq(t, `{"code":"unauthorized"}`, w.Body.String())
	})

	t.Run("a key found revoked while the stream opens is refused before the first byte", func(t *testing.T) {
		// given: the revocation's signal is already pending on the watch
		sf := newSpacesStreamFixture(t)
		watch := newStubKeyWatch()
		watch.revoke()
		router := gin.New()
		router.GET(spacesStreamRoute, SpacesStreamHandler(sf.svc, func(*gin.Context) (v2service.KeyWatch, bool) {
			return watch, true
		}, refuseKeyInTest))
		w := httptest.NewRecorder()

		// when
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, spacesStreamRoute, nil))

		// then: the server's refusal, not a stream and not a v2 error envelope
		require.Equal(t, http.StatusUnauthorized, w.Code)
		assert.JSONEq(t, `{"refused":"re-read the key before the snapshot: api key revoked"}`, w.Body.String())
		assert.NotContains(t, w.Header().Get("Content-Type"), "text/event-stream")
		assert.Equal(t, 1, watch.closeCount())
	})

	t.Run("a revoked key ends the response after stream_closed", func(t *testing.T) {
		// given
		sf := newSpacesStreamFixture(t)
		want := append(append([]v2model.SpaceStreamEvent{}, snapshot...),
			v2model.SpaceStreamEvent{Type: "stream_closed", Reason: "key_revoked"})

		// when
		w, watch := sf.run(t, spacesStreamRoute, len(want), true, func(watch *stubKeyWatch, _ *fakeLiveSearch) { watch.revoke() })

		// then
		assert.Equal(t, want, decodeSpaceStreamFrames(t, w.Body.String()))
		assert.Equal(t, 1, watch.closeCount())
	})

	t.Run("the server stopping ends the response with no event", func(t *testing.T) {
		// given
		sf := newSpacesStreamFixture(t)

		// when
		w, _ := sf.run(t, spacesStreamRoute, len(snapshot), true, func(watch *stubKeyWatch, _ *fakeLiveSearch) { close(watch.closing) })

		// then
		assert.Equal(t, snapshot, decodeSpaceStreamFrames(t, w.Body.String()))
	})

	t.Run("the heartbeat query parameter sets the keepalive cadence", func(t *testing.T) {
		// given
		sf := newSpacesStreamFixture(t)

		// when
		// when: one frame more than the snapshot can only be a keepalive
		w, _ := sf.run(t, spacesStreamRoute+"?heartbeat=1", len(snapshot)+1, false, nil)

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

	t.Run("the cap answers 429 too_many_streams once every slot is held", func(t *testing.T) {
		// given: sixteen streams held through the service's own slot counter
		sf := newSpacesStreamFixture(t)
		for i := 0; i < 16; i++ {
			stream, err := sf.svc.OpenSpacesStream(context.Background(), newStubKeyWatch())
			require.NoError(t, err)
			<-sf.subs
			defer stream.Close()
		}
		w := httptest.NewRecorder()

		// when
		sf.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, spacesStreamRoute, nil))

		// then
		require.Equal(t, http.StatusTooManyRequests, w.Code)
		var got v2model.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, v2model.CodeTooManyStreams, got.Code)
		assert.Contains(t, got.Message, "16 open spaces streams")
		watch := <-sf.watches
		assert.Equal(t, 1, watch.closeCount(), "the refused request's key watch is closed")
	})
}
