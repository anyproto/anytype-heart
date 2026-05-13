package server

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/subscription"
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
	crossSpaceSubService *mock_apicore.MockCrossSpaceSubscriptionService
	fileObjectMock       *mock_apicore.MockFileObjectService
}

func newFixture(t *testing.T) *fixture {
	mwMock := mock_apicore.NewMockClientCommands(t)
	accountMock := mock_apicore.NewMockAccountService(t)
	eventMock := mock_apicore.NewMockEventService(t)
	crossSpaceSubService := mock_apicore.NewMockCrossSpaceSubscriptionService(t)
	fileObjectMock := mock_apicore.NewMockFileObjectService(t)

	crossSpaceSubService.On("Subscribe", mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{}, nil).Maybe()
	accountMock.On("GetInfo", mock.Anything).Return(&model.AccountInfo{
		TechSpaceId: mockedTechSpaceId,
	}, nil).Once()

	server := NewServer(mwMock, accountMock, eventMock, crossSpaceSubService, fileObjectMock, mockedListenAddr, []byte{}, []byte{})

	return &fixture{
		Server:               server,
		mwMock:               mwMock,
		accountMock:          accountMock,
		eventMock:            eventMock,
		crossSpaceSubService: crossSpaceSubService,
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
