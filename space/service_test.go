package space

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient/mock_coordinatorclient"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/space/spacecore/mock_spacecore"
	"github.com/anyproto/anytype-heart/space/spacefactory/mock_spacefactory"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

const (
	testPersonalSpaceID = "personal.12345"
)

// TODO Revive tests
func TestService_Init(t *testing.T) {
	t.Run("existing account", func(t *testing.T) {
		fx := newFixture(t, false)
		defer fx.finish(t)
	})
	t.Run("new account", func(t *testing.T) {
		fx := newFixture(t, true)
		defer fx.finish(t)
	})

}

func newFixture(t *testing.T, newAccount bool) *fixture {
	ctrl := gomock.NewController(t)
	fx := &fixture{
		service:        New().(*service),
		a:              new(app.App),
		ctrl:           ctrl,
		spaceCore:      mock_spacecore.NewMockSpaceCoreService(t),
		accountService: mock_accountservice.NewMockService(ctrl),
		coordClient:    mock_coordinatorclient.NewMockCoordinatorClient(ctrl),
		factory:        mock_spacefactory.NewMockSpaceFactory(t),
		isNewAccount:   NewMockisNewAccount(t),
	}
	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.spaceCore)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.coordClient)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.accountService)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.isNewAccount)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.factory)).
		Register(fx.service)

	fx.isNewAccount.EXPECT().IsNewAccount().Return(newAccount)
	fx.spaceCore.EXPECT().DeriveID(mock.Anything, mock.Anything).Return(testPersonalSpaceID, nil)
	fx.expectRun(newAccount)

	require.NoError(t, fx.a.Start(ctx))

	return fx
}

type fixture struct {
	*service
	a              *app.App
	factory        *mock_spacefactory.MockSpaceFactory
	spaceCore      *mock_spacecore.MockSpaceCoreService
	accountService *mock_accountservice.MockService
	coordClient    *mock_coordinatorclient.MockCoordinatorClient
	ctrl           *gomock.Controller
	isNewAccount   *MockisNewAccount
}

func (fx *fixture) expectRun(newAccount bool) {
	return
}

func (fx *fixture) finish(t *testing.T) {
	require.NoError(t, fx.a.Close(ctx))
	fx.ctrl.Finish()
}
