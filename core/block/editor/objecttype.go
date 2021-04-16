package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/google/uuid"
)

type ObjectType struct {
	*Set
}

func NewObjectType(m meta.Service, dbCtrl database.Ctrl) *ObjectType {
	return &ObjectType{
		Set: NewSet(m, dbCtrl),
	}
}

func (p *ObjectType) Init(ctx *smartblock.InitContext) (err error) {
	if err = p.SmartBlock.Init(ctx); err != nil {
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

	rels := p.RelationsState(ctx.State)
	recommendedRelationsKeys := pbtypes.GetStringList(p.Details(), bundle.RelationKeyRecommendedRelations.String())
	for _, rel := range bundle.RequiredInternalRelations {
		if slice.FindPos(recommendedRelationsKeys, rel.String()) == -1 {
			recommendedRelationsKeys = append(recommendedRelationsKeys, rel.String())
		}
	}

	recommendedLayout := pbtypes.GetString(p.Details(), bundle.RelationKeyRecommendedLayout.String())
	if recommendedLayout == "" {
		recommendedLayout = relation.ObjectType_basic.String()
	} else if _, ok := relation.ObjectTypeLayout_value[recommendedLayout]; !ok {
		recommendedLayout = relation.ObjectType_basic.String()
	}

	recommendedLayoutObj := bundle.MustGetLayout(relation.ObjectTypeLayout(relation.ObjectTypeLayout_value[recommendedLayout]))
	for _, rel := range recommendedLayoutObj.RequiredRelations {
		if slice.FindPos(recommendedRelationsKeys, rel.Key) == -1 {
			recommendedRelationsKeys = append(recommendedRelationsKeys, rel.Key)
		}
	}

	var recommendedRelations []*relation.Relation
	for _, rk := range recommendedRelationsKeys {
		rel := pbtypes.GetRelation(rels, rk)
		if rel == nil {
			rel, _ = bundle.GetRelation(bundle.RelationKey(rk))
			if rel == nil {
				continue
			}
		}

		relCopy := pbtypes.CopyRelation(rel)
		relCopy.Scope = relation.Relation_type
		recommendedRelations = append(recommendedRelations, relCopy)
		dataview.Dataview.Relations = append(dataview.Dataview.Relations, relCopy)
		dataview.Dataview.Views[0].Relations = append(dataview.Dataview.Views[0].Relations, &model.BlockContentDataviewRelation{
			Key:       rel.Key,
			IsVisible: !rel.Hidden,
		})
	}

	return template.ApplyTemplate(p, ctx.State,
		template.WithEmpty,
		template.WithTitle,
		//template.WithDescription,
		template.WithFeaturedRelations,
		template.WithDataview(dataview, true),
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyObjectType.URL()}),
		template.WithObjectTypeRecommendedRelationsMigration(recommendedRelations),
		template.WithObjectTypeLayoutMigration(),
		template.WithRequiredRelations(),
	)
}
