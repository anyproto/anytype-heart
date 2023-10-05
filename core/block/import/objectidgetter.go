package importer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/core/domain"
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
	sbt := domain.TypeKey(c.subObjectType).String()
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
	service       payloadcreator.PayloadCreator
}

func NewObjectIDGetter(objectStore objectstore.ObjectStore, core core.Service, service payloadcreator.PayloadCreator) IDGetter {
	return &ObjectIDGetter{
		objectStore: objectStore,
		service:     service,
		core:        core,
	}
}

func (ou *ObjectIDGetter) Get(
	spaceID string,
	sn *converter.Snapshot,
	createdTime time.Time,
	getExisting bool,
) (string, treestorage.TreeStorageCreatePayload, error) {
	sbType := sn.SbType
	if sbType == sb.SmartBlockTypeWorkspace {
		workspaceID := ou.core.PredefinedObjects(spaceID).Workspace
		return workspaceID, treestorage.TreeStorageCreatePayload{}, nil
	}
	if sbType == sb.SmartBlockTypeWidget {
		widgetID := ou.core.PredefinedObjects(spaceID).Widgets
		return widgetID, treestorage.TreeStorageCreatePayload{}, nil
	}

	id, err := ou.getObjectByOldAnytypeID(spaceID, sn, sbType)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("get object by old anytype id: %w", err)
	}
	if id != "" {
		return id, treestorage.TreeStorageCreatePayload{}, nil
	}

	if getExisting || sbType == sb.SmartBlockTypeProfilePage {
		id = ou.getExistingObject(spaceID, sn)
		if id != "" {
			return id, treestorage.TreeStorageCreatePayload{}, nil
		}
	}

	var payload treestorage.TreeStorageCreatePayload
	if sbType == sb.SmartBlockTypeRelation || sbType == sb.SmartBlockTypeObjectType {
		id := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyId.String())
		uk, err := domain.UnmarshalUniqueKey(id)
		if err != nil {
			return "", treestorage.TreeStorageCreatePayload{}, err
		}
		payload, err = ou.service.DeriveTreePayload(context.Background(), spaceID, payloadcreator.PayloadDerivationParams{
			Key: uk,
		})
		if err != nil {
			return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("derive tree create payload: %w", err)
		}
	} else {
		payload, err = ou.service.CreateTreePayload(context.Background(), spaceID, payloadcreator.PayloadCreationParams{
			Time:           createdTime,
			SmartblockType: sbType,
		})
		if err != nil {
			return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("create tree payload: %w", err)
		}
	}

	return payload.RootRawChange.Id, payload, nil
}

func (ou *ObjectIDGetter) getObjectByOldAnytypeID(spaceID string, sn *converter.Snapshot, sbType sb.SmartBlockType) (string, error) {
	oldAnytypeID := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeyOldAnytypeID.String())

	// Check for imported objects
	ids, _, err := ou.objectStore.QueryObjectIDs(database.Query{
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
	}, []sb.SmartBlockType{sbType})
	if err == nil && len(ids) > 0 {
		return ids[0], nil
	}

	// Check for derived objects
	ids, _, err = ou.objectStore.QueryObjectIDs(database.Query{
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

func (ou *ObjectIDGetter) getExistingObject(spaceID string, sn *converter.Snapshot) string {
	source := pbtypes.GetString(sn.Snapshot.Data.Details, bundle.RelationKeySourceFilePath.String())
	ids, _, err := ou.objectStore.QueryObjectIDs(database.Query{
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
	if err == nil && len(records) > 0 {
		return records[0].Details.Fields[bundle.RelationKeyId.String()].GetStringValue()
	}
	return ""
}
