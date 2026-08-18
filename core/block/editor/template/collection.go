package template

import (
	"slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	CollectionStoreKey = "objects"
	DefaultViewLayout  = model.BlockContentDataviewView_Table
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
	// What a FRESH view shows as columns: the source's own relations, whether
	// they arrived as an object type or as a relation list. Only existing
	// views (oldContent) keep commonVisibleRelations as their seed, because
	// there the user's own column selection decides.
	sourceRelations := make([]domain.RelationKey, 0, len(relLinks))
	for _, relLink := range relLinks {
		sourceRelations = append(sourceRelations, domain.RelationKey(relLink.Key))
	}

	if oldContent == nil {
		visibleRelations := slices.Concat(defaultVisibleRelations, sourceRelations)
		view := &model.BlockContentDataviewView{
			Id:        "default",
			Type:      DefaultViewLayout,
			Name:      defaultViewName,
			Sorts:     buildSorts(isCollection, ot, nil),
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
		view.Sorts = buildSorts(isCollection, ot, view.Sorts)
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

func buildSorts(isCollection bool, ot *model.ObjectType, oldSorts []*model.BlockContentDataviewSort) []*model.BlockContentDataviewSort {
	// Special case for the chat type
	if ot != nil && (ot.Key == bundle.TypeKeyChatDerived.String() || ot.Key == bundle.TypeKeyDiscussion.String()) {
		return defaultChatSort()
	}

	if oldSorts != nil {
		return oldSorts
	}

	if isCollection {
		return defaultNameSort()
	}
	return DefaultLastModifiedDateSort()
}

func DefaultLastModifiedDateSort() []*model.BlockContentDataviewSort {
	return []*model.BlockContentDataviewSort{
		{
			Id:          "byLastModifiedDate",
			RelationKey: bundle.RelationKeyLastModifiedDate.String(),
			Type:        model.BlockContentDataviewSort_Desc,
		},
	}
}

func defaultNameSort() []*model.BlockContentDataviewSort {
	return []*model.BlockContentDataviewSort{
		{
			Id:          "byName",
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
			Id:          "byLastMessageDate",
		},
	}
}

func DefaultCollectionRelations() []domain.RelationKey {
	return defaultCollectionRelations
}

// ReconcileTypeDataviewColumns brings a type's own dataview back in line with
// the type: every property the type recommends is a column, and where nobody
// has arranged the view those columns are switched on.
//
// It runs on open and after an import rather than once, because the view can
// fall behind the type in two ways. Views built before columns were made
// visible list the type's properties and hide every one, so the type opens as
// a bare Name column. And a view built while a property's relation object was
// still being written — which an import's concurrent workers do — misses that
// property entirely.
//
// The two repairs read "arranged" differently, because the evidence differs:
//
//   - A property already in the view but hidden is only switched on when the
//     view shows nothing except Name and Type. Anything else visible means
//     someone chose these columns, and a hidden property is then their choice
//     too.
//   - A property missing from the view was never anyone's choice, so it is
//     added switched on unless the view carries columns we would not have put
//     there — a housekeeping relation like Created date being visible.
//
// Nothing is ever removed or hidden. Reports whether anything changed.
func ReconcileTypeDataviewColumns(dv *model.BlockContentDataview, relLinks []*model.RelationLink) bool {
	if dv == nil {
		return false
	}
	typeProperties := make(map[string]struct{}, len(relLinks))
	for _, link := range relLinks {
		typeProperties[link.Key] = struct{}{}
	}
	isDefault := func(key string) bool {
		return slices.Contains(defaultDataviewRelations, domain.RelationKey(key)) ||
			slices.Contains(defaultVisibleRelations, domain.RelationKey(key))
	}
	changed := false
	for _, view := range dv.Views {
		var showsOnlyDefaults, showsOnlyOurs = true, true
		present := make(map[string]struct{}, len(view.Relations))
		for _, rel := range view.Relations {
			present[rel.Key] = struct{}{}
			if !rel.IsVisible || slices.Contains(defaultVisibleRelations, domain.RelationKey(rel.Key)) {
				continue
			}
			showsOnlyDefaults = false
			if _, ours := typeProperties[rel.Key]; !ours {
				showsOnlyOurs = false
			}
		}
		for _, rel := range view.Relations {
			if _, ours := typeProperties[rel.Key]; !ours || rel.IsVisible {
				continue
			}
			if showsOnlyDefaults && !isDefault(rel.Key) {
				rel.IsVisible = true
				changed = true
			}
		}
		for _, link := range relLinks {
			if _, ok := present[link.Key]; ok {
				continue
			}
			present[link.Key] = struct{}{}
			view.Relations = append(view.Relations, &model.BlockContentDataviewRelation{
				Key:       link.Key,
				IsVisible: showsOnlyOurs && !isDefault(link.Key),
				Width:     propertyWidth(link.Format),
			})
			changed = true
		}
	}
	if !changed {
		return false
	}
	// The links carry the formats the view relations do not, and a custom
	// relation missing from them cannot be looked up from the bundle.
	linked := make(map[string]struct{}, len(dv.RelationLinks))
	for _, link := range dv.RelationLinks {
		linked[link.Key] = struct{}{}
	}
	for _, link := range relLinks {
		if _, ok := linked[link.Key]; ok {
			continue
		}
		linked[link.Key] = struct{}{}
		dv.RelationLinks = append(dv.RelationLinks, link)
	}
	return true
}
