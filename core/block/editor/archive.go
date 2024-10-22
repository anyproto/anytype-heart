package editor

import (
	"errors"

	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

// required relations for archive beside the bundle.RequiredInternalRelations
var archiveRequiredRelations = []domain.RelationKey{}

type Archive struct {
	smartblock.SmartBlock
	collection.Collection
	objectStore spaceindex.Store
}

func NewArchive(
	sb smartblock.SmartBlock,
	objectStore spaceindex.Store,
) *Archive {
	return &Archive{
		SmartBlock:  sb,
		Collection:  collection.NewCollection(sb, objectStore),
		objectStore: objectStore,
	}
}

func (p *Archive) Init(ctx *smartblock.InitContext) (err error) {
	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, archiveRequiredRelations...)
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	p.DisableLayouts()
	p.AddHook(p.updateObjects, smartblock.HookAfterApply)

	return p.updateObjects(smartblock.ApplyInfo{})
}

func (p *Archive) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 1,
		Proc: func(st *state.State) {
			template.InitTemplate(st,
				template.WithEmpty,
				template.WithNoDuplicateLinks(),
				template.WithNoObjectTypes(),
				template.WithDetailName("Archive"),
				template.WithDetailIconEmoji("🗑"),
			)
		},
	}
}

func (p *Archive) StateMigrations() migration.Migrations {
	return migration.MakeMigrations(nil)
}

func (p *Archive) Relations(_ *state.State) relationutils.Relations {
	return nil
}

func (p *Archive) updateObjects(_ smartblock.ApplyInfo) (err error) {
	archivedIds, err := p.GetIds()
	if err != nil {
		return
	}
	go func() {
		uErr := p.updateInStore(archivedIds)
		if uErr != nil {
			log.Errorf("archive: can't update in store: %v", uErr)
		}
	}()
	return nil
}

func (p *Archive) updateInStore(archivedIds []string) error {
	records, err := p.objectStore.QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
		database.FilterEq{
			Key:   bundle.RelationKeyIsArchived.String(),
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: pbtypes.Bool(true),
		},
	}}, 0, 0)
	if err != nil {
		return err
	}

	var storeArchivedIds = make([]string, 0, len(records))
	for _, rec := range records {
		storeArchivedIds = append(storeArchivedIds, pbtypes.GetString(rec.Details, bundle.RelationKeyId.String()))
	}

	removedIds, addedIds := slice.DifferenceRemovedAdded(storeArchivedIds, archivedIds)
	for _, removedId := range removedIds {
		go func(id string) {
			if err := p.ModifyLocalDetails(id, func(current *types.Struct) (*types.Struct, error) {
				if current == nil || current.Fields == nil {
					current = &types.Struct{
						Fields: map[string]*types.Value{},
					}
				}
				current.Fields[bundle.RelationKeyIsArchived.String()] = pbtypes.Bool(false)
				return current, nil
			}); err != nil {
				logArchiveError(err)
			}
		}(removedId)
	}
	for _, addedId := range addedIds {
		go func(id string) {
			if err := p.ModifyLocalDetails(id, func(current *types.Struct) (*types.Struct, error) {
				if current == nil || current.Fields == nil {
					current = &types.Struct{
						Fields: map[string]*types.Value{},
					}
				}
				current.Fields[bundle.RelationKeyIsArchived.String()] = pbtypes.Bool(true)
				return current, nil
			}); err != nil {
				logArchiveError(err)
			}
		}(addedId)
	}
	return nil
}

func logArchiveError(err error) {
	if errors.Is(err, spacestorage.ErrTreeStorageAlreadyDeleted) {
		return
	}
	log.Errorf("archive: can't set detail to object: %v", err)
}
