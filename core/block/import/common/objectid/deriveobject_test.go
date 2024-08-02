package objectid

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	types2 "github.com/anyproto/anytype-heart/core/block/import/common/types"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestDerivedObject_GetIDAndPayload(t *testing.T) {
	t.Run("try to recreate deleted object", func(t *testing.T) {
		// given
		sf := objectstore.NewStoreFixture(t)
		service := mock_space.NewMockService(t)
		deriveObject := newDerivedObject(newExistingObject(sf), service, sf)
		sn := &types2.Snapshot{
			Id: "oldId",
			Snapshot: &pb.ChangeSnapshot{
				Data: &model.SmartBlockSnapshotBase{
					Details: &types.Struct{Fields: map[string]*types.Value{
						bundle.RelationKeyUniqueKey.String(): pbtypes.String("key"),
					}},
					Key: "oldKey",
				},
			},
			SbType: coresb.SmartBlockTypePage,
		}
		space := mock_clientspace.NewMockSpace(t)
		service.EXPECT().Get(context.Background(), "spaceId").Return(space, nil)
		space.EXPECT().DeriveTreePayload(context.Background(), mock.Anything).Return(treestorage.TreeStorageCreatePayload{
			RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: "newId"},
		}, nil)

		uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypePage, "oldKey")
		assert.Nil(t, err)
		sf.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyUniqueKey: pbtypes.String(uniqueKey.Marshal()),
				bundle.RelationKeyId:        pbtypes.String("oldId"),
				bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
			},
		})

		// when
		id, _, err := deriveObject.GetIDAndPayload(context.Background(), "spaceId", sn, time.Now(), false, objectorigin.Import(model.Import_Pb))

		// then
		assert.Nil(t, err)
		assert.NotEqual(t, deriveObject.GetInternalKey(sn.SbType), "key")
		assert.Equal(t, "newId", id)
	})
}
