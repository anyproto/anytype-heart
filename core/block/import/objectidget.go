package importer

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
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

func (ou *ObjectIDGetter) Get(ctx *session.Context, snapshot *model.SmartBlockSnapshotBase, sbType sb.SmartBlockType, updateExisting bool) (string, bool, error) {
	if predefinedSmartBlockType(sbType) {
		ids, _, err := ou.core.ObjectStore().QueryObjectIds(database.Query{}, []sb.SmartBlockType{sbType})
		if err != nil {
			return "", false, err
		}
		if len(ids) > 0 {
			return ids[0], true, err
		}
	}

	if snapshot.GetDetails() != nil && sbType == sb.SmartBlockTypeSubObject {
		details := snapshot.GetDetails()
		if details.GetFields() != nil {
			name := pbtypes.GetString(details, bundle.RelationKeyName.String())
			ids, _, err := ou.core.ObjectStore().QueryObjectIds(database.Query{
				Filters: []*model.BlockContentDataviewFilter{
					{
						Condition:   model.BlockContentDataviewFilter_Equal,
						RelationKey: bundle.RelationKeyName.String(),
						Value:       pbtypes.String(name),
					},
				},
			}, []sb.SmartBlockType{sbType})
			if err != nil {
				return "", false, err
			}
			if len(ids) > 0 {
				return ids[0], true, err
			}
		}
	}

	if snapshot.Details != nil && snapshot.Details.Fields[bundle.RelationKeySource.String()] != nil && updateExisting {
		source := snapshot.Details.Fields[bundle.RelationKeySource.String()].GetStringValue()
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
				return id, true, nil
			}
		}
	}
	if snapshot.Details != nil && snapshot.Details.Fields[bundle.RelationKeyId.String()] != nil && updateExisting {
		source := snapshot.Details.Fields[bundle.RelationKeyId.String()]
		records, _, err := ou.core.ObjectStore().Query(nil, database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					Condition:   model.BlockContentDataviewFilter_Equal,
					RelationKey: bundle.RelationKeyId.String(),
					Value:       pbtypes.String(source.GetStringValue()),
				},
			},
			Limit: 1,
		})
		if err == nil {
			if len(records) > 0 {
				id := records[0].Details.Fields[bundle.RelationKeyId.String()].GetStringValue()
				return id, true, nil
			}
		}
	}
	cctx := context.Background()
	if predefinedSmartBlockType(sbType) {
		ctx := context.Background()
		id, err := ou.service.DeriveObject(ctx, sbType, true)
		if err != nil {
			return "", false, err
		}
		return id, false, err
	}

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

func predefinedSmartBlockType(sbType sb.SmartBlockType) bool {
	sbTypes := []sb.SmartBlockType{
		sb.SmartBlockTypeWorkspace,
		sb.SmartBlockTypeProfilePage,
		sb.SmartBlockTypeArchive,
		sb.SmartblockTypeMarketplaceType,
		sb.SmartblockTypeMarketplaceRelation,
		sb.SmartblockTypeMarketplaceTemplate,
		sb.SmartBlockTypeWidget,
		sb.SmartBlockTypeHome,
	}

	for _, blockType := range sbTypes {
		if blockType == sbType {
			return true
		}
	}

	return false
}

func bundledSmartBlockType(sbType sb.SmartBlockType) bool {
	sbTypes := []sb.SmartBlockType{
		sb.SmartBlockTypeBundledTemplate,
		sb.SmartBlockTypeBundledRelation,
		sb.SmartBlockTypeBundledObjectType,
	}

	for _, blockType := range sbTypes {
		if blockType == sbType {
			return true
		}
	}

	return false
}
