package database

import (
	"github.com/anyproto/any-store/anyenc"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("anytype-database")

const (
	RecordScoreField = "_score"
)

type Record struct {
	Details *domain.Details
	Meta    model.SearchMeta
}

type ObjectInfo struct {
	Id              string
	ObjectTypeUrls  []string
	Details         *domain.Details
	Relations       []*model.Relation
	Snippet         string
	HasInboundLinks bool
}

func (info *ObjectInfo) ToProto() *model.ObjectInfo {
	return &model.ObjectInfo{
		Id:              info.Id,
		ObjectTypeUrls:  info.ObjectTypeUrls,
		Details:         info.Details.ToProto(),
		Relations:       info.Relations,
		Snippet:         info.Snippet,
		HasInboundLinks: info.HasInboundLinks,
	}
}

type FilterRequest struct {
	Id               string
	Operator         model.BlockContentDataviewFilterOperator
	RelationKey      domain.RelationKey
	RelationProperty string
	Condition        model.BlockContentDataviewFilterCondition
	Value            domain.Value
	QuickOption      model.BlockContentDataviewFilterQuickOption
	Format           model.RelationFormat
	IncludeTime      bool
	NestedFilters    []FilterRequest
}

type SortRequest struct {
	RelationKey    domain.RelationKey
	Type           model.BlockContentDataviewSortType
	CustomOrder    []domain.Value
	Format         model.RelationFormat
	IncludeTime    bool
	Id             string
	EmptyPlacement model.BlockContentDataviewSortEmptyType
	NoCollate      bool
}

type Query struct {
	TextQuery       string
	SpaceId         string
	Filters         []FilterRequest // filters results. apply sequentially
	Sorts           []SortRequest   // order results. apply hierarchically
	Limit           int             // maximum number of results
	Offset          int             // skip given number of results
	PrefixNameQuery bool
}

func injectDefaultFilters(filters []FilterRequest) []FilterRequest {
	hasArchivedFilter, hasDeletedFilter, hasTypeFilter := hasDefaultFilters(filters)
	if len(filters) > 0 && len(filters[0].NestedFilters) > 0 {
		return addDefaultFiltersToNested(filters, hasArchivedFilter, hasDeletedFilter, hasTypeFilter)
	}
	return addDefaultFilters(filters, hasArchivedFilter, hasDeletedFilter, hasTypeFilter)
}

func addDefaultFiltersToNested(filters []FilterRequest, hasArchivedFilter, hasDeletedFilter, hasTypeFilter bool) []FilterRequest {
	if filters[0].Operator == model.BlockContentDataviewFilter_And {
		filters[0].NestedFilters = addDefaultFilters(filters[0].NestedFilters, hasArchivedFilter, hasDeletedFilter, hasTypeFilter)
	}
	// build And filter based on original Or filter and default filters
	if filters[0].Operator != model.BlockContentDataviewFilter_And {
		filters = addDefaultFilters(filters, hasArchivedFilter, hasDeletedFilter, hasTypeFilter)
		return []FilterRequest{
			{
				Operator:      model.BlockContentDataviewFilter_And,
				NestedFilters: filters,
			},
		}
	}
	return filters
}

func addDefaultFilters(filters []FilterRequest, hasArchivedFilter, hasDeletedFilter, hasTypeFilter bool) []FilterRequest {
	if !hasArchivedFilter {
		filters = append(filters, FilterRequest{
			RelationKey: bundle.RelationKeyIsArchived,
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.Bool(true),
		})
	}
	if !hasDeletedFilter {
		filters = append(filters, FilterRequest{
			RelationKey: bundle.RelationKeyIsDeleted,
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.Bool(true),
		})
	}
	if !hasTypeFilter {
		// temporarily exclude Space objects from search if we don't have explicit type filter
		filters = append(filters, FilterRequest{
			RelationKey: bundle.RelationKeyType,
			Condition:   model.BlockContentDataviewFilter_NotIn,
			Value:       domain.Int64(model.ObjectType_space),
		})
	}
	return filters
}

func hasDefaultFilters(filters []FilterRequest) (bool, bool, bool) {
	var (
		hasArchivedFilter bool
		hasDeletedFilter  bool
		hasTypeFilter     bool
	)
	if len(filters) == 0 {
		return false, false, false
	}
	for _, filter := range filters {
		if len(filter.NestedFilters) > 0 {
			return hasDefaultFilters(filters[0].NestedFilters)
		}
		// include archived objects if we have explicit filter about it
		if filter.RelationKey == bundle.RelationKeyIsArchived {
			hasArchivedFilter = true
		}

		if filter.RelationKey == bundle.RelationKeyResolvedLayout {
			hasTypeFilter = true
		}

		if filter.RelationKey == bundle.RelationKeyIsDeleted {
			hasDeletedFilter = true
		}
	}
	return hasArchivedFilter, hasDeletedFilter, hasTypeFilter
}

func injectDefaultOrder(qry Query, sorts []SortRequest) []SortRequest {
	var (
		hasScoreSort bool
	)
	if qry.TextQuery == "" {
		return sorts
	}

	for _, sort := range sorts {
		// include archived objects if we have explicit filter about it
		if sort.RelationKey == RecordScoreField {
			hasScoreSort = true
		}
	}

	if !hasScoreSort {
		sorts = append([]SortRequest{{RelationKey: RecordScoreField, Type: model.BlockContentDataviewSort_Desc}}, sorts...)
	}

	return sorts
}

func FiltersFromProto(filters []*model.BlockContentDataviewFilter) []FilterRequest {
	res := make([]FilterRequest, 0, len(filters))
	for _, f := range filters {
		res = append(res, FilterRequest{
			Id:               f.Id,
			Operator:         f.Operator,
			RelationKey:      domain.RelationKey(f.RelationKey),
			RelationProperty: f.RelationProperty,
			Condition:        f.Condition,
			Value:            domain.ValueFromProto(f.Value),
			QuickOption:      f.QuickOption,
			Format:           f.Format,
			IncludeTime:      f.IncludeTime,
			NestedFilters:    FiltersFromProto(f.NestedFilters),
		})
	}
	return res
}

func SortsFromProto(sorts []*model.BlockContentDataviewSort) []SortRequest {
	var res []SortRequest
	for _, s := range sorts {
		custom := make([]domain.Value, 0, len(s.CustomOrder))
		for _, item := range s.CustomOrder {
			custom = append(custom, domain.ValueFromProto(item))
		}
		res = append(res, SortRequest{
			RelationKey:    domain.RelationKey(s.RelationKey),
			Type:           s.Type,
			CustomOrder:    custom,
			Format:         s.Format,
			IncludeTime:    s.IncludeTime,
			Id:             s.Id,
			EmptyPlacement: s.EmptyPlacement,
			NoCollate:      s.NoCollate,
		})
	}
	return res
}

func NewFilters(qry Query, store ObjectStore, arena *anyenc.Arena, collatorBuffer *collate.Buffer) (filters *Filters, err error) {
	// spaceID could be empty
	qry.Filters = injectDefaultFilters(qry.Filters)
	qry.Sorts = injectDefaultOrder(qry, qry.Sorts)
	filters = new(Filters)

	qb := queryBuilder{
		spaceId:        store.SpaceId(),
		arena:          arena,
		objectStore:    store,
		collatorBuffer: collatorBuffer,
	}

	filterObj, err := MakeFilters(qry.Filters, store)
	if err != nil {
		return
	}

	filters.FilterObj = filterObj
	filters.Order = qb.extractOrder(qry.Sorts)
	return
}

type queryBuilder struct {
	spaceId        string
	arena          *anyenc.Arena
	objectStore    ObjectStore
	collatorBuffer *collate.Buffer
}

func getSpaceIDFromFilters(filters []FilterRequest) string {
	for _, f := range filters {
		if f.RelationKey == bundle.RelationKeySpaceId {
			return f.Value.String()
		}
	}
	return ""
}

func (b *queryBuilder) extractOrder(sorts []SortRequest) SetOrder {
	if len(sorts) > 0 {
		order := SetOrder{}
		for _, sort := range sorts {
			format, err := b.objectStore.GetRelationFormatByKey(sort.RelationKey)
			if err != nil {
				format = sort.Format
			}

			keyOrder := &KeyOrder{
				Key:             sort.RelationKey,
				Type:            sort.Type,
				EmptyPlacement:  sort.EmptyPlacement,
				IncludeTime:     isIncludeTime(sorts, sort),
				relationFormat:  format,
				objectStore:     b.objectStore,
				arena:           b.arena,
				collatorBuffer:  b.collatorBuffer,
				disableCollator: sort.NoCollate,
			}

			if keyOrder.Key == bundle.RelationKeyOrderId || keyOrder.Key == bundle.RelationKeySpaceOrder {
				keyOrder.disableCollator = true
			}
			order = b.appendCustomOrder(sort, order, keyOrder)
		}
		return order
	}
	return nil
}

func (b *queryBuilder) appendCustomOrder(sort SortRequest, orders SetOrder, order *KeyOrder) SetOrder {
	defer b.arena.Reset()
	if sort.Type == model.BlockContentDataviewSort_Custom && len(sort.CustomOrder) > 0 {
		idsIndices := make(map[string]int, len(sort.CustomOrder))
		var idx int
		for _, it := range sort.CustomOrder {
			val := it.ToAnyEnc(b.arena)

			raw := string(val.MarshalTo(nil))
			if raw != "" {
				idsIndices[raw] = idx
				idx++
			}
		}
		orders = append(orders, newCustomOrder(b.arena, sort.RelationKey, idsIndices, order))
	} else {
		orders = append(orders, order)
	}
	return orders
}

func isIncludeTime(sorts []SortRequest, s SortRequest) bool {
	if isSingleDateSort(sorts) {
		return true
	} else {
		return s.IncludeTime
	}
}

func isSingleDateSort(sorts []SortRequest) bool {
	return len(sorts) == 1 && sorts[0].Format == model.RelationFormat_date
}

type FulltextResult struct {
	Path            domain.ObjectPath
	Highlight       string
	HighlightRanges []*model.Range
	Score           float64
}

func (r FulltextResult) Model() model.SearchMeta {
	return model.SearchMeta{
		Highlight:       r.Highlight,
		HighlightRanges: r.HighlightRanges,
		RelationKey:     r.Path.RelationKey,
		BlockId:         r.Path.BlockId,
	}
}

// todo: rename to SearchParams?
type Filters struct {
	FilterObj Filter
	Order     Order
}
