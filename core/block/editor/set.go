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
							{Key: "id", IsVisible: false},
							{Key: "name", IsVisible: true},
							{Key: "lastOpenedDate", IsVisible: true},
							{Key: "lastModifiedDate", IsVisible: true},
							{Key: "creator", IsVisible: true}},
						Filters: nil,
					},
				},
			},
		}

		templates = append(templates, template.WithDataview(dataview), template.WithDetailName("Pages"), template.WithDetailIconEmoji("📒"))
	}

	st := p.NewState()
	if err = template.ApplyTemplate(p, st, templates...); err != nil {
		return
	}

	err = p.Apply(st)
	if err != nil {
		log.Errorf("failed to apply state: %s", err.Error())
	}

	return p.FillAggregatedOptions(nil)
}

func (p *Set) InitDataview(blockContent model.BlockContentOfDataview, name string, icon string) error {
	s := p.NewState()
	return template.ApplyTemplate(p, s,
		template.WithDetailName(name),
		template.WithDetailIconEmoji(icon),
		template.WithDataview(blockContent),
	)
}
