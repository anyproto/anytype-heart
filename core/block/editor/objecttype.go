package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/database"
	dataview2 "github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
)

type ObjectType struct {
	*Set
}

func NewObjectType(dbCtrl database.Ctrl) *ObjectType {
	return &ObjectType{
		Set: NewSet(dbCtrl),
	}
}

func (p *ObjectType) Init(ctx *smartblock.InitContext) (err error) {
	if err = p.SmartBlock.Init(ctx); err != nil {
		return
	}

	dataview := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source: []string{p.Id()},
			Views: []*model.BlockContentDataviewView{
				{
					Id:   "_view1_1",
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

	templatesDataview := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source: []string{bundle.TypeKeyTemplate.URL()},
			Views: []*model.BlockContentDataviewView{
				{
					Id:   "_view2_1",
					Type: model.BlockContentDataviewView_Table,
					Name: "All",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: "name",
							Type:        model.BlockContentDataviewSort_Asc,
						},
					},
					Relations: []*model.BlockContentDataviewRelation{},
					Filters: []*model.BlockContentDataviewFilter{
						{
							Operator:    model.BlockContentDataviewFilter_And,
							RelationKey: bundle.RelationKeyTargetObjectType.String(),
							Condition:   model.BlockContentDataviewFilter_Equal,
							Value:       pbtypes.String(p.RootId()),
						}},
				},
			},
		},
	}

	rels := p.RelationsState(ctx.State, false)
	var recommendedRelationsKeys []string
	for _, relId := range pbtypes.GetStringList(p.Details(), bundle.RelationKeyRecommendedRelations.String()) {
		relKey, err := pbtypes.RelationIdToKey(relId)
		if err != nil {
			log.Errorf("recommendedRelations has incorrect id: %s", relId)
			continue
		}
		if slice.FindPos(recommendedRelationsKeys, relKey) == -1 {
			recommendedRelationsKeys = append(recommendedRelationsKeys, relKey)
		}
	}

	for _, rel := range bundle.RequiredInternalRelations {
		if slice.FindPos(recommendedRelationsKeys, rel.String()) == -1 {
			recommendedRelationsKeys = append(recommendedRelationsKeys, rel.String())
		}
	}

	recommendedLayout := pbtypes.GetString(p.Details(), bundle.RelationKeyRecommendedLayout.String())
	if recommendedLayout == "" {
		recommendedLayout = model.ObjectType_basic.String()
	} else if _, ok := model.ObjectTypeLayout_value[recommendedLayout]; !ok {
		recommendedLayout = model.ObjectType_basic.String()
	}

	recommendedLayoutObj := bundle.MustGetLayout(model.ObjectTypeLayout(model.ObjectTypeLayout_value[recommendedLayout]))
	for _, rel := range recommendedLayoutObj.RequiredRelations {
		if slice.FindPos(recommendedRelationsKeys, rel.Key) == -1 {
			recommendedRelationsKeys = append(recommendedRelationsKeys, rel.Key)
		}
	}

	var recommendedRelations []*model.Relation
	for _, rk := range recommendedRelationsKeys {
		rel := pbtypes.GetRelation(rels, rk)
		if rel == nil {
			rel, _ = bundle.GetRelation(bundle.RelationKey(rk))
			if rel == nil {
				continue
			}
		}

		relCopy := pbtypes.CopyRelation(rel)
		relCopy.Scope = model.Relation_type
		recommendedRelations = append(recommendedRelations, relCopy)
		dataview.Dataview.Relations = append(dataview.Dataview.Relations, relCopy)
		dataview.Dataview.Views[0].Relations = append(dataview.Dataview.Views[0].Relations, &model.BlockContentDataviewRelation{
			Key:       rel.Key,
			IsVisible: !rel.Hidden,
		})
	}

	defaultValue := &types.Struct{Fields: map[string]*types.Value{bundle.RelationKeyTargetObjectType.String(): pbtypes.String(p.RootId())}}

	return smartblock.ApplyTemplate(p, ctx.State,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyObjectType.URL()}),
		template.WithEmpty,
		template.WithTitle,
		template.WithDefaultFeaturedRelations,
		template.WithDescription,
		template.WithFeaturedRelations,
		template.WithDataviewID("templates", templatesDataview, true),
		template.WithDataview(dataview, true),
		template.WithChildrenSorter(p.RootId(), func(blockIds []string) {
			i := slice.FindPos(blockIds, "templates")
			j := slice.FindPos(blockIds, template.DataviewBlockId)
			// templates dataview must come before the type dataview
			if i > j {
				blockIds[i], blockIds[j] = blockIds[j], blockIds[i]
			}
		}),
		template.WithObjectTypeRecommendedRelationsMigration(recommendedRelations),
		template.WithObjectTypeLayoutMigration(),
		template.WithRequiredRelations(),
		template.WithBlockField("templates", dataview2.DefaultDetailsFieldName, pbtypes.Struct(defaultValue)),
		func(s *state.State) {
			p.FillAggregatedOptionsState(s)
		},
	)
}
