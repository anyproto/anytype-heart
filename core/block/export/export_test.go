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
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
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

func Test_docsForExport(t *testing.T) {
	t.Run("get object with existing links", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId: pbtypes.String("id"),
			},
			{
				bundle.RelationKeyId: pbtypes.String("id1"),
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
				bundle.RelationKeyId: pbtypes.String("id"),
			},
			{
				bundle.RelationKeyId:        pbtypes.String("id1"),
				bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
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
		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:            pbtypes.String("id"),
				domain.RelationKey(relationKey): pbtypes.String("value"),
				bundle.RelationKeyType:          pbtypes.String("objectType"),
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

		storeFixture.AddObjects(t, []objectstore.TestObject{
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

		err = storeFixture.UpdateObjectLinks("id", []string{"id1"})
		assert.Nil(t, err)

		objectGetter := mock_cache.NewMockObjectGetter(t)

		smartBlockTest := smarttest.New("id")
		smartBlockRelation := smarttest.New("key")
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
		objectGetter.EXPECT().GetObject(context.Background(), "key").Return(smartBlockRelation, nil)

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
