package server

// keywatch.go is the server half of the spaces stream's key watch
// (APIV2_SPACE_STREAM.md §4): an open stream registers under the session
// token its request authenticated with, RevokeToken signals the watchers of
// the token it sweeps, and the stream re-reads its key through the same
// session mint the authentication middleware runs.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
	"github.com/anyproto/anytype-heart/pb"
)

// apiSessionGenContextKey is the gin-context key under which
// ensureAuthenticated leaves the eviction generation it observed in the
// critical section of its cache read. A key watch compares it with the
// current generation when it registers: unequal means an eviction — possibly
// an edit of this key's grant — happened after the grant the request carries
// was read and before the watch could hear about it.
const apiSessionGenContextKey = "apiSessionGen"

// keyWatch is v2service.KeyWatch for one open stream.
type keyWatch struct {
	srv *Server
	mw  apicore.ClientCommands
	// key is the bearer the request carried: the session mint needs the key
	// itself, and the request already held it.
	key string
	// token is the session token the request authenticated with. Every edit
	// and the revocation of the key sweep it (core/application tracks it
	// under the key's hash until the key is revoked), so it is what the
	// registration is keyed by.
	token   string
	state   v2service.KeyState
	changed chan struct{}
	closing <-chan struct{}

	closeOnce sync.Once
}

func (w *keyWatch) State() v2service.KeyState { return w.state }
func (w *keyWatch) Changed() <-chan struct{}  { return w.changed }
func (w *keyWatch) Closing() <-chan struct{}  { return w.closing }

// Current re-reads the key through the authentication middleware's own
// session mint, so the stream and the next request cannot disagree about the
// key's grant. It skips the cache read and caches what it minted under the
// middleware's eviction-generation rule.
func (w *keyWatch) Current(ctx context.Context) (v2service.KeyState, error) {
	w.srv.mu.Lock()
	gen := w.srv.evictGen
	w.srv.mu.Unlock()
	entry, code := w.srv.mintSession(ctx, w.mw, w.key, gen)
	switch code {
	case pb.RpcWalletCreateSessionResponseError_NULL:
		if util.KeyExpired(entry.ExpireAt, w.srv.clock()) {
			return v2service.KeyState{}, v2service.ErrKeyExpired
		}
		return v2service.KeyState{Grant: entry.Grant, ExpireAt: entry.ExpireAt}, nil
	case pb.RpcWalletCreateSessionResponseError_APP_TOKEN_NOT_FOUND_IN_THE_CURRENT_ACCOUNT:
		return v2service.KeyState{}, v2service.ErrKeyRevoked
	case pb.RpcWalletCreateSessionResponseError_APP_TOKEN_EXPIRED:
		return v2service.KeyState{}, v2service.ErrKeyExpired
	default:
		return v2service.KeyState{}, fmt.Errorf("mint a session for the key (%s): %w", code, v2service.ErrKeyUnverified)
	}
}

// Close unregisters the watch.
func (w *keyWatch) Close() {
	w.closeOnce.Do(func() {
		w.srv.mu.Lock()
		defer w.srv.mu.Unlock()
		set := w.srv.watchers[w.token]
		delete(set, w)
		if len(set) == 0 {
			delete(w.srv.watchers, w.token)
		}
	})
}

// newKeyWatch registers a watch for the key a request authenticated with.
// observedGen is the eviction generation the request's authentication saw.
//
// When an eviction happened since (the generations differ), the grant the
// request carries may predate an edit whose sweep ran before this
// registration and so signalled nobody: the key is re-read before the watch
// is handed out. The comparison is conservative — the generation counts every
// eviction of every key — which costs a redundant re-read, never a missed
// edit. A key that turns out revoked or expired is returned as the error.
func (srv *Server) newKeyWatch(ctx context.Context, mw apicore.ClientCommands, key string, entry ApiSessionEntry, observedGen uint64) (*keyWatch, error) {
	w := &keyWatch{
		srv:     srv,
		mw:      mw,
		key:     key,
		token:   entry.Token,
		state:   v2service.KeyState{Grant: entry.Grant, ExpireAt: entry.ExpireAt},
		changed: make(chan struct{}, 1),
	}
	srv.mu.Lock()
	if srv.watchers == nil {
		srv.watchers = map[string]map[*keyWatch]struct{}{}
	}
	if srv.watchers[w.token] == nil {
		srv.watchers[w.token] = map[*keyWatch]struct{}{}
	}
	srv.watchers[w.token][w] = struct{}{}
	w.closing = srv.streamsClosingLocked()
	stale := srv.evictGen != observedGen
	srv.mu.Unlock()

	if stale {
		state, err := w.Current(ctx)
		if err != nil {
			w.Close()
			return nil, fmt.Errorf("re-read the key after an eviction: %w", err)
		}
		w.state = state
	}
	return w, nil
}

// watchKey is RouteDeps.WatchKey: it builds the key watch of the request's
// own key. A key that was revoked or expired since the request authenticated
// is refused (refuseKey), and so is one that cannot be re-read at all: the
// reconnect's authentication decides.
func (srv *Server) watchKey(mw apicore.ClientCommands) func(c *gin.Context) (v2service.KeyWatch, bool) {
	return func(c *gin.Context) (v2service.KeyWatch, bool) {
		entry, ok := apiSessionFromContext(c)
		key := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || key == "" {
			refuseKey(c, v2service.ErrKeyUnverified)
			return nil, false
		}
		w, err := srv.newKeyWatch(c.Request.Context(), mw, key, entry, c.GetUint64(apiSessionGenContextKey))
		if err != nil {
			refuseKey(c, err)
			return nil, false
		}
		return w, true
	}
}

// refuseKey is RouteDeps.RefuseKey: the authentication middleware's own 401
// for a key a stream found it can no longer use, before the first byte —
// "api key expired" for an expired key, "invalid api key" for a revoked or
// unverifiable one, which is what the middleware answers any failed mint.
func refuseKey(c *gin.Context, err error) {
	message := ErrInvalidApiKey.Error()
	if errors.Is(err, v2service.ErrKeyExpired) {
		message = ErrApiKeyExpired.Error()
	}
	c.Header(util.WwwAuthenticateHeader, util.BearerChallengeInvalidToken())
	c.AbortWithStatusJSON(http.StatusUnauthorized, util.CodeToApiError(http.StatusUnauthorized, message))
}

// signalWatchersLocked tells every watch registered under token that its key
// changed. The send never blocks: the channel holds one signal, and a watch
// consumes it before it re-reads, so any number of sweeps leaves at least one
// re-read that sees the last state. Must be called with srv.mu held.
func (srv *Server) signalWatchersLocked(token string) {
	for w := range srv.watchers[token] {
		select {
		case w.changed <- struct{}{}:
		default:
		}
	}
}

// streamsClosingLocked returns the channel CloseStreams closes. Must be
// called with srv.mu held.
func (srv *Server) streamsClosingLocked() chan struct{} {
	if srv.streamsClosing == nil {
		srv.streamsClosing = make(chan struct{})
	}
	return srv.streamsClosing
}

// CloseStreams ends every stream that watches a key on this server. A server
// that is rebound or stopped is replaced by a new one, and RevokeToken
// reaches only that one: a stream left open on the old server would never
// hear its key revoked. http.Server.Shutdown does not interrupt an active
// connection, so the streams are told to end here. Safe to call more than
// once.
func (srv *Server) CloseStreams() {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	closing := srv.streamsClosingLocked()
	select {
	case <-closing:
	default:
		close(closing)
	}
}
