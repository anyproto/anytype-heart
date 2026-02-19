package export

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/maps"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	editorsb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/converter/pbjson"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/notifications/mock_notifications"
	"github.com/anyproto/anytype-heart/core/relationutils/mock_relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

const spaceId = "space1"

type fixture struct {
	*export
	picker        *mock_cache.MockObjectGetter
	store         *objectstore.StoreFixture
	sbtProvider   *mock_typeprovider.MockSmartBlockTypeProvider
	notifications *mock_notifications.MockNotifications
	process       process.Service
	fetcher       *mock_relationutils.MockRelationFormatFetcher
	sender        *mock_event.MockSender
}

func newFixture(t *testing.T) *fixture {
	objectGetter := mock_cache.NewMockObjectGetter(t)
	storeFixture := objectstore.NewStoreFixture(t)
	provider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
	notifications := mock_notifications.NewMockNotifications(t)
	processSvc := process.New()
	fetcher := mock_relationutils.NewMockRelationFormatFetcher(t)
	mockSender := mock_event.NewMockSender(t)

	a := &app.App{}
	a.Register(testutil.PrepareMock(context.Background(), a, mockSender))
	err := processSvc.Init(a)
	require.NoError(t, err)

	fetcher.EXPECT().GetRelationFormatByKey(mock.Anything, mock.Anything).RunAndReturn(func(_ string, key domain.RelationKey) (model.RelationFormat, error) {
		rel, err := bundle.GetRelation(key)
		if err != nil {
			return 0, err
		}
		return rel.Format, nil
	}).Maybe()

	service := &export{
		picker:              objectGetter,
		objectStore:         storeFixture,
		sbtProvider:         provider,
		notificationService: notifications,
		processService:      processSvc,
		formatFetcher:       fetcher,
	}

	return &fixture{
		export:        service,
		picker:        objectGetter,
		store:         storeFixture,
		sbtProvider:   provider,
		notifications: notifications,
		process:       processSvc,
		fetcher:       fetcher,
		sender:        mockSender,
	}
}

func TestFileNamer_Get(t *testing.T) {
	fn := newNamer()
	names := make(map[string]bool)
	nl := []string{
		"files/some_long_name_12345678901234567890.jpg",
		"files/some_long_name_12345678901234567890.jpg",
		"some_long_name_12345678901234567890.jpg",
		"one.png",
		"two.png",
		"two.png",
		"сделай норм!.pdf",
		"some very long name maybe note or just unreal long title.md",
		"some very long name maybe note or just unreal long title.md",
	}
	for i, v := range nl {
		nm := fn.Get(filepath.Dir(v), fmt.Sprint(i), filepath.Base(v), filepath.Ext(v))
		t.Log(nm)
		names[nm] = true
		assert.NotEmpty(t, nm, v)
	}
	assert.Equal(t, len(names), len(nl))
}

func TestExport_Export(t *testing.T) {
	objectId := "id"
	objectTypeId := "customObjectType"

	t.Run("export success", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.store.AddObjects(t, spaceId, []spaceindex.TestObject{
			prepareTestObjectForStore(objectId, objectTypeId),
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		fx.sender.EXPECT().Broadcast(mock.Anything).Return()

		notificationSend := setupNotificationsMock(fx.notifications)

		// when
		path, success, err := fx.Export(context.Background(), pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			Path:          t.TempDir(),
			ObjectIds:     []string{objectId},
			Format:        model.Export_Protobuf,
			Zip:           true,
			IncludeNested: true,
			IncludeFiles:  true,
			IsJson:        true,
		})

		// then
		<-notificationSend
		assert.NoError(t, err)
		assert.Equal(t, 2, success)

		reader, err := zip.OpenReader(path)
		assert.Nil(t, err)

		assert.Len(t, reader.File, 2)
		fileNames := make(map[string]bool, 2)
		for _, file := range reader.File {
			fileNames[file.Name] = true
		}

		objectPath := filepath.Join(ObjectsDirectory, objectId+".pb.json")
		assert.True(t, fileNames[objectPath])
		typePath := filepath.Join(TypesDirectory, objectTypeId+".pb.json")
		assert.True(t, fileNames[typePath])
	})
	t.Run("export success no progress", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.store.AddObjects(t, spaceId, []spaceindex.TestObject{
			prepareTestObjectForStore(objectId, objectTypeId),
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		// when
		_, success, err := fx.Export(context.Background(), pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			Path:          t.TempDir(),
			ObjectIds:     []string{objectId},
			Format:        model.Export_Protobuf,
			Zip:           true,
			IncludeNested: true,
			IncludeFiles:  true,
			IsJson:        true,
			NoProgress:    true,
		})

		// then
		assert.NoError(t, err)
		assert.Equal(t, 2, success)
		fx.notifications.AssertNotCalled(t, "CreateAndSend")
	})
	t.Run("empty import", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.sender.EXPECT().Broadcast(mock.Anything).Return()

		notificationSend := setupNotificationsMock(fx.notifications)

		// when
		path, success, err := fx.Export(context.Background(), pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			Path:          t.TempDir(),
			ObjectIds:     []string{objectId},
			Format:        model.Export_Protobuf,
			Zip:           true,
			IncludeNested: true,
			IncludeFiles:  true,
			IsJson:        true,
		})

		// then
		<-notificationSend
		assert.NoError(t, err)
		assert.Equal(t, 0, success)

		reader, err := zip.OpenReader(path)
		assert.NoError(t, err)
		assert.Len(t, reader.File, 0)
	})
	t.Run("import finished with error", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []spaceindex.TestObject{
			prepareTestObjectForStore(objectId, objectTypeId),
		})
		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(nil, fmt.Errorf("error"))
		fx.sender.EXPECT().Broadcast(mock.Anything).Return()

		notificationSend := setupNotificationsMock(fx.notifications)

		// when
		_, success, err := fx.Export(context.Background(), pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			Path:          t.TempDir(),
			ObjectIds:     []string{objectId},
			Format:        model.Export_Protobuf,
			Zip:           true,
			IncludeNested: true,
			IncludeFiles:  true,
			IsJson:        true,
		})

		// then
		<-notificationSend
		assert.NotNil(t, err)
		assert.Equal(t, 0, success)
	})
	t.Run("export with filters success", func(t *testing.T) {
		// given
		fx := newFixture(t)
		link := "linkId"

		fx.store.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:             domain.String(link),
				bundle.RelationKeyType:           domain.String(objectTypeId),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyDescription:    domain.String("description"),
				bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_set),
				bundle.RelationKeyCamera:         domain.String("test"),
			},
			prepareTestObjectForStore(objectId, objectTypeId),
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		smartBlockTest.AddBlock(simple.New(&model.Block{Id: objectId, ChildrenIds: []string{"linkBlock"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
		smartBlockTest.AddBlock(simple.New(&model.Block{Id: "linkBlock", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: link}}}))

		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)

		linkObject := setupObject(link, objectTypeId, smartblock.SmartBlockTypePage, map[domain.RelationKey]domain.Value{
			bundle.RelationKeySpaceId:     domain.String(spaceId),
			bundle.RelationKeyDescription: domain.String("description"),
			bundle.RelationKeyLayout:      domain.Int64(model.ObjectType_set),
			bundle.RelationKeyCamera:      domain.String("test"),
		})
		linkObject.AddBlock(simple.New(&model.Block{Id: objectId, ChildrenIds: []string{"linkBlock"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
		linkObject.AddBlock(simple.New(&model.Block{Id: "linkBlock", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "link1"}}}))

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil).Times(4)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)
		fx.picker.EXPECT().GetObject(context.Background(), link).Return(linkObject, nil)

		fx.sender.EXPECT().Broadcast(mock.Anything).Return()
		notificationSend := setupNotificationsMock(fx.notifications)
		fx.sbtProvider.EXPECT().Type(spaceId, link).Return(smartblock.SmartBlockTypePage, nil)

		// when
		path, success, err := fx.Export(context.Background(), pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			Path:          t.TempDir(),
			ObjectIds:     []string{objectId},
			Format:        model.Export_Protobuf,
			Zip:           true,
			IncludeNested: true,
			IncludeFiles:  true,
			IsJson:        true,
			LinksStateFilters: &pb.RpcObjectListExportStateFilters{
				RelationsWhiteList: []*pb.RpcObjectListExportRelationsWhiteList{
					{
						Layout:           model.ObjectType_set,
						AllowedRelations: []string{bundle.RelationKeyCamera.String()},
					},
				},
				RemoveBlocks: true,
			},
		})

		// then
		<-notificationSend
		assert.NoError(t, err)
		assert.Equal(t, 3, success)

		reader, err := zip.OpenReader(path)
		assert.NoError(t, err)

		assert.Len(t, reader.File, 3)
		fileNames := make(map[string]bool, 3)
		for _, file := range reader.File {
			fileNames[file.Name] = true
		}

		objectPath := filepath.Join(ObjectsDirectory, link+".pb.json")
		assert.True(t, fileNames[objectPath])

		file, err := os.Open(objectPath)
		if err != nil {
			return
		}
		var sn *pb.SnapshotWithType
		err = jsonpb.Unmarshal(file, sn)
		assert.NoError(t, err)
		assert.Len(t, sn.GetSnapshot().GetData().GetBlocks(), 1)
		assert.Equal(t, link, sn.GetSnapshot().GetData().GetBlocks()[0].GetId())
		assert.Len(t, sn.GetSnapshot().GetData().GetDetails().GetFields(), 1)
		assert.NotNil(t, sn.GetSnapshot().GetData().GetDetails().GetFields()[bundle.RelationKeyCamera.String()])
		assert.Len(t, sn.GetSnapshot().GetData().GetRelationLinks(), 1)
		assert.Equal(t, bundle.RelationKeyCamera.String(), sn.GetSnapshot().GetData().GetRelationLinks()[0].Key)
	})
	t.Run("export with backlinks", func(t *testing.T) {
		// given
		fx := newFixture(t)
		link1 := "linkId"

		testObject := prepareTestObjectForStore(objectId, objectTypeId)
		testObject[bundle.RelationKeyBacklinks] = domain.StringList([]string{link1})

		fx.store.AddObjects(t, spaceId, []spaceindex.TestObject{
			testObject,
			prepareTestObjectForStore(link1, objectTypeId),
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyBacklinks: domain.StringList([]string{link1}),
		})
		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)
		linkObject := setupObject(link1, objectTypeId, smartblock.SmartBlockTypePage, map[domain.RelationKey]domain.Value{
			bundle.RelationKeySpaceId: domain.String(spaceId),
		})

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil).Times(4)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)
		fx.picker.EXPECT().GetObject(context.Background(), link1).Return(linkObject, nil)

		fx.sender.EXPECT().Broadcast(mock.Anything).Return()
		notificationSend := setupNotificationsMock(fx.notifications)
		fx.sbtProvider.EXPECT().Type(spaceId, link1).Return(smartblock.SmartBlockTypePage, nil)

		// when
		path, success, err := fx.Export(context.Background(), pb.RpcObjectListExportRequest{
			SpaceId:          spaceId,
			Path:             t.TempDir(),
			ObjectIds:        []string{objectId},
			Format:           model.Export_Protobuf,
			Zip:              true,
			IncludeNested:    true,
			IncludeFiles:     true,
			IsJson:           true,
			IncludeBacklinks: true,
		})

		// then
		<-notificationSend
		assert.NoError(t, err)
		assert.Equal(t, 3, success)

		reader, err := zip.OpenReader(path)
		assert.NoError(t, err)

		assert.Len(t, reader.File, 3)
		fileNames := make(map[string]bool, 3)
		for _, file := range reader.File {
			fileNames[file.Name] = true
		}

		objectPath := filepath.Join(ObjectsDirectory, link1+".pb.json")
		assert.True(t, fileNames[objectPath])
	})
	t.Run("export without backlinks", func(t *testing.T) {
		// given
		fx := newFixture(t)
		link1 := "linkId"

		testObject := prepareTestObjectForStore(objectId, objectTypeId)
		testObject[bundle.RelationKeyBacklinks] = domain.StringList([]string{link1})

		fx.store.AddObjects(t, spaceId, []spaceindex.TestObject{
			testObject,
			prepareTestObjectForStore(link1, objectTypeId),
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyBacklinks: domain.StringList([]string{link1}),
		})
		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil).Times(4)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		fx.sender.EXPECT().Broadcast(mock.Anything).Return()
		notificationSend := setupNotificationsMock(fx.notifications)

		// when
		path, success, err := fx.Export(context.Background(), pb.RpcObjectListExportRequest{
			SpaceId:          spaceId,
			Path:             t.TempDir(),
			ObjectIds:        []string{objectId},
			Format:           model.Export_Protobuf,
			Zip:              true,
			IncludeNested:    true,
			IncludeFiles:     true,
			IsJson:           true,
			IncludeBacklinks: false,
		})

		// then
		<-notificationSend
		assert.NoError(t, err)
		assert.Equal(t, 2, success)

		reader, err := zip.OpenReader(path)
		assert.NoError(t, err)

		fileNames := make(map[string]bool, 2)
		for _, file := range reader.File {
			fileNames[file.Name] = true
		}

		objectPath := filepath.Join(ObjectsDirectory, link1+".pb.json")
		assert.False(t, fileNames[objectPath])
	})
	t.Run("export with space", func(t *testing.T) {
		// given
		fx := newFixture(t)
		workspaceId := "workspaceId"
		fx.store.AddObjects(t, spaceId, []spaceindex.TestObject{
			prepareTestObjectForStore(objectId, objectTypeId),
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
			{
				bundle.RelationKeyId:             domain.String(workspaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_space)),
			},
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		workspaceTest := setupObject(workspaceId, objectTypeId, smartblock.SmartBlockTypeWorkspace, nil)
		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)
		fx.picker.EXPECT().GetObject(context.Background(), workspaceId).Return(workspaceTest, nil)

		fx.sender.EXPECT().Broadcast(mock.Anything).Return()
		notificationSend := setupNotificationsMock(fx.notifications)

		spaceService := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Workspace: workspaceId})
		spaceService.EXPECT().Get(context.Background(), spaceId).Return(space, nil)
		fx.export.spaceService = spaceService

		// when
		path, success, err := fx.Export(context.Background(), pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			Path:          t.TempDir(),
			ObjectIds:     []string{objectId},
			Format:        model.Export_Protobuf,
			Zip:           true,
			IncludeNested: true,
			IncludeFiles:  true,
			IsJson:        true,
			IncludeSpace:  true,
		})

		// then
		<-notificationSend
		assert.NoError(t, err)
		assert.Equal(t, 3, success)

		reader, err := zip.OpenReader(path)
		assert.NoError(t, err)

		assert.Len(t, reader.File, 3)
		fileNames := make(map[string]bool, 3)
		for _, file := range reader.File {
			fileNames[file.Name] = true
		}

		objectPath := filepath.Join(ObjectsDirectory, workspaceId+".pb.json")
		assert.True(t, fileNames[objectPath])
	})
}

func Test_docsForExport(t *testing.T) {
	objectId := "id"
	objectTypeId := "customObjectType"
	relationKey := domain.RelationKey("key")
	optionId := "option"

	t.Run("get object with existing links", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String(objectId),
				bundle.RelationKeyName:    domain.String("name1"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeyName:    domain.String("name2"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
		})

		fx.sbtProvider.EXPECT().Type(spaceId, "id1").Return(smartblock.SmartBlockTypePage, nil)

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		smartBlockTest.AddBlock(simple.New(&model.Block{Id: objectId, ChildrenIds: []string{"linkBlock"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
		smartBlockTest.AddBlock(simple.New(&model.Block{Id: "linkBlock", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "id1"}}}))
		linkObject := setupObject("id1", objectTypeId, smartblock.SmartBlockTypePage, nil)

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), "id1").Return(linkObject, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{objectId},
			IncludeNested: true,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})
	t.Run("get object with non existing links", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String(objectId),
				bundle.RelationKeyName:    domain.String("name"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:        domain.String("id1"),
				bundle.RelationKeyIsDeleted: domain.Bool(true),
				bundle.RelationKeySpaceId:   domain.String(spaceId),
			},
		})
		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		smartBlockTest.AddBlock(simple.New(&model.Block{Id: objectId, ChildrenIds: []string{"linkBlock"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
		smartBlockTest.AddBlock(simple.New(&model.Block{Id: "linkBlock", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "id1"}}}))
		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)

		fx.sbtProvider.EXPECT().Type(spaceId, "id1").Return(smartblock.SmartBlockTypePage, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{objectId},
			IncludeNested: true,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 1, len(expCtx.docs))
	})
	t.Run("get object with non existing relation", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String(objectId),
				relationKey:               domain.String("value"),
				bundle.RelationKeyType:    domain.String("customObjectType"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
		})
		err := fx.store.SpaceIndex(spaceId).UpdateObjectLinks(context.Background(), objectId, []string{"id1"})
		require.Nil(t, err)

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			relationKey: domain.String("value"),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    relationKey.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:   spaceId,
			ObjectIds: []string{objectId},
			Format:    model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 1, len(expCtx.docs))
	})
	t.Run("get object with existing relation", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String(objectId),
				relationKey:               domain.String("value"),
				bundle.RelationKeyType:    domain.String(objectTypeId),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			prepareTestRelationForStore(t, relationKey, 0),
		})

		err := fx.store.SpaceIndex(spaceId).UpdateObjectLinks(context.Background(), objectId, []string{"id1"})
		require.NoError(t, err)

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			relationKey: domain.String("value"),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    relationKey.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:   spaceId,
			ObjectIds: []string{objectId},
			Format:    model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})

	t.Run("get relation options - no relation options", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id"),
				relationKey:               domain.String("value"),
				bundle.RelationKeyType:    domain.String(objectTypeId),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			prepareTestRelationForStore(t, relationKey, int64(model.RelationFormat_status)),
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			relationKey: domain.String("value"),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    relationKey.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{objectId},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})
	t.Run("get relation options - 1 relation option", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String(objectId),
				relationKey:               domain.String(optionId),
				bundle.RelationKeyType:    domain.String(objectTypeId),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			prepareTestRelationForStore(t, relationKey, int64(model.RelationFormat_tag)),
			prepareTestOptionForStore(t, relationKey, optionId),
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			relationKey: domain.String("value"),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    relationKey.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{objectId},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 3, len(expCtx.docs))
		var ObjectIds []string
		for objectId := range expCtx.docs {
			ObjectIds = append(ObjectIds, objectId)
		}
		assert.Contains(t, ObjectIds, optionId)
	})
	t.Run("get derived objects - relation, object type with recommended relations, template with link", func(t *testing.T) {
		// given
		fx := newFixture(t)
		recommendedRelationKey := domain.RelationKey("recommendedRelationKey")
		templateId := "templateId"
		templateObjectTypeId := "templateObjectTypeId"

		linkedObjectId := "linkedObjectId"
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String(objectId),
				relationKey:               domain.String("test"),
				bundle.RelationKeyType:    domain.String(objectTypeId),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			prepareTestRelationForStore(t, relationKey, int64(model.RelationFormat_longtext)),
			prepareTestObjectTypeForStore(t, objectTypeId, []string{recommendedRelationKey.String()}),
			prepareTestRelationForStore(t, recommendedRelationKey, int64(model.RelationFormat_tag)),
			{
				bundle.RelationKeyId:               domain.String(templateId),
				bundle.RelationKeyTargetObjectType: domain.String(objectTypeId),
				bundle.RelationKeySpaceId:          domain.String(spaceId),
				bundle.RelationKeyType:             domain.String(templateObjectTypeId),
			},
			prepareTestObjectForStore(linkedObjectId, objectTypeId),
		})

		template := setupObject(templateId, templateObjectTypeId, smartblock.SmartBlockTypeTemplate, nil)
		template.AddBlock(simple.New(&model.Block{Id: templateId, ChildrenIds: []string{"linkBlock"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
		template.AddBlock(simple.New(&model.Block{Id: "linkBlock", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: linkedObjectId}}}))

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			relationKey: domain.String("value"),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    relationKey.String(),
			Format: model.RelationFormat_tag,
		})
		smartBlockTest.Doc = doc

		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)
		templateObjectType := setupObject(templateObjectTypeId, templateObjectTypeId, smartblock.SmartBlockTypeObjectType, nil)
		linkedObject := setupObject(linkedObjectId, objectTypeId, smartblock.SmartBlockTypePage, nil)

		fx.picker.EXPECT().GetObject(context.Background(), mock.Anything).RunAndReturn(func(_ context.Context, id string) (editorsb.SmartBlock, error) {
			switch id {
			case objectId:
				return smartBlockTest, nil
			case templateId:
				return template, nil
			case objectTypeId:
				return objectType, nil
			case templateObjectTypeId:
				return templateObjectType, nil
			case linkedObjectId:
				return linkedObject, nil
			}
			return nil, fmt.Errorf("not found")
		}).Maybe()

		fx.sbtProvider.EXPECT().Type(spaceId, linkedObjectId).Return(smartblock.SmartBlockTypePage, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{objectId},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 6, len(expCtx.docs))
	})
	t.Run("get derived objects, object type have missing relations - return only object and its type", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			prepareTestObjectForStore(objectId, objectTypeId),
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{objectId},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})
	t.Run("objects without links", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			prepareTestObjectForStore(objectId, objectTypeId),
			prepareTestObjectForStore("object2", objectTypeId),
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{objectId},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})
	t.Run("objects with dataview", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String(objectId),
				bundle.RelationKeyName:           domain.String("name1"),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_set)),
			},
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
			prepareTestRelationForStore(t, bundle.RelationKeyTag, int64(model.RelationFormat_tag)),
			prepareTestRelationForStore(t, relationKey, 0),
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_set)),
		})
		doc := smartBlockTest.NewState()
		doc.Set(simple.New(&model.Block{
			Id:          objectId,
			ChildrenIds: []string{"blockId"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}))
		doc.Set(simple.New(&model.Block{
			Id: "blockId",
			Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Relations: []*model.BlockContentDataviewRelation{
							{
								Key: bundle.RelationKeyTag.String(),
							},
							{
								Key: relationKey.String(),
							},
						},
					},
				},
			}},
		}))
		smartBlockTest.Doc = doc

		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{objectId},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 4, len(expCtx.docs))
	})
	t.Run("objects without file", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String(objectId),
				bundle.RelationKeyName:           domain.String("name1"),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_set)),
			},
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_set)),
		})
		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{objectId},
			IncludeNested: true,
			IncludeFiles:  true,
			Format:        model.Export_Protobuf,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})
	t.Run("objects without file, not protobuf export", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String(objectId),
				bundle.RelationKeyName:           domain.String("name1"),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_set)),
			},
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{objectId},
			IncludeNested: true,
			IncludeFiles:  true,
			Format:        model.Export_Markdown,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 1, len(expCtx.docs))
	})

	t.Run("get derived objects - relation, object type with recommended relations, template with link", func(t *testing.T) {
		// given
		fx := newFixture(t)
		recommendedRelationKey := domain.RelationKey("recommendedRelationKey")

		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String(objectId),
				bundle.RelationKeySetOf:   domain.StringList([]string{relationKey.String()}),
				bundle.RelationKeyType:    domain.String(objectTypeId),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			prepareTestRelationForStore(t, relationKey, 0),
			prepareTestObjectTypeForStore(t, objectTypeId, []string{recommendedRelationKey.String()}),
			prepareTestRelationForStore(t, recommendedRelationKey, 0),
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, map[domain.RelationKey]domain.Value{
			bundle.RelationKeySetOf: domain.StringList([]string{relationKey.String()}),
		})
		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)
		relationObject := setupObject(relationKey.String(), objectTypeId, smartblock.SmartBlockTypeRelation, nil)

		fx.picker.EXPECT().GetObject(context.Background(), mock.Anything).RunAndReturn(func(_ context.Context, id string) (editorsb.SmartBlock, error) {
			switch id {
			case objectId:
				return smartBlockTest, nil
			case objectTypeId:
				return objectType, nil
			case relationKey.String():
				return relationObject, nil
			}
			return nil, fmt.Errorf("not found")
		}).Maybe()

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:   spaceId,
			ObjectIds: []string{objectId},
			Format:    model.Export_Protobuf,
		})

		// when
		err := expCtx.docsForExport(nil)
		// then
		assert.NoError(t, err)
		assert.Equal(t, 4, len(expCtx.docs))
	})
	t.Run("export template", func(t *testing.T) {
		// given
		fx := newFixture(t)

		templateType := "templateType"
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               domain.String(objectId),
				bundle.RelationKeyName:             domain.String("template"),
				bundle.RelationKeySpaceId:          domain.String(spaceId),
				bundle.RelationKeyTargetObjectType: domain.String(objectTypeId),
				bundle.RelationKeyType:             domain.String(templateType),
			},
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
			prepareTestObjectTypeForStore(t, templateType, nil),
		})

		smartBlockTest := setupObject(objectId, templateType, smartblock.SmartBlockTypePage, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyTargetObjectType: domain.String(objectTypeId),
		})
		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)
		templateTypeObject := setupObject(templateType, templateType, smartblock.SmartBlockTypeObjectType, nil)

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)
		fx.picker.EXPECT().GetObject(context.Background(), templateType).Return(templateTypeObject, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{objectId},
			IncludeNested: false,
			Format:        model.Export_Protobuf,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 3, len(expCtx.docs))
	})
	t.Run("add default object type and template from dataview", func(t *testing.T) {
		// given
		fx := newFixture(t)

		defaultObjectTypeId := "defaultObjectTypeId"
		defaultTemplateId := "defaultTemplateId"
		defaultObjectTypeTemplateId := "defaultObjectTypeTemplateId"

		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String(objectId),
				bundle.RelationKeyName:           domain.String("name"),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
			},
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
			prepareTestObjectTypeForStore(t, defaultObjectTypeId, nil),
			prepareTestObjectForStore(defaultTemplateId, bundle.TypeKeyTemplate.String()),
			{
				bundle.RelationKeyId:               domain.String(defaultObjectTypeTemplateId),
				bundle.RelationKeySpaceId:          domain.String(spaceId),
				bundle.RelationKeyType:             domain.String(bundle.TypeKeyTemplate),
				bundle.RelationKeyTargetObjectType: domain.String(defaultObjectTypeId),
			},
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		doc := smartBlockTest.NewState()
		doc.Set(simple.New(&model.Block{
			Id:          objectId,
			ChildrenIds: []string{"blockId"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}))
		doc.Set(simple.New(&model.Block{
			Id: "blockId",
			Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Id:                  "viewId",
						DefaultObjectTypeId: defaultObjectTypeId,
					},
					{
						Id:                "viewId2",
						DefaultTemplateId: defaultTemplateId,
					},
				},
			}},
		}))
		smartBlockTest.Doc = doc

		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)
		defaultObjectType := setupObject(defaultObjectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)
		defaultTemplate := setupObject(defaultTemplateId, objectTypeId, smartblock.SmartBlockTypeTemplate, nil)
		defaultObjectTypeTemplate := setupObject(defaultObjectTypeTemplateId, objectTypeId, smartblock.SmartBlockTypeTemplate, nil)

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)
		fx.picker.EXPECT().GetObject(context.Background(), defaultObjectTypeId).Return(defaultObjectType, nil)
		fx.picker.EXPECT().GetObject(context.Background(), defaultTemplateId).Return(defaultTemplate, nil)
		fx.picker.EXPECT().GetObject(context.Background(), defaultObjectTypeTemplateId).Return(defaultObjectTypeTemplate, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:   spaceId,
			ObjectIds: []string{objectId},
			Format:    model.Export_Protobuf,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		require.NoError(t, err)
		assert.Equal(t, 5, len(expCtx.docs))
	})
	t.Run("add default object type and template from dataview of set", func(t *testing.T) {
		// given
		fx := newFixture(t)

		defaultObjectTypeId := "defaultObjectTypeId"
		defaultTemplateId := "defaultTemplateId"
		defaultObjectTypeTemplateId := "defaultObjectTypeTemplateId"

		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String(objectId),
				bundle.RelationKeyName:           domain.String("name"),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_set)),
			},
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
			prepareTestObjectTypeForStore(t, defaultObjectTypeId, nil),
			prepareTestObjectForStore(defaultTemplateId, objectTypeId),
			{
				bundle.RelationKeyId:               domain.String(defaultObjectTypeTemplateId),
				bundle.RelationKeySpaceId:          domain.String(spaceId),
				bundle.RelationKeyType:             domain.String(objectTypeId),
				bundle.RelationKeyTargetObjectType: domain.String(defaultObjectTypeId),
			},
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		doc := smartBlockTest.NewState()
		doc.Set(simple.New(&model.Block{
			Id:          objectId,
			ChildrenIds: []string{"blockId"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}))
		doc.Set(simple.New(&model.Block{
			Id: "blockId",
			Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Id:                  "viewId",
						DefaultObjectTypeId: defaultObjectTypeId,
					},
					{
						Id:                "viewId2",
						DefaultTemplateId: defaultTemplateId,
					},
				},
			}},
		}))
		smartBlockTest.Doc = doc

		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)
		defaultObjectType := setupObject(defaultObjectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)
		defaultTemplate := setupObject(defaultTemplateId, objectTypeId, smartblock.SmartBlockTypeTemplate, nil)
		defaultObjectTypeTemplate := setupObject(defaultObjectTypeTemplateId, objectTypeId, smartblock.SmartBlockTypeTemplate, nil)

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)
		fx.picker.EXPECT().GetObject(context.Background(), defaultObjectTypeId).Return(defaultObjectType, nil)
		fx.picker.EXPECT().GetObject(context.Background(), defaultTemplateId).Return(defaultTemplate, nil)
		fx.picker.EXPECT().GetObject(context.Background(), defaultObjectTypeTemplateId).Return(defaultObjectTypeTemplate, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:   spaceId,
			ObjectIds: []string{objectId},
			Format:    model.Export_Protobuf,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		require.NoError(t, err)
		assert.Equal(t, 5, len(expCtx.docs))
	})
	t.Run("no default object type and template from dataview", func(t *testing.T) {
		// given
		fx := newFixture(t)

		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String(objectId),
				bundle.RelationKeyName:    domain.String("name"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
				bundle.RelationKeyType:    domain.String(objectTypeId),
				bundle.RelationKeyLayout:  domain.Int64(int64(model.ObjectType_collection)),
			},
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, nil)
		doc := smartBlockTest.NewState()
		doc.Set(simple.New(&model.Block{
			Id:          objectId,
			ChildrenIds: []string{"blockId"},
			Content:     &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}))
		doc.Set(simple.New(&model.Block{
			Id: "blockId",
			Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{
					{
						Id: "viewId",
					},
					{
						Id: "viewId2",
					},
				},
			}},
		}))
		smartBlockTest.Doc = doc

		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:   spaceId,
			ObjectIds: []string{objectId},
			Format:    model.Export_Protobuf,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		assert.NoError(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})

	t.Run("export participant", func(t *testing.T) {
		// given
		fx := newFixture(t)

		participantId := domain.NewParticipantId(spaceId, "identity")
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			prepareTestObjectForStore(objectId, objectTypeId),
			prepareTestObjectForStore(participantId, objectTypeId),
			prepareTestObjectTypeForStore(t, objectTypeId, nil),
		})

		smartBlockTest := setupObject(objectId, objectTypeId, smartblock.SmartBlockTypePage, map[domain.RelationKey]domain.Value{
			bundle.RelationKeyLastModifiedBy: domain.String(participantId),
			bundle.RelationKeyCreator:        domain.String(participantId),
		})
		objectType := setupObject(objectTypeId, objectTypeId, smartblock.SmartBlockTypeObjectType, nil)
		participant := setupObject(participantId, objectTypeId, smartblock.SmartBlockTypeParticipant, nil)

		fx.picker.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		fx.picker.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)
		fx.picker.EXPECT().GetObject(context.Background(), participantId).Return(participant, nil)

		fx.sbtProvider.EXPECT().Type(spaceId, participantId).Return(smartblock.SmartBlockTypeParticipant, nil)

		expCtx := newExportContext(fx.export, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{objectId},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		require.NoError(t, err)
		assert.Equal(t, 3, len(expCtx.docs))
		assert.NotNil(t, expCtx.docs[participantId])
	})
}

func Test_provideFileName(t *testing.T) {
	t.Run("file dir for relation", func(t *testing.T) {
		// when
		fileName := makeFileName("docId", spaceId, pbjson.NewConverter(nil).Ext(), nil, smartblock.SmartBlockTypeRelation)

		// then
		assert.Equal(t, RelationsDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("file dir for relation option", func(t *testing.T) {
		// when
		fileName := makeFileName("docId", spaceId, pbjson.NewConverter(nil).Ext(), nil, smartblock.SmartBlockTypeRelationOption)

		// then
		assert.Equal(t, RelationsOptionsDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("file dir for types", func(t *testing.T) {
		// when
		fileName := makeFileName("docId", spaceId, pbjson.NewConverter(nil).Ext(), nil, smartblock.SmartBlockTypeObjectType)

		// then
		assert.Equal(t, TypesDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("file dir for objects", func(t *testing.T) {
		// when
		fileName := makeFileName("docId", spaceId, pbjson.NewConverter(nil).Ext(), nil, smartblock.SmartBlockTypePage)

		// then
		assert.Equal(t, ObjectsDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("file dir for files objects", func(t *testing.T) {
		// when
		fileName := makeFileName("docId", spaceId, pbjson.NewConverter(nil).Ext(), nil, smartblock.SmartBlockTypeFileObject)

		// then
		assert.Equal(t, FilesObjects+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("space is not provided", func(t *testing.T) {
		// given
		st := state.NewDoc("root", nil).(*state.State)
		st.SetDetail(bundle.RelationKeySpaceId, domain.String(spaceId))

		// when
		fileName := makeFileName("docId", "", pbjson.NewConverter(st).Ext(), st, smartblock.SmartBlockTypeFileObject)

		// then
		assert.Equal(t, spaceDirectory+string(filepath.Separator)+spaceId+string(filepath.Separator)+FilesObjects+string(filepath.Separator)+"docId.pb.json", fileName)
	})
}

func Test_queryObjectsFromStoreByIds(t *testing.T) {
	t.Run("query 10 objects", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		ids := make([]string, 0, 10)
		for i := 0; i < 10; i++ {
			id := fmt.Sprintf("%d", i)
			store.AddObjects(t, spaceId, []objectstore.TestObject{
				{
					bundle.RelationKeyId:      domain.String(id),
					bundle.RelationKeySpaceId: domain.String(spaceId),
				},
			})
			ids = append(ids, id)
		}
		e := &export{objectStore: store}
		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{})

		// when
		records, err := expCtx.queryAndFilterObjectsByRelation(spaceId, ids, bundle.RelationKeyId)

		// then
		require.NoError(t, err)
		assert.Len(t, records, 10)
	})
	t.Run("query 2000 objects", func(t *testing.T) {
		// given
		fixture := objectstore.NewStoreFixture(t)
		ids := make([]string, 0, 2000)
		for i := 0; i < 2000; i++ {
			id := fmt.Sprintf("%d", i)
			fixture.AddObjects(t, spaceId, []objectstore.TestObject{
				{
					bundle.RelationKeyId:      domain.String(id),
					bundle.RelationKeySpaceId: domain.String(spaceId),
				},
			})
			ids = append(ids, id)
		}
		e := &export{objectStore: fixture}
		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{})

		// when
		records, err := expCtx.queryAndFilterObjectsByRelation(spaceId, ids, bundle.RelationKeyId)

		// then
		assert.NoError(t, err)
		assert.Len(t, records, 2000)
	})
}

func TestExport_CollectionFilterMissing(t *testing.T) {
	t.Run("collection with non-existing objects", func(t *testing.T) {
		storeFixture := objectstore.NewStoreFixture(t)

		collectionId := "collection1"
		existingobjectId := "object1"
		missingobjectId := "object2"
		deletedobjectId := "object3"

		storeFixture.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:             domain.String(collectionId),
				bundle.RelationKeyType:           domain.String(bundle.TypeKeyCollection),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:      domain.String(existingobjectId),
				bundle.RelationKeyType:    domain.String(bundle.TypeKeyPage),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:        domain.String(deletedobjectId),
				bundle.RelationKeyType:      domain.String(bundle.TypeKeyPage),
				bundle.RelationKeySpaceId:   domain.String(spaceId),
				bundle.RelationKeyIsDeleted: domain.Bool(true),
			},
		})

		e := &export{objectStore: storeFixture}
		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId: spaceId,
			Format:  model.Export_Protobuf,
		})

		collectionDetails, _ := storeFixture.GetDetails(spaceId, collectionId)
		existingObjectDetails, _ := storeFixture.GetDetails(spaceId, existingobjectId)
		expCtx.docs = map[string]*Doc{
			collectionId:     {Details: collectionDetails},
			existingobjectId: {Details: existingObjectDetails},
		}

		collectionState := state.NewDoc(collectionId, map[string]simple.Block{
			collectionId: simple.New(&model.Block{Id: collectionId}),
		}).(*state.State)
		collectionState.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
		}))
		collectionState.UpdateStoreSlice(template.CollectionStoreKey, []string{existingobjectId, missingobjectId, deletedobjectId})

		originalIds := collectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, originalIds, 3)

		expCtx.collectionFilterMissing(collectionState)

		processedIds := collectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, processedIds, 1)
		assert.Equal(t, []string{existingobjectId}, processedIds)
	})

	t.Run("collection with all existing objects", func(t *testing.T) {
		storeFixture := objectstore.NewStoreFixture(t)

		collectionId := "collection1"
		object1Id := "object1"
		object2Id := "object2"

		storeFixture.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:             domain.String(collectionId),
				bundle.RelationKeyType:           domain.String(bundle.TypeKeyCollection),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:      domain.String(object1Id),
				bundle.RelationKeyType:    domain.String(bundle.TypeKeyPage),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:      domain.String(object2Id),
				bundle.RelationKeyType:    domain.String(bundle.TypeKeyPage),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
		})

		e := &export{objectStore: storeFixture}
		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId: spaceId,
			Format:  model.Export_Protobuf,
		})

		collectionDetails, _ := storeFixture.GetDetails(spaceId, collectionId)
		object1Details, _ := storeFixture.GetDetails(spaceId, object1Id)
		object2Details, _ := storeFixture.GetDetails(spaceId, object2Id)
		expCtx.docs = map[string]*Doc{
			collectionId: {Details: collectionDetails},
			object1Id:    {Details: object1Details},
			object2Id:    {Details: object2Details},
		}

		collectionState := state.NewDoc(collectionId, map[string]simple.Block{
			collectionId: simple.New(&model.Block{Id: collectionId}),
		}).(*state.State)
		collectionState.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
		}))
		collectionState.UpdateStoreSlice(template.CollectionStoreKey, []string{object1Id, object2Id})

		originalIds := collectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, originalIds, 2)

		expCtx.collectionFilterMissing(collectionState)

		processedIds := collectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, processedIds, 2)
		assert.Equal(t, []string{object1Id, object2Id}, processedIds)
	})

	t.Run("empty collection", func(t *testing.T) {
		storeFixture := objectstore.NewStoreFixture(t)

		collectionId := "collection1"

		storeFixture.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:             domain.String(collectionId),
				bundle.RelationKeyType:           domain.String(bundle.TypeKeyCollection),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
			},
		})

		e := &export{objectStore: storeFixture}
		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId: spaceId,
			Format:  model.Export_Protobuf,
		})

		collectionDetails, _ := storeFixture.GetDetails(spaceId, collectionId)
		expCtx.docs = map[string]*Doc{
			collectionId: {Details: collectionDetails},
		}

		collectionState := state.NewDoc(collectionId, map[string]simple.Block{
			collectionId: simple.New(&model.Block{Id: collectionId}),
		}).(*state.State)
		collectionState.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
		}))
		collectionState.UpdateStoreSlice(template.CollectionStoreKey, []string{})

		originalIds := collectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, originalIds, 0)

		expCtx.collectionFilterMissing(collectionState)

		processedIds := collectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, processedIds, 0)
		assert.Equal(t, []string{}, processedIds)
	})
}

func TestExport_ExportCollectionWithNonExistingObjects(t *testing.T) {
	t.Run("export collection with missing objects filters them out 2", func(t *testing.T) {
		// given
		fx := newFixture(t)
		collectionId := "collection1"
		existingObject1 := "object1"
		existingObject2 := "object2"
		missingObject := "missingObject"

		fx.store.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:             domain.String(collectionId),
				bundle.RelationKeyType:           domain.String(bundle.TypeKeyCollection),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
			},
			prepareTestObjectForStore(existingObject1, bundle.TypeKeyPage.String()),
			prepareTestObjectForStore(existingObject2, bundle.TypeKeyPage.String()),
		})

		collectionBlock := smarttest.New(collectionId)
		collectionDoc := collectionBlock.NewState()
		collectionDoc.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:             domain.String(collectionId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
			bundle.RelationKeyType:           domain.String(bundle.TypeKeyCollection),
		}))
		collectionDoc.UpdateStoreSlice(template.CollectionStoreKey, []string{existingObject1, missingObject, existingObject2})
		collectionBlock.Doc = collectionDoc

		object1Block := setupObject(existingObject1, bundle.TypeKeyPage.String(), smartblock.SmartBlockTypePage, nil)
		object2Block := setupObject(existingObject2, bundle.TypeKeyPage.String(), smartblock.SmartBlockTypePage, nil)

		fx.picker.EXPECT().GetObject(mock.Anything, mock.Anything).Return(collectionBlock, nil).Maybe()
		fx.picker.EXPECT().GetObject(mock.Anything, mock.Anything).Return(object1Block, nil).Maybe()
		fx.picker.EXPECT().GetObject(mock.Anything, mock.Anything).Return(object2Block, nil).Maybe()
		fx.picker.EXPECT().GetObject(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("not found")).Maybe()

		fx.sbtProvider.EXPECT().Type(mock.Anything, mock.Anything).Return(smartblock.SmartBlockTypePage, nil).Maybe()
		notificationSend := setupNotificationsMock(fx.notifications)
		fx.sender.EXPECT().Broadcast(mock.Anything).Return()

		// when
		path, succeed, err := fx.Export(context.Background(), pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			Path:          t.TempDir(),
			Format:        model.Export_Protobuf,
			ObjectIds:     []string{collectionId},
			Zip:           true,
			IncludeNested: true,
			IncludeFiles:  false,
			IsJson:        true,
		})

		// then
		<-notificationSend
		assert.NoError(t, err)
		assert.NotEmpty(t, path)
		assert.Equal(t, 3, int(succeed))

		reader, err := zip.OpenReader(path)
		require.NoError(t, err)
		defer reader.Close()

		var collectionFile *zip.File
		expectedPath := filepath.Join("objects", collectionId+".pb.json")

		for _, f := range reader.File {
			if f.Name == expectedPath {
				collectionFile = f
				break
			}
		}
		require.NotNil(t, collectionFile, "Collection file not found in export")

		rc, err := collectionFile.Open()
		require.NoError(t, err)
		defer rc.Close()

		data, err := io.ReadAll(rc)
		require.NoError(t, err)

		var snapshotWithType pb.SnapshotWithType
		unmarshaler := &jsonpb.Unmarshaler{AllowUnknownFields: true}
		err = unmarshaler.Unmarshal(strings.NewReader(string(data)), &snapshotWithType)
		require.NoError(t, err)

		var collectionObjects []string

		snapshot := snapshotWithType.GetSnapshot()
		require.NotNil(t, snapshot, "Snapshot should not be nil")

		collections := snapshot.GetData().GetCollections()
		require.NotNil(t, collections, "Collections should not be nil")

		if objectsField := collections.GetFields()[template.CollectionStoreKey]; objectsField != nil {
			if objectsList := objectsField.GetListValue(); objectsList != nil {
				for _, obj := range objectsList.GetValues() {
					if objStr := obj.GetStringValue(); objStr != "" {
						collectionObjects = append(collectionObjects, objStr)
					}
				}
			}
		}

		assert.Len(t, collectionObjects, 2, "Collection should contain exactly 2 objects (missing object filtered out)")
		assert.Contains(t, collectionObjects, existingObject1, "Collection should contain existing object 1")
		assert.Contains(t, collectionObjects, existingObject2, "Collection should contain existing object 2")
		assert.NotContains(t, collectionObjects, missingObject, "Collection should not contain missing object")

		os.Remove(path)
	})
}

func setupObject(id, typeId string, sbType smartblock.SmartBlockType, details map[domain.RelationKey]domain.Value) *smarttest.SmartTest {
	smartBlockTest := smarttest.New(id)
	if details == nil {
		details = map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(id),
			bundle.RelationKeyType: domain.String(typeId),
		}
	} else {
		details[bundle.RelationKeyId] = domain.String(id)
		details[bundle.RelationKeyType] = domain.String(typeId)
	}
	doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(details))
	doc.AddBundledRelationLinks(maps.Keys(details)...)
	smartBlockTest.Doc = doc
	smartBlockTest.SetType(sbType)
	return smartBlockTest
}

func setupNotificationsMock(notif *mock_notifications.MockNotifications) chan struct{} {
	notificationSend := make(chan struct{})
	notif.EXPECT().CreateAndSend(mock.Anything).RunAndReturn(func(notification *model.Notification) error {
		close(notificationSend)
		return nil
	})
	return notificationSend
}

func prepareTestObjectForStore(id, typeId string) spaceindex.TestObject {
	return spaceindex.TestObject{
		bundle.RelationKeyId:      domain.String(id),
		bundle.RelationKeyType:    domain.String(typeId),
		bundle.RelationKeySpaceId: domain.String(spaceId),
	}
}

func prepareTestObjectTypeForStore(t *testing.T, typeId string, recommendedRelations []string) spaceindex.TestObject {
	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, typeId)
	require.NoError(t, err)
	return spaceindex.TestObject{
		bundle.RelationKeyId:                   domain.String(typeId),
		bundle.RelationKeyUniqueKey:            domain.String(uk.Marshal()),
		bundle.RelationKeyLayout:               domain.Int64(int64(model.ObjectType_objectType)),
		bundle.RelationKeyRecommendedRelations: domain.StringList(recommendedRelations),
		bundle.RelationKeySpaceId:              domain.String(spaceId),
		bundle.RelationKeyType:                 domain.String(typeId),
	}
}

func prepareTestRelationForStore(t *testing.T, relationKey domain.RelationKey, format int64) spaceindex.TestObject {
	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey.String())
	require.NoError(t, err)
	return spaceindex.TestObject{
		bundle.RelationKeyId:             domain.String(relationKey),
		bundle.RelationKeyRelationKey:    domain.String(relationKey),
		bundle.RelationKeyUniqueKey:      domain.String(uk.Marshal()),
		bundle.RelationKeyRelationFormat: domain.Int64(format),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		bundle.RelationKeySpaceId:        domain.String(spaceId),
	}
}

func prepareTestOptionForStore(t *testing.T, relationKey domain.RelationKey, optionId string) spaceindex.TestObject {
	uk, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelationOption, optionId)
	require.NoError(t, err)
	return spaceindex.TestObject{
		bundle.RelationKeyId:             domain.String(optionId),
		bundle.RelationKeyRelationKey:    domain.String(relationKey),
		bundle.RelationKeyUniqueKey:      domain.String(uk.Marshal()),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
		bundle.RelationKeySpaceId:        domain.String(spaceId),
	}
}
