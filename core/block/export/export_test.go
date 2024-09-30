package export

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gogo/protobuf/types"
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

const spaceId = "space1"

func Test_docsForExport(t *testing.T) {
	t.Run("get object with existing links", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:   pbtypes.String("id"),
				bundle.RelationKeyName: pbtypes.String("name1"),
			},
			{
				bundle.RelationKeyId:   pbtypes.String("id1"),
				bundle.RelationKeyName: pbtypes.String("name2"),
			},
		})
		err := storeFixture.SpaceId(spaceId).UpdateObjectLinks(context.Background(), "id", []string{"id1"})
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
		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:   pbtypes.String("id"),
				bundle.RelationKeyName: pbtypes.String("name"),
			},
			{
				bundle.RelationKeyId:        pbtypes.String("id1"),
				bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
			},
		})
		err := storeFixture.SpaceId(spaceId).UpdateObjectLinks(context.Background(), "id", []string{"id1"})
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
		relationKey := "key"
		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            pbtypes.String("id"),
				domain.RelationKey(relationKey): pbtypes.String("value"),
				bundle.RelationKeyType:          pbtypes.String("objectType"),
			},
		})
		err := storeFixture.SpaceId(spaceId).UpdateObjectLinks(context.Background(), "id", []string{"id1"})
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
		relationKey := "key"
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            pbtypes.String("id"),
				domain.RelationKey(relationKey): pbtypes.String("value"),
				bundle.RelationKeyType:          pbtypes.String("objectType"),
			},
			{
				bundle.RelationKeyId:          pbtypes.String(relationKey),
				bundle.RelationKeyRelationKey: pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:   pbtypes.String(uniqueKey.Marshal()),
			},
		})

		err = storeFixture.SpaceId(spaceId).UpdateObjectLinks(context.Background(), "id", []string{"id1"})
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
		relationKey := "key"
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            pbtypes.String("id"),
				domain.RelationKey(relationKey): pbtypes.String("value"),
				bundle.RelationKeyType:          pbtypes.String("objectType"),
			},
			{
				bundle.RelationKeyId:             pbtypes.String(relationKey),
				bundle.RelationKeyRelationKey:    pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:      pbtypes.String(uniqueKey.Marshal()),
				bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_status)),
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
		relationKey := "key"
		optionId := "optionId"
		uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey)
		assert.Nil(t, err)
		optionUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelationOption, optionId)
		assert.Nil(t, err)

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            pbtypes.String("id"),
				domain.RelationKey(relationKey): pbtypes.String(optionId),
				bundle.RelationKeyType:          pbtypes.String("objectType"),
			},
			{
				bundle.RelationKeyId:             pbtypes.String(relationKey),
				bundle.RelationKeyRelationKey:    pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:      pbtypes.String(uniqueKey.Marshal()),
				bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_tag)),
				bundle.RelationKeyLayout:         pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:          pbtypes.String(optionId),
				bundle.RelationKeyRelationKey: pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:   pbtypes.String(optionUniqueKey.Marshal()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relationOption)),
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

		linkedObjectId := "linkedObjectId"
		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            pbtypes.String("id"),
				domain.RelationKey(relationKey): pbtypes.String("test"),
				bundle.RelationKeyType:          pbtypes.String(objectTypeKey),
			},
			{
				bundle.RelationKeyId:          pbtypes.String(relationKey),
				bundle.RelationKeyRelationKey: pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:   pbtypes.String(uniqueKey.Marshal()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:                   pbtypes.String(objectTypeKey),
				bundle.RelationKeyUniqueKey:            pbtypes.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:               pbtypes.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: pbtypes.StringList([]string{recommendedRelationKey}),
			},
			{
				bundle.RelationKeyId:          pbtypes.String(recommendedRelationKey),
				bundle.RelationKeyRelationKey: pbtypes.String(recommendedRelationKey),
				bundle.RelationKeyUniqueKey:   pbtypes.String(recommendedRelationUniqueKey.Marshal()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:               pbtypes.String(templateId),
				bundle.RelationKeyTargetObjectType: pbtypes.String(objectTypeKey),
			},
			{
				bundle.RelationKeyId:   pbtypes.String(linkedObjectId),
				bundle.RelationKeyType: pbtypes.String(objectTypeKey),
			},
		})

		err = storeFixture.SpaceId(spaceId).UpdateObjectLinks(context.Background(), templateId, []string{linkedObjectId})
		assert.Nil(t, err)

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New("id")
		smartBlockTemplate := smarttest.New(templateId)
		smartBlockObjectType := smarttest.New(objectTypeKey)
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

		storeFixture.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:   pbtypes.String("id"),
				bundle.RelationKeyType: pbtypes.String(objectTypeKey),
			},
			{
				bundle.RelationKeyId:                   pbtypes.String(objectTypeKey),
				bundle.RelationKeyUniqueKey:            pbtypes.String(objectTypeUniqueKey.Marshal()),
				bundle.RelationKeyLayout:               pbtypes.Int64(int64(model.ObjectType_objectType)),
				bundle.RelationKeyRecommendedRelations: pbtypes.StringList([]string{addr.MissingObject}),
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
		st.SetDetail(bundle.RelationKeySpaceId.String(), pbtypes.String("spaceId"))

		// when
		fileName := e.makeFileName("docId", "", pbjson.NewConverter(st), st, smartblock.SmartBlockTypeFileObject)

		// then
		assert.Equal(t, spaceDirectory+string(filepath.Separator)+"spaceId"+string(filepath.Separator)+filesObjects+string(filepath.Separator)+"docId.pb.json", fileName)
	})
}
