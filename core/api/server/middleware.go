package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/didip/tollbooth/v8"
	"github.com/didip/tollbooth/v8/limiter"
	"github.com/gin-gonic/gin"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/filter"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/localorigin"
)

// ApiVersion is shared with the whoami body (util.ApiVersion): the header
// and the introspection mirror must report the same version.
const ApiVersion = util.ApiVersion

var log = logging.Logger("api-server")

var (
	ErrMissingAuthorizationHeader = errors.New("missing authorization header")
	ErrInvalidAuthorizationHeader = errors.New("invalid authorization header format")
	ErrInvalidApiKey              = errors.New("invalid api key")
	ErrApiKeyExpired              = errors.New("api key expired")
	ErrInsufficientKeyScope       = errors.New("api key scope does not allow json api access")
	ErrForbiddenOrigin            = errors.New("request origin is not allowed")
)

// ensureMetadataHeader is a middleware that ensures the metadata header is set.
func ensureMetadataHeader() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Anytype-Version", ApiVersion)
		c.Next()
	}
}

// ensureTrustedOrigin rejects browser-driven cross-origin requests.
//
// The API serves no CORS headers, so a site cannot read a response, but a
// cross-origin "simple" request still reaches the handler: gin's ShouldBindJSON
// parses the body whatever the Content-Type, so a form-style POST with
// Content-Type: text/plain skips the preflight and lands on the unauthenticated
// /v1/auth routes. This middleware stops the request instead.
func ensureTrustedOrigin(policy *localorigin.Policy) gin.HandlerFunc {
	return func(c *gin.Context) {
		if policy.AllowRequest(c.Request) {
			c.Next()
			return
		}
		log.Warnf("rejected api request from untrusted origin %q (host %q)", c.GetHeader("Origin"), c.Request.Host)
		apiErr := util.CodeToApiError(http.StatusForbidden, ErrForbiddenOrigin.Error())
		c.AbortWithStatusJSON(http.StatusForbidden, apiErr)
	}
}

// apiSessionContextKey is the gin-context key under which ensureAuthenticated
// stores the resolved ApiSessionEntry for downstream authorization middleware.
const apiSessionContextKey = "apiSession"

// ensureAuthenticated is a middleware that ensures the request is
// authenticated: bearer parse, key→session exchange and cache, and per-request
// expiry. It serves both route groups and decides only WHO is calling; whether
// the key's scope admits the JSON API is ensureJsonApiScope's concern, and
// that gate is /v2-only.
func (srv *Server) ensureAuthenticated(mw apicore.ClientCommands) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// RFC 6750 §3: a request with no credentials gets the bare
			// challenge, no error attribute. MCP clients are required to
			// parse WWW-Authenticate (spec rev 2025-06-18).
			c.Header(util.WwwAuthenticateHeader, util.BearerChallenge())
			apiErr := util.CodeToApiError(http.StatusUnauthorized, ErrMissingAuthorizationHeader.Error())
			c.AbortWithStatusJSON(http.StatusUnauthorized, apiErr)
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.Header(util.WwwAuthenticateHeader, util.BearerChallengeInvalidToken())
			apiErr := util.CodeToApiError(http.StatusUnauthorized, ErrInvalidAuthorizationHeader.Error())
			c.AbortWithStatusJSON(http.StatusUnauthorized, apiErr)
			return
		}
		key := strings.TrimPrefix(authHeader, "Bearer ")
		// An empty bearer value must never reach the session mint:
		// CreateSession treats an empty AppKey as "no app key" and falls
		// through to its mnemonic branch, whose success return is Full scope.
		if key == "" {
			c.Header(util.WwwAuthenticateHeader, util.BearerChallengeInvalidToken())
			apiErr := util.CodeToApiError(http.StatusUnauthorized, ErrInvalidAuthorizationHeader.Error())
			c.AbortWithStatusJSON(http.StatusUnauthorized, apiErr)
			return
		}

		// Validate the key - if the key exists in the KeyToToken map, it is considered valid.
		// Otherwise, attempt to create a new session using the key and add it to the map upon successful validation.
		// The eviction generation is snapshotted in the SAME critical section
		// as the cache read: the cache write below is conditional on it.
		srv.mu.Lock()
		apiSession, exists := srv.KeyToToken[key]
		mintGen := srv.evictGen
		srv.mu.Unlock()

		if !exists {
			response := mw.WalletCreateSession(context.Background(), &pb.RpcWalletCreateSessionRequest{Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{AppKey: key}})
			if response.Error.Code != pb.RpcWalletCreateSessionResponseError_NULL {
				// An expired key gets a distinct 401 so the client knows to
				// re-issue it instead of retrying the same key (H5: ExpireAt
				// must actually be enforced).
				message := ErrInvalidApiKey.Error()
				if response.Error.Code == pb.RpcWalletCreateSessionResponseError_APP_TOKEN_EXPIRED {
					message = ErrApiKeyExpired.Error()
				}
				c.Header(util.WwwAuthenticateHeader, util.BearerChallengeInvalidToken())
				apiErr := util.CodeToApiError(http.StatusUnauthorized, message)
				c.AbortWithStatusJSON(http.StatusUnauthorized, apiErr)
				return
			}
			apiSession = ApiSessionEntry{
				Token:     response.Token,
				AppName:   response.AppName,
				Scope:     response.AccountScope,
				ExpireAt:  response.AppExpireAt,
				Grant:     util.ApiGrantFromProto(response.Grant),
				KeyId:     response.AppHash,
				CreatedAt: response.AppCreatedAt,
			}

			// Cache only if no eviction swept while the mint was in flight. A
			// RevokeToken in that window (LinkLocalUpdateApp persists the new
			// grant FIRST, then sweeps) found no entry for this key, so the
			// entry just minted may carry the pre-edit grant. Serving THIS
			// request from it is equivalent to the request having completed
			// before the edit; CACHING it would make the stale grant permanent
			// — so on a generation mismatch the entry is dropped and the next
			// request re-mints against what the wallet holds then.
			srv.mu.Lock()
			if srv.evictGen == mintGen {
				srv.KeyToToken[key] = apiSession
			}
			srv.mu.Unlock()
		}

		// Expiry is enforced on every request, not only at session mint, so a
		// key that expires while cached stops working without a restart (H5).
		if apiSession.ExpireAt > 0 && time.Now().Unix() > apiSession.ExpireAt {
			// An eviction is an eviction: the generation bump keeps the rule
			// uniform (ANY eviction invalidates concurrent mints), so no
			// future eviction site can be the one that forgot it.
			srv.mu.Lock()
			delete(srv.KeyToToken, key)
			srv.evictGen++
			srv.mu.Unlock()
			c.Header(util.WwwAuthenticateHeader, util.BearerChallengeInvalidToken())
			apiErr := util.CodeToApiError(http.StatusUnauthorized, ErrApiKeyExpired.Error())
			c.AbortWithStatusJSON(http.StatusUnauthorized, apiErr)
			return
		}

		// Add token to request context for downstream services (subscriptions, events, etc.)
		c.Set("token", apiSession.Token)
		c.Set("apiAppName", apiSession.AppName)
		// The full resolved session rides the gin context so per-group
		// authorization middleware (the /v2-only scope gate) can read the
		// key's scope without a second lookup.
		c.Set(apiSessionContextKey, apiSession)
		// The gin context and the request context are separate carriers: the
		// analytics middleware reads only c.Request.Context(), so the app name
		// must ride there too — and so must the grant, because the v2
		// service layer (the fan-out constraint and the ensureSpace
		// backstop) reads it from the ctx its methods receive. The credential
		// description rides beside the grant for the same reason: whoami
		// derives its answer from these exact carriers.
		ctx := util.CtxWithApiAppName(c.Request.Context(), apiSession.AppName)
		ctx = util.CtxWithApiGrant(ctx, apiSession.Grant)
		ctx = util.CtxWithApiKeyInfo(ctx, util.ApiKeyInfo{
			Id:        apiSession.KeyId,
			Name:      apiSession.AppName,
			CreatedAt: apiSession.CreatedAt,
			ExpiresAt: apiSession.ExpireAt,
			Scope:     apiSession.Scope,
		})
		c.Request = c.Request.WithContext(ctx)
		srv.emitKeyStatusSignals(c, apiSession)
		c.Next()
	}
}

// emitKeyStatusSignals stamps the credential-status signal on every
// authenticated response. Anytype-Key-Status is ALWAYS present (legacy for
// nil-grant keys, scoped otherwise) so a client never reads absence as
// meaning anything; the notice sentence, the rel="deprecation" Link and the
// rate-limited log line accompany only the legacy value. Deliberately NOT
// RFC 9745 Deprecation/Sunset — see util.KeyStatusHeader.
func (srv *Server) emitKeyStatusSignals(c *gin.Context, apiSession ApiSessionEntry) {
	c.Header(util.KeyStatusHeader, util.KeyStatus(apiSession.Grant))
	if apiSession.Grant != nil {
		return
	}
	// The remedial signal addresses JSON-API keys only: a grant is only ever
	// valid on a JsonAPI-scope key (wallet.ValidateAppLinkGrant), so a
	// Limited (clipper) or Full credential cannot follow the "re-issue as a
	// scoped key" advice — and counting those keys would inflate the
	// legacy-usage metric the log line exists to feed before any sunset
	// decision.
	if apiSession.Scope != model.AccountAuth_JsonAPI {
		return
	}
	c.Header(util.NoticeHeader, util.LegacyKeyNotice)
	// Add, not Set: Link is a list-valued header and this signal must not
	// clobber a future pagination or policy link.
	c.Writer.Header().Add("Link", util.KeyDeprecationLink)
	// Info, not warn: nothing is wrong — legacy keys are grandfathered. The
	// line exists so we can tell whether anyone still presents them before a
	// sunset is ever contemplated.
	if srv.shouldLogLegacyKeyUse(apiSession.KeyId, time.Now()) {
		log.Infof("legacy unscoped api key in use: id %q, app %q", apiSession.KeyId, apiSession.AppName)
	}
}

// apiSessionFromContext resolves the session entry ensureAuthenticated
// stored, failing CLOSED on a miss: it writes the 401 challenge and aborts,
// then reports false. Authorization gates MUST go through it rather than
// reading the gin context directly — a gate that forgot the miss branch
// would decide on a zero-value ApiSessionEntry, whose Scope is Limited and
// whose Grant is nil, and a nil grant reads as "legacy key, pass".
func apiSessionFromContext(c *gin.Context) (ApiSessionEntry, bool) {
	value, _ := c.Get(apiSessionContextKey)
	apiSession, ok := value.(ApiSessionEntry)
	if !ok {
		c.Header(util.WwwAuthenticateHeader, util.BearerChallenge())
		apiErr := util.CodeToApiError(http.StatusUnauthorized, ErrMissingAuthorizationHeader.Error())
		c.AbortWithStatusJSON(http.StatusUnauthorized, apiErr)
		return ApiSessionEntry{}, false
	}
	return apiSession, true
}

// ensureJsonApiScope refuses keys whose scope does not admit the JSON API:
// only JsonAPI and Full pass, any other scope (e.g. the web clipper's
// Limited) gets 403, distinct from the 401 invalid-key path (H2: the gate
// must not be scope-blind).
//
// The gate is installed on the /v2 group only. Keys minted without a scope
// carry Limited (anytype-cli's CreateApp historically sent none), so gating
// /v1 would break those keys with no repair path but re-issuing — they are
// grandfathered on /v1, while /v2 has no shipped clients to break.
//
// Must run after ensureAuthenticated: it reads the resolved session entry
// from the gin context. Without one the request never authenticated, and the
// gate fails closed with the auth middleware's 401 rather than deciding
// authorization on nothing.
func ensureJsonApiScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiSession, ok := apiSessionFromContext(c)
		if !ok {
			return
		}

		if apiSession.Scope != model.AccountAuth_JsonAPI && apiSession.Scope != model.AccountAuth_Full {
			c.Header(util.WwwAuthenticateHeader, util.BearerChallengeInsufficientScope(""))
			apiErr := util.CodeToApiError(http.StatusForbidden, insufficientScopeMessage(apiSession))
			c.AbortWithStatusJSON(http.StatusForbidden, apiErr)
			return
		}
		c.Next()
	}
}

// ensureUngrantedKey refuses GRANTED keys on the /v1 group: a space grant
// can only be honored by /v2's gate, so serving the key on /v1 would give
// it unrestricted account-wide access the user explicitly narrowed away.
// The asymmetry is intended: a granted key is refused here while a legacy
// (nil-grant) key is served on /v1 exactly as today — grant PRESENCE
// decides, never key format (a legacy-format key can be granted in place
// and a new-format key can be unscoped).
//
// The 403 uses the v2 C6 envelope: the response's whole job is to steer the
// caller to /v2, so it speaks /v2's error language.
func ensureUngrantedKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiSession, ok := apiSessionFromContext(c)
		if !ok {
			return
		}
		if apiSession.Grant == nil {
			c.Next()
			return
		}
		c.Header(util.WwwAuthenticateHeader, util.BearerChallengeInsufficientScope(""))
		v2Err := v2model.V1NotAvailableForScopedKeys(fmt.Sprintf(
			"key %q carries a space grant (%s), which /v1 cannot honor — call the same route on /v2, or issue an unscoped key for /v1",
			apiSession.AppName, apiSession.Grant.Describe()))
		c.AbortWithStatusJSON(v2Err.Status, v2Err)
	}
}

// insufficientScopeMessage names the key and its actual scope so the 403
// reads as "re-issue the key with the right scope" rather than a transient
// permissions failure — Limited keys issued before the gate existed (e.g. by
// anytype-cli, which did not set a scope) hit this on every /v2 request while
// staying served on /v1.
func insufficientScopeMessage(entry ApiSessionEntry) string {
	return fmt.Sprintf("%s: key %q has %s scope, create a new api key with JsonAPI scope",
		ErrInsufficientKeyScope.Error(), entry.AppName, entry.Scope.String())
}

// ensureAnalyticsEvent is a middleware that ensures broadcasting an analytics event after a successful request.
func ensureAnalyticsEvent(code string, eventService apicore.EventService) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		status := c.Writer.Status()
		payload, err := util.NewAnalyticsEventForApi(c.Request.Context(), code, status)
		if err != nil {
			log.Errorf("Failed to create API analytics event: %v", err)
			return
		}

		eventService.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfPayloadBroadcast{
			PayloadBroadcast: &pb.EventPayloadBroadcast{
				Payload: payload,
			},
		}))
	}
}

// ensureRateLimit creates shared write-rate limiter middleware.
func ensureRateLimit(rate float64, burst int, isRateLimitDisabled bool) gin.HandlerFunc {
	lmt := tollbooth.NewLimiter(rate, nil)
	lmt.SetBurst(burst)
	lmt.SetIPLookup(limiter.IPLookup{
		Name:           "RemoteAddr",
		IndexFromRight: 0,
	})

	return func(c *gin.Context) {
		if isRateLimitDisabled {
			c.Next()
			return
		}
		if httpError := tollbooth.LimitByRequest(lmt, c.Writer, c.Request); httpError != nil {
			apiErr := util.CodeToApiError(httpError.StatusCode, httpError.Message)
			c.AbortWithStatusJSON(httpError.StatusCode, apiErr)
			return
		}
		c.Next()
	}
}

// ensureFilters is a middleware that ensures the filters are set in the context.
func (srv *Server) ensureFilters() gin.HandlerFunc {
	parser := filter.NewParser(srv.service)
	validator := filter.NewValidator(srv.service)

	return func(c *gin.Context) {
		spaceId := c.Param("space_id")

		// Parse filters from query parameters
		parsedFilters, err := parser.ParseQueryParams(c, spaceId)
		if err != nil {
			apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
			c.AbortWithStatusJSON(http.StatusBadRequest, apiErr)
			return
		}

		// Validate filters if we have a space context
		if spaceId != "" && parsedFilters != nil && len(parsedFilters.Filters) > 0 {
			if err := validator.ValidateFilters(spaceId, parsedFilters); err != nil {
				apiErr := util.CodeToApiError(http.StatusBadRequest, err.Error())
				c.AbortWithStatusJSON(http.StatusBadRequest, apiErr)
				return
			}
		}

		// Convert to dataview filters and set in context
		filters := parsedFilters.ToDataviewFilters()
		c.Set("filters", filters)
		c.Next()
	}
}

// ensureCacheInitialized initializes the API service caches on the first request.
func (srv *Server) ensureCacheInitialized() gin.HandlerFunc {
	return func(c *gin.Context) {
		srv.initOnce.Do(func() {
			if err := srv.service.InitializeAllCaches(); err != nil {
				log.Errorf("Failed to initialize API service caches: %v", err)
			}
		})

		c.Next()
	}
}
