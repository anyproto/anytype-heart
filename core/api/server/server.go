package server

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// ApiSessionEntry is written once at session mint and evicted only by the
// per-request expiry check and RevokeToken. A surface that edits a live
// key's scope or grant in place must also evict that key's entry, or the
// edit takes effect only after a process restart — LinkLocalUpdateApp does
// exactly that (via RevokeToken), and that coupling is what makes an
// in-place grant NARROWING take effect on the very next request instead of
// silently serving the cached wider grant.
type ApiSessionEntry struct {
	Token   string `json:"token"`
	AppName string `json:"appName"`
	// Scope is the app link's scope, cached so the /v2-only scope gate
	// (ensureJsonApiScope: only JsonAPI and Full may use /v2) can decide
	// every request without a second key lookup.
	Scope model.AccountAuthLocalApiScope `json:"scope"`
	// ExpireAt is the app link's expiry unix timestamp (0 = never); checked
	// per request so a key that expires while cached stops working.
	ExpireAt int64 `json:"expire_at"`
	// Grant is the key's space grant; nil means an unscoped/legacy key.
	// Enforcement keys off the grant, never off the key string format:
	// ensureSpaceGrant constrains /v2 requests to it, and a granted key is
	// refused on /v1 (its grant cannot be honored there).
	Grant *util.ApiGrant `json:"grant,omitempty"`
	// KeyId is the app link's hash (the id ListApps shows) and CreatedAt its
	// creation unix timestamp — cached for whoami and the legacy-key log
	// line; neither is an authorization input.
	KeyId     string `json:"key_id,omitempty"`
	CreatedAt int64  `json:"created_at,omitempty"`
}

// Server wraps the HTTP server and service logic.
type Server struct {
	engine    *gin.Engine
	service   *service.Service
	v2Service *v2service.V2Service
	// v2CreateDisabled skips the Phase-2 create routes when no creator
	// dependency was provided (read-only construction, e.g. in tests).
	v2CreateDisabled bool
	// v2EditDisabled skips the Phase-3 edit routes when no mutator
	// dependency was provided.
	v2EditDisabled bool
	chatSubSvc     apicore.ChatSubscriptionService
	// docs holds both generated OpenAPI documents. NewRouter still takes v1's
	// bytes as parameters (its signature is what the route-conformance tests
	// call), so only the v2 pair is read from here.
	docs OpenApiDocs

	mu         sync.Mutex
	KeyToToken map[string]ApiSessionEntry // appKey -> token
	// legacyKeyLogSeen records when each legacy key's usage was last logged,
	// keyed by the key id (app hash). The log line exists to tell US whether
	// anyone still presents legacy keys before a sunset is ever contemplated
	// — once per key per process start, re-armed hourly, is enough signal
	// and cannot flood the log on an agent's request loop.
	legacyKeyLogSeen map[string]time.Time
	// evictGen counts cache evictions (RevokeToken sweeps and the
	// per-request expiry delete). ensureAuthenticated snapshots it before a
	// session mint and caches the minted entry only if no eviction happened
	// in between: a RevokeToken racing a mint can only sweep entries that
	// EXIST, so without the check a grant edit landing mid-mint would be
	// swept past and the mint would then cache the pre-edit grant — with
	// nothing left to evict it, ever (cached entries are re-validated only
	// against ExpireAt).
	evictGen uint64

	initOnce sync.Once
}

// V2Deps carries the API v2 dependencies (APIV2.md §8: live smartblock
// reads + objectstore-backed lists/resolvers, plus the Phase-2 create path
// and the Phase-3 edit path). With Reader or Store nil, the /v2 route group
// is not registered — v1 keeps working standalone. With Creator nil, only
// the read surface registers; with Mutator nil, the edit routes are skipped.
type V2Deps struct {
	Reader  apicore.ObjectReader
	Creator apicore.ObjectCreator
	Mutator apicore.ObjectMutator
	Store   objectstore.ObjectStore
	// AccountId is the caller's account identity, used by Phase 4's
	// stored-view placeholder substitution (`_filter_template_2_` → the
	// caller's participant id). Empty degrades the placeholder to a warning.
	AccountId string
}

// OpenApiDocs carries the generated OpenAPI documents, one pair per API
// version (core/api/docs/v1, core/api/docs/v2). They are served verbatim; see
// registerDocumentationRoutes for the paths.
type OpenApiDocs struct {
	V1YAML []byte
	V1JSON []byte
	V2YAML []byte
	V2JSON []byte
}

// NewServer constructs a new Server with the default config and sets up the routes.
func NewServer(mw apicore.ClientCommands, accountService apicore.AccountService, eventService apicore.EventService, crossSpaceSubService apicore.CrossSpaceSubscriptionService, chatSubSvc apicore.ChatSubscriptionService, fileObjectService apicore.FileObjectService, v2Deps V2Deps, apiListenAddr string, docs OpenApiDocs) *Server {
	techSpaceId, err := getTechSpaceId(accountService)
	if err != nil {
		panic(err)
	}

	apiBaseUrl := buildApiBaseUrl(apiListenAddr)
	s := &Server{
		service:    service.NewService(mw, fileObjectService, apiBaseUrl, techSpaceId, crossSpaceSubService),
		chatSubSvc: chatSubSvc,
		docs:       docs,
	}
	if v2Deps.Reader != nil && v2Deps.Store != nil {
		s.v2Service = v2service.NewV2Service(mw, v2Deps.Reader, v2Deps.Creator, v2Deps.Mutator, v2Deps.Store, techSpaceId, v2Deps.AccountId)
		s.v2CreateDisabled = v2Deps.Creator == nil
		s.v2EditDisabled = v2Deps.Mutator == nil
	}
	s.engine = s.NewRouter(mw, eventService, docs.V1YAML, docs.V1JSON)
	s.KeyToToken = make(map[string]ApiSessionEntry)
	s.legacyKeyLogSeen = make(map[string]time.Time)

	return s
}

// legacyKeyLogInterval re-arms the per-key legacy-usage log line: the first
// request after process start logs, later requests stay silent for an hour.
const legacyKeyLogInterval = time.Hour

// shouldLogLegacyKeyUse reports whether this legacy-key request is the one
// that logs, and arms the limiter. Keyed by key id so two legacy keys each
// get their own line.
func (srv *Server) shouldLogLegacyKeyUse(keyId string, now time.Time) bool {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if last, seen := srv.legacyKeyLogSeen[keyId]; seen && now.Sub(last) < legacyKeyLogInterval {
		return false
	}
	srv.legacyKeyLogSeen[keyId] = now
	return true
}

// getTechSpaceId retrieves the tech space ID from the account service.
func getTechSpaceId(accountService apicore.AccountService) (techSpaceId string, err error) {
	accountInfo, err := accountService.GetInfo(context.Background())
	if err != nil {
		return "", err
	}
	return accountInfo.TechSpaceId, nil
}

// buildApiBaseUrl turns the API listen address into a fully-qualified base URL
// (e.g. "127.0.0.1:31009" -> "http://127.0.0.1:31009"). Wildcard hosts are
// rewritten to 127.0.0.1 so the URL is dialable from clients.
func buildApiBaseUrl(listenAddr string) string {
	addr := listenAddr
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	} else if strings.HasPrefix(addr, "0.0.0.0:") {
		addr = "127.0.0.1:" + strings.TrimPrefix(addr, "0.0.0.0:")
	}
	return "http://" + addr
}

// Stop the service to clean up caches and subscriptions
func (srv *Server) Stop() {
	srv.service.Stop()
}

// RevokeToken removes EVERY cached API key entry carrying the given session
// token — one session token can back several cached keys, and revocation
// must not leave any of them usable (H4: revocation must be complete).
//
// The generation bump is unconditional, matching no entry included: that is
// precisely the racing case, where the entry this revocation targets is
// still mid-mint and does not exist yet — the bump is what stops the mint
// from caching it afterwards.
func (srv *Server) RevokeToken(token string) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.evictGen++
	for key, entry := range srv.KeyToToken {
		if entry.Token == token {
			delete(srv.KeyToToken, key)
		}
	}
}

// Engine returns the underlying gin.Engine.
func (srv *Server) Engine() *gin.Engine {
	return srv.engine
}
