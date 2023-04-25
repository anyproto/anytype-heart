package importer

import (
	"context"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	sb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type ObjectIDGetter struct {
	core    core.Service
	service *block.Service
}

func NewObjectIDGetter(core core.Service, service *block.Service) IDGetter {
	return &ObjectIDGetter{
		core:    core,
		service: service,
	}
}

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

func (ou *ObjectIDGetter) Get(ctx *session.Context,
	sn *converter.Snapshot,
	sbType sb.SmartBlockType,
	getExisting bool) (string, bool, error) {
	if sbType == sb.SmartBlockTypeSubObject {
		return ou.getSubObjectID(sn, sbType)
	}

	if getExisting {
		id, exist := ou.getExisting(sn)
		if id != "" {
			return id, exist, nil
		}
	}
	cctx := context.Background()

	sb, release, err := ou.service.CreateTreeObject(cctx, sbType, func(id string) *smartblock.InitContext {
		return &smartblock.InitContext{
			Ctx: cctx,
		}
	})
	if err != nil {
		return "", false, err
	}
	release()
	return sb.Id(), false, nil
}

func (ou *ObjectIDGetter) getSubObjectID(sn *converter.Snapshot, sbType sb.SmartBlockType) (string, bool, error) {
	id := sn.Id
	ids, _, err := ou.core.ObjectStore().QueryObjectIds(database.Query{
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
	}
	return id, false, nil
}

func (ou *ObjectIDGetter) getExisting(sn *converter.Snapshot) (string, bool) {
	source := pbtypes.GetString(sn.Snapshot.Details, bundle.RelationKeySource.String())
	records, _, err := ou.core.ObjectStore().Query(nil, database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				Condition:   model.BlockContentDataviewFilter_Equal,
				RelationKey: bundle.RelationKeySource.String(),
				Value:       pbtypes.String(source),
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
	id := sn.Id
	records, _, err = ou.core.ObjectStore().Query(nil, database.Query{
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
