package template

import (
	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func MakeCollectionDataviewContent() *model.BlockContentOfDataview {
	relations := []*model.RelationLink{
		{
			Format: model.RelationFormat_shorttext,
			Key:    bundle.RelationKeyName.String(),
		},
	}
	viewRelations := []*model.BlockContentDataviewRelation{
		{
			Key:       bundle.RelationKeyName.String(),
			IsVisible: true,
		},
	}
	for _, relKey := range DefaultDataviewRelations {
		if pbtypes.HasRelationLink(relations, relKey.String()) {
			continue
		}
		rel := bundle.MustGetRelation(relKey)
		relations = append(relations, &model.RelationLink{
			Format: rel.Format,
			Key:    rel.Key,
		})
		viewRelations = append(viewRelations, &model.BlockContentDataviewRelation{Key: rel.Key, IsVisible: false})
	}

	blockContent := &model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			IsCollection:  true,
			RelationLinks: relations,
			Views: []*model.BlockContentDataviewView{
				{
					Id:   bson.NewObjectId().Hex(),
					Type: model.BlockContentDataviewView_Table,
					Name: "All",
					Sorts: []*model.BlockContentDataviewSort{
						{
							RelationKey: "name",
							Type:        model.BlockContentDataviewSort_Asc,
						},
					},
					Filters:   nil,
					Relations: viewRelations,
				},
			},
		},
	}
	return blockContent
}

var DefaultDataviewRelations = make([]domain.RelationKey, 0, len(bundle.RequiredInternalRelations))

func init() {
	// fill DefaultDataviewRelations
	// deprecated: we should remove this after we merge relations as objects
	for _, rel := range bundle.RequiredInternalRelations {
		if bundle.MustGetRelation(rel).Hidden {
			continue
		}
		DefaultDataviewRelations = append(DefaultDataviewRelations, rel)
	}
	DefaultDataviewRelations = append(DefaultDataviewRelations, bundle.RelationKeyTag)
}

const CollectionStoreKey = "objects"
