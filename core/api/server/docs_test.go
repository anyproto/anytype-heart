package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// newDocsFixture builds a server carrying distinguishable bytes for each of
// the four generated documents, so a route wired to the wrong document fails
// on the body rather than on the status code.
func newDocsFixture(t *testing.T) *Server {
	mwMock := mock_apicore.NewMockClientCommands(t)
	accountMock := mock_apicore.NewMockAccountService(t)
	eventMock := mock_apicore.NewMockEventService(t)
	crossSpaceSubService := mock_apicore.NewMockCrossSpaceSubscriptionService(t)
	chatSubService := mock_apicore.NewMockChatSubscriptionService(t)
	fileObjectMock := mock_apicore.NewMockFileObjectService(t)

	crossSpaceSubService.On("Subscribe", mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{}, nil).Maybe()
	accountMock.On("GetInfo", mock.Anything).Return(&model.AccountInfo{TechSpaceId: mockedTechSpaceId}, nil).Once()

	return NewServer(mwMock, accountMock, eventMock, crossSpaceSubService, chatSubService, fileObjectMock,
		V2Deps{}, mockedListenAddr, OpenApiDocs{
			V1YAML: []byte("v1-yaml"),
			V1JSON: []byte(`{"doc":"v1"}`),
			V2YAML: []byte("v2-yaml"),
			V2JSON: []byte(`{"doc":"v2"}`),
		})
}

func TestDocumentationRoutes(t *testing.T) {
	// One document per API version (core/api/docs/v1, core/api/docs/v2), and
	// the unversioned /docs/* alias still answers with v1 — it is the path
	// developers.anytype.io and existing integrations use, so repointing it at
	// v2 would break them silently.
	for _, tc := range []struct {
		path        string
		contentType string
		body        string
	}{
		{"/docs/openapi.yaml", "application/x-yaml", "v1-yaml"},
		{"/docs/openapi.json", "application/json", `{"doc":"v1"}`},
		{"/v1/docs/openapi.yaml", "application/x-yaml", "v1-yaml"},
		{"/v1/docs/openapi.json", "application/json", `{"doc":"v1"}`},
		{"/v2/docs/openapi.yaml", "application/x-yaml", "v2-yaml"},
		{"/v2/docs/openapi.json", "application/json", `{"doc":"v2"}`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			// given: no Authorization header — the documents are served ahead
			// of the authenticated /v1 and /v2 groups, as /docs/* always was
			srv := newDocsFixture(t)
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Host = localApiHost
			w := httptest.NewRecorder()

			// when
			srv.Engine().ServeHTTP(w, req)

			// then
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.body, w.Body.String())
			require.Contains(t, w.Header().Get("Content-Type"), tc.contentType)
		})
	}
}
