package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

func TestRouter_Unauthenticated(t *testing.T) {
	t.Run("GET /v1/spaces without auth returns 401", func(t *testing.T) {
		// given
		fx := newFixture(t)
		engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/spaces", nil)

		// when
		engine.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestRouter_AuthRoute(t *testing.T) {
	t.Run("POST /v1/auth/token is accessible without auth", func(t *testing.T) {
		// given
		fx := newFixture(t)
		engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/auth/token", nil)

		// when
		engine.ServeHTTP(w, req)

		// then
		require.NotEqual(t, http.StatusUnauthorized, w.Code)
	})
}

func TestRouter_MetadataHeader(t *testing.T) {
	t.Run("Response includes Anytype-Version header", func(t *testing.T) {
		// given
		fx := newFixture(t)
		engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
		fx.KeyToToken = map[string]ApiSessionEntry{"validKey": {Token: "dummyToken", AppName: "dummyApp"}}
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}, nil).Once()
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/spaces", nil)
		req.Header.Set("Authorization", "Bearer validKey")

		// when
		engine.ServeHTTP(w, req)

		// then
		require.Equal(t, "2025-11-08", w.Header().Get("Anytype-Version"))
	})
}
