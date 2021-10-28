package editor

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	database2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func NewWorkspace(dbCtrl database.Ctrl, dmservice DetailsModifier) *Workspaces {
	return &Workspaces{
		Set:             NewSet(dbCtrl),
		DetailsModifier: dmservice,
	}
}

type Workspaces struct {
	*Set
	DetailsModifier DetailsModifier
}

func (p *Workspaces) Init(ctx *smartblock.InitContext) (err error) {
	if ctx.Source.Type() != model.SmartBlockType_Workspace && ctx.Source.Type() != model.SmartBlockType_AccountOld {
		return fmt.Errorf("source type should be a workspace or an old account")
	}

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}

	dataviewAllHighlightedObjects := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source:    []string{addr.BundledRelationURLPrefix + bundle.RelationKeyName.String()},
			Relations: []*model.Relation{bundle.MustGetRelation(bundle.RelationKeyName)},
			Views: []*model.BlockContentDataviewView{
				{
					Id:   uuid.New().String(),
					Type: model.BlockContentDataviewView_Gallery,
					Name: "Highlighted",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: "name",
							Type:        model.BlockContentDataviewSort_Asc,
						},
					},
					Relations: []*model.BlockContentDataviewRelation{
						{
							Key:       bundle.RelationKeyName.String(),
							IsVisible: true,
						},
						{
							Key:       bundle.RelationKeyCreator.String(),
							IsVisible: true,
						},
					},
					Filters: []*model.BlockContentDataviewFilter{{
						RelationKey: bundle.RelationKeyWorkspaceId.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String(p.Id()),
					}, {
						RelationKey: bundle.RelationKeyId.String(),
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       pbtypes.String(p.Id()),
					}, {
						RelationKey: bundle.RelationKeyIsHighlighted.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Bool(true),
					}},
				},
			},
		},
	}

	dataviewAllWorkspaceObjects := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source:    []string{addr.BundledRelationURLPrefix + bundle.RelationKeyName.String()},
			Relations: []*model.Relation{bundle.MustGetRelation(bundle.RelationKeyName), bundle.MustGetRelation(bundle.RelationKeyCreator)},
			Views: []*model.BlockContentDataviewView{
				{
					Id:   uuid.New().String(),
					Type: model.BlockContentDataviewView_Table,
					Name: "All",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: "name",
							Type:        model.BlockContentDataviewSort_Asc,
						},
					},
					Relations: []*model.BlockContentDataviewRelation{
						{
							Key:       bundle.RelationKeyName.String(),
							IsVisible: true,
						},
						{
							Key:       bundle.RelationKeyCreator.String(),
							IsVisible: true,
						},
					},
					Filters: []*model.BlockContentDataviewFilter{{
						RelationKey: bundle.RelationKeyWorkspaceId.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.String(p.Id()),
					}, {
						RelationKey: bundle.RelationKeyId.String(),
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       pbtypes.String(p.Id()),
					}},
				},
			},
		},
	}

	p.AddHook(p.updateObjects, smartblock.HookAfterApply)
	err = smartblock.ApplyTemplate(p, ctx.State,
		template.WithEmpty,
		template.WithTitle,
		template.WithFeaturedRelations,
		template.WithForcedDetail(bundle.RelationKeyFeaturedRelations, pbtypes.StringList([]string{bundle.RelationKeyType.String(), bundle.RelationKeyCreator.String()})),
		template.WithDataviewID("highlighted", dataviewAllHighlightedObjects, true),
		template.WithDataviewID("dataview", dataviewAllWorkspaceObjects, true),
	)
	if err != nil {
		return err
	}
	defaultValue := &types.Struct{Fields: map[string]*types.Value{bundle.RelationKeyWorkspaceId.String(): pbtypes.String(p.Id())}}
	return p.Set.SetNewRecordDefaultFields("dataview", defaultValue)
}

func (p *Workspaces) updateObjects() {
	st := p.NewState()

	records, _, err := p.ObjectStore().Query(nil, database2.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyWorkspaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(p.Id()),
			},
		},
	})
	if err != nil {
		log.Errorf("workspace: can't get store workspace ids: %v", err)
		return
	}
	var storeObjectInWorkspace = make(map[string]bool, len(records))
	for _, rec := range records {
		var isHighlighted bool
		if pbtypes.GetBool(rec.Details, bundle.RelationKeyIsHighlighted.String()) {
			isHighlighted = true
		}
		id := pbtypes.GetString(rec.Details, bundle.RelationKeyId.String())
		storeObjectInWorkspace[id] = isHighlighted
	}

	// we ignore the workspace object itself
	delete(storeObjectInWorkspace, p.Id())
	var objectInWorkspace map[string]bool
	workspaceCollection := st.GetCollection(source.WorkspaceCollection)
	if workspaceCollection != nil {
		objectInWorkspace = make(map[string]bool, len(workspaceCollection.Fields))
		for objId, workspaceId := range workspaceCollection.Fields {
			if workspaceId == nil {
				continue
			}
			if v, ok := workspaceId.Kind.(*types.Value_StringValue); ok && v.StringValue == p.Id() {
				objectInWorkspace[objId] = false
			}
		}
	}

	if objectInWorkspace == nil {
		objectInWorkspace = map[string]bool{}
	}

	highlightedCollection := st.GetCollection(threads.HighlightedCollectionName)
	if highlightedCollection != nil {
		for objId, isHighlighted := range highlightedCollection.Fields {
			if isHighlighted == nil {
				continue
			}
			if v, ok := isHighlighted.Kind.(*types.Value_BoolValue); ok && v.BoolValue {
				if _, exists := objectInWorkspace[objId]; exists {
					// only set if the object is really exist in the workspace collection
					objectInWorkspace[objId] = true
				}
			}
		}
	}
	for id, isHighlighted := range objectInWorkspace {
		if wasHighlighted, exists := storeObjectInWorkspace[id]; exists && isHighlighted == wasHighlighted {
			continue
		}

		if err := p.DetailsModifier.ModifyDetails(id, func(current *types.Struct) (*types.Struct, error) {
			if current == nil || current.Fields == nil {
				current = &types.Struct{
					Fields: map[string]*types.Value{},
				}
			}
			current.Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(p.Id())
			current.Fields[bundle.RelationKeyIsHighlighted.String()] = pbtypes.Bool(isHighlighted)

			return current, nil
		}); err != nil {
			log.Errorf("workspace: can't set detail to object: %v", err)
		}
	}

	for id, _ := range storeObjectInWorkspace {
		if _, exists := objectInWorkspace[id]; exists {
			continue
		}

		err = p.Anytype().DeleteBlock(id, p.Id())
		if err != nil {
			// we will get error here if block has been already removed
			log.Errorf("workspace: failed to remove object locally")
		}
	}

}
