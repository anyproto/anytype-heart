package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/google/uuid"
)

type ObjectType struct {
	smartblock.SmartBlock
}

func NewObjectType(m meta.Service) *ObjectType {
	sb := smartblock.New(m)
	return &ObjectType{
		SmartBlock: sb,
	}
}

func (p *ObjectType) Init(s source.Source, _ bool, _ []string) (err error) {
	if err = p.SmartBlock.Init(s, true, nil); err != nil {
		return
	}

	dataview := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source: p.Id(),
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
					Relations: []*model.BlockContentDataviewRelation{},
					Filters:   nil,
				},
			},
		},
	}

	rels := p.Relations()
	recommendedRelations := pbtypes.GetStringList(p.Details(), bundle.RelationKeyRecommendedRelations.String())
	for _, rk := range recommendedRelations {
		rel := pbtypes.GetRelation(rels, rk)
		if rel == nil {
			continue
		}

		relCopy := pbtypes.CopyRelation(rel)
		dataview.Dataview.Relations = append(dataview.Dataview.Relations, relCopy)
		dataview.Dataview.Views[0].Relations = append(dataview.Dataview.Views[0].Relations, &model.BlockContentDataviewRelation{
			Key:       rel.Key,
			IsVisible: !rel.Hidden,
		})
	}

	return template.ApplyTemplate(p, nil,
		template.WithEmpty,
		template.WithDataview(dataview, true),
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyObjectType.URL()}),
		template.WithObjectTypeLayoutMigration())
}
