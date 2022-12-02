package editor

import (
	dataview2 "github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/restriction"
	"github.com/anytypeio/go-anytype-middleware/core/relation/relationutils"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
	"strings"
)

type ObjectType struct {
	*Set
}

func NewObjectType() *ObjectType {
	return &ObjectType{
		Set: NewSet(),
	}
}

func (o *ObjectType) SetStruct(st *types.Struct) error {
	o.Lock()
	defer o.Unlock()
	s := o.NewState()
	s.SetDetails(st)
	return o.Apply(s)
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
	var templatesSource string
	var isBundled bool
	if strings.HasPrefix(p.Id(), addr.BundledObjectTypeURLPrefix) {
		isBundled = true
	}

	if isBundled {
		templatesSource = bundle.TypeKeyTemplate.BundledURL()
	} else {
		templatesSource = bundle.TypeKeyTemplate.URL()
	}

	templatesDataview := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source: []string{templatesSource},
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
	var recommendedRelationsKeys []string
	for _, relId := range pbtypes.GetStringList(ctx.State.Details(), bundle.RelationKeyRecommendedRelations.String()) {
		relKey, err := pbtypes.RelationIdToKey(relId)
		if err != nil {
			log.Errorf("recommendedRelations of %s has incorrect id: %s", p.Id(), relId)
			continue
		}
		if slice.FindPos(recommendedRelationsKeys, relKey) == -1 {
			recommendedRelationsKeys = append(recommendedRelationsKeys, relKey)
		}
	}

	// todo: remove this
	/*
		for _, rel := range bundle.RequiredInternalRelations {
			if slice.FindPos(recommendedRelationsKeys, rel.String()) == -1 {
				recommendedRelationsKeys = append(recommendedRelationsKeys, rel.String())
			}
		}*/

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

	// filter out internal relations from the recommended
	recommendedRelationsKeys = slice.Filter(recommendedRelationsKeys, func(relKey string) bool {
		for _, k := range bundle.RequiredInternalRelations {
			if k.String() == relKey {
				return false
			}
		}
		return true
	})

	var relIds []string
	var r *relationutils.Relation
	for _, rel := range recommendedRelationsKeys {
		if isBundled {
			relIds = append(relIds, addr.BundledRelationURLPrefix+rel)
		} else {
			relIds = append(relIds, addr.RelationKeyToIdPrefix+rel)
		}

		if r2, _ := bundle.GetRelation(bundle.RelationKey(rel)); r2 != nil {
			if r2.Hidden {
				continue
			}
			r = &relationutils.Relation{Relation: r2}
		} else {
			r, _ = p.RelationService().FetchKey(rel)
			if r == nil {
				continue
			}
		}
		// add recommended relation to the dataview
		dataview.Dataview.RelationLinks = append(dataview.Dataview.RelationLinks, r.RelationLink())
		dataview.Dataview.Views[0].Relations = append(dataview.Dataview.Views[0].Relations, &model.BlockContentDataviewRelation{
			Key:       r.Key,
			IsVisible: true,
		})
	}

	defaultValue := &types.Struct{Fields: map[string]*types.Value{bundle.RelationKeyTargetObjectType.String(): pbtypes.String(p.RootId())}}

	if !isBundled {
		var system bool
		for _, o := range bundle.SystemTypes {
			if addr.ObjectTypeKeyToIdPrefix+o.String() == p.RootId() {
				system = true
				break
			}
		}
		if system {
			rest := p.Restrictions()
			obj := append(rest.Object.Copy(), []model.RestrictionsObjectRestriction{model.Restrictions_Delete, model.Restrictions_Relations, model.Restrictions_Blocks, model.Restrictions_Details}...)
			p.SetRestrictions(restriction.Restrictions{Object: obj, Dataview: rest.Dataview})
		}
	}
	return smartblock.ObjectApplyTemplate(p, ctx.State,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyObjectType.URL()}, model.ObjectType_objectType),
		template.WithEmpty,
		template.WithTitle,
		template.WithDefaultFeaturedRelations,
		template.WithDescription,
		template.WithFeaturedRelations,
		template.WithDataviewID("templates", templatesDataview, true),
		template.WithDataview(dataview, true),
		template.WithForcedDetail(bundle.RelationKeyRecommendedRelations, pbtypes.StringList(relIds)),
		template.MigrateRelationValue(bundle.RelationKeySource, bundle.RelationKeySourceObject),
		template.WithChildrenSorter(p.RootId(), func(blockIds []string) {
			i := slice.FindPos(blockIds, "templates")
			j := slice.FindPos(blockIds, template.DataviewBlockId)
			// templates dataview must come before the type dataview
			if i > j {
				blockIds[i], blockIds[j] = blockIds[j], blockIds[i]
			}
		}),
		template.WithCondition(!isBundled, template.WithAddedFeaturedRelations(bundle.RelationKeySourceObject)),
		template.WithObjectTypeLayoutMigration(),
		template.WithRequiredRelations(),
		template.WithBlockField("templates", dataview2.DefaultDetailsFieldName, pbtypes.Struct(defaultValue)),
	)
}
