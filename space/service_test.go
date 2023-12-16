package space

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/mock_commonspace"
	"github.com/anyproto/any-sync/commonspace/object/treesyncer/mock_treesyncer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache/mock_objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/space/internal/techspace/mock_techspace"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/mock_spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

const (
	testPersonalSpaceID = "personal.id"
)

// TODO Revive tests
func xTestService_Init(t *testing.T) {
	t.Run("existing account", func(t *testing.T) {
		fx := newFixture(t, false)
		defer fx.finish(t)
	})
	t.Run("new account", func(t *testing.T) {
		fx := newFixture(t, true)
		defer fx.finish(t)
	})

}

type indexerStub struct {
}

func (i *indexerStub) ReindexMarketplaceSpace(space Space) error {
	return nil
}

func (i *indexerStub) ReindexSpace(space Space) error {
	return nil
}

func (i *indexerStub) Init(a *app.App) (err error) {
	return nil
}

func (i *indexerStub) Name() (name string) {
	return "indexerStub"
}

func newFixture(t *testing.T, newAccount bool) *fixture {
	ctrl := gomock.NewController(t)
	fx := &fixture{
		service:       New().(*service),
		a:             new(app.App),
		ctrl:          ctrl,
		objectCache:   mock_objectcache.NewMockCache(t),
		spaceCore:     mock_spacecore.NewMockSpaceCoreService(t),
		installer:     NewMockbundledObjectsInstaller(t),
		isNewAccount:  NewMockisNewAccount(t),
		techSpace:     mock_techspace.NewMockTechSpace(t),
		personalSpace: mock_commonspace.NewMockSpace(ctrl),
	}
	fx.a.Register(&indexerStub{}).
		Register(testutil.PrepareMock(ctx, fx.a, fx.spaceCore)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.installer)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.isNewAccount)).
		Register(fx.service)

	fx.isNewAccount.EXPECT().IsNewAccount().Return(newAccount)
	fx.personalSpace.EXPECT().Id().AnyTimes().Return(testPersonalSpaceID)

	fx.expectRun(newAccount)

	require.NoError(t, fx.a.Start(ctx))

	return fx
}

type fixture struct {
	*service
	a             *app.App
	objectCache   *mock_objectcache.MockCache
	spaceCore     *mock_spacecore.MockSpaceCoreService
	installer     *MockbundledObjectsInstaller
	ctrl          *gomock.Controller
	isNewAccount  *MockisNewAccount
	techSpace     *mock_techspace.MockTechSpace
	personalSpace *mock_commonspace.MockSpace
}

func (fx *fixture) expectRun(newAccount bool) {
	fx.spaceCore.EXPECT().DeriveID(mock.Anything, spacecore.SpaceType).Return(testPersonalSpaceID, nil)

	if newAccount {
		fx.spaceCore.EXPECT().Derive(mock.Anything, spacecore.SpaceType).Return(&spacecore.AnySpace{Space: fx.personalSpace}, nil)
		// fx.objectCache.EXPECT().DeriveTreeObject(mock.Anything, testPersonalSpaceID, mock.Anything).Return(nil, nil)
		fx.techSpace.EXPECT().SpaceViewCreate(mock.Anything, testPersonalSpaceID, nil).Return(nil)
	}
	// startLoad
	fx.techSpace.EXPECT().SpaceViewExists(mock.Anything, testPersonalSpaceID).Return(true, nil)
	fx.techSpace.EXPECT().SetLocalInfo(mock.Anything, mock.Anything).Return(nil)
	// wait load
	fx.spaceCore.EXPECT().Get(mock.Anything, testPersonalSpaceID).Return(&spacecore.AnySpace{Space: fx.personalSpace}, nil)
	fx.techSpace.EXPECT().SetLocalInfo(mock.Anything, spaceinfo.SpaceLocalInfo{
		SpaceID:      testPersonalSpaceID,
		LocalStatus:  spaceinfo.LocalStatusOk,
		RemoteStatus: spaceinfo.RemoteStatusUnknown,
	}).Return(nil)

	// space init
	// fx.objectCache.EXPECT().DeriveObjectID(mock.Anything, testPersonalSpaceID, mock.Anything).Return("derived", nil)
	fx.objectCache.EXPECT().GetObject(mock.Anything, domain.FullID{ObjectID: "derived", SpaceID: testPersonalSpaceID}).Return(nil, nil)
	fx.installer.EXPECT().InstallBundledObjects(mock.Anything, testPersonalSpaceID, mock.Anything).Return(nil, nil, nil)
	ts := mock_treesyncer.NewMockTreeSyncer(fx.ctrl)
	ts.EXPECT().StartSync()
	fx.personalSpace.EXPECT().TreeSyncer().Return(ts)
}

func (fx *fixture) finish(t *testing.T) {
	require.NoError(t, fx.a.Close(ctx))
	fx.ctrl.Finish()
}
