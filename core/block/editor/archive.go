package editor

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

type DetailsModifier interface {
	ModifyDetails(ctx session.Context, objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error)
	ModifyLocalDetails(ctx session.Context, objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error)
}

type Archive struct {
	smartblock.SmartBlock
	collection.Collection
	DetailsModifier DetailsModifier
	objectStore     objectstore.ObjectStore
}

func NewArchive(
	sb smartblock.SmartBlock,
	detailsModifier DetailsModifier,
	objectStore objectstore.ObjectStore,
) *Archive {
	return &Archive{
		SmartBlock:      sb,
		Collection:      collection.NewCollection(sb),
		DetailsModifier: detailsModifier,
		objectStore:     objectStore,
	}
}

func (p *Archive) Init(ctx *smartblock.InitContext) (err error) {
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	p.DisableLayouts()
	p.AddHook(p.updateObjects(ctx.Ctx), smartblock.HookAfterApply)

	return p.updateObjects(ctx.Ctx)(smartblock.ApplyInfo{})
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

func (p *Archive) updateObjects(ctx session.Context) func(info smartblock.ApplyInfo) (err error) {
	return func(info smartblock.ApplyInfo) (err error) {
		archivedIds, err := p.GetIds()
		if err != nil {
			return
		}

		records, _, err := p.objectStore.Query(nil, database.Query{
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyIsArchived.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.Bool(true),
				},
			},
		})
		if err != nil {
			return
		}
		var storeArchivedIds = make([]string, 0, len(records))
		for _, rec := range records {
			storeArchivedIds = append(storeArchivedIds, pbtypes.GetString(rec.Details, bundle.RelationKeyId.String()))
		}

		removedIds, addedIds := slice.DifferenceRemovedAdded(storeArchivedIds, archivedIds)
		for _, removedId := range removedIds {
			go func(id string) {
				if err := p.DetailsModifier.ModifyLocalDetails(ctx, id, func(current *types.Struct) (*types.Struct, error) {
					if current == nil || current.Fields == nil {
						current = &types.Struct{
							Fields: map[string]*types.Value{},
						}
					}
					current.Fields[bundle.RelationKeyIsArchived.String()] = pbtypes.Bool(false)
					return current, nil
				}); err != nil {
					log.Errorf("archive: can't set detail to object: %v", err)
				}
			}(removedId)
		}
		for _, addedId := range addedIds {
			go func(id string) {
				if err := p.DetailsModifier.ModifyLocalDetails(ctx, id, func(current *types.Struct) (*types.Struct, error) {
					if current == nil || current.Fields == nil {
						current = &types.Struct{
							Fields: map[string]*types.Value{},
						}
					}
					current.Fields[bundle.RelationKeyIsArchived.String()] = pbtypes.Bool(true)
					return current, nil
				}); err != nil {
					log.Errorf("archive: can't set detail to object: %v", err)
				}
			}(addedId)
		}
		return
	}
}
