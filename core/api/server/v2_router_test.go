package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/api/util"
	apiv2 "github.com/anyproto/anytype-heart/core/api/v2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// newV2Fixture builds a server with the v2 dependencies present, so the /v2
// route group is registered.
func newV2ServerFixture(t *testing.T) *fixture {
	mwMock := mock_apicore.NewMockClientCommands(t)
	accountMock := mock_apicore.NewMockAccountService(t)
	eventMock := mock_apicore.NewMockEventService(t)
	crossSpaceSubService := mock_apicore.NewMockCrossSpaceSubscriptionService(t)
	chatSubService := mock_apicore.NewMockChatSubscriptionService(t)
	fileObjectMock := mock_apicore.NewMockFileObjectService(t)
	readerMock := mock_apicore.NewMockObjectReader(t)
	store := objectstore.NewStoreFixture(t)

	creatorMock := mock_apicore.NewMockObjectCreator(t)
	mutatorMock := mock_apicore.NewMockObjectMutator(t)

	crossSpaceSubService.On("Subscribe", mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{}, nil).Maybe()
	accountMock.On("GetInfo", mock.Anything).Return(&model.AccountInfo{TechSpaceId: mockedTechSpaceId}, nil).Once()

	server := NewServer(mwMock, accountMock, eventMock, crossSpaceSubService, chatSubService, fileObjectMock,
		V2Deps{Reader: readerMock, Creator: creatorMock, Mutator: mutatorMock, Store: store}, mockedListenAddr, OpenApiDocs{})

	return &fixture{
		Server:               server,
		mwMock:               mwMock,
		accountMock:          accountMock,
		eventMock:            eventMock,
		crossSpaceSubService: crossSpaceSubService,
		chatSubService:       chatSubService,
		fileObjectMock:       fileObjectMock,
		objectStore:          store,
	}
}

func TestV2Routes(t *testing.T) {
	t.Run("v2 routes require auth", func(t *testing.T) {
		// given: no Authorization header — the answer must be
		// ensureAuthenticated's 401, never the scope gate's 403: the gate
		// runs after auth and must not see an unauthenticated request
		fx := newV2ServerFixture(t)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v2/spaces", nil)
		req.Host = localApiHost

		// when
		fx.Engine().ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
		expectedJSON, err := json.Marshal(util.CodeToApiError(http.StatusUnauthorized, ErrMissingAuthorizationHeader.Error()))
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), w.Body.String())
	})

	t.Run("v2 validate responds through the shared auth", func(t *testing.T) {
		// both scopes that admit the JSON API pass the /v2 scope gate
		for _, scope := range []model.AccountAuthLocalApiScope{
			model.AccountAuth_JsonAPI,
			model.AccountAuth_Full,
		} {
			t.Run(scope.String(), func(t *testing.T) {
				// given
				fx := newV2ServerFixture(t)
				fx.KeyToToken = map[string]ApiSessionEntry{"validKey": {Token: "tok", Scope: scope}}
				fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
				w := httptest.NewRecorder()
				req := httptest.NewRequest("POST", "/v2/validate", strings.NewReader(`{"version":1,"blocks":[]}`))
				req.Host = localApiHost
				req.Header.Set("Authorization", "Bearer validKey")

				// when
				fx.Engine().ServeHTTP(w, req)

				// then
				require.Equal(t, http.StatusOK, w.Code)
				require.Contains(t, w.Body.String(), `"issues":[]`)
			})
		}
	})

	t.Run("a Limited key is refused with 403 and the actionable body", func(t *testing.T) {
		// given: a valid but Limited (web-clipper) key — authenticated, yet
		// not authorized for /v2 (H2). 403, distinct from the 401
		// invalid-key path, naming the key, its scope, and the remedy.
		fx := newV2ServerFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{
			"limitedKey": {Token: "tok", AppName: "clipper", Scope: model.AccountAuth_Limited},
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v2/spaces", nil)
		req.Host = localApiHost
		req.Header.Set("Authorization", "Bearer limitedKey")

		// when
		fx.Engine().ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusForbidden, w.Code)
		wantMessage := `api key scope does not allow json api access: key "clipper" has Limited scope, create a new api key with JsonAPI scope`
		expectedJSON, err := json.Marshal(util.CodeToApiError(http.StatusForbidden, wantMessage))
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), w.Body.String())
	})

	t.Run("expired key gets the distinct 401 on /v2", func(t *testing.T) {
		// given: expiry is enforced in ensureAuthenticated for BOTH groups —
		// only the scope refusal is /v2-only (H5 did not move)
		fx := newV2ServerFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{
			"expiredKey": {Token: "tok", Scope: model.AccountAuth_JsonAPI, ExpireAt: time.Now().Unix() - 60},
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v2/spaces", nil)
		req.Host = localApiHost
		req.Header.Set("Authorization", "Bearer expiredKey")

		// when
		fx.Engine().ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
		expectedJSON, err := json.Marshal(util.CodeToApiError(http.StatusUnauthorized, ErrApiKeyExpired.Error()))
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), w.Body.String())
	})

	t.Run("every /v2 route carries the scope gate", func(t *testing.T) {
		// The gate is installed by group membership (v2.Use in
		// apiv2.RegisterRoutes), so a /v2 route registered on the engine
		// directly — the pattern the public docs routes already use — would
		// silently carry neither auth nor the gate. This walks the REAL
		// engine's route table with a cached Limited key: every /v2 route
		// must answer the gate's exact 403, except the explicit exempt list.
		fx := newV2ServerFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{
			"limitedKey": {Token: "tok", AppName: "clipper", Scope: model.AccountAuth_Limited},
		}

		wantMessage := `api key scope does not allow json api access: key "clipper" has Limited scope, create a new api key with JsonAPI scope`
		expectedJSON, err := json.Marshal(util.CodeToApiError(http.StatusForbidden, wantMessage))
		require.NoError(t, err)

		// The exempt set is DERIVED from the authorization registry — the one
		// place the auth-exempt fact is written down (a second hand-kept list
		// here could be edited into agreement with a hole). The registry
		// class itself is verified behaviorally by the grant conformance
		// walk: an auth-exempt route must answer without credentials, every
		// other /v2 route must 401. Growing the class is an API decision,
		// not a registration accident.
		exempt := map[string]bool{}
		for key, entry := range apiv2.RouteAuthzTable() {
			if entry.Global == apiv2.GlobalAuthExempt {
				exempt[key] = true
			}
		}
		require.Len(t, exempt, 2, "the auth-exempt class is the two public documents — growing it is an API decision")

		v2Routes := 0
		for _, route := range fx.Engine().Routes() {
			if !strings.HasPrefix(route.Path, "/v2/") {
				continue
			}
			v2Routes++

			// substitute path params so gin routes the probe to the handler
			segments := strings.Split(route.Path, "/")
			for i, segment := range segments {
				if strings.HasPrefix(segment, ":") || strings.HasPrefix(segment, "*") {
					segments[i] = "x"
				}
			}
			path := strings.Join(segments, "/")

			w := httptest.NewRecorder()
			req := httptest.NewRequest(route.Method, path, strings.NewReader(`{}`))
			req.Host = localApiHost
			req.Header.Set("Authorization", "Bearer limitedKey")
			fx.Engine().ServeHTTP(w, req)

			if exempt[route.Method+" "+route.Path] {
				require.NotEqual(t, http.StatusForbidden, w.Code,
					"%s %s is exempt: a public document must not sit behind the gate", route.Method, route.Path)
				continue
			}
			require.Equal(t, http.StatusForbidden, w.Code,
				"%s %s must refuse a Limited key — is it registered on the gated v2 group?", route.Method, route.Path)
			require.JSONEq(t, string(expectedJSON), w.Body.String(),
				"%s %s must answer with the scope gate's 403 body", route.Method, route.Path)
		}
		require.GreaterOrEqual(t, v2Routes, 40, "the walk must cover the /v2 surface, not a filtered-away remnant")
	})

	t.Run("the idempotency middleware is wired on the edit routes", func(t *testing.T) {
		// the middleware itself is unit-tested, but its REGISTRATION is the
		// user-visible half of C8 on PATCH: dropping idempotencyMW from
		// registerV2EditRoutes would silently stop replay on every edit route
		// while every other test stayed green. A replayed request is answered
		// from the store before auth runs, so the marker proves the middleware
		// is in the chain without needing a full edit to succeed.
		// The body-size guard lives in the idempotency middleware and fires
		// only for a keyed mutation, so a keyed oversized request answered
		// with 413 request_too_large proves the middleware is in that route's
		// chain — without needing the edit itself to succeed.
		for _, route := range []struct{ method, path string }{
			{"PATCH", "/v2/spaces/space1/objects/obj1"},
			{"PATCH", "/v2/spaces/space1/types/task"},
			{"PATCH", "/v2/spaces/space1/properties/status"},
			// Phase-7 space mutations: a retried space create without C8
			// duplicates an ENTIRE SPACE — the worst possible duplicate
			{"POST", "/v2/spaces"},
			{"PATCH", "/v2/spaces/space1"},
			// Phase-6 chat mutations: C8 on every one — a double-sent chat
			// message is user-visible damage. DELETE is the Phase-6 widening
			// of the middleware's method set.
			{"POST", "/v2/spaces/space1/chats"},
			// C8 is route-uniform: the type/property DELETEs carry the
			// middleware too — an agent sending Idempotency-Key on every
			// mutation must not get replay protection on one DELETE and
			// silently none on another (the review's C8 finding)
			{"DELETE", "/v2/spaces/space1/types/task"},
			{"DELETE", "/v2/spaces/space1/properties/status"},
			{"POST", "/v2/spaces/space1/chats/chat1/messages"},
			{"PATCH", "/v2/spaces/space1/chats/chat1/messages/msg1"},
			{"DELETE", "/v2/spaces/space1/chats/chat1/messages/msg1"},
			{"POST", "/v2/spaces/space1/chats/chat1/messages/msg1/reactions"},
			{"POST", "/v2/spaces/space1/chats/chat1/read"},
		} {
			t.Run(route.method+" "+route.path, func(t *testing.T) {
				fx := newV2ServerFixture(t)
				fx.KeyToToken = map[string]ApiSessionEntry{"validKey": {Token: "tok", Scope: model.AccountAuth_JsonAPI}}
				fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

				req := httptest.NewRequest(route.method, route.path,
					strings.NewReader(strings.Repeat("x", apiv2.MaxRequestBody+1)))
				req.Host = localApiHost
				req.Header.Set("Authorization", "Bearer validKey")
				req.Header.Set(apiv2.IdempotencyKeyHeader, "routekey1")
				w := httptest.NewRecorder()

				fx.Engine().ServeHTTP(w, req)

				require.Equal(t, http.StatusRequestEntityTooLarge, w.Code,
					"the idempotency middleware must be registered on this route")
				require.Contains(t, w.Body.String(), `"request_too_large"`)
			})
		}
	})

	t.Run("a keyed POST /v2/spaces retry replays — exactly one space is created", func(t *testing.T) {
		// POST /v2/spaces is the ONE v2 mutation whose route has no :space_id,
		// so its idempotency store key carries an empty space component — a
		// namespace no other replay test exercises end to end. It is also the
		// mutation where a duplicate is worst: an entire space, with no v2
		// delete to recover through. The .Once() on WorkspaceCreate is the
		// load-bearing assertion — a second RPC fails the mock.
		fx := newV2ServerFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{"validKey": {Token: "tok", Scope: model.AccountAuth_JsonAPI}}
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		fx.mwMock.EXPECT().WorkspaceCreate(mock.Anything, mock.Anything).
			Return(&pb.RpcWorkspaceCreateResponse{SpaceId: "newSpace1"}).Once()

		post := func() *httptest.ResponseRecorder {
			req := httptest.NewRequest("POST", "/v2/spaces", strings.NewReader(`{"name":"Research"}`))
			req.Host = localApiHost
			req.Header.Set("Authorization", "Bearer validKey")
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(apiv2.IdempotencyKeyHeader, "spacekey1")
			w := httptest.NewRecorder()
			fx.Engine().ServeHTTP(w, req)
			return w
		}

		first := post()
		second := post()

		require.Equal(t, http.StatusCreated, first.Code)
		require.Contains(t, first.Body.String(), `"newSpace1"`)
		require.Equal(t, http.StatusCreated, second.Code)
		require.Equal(t, first.Body.String(), second.Body.String(), "the stored 201 is replayed byte-identical")
		require.Equal(t, "true", second.Header().Get("Idempotency-Replayed"))
	})

	t.Run("search is a read: no idempotency middleware on the search routes", func(t *testing.T) {
		// Phase 4: search is exempt from Idempotency-Key — the middleware is
		// per-route and deliberately not attached. The proof is behavioral:
		// the SAME keyed request executed twice must run twice; were the
		// middleware in the chain, the second 2xx would be answered from its
		// store with an Idempotency-Replayed header. (An earlier form of this
		// test sent an unauthorized request and asserted 401 — vacuous, since
		// the group-level auth aborts before any route middleware runs.)
		for _, path := range []string{"/v2/search", "/v2/spaces/space1/search"} {
			t.Run(path, func(t *testing.T) {
				fx := newV2ServerFixture(t)
				fx.KeyToToken = map[string]ApiSessionEntry{"validKey": {Token: "tok", Scope: model.AccountAuth_JsonAPI}}
				fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
				// register space1 in the store fixture's tech space so the
				// space-scoped search resolves it (C2)
				fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
					bundle.RelationKeyId:             domain.String("spaceView_space1"),
					bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
					bundle.RelationKeyTargetSpaceId:  domain.String("space1"),
				}})

				for run := 0; run < 2; run++ {
					req := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
					req.Host = localApiHost
					req.Header.Set("Authorization", "Bearer validKey")
					req.Header.Set(apiv2.IdempotencyKeyHeader, "searchkey1")
					w := httptest.NewRecorder()

					fx.Engine().ServeHTTP(w, req)

					require.Equal(t, http.StatusOK, w.Code)
					require.Empty(t, w.Header().Get("Idempotency-Replayed"),
						"a keyed search must never replay — search carries no idempotency middleware")
				}
			})
		}
	})

	t.Run("chat messages read rejects offset with cursor steering", func(t *testing.T) {
		// the messages read is cursor-paged (after/before order ids); a
		// silently honored ?offset= would let an agent believe it pages by
		// offset while the RPC ignores it — reject with steering instead
		fx := newV2ServerFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{"validKey": {Token: "tok", Scope: model.AccountAuth_JsonAPI}}
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		req := httptest.NewRequest("GET", "/v2/spaces/space1/chats/chat1/messages?offset=5", nil)
		req.Host = localApiHost
		req.Header.Set("Authorization", "Bearer validKey")
		w := httptest.NewRecorder()

		fx.Engine().ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Contains(t, w.Body.String(), "cursor-paged")
		require.Contains(t, w.Body.String(), "after")
	})

	t.Run("v2 group absent without deps", func(t *testing.T) {
		// given: the plain fixture constructs NewServer with V2Deps{}
		fx := newFixture(t)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v2/spaces", nil)
		req.Host = localApiHost

		// when
		fx.Engine().ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("create routes are registered and require auth", func(t *testing.T) {
		// given
		fx := newV2ServerFixture(t)

		for _, route := range []struct{ method, path string }{
			{"GET", "/v2/spaces/space1"},
			{"POST", "/v2/spaces"},
			{"PATCH", "/v2/spaces/space1"},
			{"POST", "/v2/spaces/space1/objects"},
			{"POST", "/v2/spaces/space1/types"},
			{"PATCH", "/v2/spaces/space1/types/task"},
			{"DELETE", "/v2/spaces/space1/types/task"},
			{"POST", "/v2/spaces/space1/properties"},
			{"PATCH", "/v2/spaces/space1/properties/status"},
			{"DELETE", "/v2/spaces/space1/properties/status"},
			{"POST", "/v2/spaces/space1/sets"},
			{"POST", "/v2/spaces/space1/collections"},
			{"POST", "/v2/spaces/space1/templates"},
			{"POST", "/v2/spaces/space1/files"},
			{"GET", "/v2/schemas"},
			{"GET", "/v2/schemas/object"},
			{"GET", "/v2/schemas/ops/replace_text"},
			{"PATCH", "/v2/spaces/space1/objects/obj1"},
			{"POST", "/v2/search"},
			{"POST", "/v2/spaces/space1/search"},
			{"GET", "/v2/spaces/space1/sets/set1/objects"},
			{"GET", "/v2/spaces/space1/sets/set1/views"},
			{"GET", "/v2/spaces/space1/collections/col1/objects"},
			{"GET", "/v2/spaces/space1/collections/col1/views"},
			{"GET", "/v2/spaces/space1/chats"},
			{"POST", "/v2/spaces/space1/chats"},
			{"GET", "/v2/spaces/space1/chats/chat1/messages"},
			{"POST", "/v2/spaces/space1/chats/chat1/messages"},
			{"PATCH", "/v2/spaces/space1/chats/chat1/messages/msg1"},
			{"DELETE", "/v2/spaces/space1/chats/chat1/messages/msg1"},
			{"POST", "/v2/spaces/space1/chats/chat1/messages/msg1/reactions"},
			{"POST", "/v2/spaces/space1/chats/chat1/read"},
		} {
			// when
			w := httptest.NewRecorder()
			req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`))
			req.Host = localApiHost
			fx.Engine().ServeHTTP(w, req)

			// then: 401 (not 404) proves the route exists behind shared auth
			require.Equal(t, http.StatusUnauthorized, w.Code, "%s %s", route.method, route.path)
		}
	})
}
