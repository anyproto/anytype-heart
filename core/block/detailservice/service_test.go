package detailservice

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/block/objectgc"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const spaceId = "spaceId"

type fileGCStub struct{}

func (f *fileGCStub) Name() string                    { return "fileGCStub" }
func (f *fileGCStub) Init(a *app.App) error           { return nil }
func (f *fileGCStub) Run(ctx context.Context) error   { return nil }
func (f *fileGCStub) Close(ctx context.Context) error { return nil }
func (f *fileGCStub) ArchiveOrphansOnLinksRemoval(spaceId, contextId string, removedLinks []string, skipBin bool, onlyBlockIds []string) (objectgc.OrphanCandidates, error) {
	return objectgc.OrphanCandidates{}, nil
}
func (f *fileGCStub) CheckObjectsOnObjectArchived(spaceId, objectId string, isArchived bool) (objectgc.OrphanCandidates, error) {
	return objectgc.OrphanCandidates{}, nil
}
func (f *fileGCStub) RestoreOrphansOnLinksAdded(spaceId, contextId string, addedLinks []string) ([]string, error) {
	return nil, nil
}

// recordingGCStub records whether the GC was consulted and would otherwise produce orphans.
type recordingGCStub struct {
	fileGCStub
	checkCalled bool
}

func (r *recordingGCStub) CheckObjectsOnObjectArchived(spaceId, objectId string, isArchived bool) (objectgc.OrphanCandidates, error) {
	r.checkCalled = true
	return objectgc.OrphanCandidates{Files: []string{"f1"}, Candidates: []string{"c1"}}, nil
}

func TestSetIsArchived_SkipCascade_NoGC(t *testing.T) {
	binId := "bin"
	fx := newFixture(t)
	gc := &recordingGCStub{}
	fx.Service.(*service).objectGC = gc
	sb := smarttest.New(binId)
	sb.AddBlock(simple.New(&model.Block{Id: binId, ChildrenIds: []string{}}))
	fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
		{bundle.RelationKeyId: domain.String("obj1"), bundle.RelationKeySpaceId: domain.String(spaceId)},
	})
	fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Archive: binId})
	fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
		if objectId == binId {
			return editor.NewArchive(sb, fx.store.SpaceIndex(spaceId)), nil
		}
		return smarttest.New(objectId), nil
	})

	// when: archive with skipCascade=true
	err := fx.SetIsArchived(nil, context.Background(), "obj1", true, true)

	// then: object archived, but GC was never consulted (no cascade)
	require.NoError(t, err)
	assert.Len(t, sb.Blocks(), 2)
	assert.False(t, gc.checkCalled, "GC must not run when skipCascade=true")
}

func TestSetIsArchived_EmitsOrphansDetected(t *testing.T) {
	binId := "bin"
	fx := newFixture(t)
	fx.Service.(*service).objectGC = &recordingGCStub{}
	sb := smarttest.New(binId)
	sb.AddBlock(simple.New(&model.Block{Id: binId, ChildrenIds: []string{}}))
	fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
		{bundle.RelationKeyId: domain.String("obj1"), bundle.RelationKeySpaceId: domain.String(spaceId)},
	})
	fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Archive: binId})
	fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
		if objectId == binId {
			return editor.NewArchive(sb, fx.store.SpaceIndex(spaceId)), nil
		}
		return smarttest.New(objectId), nil
	})
	sctx := session.NewContext(session.WithSession("tok"))

	// when: archive with cascade enabled
	err := fx.SetIsArchived(sctx, context.Background(), "obj1", true, false)
	require.NoError(t, err)

	// then: an OrphansDetected event is emitted with the candidates and originating context
	var found *pb.EventObjectOrphansDetected
	for _, m := range sctx.GetMessages() {
		if v, ok := m.Value.(*pb.EventMessageValueOfObjectOrphansDetected); ok {
			found = v.ObjectOrphansDetected
		}
	}
	require.NotNil(t, found)
	assert.Equal(t, "obj1", found.ContextId)
	assert.Equal(t, pb.EventObjectOrphansDetected_archive, found.Trigger)
	assert.ElementsMatch(t, []string{"c1"}, found.ObjectIds)
}

type fixture struct {
	Service
	getter       *mock_cache.MockObjectGetter
	resolver     *mock_idresolver.MockResolver
	spaceService *mock_space.MockService
	store        *objectstore.StoreFixture
	space        *mock_clientspace.MockSpace
	fileSerivce  *mock_fileobject.MockService
}

func newFixture(t *testing.T) *fixture {
	getter := mock_cache.NewMockObjectGetter(t)
	resolver := mock_idresolver.NewMockResolver(t)
	spaceService := mock_space.NewMockService(t)
	store := objectstore.NewStoreFixture(t)

	spc := mock_clientspace.NewMockSpace(t)
	resolver.EXPECT().ResolveSpaceID(mock.Anything).Return(spaceId, nil).Maybe()
	spaceService.EXPECT().Get(mock.Anything, mock.Anything).Return(spc, nil).Maybe()
	fileService := mock_fileobject.NewMockService(t)

	s := &service{
		objectGetter: getter,
		resolver:     resolver,
		spaceService: spaceService,
		store:        store,
		fileService:  fileService,
		objectGC:     &fileGCStub{},
	}

	return &fixture{
		s,
		getter,
		resolver,
		spaceService,
		store,
		spc,
		fileService,
	}
}

func TestService_SetDetailsList(t *testing.T) {
	details := []domain.Detail{
		{Key: bundle.RelationKeyAssignee, Value: domain.String("Mark Twain")},
		{Key: bundle.RelationKeyDone, Value: domain.Bool(true)},
		{Key: bundle.RelationKeyLinkedProjects, Value: domain.StringList([]string{"important", "urgent"})},
	}

	t.Run("no error", func(t *testing.T) {
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

		assert.Equal(t, "Mark Twain", objects["obj1"].NewState().Details().GetString(bundle.RelationKeyAssignee))
		assert.True(t, objects["obj2"].NewState().Details().GetBool(bundle.RelationKeyDone))
		assert.Equal(t, []string{"important", "urgent"}, objects["obj3"].NewState().Details().GetStringList(bundle.RelationKeyLinkedProjects))
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
		{RelationKey: bundle.RelationKeyName.String(), Set: domain.String("My favorite page").ToProto()},
		{RelationKey: bundle.RelationKeyLinks.String(), Add: domain.String("some link").ToProto()},
		{RelationKey: bundle.RelationKeyDone.String(), Set: domain.Bool(true).ToProto()},
	}

	t.Run("no error", func(t *testing.T) {
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

	t.Run("set false value", func(t *testing.T) {
		// given
		fx := newFixture(t)
		object := smarttest.New("obj1")
		err := object.SetDetails(nil, []domain.Detail{{Key: bundle.RelationKeyDone, Value: domain.Bool(true)}}, false)
		require.NoError(t, err)
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			return object, nil
		})
		doneSet := []*pb.RpcObjectListModifyDetailValuesRequestOperation{{
			RelationKey: bundle.RelationKeyDone.String(),
			Set:         pbtypes.Bool(false),
		}}

		// when
		err = fx.ModifyDetailsList(&pb.RpcObjectListModifyDetailValuesRequest{
			ObjectIds:  []string{"obj1"},
			Operations: doneSet,
		})

		// then
		assert.NoError(t, err)
		assert.False(t, object.Details().GetBool(bundle.RelationKeyDone))
	})
}

func TestService_SetSpaceInfo(t *testing.T) {
	var (
		wsObjectId = "workspace"
		details    = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:       domain.String("My space"),
			bundle.RelationKeyIconOption: domain.Int64(5),
			bundle.RelationKeyIconImage:  domain.String("kitten.jpg"),
		})
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
		assert.Equal(t, "My space", ws.NewState().Details().GetString(bundle.RelationKeyName))
		assert.Equal(t, int64(5), ws.NewState().Details().GetInt64(bundle.RelationKeyIconOption))
		assert.Equal(t, "kitten.jpg", ws.NewState().Details().GetString(bundle.RelationKeyIconImage))
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

	t.Run("reject homepage change for 1-on-1 space", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Workspace: wsObjectId})
		fx.space.EXPECT().SpaceType().Return(spacedomain.SpaceTypeOneToOne)
		homepageDetails := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyHomepage: domain.String("someObjectId"),
		})

		// when
		err := fx.SetSpaceInfo(spaceId, homepageDetails)

		// then
		require.ErrorIs(t, err, ErrHomepageChangeRestricted)
	})

	t.Run("allow homepage change for regular space", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ws := smarttest.New(wsObjectId)
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Workspace: wsObjectId})
		fx.space.EXPECT().SpaceType().Return(spacedomain.SpaceTypeRegular)
		fx.space.EXPECT().Id().Return(spaceId)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("someObjectId"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			},
		})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			assert.Equal(t, wsObjectId, objectId)
			return ws, nil
		})
		homepageDetails := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyHomepage: domain.String("someObjectId"),
		})

		// when
		err := fx.SetSpaceInfo(spaceId, homepageDetails)

		// then
		require.NoError(t, err)
		assert.Equal(t, "someObjectId", ws.NewState().Details().GetString(bundle.RelationKeyHomepage))
	})

	t.Run("allow non-homepage changes for 1-on-1 space", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ws := smarttest.New(wsObjectId)
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Workspace: wsObjectId})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			assert.Equal(t, wsObjectId, objectId)
			return ws, nil
		})
		nameDetails := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:       domain.String("Renamed space"),
			bundle.RelationKeyIconOption: domain.Int64(3),
		})

		// when
		err := fx.SetSpaceInfo(spaceId, nameDetails)

		// then
		require.NoError(t, err)
		assert.Equal(t, "Renamed space", ws.NewState().Details().GetString(bundle.RelationKeyName))
		assert.Equal(t, int64(3), ws.NewState().Details().GetInt64(bundle.RelationKeyIconOption))
	})
}

func TestService_SetListIsFavorite(t *testing.T) {
	var (
		objects = []objectstore.TestObject{
			{bundle.RelationKeyId: domain.String("obj1"), bundle.RelationKeySpaceId: domain.String(spaceId)},
			{bundle.RelationKeyId: domain.String("obj2"), bundle.RelationKeySpaceId: domain.String(spaceId)},
			{bundle.RelationKeyId: domain.String("obj3"), bundle.RelationKeySpaceId: domain.String(spaceId)},
		}
		homeId = "home"
	)

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
			{bundle.RelationKeyId: domain.String("obj1"), bundle.RelationKeySpaceId: domain.String(spaceId)},
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

		// when
		err := fx.SetIsArchived(nil, context.Background(), "obj1", true, false)

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
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Archive: binId})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			if objectId == binId {
				return editor.NewArchive(sb, fx.store.SpaceIndex(spaceId)), nil
			}
			obj := smarttest.New(objectId)
			obj.SetType(coresb.SmartBlockTypeProfilePage)
			return obj, nil
		})

		// when
		err := fx.SetIsArchived(nil, context.Background(), "obj1", true, false)

		// then
		assert.Error(t, err)
		assert.ErrorIs(t, err, restriction.ErrRestricted)
	})

	t.Run("can't delete others files", func(t *testing.T) {
		// given
		fx := newFixture(t)
		binSb := smarttest.New(binId)
		binSb.AddBlock(simple.New(&model.Block{Id: binId, ChildrenIds: []string{}}))
		objSb := smarttest.New("obj1")
		objSb.SetType(coresb.SmartBlockTypeFileObject)
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			if objectId == binId {
				return editor.NewArchive(binSb, fx.store.SpaceIndex(spaceId)), nil
			}
			return objSb, nil
		})
		fx.store.AddObjects(t, spaceId, objects)
		fx.fileSerivce.EXPECT().CanDeleteFile(mock.Anything, mock.Anything).Return(fmt.Errorf("not allowed"))
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Archive: binId})

		// when
		err := fx.SetIsArchived(nil, context.Background(), "obj1", true, false)

		// then
		assert.Error(t, err)
	})
}

func TestService_SetListIsArchived(t *testing.T) {
	var (
		objects = []objectstore.TestObject{
			{bundle.RelationKeyId: domain.String("obj1"), bundle.RelationKeySpaceId: domain.String(spaceId)},
			{bundle.RelationKeyId: domain.String("obj2"), bundle.RelationKeySpaceId: domain.String(spaceId)},
			{bundle.RelationKeyId: domain.String("obj3"), bundle.RelationKeySpaceId: domain.String(spaceId)},
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

		// when
		err := fx.SetListIsArchived(nil, context.Background(), []string{"obj1", "obj2", "obj3"}, true, false)

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

		fx.fileSerivce.EXPECT().CanDeleteFile(mock.Anything, mock.Anything).Return(nil).Maybe()
		fx.store.AddObjects(t, spaceId, objects)
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Archive: binId})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			if objectId == binId {
				return editor.NewArchive(sb, fx.store.SpaceIndex(spaceId)), nil
			}
			return smarttest.New(objectId), nil
		})

		// when
		err := fx.SetListIsArchived(nil, context.Background(), []string{"obj1", "obj2", "obj3"}, false, false)

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

		// when
		err := fx.SetListIsArchived(nil, context.Background(), []string{"obj1", "obj2", "obj3"}, true, false)

		// then
		assert.NoError(t, err)
		assert.Len(t, sb.Blocks(), 3)
	})

	t.Run("all updates failed", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, objects)
		fx.fileSerivce.EXPECT().CanDeleteFile(mock.Anything, mock.Anything).Return(nil).Maybe()
		fx.space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Archive: binId})
		fx.getter.EXPECT().GetObject(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, objectId string) (smartblock.SmartBlock, error) {
			return nil, fmt.Errorf("failed to get object")
		})

		// when
		err := fx.SetListIsArchived(nil, context.Background(), []string{"obj1", "obj2", "obj3"}, true, false)

		// then
		assert.Error(t, err)
	})
}
