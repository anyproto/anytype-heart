package template

import (
	"slices"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	CollectionStoreKey = "objects"
	defaultViewName    = "All"
	defaultWidth       = 200
	defaultWidthShort  = 100
)

var (
	defaultDataviewRelations = []domain.RelationKey{
		bundle.RelationKeyName,
		bundle.RelationKeyCreatedDate,
		bundle.RelationKeyCreator,
		bundle.RelationKeyLastModifiedDate,
		bundle.RelationKeyLastModifiedBy,
		bundle.RelationKeyLastOpenedDate,
		bundle.RelationKeyBacklinks,
	}

	defaultCollectionRelations = []domain.RelationKey{
		bundle.RelationKeyName,
		bundle.RelationKeyType,
		bundle.RelationKeyCreatedDate,
		bundle.RelationKeyCreator,
		bundle.RelationKeyLastModifiedDate,
		bundle.RelationKeyLastModifiedBy,
		bundle.RelationKeyLastOpenedDate,
		bundle.RelationKeyBacklinks,
		bundle.RelationKeyTag,
		bundle.RelationKeyDescription,
	}

	defaultVisibleRelations = []domain.RelationKey{
		bundle.RelationKeyName,
		bundle.RelationKeyType,
	}
)

func MakeDataviewContent(isCollection bool, ot *model.ObjectType, relLinks []*model.RelationLink, forceViewId string) *model.BlockContentOfDataview {
	var visibleRelations []domain.RelationKey
	var (
		defaultRelations = defaultCollectionRelations
		sorts            = DefaultLastModifiedDateSort()
	)

	if len(relLinks) == 0 {
		visibleRelations = defaultVisibleRelations
	}

	if isCollection {
		sorts = defaultNameSort()
	} else if relLinks != nil {
		for _, relLink := range relLinks {
			visibleRelations = append(visibleRelations, domain.RelationKey(relLink.Key))
		}
	} else if ot != nil {
		defaultRelations = defaultDataviewRelations
		relLinks = ot.RelationLinks
	} else {
		defaultRelations = defaultDataviewRelations
	}

	relationLinks, viewRelations := generateRelationLists(defaultRelations, relLinks, visibleRelations)
	viewId := forceViewId
	if viewId == "" {
		viewId = bson.NewObjectId().Hex()
	}
	return &model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			IsCollection:  isCollection,
			RelationLinks: relationLinks,
			Views: []*model.BlockContentDataviewView{
				{
					Id:        viewId,
					Type:      model.BlockContentDataviewView_Table,
					Name:      defaultViewName,
					Sorts:     sorts,
					Filters:   nil,
					Relations: viewRelations,
				},
			},
		},
	}
}

func propertyWidth(format model.RelationFormat) int32 {
	if slices.Contains([]model.RelationFormat{
		model.RelationFormat_number,
		model.RelationFormat_phone,
		model.RelationFormat_email,
		model.RelationFormat_tag,
		model.RelationFormat_status,
		model.RelationFormat_checkbox,
		model.RelationFormat_url,
	}, format) {
		return defaultWidthShort
	}
	return defaultWidth
}

func generateRelationLists(
	defaultRelations []domain.RelationKey,
	additionalRelations []*model.RelationLink,
	visibleRelations []domain.RelationKey,
) (
	relationLinks []*model.RelationLink,
	viewRelations []*model.BlockContentDataviewRelation,
) {
	isVisible := func(key domain.RelationKey) bool {
		return slices.Contains(visibleRelations, key)
	}

	for _, relKey := range defaultRelations {
		rel := bundle.MustGetRelation(relKey)
		relationLinks = append(relationLinks, &model.RelationLink{
			Format: rel.Format,
			Key:    rel.Key,
		})
		viewRelations = append(viewRelations, &model.BlockContentDataviewRelation{
			Key:       rel.Key,
			IsVisible: isVisible(relKey),
			Width:     propertyWidth(rel.Format),
		})
	}

	for _, relLink := range additionalRelations {
		if pbtypes.HasRelationLink(relationLinks, relLink.Key) {
			continue
		}
		relationLinks = append(relationLinks, &model.RelationLink{
			Format: relLink.Format,
			Key:    relLink.Key,
		})

		viewRelations = append(viewRelations, &model.BlockContentDataviewRelation{
			Key:       relLink.Key,
			IsVisible: isVisible(domain.RelationKey(relLink.Key)),
			Width:     propertyWidth(relLink.Format),
		})
	}
	return relationLinks, viewRelations
}

func DefaultLastModifiedDateSort() []*model.BlockContentDataviewSort {
	return []*model.BlockContentDataviewSort{
		{
			Id:          bson.NewObjectId().Hex(),
			RelationKey: bundle.RelationKeyLastModifiedDate.String(),
			Type:        model.BlockContentDataviewSort_Desc,
		},
	}
}

func defaultNameSort() []*model.BlockContentDataviewSort {
	return []*model.BlockContentDataviewSort{
		{
			Id:          bson.NewObjectId().Hex(),
			RelationKey: bundle.RelationKeyName.String(),
			Type:        model.BlockContentDataviewSort_Asc,
		},
	}
}

func DefaultCollectionRelations() []domain.RelationKey {
	return defaultCollectionRelations
}
