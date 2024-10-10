package detailservice

import (
	"context"
	"fmt"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/restriction/mock_restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const spaceId = "spaceId"

type fixture struct {
	Service
	getter       *mock_cache.MockObjectGetter
	resolver     *mock_idresolver.MockResolver
	spaceService *mock_space.MockService
	store        *objectstore.StoreFixture
	restriction  *mock_restriction.MockService
	space        *mock_clientspace.MockSpace
}

func newFixture(t *testing.T) *fixture {
	getter := mock_cache.NewMockObjectGetter(t)
	resolver := mock_idresolver.NewMockResolver(t)
	spaceService := mock_space.NewMockService(t)
	store := objectstore.NewStoreFixture(t)
	restriction := mock_restriction.NewMockService(t)

	spc := mock_clientspace.NewMockSpace(t)
	resolver.EXPECT().ResolveSpaceID(mock.Anything).Return(spaceId, nil).Maybe()
	spaceService.EXPECT().Get(mock.Anything, mock.Anything).Return(spc, nil).Maybe()

	s := &service{
		objectGetter: getter,
		resolver:     resolver,
		spaceService: spaceService,
		store:        store,
		restriction:  restriction,
	}

	return &fixture{
		s,
		getter,
		resolver,
		spaceService,
		store,
		restriction,
		spc,
	}
}

func TestService_SetDetailsList(t *testing.T) {
	details := []*model.Detail{
		{Key: bundle.RelationKeyAssignee.String(), Value: pbtypes.String("Mark Twain")},
		{Key: bundle.RelationKeyDone.String(), Value: pbtypes.Bool(true)},
		{Key: bundle.RelationKeyLinkedProjects.String(), Value: pbtypes.StringList([]string{"important", "urgent"})},
	}

	t.Run("lastUsed is updated once", func(t *testing.T) {
		// given
		fx := newFixture(t)
		objects := map[string]*smarttest.SmartTest{
			"obj1": smarttest.New("obj1"),
			"obj2": smarttest.New("obj2"),
			"obj3": smarttest.New("obj3"),
		}
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			object, ok := objects[objectId]
			require.True(t, ok)
			return object, nil
		})

		// when
		err := fx.SetDetailsList(nil, []string{"obj1", "obj2", "obj3"}, details)

		// then
		assert.NoError(t, err)
		require.Len(t, objects["obj1"].Results.LastUsedUpdates, 3)
		assert.Equal(t, []string{
			bundle.RelationKeyAssignee.String(),
			bundle.RelationKeyDone.String(),
			bundle.RelationKeyLinkedProjects.String(),
		}, objects["obj1"].Results.LastUsedUpdates)

		// lastUsed should be updated only during the work under 1st object
		assert.Len(t, objects["obj2"].Results.LastUsedUpdates, 0)
		assert.Len(t, objects["obj3"].Results.LastUsedUpdates, 0)

		assert.Equal(t, "Mark Twain", pbtypes.GetString(objects["obj1"].NewState().Details(), bundle.RelationKeyAssignee.String()))
		assert.True(t, pbtypes.GetBool(objects["obj2"].NewState().Details(), bundle.RelationKeyDone.String()))
		assert.Equal(t, []string{"important", "urgent"}, pbtypes.GetStringList(objects["obj3"].NewState().Details(), bundle.RelationKeyLinkedProjects.String()))
	})

	t.Run("some updates failed", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			if objectId != "obj2" {
				return nil, fmt.Errorf("failed to find object")
			}
			return smarttest.New(objectId), nil
		})

		// when
		err := fx.SetDetailsList(nil, []string{"obj1", "obj2", "obj3"}, details)

		// then
		assert.NoError(t, err)
	})

	t.Run("all updates failed", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, _ string) (smartblock.SmartBlock, error) {
			return nil, fmt.Errorf("failed to find object")
		})

		// when
		err := fx.SetDetailsList(nil, []string{"obj1", "obj2", "obj3"}, details)

		// then
		assert.Error(t, err)
	})
}

func TestService_ModifyDetailsList(t *testing.T) {
	ops := []*pb.RpcObjectListModifyDetailValuesRequestOperation{
		{RelationKey: bundle.RelationKeyName.String(), Set: pbtypes.String("My favorite page")},
		{RelationKey: bundle.RelationKeyLinks.String(), Add: pbtypes.String("some link")},
		{RelationKey: bundle.RelationKeyDone.String(), Set: pbtypes.Bool(true)},
	}

	t.Run("lastUsed is updated once", func(t *testing.T) {
		fx := newFixture(t)
		objects := map[string]*smarttest.SmartTest{
			"obj1": smarttest.New("obj1"),
			"obj2": smarttest.New("obj2"),
			"obj3": smarttest.New("obj3"),
		}
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			object, ok := objects[objectId]
			require.True(t, ok)
			return object, nil
		})

		// when
		err := fx.ModifyDetailsList(&pb.RpcObjectListModifyDetailValuesRequest{
			ObjectIds:  []string{"obj1", "obj2", "obj3"},
			Operations: ops,
		})

		// then
		assert.NoError(t, err)
		require.Len(t, objects["obj1"].Results.LastUsedUpdates, 3)

		// lastUsed should be updated only during the work under 1st object
		assert.Len(t, objects["obj2"].Results.LastUsedUpdates, 0)
		assert.Len(t, objects["obj3"].Results.LastUsedUpdates, 0)
	})

	t.Run("some updates failed", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			if objectId != "obj2" {
				return nil, fmt.Errorf("failed to find object")
			}
			return smarttest.New(objectId), nil
		})

		// when
		err := fx.ModifyDetailsList(&pb.RpcObjectListModifyDetailValuesRequest{
			ObjectIds:  []string{"obj1", "obj2", "obj3"},
			Operations: ops,
		})

		// then
		assert.NoError(t, err)
	})

	t.Run("all updates failed", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, _ string) (smartblock.SmartBlock, error) {
			return nil, fmt.Errorf("failed to find object")
		})

		// when
		err := fx.ModifyDetailsList(&pb.RpcObjectListModifyDetailValuesRequest{
			ObjectIds:  []string{"obj1", "obj2", "obj3"},
			Operations: ops,
		})

		// then
		assert.Error(t, err)
	})
}

func TestService_SetSpaceInfo(t *testing.T) {
	var (
		wsObjectId = "workspace"
		details    = &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyName.String():       pbtypes.String("My space"),
			bundle.RelationKeyIconOption.String(): pbtypes.Int64(5),
			bundle.RelationKeyIconImage.String():  pbtypes.String("kitten.jpg"),
		}}
	)

	t.Run("no error", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ws := smarttest.New(wsObjectId)
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Workspace: wsObjectId})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			assert.Equal(t, wsObjectId, objectId)
			return ws, nil
		})

		// when
		err := fx.SetSpaceInfo(spaceId, details)

		// then
		assert.NoError(t, err)
		assert.Equal(t, "My space", pbtypes.GetString(ws.NewState().Details(), bundle.RelationKeyName.String()))
		assert.Equal(t, int64(5), pbtypes.GetInt64(ws.NewState().Details(), bundle.RelationKeyIconOption.String()))
		assert.Equal(t, "kitten.jpg", pbtypes.GetString(ws.NewState().Details(), bundle.RelationKeyIconImage.String()))
	})

	t.Run("error on details setting", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Workspace: wsObjectId})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			assert.Equal(t, wsObjectId, objectId)
			return nil, fmt.Errorf("failed to get object")
		})

		// when
		err := fx.SetSpaceInfo(spaceId, details)

		// then
		assert.Error(t, err)
	})
}

func TestService_SetWorkspaceDashboardId(t *testing.T) {
	var (
		wsObjectId  = "workspace"
		dashboardId = "homepage"
	)

	t.Run("no error", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New(wsObjectId)
		sb.SetType(coresb.SmartBlockTypeWorkspace)
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			assert.Equal(t, wsObjectId, objectId)
			ws := &editor.Workspaces{
				SmartBlock:    sb,
				AllOperations: basic.NewBasic(sb, fx.store.SpaceIndex(spaceId), nil, nil, nil),
			}
			return ws, nil
		})

		// when
		setId, err := fx.SetWorkspaceDashboardId(nil, wsObjectId, dashboardId)

		// then
		assert.NoError(t, err)
		assert.Equal(t, dashboardId, setId)
		assert.Equal(t, dashboardId, pbtypes.GetString(sb.NewState().Details(), bundle.RelationKeySpaceDashboardId.String()))
	})

	t.Run("error if wrong smartblock type", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New(wsObjectId)
		sb.SetType(coresb.SmartBlockTypePage)
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			assert.Equal(t, wsObjectId, objectId)
			ws := &editor.Workspaces{
				SmartBlock:    sb,
				AllOperations: basic.NewBasic(sb, fx.store.SpaceIndex(spaceId), nil, nil, nil),
			}
			return ws, nil
		})

		// when
		_, err := fx.SetWorkspaceDashboardId(nil, wsObjectId, dashboardId)

		// then
		assert.Error(t, err)
		assert.ErrorIs(t, ErrUnexpectedBlockType, err)
	})
}

func TestService_SetListIsFavorite(t *testing.T) {
	var (
		objects = []objectstore.TestObject{
			{bundle.RelationKeyId: pbtypes.String("obj1"), bundle.RelationKeySpaceId: pbtypes.String(spaceId)},
			{bundle.RelationKeyId: pbtypes.String("obj2"), bundle.RelationKeySpaceId: pbtypes.String(spaceId)},
			{bundle.RelationKeyId: pbtypes.String("obj3"), bundle.RelationKeySpaceId: pbtypes.String(spaceId)},
		}
		homeId = "home"
	)

	t.Run("no error on favoriting", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New(homeId)
		sb.AddBlock(simple.New(&model.Block{Id: homeId, ChildrenIds: []string{}}))
		fx.store.AddObjects(t, spaceId, objects)
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Home: homeId})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			require.Equal(t, homeId, objectId)
			return editor.NewDashboard(sb, fx.store.SpaceIndex(spaceId), nil), nil
		})

		// when
		err := fx.SetListIsFavorite([]string{"obj1", "obj2", "obj3"}, true)

		// then
		assert.NoError(t, err)
		assert.Len(t, sb.Blocks(), 4)
	})

	t.Run("no error on unfavoriting", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New(homeId)
		sb.AddBlock(simple.New(&model.Block{Id: homeId, ChildrenIds: []string{"obj1", "obj2", "obj3"}}))
		sb.AddBlock(simple.New(&model.Block{Id: "obj1", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "obj1"}}}))
		sb.AddBlock(simple.New(&model.Block{Id: "obj2", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "obj2"}}}))
		sb.AddBlock(simple.New(&model.Block{Id: "obj3", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "obj3"}}}))
		fx.store.AddObjects(t, spaceId, objects)
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Home: homeId})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			require.Equal(t, homeId, objectId)
			return editor.NewDashboard(sb, fx.store.SpaceIndex(spaceId), nil), nil
		})

		// when
		err := fx.SetListIsFavorite([]string{"obj3", "obj1"}, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, sb.Blocks(), 2)
	})

	t.Run("some updates failed", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New(homeId)
		sb.AddBlock(simple.New(&model.Block{Id: homeId, ChildrenIds: []string{}}))
		fx.store.AddObjects(t, spaceId, objects)
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Home: homeId})
		flag := false
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			require.Equal(t, homeId, objectId)
			if flag {
				return nil, fmt.Errorf("unexpected error")
			}
			flag = true
			return editor.NewDashboard(sb, fx.store.SpaceIndex(spaceId), nil), nil
		})

		// when
		err := fx.SetListIsFavorite([]string{"obj3", "obj1"}, true)

		// then
		assert.NoError(t, err)
		assert.Len(t, sb.Blocks(), 2)
	})

	t.Run("all updates failed", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, objects)
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Home: homeId})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			require.Equal(t, homeId, objectId)
			return nil, fmt.Errorf("unexpected error")
		})

		// when
		err := fx.SetListIsFavorite([]string{"obj3", "obj1"}, true)

		// then
		assert.Error(t, err)
	})
}

func TestService_SetIsArchived(t *testing.T) {
	var (
		objects = []objectstore.TestObject{
			{bundle.RelationKeyId: pbtypes.String("obj1"), bundle.RelationKeySpaceId: pbtypes.String(spaceId)},
		}
		binId = "bin"
	)

	t.Run("no error on moving to bin", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New(binId)
		sb.AddBlock(simple.New(&model.Block{Id: binId, ChildrenIds: []string{}}))
		fx.store.AddObjects(t, spaceId, objects)
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Archive: binId})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			if objectId == binId {
				return editor.NewArchive(sb, fx.store.SpaceIndex(spaceId)), nil
			}
			return smarttest.New(objectId), nil
		})
		fx.restriction.EXPECT().CheckRestrictions(mock.Anything, mock.Anything).Return(nil)

		// when
		err := fx.SetIsArchived("obj1", true)

		// then
		assert.NoError(t, err)
		assert.Len(t, sb.Blocks(), 2)
	})

	t.Run("cannot move to bin an object with restriction", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New(binId)
		sb.AddBlock(simple.New(&model.Block{Id: binId, ChildrenIds: []string{}}))
		fx.store.AddObjects(t, spaceId, objects)
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			if objectId == binId {
				return editor.NewArchive(sb, fx.store.SpaceIndex(spaceId)), nil
			}
			return smarttest.New(objectId), nil
		})
		fx.restriction.EXPECT().CheckRestrictions(mock.Anything, mock.Anything).Return(restriction.ErrRestricted)

		// when
		err := fx.SetIsArchived("obj1", true)

		// then
		assert.Error(t, err)
		assert.ErrorIs(t, err, restriction.ErrRestricted)
	})
}

func TestService_SetListIsArchived(t *testing.T) {
	var (
		objects = []objectstore.TestObject{
			{bundle.RelationKeyId: pbtypes.String("obj1"), bundle.RelationKeySpaceId: pbtypes.String(spaceId)},
			{bundle.RelationKeyId: pbtypes.String("obj2"), bundle.RelationKeySpaceId: pbtypes.String(spaceId)},
			{bundle.RelationKeyId: pbtypes.String("obj3"), bundle.RelationKeySpaceId: pbtypes.String(spaceId)},
		}
		binId = "bin"
	)

	t.Run("no error on moving to bin", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New(binId)
		sb.AddBlock(simple.New(&model.Block{Id: binId, ChildrenIds: []string{}}))
		fx.store.AddObjects(t, spaceId, objects)
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Archive: binId})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			if objectId == binId {
				return editor.NewArchive(sb, fx.store.SpaceIndex(spaceId)), nil
			}
			return smarttest.New(objectId), nil
		})
		fx.restriction.EXPECT().CheckRestrictions(mock.Anything, mock.Anything).Return(nil)

		// when
		err := fx.SetListIsArchived([]string{"obj1", "obj2", "obj3"}, true)

		// then
		assert.NoError(t, err)
		assert.Len(t, sb.Blocks(), 4)
	})

	t.Run("no error on moving from bin", func(t *testing.T) {
		// given
		fx := newFixture(t)

		sb := smarttest.New(binId)
		sb.AddBlock(simple.New(&model.Block{Id: binId, ChildrenIds: []string{"obj1", "obj2", "obj3"}}))
		sb.AddBlock(simple.New(&model.Block{Id: "obj1", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "obj1"}}}))
		sb.AddBlock(simple.New(&model.Block{Id: "obj2", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "obj2"}}}))
		sb.AddBlock(simple.New(&model.Block{Id: "obj3", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "obj3"}}}))

		fx.store.AddObjects(t, spaceId, objects)
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Archive: binId})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			if objectId == binId {
				return editor.NewArchive(sb, fx.store.SpaceIndex(spaceId)), nil
			}
			return smarttest.New(objectId), nil
		})

		// when
		err := fx.SetListIsArchived([]string{"obj1", "obj2", "obj3"}, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, sb.Blocks(), 1)
	})

	t.Run("some updates failed", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New(binId)
		sb.AddBlock(simple.New(&model.Block{Id: binId, ChildrenIds: []string{}}))
		fx.store.AddObjects(t, spaceId, objects)
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Archive: binId})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			if objectId == binId {
				return editor.NewArchive(sb, fx.store.SpaceIndex(spaceId)), nil
			}
			if objectId == "obj2" {
				return nil, fmt.Errorf("failed to get object")
			}
			return smarttest.New(objectId), nil
		})
		fx.restriction.EXPECT().CheckRestrictions(mock.Anything, mock.Anything).Return(nil)

		// when
		err := fx.SetListIsArchived([]string{"obj1", "obj2", "obj3"}, true)

		// then
		assert.NoError(t, err)
		assert.Len(t, sb.Blocks(), 3)
	})

	t.Run("all updates failed", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, objects)
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Archive: binId})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			return nil, fmt.Errorf("failed to get object")
		})

		// when
		err := fx.SetListIsArchived([]string{"obj1", "obj2", "obj3"}, true)

		// then
		assert.Error(t, err)
	})
}
