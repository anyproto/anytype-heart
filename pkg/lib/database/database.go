package database

import (
	"github.com/gogo/protobuf/types"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("anytype-database")

const (
	RecordIDField    = "id"
	RecordScoreField = "_score"
)

type Record struct {
	Details *types.Struct
	Meta    model.SearchMeta
}

type Query struct {
	FullText    string
	Highlighter ftsearch.HighlightFormatter         // default is json
	Filters     []*model.BlockContentDataviewFilter // filters results. apply sequentially
	Sorts       []*model.BlockContentDataviewSort   // order results. apply hierarchically
	Limit       int                                 // maximum number of results
	Offset      int                                 // skip given number of results
}

func injectDefaultFilters(filters []*model.BlockContentDataviewFilter) []*model.BlockContentDataviewFilter {
	var (
		hasArchivedFilter bool
		hasDeletedFilter  bool
		hasTypeFilter     bool
	)

	for _, filter := range filters {
		// include archived objects if we have explicit filter about it
		if filter.RelationKey == bundle.RelationKeyIsArchived.String() {
			hasArchivedFilter = true
		}

		if filter.RelationKey == bundle.RelationKeyLayout.String() {
			hasTypeFilter = true
		}

		if filter.RelationKey == bundle.RelationKeyIsDeleted.String() {
			hasDeletedFilter = true
		}
	}

	if !hasArchivedFilter {
		filters = append(filters, &model.BlockContentDataviewFilter{RelationKey: bundle.RelationKeyIsArchived.String(), Condition: model.BlockContentDataviewFilter_NotEqual, Value: pbtypes.Bool(true)})
	}
	if !hasDeletedFilter {
		filters = append(filters, &model.BlockContentDataviewFilter{RelationKey: bundle.RelationKeyIsDeleted.String(), Condition: model.BlockContentDataviewFilter_NotEqual, Value: pbtypes.Bool(true)})
	}
	if !hasTypeFilter {
		// temporarily exclude Space objects from search if we don't have explicit type filter
		filters = append(filters, &model.BlockContentDataviewFilter{RelationKey: bundle.RelationKeyLayout.String(), Condition: model.BlockContentDataviewFilter_NotEqual, Value: pbtypes.Float64(float64(model.ObjectType_space))})
	}
	return filters
}

func injectDefaultOrder(qry Query, sorts []*model.BlockContentDataviewSort) []*model.BlockContentDataviewSort {
	var (
		hasScoreSort bool
	)
	if qry.FullText == "" {
		return sorts
	}

	for _, sort := range sorts {
		// include archived objects if we have explicit filter about it
		if sort.RelationKey == RecordScoreField {
			hasScoreSort = true
		}
	}

	if !hasScoreSort {
		sorts = append([]*model.BlockContentDataviewSort{{RelationKey: RecordScoreField, Type: model.BlockContentDataviewSort_Desc}}, sorts...)
	}

	return sorts
}

func NewFilters(qry Query, store ObjectStore, arena *fastjson.Arena) (filters *Filters, err error) {
	// spaceID could be empty
	spaceID := getSpaceIDFromFilters(qry.Filters)
	qry.Filters = injectDefaultFilters(qry.Filters)
	qry.Sorts = injectDefaultOrder(qry, qry.Sorts)
	filters = new(Filters)

	qb := queryBuilder{
		spaceId:     spaceID,
		arena:       arena,
		objectStore: store,
	}

	filterObj, err := MakeFiltersAnd(qry.Filters, store)
	if err != nil {
		return
	}

	filters.FilterObj = filterObj
	filters.Order = qb.extractOrder(qry.Sorts)
	return
}

type queryBuilder struct {
	spaceId     string
	arena       *fastjson.Arena
	objectStore ObjectStore
}

func getSpaceIDFromFilters(filters []*model.BlockContentDataviewFilter) string {
	for _, f := range filters {
		if f.RelationKey == bundle.RelationKeySpaceId.String() {
			return f.Value.GetStringValue()
		}
	}
	return ""
}

func (b *queryBuilder) extractOrder(sorts []*model.BlockContentDataviewSort) SetOrder {
	if len(sorts) > 0 {
		order := SetOrder{}
		for _, sort := range sorts {
			format, err := b.objectStore.GetRelationFormatByKey(sort.RelationKey)
			if err != nil {
				format = sort.Format
			}

			keyOrder := &KeyOrder{
				SpaceID:        b.spaceId,
				Key:            sort.RelationKey,
				Type:           sort.Type,
				EmptyPlacement: sort.EmptyPlacement,
				IncludeTime:    isIncludeTime(sorts, sort),
				relationFormat: format,
				Store:          b.objectStore,
				arena:          b.arena,
			}
			order = b.appendCustomOrder(sort, order, keyOrder)
		}
		return order
	}
	return nil
}

func (b *queryBuilder) appendCustomOrder(sort *model.BlockContentDataviewSort, orders SetOrder, order *KeyOrder) SetOrder {
	defer b.arena.Reset()

	if sort.Type == model.BlockContentDataviewSort_Custom && len(sort.CustomOrder) > 0 {
		idsIndices := make(map[string]int, len(sort.CustomOrder))
		var idx int
		for _, it := range sort.CustomOrder {
			jsonVal := pbtypes.ProtoValueToJson(b.arena, it)

			raw := jsonVal.String()
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

func isIncludeTime(sorts []*model.BlockContentDataviewSort, s *model.BlockContentDataviewSort) bool {
	if isSingleDateSort(sorts) {
		return true
	} else {
		return s.IncludeTime
	}
}

func isSingleDateSort(sorts []*model.BlockContentDataviewSort) bool {
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

// ListRelationOptions returns options for specific relation
func ListRelationOptions(store ObjectStore, spaceID string, relationKey string) (options []*model.RelationOption, err error) {
	filters := []*model.BlockContentDataviewFilter{
		{
			Condition:   model.BlockContentDataviewFilter_Equal,
			RelationKey: bundle.RelationKeyRelationKey.String(),
			Value:       pbtypes.String(relationKey),
		},
		{
			Condition:   model.BlockContentDataviewFilter_Equal,
			RelationKey: bundle.RelationKeyLayout.String(),
			Value:       pbtypes.Int64(int64(model.ObjectType_relationOption)),
		},
	}
	if spaceID != "" {
		filters = append(filters, &model.BlockContentDataviewFilter{
			Condition:   model.BlockContentDataviewFilter_Equal,
			RelationKey: bundle.RelationKeySpaceId.String(),
			Value:       pbtypes.String(spaceID),
		})
	}
	records, err := store.Query(Query{
		Filters: filters,
	})

	for _, rec := range records {
		options = append(options, relationutils.OptionFromStruct(rec.Details).RelationOption)
	}
	return
}
