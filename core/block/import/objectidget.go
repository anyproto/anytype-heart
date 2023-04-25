package importer

import (
	"context"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
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
	objectStore objectstore.ObjectStore
	core        core.Service
	service     *block.Service
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
	getExisting bool) (string, bool, error) {
	id, err := ou.getObjectByOldAnytypeID(sn, sbType)
	if id != "" {
		return id, true, err
	}
	if sbType == sb.SmartBlockTypeSubObject {
		return ou.getSubObjectID(sn, sbType)
	}

	if sbType == sb.SmartBlockTypeWorkspace {
		workspaceID, wErr := ou.core.GetWorkspaceIdForObject(sn.Id)
		if wErr == nil {
			return workspaceID, true, nil
		}
	}

	if getExisting {
		id, exist := ou.getExisting(sn)
		if id != "" {
			return id, exist, nil
		}
	}

	cctx := context.Background()

	sb, err := ou.service.CreateTreeObject(cctx, sbType, func(id string) *smartblock.InitContext {
		return &smartblock.InitContext{
			Ctx: cctx,
		}
	})
	if err != nil {
		return "", false, err
	}
	return sb.Id(), false, nil
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
	id := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyId.String())
	ids, _, err := ou.objectStore.QueryObjectIds(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeyId.String(),
				Value:       pbtypes.String(id),
			},
		},
	}, []sb.SmartBlockType{sbType})
	if err == nil && len(ids) > 0 {
		id = ids[0]
		return id, true, nil
	}
	if len(sn.Snapshot.Data.ObjectTypes) > 0 {
		ot := sn.Snapshot.Data.ObjectTypes
		var objects *types.Struct
		ou.cleanupSubObjectID(sn)
		req := &CreateSubObjectRequest{subObjectType: ot[0], details: sn.Snapshot.Data.Details}
		id, objects, err = ou.service.CreateObject(req, "")
		if err != nil {
			id = sn.Id
		}
		sn.Snapshot.Data.Details = pbtypes.StructMerge(sn.Snapshot.Data.Details, objects, false)
	}
	return id, false, nil
}

func (ou *ObjectIDGetter) cleanupSubObjectID(sn *converter.Snapshot) {
	subID := sn.Snapshot.Data.Details.Fields[bundle.RelationKeyId.String()].GetStringValue()
	subID = strings.TrimPrefix(subID, addr.RelationKeyToIdPrefix)
	subID = strings.TrimPrefix(subID, addr.ObjectTypeKeyToIdPrefix)
	sn.Snapshot.Data.Details.Fields[bundle.RelationKeyId.String()] = pbtypes.String(subID)
}

func (ou *ObjectIDGetter) getExisting(sn *converter.Snapshot) (string, bool) {
	source := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeySource.String())
	ids, _, err := ou.objectStore.QueryObjectIds(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySource.String(),
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
