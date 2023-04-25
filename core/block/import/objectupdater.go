package importer

import (
	"fmt"

	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	sb "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/syncer"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type ObjectUpdater struct {
	service         *block.Service
	objectStore     objectstore.ObjectStore
	syncFactory     *syncer.Factory
	relationCreator RelationCreator
}

func NewObjectUpdater(service *block.Service,
	objectStore objectstore.ObjectStore,
	syncFactory *syncer.Factory,
	relationCreator RelationCreator) Updater {
	return &ObjectUpdater{
		service:         service,
		objectStore:     objectStore,
		syncFactory:     syncFactory,
		relationCreator: relationCreator,
	}
}

func (ou *ObjectUpdater) Update(ctx *session.Context,
	snapshot *model.SmartBlockSnapshotBase,
	relations []*converter.Relation,
	pageID string) (*types.Struct, []string, error) {
	if snapshot.Details != nil && snapshot.Details.Fields[bundle.RelationKeySource.String()] != nil {
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
				filesToDelete, err := ou.update(ctx, snapshot, records[0].Details, relations, pageID)
				return records[0].Details, filesToDelete, err
			}
		}
	}
	if snapshot.Details != nil && snapshot.Details.Fields[bundle.RelationKeyId.String()] != nil {
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
				filesToDelete, err := ou.update(ctx, snapshot, records[0].Details, relations, pageID)
				return records[0].Details, filesToDelete, err
			}
		}
	}
	return nil, nil, fmt.Errorf("no source or id details")
}

func (ou *ObjectUpdater) update(ctx *session.Context,
	snapshot *model.SmartBlockSnapshotBase,
	details *types.Struct,
	relations []*converter.Relation,
	pageID string) ([]string, error) {
	simpleBlocks := make([]simple.Block, 0)
	var (
		filesToDelete         = make([]string, 0)
		oldRelationBlockToNew = make(map[string]*model.Block, 0)
	)
	id := details.Fields[bundle.RelationKeyId.String()].GetStringValue()
	allBlocksIds := make([]string, 0)
	if err := ou.service.Do(id, func(b sb.SmartBlock) error {
		s := b.NewStateCtx(ctx)
		if err := b.Iterate(func(b simple.Block) (isContinue bool) {
			if b.Model().GetLink() == nil && id != b.Model().Id {
				allBlocksIds = append(allBlocksIds, b.Model().Id)
			}
			return true
		}); err != nil {
			return err
		}
		for _, v := range allBlocksIds {
			s.Unlink(v)
		}
		for _, block := range snapshot.Blocks {
			if block.GetLink() != nil {
				// we don't add link to non-existing object,so checking existence of the object with TargetBlockId in Do
				if err := ou.service.Do(block.GetLink().TargetBlockId, func(b sb.SmartBlock) error {
					return nil
				}); err != nil {
					continue
				}
			}
			if block.Id != pageID {
				simpleBlocks = append(simpleBlocks, simple.New(block))
			}
		}
		if err := basic.NewBasic(b).PasteBlocks(s, "", model.Block_Bottom, simpleBlocks); err != nil {
			return err
		}
		return b.Apply(s)
	}); err != nil {
		return nil, err
	}
	var err error
	filesToDelete, oldRelationBlockToNew, err = ou.relationCreator.CreateRelations(ctx, snapshot, id, relations)
	if err != nil {
		return nil, err
	}
	ou.relationCreator.ReplaceRelationBlock(ctx, oldRelationBlockToNew, id)
	for _, b := range simpleBlocks {
		s := ou.syncFactory.GetSyncer(b)
		if s != nil {
			if err := s.Sync(ctx, id, b); err != nil {
				log.With(zap.String("object id", pageID)).Errorf("failed to sync %s", err.Error())
			}
		}
	}
	return filesToDelete, nil
}
