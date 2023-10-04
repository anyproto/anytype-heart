package techspace

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/mock_commonspace"
	"github.com/anyproto/any-sync/commonspace/object/treesyncer/mock_treesyncer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache/mock_objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	space "github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/mock_spacecore"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

const (
	testTechSpaceId = "techSpace.id"
)

func TestTechSpace_Init(t *testing.T) {
	var initIDs = []string{"1", "2", "3"}
	fx := newFixture(t, initIDs)
	defer fx.finish(t)
}

type fixture struct {
	TechSpace
	a           *app.App
	spaceCore   *mock_spacecore.MockSpaceCoreService
	objectCache *mock_objectcache.MockCache
	ctrl        *gomock.Controller
	techCore    *mock_commonspace.MockSpace
}

func newFixture(t *testing.T, storeIDs []string) *fixture {
	ctrl := gomock.NewController(t)
	fx := &fixture{
		TechSpace:   New(),
		a:           new(app.App),
		ctrl:        ctrl,
		spaceCore:   mock_spacecore.NewMockSpaceCoreService(t),
		objectCache: mock_objectcache.NewMockCache(t),
		techCore:    mock_commonspace.NewMockSpace(ctrl),
	}
	fx.a.Register(testutil.PrepareMock(fx.a, fx.spaceCore)).
		Register(testutil.PrepareMock(fx.a, fx.objectCache)).
		Register(fx.TechSpace)

	treeSyncer := mock_treesyncer.NewMockTreeSyncer(fx.ctrl)
	treeSyncer.EXPECT().StartSync()
	fx.techCore.EXPECT().Id().Return(testTechSpaceId).AnyTimes()
	fx.techCore.EXPECT().StoredIds().Return(storeIDs)
	for _, id := range storeIDs {
		fx.objectCache.EXPECT().GetObject(mock.Anything, domain.FullID{ObjectID: id, SpaceID: testTechSpaceId}).Return(&editor.SpaceView{SmartBlock: smarttest.New(id)}, nil)
	}
	fx.techCore.EXPECT().TreeSyncer().Return(treeSyncer)

	fx.spaceCore.EXPECT().Derive(ctx, space.TechSpaceType).Return(&space.AnySpace{Space: fx.techCore}, nil)

	require.NoError(t, fx.a.Start(ctx))
	return fx
}

func (fx *fixture) finish(t *testing.T) {
	require.NoError(t, fx.a.Close(ctx))
	fx.ctrl.Finish()
}
