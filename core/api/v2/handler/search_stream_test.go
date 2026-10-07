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
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const searchStreamRoute = "/v2/spaces/space1/search/stream"

// mentionsBody is the agent recipe the docs teach.
const mentionsBody = `{"filter":"unread_mention_count > 0","fields":["name","discussion"]}`

// fakeLiveSearch is the live search port's subscription, fed by the test.
type fakeLiveSearch struct {
	snapshot []apicore.ObjectSearchChange
	ready    chan struct{}

	mu     sync.Mutex
	live   []apicore.ObjectSearchChange
	closes int
}

func (f *fakeLiveSearch) Snapshot() []apicore.ObjectSearchChange { return f.snapshot }
func (f *fakeLiveSearch) Ready() <-chan struct{}                 { return f.ready }

func (f *fakeLiveSearch) Drain() []apicore.ObjectSearchChange {
	f.mu.Lock()
	defer f.mu.Unlock()
	live := f.live
	f.live = nil
	return live
}

func (f *fakeLiveSearch) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
}

func (f *fakeLiveSearch) push(changes ...apicore.ObjectSearchChange) {
	f.mu.Lock()
	f.live = append(f.live, changes...)
	f.mu.Unlock()
	select {
	case f.ready <- struct{}{}:
	default:
	}
}

func (f *fakeLiveSearch) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closes
}

// liveSearchPort records each open the handler makes and answers it with a
// subscription whose snapshot is one mentioned page.
type liveSearchPort struct {
	opened chan apicore.ObjectSearchOpen
	subs   chan *fakeLiveSearch
}

func mentionedPage(mentions int64) *domain.Details {
	return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:                 domain.String("page1"),
		bundle.RelationKeyName:               domain.String("Plan"),
		bundle.RelationKeyType:               domain.String("type-page"),
		bundle.RelationKeyDiscussionId:       domain.String("disc1"),
		bundle.RelationKeyUnreadMentionCount: domain.Int64(mentions),
	})
}

func newSearchStreamFixture(t *testing.T) (*v2HandlerFixture, *liveSearchPort) {
	t.Helper()
	port := &liveSearchPort{opened: make(chan apicore.ObjectSearchOpen, 32), subs: make(chan *fakeLiveSearch, 32)}
	search := mock_apicore.NewMockObjectSearchService(t)
	search.EXPECT().OpenObjectSearch(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, req apicore.ObjectSearchOpen) (apicore.ObjectSearchSubscription, error) {
			sub := &fakeLiveSearch{
				snapshot: []apicore.ObjectSearchChange{
					{Type: apicore.ObjectSearchAdded, Id: "page1", Details: mentionedPage(1)},
					{Type: apicore.ObjectSearchSnapshotComplete},
				},
				ready: make(chan struct{}, 1),
			}
			port.opened <- req
			port.subs <- sub
			return sub, nil
		}).Maybe()
	fx := newV2HandlerFixtureWithObjectSearch(t, search)
	// a real space holds the name relation (a required internal relation)
	// and its types; the counters and the discussion have no relation object
	fx.store.AddObjects(t, "space1", []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String("type-page"),
			bundle.RelationKeyName:           domain.String("Page"),
			bundle.RelationKeyUniqueKey:      domain.String("ot-page"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
		},
		{
			bundle.RelationKeyId:             domain.String("rel-name"),
			bundle.RelationKeyRelationKey:    domain.String("name"),
			bundle.RelationKeyName:           domain.String("Name"),
			bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_shorttext)),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		},
	})
	fx.router.POST("/v2/spaces/:space_id/search/stream", SearchStreamHandler(fx.svc))
	return fx, port
}

func searchStreamRequest(target, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// runSearchStream drives the handler until feed returns, then hangs up the
// way a client would: the stream itself never ends on its own.
func runSearchStream(t *testing.T, fx *v2HandlerFixture, port *liveSearchPort, req *http.Request, feed func(sub *fakeLiveSearch)) (*httptest.ResponseRecorder, *fakeLiveSearch) {
	t.Helper()
	ctx, cancel := context.WithCancel(req.Context())
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); fx.router.ServeHTTP(w, req.WithContext(ctx)) }()

	var sub *fakeLiveSearch
	select {
	case <-port.opened:
		sub = <-port.subs
		if feed != nil {
			feed(sub)
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

func decodeSearchEvent(t *testing.T, data string) v2model.SearchStreamEvent {
	t.Helper()
	var event v2model.SearchStreamEvent
	require.NoError(t, json.Unmarshal([]byte(data), &event))
	return event
}

func TestSearchStreamHandler(t *testing.T) {
	t.Run("the snapshot, snapshot_complete and live events stream as SSE frames with no id", func(t *testing.T) {
		// given
		fx, port := newSearchStreamFixture(t)
		row := func(id, name string) *v2model.ObjectRow {
			return &v2model.ObjectRow{Id: id, Name: name, Type: "page", Discussion: "disc1", Properties: map[string]any{"name": name}}
		}
		want := []v2model.SearchStreamEvent{
			{Type: "object_added", SpaceId: "space1", Object: row("page1", "Plan")},
			{Type: "snapshot_complete", SpaceId: "space1"},
			{Type: "object_updated", SpaceId: "space1", Object: row("page1", "Plan")},
			{Type: "object_removed", SpaceId: "space1", ObjectId: "page1"},
		}

		// when
		w, sub := runSearchStream(t, fx, port, searchStreamRequest(searchStreamRoute, mentionsBody), func(sub *fakeLiveSearch) {
			sub.push(
				apicore.ObjectSearchChange{Type: apicore.ObjectSearchUpdated, Id: "page1", Details: mentionedPage(2)},
				apicore.ObjectSearchChange{Type: apicore.ObjectSearchRemoved, Id: "page1"},
			)
			time.Sleep(50 * time.Millisecond) // let the loop drain before the hangup
		})

		// then
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/event-stream")
		frames := sseFrames(t, w.Body.String())
		got := make([]v2model.SearchStreamEvent, 0, len(frames))
		for _, frame := range frames {
			assert.Len(t, frame, 2, "exactly event + data, and no id: the search stream has no replay: %v", frame)
			event := decodeSearchEvent(t, frame["data"])
			assert.Equal(t, frame["event"], event.Type)
			got = append(got, event)
		}
		assert.Equal(t, want, got)
		assert.Equal(t, 1, sub.closeCount(), "leaving the stream closes the live search")
	})

	t.Run("the body opens the search it compiles to", func(t *testing.T) {
		// given
		fx, port := newSearchStreamFixture(t)
		var got apicore.ObjectSearchOpen
		recording := &liveSearchPort{opened: make(chan apicore.ObjectSearchOpen, 1), subs: port.subs}
		go func() {
			open := <-port.opened
			got = open
			recording.opened <- open
		}()

		// when
		runSearchStream(t, fx, recording, searchStreamRequest(searchStreamRoute, mentionsBody), nil)

		// then
		assert.Equal(t, "space1", got.SpaceId)
		require.NotEmpty(t, got.Filters)
		assert.Equal(t, bundle.RelationKeyUnreadMentionCount, got.Filters[0].RelationKey)
		assert.Equal(t, model.BlockContentDataviewFilter_Greater, got.Filters[0].Condition)
	})

	t.Run("a reconnect with Last-Event-ID is a fresh snapshot", func(t *testing.T) {
		// given
		fx, port := newSearchStreamFixture(t)
		req := searchStreamRequest(searchStreamRoute, mentionsBody)
		req.Header.Set("Last-Event-ID", "anything")

		// when
		w, _ := runSearchStream(t, fx, port, req, nil)

		// then
		var types []string
		for _, frame := range sseFrames(t, w.Body.String()) {
			types = append(types, frame["event"])
		}
		assert.Equal(t, []string{"object_added", "snapshot_complete"}, types)
	})

	for name, tc := range map[string]struct {
		body     string
		wantPath string
	}{
		"an unknown property":      {body: `{"filter":"unread_mention > 0"}`, wantPath: "/filter"},
		"a paging field":           {body: `{"filter":"unread_mention_count > 0","limit":10}`, wantPath: "/limit"},
		"a full-text query":        {body: `{"query":"plan"}`, wantPath: "/query"},
		"a body that is not JSON":  {body: `{"filter":`, wantPath: ""},
		"an unknown field to show": {body: `{"fields":["no_such_field"]}`, wantPath: "/fields/0"},
	} {
		t.Run(name+" is a 400 in the C6 envelope before anything opens", func(t *testing.T) {
			// given: no open may happen
			fx := newV2HandlerFixtureWithObjectSearch(t, mock_apicore.NewMockObjectSearchService(t))
			fx.router.POST("/v2/spaces/:space_id/search/stream", SearchStreamHandler(fx.svc))
			w := httptest.NewRecorder()

			// when
			fx.router.ServeHTTP(w, searchStreamRequest(searchStreamRoute, tc.body))

			// then
			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
			var got v2model.Error
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
			require.NotEmpty(t, got.Issues)
			assert.Equal(t, tc.wantPath, got.Issues[0].Path)
		})
	}

	t.Run("a space outside the key's grant refuses in the C6 envelope before the first byte", func(t *testing.T) {
		// given
		fx := newV2HandlerFixtureWithObjectSearch(t, mock_apicore.NewMockObjectSearchService(t))
		fx.router.POST("/v2/spaces/:space_id/search/stream", SearchStreamHandler(fx.svc))
		req := searchStreamRequest(searchStreamRoute, mentionsBody)
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
		fx, port := newSearchStreamFixture(t)

		// when
		w, _ := runSearchStream(t, fx, port, searchStreamRequest(searchStreamRoute+"?heartbeat=1", mentionsBody), func(*fakeLiveSearch) {
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

	t.Run("the cap answers 429 too_many_streams in the C6 envelope", func(t *testing.T) {
		// given: every slot is held
		fx, port := newSearchStreamFixture(t)
		for i := 0; i < 16; i++ {
			stream, err := fx.svc.OpenSearchStream(context.Background(), "space1", v2model.SearchRequest{})
			require.NoError(t, err)
			<-port.opened
			<-port.subs
			defer stream.Close()
		}
		w := httptest.NewRecorder()

		// when
		fx.router.ServeHTTP(w, searchStreamRequest(searchStreamRoute, mentionsBody))

		// then
		require.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Contains(t, w.Body.String(), v2model.CodeTooManyStreams)
		assert.Contains(t, w.Body.String(), "search streams")
	})

	t.Run("a failing write ends the stream and still closes the live search", func(t *testing.T) {
		// given: the peer is gone but never sent a FIN, so only the write
		// error can end the stream
		fx, port := newSearchStreamFixture(t)
		var failing *failingWriter
		fx.router.Use(func(c *gin.Context) {
			failing = &failingWriter{ResponseWriter: c.Writer, remaining: 0}
			c.Writer = failing
			c.Next()
		})
		// re-register so the middleware above is in the chain for this route
		fx.router.POST("/v2/spaces/:space_id/search/stream2", SearchStreamHandler(fx.svc))

		// when
		done := make(chan struct{})
		go func() {
			defer close(done)
			fx.router.ServeHTTP(httptest.NewRecorder(), searchStreamRequest("/v2/spaces/space1/search/stream2", mentionsBody))
		}()
		<-port.opened
		sub := <-port.subs

		// then
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("the handler kept going after its writes started failing")
		}
		assert.Equal(t, 1, failing.writes, "the stream stops at the first failed write")
		assert.Equal(t, 1, sub.closeCount(), "and still closes the live search")
	})
}
