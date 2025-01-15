package objectid

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
	if sn.Snapshot.SbType == sb.SmartBlockTypeRelationOption {
		return e.getExistingRelationOption(sn, spaceID), treestorage.TreeStorageCreatePayload{}, nil
	}
	if sn.Snapshot.SbType == sb.SmartBlockTypeRelation {
		return e.getExistingRelation(sn, spaceID), treestorage.TreeStorageCreatePayload{}, nil
	}
	return "", treestorage.TreeStorageCreatePayload{}, nil
}

func (e *existingObject) getObjectByOldAnytypeID(spaceID string, sn *common.Snapshot) (string, error) {
	oldAnytypeID := sn.Snapshot.Data.Details.GetString(bundle.RelationKeyOldAnytypeID)

	// Check for imported objects
	ids, _, err := e.objectStore.SpaceIndex(spaceID).QueryObjectIds(database.Query{
		Filters: []database.FilterRequest{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyOldAnytypeID,
				Value:       domain.String(oldAnytypeID),
			},
		},
	})
	if err == nil && len(ids) > 0 {
		return ids[0], nil
	}

	// Check for derived objects
	ids, _, err = e.objectStore.SpaceIndex(spaceID).QueryObjectIds(database.Query{
		Filters: []database.FilterRequest{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyUniqueKey,
				Value:       domain.String(oldAnytypeID), // Old id equals to unique key
			},
		},
	})
	if err == nil && len(ids) > 0 {
		return ids[0], nil
	}

	return "", err
}

func (e *existingObject) getExistingObject(spaceID string, sn *common.Snapshot) string {
	source := sn.Snapshot.Data.Details.GetString(bundle.RelationKeySourceFilePath)
	ids, _, err := e.objectStore.SpaceIndex(spaceID).QueryObjectIds(database.Query{
		Filters: []database.FilterRequest{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySourceFilePath,
				Value:       domain.String(source),
			},
		},
	})
	if err == nil && len(ids) > 0 {
		return ids[0]
	}
	return ""
}

func (e *existingObject) getExistingRelationOption(snapshot *common.Snapshot, spaceID string) string {
	name := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	key := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyRelationKey)
	ids, _, err := e.objectStore.SpaceIndex(spaceID).QueryObjectIds(database.Query{
		Filters: []database.FilterRequest{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyName,
				Value:       domain.String(name),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyRelationKey,
				Value:       domain.String(key),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyLayout,
				Value:       domain.Int64(model.ObjectType_relationOption),
			},
		},
	})
	if err == nil && len(ids) > 0 {
		return ids[0]
	}
	return ""
}

func (e *existingObject) getExistingRelation(snapshot *common.Snapshot, spaceID string) string {
	name := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	format := snapshot.Snapshot.Data.Details.GetFloat64(bundle.RelationKeyRelationFormat)
	ids, _, err := e.objectStore.SpaceIndex(spaceID).QueryObjectIds(database.Query{
		Filters: []database.FilterRequest{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyName,
				Value:       domain.String(name),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyRelationFormat,
				Value:       domain.Float64(format),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyLayout,
				Value:       domain.Int64(model.ObjectType_relation),
			},
		},
	})
	if err == nil && len(ids) > 0 {
		return ids[0]
	}
	return ""
}
