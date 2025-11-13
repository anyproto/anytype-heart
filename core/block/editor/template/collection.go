package template

import (
	"slices"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	CollectionStoreKey = "objects"
	DefaultViewLayout  = model.BlockContentDataviewView_List
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

func MakeDataviewContent(isCollection bool, ot *model.ObjectType, relLinks []*model.RelationLink, oldContent *model.BlockContentOfDataview) *model.BlockContentOfDataview {
	commonVisibleRelations := make([]domain.RelationKey, 0, len(relLinks))
	if ot != nil {
		relLinks = ot.RelationLinks
	} else {
		for _, relLink := range relLinks {
			commonVisibleRelations = append(commonVisibleRelations, domain.RelationKey(relLink.Key))
		}
	}

	if oldContent == nil {
		visibleRelations := append(defaultVisibleRelations, commonVisibleRelations...)
		view := &model.BlockContentDataviewView{
			Id:        bson.NewObjectId().Hex(),
			Type:      DefaultViewLayout,
			Name:      defaultViewName,
			Sorts:     buildSorts(isCollection, ot),
			Filters:   nil,
			Relations: BuildViewRelations(isCollection, relLinks, visibleRelations),
		}
		return &model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				IsCollection:  isCollection,
				RelationLinks: collectRelationLinksFromViews(relLinks, view),
				Views:         []*model.BlockContentDataviewView{view},
			},
		}
	}

	for _, view := range oldContent.Dataview.Views {
		if len(view.Sorts) == 0 {
			view.Sorts = buildSorts(isCollection, ot)
		}
		visibleRelations := commonVisibleRelations
		additionalRelLinks := relLinks
		for _, rel := range view.Relations {
			if rel.IsVisible {
				visibleRelations = append(visibleRelations, domain.RelationKey(rel.Key))
				format := model.RelationFormat_longtext
				if br, err := bundle.PickRelation(domain.RelationKey(rel.Key)); err == nil {
					format = br.Format
				}
				additionalRelLinks = append(additionalRelLinks, &model.RelationLink{
					Key:    rel.Key,
					Format: format,
				})
			}
		}
		view.Relations = BuildViewRelations(isCollection, additionalRelLinks, visibleRelations)
		view.DefaultObjectTypeId = ""
		view.DefaultTemplateId = ""
	}

	return &model.BlockContentOfDataview{
		Dataview: &model.BlockContentDataview{
			IsCollection:  isCollection,
			ObjectOrders:  oldContent.Dataview.ObjectOrders,
			GroupOrders:   oldContent.Dataview.GroupOrders,
			RelationLinks: collectRelationLinksFromViews(append(oldContent.Dataview.RelationLinks, relLinks...), oldContent.Dataview.Views...),
			Views:         oldContent.Dataview.Views,
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

func BuildViewRelations(isCollection bool, additionalRelations []*model.RelationLink, visibleRelations []domain.RelationKey) (viewRelations []*model.BlockContentDataviewRelation) {
	if len(visibleRelations) == 0 {
		visibleRelations = defaultVisibleRelations
	}
	isVisible := func(key domain.RelationKey) bool {
		return slices.Contains(visibleRelations, key)
	}

	defaultRelations := defaultDataviewRelations
	if isCollection {
		defaultRelations = defaultCollectionRelations
	}

	addedRelations := make(map[string]struct{})
	for _, relKey := range defaultRelations {
		rel := bundle.MustGetRelation(relKey)
		addedRelations[rel.Key] = struct{}{}
		viewRelations = append(viewRelations, &model.BlockContentDataviewRelation{
			Key:       rel.Key,
			IsVisible: isVisible(relKey),
			Width:     propertyWidth(rel.Format),
		})
	}

	for _, relLink := range additionalRelations {
		if _, isAdded := addedRelations[relLink.Key]; isAdded {
			continue
		}
		addedRelations[relLink.Key] = struct{}{}
		viewRelations = append(viewRelations, &model.BlockContentDataviewRelation{
			Key:       relLink.Key,
			IsVisible: isVisible(domain.RelationKey(relLink.Key)),
			Width:     propertyWidth(relLink.Format),
		})
	}
	return viewRelations
}

func collectRelationLinksFromViews(existingRelLinks []*model.RelationLink, views ...*model.BlockContentDataviewView) []*model.RelationLink {
	customRelations := make(map[string]model.RelationFormat, len(existingRelLinks))
	for _, relLink := range existingRelLinks {
		if !bundle.HasRelation(domain.RelationKey(relLink.Key)) {
			customRelations[relLink.Key] = relLink.Format
		}
	}

	getRelLink := func(key string) *model.RelationLink {
		if format, isCustom := customRelations[key]; isCustom {
			return &model.RelationLink{Key: key, Format: format}
		}
		return bundle.MustGetRelationLink(domain.RelationKey(key))
	}

	addedRelations := make(map[string]struct{}, len(defaultCollectionRelations))
	relLinks := make([]*model.RelationLink, 0, len(defaultCollectionRelations))
	for _, view := range views {
		for _, rel := range view.Relations {
			if _, isAdded := addedRelations[rel.Key]; !isAdded {
				relLinks = append(relLinks, getRelLink(rel.Key))
				addedRelations[rel.Key] = struct{}{}
			}
		}
	}
	return relLinks
}

func buildSorts(isCollection bool, ot *model.ObjectType) []*model.BlockContentDataviewSort {
	if isCollection {
		return defaultNameSort()
	}
	// Special case for the chat type
	if ot != nil && ot.Key == bundle.TypeKeyChatDerived.String() {
		return defaultChatSort()
	}
	return DefaultLastModifiedDateSort()
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

func defaultChatSort() []*model.BlockContentDataviewSort {
	return []*model.BlockContentDataviewSort{
		{
			RelationKey: bundle.RelationKeyLastMessageDate.String(),
			Type:        model.BlockContentDataviewSort_Desc,
			Format:      model.RelationFormat_date,
			IncludeTime: true,
			Id:          bson.NewObjectId().Hex(),
		},
	}
}

func DefaultCollectionRelations() []domain.RelationKey {
	return defaultCollectionRelations
}
