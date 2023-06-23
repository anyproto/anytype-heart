package database

import (
	"fmt"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-datastore/query"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
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

func (q Query) DSQuery(sch schema.Schema) (qq query.Query, err error) {
	qq.Limit = q.Limit
	qq.Offset = q.Offset
	f, err := NewFilters(q, sch, nil)
	if err != nil {
		return
	}
	qq.Filters = []query.Filter{f}
	if f.hasOrders() {
		qq.Orders = []query.Order{f}
	}
	return
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
		filters = append(filters, &model.BlockContentDataviewFilter{RelationKey: bundle.RelationKeyType.String(), Condition: model.BlockContentDataviewFilter_NotIn, Value: pbtypes.StringList([]string{bundle.TypeKeySpace.URL()})})
	}
	return filters
}

func NewFilters(qry Query, schema schema.Schema, store filter.OptionsGetter) (filters *Filters, err error) {
	qry.Filters = injectDefaultFilters(qry.Filters)
	filters = new(Filters)

	filterObj, dateKeys, qryFilters := applySchema(nil, schema, filters.dateKeys, qry.Filters)
	qry.Filters = qryFilters
	filters.dateKeys = dateKeys

	filterObj, err = compose(qry.Filters, store, filterObj)
	if err != nil {
		return
	}

	filters.FilterObj = filterObj
	filters.Order = extractOrder(qry.Sorts, store)
	return
}

func compose(
	filters []*model.BlockContentDataviewFilter,
	store filter.OptionsGetter,
	filterObj filter.AndFilters,
) (filter.AndFilters, error) {
	qryFilter, err := filter.MakeAndFilter(filters, store)
	if err != nil {
		return nil, err
	}

	if len(qryFilter) > 0 {
		filterObj = append(filterObj, qryFilter)
	}
	return filterObj, nil
}

func applySchema(
	relations []*model.BlockContentDataviewRelation,
	schema schema.Schema,
	dateKeys []string,
	filters []*model.BlockContentDataviewFilter,
) (filter.AndFilters, []string, []*model.BlockContentDataviewFilter) {
	mainFilter := filter.AndFilters{}
	if schema != nil {
		dateKeys = extractDateRelationKeys(relations, schema, dateKeys)
		filters = applyFilterDateOnlyWhenExactDate(filters, dateKeys)
		mainFilter = appendSchemaFilters(schema, mainFilter)
	}
	return mainFilter, dateKeys, filters
}

func appendSchemaFilters(schema schema.Schema, mainFilter filter.AndFilters) filter.AndFilters {
	if schemaFilter := schema.Filters(); schemaFilter != nil {
		mainFilter = append(mainFilter, schemaFilter)
	}
	return mainFilter
}

func applyFilterDateOnlyWhenExactDate(
	filters []*model.BlockContentDataviewFilter,
	dateKeys []string,
) []*model.BlockContentDataviewFilter {
	for _, filtr := range filters {
		if lo.Contains(dateKeys, filtr.RelationKey) && filtr.QuickOption == model.BlockContentDataviewFilter_ExactDate {
			filtr.Value = dateOnly(filtr.Value)
		}
	}
	return filters
}

func extractDateRelationKeys(
	relations []*model.BlockContentDataviewRelation,
	schema schema.Schema,
	dateKeys []string,
) []string {
	for _, relationLink := range schema.ListRelations() {
		if relationLink.Format == model.RelationFormat_date {
			if relation := getRelationByKey(relations, relationLink.Key); relation == nil || !relation.DateIncludeTime {
				dateKeys = append(dateKeys, relationLink.Key)
			}
		}
	}
	return dateKeys
}

func extractOrder(sorts []*model.BlockContentDataviewSort, store filter.OptionsGetter) filter.SetOrder {
	if len(sorts) > 0 {
		order := filter.SetOrder{}
		for _, sort := range sorts {

			keyOrder := &filter.KeyOrder{
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

func appendCustomOrder(sort *model.BlockContentDataviewSort, order filter.SetOrder, keyOrder *filter.KeyOrder) filter.SetOrder {
	if sort.Type == model.BlockContentDataviewSort_Custom && len(sort.CustomOrder) > 0 {
		order = append(order, filter.NewCustomOrder(sort.RelationKey, sort.CustomOrder, *keyOrder))
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
	FilterObj filter.Filter
	Order     filter.Order
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

func (f *Filters) unmarshalFilter(e query.Entry) filter.Getter {
	return filterGetter{dateKeys: f.dateKeys, curEl: f.unmarshal(e)}
}

func (f *Filters) unmarshalSort(e query.Entry) filter.Getter {
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
