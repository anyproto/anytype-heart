package objectid

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type existingObject struct {
	objectStore objectstore.ObjectStore
}

func newExistingObject(objectStore objectstore.ObjectStore) *existingObject {
	return &existingObject{objectStore: objectStore}
}

func (e *existingObject) GetIDAndPayload(_ context.Context, spaceID string, sn *common.Snapshot, getExisting bool) (string, treestorage.TreeStorageCreatePayload, error) {
	id, err := e.getObjectByOldAnytypeID(spaceID, sn)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("get object by old anytype id: %w", err)
	}
	if id != "" {
		return id, treestorage.TreeStorageCreatePayload{}, nil
	}
	if getExisting {
		id = e.getExistingObject(spaceID, sn)
		if id != "" {
			return id, treestorage.TreeStorageCreatePayload{}, nil
		}
	}
	if sn.SbType == sb.SmartBlockTypeRelationOption {
		return e.getExistingRelationOption(sn, spaceID), treestorage.TreeStorageCreatePayload{}, nil
	}
	if sn.SbType == sb.SmartBlockTypeRelation {
		return e.getExistingRelation(sn, spaceID), treestorage.TreeStorageCreatePayload{}, nil
	}
	return "", treestorage.TreeStorageCreatePayload{}, nil
}

func (e *existingObject) getObjectByOldAnytypeID(spaceID string, sn *common.Snapshot) (string, error) {
	oldAnytypeID := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyOldAnytypeID.String())

	// Check for imported objects
	ids, _, err := e.objectStore.SpaceId(spaceID).QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyOldAnytypeID.String(),
				Value:       pbtypes.String(oldAnytypeID),
			},
		},
	})
	if err == nil && len(ids) > 0 {
		return ids[0], nil
	}

	// Check for derived objects
	ids, _, err = e.objectStore.SpaceId(spaceID).QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Value:       pbtypes.String(oldAnytypeID), // Old id equals to unique key
			},
		},
	})
	if err == nil && len(ids) > 0 {
		return ids[0], nil
	}

	return "", err
}

func (e *existingObject) getExistingObject(spaceID string, sn *common.Snapshot) string {
	source := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeySourceFilePath.String())
	ids, _, err := e.objectStore.SpaceId(spaceID).QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySourceFilePath.String(),
				Value:       pbtypes.String(source),
			},
		},
	})
	if err == nil && len(ids) > 0 {
		return ids[0]
	}
	return ""
}

func (e *existingObject) getExistingRelationOption(snapshot *common.Snapshot, spaceID string) string {
	name := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyName.String())
	key := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyRelationKey.String())
	ids, _, err := e.objectStore.SpaceId(spaceID).QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyName.String(),
				Value:       pbtypes.String(name),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyRelationKey.String(),
				Value:       pbtypes.String(key),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyLayout.String(),
				Value:       pbtypes.Int64(int64(model.ObjectType_relationOption)),
			},
		},
	})
	if err == nil && len(ids) > 0 {
		return ids[0]
	}
	return ""
}

func (e *existingObject) getExistingRelation(snapshot *common.Snapshot, spaceID string) string {
	name := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyName.String())
	format := pbtypes.GetFloat64(snapshot.Snapshot.Data.Details, bundle.RelationKeyRelationFormat.String())
	ids, _, err := e.objectStore.SpaceId(spaceID).QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyName.String(),
				Value:       pbtypes.String(name),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyRelationFormat.String(),
				Value:       pbtypes.Float64(format),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyLayout.String(),
				Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
			},
		},
	})
	if err == nil && len(ids) > 0 {
		return ids[0]
	}
	return ""
}
