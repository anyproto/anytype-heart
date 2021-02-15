package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/database"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/meta"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
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

	templates := []template.StateTransformer{template.WithTitle, template.WithObjectTypesAndLayout([]string{bundle.TypeKeySet.URL()})}
	dataview := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source:    "https://anytype.io/schemas/object/bundled/objectType",
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
					Filters: nil,
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
					Filters: nil,
				},
			},
		},
	}
	templates = append(templates, template.WithDataview(dataview), template.WithDetailName("Types"), template.WithDetailIconEmoji("📒"))

	if err = template.ApplyTemplate(p, nil, templates...); err != nil {
		return
	}

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

	templates := []template.StateTransformer{template.WithTitle, template.WithObjectTypesAndLayout([]string{bundle.TypeKeySet.URL()})}
	dataview := model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			Source:    "https://anytype.io/schemas/object/bundled/relation",
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
					Filters: nil,
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
					Filters: nil,
				},
			},
		},
	}
	templates = append(templates, template.WithDataview(dataview), template.WithDetailName("Relations"), template.WithDetailIconEmoji("📒"))

	if err = template.ApplyTemplate(p, nil, templates...); err != nil {
		return
	}

	return p.FillAggregatedOptions(nil)
}
