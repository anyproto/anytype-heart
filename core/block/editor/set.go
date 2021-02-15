package editor

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	dataview "github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/google/uuid"
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
	sb.Dataview = dataview.NewDataview(sb, dbCtrl)
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

func (p *Set) Init(s source.Source, allowEmpty bool, _ []string) (err error) {
	err = p.SmartBlock.Init(s, true, nil)
	if err != nil {
		return err
	}

	templates := []template.StateTransformer{template.WithTitle, template.WithObjectTypesAndLayout([]string{bundle.TypeKeySet.URL()})}
	if p.Id() == p.Anytype().PredefinedBlocks().SetPages {
		dataview := model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				Source:    "https://anytype.io/schemas/object/bundled/page",
				Relations: bundle.MustGetType(bundle.TypeKeyPage).Relations,
				Views: []*model.BlockContentDataviewView{
					{
						Id:   uuid.New().String(),
						Type: model.BlockContentDataviewView_Table,
						Name: "All pages",
						Sorts: []*model.BlockContentDataviewSort{
							{
								RelationKey: "name",
								Type:        model.BlockContentDataviewSort_Asc,
							},
						},
						Relations: []*model.BlockContentDataviewRelation{
							{Key: bundle.RelationKeyId.String(), IsVisible: false},
							{Key: bundle.RelationKeyName.String(), IsVisible: true},
							{Key: bundle.RelationKeyLastOpenedDate.String(), IsVisible: true},
							{Key: bundle.RelationKeyLastModifiedDate.String(), IsVisible: true},
							{Key: bundle.RelationKeyCreator.String(), IsVisible: true}},
						Filters: nil,
					},
				},
			},
		}
		templates = append(templates, template.WithDataview(dataview), template.WithDetailName("Pages"), template.WithDetailIconEmoji("📒"))
	} else if dvBlock := p.Pick("dataview"); dvBlock != nil {
		templates = append(templates, template.WithDetail(bundle.RelationKeySetOf, pbtypes.String(dvBlock.Model().GetDataview().Source)))
	}

	if err = template.ApplyTemplate(p, nil, templates...); err != nil {
		return
	}

	return p.FillAggregatedOptions(nil)
}

func (p *Set) InitDataview(blockContent model.BlockContentOfDataview, name string, icon string) error {
	s := p.NewState()
	return template.ApplyTemplate(p, s,
		template.WithDetailName(name),
		template.WithDetail(bundle.RelationKeySetOf, pbtypes.String(blockContent.Dataview.Source)),
		template.WithDetailIconEmoji(icon),
		template.WithDataview(blockContent),
	)
}
