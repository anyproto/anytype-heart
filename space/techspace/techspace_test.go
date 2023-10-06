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

func TestTechSpace_SpaceViewCreate(t *testing.T) {
	var (
		spaceId = "space.id"
		viewId  = "viewId"
		view    = &editor.SpaceView{SmartBlock: smarttest.New(viewId)}
	)

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)

		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(ctx, domain.FullID{SpaceID: testTechSpaceId, ObjectID: viewId}).Return(nil, fmt.Errorf("not found"))
		fx.objectCache.EXPECT().DeriveTreeObject(ctx, testTechSpaceId, mock.Anything).Return(view, nil)

		require.NoError(t, fx.SpaceViewCreate(ctx, spaceId))
	})

	t.Run("err spaceView exists", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)

		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(ctx, domain.FullID{SpaceID: testTechSpaceId, ObjectID: viewId}).Return(view, nil)

		assert.EqualError(t, fx.SpaceViewCreate(ctx, spaceId), ErrSpaceViewExists.Error())
	})
}

func TestTechSpace_SpaceViewExists(t *testing.T) {
	var (
		spaceId = "space.id"
		viewId  = "viewId"
		view    = &editor.SpaceView{SmartBlock: smarttest.New(viewId)}
	)
	t.Run("exists", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)
		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(ctx, domain.FullID{ObjectID: viewId, SpaceID: testTechSpaceId}).Return(view, nil)
		exists, err := fx.SpaceViewExists(ctx, spaceId)
		require.NoError(t, err)
		assert.True(t, exists)
	})
	t.Run("not exists", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)
		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(ctx, domain.FullID{ObjectID: viewId, SpaceID: testTechSpaceId}).Return(nil, fmt.Errorf("not found"))
		exists, err := fx.SpaceViewExists(ctx, spaceId)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestTechSpace_SetInfo(t *testing.T) {
	info := spaceinfo.SpaceInfo{
		SpaceID: "space.id",
	}
	viewId := "viewid"
	spaceView := &editor.SpaceView{SmartBlock: smarttest.New(viewId)}

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)

		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(ctx, domain.FullID{ObjectID: viewId, SpaceID: testTechSpaceId}).Return(spaceView, nil)

		require.NoError(t, fx.SetInfo(ctx, info))
	})

	t.Run("err spaceView not exists", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)

		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(ctx, domain.FullID{ObjectID: viewId, SpaceID: testTechSpaceId}).Return(nil, fmt.Errorf("object not found"))

		require.EqualError(t, fx.SetInfo(ctx, info), ErrSpaceViewNotExists.Error())
	})
}

func TestTechSpace_TechSpaceId(t *testing.T) {
	fx := newFixture(t, nil)
	defer fx.finish(t)
	assert.Equal(t, testTechSpaceId, fx.TechSpaceId())
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

	// expect wakeUpIds
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

	// do not cancel wakeUpIds func
	fx.TechSpace.(*techSpace).ctxCancel = func() {}
	return fx
}

func (fx *fixture) expectDeriveTreePayload(viewId string) {
	fx.objectCache.EXPECT().DeriveTreePayload(ctx, testTechSpaceId, mock.Anything).Return(treestorage.TreeStorageCreatePayload{
		RootRawChange: &treechangeproto.RawTreeChangeWithId{
			Id: viewId,
		},
	}, nil)
}

func (fx *fixture) finish(t *testing.T) {
	require.NoError(t, fx.a.Close(ctx))
	fx.ctrl.Finish()
}
