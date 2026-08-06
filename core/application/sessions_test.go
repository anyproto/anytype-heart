package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/session"
	walletComp "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const testAppHash = "hash1"

// newAppKeyService wires a Service with a mock wallet whose ReadAppLink
// resolves appKey to a JsonAPI app link, so CreateSession's appKey branch works.
func newAppKeyService(t *testing.T, signingKey []byte, appKey string) (*Service, *mock_wallet.MockWallet) {
	t.Helper()
	s := New()
	s.sessionSigningKey = signingKey

	accountKeys, err := accountdata.NewRandom()
	require.NoError(t, err)

	walletMock := mock_wallet.NewMockWallet(t)
	walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
	walletMock.EXPECT().Init(nil).Return(nil).Maybe()
	walletMock.EXPECT().Account().Return(accountKeys).Maybe()
	walletMock.EXPECT().ReadAppLink(appKey).Return(&walletComp.AppLinkInfo{
		AppHash: testAppHash,
		AppName: "test-app",
		Scope:   int(model.AccountAuth_JsonAPI),
	}, nil).Maybe()

	a := new(app.App)
	a.Register(walletMock)
	s.app = a
	return s, walletMock
}

func appKeyRequest(appKey string) *pb.RpcWalletCreateSessionRequest {
	return &pb.RpcWalletCreateSessionRequest{
		Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{AppKey: appKey},
	}
}

func tokenRequest(token string) *pb.RpcWalletCreateSessionRequest {
	return &pb.RpcWalletCreateSessionRequest{
		Auth: &pb.RpcWalletCreateSessionRequestAuthOfToken{Token: token},
	}
}

func TestCreateSession(t *testing.T) {
	signingKey := []byte("test-signing-key-1234")

	t.Run("with app key", func(t *testing.T) {
		t.Run("with not initialized app expect error", func(t *testing.T) {
			s := New()

			_, err := s.CreateSession(appKeyRequest("appKey"))

			require.Error(t, err)
		})

		t.Run("returns the app link's scope, name and expiry", func(t *testing.T) {
			// given
			s, _ := newAppKeyService(t, signingKey, "appKey1")

			// when
			result, err := s.CreateSession(appKeyRequest("appKey1"))

			// then
			require.NoError(t, err)
			assert.NotEmpty(t, result.Token)
			assert.Equal(t, model.AccountAuth_JsonAPI, result.AccountScope)
			assert.Equal(t, "test-app", result.AppName)
			assert.Zero(t, result.AppExpireAt)

			scope, err := s.ValidateSessionToken(result.Token)
			require.NoError(t, err)
			assert.Equal(t, model.AccountAuth_JsonAPI, scope)
		})

		t.Run("every mint is tracked against the app hash", func(t *testing.T) {
			// given
			s, _ := newAppKeyService(t, signingKey, "appKey1")

			// when: the same key mints two sessions
			result1, err := s.CreateSession(appKeyRequest("appKey1"))
			require.NoError(t, err)
			result2, err := s.CreateSession(appKeyRequest("appKey1"))
			require.NoError(t, err)

			// then: both tokens are associated with the app hash
			s.appSessionsLock.Lock()
			defer s.appSessionsLock.Unlock()
			require.Len(t, s.sessionsByAppHash[testAppHash], 2)
			assert.Contains(t, s.sessionsByAppHash[testAppHash], result1.Token)
			assert.Contains(t, s.sessionsByAppHash[testAppHash], result2.Token)
			assert.Equal(t, testAppHash, s.appHashByToken[result1.Token])
			assert.Equal(t, testAppHash, s.appHashByToken[result2.Token])
		})
	})

	t.Run("with token", func(t *testing.T) {
		t.Run("derived token keeps scope and inherits the app hash", func(t *testing.T) {
			// given: a session minted from an app key
			s, _ := newAppKeyService(t, signingKey, "appKey1")
			minted, err := s.CreateSession(appKeyRequest("appKey1"))
			require.NoError(t, err)

			// when: a fresh session is derived from its token
			derived, err := s.CreateSession(tokenRequest(minted.Token))

			// then: same scope, and the derived token is tied to the app hash
			// so revocation reaches it (H4)
			require.NoError(t, err)
			assert.NotEqual(t, minted.Token, derived.Token)
			assert.Equal(t, model.AccountAuth_JsonAPI, derived.AccountScope)

			s.appSessionsLock.Lock()
			defer s.appSessionsLock.Unlock()
			assert.Equal(t, testAppHash, s.appHashByToken[derived.Token])
			assert.Contains(t, s.sessionsByAppHash[testAppHash], derived.Token)
		})

		t.Run("invalid token is refused", func(t *testing.T) {
			// given
			s := New()
			s.sessionSigningKey = signingKey

			// when
			_, err := s.CreateSession(tokenRequest("not-a-token"))

			// then
			require.Error(t, err)
		})
	})
}

func TestCloseSession(t *testing.T) {
	signingKey := []byte("test-signing-key-1234")

	t.Run("closing a session untracks its token", func(t *testing.T) {
		// given
		s, _ := newAppKeyService(t, signingKey, "appKey1")
		result, err := s.CreateSession(appKeyRequest("appKey1"))
		require.NoError(t, err)

		// when
		err = s.CloseSession(&pb.RpcWalletCloseSessionRequest{Token: result.Token})

		// then
		require.NoError(t, err)
		s.appSessionsLock.Lock()
		defer s.appSessionsLock.Unlock()
		assert.Empty(t, s.sessionsByAppHash)
		assert.Empty(t, s.appHashByToken)
	})
}

func TestLinkLocalCreateApp(t *testing.T) {
	t.Run("app not running", func(t *testing.T) {
		s := New()

		_, err := s.LinkLocalCreateApp(&pb.RpcAccountLocalLinkCreateAppRequest{
			App: &model.AccountAuthAppInfo{AppName: "x", Scope: model.AccountAuth_JsonAPI},
		})

		require.ErrorIs(t, err, ErrApplicationIsNotRunning)
	})

	t.Run("nil app info is bad input", func(t *testing.T) {
		// given
		s := New()
		s.app = new(app.App)

		// when
		_, err := s.LinkLocalCreateApp(&pb.RpcAccountLocalLinkCreateAppRequest{})

		// then
		require.ErrorIs(t, err, ErrBadInput)
	})

	t.Run("Full scope is refused, mirroring the challenge guard", func(t *testing.T) {
		// given: no PersistAppLink expectation — the guard must fire before
		// anything is written (H3)
		s := New()
		walletMock := mock_wallet.NewMockWallet(t)
		walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
		walletMock.EXPECT().Init(nil).Return(nil).Maybe()
		a := new(app.App)
		a.Register(walletMock)
		s.app = a

		// when
		_, err := s.LinkLocalCreateApp(&pb.RpcAccountLocalLinkCreateAppRequest{
			App: &model.AccountAuthAppInfo{AppName: "escalator", Scope: model.AccountAuth_Full},
		})

		// then
		require.ErrorIs(t, err, session.ErrInvalidScope)
	})

	t.Run("expireAt in the past or negative is bad input", func(t *testing.T) {
		// A duration or millisecond timestamp mistake must fail at issuance,
		// not mint a dead-on-arrival (or, negative, immortal) key. The mock
		// wallet has no PersistAppLink expectation: the guard must fire before
		// anything is written.
		tests := []struct {
			name     string
			expireAt int64
		}{
			{name: "past timestamp", expireAt: time.Now().Unix() - 60},
			{name: "negative value", expireAt: -1},
			{name: "duration mistaken for timestamp", expireAt: 3600},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// given
				s := New()
				walletMock := mock_wallet.NewMockWallet(t)
				walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
				walletMock.EXPECT().Init(nil).Return(nil).Maybe()
				a := new(app.App)
				a.Register(walletMock)
				s.app = a

				// when
				_, err := s.LinkLocalCreateApp(&pb.RpcAccountLocalLinkCreateAppRequest{
					App: &model.AccountAuthAppInfo{AppName: "clock-skewed", Scope: model.AccountAuth_JsonAPI, ExpireAt: tt.expireAt},
				})

				// then
				require.ErrorIs(t, err, ErrBadInput)
			})
		}
	})

	t.Run("JsonAPI scope persists with expiry passthrough", func(t *testing.T) {
		// given
		s := New()
		expireAt := time.Now().Unix() + 4242
		want := &walletComp.AppLinkInfo{AppHash: "h", AppKey: "k"}
		walletMock := mock_wallet.NewMockWallet(t)
		walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
		walletMock.EXPECT().Init(nil).Return(nil).Maybe()
		walletMock.EXPECT().PersistAppLink("cli", model.AccountAuth_JsonAPI, expireAt, (*walletComp.AppLinkGrant)(nil)).Return(want, nil)
		a := new(app.App)
		a.Register(walletMock)
		s.app = a

		// when
		appKey, err := s.LinkLocalCreateApp(&pb.RpcAccountLocalLinkCreateAppRequest{
			App: &model.AccountAuthAppInfo{AppName: "cli", Scope: model.AccountAuth_JsonAPI, ExpireAt: expireAt},
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, "k", appKey)
	})
}

// mockApiService is a stub for api.Service used in LinkLocalRevokeApp tests.
type mockApiService struct {
	revokedTokens []string
}

func (m *mockApiService) Name() string                                      { return api.CName }
func (m *mockApiService) Init(_ *app.App) error                             { return nil }
func (m *mockApiService) Run(_ context.Context) error                       { return nil }
func (m *mockApiService) Close(_ context.Context) error                     { return nil }
func (m *mockApiService) ReassignAddress(_ context.Context, _ string) error { return nil }
func (m *mockApiService) RevokeToken(token string)                          { m.revokedTokens = append(m.revokedTokens, token) }

func TestLinkLocalRevokeApp(t *testing.T) {
	signingKey := []byte("test-signing-key-1234")

	t.Run("app not running", func(t *testing.T) {
		// given
		s := New()

		// when
		err := s.LinkLocalRevokeApp(&pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: testAppHash})

		// then
		require.ErrorIs(t, err, ErrApplicationIsNotRunning)
	})

	t.Run("wallet revoke fails", func(t *testing.T) {
		// given
		s := New()
		walletMock := mock_wallet.NewMockWallet(t)
		walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
		walletMock.EXPECT().Init(nil).Return(nil).Maybe()
		walletMock.EXPECT().RevokeAppLink(testAppHash).Return(errors.New("revoke failed"))

		a := new(app.App)
		a.Register(walletMock)
		s.app = a

		// when
		err := s.LinkLocalRevokeApp(&pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: testAppHash})

		// then
		require.ErrorContains(t, err, "revoke failed")
	})

	t.Run("revoke with no active session", func(t *testing.T) {
		// given
		s := New()
		walletMock := mock_wallet.NewMockWallet(t)
		walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
		walletMock.EXPECT().Init(nil).Return(nil).Maybe()
		walletMock.EXPECT().RevokeAppLink(testAppHash).Return(nil)

		a := new(app.App)
		a.Register(walletMock)
		s.app = a

		// when
		err := s.LinkLocalRevokeApp(&pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: testAppHash})

		// then
		require.NoError(t, err)
		assert.Empty(t, s.sessionsByAppHash)
	})

	t.Run("revoke closes every session minted from the key", func(t *testing.T) {
		// given: two direct mints and one derived via the token branch — the
		// exact set H4 said revocation must reach
		s, walletMock := newAppKeyService(t, signingKey, "appKey1")
		walletMock.EXPECT().RevokeAppLink(testAppHash).Return(nil)
		apiMock := &mockApiService{}
		s.app.Register(apiMock)

		minted1, err := s.CreateSession(appKeyRequest("appKey1"))
		require.NoError(t, err)
		minted2, err := s.CreateSession(appKeyRequest("appKey1"))
		require.NoError(t, err)
		derived, err := s.CreateSession(tokenRequest(minted1.Token))
		require.NoError(t, err)

		// when
		err = s.LinkLocalRevokeApp(&pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: testAppHash})

		// then
		require.NoError(t, err)
		assert.Empty(t, s.sessionsByAppHash)
		assert.Empty(t, s.appHashByToken)

		// all three sessions are dead
		for _, token := range []string{minted1.Token, minted2.Token, derived.Token} {
			_, validateErr := s.ValidateSessionToken(token)
			require.Error(t, validateErr, "token must be dead after revoke")
		}

		// and all three were evicted from the API key cache
		assert.ElementsMatch(t,
			[]string{minted1.Token, minted2.Token, derived.Token},
			apiMock.revokedTokens)
	})

	t.Run("revoke with active session without api service", func(t *testing.T) {
		// given
		s, walletMock := newAppKeyService(t, signingKey, "appKey1")
		walletMock.EXPECT().RevokeAppLink(testAppHash).Return(nil)
		minted, err := s.CreateSession(appKeyRequest("appKey1"))
		require.NoError(t, err)

		// when
		err = s.LinkLocalRevokeApp(&pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: testAppHash})

		// then
		require.NoError(t, err)
		assert.Empty(t, s.sessionsByAppHash)

		// session should still be closed
		_, validateErr := s.ValidateSessionToken(minted.Token)
		require.Error(t, validateErr)
	})

	t.Run("revoke reaches sessions minted through the real wallet", func(t *testing.T) {
		// given: the REAL wallet, not the mock — the mock's ReadAppLink returns
		// an AppHash by construction, but the real read path must re-derive it
		// (AppLinkInfo.AppHash is json:"-"). If it ever comes back empty again,
		// sessions get tracked under "" while revocation looks up the real
		// hash, and a revoked API key keeps working until restart.
		s := New()
		s.sessionSigningKey = signingKey
		w := walletComp.NewWithRepoDirAndRandomKeys(t.TempDir())
		require.NoError(t, w.Init(nil))
		apiMock := &mockApiService{}
		a := new(app.App)
		a.Register(w)
		a.Register(apiMock)
		s.app = a

		appKey, err := s.LinkLocalCreateApp(&pb.RpcAccountLocalLinkCreateAppRequest{
			App: &model.AccountAuthAppInfo{AppName: "real-wallet-app", Scope: model.AccountAuth_JsonAPI},
		})
		require.NoError(t, err)

		minted, err := s.CreateSession(appKeyRequest(appKey))
		require.NoError(t, err)

		// the hash the desktop UI would revoke is the LISTED one — it must
		// match the hash the mint was tracked under
		apps, err := s.LinkLocalListApps()
		require.NoError(t, err)
		require.Len(t, apps, 1)
		require.True(t, apps[0].IsActive, "the listed hash must match the tracked one")

		// when
		err = s.LinkLocalRevokeApp(&pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: apps[0].AppHash})

		// then
		require.NoError(t, err)
		_, validateErr := s.ValidateSessionToken(minted.Token)
		require.Error(t, validateErr, "session minted from the revoked key must be dead")
		assert.Equal(t, []string{minted.Token}, apiMock.revokedTokens)
	})

	t.Run("concurrent token derive cannot outlive a revoke", func(t *testing.T) {
		// A client holding a doomed token can spam WalletCreateSession(token:)
		// while the key is being revoked, trying to launder it into a fresh
		// session the sweep cannot see. Mint and sweep are serialized, so after
		// the revoke EVERY session — minted before or during — must be dead and
		// both indexes empty. Run under -race this also exercises the locking.
		for i := 0; i < 500; i++ {
			s, walletMock := newAppKeyService(t, signingKey, "appKey1")
			walletMock.EXPECT().RevokeAppLink(testAppHash).Return(nil)
			apiMock := &mockApiService{}
			s.app.Register(apiMock)

			minted, err := s.CreateSession(appKeyRequest("appKey1"))
			require.NoError(t, err)

			var derivedTokens []string
			done := make(chan struct{})
			go func() {
				defer close(done)
				for {
					derived, deriveErr := s.CreateSession(tokenRequest(minted.Token))
					if deriveErr != nil {
						// the source token died — the revoke sweep won
						return
					}
					derivedTokens = append(derivedTokens, derived.Token)
				}
			}()

			err = s.LinkLocalRevokeApp(&pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: testAppHash})
			require.NoError(t, err)
			<-done

			for _, token := range append(derivedTokens, minted.Token) {
				_, validateErr := s.ValidateSessionToken(token)
				require.Error(t, validateErr, "no session may survive the revoke")
			}
			assert.ElementsMatch(t, append(derivedTokens, minted.Token), apiMock.revokedTokens,
				"every session, including ones derived mid-revoke, must be evicted from the API cache")

			s.appSessionsLock.Lock()
			assert.Empty(t, s.sessionsByAppHash)
			assert.Empty(t, s.appHashByToken)
			s.appSessionsLock.Unlock()
		}
	})

	t.Run("revoke does not affect other apps' sessions", func(t *testing.T) {
		// given
		s := New()
		s.sessionSigningKey = signingKey

		token1, err := s.sessions.StartSession(signingKey, model.AccountAuth_JsonAPI)
		require.NoError(t, err)
		token2, err := s.sessions.StartSession(signingKey, model.AccountAuth_JsonAPI)
		require.NoError(t, err)
		s.appSessionsLock.Lock()
		s.trackAppSessionLocked(testAppHash, token1)
		s.trackAppSessionLocked("hash2", token2)
		s.appSessionsLock.Unlock()

		walletMock := mock_wallet.NewMockWallet(t)
		walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
		walletMock.EXPECT().Init(nil).Return(nil).Maybe()
		walletMock.EXPECT().RevokeAppLink(testAppHash).Return(nil)

		apiMock := &mockApiService{}

		a := new(app.App)
		a.Register(walletMock)
		a.Register(apiMock)
		s.app = a

		// when
		err = s.LinkLocalRevokeApp(&pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: testAppHash})

		// then
		require.NoError(t, err)
		require.Len(t, s.sessionsByAppHash, 1)
		assert.Contains(t, s.sessionsByAppHash["hash2"], token2)

		// token1 session closed, token2 session still valid
		_, err = s.ValidateSessionToken(token1)
		require.Error(t, err)
		_, err = s.ValidateSessionToken(token2)
		require.NoError(t, err)

		// only token1 was revoked from API
		require.Len(t, apiMock.revokedTokens, 1)
		assert.Equal(t, token1, apiMock.revokedTokens[0])
	})
}

func testProtoGrant() *model.AccountAuthAppGrant {
	return &model.AccountAuthAppGrant{
		SpaceIds: []string{"space1", "space2"},
		Perm:     model.AccountAuthAppGrant_ReadWrite,
	}
}

func testWalletGrant() *walletComp.AppLinkGrant {
	return walletComp.AppLinkGrantFromProto(testProtoGrant())
}

func TestCreateSessionGrant(t *testing.T) {
	signingKey := []byte("test-signing-key-1234")

	t.Run("granted key returns its grant", func(t *testing.T) {
		// given: P1b's HTTP middleware learns the grant from this result — a
		// dropped grant here would make every gate built on it decoration
		s := New()
		s.sessionSigningKey = signingKey
		accountKeys, err := accountdata.NewRandom()
		require.NoError(t, err)
		walletMock := mock_wallet.NewMockWallet(t)
		walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
		walletMock.EXPECT().Init(nil).Return(nil).Maybe()
		walletMock.EXPECT().Account().Return(accountKeys).Maybe()
		walletMock.EXPECT().ReadAppLink("grantedKey").Return(&walletComp.AppLinkInfo{
			AppHash: testAppHash,
			AppName: "granted-app",
			Scope:   int(model.AccountAuth_JsonAPI),
			Grant:   testWalletGrant(),
		}, nil)
		a := new(app.App)
		a.Register(walletMock)
		s.app = a
		want := testProtoGrant()

		// when
		result, err := s.CreateSession(appKeyRequest("grantedKey"))

		// then
		require.NoError(t, err)
		assert.Equal(t, want, result.Grant)
	})

	t.Run("derived token carries no grant", func(t *testing.T) {
		// given: the derive branch has only the app HASH — it must never be
		// used for app-level grant decisions (see the branch comment)
		s, _ := newAppKeyService(t, signingKey, "appKey1")
		minted, err := s.CreateSession(appKeyRequest("appKey1"))
		require.NoError(t, err)

		// when
		derived, err := s.CreateSession(tokenRequest(minted.Token))

		// then
		require.NoError(t, err)
		assert.Nil(t, derived.Grant)
	})
}

func TestLinkLocalCreateAppGrant(t *testing.T) {
	t.Run("grant is converted and persisted", func(t *testing.T) {
		// given
		s := New()
		want := &walletComp.AppLinkInfo{AppHash: "h", AppKey: "k"}
		walletMock := mock_wallet.NewMockWallet(t)
		walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
		walletMock.EXPECT().Init(nil).Return(nil).Maybe()
		walletMock.EXPECT().PersistAppLink("scoped-cli", model.AccountAuth_JsonAPI, int64(0), testWalletGrant()).Return(want, nil)
		a := new(app.App)
		a.Register(walletMock)
		s.app = a

		// when
		appKey, err := s.LinkLocalCreateApp(&pb.RpcAccountLocalLinkCreateAppRequest{
			App: &model.AccountAuthAppInfo{AppName: "scoped-cli", Scope: model.AccountAuth_JsonAPI, Grant: testProtoGrant()},
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, "k", appKey)
	})

	t.Run("invalid grant is refused by the wallet", func(t *testing.T) {
		// given: validation lives at the persist point — the real wallet, not
		// a mock, so the rejection path is the shipped one
		s := New()
		w := walletComp.NewWithRepoDirAndRandomKeys(t.TempDir())
		require.NoError(t, w.Init(nil))
		a := new(app.App)
		a.Register(w)
		s.app = a

		// when: a grant with no spaces
		_, err := s.LinkLocalCreateApp(&pb.RpcAccountLocalLinkCreateAppRequest{
			App: &model.AccountAuthAppInfo{
				AppName: "no-spaces",
				Scope:   model.AccountAuth_JsonAPI,
				Grant:   &model.AccountAuthAppGrant{Perm: model.AccountAuthAppGrant_Read},
			},
		})

		// then
		require.ErrorIs(t, err, walletComp.ErrInvalidGrant)
	})

	t.Run("grant on a Limited key is refused", func(t *testing.T) {
		// given
		s := New()
		w := walletComp.NewWithRepoDirAndRandomKeys(t.TempDir())
		require.NoError(t, w.Init(nil))
		a := new(app.App)
		a.Register(w)
		s.app = a

		// when
		_, err := s.LinkLocalCreateApp(&pb.RpcAccountLocalLinkCreateAppRequest{
			App: &model.AccountAuthAppInfo{AppName: "clipper", Scope: model.AccountAuth_Limited, Grant: testProtoGrant()},
		})

		// then
		require.ErrorIs(t, err, walletComp.ErrInvalidGrant)
	})
}

func TestLinkLocalUpdateApp(t *testing.T) {
	signingKey := []byte("test-signing-key-1234")

	t.Run("app not running", func(t *testing.T) {
		// given
		s := New()

		// when
		err := s.LinkLocalUpdateApp(&pb.RpcAccountLocalLinkUpdateAppRequest{AppHash: testAppHash})

		// then
		require.ErrorIs(t, err, ErrApplicationIsNotRunning)
	})

	t.Run("empty app hash is bad input", func(t *testing.T) {
		// given
		s := New()
		s.app = new(app.App)

		// when
		err := s.LinkLocalUpdateApp(&pb.RpcAccountLocalLinkUpdateAppRequest{Grant: testProtoGrant()})

		// then
		require.ErrorIs(t, err, ErrBadInput)
	})

	t.Run("replacing a grant evicts the key's cached http entries but keeps sessions alive", func(t *testing.T) {
		// given: the API server caches key→session at mint; an in-place grant
		// edit must drop that cache (or the edit waits for a restart), while
		// the sessions stay valid — the key was edited, not revoked
		s, walletMock := newAppKeyService(t, signingKey, "appKey1")
		walletMock.EXPECT().UpdateAppLinkGrant(testAppHash, testWalletGrant()).Return(nil)
		apiMock := &mockApiService{}
		s.app.Register(apiMock)

		minted1, err := s.CreateSession(appKeyRequest("appKey1"))
		require.NoError(t, err)
		minted2, err := s.CreateSession(appKeyRequest("appKey1"))
		require.NoError(t, err)

		// when
		err = s.LinkLocalUpdateApp(&pb.RpcAccountLocalLinkUpdateAppRequest{
			AppHash: testAppHash,
			Grant:   testProtoGrant(),
		})

		// then
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{minted1.Token, minted2.Token}, apiMock.revokedTokens)
		for _, token := range []string{minted1.Token, minted2.Token} {
			_, validateErr := s.ValidateSessionToken(token)
			require.NoError(t, validateErr, "sessions must survive a grant edit")
		}
		s.appSessionsLock.Lock()
		defer s.appSessionsLock.Unlock()
		require.Len(t, s.sessionsByAppHash[testAppHash], 2, "tracking must survive a grant edit")
	})

	t.Run("repeated edits accumulate tracked sessions until revoke sweeps them", func(t *testing.T) {
		// given: this pins a DELIBERATE choice — every eviction makes the next
		// HTTP request mint a fresh session while the superseded one stays
		// alive (edited ≠ revoked, and the application layer cannot tell a
		// cache-minted session from one a gRPC client still holds). The growth
		// is bounded by the number of user-driven edits, every accumulated
		// token stays tracked, and revocation still closes all of them; if
		// P1b decides to close superseded cache sessions instead, this test is
		// the contract to renegotiate.
		s, walletMock := newAppKeyService(t, signingKey, "appKey1")
		walletMock.EXPECT().UpdateAppLinkGrant(testAppHash, testWalletGrant()).Return(nil).Times(2)
		walletMock.EXPECT().RevokeAppLink(testAppHash).Return(nil)
		apiMock := &mockApiService{}
		s.app.Register(apiMock)

		tokens := make([]string, 0, 3)
		mint := func() {
			minted, err := s.CreateSession(appKeyRequest("appKey1"))
			require.NoError(t, err)
			tokens = append(tokens, minted.Token)
		}
		edit := func() {
			require.NoError(t, s.LinkLocalUpdateApp(&pb.RpcAccountLocalLinkUpdateAppRequest{
				AppHash: testAppHash,
				Grant:   testProtoGrant(),
			}))
		}

		// when: mint, then two edit→re-mint cycles (each edit evicts the HTTP
		// cache, so the next request mints anew)
		mint()
		edit()
		mint()
		edit()
		mint()

		// then: one tracked session per mint, all alive
		s.appSessionsLock.Lock()
		require.Len(t, s.sessionsByAppHash[testAppHash], 3, "one tracked session per re-mint, none dropped")
		s.appSessionsLock.Unlock()
		for _, token := range tokens {
			_, validateErr := s.ValidateSessionToken(token)
			require.NoError(t, validateErr)
		}

		// and: revocation reaches every accumulated session (H4 completeness
		// is what makes the growth acceptable)
		require.NoError(t, s.LinkLocalRevokeApp(&pb.RpcAccountLocalLinkRevokeAppRequest{AppHash: testAppHash}))
		for _, token := range tokens {
			_, validateErr := s.ValidateSessionToken(token)
			require.Error(t, validateErr, "no session may survive the revoke")
		}
		s.appSessionsLock.Lock()
		defer s.appSessionsLock.Unlock()
		require.Empty(t, s.sessionsByAppHash[testAppHash])
	})

	t.Run("nil grant clears the scoping", func(t *testing.T) {
		// given
		s := New()
		walletMock := mock_wallet.NewMockWallet(t)
		walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
		walletMock.EXPECT().Init(nil).Return(nil).Maybe()
		walletMock.EXPECT().UpdateAppLinkGrant(testAppHash, (*walletComp.AppLinkGrant)(nil)).Return(nil)
		a := new(app.App)
		a.Register(walletMock)
		s.app = a

		// when
		err := s.LinkLocalUpdateApp(&pb.RpcAccountLocalLinkUpdateAppRequest{AppHash: testAppHash})

		// then
		require.NoError(t, err)
	})

	t.Run("wallet errors propagate", func(t *testing.T) {
		// given
		s := New()
		walletMock := mock_wallet.NewMockWallet(t)
		walletMock.EXPECT().Name().Return(walletComp.CName).Maybe()
		walletMock.EXPECT().Init(nil).Return(nil).Maybe()
		walletMock.EXPECT().UpdateAppLinkGrant(testAppHash, testWalletGrant()).Return(walletComp.ErrAppLinkNotFound)
		a := new(app.App)
		a.Register(walletMock)
		s.app = a

		// when
		err := s.LinkLocalUpdateApp(&pb.RpcAccountLocalLinkUpdateAppRequest{
			AppHash: testAppHash,
			Grant:   testProtoGrant(),
		})

		// then
		require.ErrorIs(t, err, walletComp.ErrAppLinkNotFound)
	})
}

func TestChallengeFlowGrant(t *testing.T) {
	signingKey := []byte("test-signing-key-1234")

	newChallengeService := func(t *testing.T) (*Service, walletComp.Wallet, *mock_event.MockSender) {
		t.Helper()
		s := New()
		s.sessionSigningKey = signingKey
		w := walletComp.NewWithRepoDirAndRandomKeys(t.TempDir())
		require.NoError(t, w.Init(nil))
		sender := mock_event.NewMockSender(t)
		s.eventSender = sender
		a := new(app.App)
		a.Register(w)
		s.app = a
		return s, w, sender
	}

	t.Run("solve challenge persists the requested grant", func(t *testing.T) {
		// given: the full pairing flow against the real wallet and the real
		// session service — this is how a CLI user gets a scoped key before
		// any consent picker exists
		s, w, sender := newChallengeService(t)
		requested := testProtoGrant()

		var challengeValue string
		var broadcastGrant *model.AccountAuthAppGrant
		sender.EXPECT().Broadcast(mock.Anything).Run(func(ev *pb.Event) {
			for _, msg := range ev.Messages {
				if ch := msg.GetAccountLinkChallenge(); ch != nil {
					challengeValue = ch.Challenge
					broadcastGrant = ch.RequestedGrant
				}
			}
		}).Return()

		challengeId, err := s.LinkLocalStartNewChallenge(model.AccountAuth_JsonAPI, &pb.EventAccountLinkChallengeClientInfo{Name: "cli-grant"}, requested)
		require.NoError(t, err)
		require.NotEmpty(t, challengeValue)
		// the future consent picker sees exactly what was requested
		require.Equal(t, requested, broadcastGrant)

		// when
		_, appKey, err := s.LinkLocalSolveChallenge(&pb.RpcAccountLocalLinkSolveChallengeRequest{
			ChallengeId: challengeId,
			Answer:      challengeValue,
		})

		// then: the persisted key carries the grant and the new key format
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(appKey, "anytype_"), "challenge-issued JsonAPI keys must mint the new format, got %q", appKey)
		link, err := w.ReadAppLink(appKey)
		require.NoError(t, err)
		assert.Equal(t, testWalletGrant(), link.Grant)

		apps, err := s.LinkLocalListApps()
		require.NoError(t, err)
		require.Len(t, apps, 1)
		assert.Equal(t, requested, apps[0].Grant)
	})

	t.Run("challenge without grant persists an unscoped key", func(t *testing.T) {
		// given
		s, w, sender := newChallengeService(t)
		var challengeValue string
		sender.EXPECT().Broadcast(mock.Anything).Run(func(ev *pb.Event) {
			for _, msg := range ev.Messages {
				if ch := msg.GetAccountLinkChallenge(); ch != nil {
					challengeValue = ch.Challenge
				}
			}
		}).Return()
		challengeId, err := s.LinkLocalStartNewChallenge(model.AccountAuth_JsonAPI, &pb.EventAccountLinkChallengeClientInfo{Name: "cli-plain"}, nil)
		require.NoError(t, err)

		// when
		_, appKey, err := s.LinkLocalSolveChallenge(&pb.RpcAccountLocalLinkSolveChallengeRequest{
			ChallengeId: challengeId,
			Answer:      challengeValue,
		})

		// then
		require.NoError(t, err)
		link, err := w.ReadAppLink(appKey)
		require.NoError(t, err)
		assert.Nil(t, link.Grant)
	})

	t.Run("invalid requested grant is refused before the challenge exists", func(t *testing.T) {
		// given: no Broadcast expectation — an invalid grant must fail before
		// the user is shown a code
		s, _, _ := newChallengeService(t)

		// when: a grant with no spaces
		_, err := s.LinkLocalStartNewChallenge(model.AccountAuth_JsonAPI, &pb.EventAccountLinkChallengeClientInfo{Name: "cli-bad"}, &model.AccountAuthAppGrant{Perm: model.AccountAuthAppGrant_Read})

		// then
		require.ErrorIs(t, err, walletComp.ErrInvalidGrant)
	})

	t.Run("requested grant on a Limited challenge is refused", func(t *testing.T) {
		// given
		s, _, _ := newChallengeService(t)

		// when
		_, err := s.LinkLocalStartNewChallenge(model.AccountAuth_Limited, &pb.EventAccountLinkChallengeClientInfo{Name: "clipper"}, testProtoGrant())

		// then
		require.ErrorIs(t, err, walletComp.ErrInvalidGrant)
	})
}
