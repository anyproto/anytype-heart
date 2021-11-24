package editor

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	dataview "github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
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

func getDefaultViewRelations(rels []*model.Relation) []*model.BlockContentDataviewRelation {
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
						Relations: getDefaultViewRelations(rels),
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
	if err = smartblock.ApplyTemplate(p, ctx.State, templates...); err != nil {
		return
	}
	p.applyRestrictions(ctx.State)
	return p.FillAggregatedOptions(nil)
}

func (p *Set) InitDataview(blockContent *model.BlockContentOfDataview, name, icon string) error {
	s := p.NewState()

	tmpls := []template.StateTransformer{
		template.WithForcedDetail(bundle.RelationKeyName, pbtypes.String(name)),
		template.WithForcedDetail(bundle.RelationKeyIconEmoji, pbtypes.String(icon)),
		template.WithRequiredRelations(),
		template.WithMaxCountMigration,
	}
	if blockContent != nil {
		for i, view := range blockContent.Dataview.Views {
			if view.Relations == nil {
				blockContent.Dataview.Views[i].Relations = getDefaultViewRelations(blockContent.Dataview.Relations)
			}
		}
		tmpls = append(tmpls,
			template.WithForcedDetail(bundle.RelationKeySetOf, pbtypes.StringList(blockContent.Dataview.Source)),
			template.WithDataview(*blockContent, false),
		)
	}

	if err := smartblock.ApplyTemplate(p, s, tmpls...); err != nil {
		return err
	}
	p.applyRestrictions(s)
	return nil
}

func (p *Set) applyRestrictions(s *state.State) {
	var restrictedSources = []string{
		bundle.TypeKeyFile.URL(),
		bundle.TypeKeyImage.URL(),
		bundle.TypeKeyVideo.URL(),
		bundle.TypeKeyAudio.URL(),
		bundle.TypeKeyObjectType.URL(),
		bundle.TypeKeySet.URL(),
		bundle.TypeKeyRelation.URL(),
	}
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if dv := b.Model().GetDataview(); dv != nil && len(dv.Source) == 1 {
			if slice.FindPos(restrictedSources, dv.Source[0]) != -1 {
				br := model.RestrictionsDataviewRestrictions{
					BlockId: b.Model().Id,
					Restrictions: []model.RestrictionsDataviewRestriction{
						model.Restrictions_DVRelation, model.Restrictions_DVCreateObject,
					},
				}
				r := p.Restrictions().Copy()
				var found bool
				for i, dr := range r.Dataview {
					if dr.BlockId == br.BlockId {
						r.Dataview[i].Restrictions = br.Restrictions
						found = true
						break
					}
				}
				if !found {
					r.Dataview = append(r.Dataview, br)
				}
				p.SetRestrictions(r)
			}
		}
		return true
	})
}
