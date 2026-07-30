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
	if sn.Snapshot.SbType == sb.SmartBlockTypeObjectType {
		return e.getExistingObjectType(sn, spaceID), treestorage.TreeStorageCreatePayload{}, nil
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
				RelationKey: bundle.RelationKeyResolvedLayout,
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
	records, err := e.objectStore.SpaceIndex(spaceID).QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
		database.FilterEq{
			Key:   bundle.RelationKeyRelationFormat,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: snapshot.Snapshot.Data.Details.Get(bundle.RelationKeyRelationFormat),
		},
		database.FilterEq{
			Key:   bundle.RelationKeyResolvedLayout,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: domain.Int64(model.ObjectType_relation),
		},
		database.FiltersOr{
			database.FilterEq{
				Key:   bundle.RelationKeyName,
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: snapshot.Snapshot.Data.Details.Get(bundle.RelationKeyName),
			},
			database.FilterEq{
				Key:   bundle.RelationKeyRelationKey,
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: snapshot.Snapshot.Data.Details.Get(bundle.RelationKeyRelationKey),
			},
		},
	}}, 1, 0)
	if err == nil && len(records) > 0 {
		return records[0].Details.GetString(bundle.RelationKeyId)
	}
	return ""
}

func (e *existingObject) getExistingObjectType(snapshot *common.Snapshot, spaceID string) string {
	identity, ok := objectTypeIdentityFilter(snapshot)
	if !ok {
		return ""
	}

	records, err := e.objectStore.SpaceIndex(spaceID).QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
		database.FilterEq{
			Key:   bundle.RelationKeyResolvedLayout,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: domain.Int64(model.ObjectType_objectType),
		},
		identity,
	}}, 1, 0)
	if err == nil && len(records) > 0 {
		return records[0].Details.GetString(bundle.RelationKeyId)
	}

	return ""
}

// objectTypeIdentityFilter tells which existing type the snapshot should be merged into.
//
// A unique key identifies a type exactly, so when the snapshot carries one we match on it alone.
// Matching such a snapshot by name as well would merge two unrelated types that only share a
// title — an imported ot-pmProject named "Project" into the bundled ot-project, for instance —
// and the merge silently discards the imported dataview, description and recommended relations.
// Name is only used for legacy exports, which predate unique keys and offer nothing else to
// match on.
func objectTypeIdentityFilter(snapshot *common.Snapshot) (database.Filter, bool) {
	if uniqueKey := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyUniqueKey); uniqueKey != "" {
		return database.FilterEq{
			Key:   bundle.RelationKeyUniqueKey,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: domain.String(uniqueKey),
		}, true
	}
	if name := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyName); name != "" {
		return database.FilterEq{
			Key:   bundle.RelationKeyName,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: domain.String(name),
		}, true
	}
	return nil, false
}
