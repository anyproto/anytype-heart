package space

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/mock_commonspace"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache/mock_objectcache"
	"github.com/anyproto/anytype-heart/core/indexer/mock_indexer"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/mock_spacecore"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

const (
	testPersonalSpaceID = "personal.id"
)

func TestService_Init(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	fx := &fixture{
		service:       New().(*service),
		a:             new(app.App),
		ctrl:          ctrl,
		objectCache:   mock_objectcache.NewMockCache(t),
		indexer:       mock_indexer.NewMockIndexer(t),
		spaceCore:     mock_spacecore.NewMockSpaceCoreService(t),
		installer:     mock_space.NewMockbundledObjectsInstaller(t),
		personalSpace: mock_commonspace.NewMockSpace(ctrl),
	}

	fx.objectCache.EXPECT().Name().Return(objectcache.CName).Maybe()
	fx.objectCache.EXPECT().Init(mock.Anything).Maybe()
	fx.objectCache.EXPECT().Run(mock.Anything).Maybe()
	fx.objectCache.EXPECT().Close(mock.Anything).Maybe()

	fx.a.Register(testutil.PrepareMock(fx.a, fx.objectCache)).
		Register(testutil.PrepareMock(fx.a, fx.indexer)).
		Register(testutil.PrepareMock(fx.a, fx.spaceCore)).
		Register(testutil.PrepareMock(fx.a, fx.installer)).
		Register(fx.service)

	fx.expectRun()

	require.NoError(t, fx.a.Start(ctx))

	return fx
}

type fixture struct {
	*service
	objectCache   *mock_objectcache.MockCache
	indexer       *mock_indexer.MockIndexer
	spaceCore     *mock_spacecore.MockSpaceCoreService
	installer     *mock_space.MockbundledObjectsInstaller
	a             *app.App
	ctrl          *gomock.Controller
	personalSpace *mock_commonspace.MockSpace
}

func (fx *fixture) expectRun() {
	fx.spaceCore.EXPECT().DeriveID(ctx, spacecore.SpaceType).Return(testPersonalSpaceID, nil)
	fx.spaceCore.EXPECT().Derive(ctx, spacecore.TechSpaceType).Return(&spacecore.AnySpace{Space: fx.personalSpace}, nil)
	fx.indexer.EXPECT().ReindexCommonObjects()
}

func (fx *fixture) finish(t *testing.T) {
	if fx.wakeUpViewsCh != nil {
		<-fx.wakeUpViewsCh
	}
	require.NoError(t, fx.a.Close(ctx))
	fx.ctrl.Finish()
}
