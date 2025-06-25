package objectid

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
)

func TestDerivedObject_GetIDAndPayload(t *testing.T) {
	t.Run("try to recreate deleted object", func(t *testing.T) {
		// given
		sf := objectstore.NewStoreFixture(t)
		service := mock_space.NewMockService(t)
		deriveObject := newDerivedObject(newExistingObject(sf), service, sf)
		sn := &common.Snapshot{
			Id: "oldId",
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyUniqueKey: domain.String("key"),
					}),
					Key: "oldKey",
				},
				SbType: coresb.SmartBlockTypePage,
			},
		}
		space := mock_clientspace.NewMockSpace(t)
		service.EXPECT().Get(context.Background(), "spaceId").Return(space, nil)
		space.EXPECT().DeriveTreePayload(context.Background(), mock.Anything).Return(treestorage.TreeStorageCreatePayload{
			RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: "newId"},
		}, nil)

		uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypePage, "oldKey")
		assert.Nil(t, err)
		sf.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyUniqueKey: domain.String(uniqueKey.Marshal()),
				bundle.RelationKeyId:        domain.String("oldId"),
				bundle.RelationKeyIsDeleted: domain.Bool(true),
			},
		})

		// when
		id, _, err := deriveObject.GetIDAndPayload(context.Background(), "spaceId", sn, time.Now(), false, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.NotEqual(t, deriveObject.GetInternalKey(sn.Snapshot.SbType), "key")
		assert.Equal(t, "newId", id)
	})
	t.Run("existing object", func(t *testing.T) {
		// given
		sf := objectstore.NewStoreFixture(t)
		service := mock_space.NewMockService(t)
		deriveObject := newDerivedObject(newExistingObject(sf), service, sf)
		sn := &common.Snapshot{
			Id: "oldId",
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyName:           domain.String("IMPORTED NAME"),
						bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_number)),
						bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyName.String()),
					}),
				},
				SbType: coresb.SmartBlockTypeRelation,
			},
		}

		uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, "oldKey")
		assert.Nil(t, err)
		sf.AddObjects(t, "spaceId", []objectstore.TestObject{
			{
				bundle.RelationKeyUniqueKey:      domain.String(uniqueKey.Marshal()),
				bundle.RelationKeyId:             domain.String("oldId"),
				bundle.RelationKeyName:           domain.String("name"),
				bundle.RelationKeyRelationKey:    domain.String(bundle.RelationKeyName.String()),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_number)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeySpaceId:        domain.String("spaceId"),
			},
		})

		// when
		id, _, err := deriveObject.GetIDAndPayload(context.Background(), "spaceId", sn, time.Now(), false, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.Equal(t, "oldId", id)
	})
}
