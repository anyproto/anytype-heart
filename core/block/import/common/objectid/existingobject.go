package objectid

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
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
	relationOption := e.getExistingRelationOption(sn)
	return relationOption, treestorage.TreeStorageCreatePayload{}, nil
}

func (e *existingObject) getObjectByOldAnytypeID(spaceID string, sn *common.Snapshot) (string, error) {
	oldAnytypeID := sn.Snapshot.Data.Details.GetString(bundle.RelationKeyOldAnytypeID)

	// Check for imported objects
	ids, _, err := e.objectStore.QueryObjectIDs(database.Query{
		Filters: []database.FilterRequest{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyOldAnytypeID.String(),
				Value:       domain.String(oldAnytypeID),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySpaceId.String(),
				Value:       domain.String(spaceID),
			},
		},
	})
	if err == nil && len(ids) > 0 {
		return ids[0], nil
	}

	// Check for derived objects
	ids, _, err = e.objectStore.QueryObjectIDs(database.Query{
		Filters: []database.FilterRequest{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Value:       domain.String(oldAnytypeID), // Old id equals to unique key
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySpaceId.String(),
				Value:       domain.String(spaceID),
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
	ids, _, err := e.objectStore.QueryObjectIDs(database.Query{
		Filters: []database.FilterRequest{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySourceFilePath.String(),
				Value:       domain.String(source),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySpaceId.String(),
				Value:       domain.String(spaceID),
			},
		},
	})
	if err == nil && len(ids) > 0 {
		return ids[0]
	}
	return ""
}

func (e *existingObject) getExistingRelationOption(snapshot *common.Snapshot) string {
	name := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyName)
	key := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyRelationKey)
	ids, _, err := e.objectStore.QueryObjectIDs(database.Query{
		Filters: []database.FilterRequest{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyName.String(),
				Value:       domain.String(name),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyRelationKey.String(),
				Value:       domain.String(key),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyType.String(),
				Value:       domain.String(bundle.TypeKeyRelationOption.URL()),
			},
		},
	})
	if err == nil && len(ids) > 0 {
		return ids[0]
	}
	return ""
}
