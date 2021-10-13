package editor

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/doc"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func NewWorkspace(m meta.Service, dbCtrl database.Ctrl) *Workspaces {
	return &Workspaces{
		Set: NewSet(m, dbCtrl),
	}
}

type Workspaces struct {
	*Set
}

func (p *Workspaces) Init(ctx *smartblock.InitContext) (err error) {
	if ctx.Source.Type() != model.SmartBlockType_Workspace {
		return fmt.Errorf("source type should be a workspace")
	}

	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}

	dataview := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source:    []string{addr.BundledRelationURLPrefix + bundle.RelationKeyName.String()},
			Relations: []*model.Relation{bundle.MustGetRelation(bundle.RelationKeyName)},
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
					Relations: []*model.BlockContentDataviewRelation{{
						Key:       bundle.RelationKeyName.String(),
						IsVisible: true,
					}},
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

	err = template.ApplyTemplate(p, ctx.State,
		template.WithEmpty,
		template.WithTitle,
		template.WithDataview(dataview, true),
	)
	if err != nil {
		return err
	}

	defaultValue := &types.Struct{Fields: map[string]*types.Value{bundle.RelationKeyWorkspaceId.String(): pbtypes.String(p.Id())}}
	return p.Set.SetNewRecordDefaultFields("dataview", defaultValue)
}

func (v *Workspaces) GetDocInfo() (doc.DocInfo, error) {
	info, err := v.Set.GetDocInfo()
	if err != nil {
		return doc.DocInfo{}, err
	}
	injectedDetails := make(map[string]*types.Struct)

	workspaceCollection := info.State.GetCollection(source.WorkspaceCollection)
	if workspaceCollection != nil {
		for objId, workspaceId := range workspaceCollection {
			if injectedDetails[objId] == nil {
				injectedDetails[objId] = &types.Struct{Fields: map[string]*types.Value{}}
			}
			injectedDetails[objId].Fields[bundle.RelationKeyWorkspaceId.String()] = pbtypes.String(workspaceId.(string))
		}
	}

	highlightedCollection := info.State.GetCollection(source.HighlightedCollection)
	if highlightedCollection != nil {
		for objId, isHighlighted := range highlightedCollection {
			if injectedDetails[objId] == nil {
				injectedDetails[objId] = &types.Struct{Fields: map[string]*types.Value{}}
			}
			injectedDetails[objId].Fields[bundle.RelationKeyIsHighlighted.String()] = pbtypes.Bool(isHighlighted.(bool))
		}
	}
	info.InjectedDetails = injectedDetails

	return info, nil
}
