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
	"github.com/anyproto/any-sync/net/peer"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache/mock_objectcache"
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

type spaceViewStub struct {
	*smarttest.SmartTest
}

func newSpaceViewStub(id string) *spaceViewStub {
	return &spaceViewStub{SmartTest: smarttest.New(id)}
}

func (s *spaceViewStub) SetSpaceData(details *types.Struct) error {
	return nil
}

func (s *spaceViewStub) SetSpaceLocalInfo(info spaceinfo.SpaceLocalInfo) (err error) {
	return nil
}

func (s *spaceViewStub) SetSpacePersistentInfo(info spaceinfo.SpacePersistentInfo) (err error) {
	return nil
}

func TestTechSpace_SpaceViewCreate(t *testing.T) {
	var (
		spaceId = "space.id"
		viewId  = "viewId"
		view    = newSpaceViewStub(viewId)
	)

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)

		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(ctx, viewId).Return(nil, fmt.Errorf("not found"))
		fx.objectCache.EXPECT().DeriveTreeObject(ctx, mock.Anything).Return(view, nil)

		require.NoError(t, fx.SpaceViewCreate(ctx, spaceId, false))
	})

	t.Run("err spaceView exists", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)

		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(ctx, viewId).Return(view, nil)

		assert.EqualError(t, fx.SpaceViewCreate(ctx, spaceId, false), ErrSpaceViewExists.Error())
	})
}

func TestTechSpace_SpaceViewExists(t *testing.T) {
	var (
		spaceId = "space.id"
		viewId  = "viewId"
		view    = newSpaceViewStub(viewId)
	)
	t.Run("exists", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)
		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(mock.Anything, viewId).RunAndReturn(func(peerCtx context.Context, s string) (smartblock.SmartBlock, error) {
			id, err := peer.CtxPeerId(peerCtx)
			require.NoError(t, err)
			require.Equal(t, peer.CtxResponsiblePeers, id)
			return view, nil
		})
		exists, err := fx.SpaceViewExists(ctx, spaceId)
		require.NoError(t, err)
		assert.True(t, exists)
	})
	t.Run("not exists", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)
		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(mock.Anything, viewId).RunAndReturn(func(peerCtx context.Context, s string) (smartblock.SmartBlock, error) {
			id, err := peer.CtxPeerId(peerCtx)
			require.NoError(t, err)
			require.Equal(t, peer.CtxResponsiblePeers, id)
			return nil, fmt.Errorf("not found")
		})
		exists, err := fx.SpaceViewExists(ctx, spaceId)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestTechSpace_SetInfo(t *testing.T) {
	info := spaceinfo.SpaceLocalInfo{
		SpaceID: "space.id",
	}
	viewId := "viewid"
	spaceView := newSpaceViewStub(viewId)

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)

		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(ctx, viewId).Return(spaceView, nil)

		require.NoError(t, fx.SetLocalInfo(ctx, info))
	})

	t.Run("err spaceView not exists", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)

		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(ctx, viewId).Return(nil, fmt.Errorf("object not found"))

		require.EqualError(t, fx.SetLocalInfo(ctx, info), ErrSpaceViewNotExists.Error())
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
	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.spaceCore))

	// expect wakeUpIds
	treeSyncer := mock_treesyncer.NewMockTreeSyncer(fx.ctrl)
	treeSyncer.EXPECT().StartSync()
	fx.techCore.EXPECT().Id().Return(testTechSpaceId).AnyTimes()
	fx.techCore.EXPECT().StoredIds().Return(storeIDs)
	for _, id := range storeIDs {
		fx.objectCache.EXPECT().GetObject(mock.Anything, id).Return(newSpaceViewStub(id), nil)
	}
	fx.techCore.EXPECT().TreeSyncer().Return(treeSyncer)

	require.NoError(t, fx.a.Start(ctx))
	err := fx.TechSpace.Run(fx.techCore, fx.objectCache)
	require.NoError(t, err)

	// do not cancel wakeUpIds func
	fx.TechSpace.(*techSpace).ctxCancel = func() {}

	return fx
}

func (fx *fixture) expectDeriveTreePayload(viewId string) {
	fx.objectCache.EXPECT().DeriveTreePayload(ctx, mock.Anything).Return(treestorage.TreeStorageCreatePayload{
		RootRawChange: &treechangeproto.RawTreeChangeWithId{
			Id: viewId,
		},
	}, nil)
}

func (fx *fixture) finish(t *testing.T) {
	require.NoError(t, fx.a.Close(ctx))
	require.NoError(t, fx.TechSpace.Close(ctx))
	fx.ctrl.Finish()
}
