package techspace

import (
	"context"
	"fmt"
	"testing"
	"time"

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
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var ctx = context.Background()

const (
	testTechSpaceId = "techspaceId"
)

func TestTechSpace_Init(t *testing.T) {
	var initIDs = []string{"1", "2", "3"}
	fx := newFixture(t, initIDs)
	defer fx.finish(t)
}

type spaceViewStub struct {
	*smarttest.SmartTest
	data *types.Struct
}

var _ SpaceView = (*spaceViewStub)(nil)

func newSpaceViewStub(id string) *spaceViewStub {
	return &spaceViewStub{SmartTest: smarttest.New(id)}
}

func (s *spaceViewStub) SetSharedSpacesLimit(limits int) (err error) {
	return
}

func (s *spaceViewStub) GetSharedSpacesLimit() (limits int) {
	return
}

func (s *spaceViewStub) GetPersistentInfo() spaceinfo.SpacePersistentInfo {
	return spaceinfo.NewSpacePersistentInfo("spaceId")
}

func (s *spaceViewStub) GetLocalInfo() spaceinfo.SpaceLocalInfo {
	return spaceinfo.NewSpaceLocalInfo("spaceId")
}

func (s *spaceViewStub) SetInviteFileInfo(fileCid string, fileKey string) (err error) {
	return
}

func (s *spaceViewStub) SetAclIsEmpty(isEmpty bool) (err error) {
	return
}

func (s *spaceViewStub) RemoveExistingInviteInfo() (fileCid string, err error) {
	return
}

func (s *spaceViewStub) GetSpaceDescription() (data spaceinfo.SpaceDescription) {
	return
}

func (s *spaceViewStub) GetExistingInviteInfo() (fileCid string, fileKey string) {
	return
}

func (s *spaceViewStub) SetSpaceData(details *types.Struct) error {
	s.data = details
	return nil
}

func (s *spaceViewStub) SetSpaceLocalInfo(info spaceinfo.SpaceLocalInfo) (err error) {
	return nil
}

func (s *spaceViewStub) SetSpacePersistentInfo(info spaceinfo.SpacePersistentInfo) (err error) {
	return nil
}

func (s *spaceViewStub) SetAccessType(acc spaceinfo.AccessType) (err error) {
	return nil
}

func TestTechSpace_SpaceViewCreate(t *testing.T) {
	var (
		spaceId = "spaceId"
		viewId  = "viewId"
		view    = newSpaceViewStub(viewId)
	)

	t.Run("success", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)

		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(ctx, viewId).Return(nil, fmt.Errorf("not found"))
		fx.objectCache.EXPECT().DeriveTreeObject(ctx, mock.Anything).Return(view, nil)
		info := spaceinfo.NewSpacePersistentInfo(spaceId)
		info.SetAccountStatus(spaceinfo.AccountStatusUnknown)

		require.NoError(t, fx.SpaceViewCreate(ctx, spaceId, false, info))
	})

	t.Run("err spaceView exists", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)

		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(ctx, viewId).Return(view, nil)
		info := spaceinfo.NewSpacePersistentInfo(spaceId)
		info.SetAccountStatus(spaceinfo.AccountStatusUnknown)

		assert.EqualError(t, fx.SpaceViewCreate(ctx, spaceId, false, info), ErrSpaceViewExists.Error())
	})
}

func TestTechSpace_SpaceViewExists(t *testing.T) {
	var (
		spaceId = "spaceId"
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

func TestTechSpace_SpaceViewSetData(t *testing.T) {
	var (
		spaceId = "spaceId"
		viewId  = "viewId"
		view    = newSpaceViewStub(viewId)
	)
	t.Run("set data", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)
		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(mock.Anything, viewId).Return(view, nil)

		details := pbtypes.ToStruct(map[string]interface{}{
			"key": "value",
		})
		err := fx.SpaceViewSetData(ctx, spaceId, details)
		require.NoError(t, err)
		assert.Equal(t, details, view.data)
	})
}

func TestTechSpace_GetSpaceView(t *testing.T) {
	var (
		spaceId = "spaceId"
		viewId  = "viewId"
		view    = newSpaceViewStub(viewId)
	)
	t.Run("get view", func(t *testing.T) {
		fx := newFixture(t, nil)
		defer fx.finish(t)
		fx.expectDeriveTreePayload(viewId)
		fx.objectCache.EXPECT().GetObject(mock.Anything, viewId).Return(view, nil)

		other, err := fx.GetSpaceView(ctx, spaceId)
		require.NoError(t, err)
		assert.Equal(t, view, other)
	})
}

func TestTechSpace_SetInfo(t *testing.T) {
	info := spaceinfo.SpaceLocalInfo{
		SpaceId: "spaceId",
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

func TestTechSpace_WakeUpViews(t *testing.T) {
	t.Run("wake up views before close", func(t *testing.T) {
		fx := newFixture(t, []string{"1", "2", "3"})
		defer fx.finish(t)
		treeSyncer := mock_treesyncer.NewMockTreeSyncer(fx.ctrl)
		treeSyncer.EXPECT().StartSync()
		fx.techCore.EXPECT().StoredIds().Return(fx.ids)
		for _, id := range fx.ids {
			fx.objectCache.EXPECT().GetObject(mock.Anything, id).Return(newSpaceViewStub(id), nil)
		}
		fx.techCore.EXPECT().TreeSyncer().Return(treeSyncer)
		fx.WakeUpViews()
	})
	t.Run("wake up views twice don't call get objects twice", func(t *testing.T) {
		fx := newFixture(t, []string{"1", "2", "3"})
		defer fx.finish(t)
		treeSyncer := mock_treesyncer.NewMockTreeSyncer(fx.ctrl)
		treeSyncer.EXPECT().StartSync()
		fx.techCore.EXPECT().StoredIds().Times(1).Return(fx.ids)
		for _, id := range fx.ids {
			fx.objectCache.EXPECT().GetObject(mock.Anything, id).Times(1).Return(newSpaceViewStub(id), nil)
		}
		fx.techCore.EXPECT().TreeSyncer().Times(1).Return(treeSyncer)
		fx.WakeUpViews()
		fx.WakeUpViews()
	})
	t.Run("wake up views after close", func(t *testing.T) {
		fx := newFixture(t, []string{"1", "2", "3"})
		fx.finish(t)
		fx.WakeUpViews()
	})
}

func TestTechSpace_WaitViews(t *testing.T) {
	t.Run("wait after wake up views", func(t *testing.T) {
		fx := newFixture(t, []string{"1", "2"})
		// not calling finish to not wait for the views by default
		treeSyncer := mock_treesyncer.NewMockTreeSyncer(fx.ctrl)
		treeSyncer.EXPECT().StartSync()
		fx.techCore.EXPECT().StoredIds().Return(fx.ids)
		for _, id := range fx.ids {
			fx.objectCache.EXPECT().GetObject(mock.Anything, id).RunAndReturn(func(ctx2 context.Context, s string) (smartblock.SmartBlock, error) {
				// adding sleep to prove that we are indeed waiting
				time.Sleep(100 * time.Millisecond)
				return newSpaceViewStub(id), nil
			})
		}
		fx.techCore.EXPECT().TreeSyncer().Return(treeSyncer)
		fx.WakeUpViews()
		err := fx.WaitViews()
		require.NoError(t, err)
	})
	t.Run("wait without wake up views", func(t *testing.T) {
		fx := newFixture(t, []string{})
		defer fx.finish(t)
		err := fx.WaitViews()
		require.Equal(t, ErrNotStarted, err)
	})
	t.Run("wait views after close", func(t *testing.T) {
		fx := newFixture(t, []string{})
		treeSyncer := mock_treesyncer.NewMockTreeSyncer(fx.ctrl)
		treeSyncer.EXPECT().StartSync()
		fx.techCore.EXPECT().StoredIds().Return(fx.ids)
		fx.techCore.EXPECT().TreeSyncer().Return(treeSyncer)
		fx.WakeUpViews()
		fx.finish(t)
		err := fx.WaitViews()
		require.Equal(t, fx.TechSpace.(*techSpace).ctx.Err(), err)
	})
}

type fixture struct {
	TechSpace
	a           *app.App
	spaceCore   *mock_spacecore.MockSpaceCoreService
	objectCache *mock_objectcache.MockCache
	ctrl        *gomock.Controller
	techCore    *mock_commonspace.MockSpace
	ids         []string
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
		ids:         storeIDs,
	}
	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.spaceCore))

	// expect wakeUpIds
	fx.techCore.EXPECT().Id().Return(testTechSpaceId).AnyTimes()

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
