package objectid

import (
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type ExistingObject struct {
	objectStore objectstore.ObjectStore
}

func NewExistingObject(objectStore objectstore.ObjectStore) *ExistingObject {
	return &ExistingObject{objectStore: objectStore}
}

func (e *ExistingObject) GetID(spaceID string, sn *converter.Snapshot, _ time.Time, getExisting bool) (string, treestorage.TreeStorageCreatePayload, error) {
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
	return "", treestorage.TreeStorageCreatePayload{}, nil
}

func (e *ExistingObject) getObjectByOldAnytypeID(spaceID string, sn *converter.Snapshot) (string, error) {
	oldAnytypeID := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyOldAnytypeID.String())

	// Check for imported objects
	ids, _, err := e.objectStore.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyOldAnytypeID.String(),
				Value:       pbtypes.String(oldAnytypeID),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySpaceId.String(),
				Value:       pbtypes.String(spaceID),
			},
		},
	}, []sb.SmartBlockType{sn.SbType})
	if err == nil && len(ids) > 0 {
		return ids[0], nil
	}

	// Check for derived objects
	ids, _, err = e.objectStore.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Value:       pbtypes.String(oldAnytypeID), // Old id equals to unique key
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySpaceId.String(),
				Value:       pbtypes.String(spaceID),
			},
		},
	}, []sb.SmartBlockType{sn.SbType})
	if err == nil && len(ids) > 0 {
		return ids[0], nil
	}

	return "", err
}

func (e *ExistingObject) getExistingObject(spaceID string, sn *converter.Snapshot) string {
	source := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeySourceFilePath.String())
	ids, _, err := e.objectStore.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySourceFilePath.String(),
				Value:       pbtypes.String(source),
			},
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySpaceId.String(),
				Value:       pbtypes.String(spaceID),
			},
		},
	}, []sb.SmartBlockType{sn.SbType})
	if err == nil && len(ids) > 0 {
		return ids[0]
	}
	return ""
}
