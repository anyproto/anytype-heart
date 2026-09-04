package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pb"
)

// whoamiWith performs GET /v2/auth/whoami with the given bearer through
// the engine and returns the recorder.
func whoamiWith(fx *fixture, bearer string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v2/auth/whoami", nil)
	req.Host = localApiHost
	req.Header.Set("Authorization", "Bearer "+bearer)
	fx.Engine().ServeHTTP(w, req)
	return w
}

func TestInternalKey(t *testing.T) {
	t.Run("authenticates as a Full-scope unscoped session on /v2", func(t *testing.T) {
		// given
		fx := newV2ServerFixture(t)
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		// when
		w := whoamiWith(fx, fx.InternalKey())

		// then
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var got v2model.WhoamiResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, "full", got.Scope)
		assert.False(t, got.Grant.Scoped)
		assert.Equal(t, InternalAppName, got.Key.Name)
		assert.Equal(t, internalKeyId, got.Key.Id)
		assert.Nil(t, got.Key.ExpiresAt, "never expires")
		// the entry is the process itself, never a cached key
		assert.Empty(t, fx.KeyToToken)
	})

	t.Run("survives every RevokeToken sweep", func(t *testing.T) {
		// given
		fx := newV2ServerFixture(t)
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		fx.RevokeToken(fx.internalSession.Token)
		fx.RevokeToken("some-other-token")

		// when
		w := whoamiWith(fx, fx.InternalKey())

		// then
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("a foreign key still goes through the mint path", func(t *testing.T) {
		// given: the wallet rejects the key
		fx := newV2ServerFixture(t)
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		fx.mwMock.On("WalletCreateSession", mock.Anything, mock.Anything).Return(&pb.RpcWalletCreateSessionResponse{
			Error: &pb.RpcWalletCreateSessionResponseError{Code: pb.RpcWalletCreateSessionResponseError_BAD_INPUT},
		}).Once()

		// when
		w := whoamiWith(fx, "not-the-internal-key")

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("two servers mint different keys", func(t *testing.T) {
		// given
		a := newV2ServerFixture(t)
		b := newV2ServerFixture(t)

		// then
		assert.Len(t, a.InternalKey(), 64, "32 random bytes, hex")
		assert.NotEqual(t, a.InternalKey(), b.InternalKey())
		assert.NotEqual(t, a.internalSession.Token, b.internalSession.Token)
	})
}
