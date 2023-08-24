package importer

import (
	"context"
	"strings"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/uniquekey"
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

func (ou *ObjectIDGetter) Get(
	ctx context.Context,
	spaceID string,
	sn *converter.Snapshot,
	sbType sb.SmartBlockType,
	createdTime time.Time,
	getExisting bool,
	oldToNewIDs map[string]string,
) (string, treestorage.TreeStorageCreatePayload, error) {
	if sbType == sb.SmartBlockTypeWorkspace {
		workspaceID, wErr := ou.core.GetWorkspaceIdForObject(spaceID, sn.Id)
		if wErr == nil {
			return workspaceID, treestorage.TreeStorageCreatePayload{}, nil
		}
	}
	if sbType == sb.SmartBlockTypeWidget {
		widgetID := ou.core.PredefinedObjects(spaceID).Widgets
		return widgetID, treestorage.TreeStorageCreatePayload{}, nil
	}

	id, err := ou.getObjectByOldAnytypeID(sn, sbType)
	if id != "" {
		return id, treestorage.TreeStorageCreatePayload{}, err
	}

	if getExisting || sbType == sb.SmartBlockTypeProfilePage {
		id = ou.getExistingObject(sn)
		if id != "" {
			return id, treestorage.TreeStorageCreatePayload{}, nil
		}
	}

	var payload treestorage.TreeStorageCreatePayload
	if sbType == sb.SmartBlockTypeRelation || sbType == sb.SmartBlockTypeObjectType {
		id := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyId.String())
		uk, err := uniquekey.UnmarshalFromString(id)
		if err != nil {
			return "", treestorage.TreeStorageCreatePayload{}, err
		}
		payload, err = ou.service.DeriveTreeCreatePayload(context.Background(), spaceID, uk)
	} else {
		payload, err = ou.service.CreateTreePayload(context.Background(), spaceID, sbType, createdTime)
	}
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

func (ou *ObjectIDGetter) getIDBySourceObject(sn *converter.Snapshot) string {
	so := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeySourceObject.String())
	if strings.HasPrefix(so, addr.BundledObjectTypeURLPrefix) ||
		strings.HasPrefix(so, addr.BundledRelationURLPrefix) {
		return sn.Id
	}
	return ""
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
	records, _, err := ou.objectStore.Query(database.Query{
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
