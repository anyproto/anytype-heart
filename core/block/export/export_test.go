package export

import (
	"archive/zip"
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
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
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
func TestExport_Export(t *testing.T) {
	t.Run("export success", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "customObjectType"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)

		spaceId := "spaceId"
		objectID := "id"
		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      pbtypes.String(objectID),
				bundle.RelationKeyType:    pbtypes.String(objectTypeId),
				bundle.RelationKeySpaceId: pbtypes.String(spaceId),
			},
			{
				bundle.RelationKeyId:                   pbtypes.String(objectTypeId),
				bundle.RelationKeyUniqueKey:            pbtypes.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:               pbtypes.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: pbtypes.StringList([]string{addr.MissingObject}),
				bundle.RelationKeySpaceId:              pbtypes.String(spaceId),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New(objectID)
		doc := smartBlockTest.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String(objectID),
				bundle.RelationKeyType.String(): pbtypes.String(objectTypeId),
			}})
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String(objectTypeId),
				bundle.RelationKeyType.String(): pbtypes.String(objectTypeId),
			}})
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

		spaceService := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().Do(objectID, mock.Anything).Return(nil)
		spaceService.EXPECT().Get(context.Background(), "spaceId").Return(space, nil)

		notifications := mock_notifications.NewMockNotifications(t)
		notifications.EXPECT().CreateAndSend(mock.Anything).Return(nil)

		e := &export{
			objectStore:         storeFixture,
			picker:              objectGetter,
			processService:      service,
			spaceService:        spaceService,
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
		assert.Nil(t, err)
		assert.Equal(t, 2, success)

		reader, err := zip.OpenReader(path)
		assert.Nil(t, err)

		assert.Len(t, reader.File, 2)
		fileNames := make(map[string]bool, 2)
		for _, file := range reader.File {
			fileNames[file.Name] = true
		}
		assert.True(t, fileNames[objectsDirectory+string(filepath.Separator)+objectID+".pb.json"])
		assert.True(t, fileNames[typesDirectory+string(filepath.Separator)+objectTypeId+".pb.json"])
	})
	t.Run("empty import", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		spaceId := "spaceId"
		objectID := "id"

		objectGetter := mock_cache.NewMockObjectGetter(t)

		a := &app.App{}
		mockSender := mock_event.NewMockSender(t)
		mockSender.EXPECT().Broadcast(mock.Anything).Return()
		a.Register(testutil.PrepareMock(context.Background(), a, mockSender))
		service := process.New()
		err := service.Init(a)
		assert.Nil(t, err)

		spaceService := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		spaceService.EXPECT().Get(context.Background(), "spaceId").Return(space, nil)

		notifications := mock_notifications.NewMockNotifications(t)
		notifications.EXPECT().CreateAndSend(mock.Anything).Return(nil)

		e := &export{
			objectStore:         storeFixture,
			picker:              objectGetter,
			processService:      service,
			spaceService:        spaceService,
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
		assert.Nil(t, err)
		assert.Equal(t, 0, success)

		reader, err := zip.OpenReader(path)
		assert.Nil(t, err)
		assert.Len(t, reader.File, 0)
	})
	t.Run("import finished with error", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		spaceId := "spaceId"
		objectID := "id"

		objectGetter := mock_cache.NewMockObjectGetter(t)

		a := &app.App{}
		mockSender := mock_event.NewMockSender(t)
		mockSender.EXPECT().Broadcast(mock.Anything).Return()
		a.Register(testutil.PrepareMock(context.Background(), a, mockSender))
		service := process.New()
		err := service.Init(a)
		assert.Nil(t, err)

		spaceService := mock_space.NewMockService(t)
		spaceService.EXPECT().Get(context.Background(), "spaceId").Return(nil, fmt.Errorf("error"))

		notifications := mock_notifications.NewMockNotifications(t)
		notifications.EXPECT().CreateAndSend(mock.Anything).Return(nil)

		e := &export{
			objectStore:         storeFixture,
			picker:              objectGetter,
			processService:      service,
			spaceService:        spaceService,
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
		assert.NotNil(t, err)
		assert.Equal(t, 0, success)
	})
}
func Test_docsForExport(t *testing.T) {
	t.Run("get object with existing links", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      pbtypes.String("id"),
				bundle.RelationKeyName:    pbtypes.String("name1"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("id1"),
				bundle.RelationKeyName:    pbtypes.String("name2"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
			},
		})
		err := storeFixture.UpdateObjectLinks("id", []string{"id1"})
		assert.Nil(t, err)

		provider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		provider.EXPECT().Type("spaceId", "id1").Return(smartblock.SmartBlockTypePage, nil)
		e := &export{
			objectStore: storeFixture,
			sbtProvider: provider,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       "spaceId",
			ObjectIds:     []string{"id"},
			IncludeNested: true,
		})

		// when
		err = expCtx.docsForExport()

		// then
		assert.Nil(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})
	t.Run("get object with non existing links", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      pbtypes.String("id"),
				bundle.RelationKeyName:    pbtypes.String("name"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:        pbtypes.String("id1"),
				bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
				bundle.RelationKeySpaceId:   pbtypes.String("spaceId"),
			},
		})
		err := storeFixture.UpdateObjectLinks("id", []string{"id1"})
		assert.Nil(t, err)

		provider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		provider.EXPECT().Type("spaceId", "id1").Return(smartblock.SmartBlockTypePage, nil)
		e := &export{
			objectStore: storeFixture,
			sbtProvider: provider,
		}
		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       "spaceId",
			ObjectIds:     []string{"id"},
			IncludeNested: true,
		})

		// when
		err = expCtx.docsForExport()

		// then
		assert.Nil(t, err)
		assert.Equal(t, 1, len(expCtx.docs))
	})
	t.Run("get object with non existing relation", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		relationKey := "key"
		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            pbtypes.String("id"),
				domain.RelationKey(relationKey): pbtypes.String("value"),
				bundle.RelationKeyType:          pbtypes.String("objectType"),
				bundle.RelationKeySpaceId:       pbtypes.String("spaceId"),
			},
		})
		err := storeFixture.UpdateObjectLinks("id", []string{"id1"})
		assert.Nil(t, err)

		objectGetter := mock_cache.NewMockObjectGetter(t)
		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String("id"),
				relationKey:                     pbtypes.String("value"),
				bundle.RelationKeyType.String(): pbtypes.String("objectType"),
			}})
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    relationKey,
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New("objectType")
		objectTypeDoc := objectType.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String("objectType"),
				bundle.RelationKeyType.String(): pbtypes.String("objectType"),
			}})
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
			SpaceId:   "spaceId",
			ObjectIds: []string{"id"},
			Format:    model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport()

		// then
		assert.Nil(t, err)
		assert.Equal(t, 1, len(expCtx.docs))
	})
	t.Run("get object with existing relation", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		relationKey := "key"
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            pbtypes.String("id"),
				domain.RelationKey(relationKey): pbtypes.String("value"),
				bundle.RelationKeyType:          pbtypes.String("objectType"),
				bundle.RelationKeySpaceId:       pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:          pbtypes.String(relationKey),
				bundle.RelationKeyRelationKey: pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:   pbtypes.String(uniqueKey.Marshal()),
				bundle.RelationKeySpaceId:     pbtypes.String("spaceId"),
			},
		})

		err = storeFixture.UpdateObjectLinks("id", []string{"id1"})
		assert.Nil(t, err)

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String("id"),
				relationKey:                     pbtypes.String("value"),
				bundle.RelationKeyType.String(): pbtypes.String("objectType"),
			}})
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    relationKey,
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New("objectType")
		objectTypeDoc := objectType.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String("objectType"),
				bundle.RelationKeyType.String(): pbtypes.String("objectType"),
			}})
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
			SpaceId:   "spaceId",
			ObjectIds: []string{"id"},
			Format:    model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport()

		// then
		assert.Nil(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})

	t.Run("get relation options - no relation options", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		relationKey := "key"
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            pbtypes.String("id"),
				domain.RelationKey(relationKey): pbtypes.String("value"),
				bundle.RelationKeyType:          pbtypes.String("objectType"),
				bundle.RelationKeySpaceId:       pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:             pbtypes.String(relationKey),
				bundle.RelationKeyRelationKey:    pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:      pbtypes.String(uniqueKey.Marshal()),
				bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_status)),
				bundle.RelationKeySpaceId:        pbtypes.String("spaceId"),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String("id"),
				relationKey:                     pbtypes.String("value"),
				bundle.RelationKeyType.String(): pbtypes.String("objectType"),
			}})
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    relationKey,
			Format: model.RelationFormat_tag,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New("objectType")
		objectTypeDoc := objectType.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String("objectType"),
				bundle.RelationKeyType.String(): pbtypes.String("objectType"),
			}})
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
			SpaceId:       "spaceId",
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport()

		// then
		assert.Nil(t, err)
		assert.Equal(t, 2, len(expCtx.docs))
	})
	t.Run("get relation options - 1 relation option", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		relationKey := "key"
		optionId := "optionId"
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey)
		assert.Nil(t, err)
		optionUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelationOption, optionId)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            pbtypes.String("id"),
				domain.RelationKey(relationKey): pbtypes.String(optionId),
				bundle.RelationKeyType:          pbtypes.String("objectType"),
				bundle.RelationKeySpaceId:       pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:             pbtypes.String(relationKey),
				bundle.RelationKeyRelationKey:    pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:      pbtypes.String(uniqueKey.Marshal()),
				bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_tag)),
				bundle.RelationKeyLayout:         pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:        pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:          pbtypes.String(optionId),
				bundle.RelationKeyRelationKey: pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:   pbtypes.String(optionUniqueKey.Marshal()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relationOption)),
				bundle.RelationKeySpaceId:     pbtypes.String("spaceId"),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String("id"),
				relationKey:                     pbtypes.String("value"),
				bundle.RelationKeyType.String(): pbtypes.String("objectType"),
			}})
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    relationKey,
			Format: model.RelationFormat_tag,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New("objectType")
		objectTypeDoc := objectType.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String("objectType"),
				bundle.RelationKeyType.String(): pbtypes.String("objectType"),
			}})
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
			SpaceId:       "spaceId",
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport()

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
		relationKey := "key"
		objectTypeKey := "customObjectType"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeKey)
		assert.Nil(t, err)
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey)
		assert.Nil(t, err)

		recommendedRelationKey := "recommendedRelationKey"
		recommendedRelationUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, recommendedRelationKey)
		assert.Nil(t, err)

		templateId := "templateId"
		templateObjectTypeId := "templateObjectTypeId"

		linkedObjectId := "linkedObjectId"
		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            pbtypes.String("id"),
				domain.RelationKey(relationKey): pbtypes.String("test"),
				bundle.RelationKeyType:          pbtypes.String(objectTypeKey),
				bundle.RelationKeySpaceId:       pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:          pbtypes.String(relationKey),
				bundle.RelationKeyRelationKey: pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:   pbtypes.String(uniqueKey.Marshal()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:     pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:                   pbtypes.String(objectTypeKey),
				bundle.RelationKeyUniqueKey:            pbtypes.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:               pbtypes.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: pbtypes.StringList([]string{recommendedRelationKey}),
				bundle.RelationKeySpaceId:              pbtypes.String("spaceId"),
				bundle.RelationKeyType:                 pbtypes.String(objectTypeKey),
			},
			{
				bundle.RelationKeyId:          pbtypes.String(recommendedRelationKey),
				bundle.RelationKeyRelationKey: pbtypes.String(recommendedRelationKey),
				bundle.RelationKeyUniqueKey:   pbtypes.String(recommendedRelationUniqueKey.Marshal()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:     pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:               pbtypes.String(templateId),
				bundle.RelationKeyTargetObjectType: pbtypes.String(objectTypeKey),
				bundle.RelationKeySpaceId:          pbtypes.String("spaceId"),
				bundle.RelationKeyType:             pbtypes.String(templateObjectTypeId),
			},
			{
				bundle.RelationKeyId:      pbtypes.String(linkedObjectId),
				bundle.RelationKeyType:    pbtypes.String(objectTypeKey),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
			},
		})

		err = storeFixture.UpdateObjectLinks(templateId, []string{linkedObjectId})
		assert.Nil(t, err)

		objectGetter := mock_cache.NewMockObjectGetter(t)

		template := smarttest.New(templateId)
		templateDoc := template.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String(templateId),
				bundle.RelationKeyType.String(): pbtypes.String(templateObjectTypeId),
			}})
		templateDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		template.Doc = templateDoc

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String("id"),
				relationKey:                     pbtypes.String("value"),
				bundle.RelationKeyType.String(): pbtypes.String(objectTypeKey),
			}})
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    relationKey,
			Format: model.RelationFormat_tag,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeKey)
		objectTypeDoc := objectType.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String(objectTypeKey),
				bundle.RelationKeyType.String(): pbtypes.String(objectTypeKey),
			}})
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		templateObjectType := smarttest.New(objectTypeKey)
		templateObjectTypeDoc := templateObjectType.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String(templateId),
				bundle.RelationKeyType.String(): pbtypes.String(templateObjectTypeId),
			}})
		templateObjectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		templateObjectType.Doc = templateObjectTypeDoc

		linkedObject := smarttest.New(objectTypeKey)
		linkedObjectDoc := smartBlockTest.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String(linkedObjectId),
				bundle.RelationKeyType.String(): pbtypes.String(objectTypeKey),
			}})
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
		provider.EXPECT().Type("spaceId", linkedObjectId).Return(smartblock.SmartBlockTypePage, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
			sbtProvider: provider,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       "spaceId",
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport()

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

		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      pbtypes.String("id"),
				bundle.RelationKeyType:    pbtypes.String(objectTypeId),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:                   pbtypes.String(objectTypeId),
				bundle.RelationKeyUniqueKey:            pbtypes.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:               pbtypes.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: pbtypes.StringList([]string{addr.MissingObject}),
				bundle.RelationKeySpaceId:              pbtypes.String("spaceId"),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String("id"),
				bundle.RelationKeyType.String(): pbtypes.String(objectTypeId),
			}})
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String(objectTypeId),
				bundle.RelationKeyType.String(): pbtypes.String(objectTypeId),
			}})
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
			SpaceId:       "spaceId",
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport()

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

		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      pbtypes.String("id"),
				bundle.RelationKeyName:    pbtypes.String("name1"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				bundle.RelationKeyType:    pbtypes.String(objectTypeId),
			},
			{
				bundle.RelationKeyId:      pbtypes.String("id1"),
				bundle.RelationKeyName:    pbtypes.String("name2"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				bundle.RelationKeyType:    pbtypes.String(objectTypeId),
			},
			{
				bundle.RelationKeyId:        pbtypes.String(objectTypeId),
				bundle.RelationKeyUniqueKey: pbtypes.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:    pbtypes.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:   pbtypes.String("spaceId"),
				bundle.RelationKeyType:      pbtypes.String(objectTypeId),
			},
		})

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String("id"),
				bundle.RelationKeyType.String(): pbtypes.String(objectTypeId),
			}})
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String(objectTypeId),
				bundle.RelationKeyType.String(): pbtypes.String(objectTypeId),
			}})
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
			SpaceId:       "spaceId",
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport()

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

		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      pbtypes.String("id"),
				bundle.RelationKeyName:    pbtypes.String("name1"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				bundle.RelationKeyType:    pbtypes.String(objectTypeId),
				bundle.RelationKeyLayout:  pbtypes.Int64(int64(model.ObjectType_set)),
			},
			{
				bundle.RelationKeyId:        pbtypes.String(objectTypeId),
				bundle.RelationKeyUniqueKey: pbtypes.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:    pbtypes.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:   pbtypes.String("spaceId"),
				bundle.RelationKeyType:      pbtypes.String(objectTypeId),
			},
			{
				bundle.RelationKeyId:          pbtypes.String(bundle.RelationKeyTag.String()),
				bundle.RelationKeyName:        pbtypes.String(bundle.RelationKeyTag.String()),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyTag.String()),
				bundle.RelationKeySpaceId:     pbtypes.String("spaceId"),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyUniqueKey:   pbtypes.String(bundle.RelationKeyTag.URL()),
			},
			{
				bundle.RelationKeyId:          pbtypes.String(relationKey),
				bundle.RelationKeyName:        pbtypes.String(relationKey),
				bundle.RelationKeyRelationKey: pbtypes.String(relationKey),
				bundle.RelationKeySpaceId:     pbtypes.String("spaceId"),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyUniqueKey:   pbtypes.String(relationKeyUniqueKey.Marshal()),
			},
		})

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():     pbtypes.String("id"),
				bundle.RelationKeyType.String():   pbtypes.String(objectTypeId),
				bundle.RelationKeyLayout.String(): pbtypes.Int64(int64(model.ObjectType_set)),
			}})
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyLayout.String(),
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
		objectTypeDoc := objectType.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String(objectTypeId),
				bundle.RelationKeyType.String(): pbtypes.String(objectTypeId),
			}})
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
			SpaceId:       "spaceId",
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport()

		// then
		assert.Nil(t, err)
		assert.Equal(t, 3, len(expCtx.docs))
	})
	t.Run("objects without file", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeId := "objectTypeId"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeId)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      pbtypes.String("id"),
				bundle.RelationKeyName:    pbtypes.String("name1"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				bundle.RelationKeyType:    pbtypes.String(objectTypeId),
				bundle.RelationKeyLayout:  pbtypes.Int64(int64(model.ObjectType_set)),
			},
			{
				bundle.RelationKeyId:        pbtypes.String(objectTypeId),
				bundle.RelationKeyUniqueKey: pbtypes.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:    pbtypes.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:   pbtypes.String("spaceId"),
				bundle.RelationKeyType:      pbtypes.String(objectTypeId),
			},
		})

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():     pbtypes.String("id"),
				bundle.RelationKeyType.String():   pbtypes.String(objectTypeId),
				bundle.RelationKeyLayout.String(): pbtypes.Int64(int64(model.ObjectType_set)),
			}})
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyLayout.String(),
			Format: model.RelationFormat_number,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeId)
		objectTypeDoc := objectType.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String(objectTypeId),
				bundle.RelationKeyType.String(): pbtypes.String(objectTypeId),
			}})
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

		service := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().Do("id", mock.Anything).Return(nil)

		service.EXPECT().Get(context.Background(), "spaceId").Return(space, nil)
		e := &export{
			objectStore:  storeFixture,
			picker:       objectGetter,
			spaceService: service,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       "spaceId",
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			IncludeFiles:  true,
			Format:        model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport()

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

		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      pbtypes.String("id"),
				bundle.RelationKeyName:    pbtypes.String("name1"),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				bundle.RelationKeyType:    pbtypes.String(objectTypeId),
				bundle.RelationKeyLayout:  pbtypes.Int64(int64(model.ObjectType_set)),
			},
			{
				bundle.RelationKeyId:        pbtypes.String(objectTypeId),
				bundle.RelationKeyUniqueKey: pbtypes.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:    pbtypes.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:   pbtypes.String("spaceId"),
				bundle.RelationKeyType:      pbtypes.String(objectTypeId),
			},
		})

		service := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		space.EXPECT().Do("id", mock.Anything).Return(nil)
		service.EXPECT().Get(context.Background(), "spaceId").Return(space, nil)
		e := &export{
			objectStore:  storeFixture,
			spaceService: service,
		}

		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{
			SpaceId:       "spaceId",
			ObjectIds:     []string{"id"},
			IncludeNested: true,
			IncludeFiles:  true,
			Format:        model.Export_Markdown,
		})

		// when
		err = expCtx.docsForExport()

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

		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:      pbtypes.String("id"),
				bundle.RelationKeySetOf:   pbtypes.StringList([]string{relationKey}),
				bundle.RelationKeyType:    pbtypes.String(objectTypeKey),
				bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:          pbtypes.String(relationKey),
				bundle.RelationKeyRelationKey: pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:   pbtypes.String(uniqueKey.Marshal()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:     pbtypes.String("spaceId"),
				bundle.RelationKeyType:        pbtypes.String(relationObjectTypeKey),
			},
			{
				bundle.RelationKeyId:                   pbtypes.String(objectTypeKey),
				bundle.RelationKeyUniqueKey:            pbtypes.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:               pbtypes.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: pbtypes.StringList([]string{recommendedRelationKey}),
				bundle.RelationKeySpaceId:              pbtypes.String("spaceId"),
				bundle.RelationKeyType:                 pbtypes.String(objectTypeKey),
			},
			{
				bundle.RelationKeyId:        pbtypes.String(relationObjectTypeKey),
				bundle.RelationKeyUniqueKey: pbtypes.String(relationObjectTypeUK.Marshal()),
				bundle.RelationKeyLayout:    pbtypes.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeySpaceId:   pbtypes.String("spaceId"),
				bundle.RelationKeyType:      pbtypes.String(objectTypeKey),
			},
			{
				bundle.RelationKeyId:          pbtypes.String(recommendedRelationKey),
				bundle.RelationKeyRelationKey: pbtypes.String(recommendedRelationKey),
				bundle.RelationKeyUniqueKey:   pbtypes.String(recommendedRelationUniqueKey.Marshal()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:     pbtypes.String("spaceId"),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New("id")
		doc := smartBlockTest.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():    pbtypes.String("id"),
				bundle.RelationKeySetOf.String(): pbtypes.StringList([]string{relationKey}),
				bundle.RelationKeyType.String():  pbtypes.String(objectTypeKey),
			}})
		doc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		})
		smartBlockTest.Doc = doc

		objectType := smarttest.New(objectTypeKey)
		objectTypeDoc := objectType.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String(objectTypeKey),
				bundle.RelationKeyType.String(): pbtypes.String(objectTypeKey),
			}})
		objectTypeDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		objectType.Doc = objectTypeDoc

		relationObject := smarttest.New(relationKey)
		relationObjectDoc := relationObject.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String(relationKey),
				bundle.RelationKeyType.String(): pbtypes.String(relationObjectTypeKey),
			}})
		relationObjectDoc.AddRelationLinks(&model.RelationLink{
			Key:    bundle.RelationKeyId.String(),
			Format: model.RelationFormat_longtext,
		}, &model.RelationLink{
			Key:    bundle.RelationKeyType.String(),
			Format: model.RelationFormat_longtext,
		})
		relationObject.Doc = relationObjectDoc

		relationObjectType := smarttest.New(relationObjectTypeKey)
		relationObjectTypeDoc := relationObjectType.NewState().SetDetails(&types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():   pbtypes.String(relationObjectTypeKey),
				bundle.RelationKeyType.String(): pbtypes.String(objectTypeKey),
			}})
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
			SpaceId:   "spaceId",
			ObjectIds: []string{"id"},
			Format:    model.Export_Protobuf,
		})

		// when
		err = expCtx.docsForExport()
		// then
		assert.Nil(t, err)
		assert.Equal(t, 5, len(expCtx.docs))
	})
}

func Test_provideFileName(t *testing.T) {
	t.Run("file dir for relation", func(t *testing.T) {
		// when
		fileName := makeFileName("docId", "spaceId", pbjson.NewConverter(nil).Ext(), nil, smartblock.SmartBlockTypeRelation)

		// then
		assert.Equal(t, relationsDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("file dir for relation option", func(t *testing.T) {
		// when
		fileName := makeFileName("docId", "spaceId", pbjson.NewConverter(nil).Ext(), nil, smartblock.SmartBlockTypeRelationOption)

		// then
		assert.Equal(t, relationsOptionsDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("file dir for types", func(t *testing.T) {
		// when
		fileName := makeFileName("docId", "spaceId", pbjson.NewConverter(nil).Ext(), nil, smartblock.SmartBlockTypeObjectType)

		// then
		assert.Equal(t, typesDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("file dir for objects", func(t *testing.T) {
		// when
		fileName := makeFileName("docId", "spaceId", pbjson.NewConverter(nil).Ext(), nil, smartblock.SmartBlockTypePage)

		// then
		assert.Equal(t, objectsDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("file dir for files objects", func(t *testing.T) {
		// when
		fileName := makeFileName("docId", "spaceId", pbjson.NewConverter(nil).Ext(), nil, smartblock.SmartBlockTypeFileObject)

		// then
		assert.Equal(t, filesObjects+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("space is not provided", func(t *testing.T) {
		// given
		st := state.NewDoc("root", nil).(*state.State)
		st.SetDetail(bundle.RelationKeySpaceId.String(), pbtypes.String("spaceId"))

		// when
		fileName := makeFileName("docId", "", pbjson.NewConverter(st).Ext(), st, smartblock.SmartBlockTypeFileObject)

		// then
		assert.Equal(t, spaceDirectory+string(filepath.Separator)+"spaceId"+string(filepath.Separator)+filesObjects+string(filepath.Separator)+"docId.pb.json", fileName)
	})
}

func Test_queryObjectsFromStoreByIds(t *testing.T) {
	t.Run("query 10 objects", func(t *testing.T) {
		// given
		fixture := objectstore.NewStoreFixture(t)
		ids := make([]string, 0, 10)
		for i := 0; i < 10; i++ {
			id := fmt.Sprintf("%d", i)
			fixture.AddObjects(t, []objectstore.TestObject{
				{
					bundle.RelationKeyId:      pbtypes.String(id),
					bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				},
			})
			ids = append(ids, id)
		}
		e := &export{objectStore: fixture}
		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{})

		// when
		records, err := expCtx.queryAndFilterObjectsByRelation("spaceId", ids, bundle.RelationKeyId.String())

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
			fixture.AddObjects(t, []objectstore.TestObject{
				{
					bundle.RelationKeyId:      pbtypes.String(id),
					bundle.RelationKeySpaceId: pbtypes.String("spaceId"),
				},
			})
			ids = append(ids, id)
		}
		e := &export{objectStore: fixture}
		expCtx := newExportContext(e, pb.RpcObjectListExportRequest{})

		// when
		records, err := expCtx.queryAndFilterObjectsByRelation("spaceId", ids, bundle.RelationKeyId.String())

		// then
		assert.Nil(t, err)
		assert.Len(t, records, 2000)
	})
}
