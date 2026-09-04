package api

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/core/mock_apicore"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// newToolsHostFixture builds the component the way Init would, from mocks,
// with no listen address — the mobile configuration.
func newToolsHostFixture(t *testing.T, accountMock *mock_apicore.MockAccountService) *apiService {
	crossSpaceSub := mock_apicore.NewMockCrossSpaceSubscriptionService(t)
	crossSpaceSub.On("Subscribe", mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{}, nil).Maybe()
	eventMock := mock_apicore.NewMockEventService(t)
	eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
	return &apiService{
		mw:                   mock_apicore.NewMockClientCommands(t),
		accountService:       accountMock,
		eventService:         eventMock,
		crossSpaceSubService: crossSpaceSub,
		chatSubService:       mock_apicore.NewMockChatSubscriptionService(t),
		fileObjectService:    mock_apicore.NewMockFileObjectService(t),
		objectReader:         mock_apicore.NewMockObjectReader(t),
		objectCreator:        mock_apicore.NewMockObjectCreator(t),
		objectMutator:        mock_apicore.NewMockObjectMutator(t),
		objectProvenance:     mock_apicore.NewMockObjectProvenance(t),
		objectStore:          objectstore.NewStoreFixture(t),
	}
}

func runningAccount(t *testing.T) *mock_apicore.MockAccountService {
	accountMock := mock_apicore.NewMockAccountService(t)
	accountMock.On("GetInfo", mock.Anything).Return(&model.AccountInfo{TechSpaceId: "tech1"}, nil)
	return accountMock
}

func TestToolsHost(t *testing.T) {
	t.Run("builds the engine without a listener and reuses it", func(t *testing.T) {
		// given
		fx := newToolsHostFixture(t, runningAccount(t))

		// when
		host, err := fx.ToolsHost()

		// then
		require.NoError(t, err)
		require.NotNil(t, host)
		assert.Nil(t, fx.httpSrv, "nothing listens")
		require.NotNil(t, fx.srv, "the engine exists")
		again, err := fx.ToolsHost()
		require.NoError(t, err)
		assert.Same(t, host, again)
	})

	t.Run("the host reaches the engine as the account holder", func(t *testing.T) {
		// given: spaces is the bootstrap tool; an empty store lists none —
		// the call still crosses auth, the scope gate and the v2 service
		fx := newToolsHostFixture(t, runningAccount(t))
		host, err := fx.ToolsHost()
		require.NoError(t, err)

		// when
		got := host.Call(context.Background(), "spaces", nil)

		// then
		assert.False(t, got.IsError, got.Text)
		assert.Contains(t, got.Text, "no spaces")
	})

	t.Run("an unknown tool is answered in-band", func(t *testing.T) {
		// given
		fx := newToolsHostFixture(t, runningAccount(t))
		host, err := fx.ToolsHost()
		require.NoError(t, err)

		// when
		got := host.Call(context.Background(), "nope", nil)

		// then
		assert.True(t, got.IsError)
		assert.Contains(t, got.Text, `unknown tool "nope"`)
	})

	t.Run("no account info is an error, not a panic", func(t *testing.T) {
		// given
		accountMock := mock_apicore.NewMockAccountService(t)
		accountMock.On("GetInfo", mock.Anything).Return(nil, errors.New("account is stopping"))
		fx := newToolsHostFixture(t, accountMock)

		// when
		host, err := fx.ToolsHost()

		// then
		require.Error(t, err)
		assert.Nil(t, host)
		assert.Nil(t, fx.srv)
	})

	t.Run("ReassignAddress rebuilds the engine the host resolves", func(t *testing.T) {
		// given: a host built on the mobile configuration
		fx := newToolsHostFixture(t, runningAccount(t))
		host, err := fx.ToolsHost()
		require.NoError(t, err)
		first := fx.srv

		// when: the address changes to "still nothing" — the old server is
		// dropped and no new one is built until the next tool call
		require.NoError(t, fx.ReassignAddress(context.Background(), ""))

		// then
		assert.Nil(t, fx.srv)
		got := host.Call(context.Background(), "spaces", nil)
		assert.True(t, got.IsError)
		assert.Contains(t, got.Text, "account is not running")
		again, err := fx.ToolsHost()
		require.NoError(t, err)
		assert.Same(t, host, again, "the host is kept; only the engine was rebuilt")
		assert.NotSame(t, first, fx.srv)
	})
}
