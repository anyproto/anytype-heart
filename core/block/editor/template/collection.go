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
	// Explicitly passed relLinks are the caller's visible set — even when ot
	// is also given (an object type's own dataview passes its recommended
	// relations here so its default "All" view shows them as columns,
	// core/block/editor/objecttype.go). ot.RelationLinks is only the fallback
	// column source when the caller names no relations itself; those columns
	// stay hidden. GO-5969 inverted this precedence, which left every custom
	// column of a freshly generated type view hidden (GO-7383).
	commonVisibleRelations := make([]domain.RelationKey, 0, len(relLinks))
	if len(relLinks) > 0 {
		for _, relLink := range relLinks {
			commonVisibleRelations = append(commonVisibleRelations, domain.RelationKey(relLink.Key))
		}
	} else if ot != nil {
		relLinks = ot.RelationLinks
	}
	if oldContent == nil {
		visibleRelations := slices.Concat(defaultVisibleRelations, commonVisibleRelations)
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

// ViewColumnPrune names one view and the property keys the prune touched in
// it. Both halves of a prune plan are reported this way — the columns that
// went, and the ones that stayed — so a caller hears about each view by the
// name it is shown under rather than by its id.
type ViewColumnPrune struct {
	ViewId   string
	ViewName string
	Keys     []string
}

// TypeDataviewColumnPlan is what removing a set of properties from a type
// does to the type's own dataview: Pruned lists the views that lose the
// column, InUse the views that keep it because they group, sort or filter by
// it.
//
// A view that arranges itself by a property is doing more than showing it,
// and dropping the column out from under that arrangement would leave a
// kanban board grouped by a column nobody can see. Those views are reported
// and left exactly as they are, for their owner to change deliberately.
type TypeDataviewColumnPlan struct {
	Pruned []ViewColumnPrune
	InUse  []ViewColumnPrune
}

// Empty reports whether the plan changes nothing and has nothing to say.
func (p TypeDataviewColumnPlan) Empty() bool {
	return len(p.Pruned) == 0 && len(p.InUse) == 0
}

// PlanTypeDataviewColumnPrune computes the prune without applying it, so the
// preview a dry run serves and the edit a real run makes are the same rule
// read twice rather than two rules that agree by hand.
func PlanTypeDataviewColumnPrune(dv *model.BlockContentDataview, keys []string) TypeDataviewColumnPlan {
	var plan TypeDataviewColumnPlan
	if dv == nil || len(keys) == 0 {
		return plan
	}
	wanted := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key != "" {
			wanted[key] = struct{}{}
		}
	}
	for _, view := range dv.Views {
		if view == nil {
			continue
		}
		var going, staying []string
		for _, key := range keys {
			if key == "" || !viewHasColumn(view, key) {
				continue
			}
			if viewArrangesBy(view, key) {
				staying = append(staying, key)
				continue
			}
			going = append(going, key)
		}
		// a view that arranges itself by ANY of the removed keys is left
		// whole: pruning its other columns would still be a rewrite of a view
		// this plan has already decided not to touch
		if len(staying) > 0 {
			plan.InUse = append(plan.InUse, ViewColumnPrune{ViewId: view.Id, ViewName: view.Name, Keys: staying})
			continue
		}
		if len(going) > 0 {
			plan.Pruned = append(plan.Pruned, ViewColumnPrune{ViewId: view.Id, ViewName: view.Name, Keys: going})
		}
	}
	return plan
}

// PruneTypeDataviewColumns is the removal direction of
// ReconcileTypeDataviewColumns: a property the type no longer recommends
// stops being a column of its views. Without it a detached property keeps
// showing as a column for as long as the view lives, which is how a type
// could read as still carrying a field its definition had already dropped.
//
// Only the views PlanTypeDataviewColumnPrune clears are touched.
//
// A RelationLink no view references any more goes too. Leaving it looked free
// — it is the format cache the reconcile appends to — but it is not: the
// served document builds its `properties` array by walking RelationLinks
// (any-block codec/anyblockjson/dataview.go), so a stale link means GET still
// lists a property the type no longer recommends, which is the exact "checked
// and was reassured" reading this prune exists to end. It also feeds
// syncViewRelationsAndRelationLinks, which re-adds a link with no column as a
// hidden column — putting the pruned column back on the next edit.
//
// A link a surviving view still uses stays, so a view kept whole by the
// in-use guard keeps its formats. Reports whether anything changed.
func PruneTypeDataviewColumns(dv *model.BlockContentDataview, keys []string) bool {
	plan := PlanTypeDataviewColumnPrune(dv, keys)
	if len(plan.Pruned) == 0 {
		return false
	}
	byId := make(map[string]map[string]struct{}, len(plan.Pruned))
	for _, pruned := range plan.Pruned {
		set := make(map[string]struct{}, len(pruned.Keys))
		for _, key := range pruned.Keys {
			set[key] = struct{}{}
		}
		byId[pruned.ViewId] = set
	}
	changed := false
	for _, view := range dv.Views {
		if view == nil {
			continue
		}
		going, ok := byId[view.Id]
		if !ok {
			continue
		}
		kept := view.Relations[:0]
		for _, rel := range view.Relations {
			if rel == nil {
				continue
			}
			if _, drop := going[rel.Key]; drop {
				changed = true
				continue
			}
			kept = append(kept, rel)
		}
		view.Relations = kept
	}
	if !changed {
		return false
	}
	// drop the links nothing references now. Walk the views AFTER the prune,
	// so a key a view kept whole still counts as referenced.
	referenced := make(map[string]struct{}, len(dv.RelationLinks))
	for _, view := range dv.Views {
		if view == nil {
			continue
		}
		for _, rel := range view.Relations {
			if rel != nil {
				referenced[rel.Key] = struct{}{}
			}
		}
	}
	pruning := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		pruning[key] = struct{}{}
	}
	links := dv.RelationLinks[:0]
	for _, link := range dv.RelationLinks {
		if link == nil {
			continue
		}
		if _, asked := pruning[link.Key]; asked {
			if _, used := referenced[link.Key]; !used {
				continue
			}
		}
		links = append(links, link)
	}
	dv.RelationLinks = links
	return changed
}

func viewHasColumn(view *model.BlockContentDataviewView, key string) bool {
	for _, rel := range view.Relations {
		if rel != nil && rel.Key == key {
			return true
		}
	}
	return false
}

// viewArrangesBy reports whether the view uses the property for something
// other than showing it: the group it boards by, one of its sorts, or one of
// its filters (nested groups included).
func viewArrangesBy(view *model.BlockContentDataviewView, key string) bool {
	// grouping, covering and the calendar's date are all arrangements: the
	// view is doing something WITH the property, not merely showing it.
	// Dropping the column out from under any of them leaves the view
	// arranged by something nobody can see.
	if view.GroupRelationKey == key || view.CoverRelationKey == key || view.EndRelationKey == key {
		return true
	}
	for _, sort := range view.Sorts {
		if sort != nil && sort.RelationKey == key {
			return true
		}
	}
	return filtersUseKey(view.Filters, key)
}

func filtersUseKey(filters []*model.BlockContentDataviewFilter, key string) bool {
	for _, filter := range filters {
		if filter == nil {
			continue
		}
		if filter.RelationKey == key {
			return true
		}
		if filtersUseKey(filter.NestedFilters, key) {
			return true
		}
	}
	return false
}
