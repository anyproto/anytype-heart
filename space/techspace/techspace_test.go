package techspace

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/mock_commonspace"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/object/treesyncer/mock_treesyncer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache/mock_objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	space "github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/mock_spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
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

func TestTechSpace_CreateSpaceView(t *testing.T) {
	fx := newFixture(t, nil)
	defer fx.finish(t)

	spaceView := &editor.SpaceView{}
	fx.objectCache.EXPECT().DeriveTreeObject(ctx, testTechSpaceId, mock.Anything).Return(spaceView, nil)

	res, err := fx.CreateSpaceView(ctx, "new.space")
	require.NoError(t, err)
	assert.Equal(t, spaceView, res)
}

func TestTechSpace_DeriveSpaceViewID(t *testing.T) {
	fx := newFixture(t, nil)
	defer fx.finish(t)

	payload := treestorage.TreeStorageCreatePayload{
		RootRawChange: &treechangeproto.RawTreeChangeWithId{
			Id: "viewId",
		},
	}
	fx.objectCache.EXPECT().DeriveTreePayload(ctx, testTechSpaceId, mock.Anything).Return(payload, nil)

	res, err := fx.DeriveSpaceViewID(ctx, "new.space")
	require.NoError(t, err)
	assert.Equal(t, payload.RootRawChange.Id, res)
}

func TestTechSpace_SetInfo(t *testing.T) {
	info := spaceinfo.SpaceInfo{
		SpaceID: "1",
		ViewID:  "2",
	}
	spaceView := &editor.SpaceView{SmartBlock: smarttest.New(info.ViewID)}

	t.Run("existing spaceView", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)

		fx.objectCache.EXPECT().GetObject(ctx, domain.FullID{ObjectID: info.ViewID, SpaceID: testTechSpaceId}).Return(spaceView, nil)

		require.NoError(t, fx.SetInfo(ctx, info))
		assert.Equal(t, info, fx.GetInfo(info.SpaceID))
	})

	t.Run("create spaceView", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)

		fx.objectCache.EXPECT().GetObject(ctx, domain.FullID{ObjectID: info.ViewID, SpaceID: testTechSpaceId}).Return(nil, fmt.Errorf("no object"))
		fx.objectCache.EXPECT().DeriveTreeObject(ctx, testTechSpaceId, mock.Anything).Return(spaceView, nil)

		require.NoError(t, fx.SetInfo(ctx, info))
		// second call with same info
		require.NoError(t, fx.SetInfo(ctx, info))

		assert.Equal(t, info, fx.GetInfo(info.SpaceID))
	})
}

func TestTechSpace_SetStatuses(t *testing.T) {
	info := spaceinfo.SpaceInfo{
		SpaceID: "1",
		ViewID:  "2",
	}
	spaceView := &editor.SpaceView{SmartBlock: smarttest.New(info.ViewID)}

	t.Run("changed", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)

		fx.objectCache.EXPECT().GetObject(ctx, domain.FullID{ObjectID: info.ViewID, SpaceID: testTechSpaceId}).Return(spaceView, nil)
		require.NoError(t, fx.SetInfo(ctx, info))

		changedInfo := info
		changedInfo.LocalStatus = spaceinfo.LocalStatusLoading
		changedInfo.RemoteStatus = spaceinfo.RemoteStatusError

		require.NoError(t, fx.SetStatuses(ctx, info.SpaceID, changedInfo.LocalStatus, changedInfo.RemoteStatus))
		assert.Equal(t, changedInfo, fx.GetInfo(info.SpaceID))
	})
	t.Run("not changed", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)

		fx.objectCache.EXPECT().GetObject(ctx, domain.FullID{ObjectID: info.ViewID, SpaceID: testTechSpaceId}).Return(spaceView, nil)
		require.NoError(t, fx.SetInfo(ctx, info))

		require.NoError(t, fx.SetStatuses(ctx, info.SpaceID, info.LocalStatus, info.RemoteStatus))
		assert.Equal(t, info, fx.GetInfo(info.SpaceID))
	})
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
	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.spaceCore)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.objectCache)).
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
