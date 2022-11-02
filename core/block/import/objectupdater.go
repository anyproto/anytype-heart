package importer

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	sb "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/syncer"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

type ObjectUpdater struct {
	service     block.Service
	core        core.Service
	syncFactory *syncer.Factory
}

func NewObjectUpdater(service block.Service, core core.Service, syncFactory *syncer.Factory) Updater {
	return &ObjectUpdater{
		service:     service,
		core:        core,
		syncFactory: syncFactory,
	}
}

func (ou *ObjectUpdater) Update(ctx *session.Context, snapshot *model.SmartBlockSnapshotBase, pageID string) (*types.Struct, error) {
	if snapshot.Details != nil && snapshot.Details.Fields[bundle.RelationKeySource.String()] != nil {
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
				return records[0].Details, ou.update(ctx, snapshot, records, pageID)
			}
		}
	}
	if snapshot.Details != nil && snapshot.Details.Fields[bundle.RelationKeyId.String()] != nil {
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
				return records[0].Details, ou.update(ctx, snapshot, records, pageID)
			}
		}
	}
	return nil, fmt.Errorf("no source or id details")
}

func (ou *ObjectUpdater) update(ctx *session.Context,
	snapshot *model.SmartBlockSnapshotBase,
	records []database.Record,
	pageID string) error {
	details := records[0]
	simpleBlocks := make([]simple.Block, 0)
	id := details.Details.Fields[bundle.RelationKeyId.String()].GetStringValue()
	if details.Details != nil {
		err := ou.service.Do(id, func(b sb.SmartBlock) error {
			bs := basic.NewBasic(b)
			if err := b.Iterate(func(b simple.Block) (isContinue bool) {
				err := bs.Unlink(ctx, b.Model().Id)
				return err == nil
			}); err != nil {
				return err
			}
			for _, block := range snapshot.Blocks {
				if block.Id != pageID {
					simpleBlocks = append(simpleBlocks, simple.New(block))
				}
			}
			if err := bs.PasteBlocks(simpleBlocks, model.Block_Bottom); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}
		for _, b := range simpleBlocks {
			s := ou.syncFactory.GetSyncer(b)
			if s != nil {
				s.Sync(ctx, id, b)
			}
		}
	}
	return nil
}
