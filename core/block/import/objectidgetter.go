package importer

import (
	"context"
	"strings"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type CreateSubObjectRequest struct {
	subObjectType string
	details       *types.Struct
}

func (c CreateSubObjectRequest) GetDetails() *types.Struct {
	sbt := bundle.TypeKey(c.subObjectType).String()
	detailsType := &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyType.String(): pbtypes.String(sbt),
		},
	}
	return pbtypes.StructMerge(c.details, detailsType, false)
}

type ObjectIDGetter struct {
	objectStore   objectstore.ObjectStore
	core          core.Service
	createPayload map[string]treestorage.TreeStorageCreatePayload
	service       *block.Service
}

func NewObjectIDGetter(objectStore objectstore.ObjectStore, core core.Service, service *block.Service) IDGetter {
	return &ObjectIDGetter{
		objectStore: objectStore,
		service:     service,
		core:        core,
	}
}

func (ou *ObjectIDGetter) Get(ctx session.Context,
	sn *converter.Snapshot,
	sbType sb.SmartBlockType,
	createdTime time.Time,
	getExisting bool,
	oldToNewIDs map[string]string) (string, treestorage.TreeStorageCreatePayload, error) {
	if sbType == sb.SmartBlockTypeWorkspace {
		workspaceID, wErr := ou.core.GetWorkspaceIdForObject(sn.Id)
		if wErr == nil {
			return workspaceID, treestorage.TreeStorageCreatePayload{}, nil
		}
	}
	if sbType == sb.SmartBlockTypeWidget {
		widgetID := ou.core.PredefinedBlocks().Widgets
		return widgetID, treestorage.TreeStorageCreatePayload{}, nil
	}

	id, err := ou.getObjectByOldAnytypeID(sn, sbType)
	if id != "" {
		return id, treestorage.TreeStorageCreatePayload{}, err
	}
	if sbType == sb.SmartBlockTypeSubObject {
		id, err = ou.getSubObjectID(sn, oldToNewIDs)
		return id, treestorage.TreeStorageCreatePayload{}, err
	}

	if getExisting || sbType == sb.SmartBlockTypeProfilePage {
		id = ou.getExistingObject(sn)
		if id != "" {
			return id, treestorage.TreeStorageCreatePayload{}, nil
		}
	}

	cctx := context.Background()

	payload, err := ou.service.CreateTreePayload(cctx, sbType, createdTime)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, err
	}
	return payload.RootRawChange.Id, payload, nil
}

func (ou *ObjectIDGetter) getObjectByOldAnytypeID(sn *converter.Snapshot, sbType sb.SmartBlockType) (string, error) {
	oldAnytypeID := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyOldAnytypeID.String())
	ids, _, err := ou.objectStore.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyOldAnytypeID.String(),
				Value:       pbtypes.String(oldAnytypeID),
			},
		},
	}, []sb.SmartBlockType{sbType})
	if err == nil && len(ids) > 0 {
		return ids[0], nil
	}

	return "", err
}

func (ou *ObjectIDGetter) getSubObjectID(sn *converter.Snapshot, oldToNewIDs map[string]string) (string, error) {
	ids, err := ou.getAlreadyExistingSubObject(sn, oldToNewIDs)
	if err == nil && len(ids) > 0 {
		return ids[0], nil
	}

	id := ou.createSubObject(sn)
	return id, nil
}

func (ou *ObjectIDGetter) createSubObject(sn *converter.Snapshot) string {
	ot := sn.Snapshot.Data.ObjectTypes
	var (
		objects *types.Struct
		id      string
	)
	ou.cleanupSubObjectID(sn)
	so := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeySourceObject.String())
	if so != "" &&
		!bundle.HasRelation(strings.TrimPrefix(so, addr.BundledRelationURLPrefix)) &&
		!bundle.HasObjectType(strings.TrimPrefix(so, addr.BundledObjectTypeURLPrefix)) {
		// remove sourceObject in case we have removed it from the library
		delete(sn.Snapshot.Data.Details.Fields, bundle.RelationKeySourceObject.String())
	}

	req := &CreateSubObjectRequest{subObjectType: ot[0], details: sn.Snapshot.Data.Details}
	id, objects, err := ou.service.CreateObject(req, "")
	if err != nil {
		id = sn.Id
	}
	sn.Snapshot.Data.Details = pbtypes.StructMerge(sn.Snapshot.Data.Details, objects, false)
	return id
}

func (ou *ObjectIDGetter) getIDBySourceObject(sn *converter.Snapshot) string {
	so := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeySourceObject.String())
	if strings.HasPrefix(so, addr.BundledObjectTypeURLPrefix) ||
		strings.HasPrefix(so, addr.BundledRelationURLPrefix) {
		return sn.Id
	}
	return ""
}

func (ou *ObjectIDGetter) getAlreadyExistingSubObject(snapshot *converter.Snapshot, oldToNewIDs map[string]string) ([]string, error) {
	id := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyId.String())

	ids, _, err := ou.objectStore.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyId.String(),
				Value:       pbtypes.String(id),
			},
		},
	}, []sb.SmartBlockType{snapshot.SbType})
	var subObjectType string
	if len(snapshot.Snapshot.Data.GetObjectTypes()) != 0 {
		subObjectType = snapshot.Snapshot.Data.GetObjectTypes()[0]
	}
	if len(ids) == 0 && subObjectType == bundle.TypeKeyRelation.URL() {
		return ou.getExistingRelation(snapshot, ids)
	}
	if len(ids) == 0 && subObjectType == bundle.TypeKeyRelationOption.URL() {
		return ou.getExistingRelationOption(snapshot, ids, oldToNewIDs)
	}
	return ids, err
}

func (ou *ObjectIDGetter) getExistingRelation(snapshot *converter.Snapshot, ids []string) ([]string, error) {
	name := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyName.String())
	format := pbtypes.GetFloat64(snapshot.Snapshot.Data.Details, bundle.RelationKeyRelationFormat.String())
	ids, _, err := ou.objectStore.QueryObjectIDs(database.Query{
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
		},
	}, []sb.SmartBlockType{snapshot.SbType})
	return ids, err
}

func (ou *ObjectIDGetter) getExistingRelationOption(snapshot *converter.Snapshot, ids []string, oldToNewIDs map[string]string) ([]string, error) {
	name := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyName.String())
	key := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyRelationKey.String())
	relationID := addr.RelationKeyToIdPrefix + key
	if newRelationID, ok := oldToNewIDs[relationID]; ok {
		key = strings.TrimPrefix(newRelationID, addr.RelationKeyToIdPrefix)
	}
	ids, _, err := ou.objectStore.QueryObjectIDs(database.Query{
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
		},
	}, []sb.SmartBlockType{snapshot.SbType})
	return ids, err
}

func (ou *ObjectIDGetter) cleanupSubObjectID(sn *converter.Snapshot) {
	subID := sn.Snapshot.Data.Details.Fields[bundle.RelationKeyId.String()].GetStringValue()
	if subID == "" && (strings.HasPrefix(sn.Id, addr.RelationKeyToIdPrefix) || strings.HasPrefix(sn.Id, addr.ObjectTypeKeyToIdPrefix)) {
		// preserve id for the case of relation or object type
		subID = sn.Id
	}
	subID = ou.removePrefixesFromSubID(subID)
	sn.Snapshot.Data.Details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(subID)
}

func (ou *ObjectIDGetter) removePrefixesFromSubID(subID string) string {
	subID = strings.TrimPrefix(subID, addr.RelationKeyToIdPrefix)
	subID = strings.TrimPrefix(subID, addr.ObjectTypeKeyToIdPrefix)
	return subID
}

func (ou *ObjectIDGetter) getExistingObject(sn *converter.Snapshot) string {
	source := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeySourceFilePath.String())
	ids, _, err := ou.objectStore.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySourceFilePath.String(),
				Value:       pbtypes.String(source),
			},
		},
	}, []sb.SmartBlockType{sn.SbType})
	if err == nil {
		if len(ids) > 0 {
			id := ids[0]
			return id
		}
	}
	id := sn.Id
	records, _, err := ou.objectStore.Query(nil, database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyId.String(),
				Value:       pbtypes.String(id),
			},
		},
		Limit: 1,
	})
	if err == nil {
		if len(records) > 0 {
			id := records[0].Details.Fields[bundle.RelationKeyId.String()].GetStringValue()
			return id
		}
	}
	return ""
}
