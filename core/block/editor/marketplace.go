package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/google/uuid"
)

func NewMarketplaceType(ms meta.Service, dbCtrl database.Ctrl) *MarketplaceType {
	return &MarketplaceType{Set: NewSet(ms, dbCtrl)}
}

type MarketplaceType struct {
	*Set
}

func (p *MarketplaceType) Init(s source.Source, allowEmpty bool, _ []string) (err error) {
	err = p.SmartBlock.Init(s, true, nil)
	if err != nil {
		return err
	}

	ot := bundle.TypeKeyObjectType.URL()
	templates := []template.StateTransformer{
		template.WithTitle,
		template.WithForcedDetail(bundle.RelationKeySetOf, pbtypes.StringList([]string{ot})),
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeySet.URL()})}
	dataview := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source:    ot,
			Relations: bundle.MustGetType(bundle.TypeKeyObjectType).Relations,
			Views: []*model.BlockContentDataviewView{
				{
					Id:    uuid.New().String(),
					Type:  model.BlockContentDataviewView_Gallery,
					Name:  "Marketplace",
					Sorts: []*model.BlockContentDataviewSort{},
					Relations: []*model.BlockContentDataviewRelation{
						{Key: bundle.RelationKeyId.String(), IsVisible: false},
						{Key: bundle.RelationKeyName.String(), IsVisible: true},
						{Key: bundle.RelationKeyDescription.String(), IsVisible: true},
						{Key: bundle.RelationKeyIconImage.String(), IsVisible: true},
						{Key: bundle.RelationKeyCreator.String(), IsVisible: true}},
					Filters: []*model.BlockContentDataviewFilter{{
						RelationKey: bundle.RelationKeyIsHidden.String(),
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       pbtypes.Bool(true),
					}},
				},
				{
					Id:    uuid.New().String(),
					Type:  model.BlockContentDataviewView_Gallery,
					Name:  "Library",
					Sorts: []*model.BlockContentDataviewSort{},
					Relations: []*model.BlockContentDataviewRelation{
						{Key: bundle.RelationKeyId.String(), IsVisible: false},
						{Key: bundle.RelationKeyName.String(), IsVisible: true},
						{Key: bundle.RelationKeyDescription.String(), IsVisible: true},
						{Key: bundle.RelationKeyIconImage.String(), IsVisible: true},
						{Key: bundle.RelationKeyCreator.String(), IsVisible: true}},
					Filters: []*model.BlockContentDataviewFilter{{
						RelationKey: bundle.RelationKeyIsHidden.String(),
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       pbtypes.Bool(true),
					}},
				},
			},
		},
	}
	templates = append(templates, template.WithDataview(dataview, true), template.WithDetailName("Types"), template.WithDetailIconEmoji("📒"))

	if err = template.ApplyTemplate(p, nil, templates...); err != nil {
		return
	}
	p.WithSystemObjects(true)
	return p.FillAggregatedOptions(nil)
}

type MarketplaceRelation struct {
	*Set
}

func NewMarketplaceRelation(ms meta.Service, dbCtrl database.Ctrl) *MarketplaceRelation {
	return &MarketplaceRelation{Set: NewSet(ms, dbCtrl)}
}

func (p *MarketplaceRelation) Init(s source.Source, allowEmpty bool, _ []string) (err error) {
	err = p.SmartBlock.Init(s, true, nil)
	if err != nil {
		return err
	}

	ot := bundle.TypeKeyRelation.URL()
	templates := []template.StateTransformer{
		template.WithTitle,
		template.WithForcedDetail(bundle.RelationKeySetOf, pbtypes.StringList([]string{ot})),
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeySet.URL()})}
	dataview := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source:    ot,
			Relations: bundle.MustGetType(bundle.TypeKeyRelation).Relations,
			Views: []*model.BlockContentDataviewView{
				{
					Id:    uuid.New().String(),
					Type:  model.BlockContentDataviewView_Gallery,
					Name:  "Marketplace",
					Sorts: []*model.BlockContentDataviewSort{},
					Relations: []*model.BlockContentDataviewRelation{
						{Key: bundle.RelationKeyId.String(), IsVisible: false},
						{Key: bundle.RelationKeyDescription.String(), IsVisible: true},
						{Key: bundle.RelationKeyIconImage.String(), IsVisible: true},
						{Key: bundle.RelationKeyCreator.String(), IsVisible: true}},
					Filters: []*model.BlockContentDataviewFilter{{
						RelationKey: bundle.RelationKeyIsHidden.String(),
						Condition:   model.BlockContentDataviewFilter_NotEqual,
						Value:       pbtypes.Bool(true),
					}},
				},
				{
					Id:    uuid.New().String(),
					Type:  model.BlockContentDataviewView_Gallery,
					Name:  "Library",
					Sorts: []*model.BlockContentDataviewSort{},
					Relations: []*model.BlockContentDataviewRelation{
						{Key: bundle.RelationKeyId.String(), IsVisible: false},
						{Key: bundle.RelationKeyDescription.String(), IsVisible: true},
						{Key: bundle.RelationKeyIconImage.String(), IsVisible: true},
						{Key: bundle.RelationKeyCreator.String(), IsVisible: true}},
					Filters: []*model.BlockContentDataviewFilter{{
						Condition: model.BlockContentDataviewFilter_NotEqual,
						Value:     pbtypes.Bool(true),
					}},
				},
			},
		},
	}
	templates = append(templates, template.WithDataview(dataview, true), template.WithDetailName("Relations"), template.WithDetailIconEmoji("📒"))

	if err = template.ApplyTemplate(p, nil, templates...); err != nil {
		return
	}

	return p.FillAggregatedOptions(nil)
}
