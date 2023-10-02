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
	"github.com/anyproto/anytype-heart/core/indexer/mock_indexer"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/mock_spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

const (
	testPersonalSpaceID = "personal.id"
)

func TestService_Init(t *testing.T) {
	t.Run("existing account", func(t *testing.T) {
		fx := newFixture(t, false)
		defer fx.finish(t)
	})

}

func newFixture(t *testing.T, newAccount bool) *fixture {
	ctrl := gomock.NewController(t)
	fx := &fixture{
		service:       New().(*service),
		a:             new(app.App),
		ctrl:          ctrl,
		objectCache:   mock_objectcache.NewMockCache(t),
		indexer:       mock_indexer.NewMockIndexer(t),
		spaceCore:     mock_spacecore.NewMockSpaceCoreService(t),
		installer:     mock_space.NewMockbundledObjectsInstaller(t),
		isNewAccount:  mock_space.NewMockisNewAccount(t),
		techSpace:     mock_techspace.NewMockTechSpace(t),
		personalSpace: mock_commonspace.NewMockSpace(ctrl),
	}

	fx.a.Register(testutil.PrepareMock(fx.a, fx.objectCache)).
		Register(testutil.PrepareMock(fx.a, fx.indexer)).
		Register(testutil.PrepareMock(fx.a, fx.spaceCore)).
		Register(testutil.PrepareMock(fx.a, fx.installer)).
		Register(testutil.PrepareMock(fx.a, fx.isNewAccount)).
		Register(testutil.PrepareMock(fx.a, fx.techSpace)).
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
	indexer       *mock_indexer.MockIndexer
	spaceCore     *mock_spacecore.MockSpaceCoreService
	installer     *mock_space.MockbundledObjectsInstaller
	ctrl          *gomock.Controller
	isNewAccount  *mock_space.MockisNewAccount
	techSpace     *mock_techspace.MockTechSpace
	personalSpace *mock_commonspace.MockSpace
}

func (fx *fixture) expectRun(newAccount bool) {
	fx.spaceCore.EXPECT().DeriveID(mock.Anything, spacecore.SpaceType).Return(testPersonalSpaceID, nil)
	fx.indexer.EXPECT().ReindexCommonObjects().Return(nil)

	if !newAccount {
		fx.techSpace.EXPECT().GetInfo(testPersonalSpaceID).Return(spaceinfo.SpaceInfo{SpaceID: testPersonalSpaceID}).Times(1)
		fx.techSpace.EXPECT().DeriveSpaceViewID(mock.Anything, testPersonalSpaceID).Return("personalViewID", nil)
		fx.techSpace.EXPECT().SetInfo(mock.Anything, mock.Anything).Return(nil)
		fx.techSpace.EXPECT().GetInfo(testPersonalSpaceID).Return(spaceinfo.SpaceInfo{SpaceID: testPersonalSpaceID, LocalStatus: spaceinfo.LocalStatusLoading}).Times(1)
		fx.techSpace.EXPECT().GetInfo(testPersonalSpaceID).Return(spaceinfo.SpaceInfo{SpaceID: testPersonalSpaceID, LocalStatus: spaceinfo.LocalStatusOk}).Times(1)
		fx.spaceCore.EXPECT().Get(mock.Anything, testPersonalSpaceID).Return(&spacecore.AnySpace{Space: fx.personalSpace}, nil)
		fx.techSpace.EXPECT().SetStatuses(mock.Anything, testPersonalSpaceID, spaceinfo.LocalStatusOk, spaceinfo.RemoteStatusUnknown).Return(nil)

	}
	fx.objectCache.EXPECT().DeriveObjectID(mock.Anything, testPersonalSpaceID, mock.Anything).Return("derived", nil)
	fx.objectCache.EXPECT().GetObject(mock.Anything, domain.FullID{ObjectID: "derived", SpaceID: testPersonalSpaceID}).Return(nil, nil)
	fx.installer.EXPECT().InstallBundledObjects(mock.Anything, testPersonalSpaceID, mock.Anything).Return(nil, nil, nil)
	fx.indexer.EXPECT().ReindexSpace(testPersonalSpaceID).Return(nil)
	ts := mock_treesyncer.NewMockTreeSyncer(fx.ctrl)
	ts.EXPECT().StartSync()
	fx.personalSpace.EXPECT().TreeSyncer().Return(ts)
}

func (fx *fixture) finish(t *testing.T) {
	require.NoError(t, fx.a.Close(ctx))
	fx.ctrl.Finish()
}
