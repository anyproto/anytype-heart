package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/apicore/mock_apicore"
)

type fixture struct {
	*Server
	accountService mock_apicore.MockAccountService
	exportService  mock_apicore.MockExportService
	mwMock         *mock_apicore.MockClientCommands
}

func newFixture(t *testing.T) *fixture {
	mwMock := mock_apicore.NewMockClientCommands(t)
	accountService := mock_apicore.NewMockAccountService(t)
	exportService := mock_apicore.NewMockExportService(t)
	server := NewServer(mwMock, accountService, exportService)

	return &fixture{
		Server:         server,
		accountService: *accountService,
		exportService:  *exportService,
		mwMock:         mwMock,
	}
}

func TestNewServer(t *testing.T) {
	t.Run("returns valid server", func(t *testing.T) {
		// when
		s := newFixture(t)

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
		s := newFixture(t)

		// when
		engine := s.Engine()

		// then
		require.Equal(t, s.engine, engine)
	})
}
