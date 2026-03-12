package application

import (
	"context"
	"errors"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api"
	"github.com/anyproto/anytype-heart/core/session"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestCreateSession(t *testing.T) {
	t.Run("with app key", func(t *testing.T) {
		t.Run("with not initialized app expect error", func(t *testing.T) {
			s := New()

			_, _, err := s.CreateSession(&pb.RpcWalletCreateSessionRequest{
				Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{
					AppKey: "appKey",
				},
			})

			require.Error(t, err)
		})
	})
}

// mockApiService is a stub for api.Service used in LinkLocalRevokeApp tests.
type mockApiService struct {
	revokedTokens []string
}

func (m *mockApiService) Name() string                                        { return api.CName }
func (m *mockApiService) Init(_ *app.App) error                               { return nil }
func (m *mockApiService) Run(_ context.Context) error                         { return nil }
func (m *mockApiService) Close(_ context.Context) error                       { return nil }
func (m *mockApiService) ReassignAddress(_ context.Context, _ string) error   { return nil }
func (m *mockApiService) RevokeToken(token string)                            { m.revokedTokens = append(m.revokedTokens, token) }

func TestLinkLocalRevokeApp(t *testing.T) {
	signingKey := []byte("test-signing-key-1234")

	t.Run("app not running", func(t *testing.T) {
		// given
		s := New()

		// when
		err := s.LinkLocalRevokeApp(&pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: "hash1"})

		// then
		require.ErrorIs(t, err, ErrApplicationIsNotRunning)
	})

	t.Run("wallet revoke fails", func(t *testing.T) {
		// given
		s := New()
		walletMock := mock_wallet.NewMockWallet(t)
		walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
		walletMock.EXPECT().Init(nil).Return(nil).Maybe()
		walletMock.EXPECT().RevokeAppLink("hash1").Return(errors.New("revoke failed"))

		a := new(app.App)
		a.Register(walletMock)
		s.app = a

		// when
		err := s.LinkLocalRevokeApp(&pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: "hash1"})

		// then
		require.EqualError(t, err, "revoke failed")
	})

	t.Run("revoke with no active session", func(t *testing.T) {
		// given
		s := New()
		walletMock := mock_wallet.NewMockWallet(t)
		walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
		walletMock.EXPECT().Init(nil).Return(nil).Maybe()
		walletMock.EXPECT().RevokeAppLink("hash1").Return(nil)

		a := new(app.App)
		a.Register(walletMock)
		s.app = a

		// when
		err := s.LinkLocalRevokeApp(&pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: "hash1"})

		// then
		require.NoError(t, err)
		assert.Empty(t, s.sessionsByAppHash)
	})

	t.Run("revoke with active session and api service", func(t *testing.T) {
		// given
		s := New()
		s.sessionSigningKey = signingKey

		sessions := session.New()
		token, err := sessions.StartSession(signingKey, model.AccountAuth_JsonAPI)
		require.NoError(t, err)
		s.sessions = sessions
		s.sessionsByAppHash["hash1"] = token

		walletMock := mock_wallet.NewMockWallet(t)
		walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
		walletMock.EXPECT().Init(nil).Return(nil).Maybe()
		walletMock.EXPECT().RevokeAppLink("hash1").Return(nil)

		apiMock := &mockApiService{}

		a := new(app.App)
		a.Register(walletMock)
		a.Register(apiMock)
		s.app = a

		// when
		err = s.LinkLocalRevokeApp(&pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: "hash1"})

		// then
		require.NoError(t, err)
		assert.Empty(t, s.sessionsByAppHash)
		require.Len(t, apiMock.revokedTokens, 1)
		assert.Equal(t, token, apiMock.revokedTokens[0])

		// session should be closed
		_, validateErr := sessions.ValidateToken(signingKey, token)
		require.Error(t, validateErr)
	})

	t.Run("revoke with active session without api service", func(t *testing.T) {
		// given
		s := New()
		s.sessionSigningKey = signingKey

		sessions := session.New()
		token, err := sessions.StartSession(signingKey, model.AccountAuth_JsonAPI)
		require.NoError(t, err)
		s.sessions = sessions
		s.sessionsByAppHash["hash1"] = token

		walletMock := mock_wallet.NewMockWallet(t)
		walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
		walletMock.EXPECT().Init(nil).Return(nil).Maybe()
		walletMock.EXPECT().RevokeAppLink("hash1").Return(nil)

		a := new(app.App)
		a.Register(walletMock)
		s.app = a

		// when
		err = s.LinkLocalRevokeApp(&pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: "hash1"})

		// then
		require.NoError(t, err)
		assert.Empty(t, s.sessionsByAppHash)

		// session should still be closed
		_, validateErr := sessions.ValidateToken(signingKey, token)
		require.Error(t, validateErr)
	})

	t.Run("revoke does not affect other sessions", func(t *testing.T) {
		// given
		s := New()
		s.sessionSigningKey = signingKey

		sessions := session.New()
		token1, err := sessions.StartSession(signingKey, model.AccountAuth_JsonAPI)
		require.NoError(t, err)
		token2, err := sessions.StartSession(signingKey, model.AccountAuth_JsonAPI)
		require.NoError(t, err)
		s.sessions = sessions
		s.sessionsByAppHash["hash1"] = token1
		s.sessionsByAppHash["hash2"] = token2

		walletMock := mock_wallet.NewMockWallet(t)
		walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
		walletMock.EXPECT().Init(nil).Return(nil).Maybe()
		walletMock.EXPECT().RevokeAppLink("hash1").Return(nil)

		apiMock := &mockApiService{}

		a := new(app.App)
		a.Register(walletMock)
		a.Register(apiMock)
		s.app = a

		// when
		err = s.LinkLocalRevokeApp(&pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: "hash1"})

		// then
		require.NoError(t, err)
		require.Len(t, s.sessionsByAppHash, 1)
		assert.Equal(t, token2, s.sessionsByAppHash["hash2"])

		// token1 session closed, token2 session still valid
		_, err = sessions.ValidateToken(signingKey, token1)
		require.Error(t, err)
		_, err = sessions.ValidateToken(signingKey, token2)
		require.NoError(t, err)

		// only token1 was revoked from API
		require.Len(t, apiMock.revokedTokens, 1)
		assert.Equal(t, token1, apiMock.revokedTokens[0])
	})
}
