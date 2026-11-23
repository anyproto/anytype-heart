package server

// spaces_stream_test.go drives the spaces stream end to end through the real
// engine over real HTTP: the authentication middleware, the key watch, the
// v2 service and the SSE handler. The space views come from a fake live
// search; everything about the KEY — the cache, the eviction generation,
// RevokeToken's signal, the re-read — is the production code.

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Space ids in the production shape, so the stream's full-id rule is live.
const (
	streamSpaceA = "bafyreia4znhhjvxek2iux7enfzilekj5vwlgddgogpfgnx5qnim7bugaxa.28y6mgnwgodt7"
	streamSpaceB = "bafyreiay4rdeleruyuy6x575hvhtifedmjq4g3ojpffvpbgmuackq3oake.28y6mgnwgodt7"
)

func streamView(viewId, target, name string) *domain.Details {
	return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyId:            domain.String(viewId),
		bundle.RelationKeyTargetSpaceId: domain.String(target),
		bundle.RelationKeyName:          domain.String(name),
	})
}

// fakeViewSearch is the live search port: every open gets the same views as
// its snapshot, and push feeds every open subscription.
type fakeViewSearch struct {
	views []*domain.Details

	mu   sync.Mutex
	subs []*fakeViewSub
}

type fakeViewSub struct {
	snapshot []apicore.ObjectSearchChange
	ready    chan struct{}

	mu   sync.Mutex
	live []apicore.ObjectSearchChange
}

func (s *fakeViewSub) Snapshot() []apicore.ObjectSearchChange { return s.snapshot }
func (s *fakeViewSub) Ready() <-chan struct{}                 { return s.ready }
func (s *fakeViewSub) Close()                                 {}

func (s *fakeViewSub) Drain() []apicore.ObjectSearchChange {
	s.mu.Lock()
	defer s.mu.Unlock()
	live := s.live
	s.live = nil
	return live
}

func (f *fakeViewSearch) OpenObjectSearch(context.Context, apicore.ObjectSearchOpen) (apicore.ObjectSearchSubscription, error) {
	sub := &fakeViewSub{ready: make(chan struct{}, 1)}
	for _, view := range f.views {
		sub.snapshot = append(sub.snapshot, apicore.ObjectSearchChange{
			Type: apicore.ObjectSearchAdded, Id: view.GetString(bundle.RelationKeyId), Details: view,
		})
	}
	sub.snapshot = append(sub.snapshot, apicore.ObjectSearchChange{Type: apicore.ObjectSearchSnapshotComplete})
	f.mu.Lock()
	f.subs = append(f.subs, sub)
	f.mu.Unlock()
	return sub, nil
}

// rename is an engine change every open stream must emit: a view's new name.
func (f *fakeViewSearch) rename(viewId, target, name string) {
	change := apicore.ObjectSearchChange{Type: apicore.ObjectSearchUpdated, Id: viewId, Details: streamView(viewId, target, name)}
	f.mu.Lock()
	subs := append([]*fakeViewSub(nil), f.subs...)
	f.mu.Unlock()
	for _, sub := range subs {
		sub.mu.Lock()
		sub.live = append(sub.live, change)
		sub.mu.Unlock()
		select {
		case sub.ready <- struct{}{}:
		default:
		}
	}
}

// withSpaceViews makes the fixture's live search serve the given views.
func (fx *fixture) withSpaceViews(views ...*domain.Details) *fakeViewSearch {
	search := &fakeViewSearch{views: views}
	fx.objectSearchMock.EXPECT().OpenObjectSearch(mock.Anything, mock.Anything).RunAndReturn(search.OpenObjectSearch).Maybe()
	return search
}

// sseStream reads one Server-Sent Events response. Every read has a
// deadline: a stream that should have ended, or an event that never came,
// fails the test instead of hanging it.
type sseStream struct {
	t      *testing.T
	resp   *http.Response
	events chan v2model.SpaceStreamEvent
}

func openSpacesStream(t *testing.T, baseUrl, key string) *sseStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseUrl+"/v2/spaces/stream", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		cancel()
		_ = resp.Body.Close()
	})
	s := &sseStream{t: t, resp: resp, events: make(chan v2model.SpaceStreamEvent, 256)}
	if resp.StatusCode != http.StatusOK {
		close(s.events)
		return s
	}
	go func() {
		defer close(s.events)
		scanner := bufio.NewScanner(resp.Body)
		var name, data string
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				if name != "" {
					var event v2model.SpaceStreamEvent
					if err := json.Unmarshal([]byte(data), &event); err != nil || event.Type != name {
						event = v2model.SpaceStreamEvent{Type: "malformed frame: " + name + " " + data}
					}
					s.events <- event
				}
				name, data = "", ""
			case strings.HasPrefix(line, "event: "):
				name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			case strings.HasPrefix(line, "id:"):
				name = "unexpected id line"
			}
		}
	}()
	return s
}

// next returns the next n events.
func (s *sseStream) next(n int) []v2model.SpaceStreamEvent {
	s.t.Helper()
	out := make([]v2model.SpaceStreamEvent, 0, n)
	for len(out) < n {
		select {
		case event, open := <-s.events:
			if !open {
				s.t.Fatalf("the stream ended after %d of %d events: %+v", len(out), n, out)
			}
			out = append(out, event)
		case <-time.After(5 * time.Second):
			s.t.Fatalf("got %d of %d events: %+v", len(out), n, out)
		}
	}
	return out
}

// ended requires the response body to end with no further event.
func (s *sseStream) ended() {
	s.t.Helper()
	select {
	case event, open := <-s.events:
		if open {
			s.t.Fatalf("an event followed where the stream had to end: %+v", event)
		}
	case <-time.After(5 * time.Second):
		s.t.Fatal("the stream stayed open: the response body did not end")
	}
}

func spaceAdded(id, name string) v2model.SpaceStreamEvent {
	return v2model.SpaceStreamEvent{Type: v2model.SpaceStreamEventSpaceAdded, Space: &v2model.SpaceRow{Id: id, Name: name}}
}

func spaceUpdated(id, name string) v2model.SpaceStreamEvent {
	return v2model.SpaceStreamEvent{Type: v2model.SpaceStreamEventSpaceUpdated, Space: &v2model.SpaceRow{Id: id, Name: name}}
}

func streamFlag(event string, flag bool) v2model.SpaceStreamEvent {
	return v2model.SpaceStreamEvent{Type: event, HasNotGrantedSpaces: &flag}
}

func mintRequestFor(key string) *pb.RpcWalletCreateSessionRequest {
	return &pb.RpcWalletCreateSessionRequest{Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{AppKey: key}}
}

// minted is a successful session mint carrying a grant of the given spaces.
func minted(token string, spaces ...string) *pb.RpcWalletCreateSessionResponse {
	return &pb.RpcWalletCreateSessionResponse{
		Token:        token,
		AccountScope: model.AccountAuth_JsonAPI,
		AppName:      "agent",
		Grant:        &model.AccountAuthAppGrant{SpaceIds: spaces, Perm: model.AccountAuthAppGrant_ReadWrite},
		Error:        &pb.RpcWalletCreateSessionResponseError{Code: pb.RpcWalletCreateSessionResponseError_NULL},
	}
}

func mintRefused(code pb.RpcWalletCreateSessionResponseErrorCode) *pb.RpcWalletCreateSessionResponse {
	return &pb.RpcWalletCreateSessionResponse{Error: &pb.RpcWalletCreateSessionResponseError{Code: code}}
}

// watcherCount is how many key watches the server holds.
func (fx *fixture) watcherCount() int {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	n := 0
	for _, set := range fx.watchers {
		n += len(set)
	}
	return n
}

// requireNoWatchers waits, with a deadline, for the registry to empty: the
// handler unregisters as it returns, after the response body ended.
func (fx *fixture) requireNoWatchers(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for fx.watcherCount() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%d key watches are still registered", fx.watcherCount())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func newSpacesStreamServer(t *testing.T) (*fixture, *fakeViewSearch, *httptest.Server) {
	t.Helper()
	fx := newV2ServerFixture(t)
	fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
	search := fx.withSpaceViews(streamView("view-a", streamSpaceA, "A"), streamView("view-b", streamSpaceB, "B"))
	ts := httptest.NewServer(fx.Engine())
	t.Cleanup(ts.Close)
	return fx, search, ts
}

func TestSpacesStreamEditDuringMintIsNotLost(t *testing.T) {
	// R1 on the cache-miss path. The narrowing lands while the request's
	// own mint is in flight: the mint answers the WIDE grant and the edit's
	// sweep finds no watcher, because the stream does not exist yet. Only
	// the eviction generation says anything happened — the watch compares it
	// when it registers and re-reads the key before the snapshot. Without
	// that comparison the snapshot carries B and nothing ever removes it.
	fx, search, ts := newSpacesStreamServer(t)
	fx.mwMock.On("WalletCreateSession", mock.Anything, mintRequestFor("raceKey")).
		Run(func(mock.Arguments) { fx.RevokeToken("raceTok") }).
		Return(minted("raceTok", streamSpaceA, streamSpaceB), nil).Once()
	fx.mwMock.On("WalletCreateSession", mock.Anything, mintRequestFor("raceKey")).
		Return(minted("raceTok2", streamSpaceA), nil).Once()

	// when
	stream := openSpacesStream(t, ts.URL, "raceKey")

	// then: A alone, and B is reported hidden
	require.Equal(t, http.StatusOK, stream.resp.StatusCode)
	want := []v2model.SpaceStreamEvent{spaceAdded(streamSpaceA, "A"), streamFlag("snapshot_complete", true)}
	assert.Equal(t, want, stream.next(2))

	// and no removal follows: the next thing the stream says is the next change
	search.rename("view-a", streamSpaceA, "A2")
	assert.Equal(t, []v2model.SpaceStreamEvent{spaceUpdated(streamSpaceA, "A2")}, stream.next(1))
	fx.mwMock.AssertNumberOfCalls(t, "WalletCreateSession", 2)
}

func TestSpacesStreamOpenRefusals(t *testing.T) {
	// A key revoked or expired between the request's authentication and the
	// stream's registration is answered before the first byte, with the
	// authentication middleware's own 401.
	for name, tc := range map[string]struct {
		code    pb.RpcWalletCreateSessionResponseErrorCode
		message string
	}{
		"revoked":      {pb.RpcWalletCreateSessionResponseError_APP_TOKEN_NOT_FOUND_IN_THE_CURRENT_ACCOUNT, "invalid api key"},
		"expired":      {pb.RpcWalletCreateSessionResponseError_APP_TOKEN_EXPIRED, "api key expired"},
		"unverifiable": {pb.RpcWalletCreateSessionResponseError_UNKNOWN_ERROR, "invalid api key"},
	} {
		t.Run(name, func(t *testing.T) {
			// given: the sweep lands during the opening mint
			fx, _, ts := newSpacesStreamServer(t)
			fx.mwMock.On("WalletCreateSession", mock.Anything, mintRequestFor("goneKey")).
				Run(func(mock.Arguments) { fx.RevokeToken("goneTok") }).
				Return(minted("goneTok", streamSpaceA), nil).Once()
			fx.mwMock.On("WalletCreateSession", mock.Anything, mintRequestFor("goneKey")).
				Return(mintRefused(tc.code), nil).Once()

			// when
			stream := openSpacesStream(t, ts.URL, "goneKey")

			// then
			require.Equal(t, http.StatusUnauthorized, stream.resp.StatusCode)
			assert.Equal(t, util.BearerChallengeInvalidToken(), stream.resp.Header.Get(util.WwwAuthenticateHeader))
			body, err := io.ReadAll(stream.resp.Body)
			require.NoError(t, err)
			assert.Contains(t, string(body), tc.message)
			assert.Equal(t, 0, fx.watcherCount(), "a refused open leaves no registration")
		})
	}
}

func TestSpacesStreamEditAndRevoke(t *testing.T) {
	wide := &util.ApiGrant{Spaces: []string{streamSpaceA, streamSpaceB}, Perms: util.GrantPermsReadWrite}

	t.Run("an edit narrows the open stream, which stays open; a revoke ends it; a later sweep mints nothing", func(t *testing.T) {
		// given: a cached session (token "tok") with both spaces
		fx, search, ts := newSpacesStreamServer(t)
		grantedSession(fx, "liveKey", wide)
		stream := openSpacesStream(t, ts.URL, "liveKey")
		require.Equal(t, http.StatusOK, stream.resp.StatusCode)
		require.Equal(t, []v2model.SpaceStreamEvent{
			spaceAdded(streamSpaceA, "A"), spaceAdded(streamSpaceB, "B"), streamFlag("snapshot_complete", false),
		}, stream.next(3))

		// when: the grant is narrowed to A (persisted first, then swept)
		fx.mwMock.On("WalletCreateSession", mock.Anything, mintRequestFor("liveKey")).
			Return(minted("tok2", streamSpaceA), nil).Once()
		fx.RevokeToken("tok")

		// then: B leaves with the reason, and the flag follows
		assert.Equal(t, []v2model.SpaceStreamEvent{
			{Type: "space_removed", SpaceId: streamSpaceB, Reason: "access_removed"},
			streamFlag("not_granted_updated", true),
		}, stream.next(2))
		// and the stream is still open: a later change arrives
		search.rename("view-a", streamSpaceA, "A2")
		assert.Equal(t, []v2model.SpaceStreamEvent{spaceUpdated(streamSpaceA, "A2")}, stream.next(1))

		// when: the key is revoked (the link is deleted, then swept)
		fx.mwMock.On("WalletCreateSession", mock.Anything, mintRequestFor("liveKey")).
			Return(mintRefused(pb.RpcWalletCreateSessionResponseError_APP_TOKEN_NOT_FOUND_IN_THE_CURRENT_ACCOUNT), nil).Once()
		fx.RevokeToken("tok")

		// then: the last frame says why, and the body ends
		assert.Equal(t, []v2model.SpaceStreamEvent{{Type: "stream_closed", Reason: "key_revoked"}}, stream.next(1))
		stream.ended()

		// and a sweep after the end finds no watcher and mints nothing
		fx.requireNoWatchers(t)
		fx.RevokeToken("tok")
		fx.mwMock.AssertNumberOfCalls(t, "WalletCreateSession", 2)
	})

	t.Run("an edit's sweep and a revoke's, in either order, end the stream as revoked", func(t *testing.T) {
		// given: the link is already deleted when the first sweep is heard —
		// every re-read reports the link as it is at read time
		fx, _, ts := newSpacesStreamServer(t)
		grantedSession(fx, "liveKey", wide)
		stream := openSpacesStream(t, ts.URL, "liveKey")
		require.Equal(t, http.StatusOK, stream.resp.StatusCode)
		stream.next(3)
		fx.mwMock.On("WalletCreateSession", mock.Anything, mintRequestFor("liveKey")).
			Return(mintRefused(pb.RpcWalletCreateSessionResponseError_APP_TOKEN_NOT_FOUND_IN_THE_CURRENT_ACCOUNT), nil).Once()

		// when: both sweeps land back to back
		fx.RevokeToken("tok")
		fx.RevokeToken("tok")

		// then: one re-read, one closing frame, the end
		assert.Equal(t, []v2model.SpaceStreamEvent{{Type: "stream_closed", Reason: "key_revoked"}}, stream.next(1))
		stream.ended()
		fx.requireNoWatchers(t)
		fx.mwMock.AssertNumberOfCalls(t, "WalletCreateSession", 1)
	})

	t.Run("a sweep of another key's token re-reads nothing", func(t *testing.T) {
		// given: no mint is expected — the strict mock fails one
		fx, search, ts := newSpacesStreamServer(t)
		grantedSession(fx, "liveKey", wide)
		stream := openSpacesStream(t, ts.URL, "liveKey")
		require.Equal(t, http.StatusOK, stream.resp.StatusCode)
		stream.next(3)

		// when
		fx.RevokeToken("someone-elses-token")
		search.rename("view-a", streamSpaceA, "A2")

		// then: the stream goes on untouched
		assert.Equal(t, []v2model.SpaceStreamEvent{spaceUpdated(streamSpaceA, "A2")}, stream.next(1))
		fx.mwMock.AssertNotCalled(t, "WalletCreateSession", mock.Anything, mock.Anything)
	})
}

func TestSpacesStreamEndsOnServerStop(t *testing.T) {
	// A rebound or stopped server is replaced, and RevokeToken reaches only
	// its replacement; http.Server.Shutdown does not interrupt an open
	// stream. CloseStreams is what ends it — a stream that selected only on
	// its request context would keep the body open past the deadline.
	fx, _, ts := newSpacesStreamServer(t)
	grantedSession(fx, "liveKey", &util.ApiGrant{AllSpaces: true, Perms: util.GrantPermsRead})
	stream := openSpacesStream(t, ts.URL, "liveKey")
	require.Equal(t, http.StatusOK, stream.resp.StatusCode)
	stream.next(3)

	// when
	fx.CloseStreams()

	// then: the body ends, with no closing event
	stream.ended()
	fx.requireNoWatchers(t)
}

func TestKeyExpiryBoundary(t *testing.T) {
	// The middleware's half of the expiry boundary (the stream's half is
	// TestSpacesStreamExpiry in the v2 service): one predicate,
	// util.KeyExpired, serves both. A key is valid through its expiry second
	// and refused one second later.
	const expireAt = int64(1_800_000_000)
	entry := ApiSessionEntry{Token: "tok", AppName: "agent", Scope: model.AccountAuth_JsonAPI, ExpireAt: expireAt}

	t.Run("at the expiry second the request is served", func(t *testing.T) {
		// given
		fx := newV2ServerFixture(t)
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		fx.KeyToToken = map[string]ApiSessionEntry{"expiringKey": entry}
		fx.now = func() time.Time { return time.Unix(expireAt, 0).Add(999 * time.Millisecond) }

		// when
		w := serveWithKey(fx, http.MethodGet, "/v2/spaces", "expiringKey")

		// then
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Contains(t, fx.KeyToToken, "expiringKey")
	})

	t.Run("one second later it is refused as expired and evicted", func(t *testing.T) {
		// given
		fx := newV2ServerFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{"expiringKey": entry}
		fx.now = func() time.Time { return time.Unix(expireAt+1, 0) }
		gen := fx.evictGen

		// when
		w := serveWithKey(fx, http.MethodGet, "/v2/spaces", "expiringKey")

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "api key expired")
		assert.NotContains(t, fx.KeyToToken, "expiringKey")
		assert.Equal(t, gen+1, fx.evictGen, "an eviction is an eviction")
	})
}
