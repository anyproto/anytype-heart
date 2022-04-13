package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
)

func NewArchive(service DetailsModifier) *Archive {
	sb := smartblock.New()
	return &Archive{
		SmartBlock:      sb,
		Collection:      collection.NewCollection(sb),
		DetailsModifier: service,
	}
}

type DetailsModifier interface {
	ModifyDetails(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error)
	ModifyLocalDetails(objectId string, modifier func(current *types.Struct) (*types.Struct, error)) (err error)
}

type Archive struct {
	smartblock.SmartBlock
	collection.Collection
	DetailsModifier DetailsModifier
}

func (p *Archive) Init(ctx *smartblock.InitContext) (err error) {
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}
	p.SmartBlock.DisableLayouts()
	p.AddHook(p.updateObjects, smartblock.HookAfterApply)
	return smartblock.ObjectApplyTemplate(p, ctx.State, template.WithEmpty, template.WithNoDuplicateLinks(), template.WithNoObjectTypes(), template.WithDetailName("Archive"), template.WithDetailIconEmoji("🗑"))
}

func (p *Archive) Relations() []*model.Relation {
	return nil
}

func (p *Archive) updateObjects() {
	archivedIds, err := p.GetIds()
	if err != nil {
		log.Errorf("archive: can't get archived ids: %v", err)
		return
	}

	records, _, err := p.ObjectStore().Query(nil, database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyIsArchived.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(true),
			},
		},
	})
	if err != nil {
		log.Errorf("archive: can't get store archived ids: %v", err)
		return
	}
	var storeArchivedIds = make([]string, 0, len(records))
	for _, rec := range records {
		storeArchivedIds = append(storeArchivedIds, pbtypes.GetString(rec.Details, bundle.RelationKeyId.String()))
	}

	removedIds, addedIds := slice.DifferenceRemovedAdded(storeArchivedIds, archivedIds)
	for _, removedId := range removedIds {
		go func(id string) {
			if err := p.DetailsModifier.ModifyLocalDetails(id, func(current *types.Struct) (*types.Struct, error) {
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
			if err := p.DetailsModifier.ModifyLocalDetails(id, func(current *types.Struct) (*types.Struct, error) {
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
}
