package server

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	mockedGatewayUrl  = "http://localhost:31006"
	mockedTechSpaceId = "tech123"
)

type fixture struct {
	*Server
	eventService   mock_apicore.MockEventService
	mwMock         *mock_apicore.MockClientCommands
}

func newFixture(t *testing.T) *fixture {
	mwMock := mock_apicore.NewMockClientCommands(t)
	accountService := mock_apicore.NewMockAccountService(t)
	eventService := mock_apicore.NewMockEventService(t)
	accountService.On("GetInfo", mock.Anything).Return(&model.AccountInfo{
		GatewayUrl:  mockedGatewayUrl,
		TechSpaceId: mockedTechSpaceId,
	}, nil).Once()
	server := NewServer(mwMock, accountService, eventService)

	return &fixture{
		Server: server,
		mwMock: mwMock,
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

func TestServer_GetAccountInfo(t *testing.T) {
	t.Run("successful retrieval", func(t *testing.T) {
		// given
		mockAcc := mock_apicore.NewMockAccountService(t)
		mockAcc.On("GetInfo", mock.Anything).Return(&model.AccountInfo{
			GatewayUrl:  mockedGatewayUrl,
			TechSpaceId: mockedTechSpaceId,
		}, nil).Once()

		// when
		gatewayUrl, techSpaceId, err := getAccountInfo(mockAcc)

		// then
		require.NoError(t, err)
		require.Equal(t, mockedGatewayUrl, gatewayUrl)
		require.Equal(t, mockedTechSpaceId, techSpaceId)
	})

	t.Run("error retrieving account info", func(t *testing.T) {
		// given
		mockAcc := mock_apicore.NewMockAccountService(t)
		expectedError := errors.New("failed to get info")
		mockAcc.On("GetInfo", mock.Anything).Return(nil, expectedError)

		// when
		gatewayUrl, techSpaceId, err := getAccountInfo(mockAcc)

		// then
		require.Error(t, err)
		require.Equal(t, "", gatewayUrl)
		require.Equal(t, "", techSpaceId)
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
