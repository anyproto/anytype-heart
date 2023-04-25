package importer

import (
	"context"
	"strings"
	"time"

	"github.com/anytypeio/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	sb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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

func (ou *ObjectIDGetter) Get(ctx *session.Context,
	sn *converter.Snapshot,
	sbType sb.SmartBlockType,
	createdTime time.Time,
	getExisting bool) (string, bool, treestorage.TreeStorageCreatePayload, error) {
	if sbType == sb.SmartBlockTypeWorkspace {
		workspaceID, wErr := ou.core.GetWorkspaceIdForObject(sn.Id)
		if wErr == nil {
			return workspaceID, true, treestorage.TreeStorageCreatePayload{}, nil
		}
	}
	if sbType == sb.SmartBlockTypeWidget {
		widgetID := ou.core.PredefinedBlocks().Widgets
		return widgetID, false, treestorage.TreeStorageCreatePayload{}, nil
	}

	id, err := ou.getObjectByOldAnytypeID(sn, sbType)
	if id != "" {
		return id, true, treestorage.TreeStorageCreatePayload{}, err
	}
	if sbType == sb.SmartBlockTypeSubObject {
		id, exist, err := ou.getSubObjectID(sn, sbType)
		return id, exist, treestorage.TreeStorageCreatePayload{}, err
	}

	if getExisting || sbType == sb.SmartBlockTypeProfilePage {
		id, exist := ou.getExisting(sn)
		if id != "" {
			return id, exist, treestorage.TreeStorageCreatePayload{}, nil
		}
	}

	cctx := context.Background()

	payload, err := ou.service.CreateTreePayload(cctx, sbType, createdTime)
	if err != nil {
		return "", false, treestorage.TreeStorageCreatePayload{}, err
	}
	return payload.RootRawChange.Id, false, payload, nil
}

func (ou *ObjectIDGetter) getObjectByOldAnytypeID(sn *converter.Snapshot, sbType sb.SmartBlockType) (string, error) {
	oldAnytypeID := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyOldAnytypeID.String())
	ids, _, err := ou.objectStore.QueryObjectIds(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyOldAnytypeID.String(),
				Value:       pbtypes.String(oldAnytypeID),
			},
		},
	}, nil)
	if err == nil && len(ids) > 0 {
		return ids[0], nil
	}

	return "", err
}

func (ou *ObjectIDGetter) getSubObjectID(sn *converter.Snapshot, sbType sb.SmartBlockType) (string, bool, error) {
	// in case it already
	id := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyId.String())
	ids, err := ou.getAlreadyExistingObject(id, sbType)
	if err == nil && len(ids) > 0 {
		return ids[0], true, nil
	}

	id = ou.createSubObject(sn)
	return id, false, nil
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

func (ou *ObjectIDGetter) getAlreadyExistingObject(id string, sbType sb.SmartBlockType) ([]string, error) {
	ids, _, err := ou.objectStore.QueryObjectIds(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyId.String(),
				Value:       pbtypes.String(id),
			},
		},
	}, []sb.SmartBlockType{sbType})
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

func (ou *ObjectIDGetter) getExisting(sn *converter.Snapshot) (string, bool) {
	source := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeySourceFilePath.String())
	ids, _, err := ou.objectStore.QueryObjectIds(database.Query{
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
			return id, true
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
			return id, true
		}
	}
	return "", false
}
