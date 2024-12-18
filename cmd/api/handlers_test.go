package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/mock_core"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service/mock_service"
)

type fixture struct {
	*ApiServer
	mwMock         *mock_service.MockClientCommandsServer
	mwInternalMock *mock_core.MockMiddlewareInternal
	router         *gin.Engine
}

func newFixture(t *testing.T) *fixture {
	mw := mock_service.NewMockClientCommandsServer(t)
	mwInternal := mock_core.NewMockMiddlewareInternal(t)
	apiServer := &ApiServer{mw: mw, mwInternal: mwInternal, router: gin.Default()}

	apiServer.router.POST("/auth/displayCode", apiServer.authDisplayCodeHandler)
	apiServer.router.GET("/auth/token", apiServer.authTokenHandler)

	return &fixture{
		ApiServer:      apiServer,
		mwMock:         mw,
		mwInternalMock: mwInternal,
		router:         apiServer.router,
	}
}

func TestApiServer_AuthDisplayCodeHandler(t *testing.T) {
	t.Run("successful challenge creation", func(t *testing.T) {
		fx := newFixture(t)

		fx.mwMock.On("AccountLocalLinkNewChallenge", mock.Anything, &pb.RpcAccountLocalLinkNewChallengeRequest{AppName: "api-test"}).
			Return(&pb.RpcAccountLocalLinkNewChallengeResponse{
				ChallengeId: "mocked-challenge-id",
				Error:       &pb.RpcAccountLocalLinkNewChallengeResponseError{Code: pb.RpcAccountLocalLinkNewChallengeResponseError_NULL},
			}).Once()

		req, _ := http.NewRequest("POST", "/auth/displayCode", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "mocked-challenge-id")
	})

	t.Run("failed challenge creation", func(t *testing.T) {
		fx := newFixture(t)

		// Mock middleware behavior
		fx.mwMock.On("AccountLocalLinkNewChallenge", mock.Anything, &pb.RpcAccountLocalLinkNewChallengeRequest{AppName: "api-test"}).
			Return(&pb.RpcAccountLocalLinkNewChallengeResponse{
				Error: &pb.RpcAccountLocalLinkNewChallengeResponseError{Code: pb.RpcAccountLocalLinkNewChallengeResponseError_UNKNOWN_ERROR},
			}).Once()

		req, _ := http.NewRequest("POST", "/auth/displayCode", nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestApiServer_AuthTokenHandler(t *testing.T) {
	t.Run("successful token retrieval", func(t *testing.T) {
		fx := newFixture(t)
		challengeId := "mocked-challenge-id"
		code := "mocked-code"

		// Mock middleware behavior
		fx.mwMock.On("AccountLocalLinkSolveChallenge", mock.Anything, &pb.RpcAccountLocalLinkSolveChallengeRequest{
			ChallengeId: challengeId,
			Answer:      code,
		}).
			Return(&pb.RpcAccountLocalLinkSolveChallengeResponse{
				SessionToken: "mocked-session-token",
				AppKey:       "mocked-app-key",
				Error:        &pb.RpcAccountLocalLinkSolveChallengeResponseError{Code: pb.RpcAccountLocalLinkSolveChallengeResponseError_NULL},
			}).Once()

		req, _ := http.NewRequest("GET", "/auth/token?challengeId="+challengeId+"&code="+code, nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "mocked-session-token")
		require.Contains(t, w.Body.String(), "mocked-app-key")
	})

	t.Run("failed token retrieval", func(t *testing.T) {
		fx := newFixture(t)
		challengeId := "mocked-challenge-id"
		code := "mocked-code"

		fx.mwMock.On("AccountLocalLinkSolveChallenge", mock.Anything, &pb.RpcAccountLocalLinkSolveChallengeRequest{
			ChallengeId: challengeId,
			Answer:      code,
		}).
			Return(&pb.RpcAccountLocalLinkSolveChallengeResponse{
				Error: &pb.RpcAccountLocalLinkSolveChallengeResponseError{Code: pb.RpcAccountLocalLinkSolveChallengeResponseError_UNKNOWN_ERROR},
			}).Once()

		req, _ := http.NewRequest("GET", "/auth/token?challengeId="+challengeId+"&code="+code, nil)
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
