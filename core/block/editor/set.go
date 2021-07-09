package editor

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	dataview "github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var ErrAlreadyHasDataviewBlock = fmt.Errorf("already has the dataview block")

func NewSet(
	ms meta.Service,
	dbCtrl database.Ctrl,
) *Set {
	sb := &Set{
		SmartBlock: smartblock.New(ms),
	}

	sb.Basic = basic.NewBasic(sb)
	sb.IHistory = basic.NewHistory(sb)
	sb.Dataview = dataview.NewDataview(sb)
	sb.Router = database.New(dbCtrl)
	sb.Text = stext.NewText(sb.SmartBlock)
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
		template.WithForcedDetail(bundle.RelationKeyFeaturedRelations, pbtypes.StringList([]string{bundle.RelationKeyDescription.String(), bundle.RelationKeyType.String(), bundle.RelationKeySetOf.String(), bundle.RelationKeyCreator.String()})),
		template.WithDescription,
		template.WithFeaturedRelations,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeySet.URL()}),
	}
	if p.Id() == p.Anytype().PredefinedBlocks().SetPages {
		dataview := model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				Source:    "_otpage",
				Relations: bundle.MustGetType(bundle.TypeKeyPage).Relations,
				Views: []*model.BlockContentDataviewView{
					{
						Id:   uuid.New().String(),
						Type: model.BlockContentDataviewView_Table,
						Name: "All drafts",
						Sorts: []*model.BlockContentDataviewSort{
							{
								RelationKey: "name",
								Type:        model.BlockContentDataviewSort_Asc,
							},
						},
						Relations: getDefaultViewRelations(bundle.MustGetType(bundle.TypeKeyPage).Relations),
						Filters:   nil,
					},
				},
			},
		}
		var (
			oldName, oldIcon = "Pages", "📒"
			newName, newIcon = "Drafts", "⚪"
			forcedDataview   bool
		)
		if slice.FindPos([]string{oldName, ""}, pbtypes.GetString(p.Details(), bundle.RelationKeyName.String())) > -1 &&
			pbtypes.GetString(p.Details(), bundle.RelationKeyIconEmoji.String()) == oldIcon {
			// we should migrate existing dataview
			templates = append(templates, template.WithForcedDetail(bundle.RelationKeyName, pbtypes.String(newName)))
			templates = append(templates, template.WithForcedDetail(bundle.RelationKeyIconEmoji, pbtypes.String(newIcon)))
			forcedDataview = true
		}

		templates = append(templates,
			template.WithDataview(dataview, forcedDataview),
			template.WithDetailName(newName),
			template.WithForcedDetail(bundle.RelationKeySetOf, pbtypes.StringList([]string{"_otpage"})),
			template.WithDetailIconEmoji(newIcon))
	} else if dvBlock := p.Pick("dataview"); dvBlock != nil {
		templates = append(templates, template.WithForcedDetail(bundle.RelationKeySetOf, pbtypes.StringList([]string{dvBlock.Model().GetDataview().Source})))
	}
	templates = append(templates, template.WithTitle)
	if err = template.ApplyTemplate(p, ctx.State, templates...); err != nil {
		return
	}
	p.applyRestrictions(ctx.State)
	return p.FillAggregatedOptions(nil)
}

func (p *Set) InitDataview(blockContent model.BlockContentOfDataview, name, icon string) error {
	s := p.NewState()

	for i, view := range blockContent.Dataview.Views {
		if view.Relations == nil {
			blockContent.Dataview.Views[i].Relations = getDefaultViewRelations(blockContent.Dataview.Relations)
		}
	}

	if err := template.ApplyTemplate(p, s,
		template.WithForcedDetail(bundle.RelationKeyName, pbtypes.String(name)),
		template.WithForcedDetail(bundle.RelationKeyIconEmoji, pbtypes.String(icon)),
		template.WithForcedDetail(bundle.RelationKeySetOf, pbtypes.StringList([]string{blockContent.Dataview.Source})),
		template.WithDataview(blockContent, false),
		template.WithRequiredRelations(),
		template.WithMaxCountMigration,
	); err != nil {
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
	}
	s.Iterate(func(b simple.Block) (isContinue bool) {
		if dv := b.Model().GetDataview(); dv != nil {
			if slice.FindPos(restrictedSources, dv.Source) != -1 {
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
