package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
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
		V2Deps{Reader: readerMock, Creator: creatorMock, Mutator: mutatorMock, Store: store}, mockedListenAddr, []byte{}, []byte{})

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
		// given
		fx := newV2ServerFixture(t)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v2/spaces", nil)
		req.Host = localApiHost

		// when
		fx.Engine().ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("v2 validate responds through the shared auth", func(t *testing.T) {
		// given
		fx := newV2ServerFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{"validKey": {Token: "tok"}}
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

	t.Run("the idempotency middleware is wired on the edit routes", func(t *testing.T) {
		// the middleware itself is unit-tested, but its REGISTRATION is the
		// user-visible half of C8 on PATCH/PUT: dropping idempotencyMW from
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
			{"PUT", "/v2/spaces/space1/objects/obj1"},
			{"PATCH", "/v2/spaces/space1/types/task"},
			{"PATCH", "/v2/spaces/space1/properties/status"},
		} {
			t.Run(route.method+" "+route.path, func(t *testing.T) {
				fx := newV2ServerFixture(t)
				fx.KeyToToken = map[string]ApiSessionEntry{"validKey": {Token: "tok"}}
				fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

				req := httptest.NewRequest(route.method, route.path,
					strings.NewReader(strings.Repeat("x", maxV2RequestBody+1)))
				req.Host = localApiHost
				req.Header.Set("Authorization", "Bearer validKey")
				req.Header.Set(IdempotencyKeyHeader, "routekey1")
				w := httptest.NewRecorder()

				fx.Engine().ServeHTTP(w, req)

				require.Equal(t, http.StatusRequestEntityTooLarge, w.Code,
					"the idempotency middleware must be registered on this route")
				require.Contains(t, w.Body.String(), `"request_too_large"`)
			})
		}
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
				fx.KeyToToken = map[string]ApiSessionEntry{"validKey": {Token: "tok"}}
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
					req.Header.Set(IdempotencyKeyHeader, "searchkey1")
					w := httptest.NewRecorder()

					fx.Engine().ServeHTTP(w, req)

					require.Equal(t, http.StatusOK, w.Code)
					require.Empty(t, w.Header().Get("Idempotency-Replayed"),
						"a keyed search must never replay — search carries no idempotency middleware")
				}
			})
		}
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
			{"GET", "/v2/schemas/ops/replaceText"},
			{"PATCH", "/v2/spaces/space1/objects/obj1"},
			{"PUT", "/v2/spaces/space1/objects/obj1"},
			{"POST", "/v2/search"},
			{"POST", "/v2/spaces/space1/search"},
			{"GET", "/v2/spaces/space1/sets/set1/objects"},
			{"GET", "/v2/spaces/space1/sets/set1/views"},
			{"GET", "/v2/spaces/space1/collections/col1/objects"},
			{"GET", "/v2/spaces/space1/collections/col1/views"},
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
