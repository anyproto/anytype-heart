package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/subscription"
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
