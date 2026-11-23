package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/api/util"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	mockedTechSpaceId = "tech123"
	mockedListenAddr  = "127.0.0.1:31009"
)

type fixture struct {
	*Server
	mwMock               *mock_apicore.MockClientCommands
	accountMock          *mock_apicore.MockAccountService
	eventMock            *mock_apicore.MockEventService
	widgetsMock          *mock_apicore.MockWidgets
	crossSpaceSubService *mock_apicore.MockCrossSpaceSubscriptionService
	chatSubService       *mock_apicore.MockChatSubscriptionService
	fileObjectMock       *mock_apicore.MockFileObjectService
	// objectStore and readerMock are set by the v2 fixture only (nil on the
	// plain fixture).
	objectStore *objectstore.StoreFixture
	readerMock  *mock_apicore.MockObjectReader
	// objectSearchMock is the v2 fixture's live search port; a spaces stream
	// test makes it serve space views (withSpaceViews).
	objectSearchMock *mock_apicore.MockObjectSearchService
}

func newFixture(t *testing.T) *fixture {
	mwMock := mock_apicore.NewMockClientCommands(t)
	accountMock := mock_apicore.NewMockAccountService(t)
	eventMock := mock_apicore.NewMockEventService(t)
	crossSpaceSubService := mock_apicore.NewMockCrossSpaceSubscriptionService(t)
	chatSubService := mock_apicore.NewMockChatSubscriptionService(t)
	fileObjectMock := mock_apicore.NewMockFileObjectService(t)

	crossSpaceSubService.On("Subscribe", mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{}, nil).Maybe()
	accountMock.On("GetInfo", mock.Anything).Return(&model.AccountInfo{
		TechSpaceId: mockedTechSpaceId,
	}, nil).Once()

	server := NewServer(mwMock, accountMock, eventMock, crossSpaceSubService, chatSubService, fileObjectMock, V2Deps{}, mockedListenAddr, OpenApiDocs{})

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

func TestNewServer(t *testing.T) {
	t.Run("returns valid server", func(t *testing.T) {
		// when
		s := newFixture(t)

		// then
		require.NotNil(t, s)
		require.NotNil(t, s.service)
		require.NotNil(t, s.engine)
		require.NotNil(t, s.KeyToToken)
	})
}

func TestServer_GetTechSpaceId(t *testing.T) {
	t.Run("successful retrieval", func(t *testing.T) {
		// given
		mockAcc := mock_apicore.NewMockAccountService(t)
		mockAcc.On("GetInfo", mock.Anything).Return(&model.AccountInfo{
			TechSpaceId: mockedTechSpaceId,
		}, nil).Once()

		// when
		techSpaceId, err := getTechSpaceId(mockAcc)

		// then
		require.NoError(t, err)
		require.Equal(t, mockedTechSpaceId, techSpaceId)
	})

	t.Run("error retrieving account info", func(t *testing.T) {
		// given
		mockAcc := mock_apicore.NewMockAccountService(t)
		expectedError := errors.New("failed to get info")
		mockAcc.On("GetInfo", mock.Anything).Return(nil, expectedError)

		// when
		techSpaceId, err := getTechSpaceId(mockAcc)

		// then
		require.Error(t, err)
		require.Equal(t, "", techSpaceId)
	})
}

func TestBuildApiBaseUrl(t *testing.T) {
	tests := []struct {
		listenAddr string
		expected   string
	}{
		{"127.0.0.1:31009", "http://127.0.0.1:31009"},
		{":31009", "http://127.0.0.1:31009"},
		{"0.0.0.0:31009", "http://127.0.0.1:31009"},
		{"localhost:31009", "http://localhost:31009"},
	}

	for _, tt := range tests {
		t.Run(tt.listenAddr, func(t *testing.T) {
			require.Equal(t, tt.expected, buildApiBaseUrl(tt.listenAddr))
		})
	}
}

func TestServer_RevokeToken(t *testing.T) {
	t.Run("evicts every cache entry carrying the token", func(t *testing.T) {
		// given: two keys mapping to the same session token plus an unrelated
		// key — revocation must evict every entry carrying the token, not just
		// the first match (H4: revocation must be complete)
		s := newFixture(t)
		s.KeyToToken = map[string]ApiSessionEntry{
			"key1":  {Token: "revoked-token"},
			"key2":  {Token: "revoked-token"},
			"other": {Token: "other-token"},
		}

		// when
		s.RevokeToken("revoked-token")

		// then
		want := map[string]ApiSessionEntry{
			"other": {Token: "other-token"},
		}
		require.Equal(t, want, s.KeyToToken)
	})

	t.Run("no-op when the token is not cached", func(t *testing.T) {
		// given
		s := newFixture(t)
		s.KeyToToken = map[string]ApiSessionEntry{
			"key1": {Token: "token1"},
		}

		// when
		s.RevokeToken("unknown-token")

		// then
		require.Len(t, s.KeyToToken, 1)
	})
}

func TestServer_Engine(t *testing.T) {
	t.Run("Engine returns same engine instance", func(t *testing.T) {
		// given
		s := newFixture(t)

		// when
		engine := s.Engine()

		// then
		require.Equal(t, s.engine, engine)
	})
}

// signalled reports whether a change signal is pending on the watch, and
// consumes it.
func signalled(w *keyWatch) bool {
	select {
	case <-w.Changed():
		return true
	default:
		return false
	}
}

func closing(w *keyWatch) bool {
	select {
	case <-w.Closing():
		return true
	default:
		return false
	}
}

func TestKeyWatchRegistration(t *testing.T) {
	ctx := context.Background()
	entry := ApiSessionEntry{
		Token:    "tok",
		Scope:    model.AccountAuth_JsonAPI,
		Grant:    &util.ApiGrant{Spaces: []string{"spaceA", "spaceB"}, Perms: util.GrantPermsRead},
		ExpireAt: 42,
	}
	mintRequest := &pb.RpcWalletCreateSessionRequest{Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{AppKey: "watchedKey"}}
	narrowed := &pb.RpcWalletCreateSessionResponse{
		Token:        "tok2",
		AccountScope: model.AccountAuth_JsonAPI,
		AppExpireAt:  77,
		Grant:        &model.AccountAuthAppGrant{SpaceIds: []string{"spaceA"}, Perm: model.AccountAuthAppGrant_Read},
		Error:        &pb.RpcWalletCreateSessionResponseError{Code: pb.RpcWalletCreateSessionResponseError_NULL},
	}
	refused := func(code pb.RpcWalletCreateSessionResponseErrorCode) *pb.RpcWalletCreateSessionResponse {
		return &pb.RpcWalletCreateSessionResponse{Error: &pb.RpcWalletCreateSessionResponseError{Code: code}}
	}
	// the clock stands before every expiry the cases use (42 and 77)
	newFixture := func(t *testing.T) *fixture {
		fx := newFixture(t)
		fx.now = func() time.Time { return time.Unix(10, 0) }
		return fx
	}

	t.Run("a sweep of the watch's own token signals it; another token's does not", func(t *testing.T) {
		// given: the strict mock has no mint expectation — registering with a
		// current generation, and being signalled, re-read nothing
		fx := newFixture(t)
		w, err := fx.newKeyWatch(ctx, fx.mwMock, "watchedKey", entry, fx.evictGen)
		require.NoError(t, err)
		require.Equal(t, v2service.KeyState{Grant: entry.Grant, ExpireAt: 42}, w.State())

		// when / then
		fx.RevokeToken("someone-elses-token")
		assert.False(t, signalled(w), "another token's sweep is not this key's business")
		fx.RevokeToken("tok")
		assert.True(t, signalled(w))
		assert.False(t, signalled(w), "one sweep, one signal")
	})

	t.Run("signals coalesce and a sweep never blocks", func(t *testing.T) {
		// given
		fx := newFixture(t)
		w, err := fx.newKeyWatch(ctx, fx.mwMock, "watchedKey", entry, fx.evictGen)
		require.NoError(t, err)

		// when: three sweeps with nobody reading
		fx.RevokeToken("tok")
		fx.RevokeToken("tok")
		fx.RevokeToken("tok")

		// then
		assert.True(t, signalled(w))
		assert.False(t, signalled(w))
	})

	t.Run("every watch of a token is signalled", func(t *testing.T) {
		// given
		fx := newFixture(t)
		first, err := fx.newKeyWatch(ctx, fx.mwMock, "watchedKey", entry, fx.evictGen)
		require.NoError(t, err)
		second, err := fx.newKeyWatch(ctx, fx.mwMock, "watchedKey", entry, fx.evictGen)
		require.NoError(t, err)

		// when
		fx.RevokeToken("tok")

		// then
		assert.True(t, signalled(first))
		assert.True(t, signalled(second))
	})

	t.Run("a generation older than the server's is stale: the key is re-read before the watch is handed out", func(t *testing.T) {
		// given: an eviction happened after the request read its grant
		fx := newFixture(t)
		observed := fx.evictGen
		fx.RevokeToken("tok")
		fx.mwMock.On("WalletCreateSession", mock.Anything, mintRequest).Return(narrowed, nil).Once()

		// when
		w, err := fx.newKeyWatch(ctx, fx.mwMock, "watchedKey", entry, observed)

		// then: the state is the wallet's, not the request's
		require.NoError(t, err)
		want := v2service.KeyState{Grant: &util.ApiGrant{Spaces: []string{"spaceA"}, Perms: util.GrantPermsRead}, ExpireAt: 77}
		assert.Equal(t, want, w.State())
		assert.Len(t, fx.watchers["tok"], 1, "registered under the token the request authenticated with")
	})

	t.Run("a stale key that is revoked, expired or unverifiable is refused and leaves no registration", func(t *testing.T) {
		for code, want := range map[pb.RpcWalletCreateSessionResponseErrorCode]error{
			pb.RpcWalletCreateSessionResponseError_APP_TOKEN_NOT_FOUND_IN_THE_CURRENT_ACCOUNT: v2service.ErrKeyRevoked,
			pb.RpcWalletCreateSessionResponseError_APP_TOKEN_EXPIRED:                          v2service.ErrKeyExpired,
			pb.RpcWalletCreateSessionResponseError_UNKNOWN_ERROR:                              v2service.ErrKeyUnverified,
		} {
			// given
			fx := newFixture(t)
			observed := fx.evictGen
			fx.RevokeToken("tok")
			fx.mwMock.On("WalletCreateSession", mock.Anything, mintRequest).Return(refused(code), nil).Once()

			// when
			w, err := fx.newKeyWatch(ctx, fx.mwMock, "watchedKey", entry, observed)

			// then
			require.ErrorIs(t, err, want, code.String())
			assert.Nil(t, w)
			assert.Empty(t, fx.watchers, code.String())
		}
	})

	t.Run("Close empties the registry, and closing twice is harmless", func(t *testing.T) {
		// given
		fx := newFixture(t)
		first, err := fx.newKeyWatch(ctx, fx.mwMock, "watchedKey", entry, fx.evictGen)
		require.NoError(t, err)
		second, err := fx.newKeyWatch(ctx, fx.mwMock, "watchedKey", entry, fx.evictGen)
		require.NoError(t, err)
		require.Len(t, fx.watchers["tok"], 2)

		// when / then
		first.Close()
		first.Close()
		assert.Len(t, fx.watchers["tok"], 1)
		second.Close()
		assert.Empty(t, fx.watchers, "no watch, no entry: the registry does not grow with every stream ever opened")
		// and a closed watch is not signalled
		fx.RevokeToken("tok")
		assert.False(t, signalled(first))
	})

	t.Run("CloseStreams closes every Closing, and a watch made afterwards is born closing", func(t *testing.T) {
		// given
		fx := newFixture(t)
		first, err := fx.newKeyWatch(ctx, fx.mwMock, "watchedKey", entry, fx.evictGen)
		require.NoError(t, err)
		other := entry
		other.Token = "otherTok"
		second, err := fx.newKeyWatch(ctx, fx.mwMock, "otherKey", other, fx.evictGen)
		require.NoError(t, err)
		require.False(t, closing(first))
		require.False(t, closing(second))

		// when
		fx.CloseStreams()
		fx.CloseStreams()

		// then
		assert.True(t, closing(first))
		assert.True(t, closing(second))
		late, err := fx.newKeyWatch(ctx, fx.mwMock, "watchedKey", entry, fx.evictGen)
		require.NoError(t, err)
		assert.True(t, closing(late))
	})

	t.Run("Current re-reads through the session mint and caches what it minted", func(t *testing.T) {
		// given
		fx := newFixture(t)
		w, err := fx.newKeyWatch(ctx, fx.mwMock, "watchedKey", entry, fx.evictGen)
		require.NoError(t, err)
		fx.mwMock.On("WalletCreateSession", mock.Anything, mintRequest).Return(narrowed, nil).Once()

		// when
		got, err := w.Current(ctx)

		// then
		require.NoError(t, err)
		assert.Equal(t, v2service.KeyState{Grant: &util.ApiGrant{Spaces: []string{"spaceA"}, Perms: util.GrantPermsRead}, ExpireAt: 77}, got)
		assert.Equal(t, "tok2", fx.KeyToToken["watchedKey"].Token, "the next request is a cache hit on the re-read entry")
		assert.Len(t, fx.watchers["tok"], 1, "the watch stays under the token it registered with")
	})

	t.Run("Current does not cache an entry an eviction raced", func(t *testing.T) {
		// given: a second edit's sweep lands while the re-read's mint is in flight
		fx := newFixture(t)
		w, err := fx.newKeyWatch(ctx, fx.mwMock, "watchedKey", entry, fx.evictGen)
		require.NoError(t, err)
		fx.mwMock.On("WalletCreateSession", mock.Anything, mintRequest).
			Run(func(mock.Arguments) { fx.RevokeToken("tok") }).
			Return(narrowed, nil).Once()

		// when
		_, err = w.Current(ctx)

		// then: the possibly stale entry is not cached, and the racing sweep
		// left a signal, so the stream re-reads once more
		require.NoError(t, err)
		assert.NotContains(t, fx.KeyToToken, "watchedKey")
		assert.True(t, signalled(w))
	})

	t.Run("Current says why a key can no longer be used", func(t *testing.T) {
		for code, want := range map[pb.RpcWalletCreateSessionResponseErrorCode]error{
			pb.RpcWalletCreateSessionResponseError_APP_TOKEN_NOT_FOUND_IN_THE_CURRENT_ACCOUNT: v2service.ErrKeyRevoked,
			pb.RpcWalletCreateSessionResponseError_APP_TOKEN_EXPIRED:                          v2service.ErrKeyExpired,
			pb.RpcWalletCreateSessionResponseError_UNKNOWN_ERROR:                              v2service.ErrKeyUnverified,
			pb.RpcWalletCreateSessionResponseError_BAD_INPUT:                                  v2service.ErrKeyUnverified,
		} {
			// given
			fx := newFixture(t)
			w, err := fx.newKeyWatch(ctx, fx.mwMock, "watchedKey", entry, fx.evictGen)
			require.NoError(t, err)
			fx.mwMock.On("WalletCreateSession", mock.Anything, mintRequest).Return(refused(code), nil).Once()

			// when
			_, err = w.Current(ctx)

			// then
			require.ErrorIs(t, err, want, code.String())
		}
	})

	t.Run("Current reports a key whose re-read expiry already passed as expired", func(t *testing.T) {
		// given: the wallet answers a session, but the clock is past its expiry
		fx := newFixture(t)
		fx.now = func() time.Time { return time.Unix(78, 0) }
		w, err := fx.newKeyWatch(ctx, fx.mwMock, "watchedKey", entry, fx.evictGen)
		require.NoError(t, err)
		fx.mwMock.On("WalletCreateSession", mock.Anything, mintRequest).Return(narrowed, nil).Once()

		// when
		_, err = w.Current(ctx)

		// then
		require.ErrorIs(t, err, v2service.ErrKeyExpired)
	})
}
