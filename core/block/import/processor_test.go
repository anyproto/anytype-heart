package importer

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/objectid/mock_objectid"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestReplaceRelationKeyWithNew(t *testing.T) {
	t.Run("no matching relation id in oldIDToNew map", func(t *testing.T) {
		// given
		option := &common.Snapshot{
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyRelationKey: domain.String("key"),
					}),
				},
				SbType: smartblock.SmartBlockTypeSubObject,
			},
		}
		oldIDToNew := make(map[string]string, 0)

		// when
		replaceRelationKeyValue(option, oldIDToNew)

		// then
		assert.Equal(t, "key", option.Snapshot.Data.Details.GetString(bundle.RelationKeyRelationKey))
	})
	t.Run("oldIDToNew map have relation id", func(t *testing.T) {
		// given
		option := &common.Snapshot{
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyRelationKey: domain.String("key"),
					}),
				},
				SbType: smartblock.SmartBlockTypeSubObject,
			},
		}
		oldIDToNew := map[string]string{"key": "newkey"}

		// when
		replaceRelationKeyValue(option, oldIDToNew)

		// then
		assert.Equal(t, "newkey", option.Snapshot.Data.Details.GetString(bundle.RelationKeyRelationKey))
	})

	t.Run("no details", func(t *testing.T) {
		// given
		option := &common.Snapshot{
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Details: nil,
				},
				SbType: smartblock.SmartBlockTypeSubObject,
			},
		}
		oldIDToNew := map[string]string{"rel-key": "rel-newkey"}

		// when
		replaceRelationKeyValue(option, oldIDToNew)

		// then
		assert.Nil(t, option.Snapshot.Data.Details)
	})
}

func TestImportProcessor_processSnapshot(t *testing.T) {
	t.Run("get object new id", func(t *testing.T) {
		// given
		p := importProcessor{
			deps: &Dependencies{},
			request: &ImportRequest{
				RpcObjectImportRequest: &pb.RpcObjectImportRequest{SpaceId: "spaceId"},
				Origin:                 objectorigin.Import(model.Import_Pb),
			},
			oldIDToNew:     make(map[string]string),
			createPayloads: make(map[string]treestorage.TreeStorageCreatePayload),
		}
		sn := &common.Snapshot{
			Id: "oldId",
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{},
			},
		}
		idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
		idGetter.EXPECT().GetInternalKey(mock.Anything).Return("").Times(1)
		idGetter.EXPECT().GetIDAndPayload(context.Background(), "spaceId", sn, mock.Anything, false, objectorigin.Import(model.Import_Pb)).Return("newId", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		p.deps.idProvider = idGetter

		// when
		err := p.processSnapshot(context.Background(), sn)

		// then
		assert.Nil(t, err)
		assert.Equal(t, "newId", p.oldIDToNew["oldId"])
	})
	t.Run("get object new id and new key", func(t *testing.T) {
		// given
		p := importProcessor{
			deps: &Dependencies{},
			request: &ImportRequest{
				RpcObjectImportRequest: &pb.RpcObjectImportRequest{SpaceId: "spaceId"},
				Origin:                 objectorigin.Import(model.Import_Pb),
			},
			oldIDToNew:     make(map[string]string),
			createPayloads: make(map[string]treestorage.TreeStorageCreatePayload),
		}
		sn := &common.Snapshot{
			Id: "oldId",
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Key: "key",
				},
			},
		}
		idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
		idGetter.EXPECT().GetInternalKey(mock.Anything).Return("newKey").Times(1)
		idGetter.EXPECT().GetIDAndPayload(context.Background(), "spaceId", sn, mock.Anything, false, objectorigin.Import(model.Import_Pb)).Return("newId", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		p.deps.idProvider = idGetter

		// when
		err := p.processSnapshot(context.Background(), sn)

		// then
		assert.Nil(t, err)
		assert.Equal(t, "newId", p.oldIDToNew["oldId"])
		assert.Equal(t, "newKey", p.oldIDToNew["key"])
	})
	t.Run("get object new id and new key", func(t *testing.T) {
		// given
		p := importProcessor{
			deps: &Dependencies{},
			request: &ImportRequest{
				RpcObjectImportRequest: &pb.RpcObjectImportRequest{SpaceId: "spaceId"},
				Origin:                 objectorigin.Import(model.Import_Pb),
			},
			oldIDToNew:     make(map[string]string),
			createPayloads: make(map[string]treestorage.TreeStorageCreatePayload),
		}
		sn := &common.Snapshot{
			Id: "oldId",
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyUniqueKey: domain.String("key"),
					}),
				},
			},
		}
		idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
		idGetter.EXPECT().GetInternalKey(mock.Anything).Return("newKey").Times(1)
		idGetter.EXPECT().GetIDAndPayload(context.Background(), "spaceId", sn, mock.Anything, false, objectorigin.Import(model.Import_Pb)).Return("newId", treestorage.TreeStorageCreatePayload{}, nil).Times(1)
		p.deps.idProvider = idGetter

		// when
		err := p.processSnapshot(context.Background(), sn)

		// then
		assert.Nil(t, err)
		assert.Equal(t, "newId", p.oldIDToNew["oldId"])
		assert.Equal(t, "newKey", p.oldIDToNew["key"])
	})
	t.Run("don't add create payload", func(t *testing.T) {
		// given
		p := importProcessor{
			deps: &Dependencies{},
			request: &ImportRequest{
				RpcObjectImportRequest: &pb.RpcObjectImportRequest{SpaceId: "spaceId"},
				Origin:                 objectorigin.Import(model.Import_Pb),
			},
			oldIDToNew:     make(map[string]string),
			createPayloads: make(map[string]treestorage.TreeStorageCreatePayload),
		}
		sn := &common.Snapshot{
			Id: "oldId",
			Snapshot: &common.SnapshotModel{
				Data: &common.StateSnapshot{
					Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
						bundle.RelationKeyUniqueKey: domain.String("key"),
					}),
				},
			},
		}
		idGetter := mock_objectid.NewMockIdAndKeyProvider(t)
		idGetter.EXPECT().GetInternalKey(mock.Anything).Return("newKey").Times(1)
		idGetter.EXPECT().GetIDAndPayload(context.Background(), "spaceId", sn, mock.Anything, false, objectorigin.Import(model.Import_Pb)).Return("newId", treestorage.TreeStorageCreatePayload{
			RootRawChange: &treechangeproto.RawTreeChangeWithId{},
		}, nil).Times(1)
		p.deps.idProvider = idGetter

		// when
		err := p.processSnapshot(context.Background(), sn)

		// then
		assert.Nil(t, err)
		assert.Equal(t, "newId", p.oldIDToNew["oldId"])
		assert.Equal(t, "newKey", p.oldIDToNew["key"])
		assert.NotNil(t, p.createPayloads["newId"])
	})
}
