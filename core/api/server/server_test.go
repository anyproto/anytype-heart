package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/pb/service/mock_service"
)

type srvFixture struct {
	*Server
	accountService account.Service
	mwMock         *mock_service.MockClientCommandsServer
}

func newServerFixture(t *testing.T) *srvFixture {
	mwMock := mock_service.NewMockClientCommandsServer(t)
	accountService := mock_account.NewMockService(t)
	server := NewServer(accountService, mwMock)

	return &srvFixture{
		Server:         server,
		accountService: accountService,
		mwMock:         mwMock,
	}
}

func TestNewServer(t *testing.T) {
	t.Run("returns valid server", func(t *testing.T) {
		// when
		s := newServerFixture(t)

		// then
		require.NotNil(t, s)
		require.NotNil(t, s.engine)
		require.NotNil(t, s.KeyToToken)

		require.NotNil(t, s.authService)
		require.NotNil(t, s.exportService)
		require.NotNil(t, s.spaceService)
		require.NotNil(t, s.objectService)
		require.NotNil(t, s.listService)
		require.NotNil(t, s.searchService)

	})
}

func TestServer_Engine(t *testing.T) {
	t.Run("Engine returns same engine instance", func(t *testing.T) {
		// given
		s := newServerFixture(t)

		// when
		engine := s.Engine()

		// then
		require.Equal(t, s.engine, engine)
	})
}
