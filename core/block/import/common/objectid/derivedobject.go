package objectid

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type derivedObject struct {
	existingObject *existingObject
	spaceService   space.Service
	objectStore    objectstore.ObjectStore
	internalKey    string
}

func newDerivedObject(existingObject *existingObject, spaceService space.Service, objectStore objectstore.ObjectStore) *derivedObject {
	return &derivedObject{existingObject: existingObject, spaceService: spaceService, objectStore: objectStore}
}

func (d *derivedObject) GetIDAndPayload(ctx context.Context, spaceID string, sn *common.Snapshot, _ time.Time, getExisting bool, _ objectorigin.ObjectOrigin) (string, treestorage.TreeStorageCreatePayload, error) {
	id, payload, err := d.existingObject.GetIDAndPayload(ctx, spaceID, sn, true)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, err
	}
	if id != "" {
		d.internalKey, err = d.getInternalKey(spaceID, id)
		if err != nil {
			return "", treestorage.TreeStorageCreatePayload{}, err
		}
		return id, payload, nil
	}
	rawUniqueKey := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyUniqueKey.String())
	uniqueKey, err := domain.UnmarshalUniqueKey(rawUniqueKey)
	if err != nil {
		uniqueKey, err = domain.NewUniqueKey(sn.SbType, sn.Snapshot.Data.Key)
		if err != nil {
			return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("create unique key from %s and %q: %w", sn.SbType, sn.Snapshot.Data.Key, err)
		}
	}

	var key string
	if d.isDeletedObject(spaceID, uniqueKey.Marshal()) {
		key = bson.NewObjectId().Hex()
		uniqueKey, err = domain.NewUniqueKey(sn.SbType, key)
		if err != nil {
			return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("create unique key from %s: %w", sn.SbType, err)
		}
	}
	d.internalKey = key
	spc, err := d.spaceService.Get(ctx, spaceID)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("get space : %w", err)
	}
	payload, err = spc.DeriveTreePayload(ctx, payloadcreator.PayloadDerivationParams{
		Key: uniqueKey,
	})
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("derive tree create payload: %w", err)
	}
	return payload.RootRawChange.Id, payload, nil
}
func (d *derivedObject) GetInternalKey(sbType sb.SmartBlockType) string {
	return d.internalKey
}

func (d *derivedObject) isDeletedObject(spaceId string, uniqueKey string) bool {
	ids, _, err := d.objectStore.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySpaceId.String(),
				Value:       pbtypes.String(spaceId),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Value:       pbtypes.String(uniqueKey),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyIsDeleted.String(),
				Value:       pbtypes.Bool(true),
			},
		},
	})
	return err == nil && len(ids) > 0
}

func (d *derivedObject) getInternalKey(spaceID, objectId string) (string, error) {
	ids, err := d.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyId.String(),
				Value:       pbtypes.String(objectId),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySpaceId.String(),
				Value:       pbtypes.String(spaceID),
			},
		},
	})
	if err == nil && len(ids) > 0 {
		uniqueKey := pbtypes.GetString(ids[0].Details, bundle.RelationKeyUniqueKey.String())
		key, err := domain.UnmarshalUniqueKey(uniqueKey)
		if err != nil {
			return "", nil
		}
		return key.InternalKey(), err
	}
	return "", nil
}
