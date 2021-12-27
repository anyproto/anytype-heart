package editor

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	dataview "github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/globalsign/mgo/bson"
)

var ErrAlreadyHasDataviewBlock = fmt.Errorf("already has the dataview block")

func NewSet(dbCtrl database.Ctrl) *Set {
	sb := &Set{
		SmartBlock: smartblock.New(),
	}

	sb.Basic = basic.NewBasic(sb)
	sb.IHistory = basic.NewHistory(sb)
	sb.Dataview = dataview.NewDataview(sb)
	sb.Router = database.New(dbCtrl)
	sb.Text = stext.NewText(sb)
	return sb
}

type Set struct {
	smartblock.SmartBlock
	basic.Basic
	basic.IHistory
	dataview.Dataview
	database.Router
	stext.Text
}

func (p *Set) Init(ctx *smartblock.InitContext) (err error) {
	err = p.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}

	templates := []template.StateTransformer{
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeySet.URL()}),
		template.WithForcedDetail(bundle.RelationKeyFeaturedRelations, pbtypes.StringList([]string{bundle.RelationKeyDescription.String(), bundle.RelationKeyType.String(), bundle.RelationKeySetOf.String(), bundle.RelationKeyCreator.String()})),
		template.WithDescription,
		template.WithFeaturedRelations,
		template.WithBlockEditRestricted(p.Id()),
	}
	if p.Id() == p.Anytype().PredefinedBlocks().SetPages && p.Pick(template.DataviewBlockId) == nil {
		rels := pbtypes.MergeRelations(bundle.MustGetType(bundle.TypeKeyNote).Relations, bundle.MustGetRelations(dataview.DefaultDataviewRelations))
		dataview := model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				Source:    []string{bundle.TypeKeyNote.URL()},
				Relations: rels,
				Views: []*model.BlockContentDataviewView{
					{
						Id:   bson.NewObjectId().Hex(),
						Type: model.BlockContentDataviewView_Table,
						Name: "All notes",
						Sorts: []*model.BlockContentDataviewSort{
							{
								RelationKey: bundle.RelationKeyLastModifiedDate.String(),
								Type:        model.BlockContentDataviewSort_Desc,
							},
						},
						Relations: GetDefaultViewRelations(rels),
						Filters:   nil,
					},
				},
			},
		}

		templates = append(templates,
			template.WithDataview(dataview, false),
			template.WithDetailName("Notes"),
			template.WithDetail(bundle.RelationKeySetOf, pbtypes.StringList([]string{bundle.TypeKeyNote.URL()})),
			template.WithDetailIconEmoji("⚪"))
	} else if dvBlock := p.Pick(template.DataviewBlockId); dvBlock != nil {
		setOf := dvBlock.Model().GetDataview().Source
		templates = append(templates, template.WithForcedDetail(bundle.RelationKeySetOf, pbtypes.StringList(setOf)))
		// add missing done relation
		templates = append(templates, template.WithDataviewRequiredRelation(template.DataviewBlockId, bundle.RelationKeyDone))
	}
	templates = append(templates, template.WithTitle)
	return smartblock.ApplyTemplate(p, ctx.State, templates...)
}

func GetDefaultViewRelations(rels []*model.Relation) []*model.BlockContentDataviewRelation {
	var viewRels = make([]*model.BlockContentDataviewRelation, 0, len(rels))
	for _, rel := range rels {
		if rel.Hidden && rel.Key != bundle.RelationKeyName.String() {
			continue
		}
		var visible bool
		if rel.Key == bundle.RelationKeyName.String() {
			visible = true
		}
		viewRels = append(viewRels, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: visible})
	}
	return viewRels
}
