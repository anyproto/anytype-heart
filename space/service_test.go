package space

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient/mock_coordinatorclient"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller/mock_spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/internal/techspace/mock_techspace"
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
	deriveMetadata = func(acc crypto.PrivKey) ([]byte, error) {
		return []byte("metadata"), nil
	}
	fx.isNewAccount.EXPECT().IsNewAccount().Return(newAccount)
	fx.spaceCore.EXPECT().DeriveID(mock.Anything, mock.Anything).Return(testPersonalSpaceID, nil)
	fx.accountService.EXPECT().Account().Return(&accountdata.AccountKeys{})
	fx.expectRun(t, newAccount)

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

type lwMock struct {
	sp clientspace.Space
}

func (l lwMock) WaitLoad(ctx context.Context) (sp clientspace.Space, err error) {
	return l.sp, nil
}

func (fx *fixture) expectRun(t *testing.T, newAccount bool) {
	clientSpace := mock_clientspace.NewMockSpace(t)
	mpCtrl := mock_spacecontroller.NewMockSpaceController(t)
	fx.factory.EXPECT().CreateMarketplaceSpace(mock.Anything).Return(mpCtrl, nil)
	mpCtrl.EXPECT().Start(mock.Anything).Return(nil)
	ts := mock_techspace.NewMockTechSpace(t)
	fx.factory.EXPECT().CreateAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: ts}, nil)
	prCtrl := mock_spacecontroller.NewMockSpaceController(t)
	fx.coordClient.EXPECT().StatusCheckMany(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, fmt.Errorf("test not check statuses"))
	if newAccount {
		fx.factory.EXPECT().CreatePersonalSpace(mock.Anything).Return(prCtrl, nil)
		lw := lwMock{clientSpace}
		prCtrl.EXPECT().Current().Return(lw)
	} else {
		fx.factory.EXPECT().NewPersonalSpace(mock.Anything).Return(prCtrl, nil)
		lw := lwMock{clientSpace}
		prCtrl.EXPECT().Current().Return(lw)
	}
	prCtrl.EXPECT().Mode().Return(mode.ModeLoading)
	ts.EXPECT().Close(mock.Anything).Return(nil)
	mpCtrl.EXPECT().Close(mock.Anything).Return(nil)
	prCtrl.EXPECT().Close(mock.Anything).Return(nil)
	return
}

func (fx *fixture) finish(t *testing.T) {
	require.NoError(t, fx.a.Close(ctx))
	fx.ctrl.Finish()
}
