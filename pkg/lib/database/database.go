package database

import (
	"fmt"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-datastore/query"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("anytype-database")

const RecordIDField = "id"

type Record struct {
	Details *types.Struct
}

func (r Record) Get(key string) *types.Value {
	return pbtypes.Get(r.Details, key)
}

type Query struct {
	FullText string
	Filters  []*model.BlockContentDataviewFilter // filters results. apply sequentially
	Sorts    []*model.BlockContentDataviewSort   // order results. apply hierarchically
	Limit    int                                 // maximum number of results
	Offset   int                                 // skip given number of results
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

		if filter.RelationKey == bundle.RelationKeyType.String() {
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
		filters = append(filters, &model.BlockContentDataviewFilter{RelationKey: bundle.RelationKeyType.String(), Condition: model.BlockContentDataviewFilter_NotIn, Value: pbtypes.Float64(float64(model.ObjectType_space))})
	}
	return filters
}

func NewFilters(qry Query, store ObjectStore) (filters *Filters, err error) {
	qry.Filters = injectDefaultFilters(qry.Filters)
	filters = new(Filters)

	filterObj, err := compose(qry.Filters, store)
	if err != nil {
		return
	}

	filters.FilterObj = filterObj
	filters.Order = extractOrder(qry.Sorts, store)
	return
}

func compose(
	filters []*model.BlockContentDataviewFilter,
	store ObjectStore,
) (AndFilters, error) {
	var filterObj AndFilters
	qryFilter, err := MakeAndFilter(filters, store)
	if err != nil {
		return nil, err
	}

	if len(qryFilter) > 0 {
		filterObj = append(filterObj, qryFilter)
	}
	return filterObj, nil
}

func extractOrder(sorts []*model.BlockContentDataviewSort, store ObjectStore) SetOrder {
	if len(sorts) > 0 {
		order := SetOrder{}
		for _, sort := range sorts {

			keyOrder := &KeyOrder{
				Key:            sort.RelationKey,
				Type:           sort.Type,
				EmptyLast:      sort.RelationKey == bundle.RelationKeyName.String(),
				IncludeTime:    isIncludeTime(sorts, sort),
				RelationFormat: sort.Format,
				Store:          store,
			}

			order = appendCustomOrder(sort, order, keyOrder)
		}
		return order
	}
	return nil
}

func appendCustomOrder(sort *model.BlockContentDataviewSort, order SetOrder, keyOrder *KeyOrder) SetOrder {
	if sort.Type == model.BlockContentDataviewSort_Custom && len(sort.CustomOrder) > 0 {
		order = append(order, NewCustomOrder(sort.RelationKey, sort.CustomOrder, *keyOrder))
	} else {
		order = append(order, keyOrder)
	}
	return order
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

type filterGetter struct {
	dateKeys []string
	curEl    *types.Struct
}

func (f filterGetter) Get(key string) *types.Value {
	res := pbtypes.Get(f.curEl, key)
	if res != nil && lo.Contains(f.dateKeys, key) {
		res = dateOnly(res)
	}
	return res
}

type sortGetter struct {
	curEl *types.Struct
}

func (f sortGetter) Get(key string) *types.Value {
	return pbtypes.Get(f.curEl, key)
}

type Filters struct {
	FilterObj Filter
	Order     Order
	dateKeys  []string
}

func (f *Filters) Filter(e query.Entry) bool {
	g := f.unmarshalFilter(e)
	if g == nil {
		return false
	}
	res := f.FilterObj.FilterObject(g)
	return res
}

func (f *Filters) Compare(a, b query.Entry) int {
	if f.Order == nil {
		return 0
	}
	ag := f.unmarshalSort(a)
	if ag == nil {
		return 0
	}
	bg := f.unmarshalSort(b)
	if bg == nil {
		return 0
	}
	return f.Order.Compare(ag, bg)
}

func (f *Filters) unmarshalFilter(e query.Entry) Getter {
	return filterGetter{dateKeys: f.dateKeys, curEl: f.unmarshal(e)}
}

func (f *Filters) unmarshalSort(e query.Entry) Getter {
	return sortGetter{curEl: f.unmarshal(e)}
}

func (f *Filters) unmarshal(e query.Entry) *types.Struct {
	var details model.ObjectDetails
	err := proto.Unmarshal(e.Value, &details)
	if err != nil {
		log.Errorf("query filters decode error: %s", err.Error())
		return nil
	}
	return details.Details
}

func (f *Filters) hasOrders() bool {
	return f.Order != nil
}

func (f *Filters) String() string {
	var filterString string
	var orderString string
	var separator string
	if f.FilterObj != nil {
		filterString = fmt.Sprintf("WHERE %v", f.FilterObj.String())
		separator = " "
	}
	if f.Order != nil {
		orderString = fmt.Sprintf("%sORDER BY %v", separator, f.Order.String())
	}
	return fmt.Sprintf("%s%s", filterString, orderString)
}

func dateOnly(v *types.Value) *types.Value {
	if n, isNumber := v.GetKind().(*types.Value_NumberValue); isNumber {
		tm := time.Unix(int64(n.NumberValue), 0).In(time.UTC)                 // we have all values stored in UTC, including filters
		tm = time.Date(tm.Year(), tm.Month(), tm.Day(), 0, 0, 0, 0, time.UTC) // reset time, preserving UTC tz
		return pbtypes.Float64(float64(tm.Unix()))
	}
	// reset to NULL otherwise
	return &types.Value{Kind: &types.Value_NullValue{}}
}

func getRelationByKey(relations []*model.BlockContentDataviewRelation, key string) *model.BlockContentDataviewRelation {
	for _, relation := range relations {
		if relation.Key == key {
			return relation
		}
	}
	return nil
}
