package importer

import (
	"context"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	sb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type ObjectIDGetter struct {
	objectStore objectstore.ObjectStore
	service     *block.Service
}

func NewObjectIDGetter(objectStore objectstore.ObjectStore, service *block.Service) IDGetter {
	return &ObjectIDGetter{
		objectStore: objectStore,
		service:     service,
	}
}

func (ou *ObjectIDGetter) Get(ctx *session.Context, snapshot *model.SmartBlockSnapshotBase, sbType sb.SmartBlockType, updateExisting bool) (string, bool, error) {
	if snapshot.Details != nil && snapshot.Details.Fields[bundle.RelationKeySource.String()] != nil && updateExisting {
		source := snapshot.Details.Fields[bundle.RelationKeySource.String()].GetStringValue()
		records, _, err := ou.objectStore.Query(nil, database.Query{
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
		records, _, err := ou.objectStore.Query(nil, database.Query{
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
