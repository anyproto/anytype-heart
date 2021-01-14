package editor

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	dataview "github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	sDataview "github.com/anytypeio/go-anytype-middleware/core/block/simple/dataview"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/stext"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/google/uuid"
)

var ErrAlreadyHasDataviewBlock = fmt.Errorf("already has the dataview block")

func NewSet(
	ms meta.Service,
	dbCtrl database.Ctrl,
) *Set {
	sb := &Set{
		SmartBlock: smartblock.New(ms, objects.BundledObjectTypeURLPrefix+"set"),
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

	templates := []template.StateTransformer{template.WithTitle, template.WithObjectTypes([]string{p.DefaultObjectTypeUrl()})}
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
							{Key: "createdDate", IsVisible: true}},
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

	st.Iterate(func(b simple.Block) (isContinue bool) {
		if dvBlock, ok := b.(sDataview.Block); !ok {
			return true
		} else {
			dvc := dvBlock.Model().GetDataview()

			for _, rel := range dvc.Relations {
				if rel.Format != pbrelation.RelationFormat_status && rel.Format != pbrelation.RelationFormat_tag {
					continue
				}

				objScopeOptions, restScopeOptions, err := p.Anytype().ObjectStore().GetAggregatedOptionsForRelation(rel.Key, dvc.Source)
				if err != nil {
					log.Errorf("failed to GetAggregatedOptionsForRelation %s", err.Error())
					continue
				}

				/*formatScopeOptions, err := d.Anytype().ObjectStore().GetAggregatedOptionsForFormat(rel.Key, d.Id())
				if err != nil {
					log.Errorf("failed to GetAggregatedOptionsForRelation %s", err.Error())
					continue
				}*/

				dvc.AggregatedOptions = append(dvc.AggregatedOptions,
					&model.BlockContentDataviewAggregatedOptions{
						RelationKey: rel.Key,
						Local:       objScopeOptions,
						ByRelation:  restScopeOptions,
					})
			}
			st.Set(b)
		}
		return true
	})
	err = p.Apply(st)
	if err != nil {
		log.Errorf("failed to apply state: %s", err.Error())
	}
	return
}

func (p *Set) InitDataview(blockContent model.BlockContentOfDataview, name string, icon string) error {
	s := p.NewState()
	return template.ApplyTemplate(p, s, template.WithDetailName(name), template.WithDetailIconEmoji(icon), template.WithDataview(blockContent))
}
