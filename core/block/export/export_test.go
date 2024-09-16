package export

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/converter/pbjson"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider/mock_typeprovider"
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

func Test_docsForExport(t *testing.T) {
	t.Run("get object with existing links", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:   domain.String("id"),
				bundle.RelationKeyName: domain.String("name1"),
			},
			{
				bundle.RelationKeyId:   domain.String("id1"),
				bundle.RelationKeyName: domain.String("name2"),
			},
		})
		err := storeFixture.UpdateObjectLinks(context.Background(), "id", []string{"id1"})
		assert.Nil(t, err)

		provider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		provider.EXPECT().Type("spaceId", "id1").Return(smartblock.SmartBlockTypePage, nil)
		e := &export{
			objectStore: storeFixture,
			sbtProvider: provider,
		}

		// when
		docsForExport, err := e.docsForExport("spaceId", pb.RpcObjectListExportRequest{
			SpaceId:       "spaceId",
			ObjectIds:     []string{"id"},
			IncludeNested: true,
		})

		// then
		assert.Nil(t, err)
		assert.Equal(t, 2, len(docsForExport))
	})
	t.Run("get object with non existing links", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:   domain.String("id"),
				bundle.RelationKeyName: domain.String("name"),
			},
			{
				bundle.RelationKeyId:        domain.String("id1"),
				bundle.RelationKeyIsDeleted: domain.Bool(true),
			},
		})
		err := storeFixture.UpdateObjectLinks(context.Background(), "id", []string{"id1"})
		assert.Nil(t, err)

		provider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		provider.EXPECT().Type("spaceId", "id1").Return(smartblock.SmartBlockTypePage, nil)
		e := &export{
			objectStore: storeFixture,
			sbtProvider: provider,
		}

		// when
		docsForExport, err := e.docsForExport("spaceId", pb.RpcObjectListExportRequest{
			SpaceId:       "spaceId",
			ObjectIds:     []string{"id"},
			IncludeNested: true,
		})

		// then
		assert.Nil(t, err)
		assert.Equal(t, 1, len(docsForExport))
	})
	t.Run("get object with non existing relation", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		relationKey := domain.RelationKey("key")
		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            domain.String("id"),
				domain.RelationKey(relationKey): domain.String("value"),
				bundle.RelationKeyType:          domain.String("objectType"),
			},
		})
		err := storeFixture.UpdateObjectLinks(context.Background(), "id", []string{"id1"})
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
		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		// when
		docsForExport, err := e.docsForExport("spaceId", pb.RpcObjectListExportRequest{
			SpaceId:   "spaceId",
			ObjectIds: []string{"id"},
			Format:    model.Export_Protobuf,
		})

		// then
		assert.Nil(t, err)
		assert.Equal(t, 1, len(docsForExport))
	})
	t.Run("get object with existing relation", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		relationKey := domain.RelationKey("key")
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey.String())
		assert.Nil(t, err)

		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            domain.String("id"),
				domain.RelationKey(relationKey): domain.String("value"),
				bundle.RelationKeyType:          domain.String("objectType"),
			},
			{
				bundle.RelationKeyId:          domain.String(relationKey),
				bundle.RelationKeyRelationKey: domain.String(relationKey),
				bundle.RelationKeyUniqueKey:   domain.String(uniqueKey.Marshal()),
			},
		})

		err = storeFixture.UpdateObjectLinks(context.Background(), "id", []string{"id1"})
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

		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		// when
		docsForExport, err := e.docsForExport("spaceId", pb.RpcObjectListExportRequest{
			SpaceId:   "spaceId",
			ObjectIds: []string{"id"},
			Format:    model.Export_Protobuf,
		})

		// then
		assert.Nil(t, err)
		assert.Equal(t, 2, len(docsForExport))
	})

	t.Run("get relation options - no relation options", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		relationKey := domain.RelationKey("key")
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey.String())
		assert.Nil(t, err)

		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            domain.String("id"),
				domain.RelationKey(relationKey): domain.String("value"),
				bundle.RelationKeyType:          domain.String("objectType"),
			},
			{
				bundle.RelationKeyId:             domain.String(relationKey),
				bundle.RelationKeyRelationKey:    domain.String(relationKey),
				bundle.RelationKeyUniqueKey:      domain.String(uniqueKey.Marshal()),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
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

		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		// when
		docsForExport, err := e.docsForExport("spaceId", pb.RpcObjectListExportRequest{
			SpaceId:   "spaceId",
			ObjectIds: []string{"id"},
			Format:    model.Export_Protobuf,
		})

		// then
		assert.Nil(t, err)
		assert.Equal(t, 2, len(docsForExport))
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

		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            domain.String("id"),
				domain.RelationKey(relationKey): domain.String(optionId),
				bundle.RelationKeyType:          domain.String("objectType"),
			},
			{
				bundle.RelationKeyId:             domain.String(relationKey),
				bundle.RelationKeyRelationKey:    domain.String(relationKey),
				bundle.RelationKeyUniqueKey:      domain.String(uniqueKey.Marshal()),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
				bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:          domain.String(optionId),
				bundle.RelationKeyRelationKey: domain.String(relationKey),
				bundle.RelationKeyUniqueKey:   domain.String(optionUniqueKey.Marshal()),
				bundle.RelationKeyLayout:      domain.Int64(int64(model.ObjectType_relationOption)),
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

		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		// when
		docsForExport, err := e.docsForExport("spaceId", pb.RpcObjectListExportRequest{
			SpaceId:   "spaceId",
			ObjectIds: []string{"id"},
			Format:    model.Export_Protobuf,
		})

		// then
		assert.Nil(t, err)
		assert.Equal(t, 3, len(docsForExport))
		var objectIds []string
		for objectId := range docsForExport {
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

		linkedObjectId := "linkedObjectId"
		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            domain.String("id"),
				domain.RelationKey(relationKey): domain.String("test"),
				bundle.RelationKeyType:          domain.String(objectTypeKey),
			},
			{
				bundle.RelationKeyId:          domain.String(relationKey),
				bundle.RelationKeyRelationKey: domain.String(relationKey),
				bundle.RelationKeyUniqueKey:   domain.String(uniqueKey.Marshal()),
				bundle.RelationKeyLayout:      domain.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:                   domain.String(objectTypeKey),
				bundle.RelationKeyUniqueKey:            domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:               domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{recommendedRelationKey}),
			},
			{
				bundle.RelationKeyId:          domain.String(recommendedRelationKey),
				bundle.RelationKeyRelationKey: domain.String(recommendedRelationKey),
				bundle.RelationKeyUniqueKey:   domain.String(recommendedRelationUniqueKey.Marshal()),
				bundle.RelationKeyLayout:      domain.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:               domain.String(templateId),
				bundle.RelationKeyTargetObjectType: domain.String(objectTypeKey),
			},
			{
				bundle.RelationKeyId:   domain.String(linkedObjectId),
				bundle.RelationKeyType: domain.String(objectTypeKey),
			},
		})

		err = storeFixture.UpdateObjectLinks(context.Background(), templateId, []string{linkedObjectId})
		assert.Nil(t, err)

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New("id")
		smartBlockTemplate := smarttest.New(templateId)
		smartBlockObjectType := smarttest.New(objectTypeKey)
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

		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), templateId).Return(smartBlockTemplate, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeKey).Return(smartBlockObjectType, nil)

		provider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
		provider.EXPECT().Type("spaceId", linkedObjectId).Return(smartblock.SmartBlockTypePage, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
			sbtProvider: provider,
		}

		// when
		docsForExport, err := e.docsForExport("spaceId", pb.RpcObjectListExportRequest{
			SpaceId:       "spaceId",
			ObjectIds:     []string{"id"},
			Format:        model.Export_Protobuf,
			IncludeNested: true,
		})

		// then
		assert.Nil(t, err)
		assert.Equal(t, 6, len(docsForExport))
	})
	t.Run("get derived objects, object type have missing relations - return only object and its type", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectTypeKey := "customObjectType"
		objectTypeUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, objectTypeKey)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:   domain.String("id"),
				bundle.RelationKeyType: domain.String(objectTypeKey),
			},
			{
				bundle.RelationKeyId:                   domain.String(objectTypeKey),
				bundle.RelationKeyUniqueKey:            domain.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:               domain.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: domain.StringList([]string{addr.MissingObject}),
			},
		})

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New("id")
		smartBlockObjectType := smarttest.New(objectTypeKey)

		objectGetter.EXPECT().GetObject(context.Background(), "id").Return(smartBlockTest, nil)
		objectGetter.EXPECT().GetObject(context.Background(), objectTypeKey).Return(smartBlockObjectType, nil)

		e := &export{
			objectStore: storeFixture,
			picker:      objectGetter,
		}

		// when
		docsForExport, err := e.docsForExport("spaceId", pb.RpcObjectListExportRequest{
			SpaceId:   "spaceId",
			ObjectIds: []string{"id"},
			Format:    model.Export_Protobuf,
		})

		// then
		assert.Nil(t, err)
		assert.Equal(t, 2, len(docsForExport))
	})
}

func Test_provideFileName(t *testing.T) {
	t.Run("file dir for relation", func(t *testing.T) {
		// given
		e := &export{}

		// when
		fileName := e.makeFileName("docId", "spaceId", pbjson.NewConverter(nil), nil, smartblock.SmartBlockTypeRelation)

		// then
		assert.Equal(t, relationsDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("file dir for relation option", func(t *testing.T) {
		// given
		e := &export{}

		// when
		fileName := e.makeFileName("docId", "spaceId", pbjson.NewConverter(nil), nil, smartblock.SmartBlockTypeRelationOption)

		// then
		assert.Equal(t, relationsOptionsDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("file dir for types", func(t *testing.T) {
		// given
		e := &export{}

		// when
		fileName := e.makeFileName("docId", "spaceId", pbjson.NewConverter(nil), nil, smartblock.SmartBlockTypeObjectType)

		// then
		assert.Equal(t, typesDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("file dir for objects", func(t *testing.T) {
		// given
		e := &export{}

		// when
		fileName := e.makeFileName("docId", "spaceId", pbjson.NewConverter(nil), nil, smartblock.SmartBlockTypePage)

		// then
		assert.Equal(t, objectsDirectory+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("file dir for files objects", func(t *testing.T) {
		// given
		e := &export{}

		// when
		fileName := e.makeFileName("docId", "spaceId", pbjson.NewConverter(nil), nil, smartblock.SmartBlockTypeFileObject)

		// then
		assert.Equal(t, filesObjects+string(filepath.Separator)+"docId.pb.json", fileName)
	})
	t.Run("space is not provided", func(t *testing.T) {
		// given
		e := &export{}
		st := state.NewDoc("root", nil).(*state.State)
		st.SetDetail(bundle.RelationKeySpaceId, domain.String("spaceId"))

		// when
		fileName := e.makeFileName("docId", "", pbjson.NewConverter(st), st, smartblock.SmartBlockTypeFileObject)

		// then
		assert.Equal(t, spaceDirectory+string(filepath.Separator)+"spaceId"+string(filepath.Separator)+filesObjects+string(filepath.Separator)+"docId.pb.json", fileName)
	})
}
