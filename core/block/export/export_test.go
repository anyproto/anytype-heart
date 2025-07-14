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

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/converter/pbjson"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/notifications/mock_notifications"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

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

const spaceId = "space1"

func TestExport_Export(t *testing.T) {
	t.Run("export success", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "customObjectType"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)

		objectID := "id"
		storeFixture.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:      domain.String(objectID),
				bundle.RelationKeyType:    domain.String(objectTypeId),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:                   domain.String(objectTypeId),
				bundle.RelationKeyUniqueKey:            domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout:       domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{addr.MissingObject}),
				bundle.RelationKeySpaceId:              domain.String(spaceId),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New(objectID)
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectID),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc
		objectType.SetType(smartblock.SmartBlockTypeObjectType)
		objectGetter.EXPECT().GetObject(context.Background(), objectID).Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		a := &app.App{}
		mockSender := mock_event.NewMockSender(t)
		mockSender.EXPECT().Broadcast(mock.Anything).Return()
		a.Register(testutil.PrepareMock(context.Background(), a, mockSender))
		service := process.New()
		err = service.Init(a)
		assert.Nil(t, err)

		notifications := mock_notifications.NewMockNotifications(t)
		notificationSend := make(chan struct{})
		notifications.EXPECT().CreateAndSend(mock.Anything).RunAndReturn(func(notification *model.Notification) error {
			close(notificationSend)
			return nil
		})

		e := &export{
			objectStore:         storeFixture,
			picker:              objectGetter,
			processService:      service,
			notificationService: notifications,
		}

		// when
		path, success, err := e.Export(context.Background(), pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			Path:          t.TempDir(),
			ObjectIds:     []string{objectID},
			Format:        model.Export_Protobuf,
			Zip:           true,
			IncludeNested: true,
			IncludeFiles:  true,
			IsJson:        true,
		})

		// then
		<-notificationSend
		assert.Nil(t, err)
		assert.Equal(t, 2, success)

		reader, err := zip.OpenReader(path)
		assert.Nil(t, err)

		assert.Len(t, reader.File, 2)
		fileNames := make(map[string]bool, 2)
		for _, file := range reader.File {
			fileNames[file.Name] = true
		}

		objectPath := filepath.Join(objectsDirectory, objectID+".pb.json")
		assert.True(t, fileNames[objectPath])
		typePath := filepath.Join(typesDirectory, objectTypeId+".pb.json")
		assert.True(t, fileNames[typePath])
	})
	t.Run("export success no progress", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "customObjectType"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)

		objectID := "id"
		storeFixture.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:      domain.String(objectID),
				bundle.RelationKeyType:    domain.String(objectTypeId),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:                   domain.String(objectTypeId),
				bundle.RelationKeyUniqueKey:            domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:               domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{addr.MissingObject}),
				bundle.RelationKeySpaceId:              domain.String(spaceId),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New(objectID)
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectID),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc
		objectType.SetType(smartblock.SmartBlockTypeObjectType)
		objectGetter.EXPECT().GetObject(context.Background(), objectID).Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		a := &app.App{}
		mockSender := mock_event.NewMockSender(t)
		a.Register(testutil.PrepareMock(context.Background(), a, mockSender))
		service := process.New()
		err = service.Init(a)
		assert.Nil(t, err)

		notifications := mock_notifications.NewMockNotifications(t)

		e := &export{
			objectStore:         storeFixture,
			picker:              objectGetter,
			processService:      service,
			notificationService: notifications,
		}

		// when
		_, success, err := e.Export(context.Background(), pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			Path:          t.TempDir(),
			ObjectIds:     []string{objectID},
			Format:        model.Export_Protobuf,
			Zip:           true,
			IncludeNested: true,
			IncludeFiles:  true,
			IsJson:        true,
			NoProgress:    true,
		})

		// then
		assert.Nil(t, err)
		assert.Equal(t, 2, success)
		notifications.AssertNotCalled(t, "CreateAndSend")
	})
	t.Run("empty import", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectID := "id"

		objectGetter := mock_cache.NewMockObjectGetter(t)

		a := &app.App{}
		mockSender := mock_event.NewMockSender(t)
		mockSender.EXPECT().Broadcast(mock.Anything).Return()
		a.Register(testutil.PrepareMock(context.Background(), a, mockSender))
		service := process.New()
		err := service.Init(a)
		assert.Nil(t, err)

		notifications := mock_notifications.NewMockNotifications(t)
		notificationSend := make(chan struct{})
		notifications.EXPECT().CreateAndSend(mock.Anything).RunAndReturn(func(notification *model.Notification) error {
			close(notificationSend)
			return nil
		})

		e := &export{
			objectStore:         storeFixture,
			picker:              objectGetter,
			processService:      service,
			notificationService: notifications,
		}

		// when
		path, success, err := e.Export(context.Background(), pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			Path:          t.TempDir(),
			ObjectIds:     []string{objectID},
			Format:        model.Export_Protobuf,
			Zip:           true,
			IncludeNested: true,
			IncludeFiles:  true,
			IsJson:        true,
		})

		// then
		<-notificationSend
		assert.Nil(t, err)
		assert.Equal(t, 0, success)

		reader, err := zip.OpenReader(path)
		assert.Nil(t, err)
		assert.Len(t, reader.File, 0)
	})
	t.Run("import finished with error", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "customObjectType"

		objectID := "id"
		storeFixture.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:      domain.String(objectID),
				bundle.RelationKeyType:    domain.String(objectTypeId),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
		})
		objectGetter := mock_cache.NewMockObjectGetter(t)
		objectGetter.EXPECT().GetObject(context.Background(), objectID).Return(nil, fmt.Errorf("error"))

		a := &app.App{}
		mockSender := mock_event.NewMockSender(t)
		mockSender.EXPECT().Broadcast(mock.Anything).Return()
		a.Register(testutil.PrepareMock(context.Background(), a, mockSender))
		service := process.New()
		err := service.Init(a)
		assert.Nil(t, err)

		notifications := mock_notifications.NewMockNotifications(t)
		notificationSend := make(chan struct{})
		notifications.EXPECT().CreateAndSend(mock.Anything).RunAndReturn(func(notification *model.Notification) error {
			close(notificationSend)
			return nil
		})

		e := &export{
			objectStore:         storeFixture,
			picker:              objectGetter,
			processService:      service,
			notificationService: notifications,
		}

		// when
		_, success, err := e.Export(context.Background(), pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			Path:          t.TempDir(),
			ObjectIds:     []string{objectID},
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
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "objectTypeId"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)
		objectId := "objectID"
		link := "linkId"

		storeFixture.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:          domain.String(link),
				bundle.RelationKeyType:        domain.String(objectTypeId),
				bundle.RelationKeySpaceId:     domain.String(spaceId),
				bundle.RelationKeyDescription: domain.String("description"),
				bundle.RelationKeyLayout:      domain.Int64(model.ObjectType_set),
				bundle.RelationKeyCamera:      domain.String("test"),
			},
			{
				bundle.RelationKeyId:      domain.String(objectId),
				bundle.RelationKeyType:    domain.String(objectTypeId),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:                   domain.String(objectTypeId),
				bundle.RelationKeyUniqueKey:            domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:               domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{addr.MissingObject}),
				bundle.RelationKeySpaceId:              domain.String(spaceId),
				bundle.RelationKeyType:                 domain.String(objectTypeId),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetterComponent(t)

		smartBlockTest := smarttest.New(objectId)
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc
		smartBlockTest.AddBlock(simple.New(&model.Block{Id: objectId, ChildrenIds: []string{"linkBlock"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
		smartBlockTest.AddBlock(simple.New(&model.Block{Id: "linkBlock", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: link}}}))

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc
		objectType.SetType(smartblock.SmartBlockTypeObjectType)

		linkObject := smarttest.New(link)
		linkObjectDoc := linkObject.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:          domain.String(link),
			bundle.RelationKeyType:        domain.String(objectTypeId),
			bundle.RelationKeySpaceId:     domain.String(spaceId),
			bundle.RelationKeyDescription: domain.String("description"),
			bundle.RelationKeyLayout:      domain.Int64(model.ObjectType_set),
			bundle.RelationKeyCamera:      domain.String("test"),
		}))
		linkObjectDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeySpaceId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyDescription.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyLayout.String(),
			Format: model.RelationFormat_number,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyCamera.String(),
			Format: model.RelationFormat_longtext,
		})
		linkObject.Doc = linkObjectDoc
		linkObject.AddBlock(simple.New(&model.Block{Id: objectId, ChildrenIds: []string{"linkBlock"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
		linkObject.AddBlock(simple.New(&model.Block{Id: "linkBlock", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "link1"}}}))

		objectGetter.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil).Times(4)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)
		objectGetter.EXPECT().GetObject(context.Background(), link).Return(linkObject, nil)

		a := &app.App{}
		mockSender := mock_event.NewMockSender(t)
		mockSender.EXPECT().Broadcast(mock.Anything).Return()
		a.Register(testutil.PrepareMock(context.Background(), a, mockSender))
		service := process.New()
		err = service.Init(a)
		assert.Nil(t, err)

		notifications := mock_notifications.NewMockNotifications(t)
		notificationSend := make(chan struct{})
		notifications.EXPECT().CreateAndSend(mock.Anything).RunAndReturn(func(notification *model.Notification) error {
			close(notificationSend)
			return nil
		})

		provider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		provider.EXPECT().Type(spaceId, link).Return(smartblock.SmartBlockTypePage, nil)

		e := &export{
			objectStore:         storeFixture,
			picker:              objectGetter,
			processService:      service,
			notificationService: notifications,
			sbtProvider:         provider,
		}

		// when
		path, success, err := e.Export(context.Background(), pb.RpcObjectListExportRequest{
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
		assert.Nil(t, err)
		assert.Equal(t, 3, success)

		reader, err := zip.OpenReader(path)
		assert.Nil(t, err)

		assert.Len(t, reader.File, 3)
		fileNames := make(map[string]bool, 3)
		for _, file := range reader.File {
			fileNames[file.Name] = true
		}

		objectPath := filepath.Join(objectsDirectory, link+".pb.json")
		assert.True(t, fileNames[objectPath])

		file, err := os.Open(objectPath)
		if err != nil {
			return
		}
		var sn *pb.SnapshotWithType
		err = jsonpb.Unmarshal(file, sn)
		assert.Nil(t, err)
		assert.Len(t, sn.GetSnapshot().GetData().GetBlocks(), 1)
		assert.Equal(t, link, sn.GetSnapshot().GetData().GetBlocks()[0].GetId())
		assert.Len(t, sn.GetSnapshot().GetData().GetDetails().GetFields(), 1)
		assert.NotNil(t, sn.GetSnapshot().GetData().GetDetails().GetFields()[bundle.RelationKeyCamera.String()])
		assert.Len(t, sn.GetSnapshot().GetData().GetRelationLinks(), 1)
		assert.Equal(t, bundle.RelationKeyCamera.String(), sn.GetSnapshot().GetData().GetRelationLinks()[0].Key)
	})
	t.Run("export with backlinks", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "objectTypeId"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)
		objectId := "objectID"
		link1 := "linkId"

		storeFixture.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:      domain.String(link1),
				bundle.RelationKeyType:    domain.String(objectTypeId),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:        domain.String(objectId),
				bundle.RelationKeyType:      domain.String(objectTypeId),
				bundle.RelationKeySpaceId:   domain.String(spaceId),
				bundle.RelationKeyBacklinks: domain.StringList([]string{link1}),
			},
			{
				bundle.RelationKeyId:                   domain.String(objectTypeId),
				bundle.RelationKeyUniqueKey:            domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:               domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{addr.MissingObject}),
				bundle.RelationKeySpaceId:              domain.String(spaceId),
				bundle.RelationKeyType:                 domain.String(objectTypeId),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetterComponent(t)

		smartBlockTest := smarttest.New(objectId)
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:        domain.String(objectId),
			bundle.RelationKeyType:      domain.String(objectTypeId),
			bundle.RelationKeyBacklinks: domain.StringList([]string{link1}),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyBacklinks.String(),
			Format: model.RelationFormat_object,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc
		objectType.SetType(smartblock.SmartBlockTypeObjectType)

		linkObject := smarttest.New(link1)
		linkObjectDoc := linkObject.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:      domain.String(link1),
			bundle.RelationKeyType:    domain.String(objectTypeId),
			bundle.RelationKeySpaceId: domain.String(spaceId),
		}))
		linkObjectDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeySpaceId.String(),
			Format: model.RelationFormat_longtext,
		})
		linkObject.Doc = linkObjectDoc

		objectGetter.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil).Times(4)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)
		objectGetter.EXPECT().GetObject(context.Background(), link1).Return(linkObject, nil)

		a := &app.App{}
		mockSender := mock_event.NewMockSender(t)
		mockSender.EXPECT().Broadcast(mock.Anything).Return()
		a.Register(testutil.PrepareMock(context.Background(), a, mockSender))
		service := process.New()
		err = service.Init(a)
		assert.Nil(t, err)

		notifications := mock_notifications.NewMockNotifications(t)
		notificationSend := make(chan struct{})
		notifications.EXPECT().CreateAndSend(mock.Anything).RunAndReturn(func(notification *model.Notification) error {
			close(notificationSend)
			return nil
		})

		provider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		provider.EXPECT().Type(spaceId, link1).Return(smartblock.SmartBlockTypePage, nil)

		e := &export{
			objectStore:         storeFixture,
			picker:              objectGetter,
			processService:      service,
			notificationService: notifications,
			sbtProvider:         provider,
		}

		// when
		path, success, err := e.Export(context.Background(), pb.RpcObjectListExportRequest{
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
		assert.Nil(t, err)
		assert.Equal(t, 3, success)

		reader, err := zip.OpenReader(path)
		assert.Nil(t, err)

		assert.Len(t, reader.File, 3)
		fileNames := make(map[string]bool, 3)
		for _, file := range reader.File {
			fileNames[file.Name] = true
		}

		objectPath := filepath.Join(objectsDirectory, link1+".pb.json")
		assert.True(t, fileNames[objectPath])
	})
	t.Run("export without backlinks", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "objectTypeId"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)
		objectId := "objectID"
		link1 := "linkId"

		storeFixture.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:      domain.String(link1),
				bundle.RelationKeyType:    domain.String(objectTypeId),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:        domain.String(objectId),
				bundle.RelationKeyType:      domain.String(objectTypeId),
				bundle.RelationKeySpaceId:   domain.String(spaceId),
				bundle.RelationKeyBacklinks: domain.StringList([]string{link1}),
			},
			{
				bundle.RelationKeyId:                   domain.String(objectTypeId),
				bundle.RelationKeyUniqueKey:            domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:               domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{addr.MissingObject}),
				bundle.RelationKeySpaceId:              domain.String(spaceId),
				bundle.RelationKeyType:                 domain.String(objectTypeId),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetterComponent(t)

		smartBlockTest := smarttest.New(objectId)
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:        domain.String(objectId),
			bundle.RelationKeyType:      domain.String(objectTypeId),
			bundle.RelationKeyBacklinks: domain.StringList([]string{link1}),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyBacklinks.String(),
			Format: model.RelationFormat_object,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc
		objectType.SetType(smartblock.SmartBlockTypeObjectType)

		objectGetter.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil).Times(4)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		a := &app.App{}
		mockSender := mock_event.NewMockSender(t)
		mockSender.EXPECT().Broadcast(mock.Anything).Return()
		a.Register(testutil.PrepareMock(context.Background(), a, mockSender))
		service := process.New()
		err = service.Init(a)
		assert.Nil(t, err)

		notifications := mock_notifications.NewMockNotifications(t)
		notificationSend := make(chan struct{})
		notifications.EXPECT().CreateAndSend(mock.Anything).RunAndReturn(func(notification *model.Notification) error {
			close(notificationSend)
			return nil
		})

		provider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)

		e := &export{
			objectStore:         storeFixture,
			picker:              objectGetter,
			processService:      service,
			notificationService: notifications,
			sbtProvider:         provider,
		}

		// when
		path, success, err := e.Export(context.Background(), pb.RpcObjectListExportRequest{
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
		assert.Nil(t, err)
		assert.Equal(t, 2, success)

		reader, err := zip.OpenReader(path)
		assert.Nil(t, err)

		fileNames := make(map[string]bool, 2)
		for _, file := range reader.File {
			fileNames[file.Name] = true
		}

		objectPath := filepath.Join(objectsDirectory, link1+".pb.json")
		assert.False(t, fileNames[objectPath])
	})
	t.Run("export with space", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "customObjectType"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)

		objectID := "id"
		workspaceId := "workspaceId"
		storeFixture.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:      domain.String(objectID),
				bundle.RelationKeyType:    domain.String(objectTypeId),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:             domain.String(workspaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_space)),
			},
			{
				bundle.RelationKeyId:                   domain.String(objectTypeId),
				bundle.RelationKeyUniqueKey:            domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout:       domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{addr.MissingObject}),
				bundle.RelationKeySpaceId:              domain.String(spaceId),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New(objectID)
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectID),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		workspaceTest := smarttest.New(workspaceId)
		workspaceDoc := workspaceTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(workspaceId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		workspaceDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		workspaceTest.Doc = workspaceDoc

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc
		objectType.SetType(smartblock.SmartBlockTypeObjectType)
		objectGetter.EXPECT().GetObject(context.Background(), objectID).Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)
		objectGetter.EXPECT().GetObject(context.Background(), workspaceId).Return(workspaceTest, nil)

		a := &app.App{}
		mockSender := mock_event.NewMockSender(t)
		mockSender.EXPECT().Broadcast(mock.Anything).Return()
		a.Register(testutil.PrepareMock(context.Background(), a, mockSender))
		service := process.New()
		err = service.Init(a)
		assert.Nil(t, err)

		notifications := mock_notifications.NewMockNotifications(t)
		notificationSend := make(chan struct{})
		notifications.EXPECT().CreateAndSend(mock.Anything).RunAndReturn(func(notification *model.Notification) error {
			close(notificationSend)
			return nil
		})

		spaceService := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().DerivedIDs().Return(threads.DerivedSmartblockIds{Workspace: workspaceId})
		spaceService.EXPECT().Get(context.Background(), spaceId).Return(space, nil)

		e := &export{
			objectStore:         storeFixture,
			picker:              objectGetter,
			processService:      service,
			notificationService: notifications,
			spaceService:        spaceService,
		}

		// when
		path, success, err := e.Export(context.Background(), pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			Path:          t.TempDir(),
			ObjectIds:     []string{objectID},
			Format:        model.Export_Protobuf,
			Zip:           true,
			IncludeNested: true,
			IncludeFiles:  true,
			IsJson:        true,
			IncludeSpace:  true,
		})

		// then
		<-notificationSend
		assert.Nil(t, err)
		assert.Equal(t, 3, success)

		reader, err := zip.OpenReader(path)
		assert.Nil(t, err)

		assert.Len(t, reader.File, 3)
		fileNames := make(map[string]bool, 3)
		for _, file := range reader.File {
			fileNames[file.Name] = true
		}

		objectPath := filepath.Join(objectsDirectory, workspaceId+".pb.json")
		assert.True(t, fileNames[objectPath])
	})
}

func Test_docsForExport(t *testing.T) {
	t.Run("get object with existing links", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id"),
				bundle.RelationKeyName:    domain.String("name1"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeyName:    domain.String("name2"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
		})

		provider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		provider.EXPECT().Type(spaceId, "id1").Return(smartblock.SmartBlockTypePage, nil)

		objectGetter := mock_cache.NewMockObjectGetterComponent(t)
		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("id"),
			bundle.RelationKeyType: domain.String("objectTypeId"),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		smartBlockTest.AddBlock(simple.New(&model.Block{Id: "id", ChildrenIds: []string{"linkBlock"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
		smartBlockTest.AddBlock(simple.New(&model.Block{Id: "linkBlock", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "id1"}}}))

		linkObject := smarttest.New("id1")
		linkObjectDoc := linkObject.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("id1"),
			bundle.RelationKeyType: domain.String("objectTypeId"),
		}))
		linkObjectDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		linkObject.Doc = linkObjectDoc

		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), "id1").Return(linkObject, nil)

		e := &export{
			objectStore: storeFixture,
			sbtProvider: provider,
			picker:      objectGetter,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{"id"},
			IncludeNested: true,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})
	t.Run("get object with non existing links", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id"),
				bundle.RelationKeyName:    domain.String("name"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:        domain.String("id1"),
				bundle.RelationKeyIsDeleted: domain.Bool(true),
				bundle.RelationKeySpaceId:   domain.String(spaceId),
			},
		})
		objectGetter := mock_cache.NewMockObjectGetterComponent(t)
		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("id"),
			bundle.RelationKeyType: domain.String("objectTypeId"),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		smartBlockTest.AddBlock(simple.New(&model.Block{Id: "id", ChildrenIds: []string{"linkBlock"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
		smartBlockTest.AddBlock(simple.New(&model.Block{Id: "linkBlock", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "id1"}}}))

		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)

		provider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		provider.EXPECT().Type(spaceId, "id1").Return(smartblock.SmartBlockTypePage, nil)
		e := &export{
			objectStore: storeFixture,
			sbtProvider: provider,
			picker:      objectGetter,
		}
		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{"id"},
			IncludeNested: true,
		})

		// when
		err := expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 1, len(expCtx.docs))
	})
	t.Run("get object with non existing relation", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		relationKey := domain.RelationKey("key")
		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id"),
				relationKey:               domain.String("value"),
				bundle.RelationKeyType:    domain.String("objectType"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
		})
		err := storeFixture.SpaceIndex(spaceId).UpdateObjectLinks(context.Background(), "id", []string{"id1"})
		assert.Nil(t, err)

		objectGetter := mock_cache.NewMockObjectGetter(t)
		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("id"),
			relationKey:            domain.String("value"),
			bundle.RelationKeyType: domain.String("objectType"),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    relationKey.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New("objectType")
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("objectType"),
			bundle.RelationKeyType: domain.String("objectType"),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), "objectType").Return(objectType, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:   spaceId,
			ObjectIds: []string{"id"},
			Format:    model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 1, len(expCtx.docs))
	})
	t.Run("get object with existing relation", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		relationKey := domain.RelationKey("key")
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey.String())
		assert.Nil(t, err)

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id"),
				relationKey:               domain.String("value"),
				bundle.RelationKeyType:    domain.String("objectType"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:          domain.String(relationKey),
				bundle.RelationKeyRelationKey: domain.String(relationKey),
				bundle.RelationKeyUniqueKey:   domain.String(uniqueKey.Marshal()),
				bundle.RelationKeySpaceId:     domain.String(spaceId),
			},
		})

		err = storeFixture.SpaceIndex(spaceId).UpdateObjectLinks(context.Background(), "id", []string{"id1"})
		assert.Nil(t, err)

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("id"),
			relationKey:            domain.String("value"),
			bundle.RelationKeyType: domain.String("objectType"),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    relationKey.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New("objectType")
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("objectType"),
			bundle.RelationKeyType: domain.String("objectType"),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), "objectType").Return(objectType, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}
		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:   spaceId,
			ObjectIds: []string{"id"},
			Format:    model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})

	t.Run("get relation options - no relation options", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		relationKey := domain.RelationKey("key")
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey.String())
		assert.Nil(t, err)

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id"),
				relationKey:               domain.String("value"),
				bundle.RelationKeyType:    domain.String("objectType"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:             domain.String(relationKey),
				bundle.RelationKeyRelationKey:    domain.String(relationKey),
				bundle.RelationKeyUniqueKey:      domain.String(uniqueKey.Marshal()),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("id"),
			relationKey:            domain.String("value"),
			bundle.RelationKeyType: domain.String("objectType"),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    relationKey.String(),
			Format: model.RelationFormat_tag,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New("objectType")
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("objectType"),
			bundle.RelationKeyType: domain.String("objectType"),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), "objectType").Return(objectType, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})
	t.Run("get relation options - 1 relation option", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		relationKey := domain.RelationKey("key")
		optionId := "optionId"
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey.String())
		assert.Nil(t, err)
		optionUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelationOption, optionId)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id"),
				relationKey:               domain.String(optionId),
				bundle.RelationKeyType:    domain.String("objectType"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:             domain.String(relationKey),
				bundle.RelationKeyRelationKey:    domain.String(relationKey),
				bundle.RelationKeyUniqueKey:      domain.String(uniqueKey.Marshal()),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:             domain.String(optionId),
				bundle.RelationKeyRelationKey:    domain.String(relationKey),
				bundle.RelationKeyUniqueKey:      domain.String(optionUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("id"),
			relationKey:            domain.String("value"),
			bundle.RelationKeyType: domain.String("objectType"),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    relationKey.String(),
			Format: model.RelationFormat_tag,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New("objectType")
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("objectType"),
			bundle.RelationKeyType: domain.String("objectType"),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), "objectType").Return(objectType, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 3, len(expCtx.docs))
		var objectIds []string
		for objectId := range expCtx.docs {
			objectIds = append(objectIds, objectId)
		}
		assert.Contains(t, objectIds, optionId)
	})
	t.Run("get derived objects - relation, object type with recommended relations, template with link", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		relationKey := domain.RelationKey("key")
		objectTypeKey := "customObjectType"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeKey)
		assert.Nil(t, err)
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey.String())
		assert.Nil(t, err)

		recommendedRelationKey := "recommendedRelationKey"
		recommendedRelationUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, recommendedRelationKey)
		assert.Nil(t, err)

		templateId := "templateId"
		templateObjectTypeId := "templateObjectTypeId"

		linkedObjectId := "linkedObjectId"
		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id"),
				relationKey:               domain.String("test"),
				bundle.RelationKeyType:    domain.String(objectTypeKey),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:             domain.String(relationKey),
				bundle.RelationKeyRelationKey:    domain.String(relationKey),
				bundle.RelationKeyUniqueKey:      domain.String(uniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:                   domain.String(objectTypeKey),
				bundle.RelationKeyUniqueKey:            domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout:       domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{recommendedRelationKey}),
				bundle.RelationKeySpaceId:              domain.String(spaceId),
				bundle.RelationKeyType:                 domain.String(objectTypeKey),
			},
			{
				bundle.RelationKeyId:             domain.String(recommendedRelationKey),
				bundle.RelationKeyRelationKey:    domain.String(recommendedRelationKey),
				bundle.RelationKeyUniqueKey:      domain.String(recommendedRelationUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:               domain.String(templateId),
				bundle.RelationKeyTargetObjectType: domain.String(objectTypeKey),
				bundle.RelationKeySpaceId:          domain.String(spaceId),
				bundle.RelationKeyType:             domain.String(templateObjectTypeId),
			},
			{
				bundle.RelationKeyId:      domain.String(linkedObjectId),
				bundle.RelationKeyType:    domain.String(objectTypeKey),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetter(t)

		template := smarttest.New(templateId)
		templateDoc := template.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(templateId),
			bundle.RelationKeyType: domain.String(templateObjectTypeId),
		}))
		templateDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		template.Doc = templateDoc
		template.AddBlock(simple.New(&model.Block{Id: templateId, ChildrenIds: []string{"linkBlock"}, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}))
		template.AddBlock(simple.New(&model.Block{Id: "linkBlock", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: linkedObjectId}}}))

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("id"),
			relationKey:            domain.String("value"),
			bundle.RelationKeyType: domain.String(objectTypeKey),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    relationKey.String(),
			Format: model.RelationFormat_tag,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeKey)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeKey),
			bundle.RelationKeyType: domain.String(objectTypeKey),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		templateObjectType := smarttest.New(objectTypeKey)
		templateObjectTypeDoc := templateObjectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(templateId),
			bundle.RelationKeyType: domain.String(templateObjectTypeId),
		}))
		templateObjectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		templateObjectType.Doc = templateObjectTypeDoc

		linkedObject := smarttest.New(objectTypeKey)
		linkedObjectDoc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(linkedObjectId),
			bundle.RelationKeyType: domain.String(objectTypeKey),
		}))
		linkedObjectDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		linkedObject.Doc = linkedObjectDoc

		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), templateId).Return(template, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeKey).Return(objectType, nil)
		objectGetter.EXPECT().GetObject(context.Background(), templateObjectTypeId).Return(templateObjectType, nil)
		objectGetter.EXPECT().GetObject(context.Background(), linkedObjectId).Return(linkedObject, nil)

		provider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		provider.EXPECT().Type(spaceId, linkedObjectId).Return(smartblock.SmartBlockTypePage, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
			sbtProvider: provider,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 6, len(expCtx.docs))
	})
	t.Run("get derived objects, object type have missing relations - return only object and its type", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "customObjectType"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id"),
				bundle.RelationKeyType:    domain.String(objectTypeId),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:                   domain.String(objectTypeId),
				bundle.RelationKeyUniqueKey:            domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout:       domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{addr.MissingObject}),
				bundle.RelationKeySpaceId:              domain.String(spaceId),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("id"),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})
	t.Run("objects without links", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "objectTypeId"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id"),
				bundle.RelationKeyName:    domain.String("name1"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
				bundle.RelationKeyType:    domain.String(objectTypeId),
			},
			{
				bundle.RelationKeyId:      domain.String("id1"),
				bundle.RelationKeyName:    domain.String("name2"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
				bundle.RelationKeyType:    domain.String(objectTypeId),
			},
			{
				bundle.RelationKeyId:             domain.String(objectTypeId),
				bundle.RelationKeyUniqueKey:      domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
			},
		})

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("id"),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		objectGetter := mock_cache.NewMockObjectGetter(t)
		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})
	t.Run("objects with dataview", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "objectTypeId"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)

		relationKey := "key"
		relationKeyUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("id"),
				bundle.RelationKeyName:           domain.String("name1"),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_set)),
			},
			{
				bundle.RelationKeyId:             domain.String(objectTypeId),
				bundle.RelationKeyUniqueKey:      domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
			},
			{
				bundle.RelationKeyId:             domain.String(bundle.RelationKeyTag.String()),
				bundle.RelationKeyName:           domain.String(bundle.RelationKeyTag.String()),
				bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyTag.String()),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyUniqueKey:      domain.String(bundle.RelationKeyTag.URL()),
			},
			{
				bundle.RelationKeyId:             domain.String(relationKey),
				bundle.RelationKeyName:           domain.String(relationKey),
				bundle.RelationKeyRelationKey:    domain.String(relationKey),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyUniqueKey:      domain.String(relationKeyUniqueKey.Marshal()),
			},
		})

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:             domain.String("id"),
			bundle.RelationKeyType:           domain.String(objectTypeId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_set)),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyResolvedLayout.String(),
			Format: model.RelationFormat_number,
		})
		doc.Set(simple.New(&model.Block{
			Id:          "id",
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
								Key: relationKey,
							},
						},
					},
				},
			}},
		}))
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		objectGetter := mock_cache.NewMockObjectGetter(t)
		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 4, len(expCtx.docs))
	})
	t.Run("objects without file", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "objectTypeId"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("id"),
				bundle.RelationKeyName:           domain.String("name1"),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_set)),
			},
			{
				bundle.RelationKeyId:             domain.String(objectTypeId),
				bundle.RelationKeyUniqueKey:      domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
			},
		})

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:             domain.String("id"),
			bundle.RelationKeyType:           domain.String(objectTypeId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_set)),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyResolvedLayout.String(),
			Format: model.RelationFormat_number,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		objectGetter := mock_cache.NewMockObjectGetter(t)
		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			IncludeFiles:  true,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})
	t.Run("objects without file, not protobuf export", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "objectTypeId"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("id"),
				bundle.RelationKeyName:           domain.String("name1"),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_set)),
			},
			{
				bundle.RelationKeyId:             domain.String(objectTypeId),
				bundle.RelationKeyUniqueKey:      domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
			},
		})

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String("id"),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectGetter := mock_cache.NewMockObjectGetter(t)
		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			IncludeFiles:  true,
			Format:        model.Export_Markdown,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 1, len(expCtx.docs))
	})

	t.Run("get derived objects - relation, object type with recommended relations, template with link", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeKey := "customObjectType"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeKey)
		assert.Nil(t, err)

		relationKey := "key"
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey)
		assert.Nil(t, err)

		recommendedRelationKey := "recommendedRelationKey"
		recommendedRelationUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, recommendedRelationKey)
		assert.Nil(t, err)

		relationObjectTypeKey := "relation"
		relationObjectTypeUK, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, relationObjectTypeKey)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id"),
				bundle.RelationKeySetOf:   domain.StringList([]string{relationKey}),
				bundle.RelationKeyType:    domain.String(objectTypeKey),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:             domain.String(relationKey),
				bundle.RelationKeyRelationKey:    domain.String(relationKey),
				bundle.RelationKeyUniqueKey:      domain.String(uniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(relationObjectTypeKey),
			},
			{
				bundle.RelationKeyId:                   domain.String(objectTypeKey),
				bundle.RelationKeyUniqueKey:            domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout:       domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{recommendedRelationKey}),
				bundle.RelationKeySpaceId:              domain.String(spaceId),
				bundle.RelationKeyType:                 domain.String(objectTypeKey),
			},
			{
				bundle.RelationKeyId:             domain.String(relationObjectTypeKey),
				bundle.RelationKeyUniqueKey:      domain.String(relationObjectTypeUK.Marshal()),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeKey),
			},
			{
				bundle.RelationKeyId:             domain.String(recommendedRelationKey),
				bundle.RelationKeyRelationKey:    domain.String(recommendedRelationKey),
				bundle.RelationKeyUniqueKey:      domain.String(recommendedRelationUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:    domain.String("id"),
			bundle.RelationKeySetOf: domain.StringList([]string{relationKey}),
			bundle.RelationKeyType:  domain.String(objectTypeKey),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeKey)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeKey),
			bundle.RelationKeyType: domain.String(objectTypeKey),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		relationObject := smarttest.New(relationKey)
		relationObjectDoc := relationObject.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(relationKey),
			bundle.RelationKeyType: domain.String(relationObjectTypeKey),
		}))
		relationObjectDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		relationObject.Doc = relationObjectDoc

		relationObjectType := smarttest.New(relationObjectTypeKey)
		relationObjectTypeDoc := relationObjectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(relationObjectTypeKey),
			bundle.RelationKeyType: domain.String(objectTypeKey),
		}))
		relationObjectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		relationObjectType.Doc = relationObjectTypeDoc

		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeKey).Return(objectType, nil)
		objectGetter.EXPECT().GetObject(context.Background(), relationKey).Return(relationObject, nil)
		objectGetter.EXPECT().GetObject(context.Background(), relationObjectTypeKey).Return(relationObjectType, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:   spaceId,
			ObjectIds: []string{"id"},
			Format:    model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)
		// then
		assert.Nil(t, err)
		assert.Equal(t, 5, len(expCtx.docs))
	})
	t.Run("export template", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeKey := "customObjectType"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeKey)
		assert.Nil(t, err)

		templateType := "templateType"
		templateObjectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, templateType)
		assert.Nil(t, err)

		objectId := "objectId"
		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               domain.String(objectId),
				bundle.RelationKeyName:             domain.String("template"),
				bundle.RelationKeySpaceId:          domain.String(spaceId),
				bundle.RelationKeyTargetObjectType: domain.String(objectTypeKey),
				bundle.RelationKeyType:             domain.String(templateType),
			},
			{
				bundle.RelationKeyId:        domain.String(objectTypeKey),
				bundle.RelationKeyUniqueKey: domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:    domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:   domain.String(spaceId),
				bundle.RelationKeyType:      domain.String(objectTypeKey),
			},
			{
				bundle.RelationKeyId:        domain.String(templateType),
				bundle.RelationKeyUniqueKey: domain.String(templateObjectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:    domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:   domain.String(spaceId),
				bundle.RelationKeyType:      domain.String(objectTypeKey),
			},
		})

		smartBlockTest := smarttest.New(objectId)
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:               domain.String(objectId),
			bundle.RelationKeyType:             domain.String(templateType),
			bundle.RelationKeyTargetObjectType: domain.String(objectTypeKey),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeKey)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeKey),
			bundle.RelationKeyType: domain.String(objectTypeKey),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		templateTypeObject := smarttest.New(templateType)
		templateTypeObjectDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(templateType),
			bundle.RelationKeyType: domain.String(templateType),
		}))
		templateTypeObjectDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		templateTypeObject.Doc = templateTypeObjectDoc

		objectGetter := mock_cache.NewMockObjectGetter(t)
		objectGetter.EXPECT().GetObject(context.Background(), objectId).Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeKey).Return(objectType, nil)
		objectGetter.EXPECT().GetObject(context.Background(), templateType).Return(templateTypeObject, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{objectId},
			IncludeNested: false,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 3, len(expCtx.docs))
	})
	t.Run("add default object type and template from dataview", func(t *testing.T) {
		// given
		id := "id"

		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "objectTypeId"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)

		defaultObjectTypeId := "defaultObjectTypeId"
		defaultObjectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, defaultObjectTypeId)
		assert.Nil(t, err)

		defaultTemplateId := "defaultTemplateId"
		defaultObjectTypeTemplateId := "defaultObjectTypeTemplateId"

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String(id),
				bundle.RelationKeyName:           domain.String("name"),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
			},
			{
				bundle.RelationKeyId:             domain.String(objectTypeId),
				bundle.RelationKeyUniqueKey:      domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(bundle.TypeKeyObjectType),
			},
			{
				bundle.RelationKeyId:             domain.String(defaultObjectTypeId),
				bundle.RelationKeyUniqueKey:      domain.String(defaultObjectTypeUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(bundle.TypeKeyObjectType),
			},
			{
				bundle.RelationKeyId:      domain.String(defaultTemplateId),
				bundle.RelationKeySpaceId: domain.String(spaceId),
				bundle.RelationKeyType:    domain.String(bundle.TypeKeyTemplate),
			},
			{
				bundle.RelationKeyId:               domain.String(defaultObjectTypeTemplateId),
				bundle.RelationKeySpaceId:          domain.String(spaceId),
				bundle.RelationKeyType:             domain.String(bundle.TypeKeyTemplate),
				bundle.RelationKeyTargetObjectType: domain.String(defaultObjectTypeId),
			},
		})

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(id),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		})

		doc.Set(simple.New(&model.Block{
			Id:          "id",
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

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		defaultObjectType := smarttest.New(defaultObjectTypeId)
		defaultObjectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(defaultObjectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		defaultObjectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		defaultObjectType.Doc = defaultObjectTypeDoc

		defaultTemplate := smarttest.New(defaultObjectTypeId)
		defaultTemplateDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(defaultTemplateId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		defaultTemplateDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		defaultTemplate.Doc = defaultTemplateDoc

		defaultObjectTypeTemplate := smarttest.New(defaultObjectTypeTemplateId)
		defaultObjectTypeTemplateDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(defaultObjectTypeTemplateId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		defaultObjectTypeTemplateDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		defaultObjectTypeTemplate.Doc = defaultObjectTypeTemplateDoc

		objectGetter := mock_cache.NewMockObjectGetter(t)

		objectGetter.EXPECT().GetObject(context.Background(), id).Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)
		objectGetter.EXPECT().GetObject(context.Background(), defaultObjectTypeId).Return(defaultObjectType, nil)
		objectGetter.EXPECT().GetObject(context.Background(), defaultTemplateId).Return(defaultTemplate, nil)
		objectGetter.EXPECT().GetObject(context.Background(), defaultObjectTypeTemplateId).Return(defaultObjectTypeTemplate, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:   spaceId,
			ObjectIds: []string{"id"},
			Format:    model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 5, len(expCtx.docs))
	})
	t.Run("add default object type and template from dataview of set", func(t *testing.T) {
		// given
		id := "id"

		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "objectTypeId"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)

		defaultObjectTypeId := "defaultObjectTypeId"
		defaultObjectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)

		defaultTemplateId := "defaultTemplateId"
		defaultObjectTypeTemplateId := "defaultObjectTypeTemplateId"

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String(id),
				bundle.RelationKeyName:           domain.String("name"),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_set)),
			},
			{
				bundle.RelationKeyId:             domain.String(objectTypeId),
				bundle.RelationKeyUniqueKey:      domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
			},
			{
				bundle.RelationKeyId:             domain.String(defaultObjectTypeId),
				bundle.RelationKeyUniqueKey:      domain.String(defaultObjectTypeUniqueKey.Marshal()),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
				bundle.RelationKeyType:           domain.String(objectTypeId),
			},
			{
				bundle.RelationKeyId:      domain.String(defaultTemplateId),
				bundle.RelationKeySpaceId: domain.String(spaceId),
				bundle.RelationKeyType:    domain.String(objectTypeId),
			},
			{
				bundle.RelationKeyId:               domain.String(defaultObjectTypeTemplateId),
				bundle.RelationKeySpaceId:          domain.String(spaceId),
				bundle.RelationKeyType:             domain.String(objectTypeId),
				bundle.RelationKeyTargetObjectType: domain.String(defaultObjectTypeId),
			},
		})

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(id),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		})

		doc.Set(simple.New(&model.Block{
			Id:          "id",
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

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		defaultObjectType := smarttest.New(defaultObjectTypeId)
		defaultObjectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(defaultObjectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		defaultObjectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		defaultObjectType.Doc = defaultObjectTypeDoc

		defaultTemplate := smarttest.New(defaultObjectTypeId)
		defaultTemplateDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(defaultTemplateId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		defaultTemplateDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		defaultTemplate.Doc = defaultTemplateDoc

		defaultObjectTypeTemplate := smarttest.New(defaultObjectTypeTemplateId)
		defaultObjectTypeTemplateDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(defaultObjectTypeTemplateId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		defaultObjectTypeTemplateDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		defaultObjectTypeTemplate.Doc = defaultObjectTypeTemplateDoc

		objectGetter := mock_cache.NewMockObjectGetter(t)

		objectGetter.EXPECT().GetObject(context.Background(), id).Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)
		objectGetter.EXPECT().GetObject(context.Background(), defaultObjectTypeId).Return(defaultObjectType, nil)
		objectGetter.EXPECT().GetObject(context.Background(), defaultTemplateId).Return(defaultTemplate, nil)
		objectGetter.EXPECT().GetObject(context.Background(), defaultObjectTypeTemplateId).Return(defaultObjectTypeTemplate, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:   spaceId,
			ObjectIds: []string{"id"},
			Format:    model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 5, len(expCtx.docs))
	})
	t.Run("no default object type and template from dataview", func(t *testing.T) {
		// given
		id := "id"

		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "objectTypeId"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String(id),
				bundle.RelationKeyName:    domain.String("name"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
				bundle.RelationKeyType:    domain.String(objectTypeId),
				bundle.RelationKeyLayout:  domain.Int64(int64(model.ObjectType_collection)),
			},
			{
				bundle.RelationKeyId:        domain.String(objectTypeId),
				bundle.RelationKeyUniqueKey: domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:    domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:   domain.String(spaceId),
				bundle.RelationKeyType:      domain.String(objectTypeId),
			},
		})

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(id),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		})

		doc.Set(simple.New(&model.Block{
			Id:          "id",
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

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		objectGetter := mock_cache.NewMockObjectGetter(t)
		objectGetter.EXPECT().GetObject(context.Background(), id).Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:   spaceId,
			ObjectIds: []string{"id"},
			Format:    model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})

	t.Run("export participant", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "objectTypeId"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)

		participantId := domain.NewParticipantId(spaceId, "identity")
		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      domain.String("id"),
				bundle.RelationKeyName:    domain.String("name1"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
				bundle.RelationKeyType:    domain.String(objectTypeId),
			},
			{
				bundle.RelationKeyId:      domain.String(participantId),
				bundle.RelationKeyName:    domain.String("test"),
				bundle.RelationKeySpaceId: domain.String(spaceId),
				bundle.RelationKeyType:    domain.String(objectTypeId),
			},
			{
				bundle.RelationKeyId:        domain.String(objectTypeId),
				bundle.RelationKeyUniqueKey: domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:    domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:   domain.String(spaceId),
				bundle.RelationKeyType:      domain.String(objectTypeId),
			},
		})

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:             domain.String("id"),
			bundle.RelationKeyType:           domain.String(objectTypeId),
			bundle.RelationKeyLastModifiedBy: domain.String(participantId),
			bundle.RelationKeyCreator:        domain.String(participantId),
		}))
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyLastModifiedBy.String(),
			Format: model.RelationFormat_object,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyCreator.String(),
			Format: model.RelationFormat_object,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(objectTypeId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		participant := smarttest.New(participantId)
		participantDoc := participant.NewState().SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(participantId),
			bundle.RelationKeyType: domain.String(objectTypeId),
		}))
		participantDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		participant.Doc = participantDoc

		objectGetter := mock_cache.NewMockObjectGetter(t)
		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeId).Return(objectType, nil)
		objectGetter.EXPECT().GetObject(context.Background(), participantId).Return(participant, nil)

		provider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		provider.EXPECT().Type(spaceId, participantId).Return(smartblock.SmartBlockTypeParticipant, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
			sbtProvider: provider,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport(nil)

		// then
		assert.Nil(t, err)
		assert.Equal(t, 3, len(expCtx.docs))
		assert.NotNil(t, expCtx.docs[participantId])
	})
}

func Test_provideFileName(t *testing.T) {
	t.Run("file dir for relation", func(t *testing.T) {
		// when
		fileName := makeFileName("docId", spaceId, pbjson.NewConverter(nil).Ext(), nil, smartblock.SmartBlockTypeRelation)

		// then
		assert.Equal(t, relationsDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("file dir for relation option", func(t *testing.T) {
		// when
		fileName := makeFileName("docId", spaceId, pbjson.NewConverter(nil).Ext(), nil, smartblock.SmartBlockTypeRelationOption)

		// then
		assert.Equal(t, relationsOptionsDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("file dir for types", func(t *testing.T) {
		// when
		fileName := makeFileName("docId", spaceId, pbjson.NewConverter(nil).Ext(), nil, smartblock.SmartBlockTypeObjectType)

		// then
		assert.Equal(t, typesDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("file dir for objects", func(t *testing.T) {
		// when
		fileName := makeFileName("docId", spaceId, pbjson.NewConverter(nil).Ext(), nil, smartblock.SmartBlockTypePage)

		// then
		assert.Equal(t, objectsDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
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
		assert.Nil(t, err)
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
		assert.Nil(t, err)
		assert.Len(t, records, 2000)
	})
}

func TestExport_CollectionFilterMissing(t *testing.T) {
	t.Run("collection with non-existing objects", func(t *testing.T) {
		storeFixture := objectstore.NewStoreFixture(t)

		collectionId := "collection1"
		existingObjectId := "object1"
		missingObjectId := "object2"
		deletedObjectId := "object3"

		storeFixture.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:             domain.String(collectionId),
				bundle.RelationKeyType:           domain.String(bundle.TypeKeyCollection),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:      domain.String(existingObjectId),
				bundle.RelationKeyType:    domain.String(bundle.TypeKeyPage),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:        domain.String(deletedObjectId),
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
		existingObjectDetails, _ := storeFixture.GetDetails(spaceId, existingObjectId)
		expCtx.docs = map[string]*Doc{
			collectionId:     {Details: collectionDetails},
			existingObjectId: {Details: existingObjectDetails},
		}

		collectionState := state.NewDoc(collectionId, map[string]simple.Block{
			collectionId: simple.New(&model.Block{Id: collectionId}),
		}).(*state.State)
		collectionState.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
		}))
		collectionState.UpdateStoreSlice(template.CollectionStoreKey, []string{existingObjectId, missingObjectId, deletedObjectId})

		originalIds := collectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, originalIds, 3)

		expCtx.collectionFilterMissing(collectionState)

		processedIds := collectionState.GetStoreSlice(template.CollectionStoreKey)
		assert.Len(t, processedIds, 1)
		assert.Equal(t, []string{existingObjectId}, processedIds)
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
	t.Run("export collection with missing objects filters them out", func(t *testing.T) {
		storeFixture := objectstore.NewStoreFixture(t)
		collectionId := "collection1"
		existingObject1 := "object1"
		existingObject2 := "object2"
		missingObject := "missingObject"

		storeFixture.AddObjects(t, spaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:             domain.String(collectionId),
				bundle.RelationKeyType:           domain.String(bundle.TypeKeyCollection),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
				bundle.RelationKeySpaceId:        domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:      domain.String(existingObject1),
				bundle.RelationKeyType:    domain.String(bundle.TypeKeyPage),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
			{
				bundle.RelationKeyId:      domain.String(existingObject2),
				bundle.RelationKeyType:    domain.String(bundle.TypeKeyPage),
				bundle.RelationKeySpaceId: domain.String(spaceId),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetter(t)

		collectionBlock := smarttest.New(collectionId)
		collectionDoc := collectionBlock.NewState()
		collectionDoc.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:             domain.String(collectionId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_collection)),
			bundle.RelationKeyType:           domain.String(bundle.TypeKeyCollection),
		}))
		collectionDoc.UpdateStoreSlice(template.CollectionStoreKey, []string{existingObject1, missingObject, existingObject2})
		collectionBlock.Doc = collectionDoc

		object1Block := smarttest.New(existingObject1)
		object1Doc := object1Block.NewState()
		object1Doc.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(existingObject1),
			bundle.RelationKeyType: domain.String(bundle.TypeKeyPage),
		}))
		object1Block.Doc = object1Doc

		object2Block := smarttest.New(existingObject2)
		object2Doc := object2Block.NewState()
		object2Doc.SetDetails(domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:   domain.String(existingObject2),
			bundle.RelationKeyType: domain.String(bundle.TypeKeyPage),
		}))
		object2Block.Doc = object2Doc

		objectGetter.EXPECT().GetObject(mock.Anything, mock.Anything).Return(collectionBlock, nil).Maybe()
		objectGetter.EXPECT().GetObject(mock.Anything, mock.Anything).Return(object1Block, nil).Maybe()
		objectGetter.EXPECT().GetObject(mock.Anything, mock.Anything).Return(object2Block, nil).Maybe()
		objectGetter.EXPECT().GetObject(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("not found")).Maybe()

		sbtProvider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		sbtProvider.EXPECT().Type(mock.Anything, mock.Anything).Return(smartblock.SmartBlockTypePage, nil).Maybe()
		sbtProvider.EXPECT().Type(mock.Anything, mock.Anything).Return(smartblock.SmartBlockTypePage, nil).Maybe()
		sbtProvider.EXPECT().Type(mock.Anything, mock.Anything).Return(smartblock.SmartBlockTypePage, nil).Maybe()

		syncService := mock_notifications.NewMockNotifications(t)
		notificationSend := make(chan struct{})
		syncService.EXPECT().CreateAndSend(mock.Anything).RunAndReturn(func(notification *model.Notification) error {
			close(notificationSend)
			return nil
		})

		a := &app.App{}
		mockSender := mock_event.NewMockSender(t)
		mockSender.EXPECT().Broadcast(mock.Anything).Return()
		a.Register(testutil.PrepareMock(context.Background(), a, mockSender))

		service := process.New()
		err := service.Init(a)
		assert.Nil(t, err)

		e := &export{
			objectStore:         storeFixture,
			picker:              objectGetter,
			sbtProvider:         sbtProvider,
			notificationService: syncService,
			processService:      service,
		}

		path, succeed, err := e.Export(context.Background(), pb.RpcObjectListExportRequest{
			SpaceId:       spaceId,
			Path:          t.TempDir(),
			Format:        model.Export_Protobuf,
			ObjectIds:     []string{collectionId},
			Zip:           true,
			IncludeNested: true,
			IncludeFiles:  false,
			IsJson:        true,
		})

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
