package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/collection"
	_import "github.com/anytypeio/go-anytype-middleware/core/block/editor/import"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
)

func NewDashboard(importServices _import.Services, dmservice DetailsModifier) *Dashboard {
	sb := smartblock.New()
	return &Dashboard{
		SmartBlock:      sb,
		Basic:           basic.NewBasic(sb), // deprecated
		Import:          _import.NewImport(sb, importServices),
		Collection:      collection.NewCollection(sb),
		DetailsModifier: dmservice,
	}
}

type Dashboard struct {
	smartblock.SmartBlock
	basic.Basic
	_import.Import
	collection.Collection
	DetailsModifier DetailsModifier
}

func (p *Dashboard) Init(ctx *smartblock.InitContext) (err error) {
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	p.DisableLayouts()
	return p.init(ctx.State)
}

func (p *Dashboard) init(s *state.State) (err error) {
	state.CleanupLayouts(s)
	p.AddHook(p.updateObjects, smartblock.HookAfterApply)
	if err = smartblock.ApplyTemplate(p, s,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyDashboard.URL()}),
		template.WithEmpty,
		template.WithDetailName("Home"),
		template.WithDetailIconEmoji("🏠"),
		template.WithNoRootLink(p.Anytype().PredefinedBlocks().Archive),
		template.WithRootLink(p.Anytype().PredefinedBlocks().SetPages, model.BlockContentLink_Dataview),
		template.WithRequiredRelations(),
		template.WithNoDuplicateLinks(),
	); err != nil {
		return
	}

	log.Infof("create default structure for dashboard: %v", s.RootId())
	return
}

func (p *Dashboard) updateObjects() {
	favoritedIds, err := p.GetIds()
	if err != nil {
		log.Errorf("archive: can't get archived ids: %v", err)
		return
	}

	records, _, err := p.ObjectStore().Query(nil, database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyIsFavorite.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(true),
			},
		},
	})
	if err != nil {
		log.Errorf("favorite: can't get store favorited ids: %v", err)
		return
	}
	var storeFavoritedIds = make([]string, 0, len(records))
	for _, rec := range records {
		storeFavoritedIds = append(storeFavoritedIds, pbtypes.GetString(rec.Details, bundle.RelationKeyId.String()))
	}

	removedIds, addedIds := slice.DifferenceRemovedAdded(storeFavoritedIds, favoritedIds)
	for _, removedId := range removedIds {
		go func(id string) {
			if err := p.DetailsModifier.ModifyLocalDetails(id, func(current *types.Struct) (*types.Struct, error) {
				if current == nil || current.Fields == nil {
					current = &types.Struct{
						Fields: map[string]*types.Value{},
					}
				}
				current.Fields[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(false)
				return current, nil
			}); err != nil {
				log.Errorf("favorite: can't set detail to object: %v", err)
			}
		}(removedId)
	}
	for _, addedId := range addedIds {
		go func(id string) {
			if err := p.DetailsModifier.ModifyLocalDetails(id, func(current *types.Struct) (*types.Struct, error) {
				if current == nil || current.Fields == nil {
					current = &types.Struct{
						Fields: map[string]*types.Value{},
					}
				}
				current.Fields[bundle.RelationKeyIsFavorite.String()] = pbtypes.Bool(true)
				return current, nil
			}); err != nil {
				log.Errorf("favorite: can't set detail to object: %v", err)
			}
		}(addedId)
	}
}
