package database

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/dateutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

var (
	ErrValueMustBeListSupporting = errors.New("value must be list supporting")
)

func MakeFilters(protoFilters []*model.BlockContentDataviewFilter, store ObjectStore) (Filter, error) {
	spaceId := getSpaceIDFromFilters(protoFilters)
	// to avoid unnecessary nested filter
	if len(protoFilters) == 1 && len(protoFilters[0].NestedFilters) > 0 && protoFilters[0].Operator != model.BlockContentDataviewFilter_No {
		return MakeFilter(spaceId, protoFilters[0], store)
	}
	return MakeFilter(spaceId, &model.BlockContentDataviewFilter{
		Operator:      model.BlockContentDataviewFilter_And,
		NestedFilters: protoFilters,
	}, store)
}

func MakeFilter(spaceId string, protoFilter *model.BlockContentDataviewFilter, store ObjectStore) (Filter, error) {
	if protoFilter.Operator == model.BlockContentDataviewFilter_No {
		return makeFilter(spaceId, protoFilter, store)
	}
	filters := make([]Filter, 0, len(protoFilter.NestedFilters))
	for _, nestedFilter := range protoFilter.NestedFilters {
		filter, err := MakeFilter(spaceId, nestedFilter, store)
		if err != nil {
			return nil, err
		}
		if filter != nil {
			filters = append(filters, filter)
		}
	}
	switch protoFilter.Operator {
	case model.BlockContentDataviewFilter_And, model.BlockContentDataviewFilter_No:
		return FiltersAnd(filters), nil
	case model.BlockContentDataviewFilter_Or:
		return FiltersOr(filters), nil
	}
	return nil, fmt.Errorf("unsupported filter operator %v", protoFilter.Operator)
}

func NestedRelationKey(baseRelationKey domain.RelationKey, nestedRelationKey domain.RelationKey) string {
	return fmt.Sprintf("%s.%s", baseRelationKey.String(), nestedRelationKey.String())
}

func makeFilter(spaceID string, rawFilter *model.BlockContentDataviewFilter, store ObjectStore) (Filter, error) {
	if store == nil {
		return nil, fmt.Errorf("objectStore dependency is nil")
	}
	if rawFilter.Condition == model.BlockContentDataviewFilter_None {
		return nil, nil
	}
	rawFilters := transformQuickOption(rawFilter, nil)

	if len(rawFilters) == 1 {
		return makeFilterByCondition(spaceID, rawFilters[0], store)
	}
	resultFilters := FiltersAnd{}
	for _, filter := range rawFilters {
		filterByCondition, err := makeFilterByCondition(spaceID, filter, store)
		if err != nil {
			return nil, err
		}
		resultFilters = append(resultFilters, filterByCondition)
	}
	return resultFilters, nil
}

func makeFilterByCondition(spaceID string, rawFilter *model.BlockContentDataviewFilter, store ObjectStore) (Filter, error) {
	parts := strings.SplitN(rawFilter.RelationKey, ".", 2)
	if len(parts) == 2 {
		relationKey := parts[0]
		nestedRelationKey := parts[1]

		if rawFilter.Condition == model.BlockContentDataviewFilter_NotEqual {
			return makeFilterNestedNotIn(spaceID, rawFilter, store, relationKey, nestedRelationKey)
		} else {
			return makeFilterNestedIn(spaceID, rawFilter, store, relationKey, nestedRelationKey)
		}
	}

	// replaces "value == false" to "value != true" for expected work with checkboxes
	if rawFilter.Condition == model.BlockContentDataviewFilter_Equal && rawFilter.Value != nil && rawFilter.Value.Equal(pbtypes.Bool(false)) {
		rawFilter = &model.BlockContentDataviewFilter{
			RelationKey:      rawFilter.RelationKey,
			RelationProperty: rawFilter.RelationProperty,
			Condition:        model.BlockContentDataviewFilter_NotEqual,
			Value:            pbtypes.Bool(true),
		}
	}
	// replaces "value != false" to "value == true" for expected work with checkboxes
	if rawFilter.Condition == model.BlockContentDataviewFilter_NotEqual && rawFilter.Value != nil && rawFilter.Value.Equal(pbtypes.Bool(false)) {
		rawFilter = &model.BlockContentDataviewFilter{
			RelationKey:      rawFilter.RelationKey,
			RelationProperty: rawFilter.RelationProperty,
			Condition:        model.BlockContentDataviewFilter_Equal,
			Value:            pbtypes.Bool(true),
		}
	}

	if str := rawFilter.Value.GetStructValue(); str != nil {
		filter, err := makeComplexFilter(rawFilter, str)
		if err == nil {
			return filter, nil
		}
		log.Errorf("failed to build complex filter: %v", err)
	}

	switch rawFilter.Condition {
	case model.BlockContentDataviewFilter_Equal,
		model.BlockContentDataviewFilter_Greater,
		model.BlockContentDataviewFilter_Less,
		model.BlockContentDataviewFilter_GreaterOrEqual,
		model.BlockContentDataviewFilter_LessOrEqual,
		model.BlockContentDataviewFilter_NotEqual:
		return FilterEq{
			Key:   rawFilter.RelationKey,
			Cond:  rawFilter.Condition,
			Value: rawFilter.Value,
		}, nil
	case model.BlockContentDataviewFilter_Like:
		return FilterLike{
			Key:   rawFilter.RelationKey,
			Value: rawFilter.Value,
		}, nil
	case model.BlockContentDataviewFilter_NotLike:
		return FilterNot{FilterLike{
			Key:   rawFilter.RelationKey,
			Value: rawFilter.Value,
		}}, nil
	case model.BlockContentDataviewFilter_In:
		// hack for queries for relations containing date objects ids with format _date_YYYY-MM-DD-hh-mm-ss
		// to find all date object ids of the same day we search by prefix _date_YYYY-MM-DD
		if ts, err := dateutil.ParseDateId(rawFilter.Value.GetStringValue()); err == nil {
			return FilterHasPrefix{
				Key:    rawFilter.RelationKey,
				Prefix: dateutil.TimeToDateId(ts),
			}, nil
		}
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return FilterIn{
			Key:   rawFilter.RelationKey,
			Value: list,
		}, nil
	case model.BlockContentDataviewFilter_NotIn:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return FilterNot{FilterIn{
			Key:   rawFilter.RelationKey,
			Value: list,
		}}, nil
	case model.BlockContentDataviewFilter_Empty:
		return FilterEmpty{
			Key: rawFilter.RelationKey,
		}, nil
	case model.BlockContentDataviewFilter_NotEmpty:
		return FilterNot{FilterEmpty{
			Key: rawFilter.RelationKey,
		}}, nil
	case model.BlockContentDataviewFilter_AllIn:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return FilterAllIn{
			Key:   rawFilter.RelationKey,
			Value: list,
		}, nil
	case model.BlockContentDataviewFilter_NotAllIn:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return FilterNot{FilterAllIn{
			Key:   rawFilter.RelationKey,
			Value: list,
		}}, nil
	case model.BlockContentDataviewFilter_ExactIn:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return newFilterOptionsEqual(&anyenc.Arena{}, rawFilter.RelationKey, list, optionsToMap(spaceID, rawFilter.RelationKey, store)), nil
	case model.BlockContentDataviewFilter_NotExactIn:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return FilterNot{newFilterOptionsEqual(&anyenc.Arena{}, rawFilter.RelationKey, list, optionsToMap(spaceID, rawFilter.RelationKey, store))}, nil
	case model.BlockContentDataviewFilter_Exists:
		return FilterExists{
			Key: rawFilter.RelationKey,
		}, nil
	default:
		return nil, fmt.Errorf("unexpected filter cond: %v", rawFilter.Condition)
	}
}

func makeComplexFilter(rawFilter *model.BlockContentDataviewFilter, s *types.Struct) (Filter, error) {
	filterType := pbtypes.GetString(s, bundle.RelationKeyType.String())
	// TODO: rewrite to switch statement once we have more filter types
	if filterType == "valueFromRelation" {
		return Filter2ValuesComp{
			Key1: rawFilter.RelationKey,
			Key2: pbtypes.GetString(s, bundle.RelationKeyRelationKey.String()),
			Cond: rawFilter.Condition,
		}, nil
	}
	return nil, fmt.Errorf("unsupported type of complex filter: %s", filterType)
}

type WithNestedFilter interface {
	IterateNestedFilters(func(nestedFilter Filter) error) error
}

type Filter interface {
	FilterObject(g *types.Struct) bool
	AnystoreFilter() query.Filter
}

type FiltersAnd []Filter

var _ WithNestedFilter = FiltersAnd{}

func (a FiltersAnd) FilterObject(g *types.Struct) bool {
	for _, f := range a {
		if !f.FilterObject(g) {
			return false
		}
	}
	return true
}

func (a FiltersAnd) AnystoreFilter() query.Filter {
	filters := make([]query.Filter, 0, len(a))
	for _, f := range a {
		anystoreFilter := f.AnystoreFilter()
		filters = append(filters, anystoreFilter)
	}
	return query.And(filters)
}

func (a FiltersAnd) IterateNestedFilters(fn func(nestedFilter Filter) error) error {
	return iterateNestedFilters(a, fn)
}

type FiltersOr []Filter

var _ WithNestedFilter = FiltersOr{}

func (fo FiltersOr) FilterObject(g *types.Struct) bool {
	if len(fo) == 0 {
		return true
	}
	for _, f := range fo {
		if f.FilterObject(g) {
			return true
		}
	}
	return false
}

func (fo FiltersOr) AnystoreFilter() query.Filter {
	filters := make([]query.Filter, 0, len(fo))
	for _, f := range fo {
		anystoreFilter := f.AnystoreFilter()
		filters = append(filters, anystoreFilter)
	}
	return query.Or(filters)
}

func (fo FiltersOr) IterateNestedFilters(fn func(nestedFilter Filter) error) error {
	return iterateNestedFilters(fo, fn)
}

func iterateNestedFilters[F ~[]Filter](composedFilter F, fn func(nestedFilter Filter) error) error {
	for _, f := range composedFilter {
		if withNested, ok := f.(WithNestedFilter); ok {
			err := withNested.IterateNestedFilters(fn)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type FilterNot struct {
	Filter Filter
}

func (n FilterNot) FilterObject(g *types.Struct) bool {
	if n.Filter == nil {
		return false
	}
	return !n.Filter.FilterObject(g)
}

func (f FilterNot) AnystoreFilter() query.Filter {
	filter := f.Filter.AnystoreFilter()
	return negateFilter(filter)
}

func negateFilter(filter query.Filter) query.Filter {
	switch v := filter.(type) {
	case query.And:
		negated := make(query.Or, 0, len(v))
		for _, f := range v {
			negated = append(negated, negateFilter(f))
		}
		return negated
	case query.Or:
		negated := make(query.And, 0, len(v))
		for _, f := range v {
			negated = append(negated, negateFilter(f))
		}
		return negated
	case query.Key:
		if nested, ok := v.Filter.(query.Not); ok {
			return query.Key{
				Path:   v.Path,
				Filter: nested.Filter,
			}
		}
		return query.Key{
			Path: v.Path,
			Filter: query.Not{
				Filter: v.Filter,
			},
		}
	default:
		return query.Not{Filter: filter}
	}
}

type FilterEq struct {
	Key   string
	Cond  model.BlockContentDataviewFilterCondition
	Value *types.Value
}

func (e FilterEq) AnystoreFilter() query.Filter {
	path := []string{e.Key}
	var op query.CompOp
	switch e.Cond {
	case model.BlockContentDataviewFilter_Equal:
		op = query.CompOpEq
	case model.BlockContentDataviewFilter_Greater:
		op = query.CompOpGt
	case model.BlockContentDataviewFilter_GreaterOrEqual:
		op = query.CompOpGte
	case model.BlockContentDataviewFilter_Less:
		op = query.CompOpLt
	case model.BlockContentDataviewFilter_LessOrEqual:
		op = query.CompOpLte
	case model.BlockContentDataviewFilter_NotEqual:
		op = query.CompOpNe
	}
	return query.Key{
		Path:   path,
		Filter: query.NewCompValue(op, encodeScalarPbValue(&anyenc.Arena{}, e.Value)),
	}
}

func encodeScalarPbValue(a *anyenc.Arena, v *types.Value) *anyenc.Value {
	if v == nil || v.Kind == nil {
		return nil
	}
	switch v.Kind.(type) {
	case *types.Value_NullValue:
		return a.NewNull()
	case *types.Value_StringValue:
		return a.NewString(v.GetStringValue())
	case *types.Value_NumberValue:
		return a.NewNumberFloat64(v.GetNumberValue())
	case *types.Value_BoolValue:
		if v.GetBoolValue() {
			return a.NewTrue()
		} else {
			return a.NewFalse()
		}
	case *types.Value_StructValue:
		return nil
	case *types.Value_ListValue:
		return nil
	}
	return nil
}

func (e FilterEq) FilterObject(g *types.Struct) bool {
	val := pbtypes.Get(g, e.Key)
	return e.filterObject(val)
}

func (e FilterEq) filterObject(v *types.Value) bool {
	if list := v.GetListValue(); list != nil && e.Value.GetListValue() == nil {
		for _, lv := range list.Values {
			if e.filterObject(lv) {
				return true
			}
		}
		return false
	}
	comp := e.Value.Compare(v)
	switch e.Cond {
	case model.BlockContentDataviewFilter_Equal:
		return comp == 0
	case model.BlockContentDataviewFilter_Greater:
		return comp == -1
	case model.BlockContentDataviewFilter_GreaterOrEqual:
		return comp <= 0
	case model.BlockContentDataviewFilter_Less:
		return comp == 1
	case model.BlockContentDataviewFilter_LessOrEqual:
		return comp >= 0
	case model.BlockContentDataviewFilter_NotEqual:
		return comp != 0
	}
	return false
}

type FilterHasPrefix struct {
	Key, Prefix string
}

func (p FilterHasPrefix) FilterObject(s *types.Struct) bool {
	val := pbtypes.Get(s, p.Key)
	if strings.HasPrefix(val.GetStringValue(), p.Prefix) {
		return true
	}

	list := val.GetListValue()
	if list == nil {
		return false
	}

	for _, v := range list.Values {
		if strings.HasPrefix(v.GetStringValue(), p.Prefix) {
			return true
		}
	}
	return false
}

func (p FilterHasPrefix) AnystoreFilter() query.Filter {
	re, err := regexp.Compile("^" + regexp.QuoteMeta(p.Prefix))
	if err != nil {
		log.Errorf("failed to build anystore HAS PREFIX filter: %v", err)
	}
	return query.Key{
		Path:   []string{p.Key},
		Filter: query.Regexp{Regexp: re},
	}
}

// any
type FilterIn struct {
	Key   string
	Value *types.ListValue
}

func (i FilterIn) FilterObject(g *types.Struct) bool {
	val := pbtypes.Get(g, i.Key)
	for _, v := range i.Value.Values {
		eq := FilterEq{Value: v, Cond: model.BlockContentDataviewFilter_Equal}
		if eq.filterObject(val) {
			return true
		}
	}
	return false
}

func (i FilterIn) AnystoreFilter() query.Filter {
	path := []string{i.Key}
	conds := make([]query.Filter, 0, len(i.Value.GetValues()))
	arena := &anyenc.Arena{}
	for _, v := range i.Value.GetValues() {
		conds = append(conds, query.Key{
			Path:   path,
			Filter: query.NewCompValue(query.CompOpEq, encodeScalarPbValue(arena, v)),
		})
	}
	return query.Or(conds)
}

type FilterLike struct {
	Key   string
	Value *types.Value
}

func (l FilterLike) FilterObject(g *types.Struct) bool {
	val := pbtypes.Get(g, l.Key)
	if val == nil {
		return false
	}
	valStr := val.GetStringValue()
	if valStr == "" {
		return false
	}
	return strings.Contains(strings.ToLower(valStr), strings.ToLower(l.Value.GetStringValue()))
}

func (l FilterLike) AnystoreFilter() query.Filter {
	re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(l.Value.GetStringValue()))
	if err != nil {
		log.Errorf("failed to build anystore LIKE filter: %v", err)
	}
	return query.Key{
		Path: []string{l.Key},
		Filter: query.Regexp{
			Regexp: re,
		},
	}
}

type FilterExists struct {
	Key string
}

func (e FilterExists) FilterObject(g *types.Struct) bool {
	val := pbtypes.Get(g, e.Key)
	if val == nil {
		return false
	}

	return true
}

func (e FilterExists) AnystoreFilter() query.Filter {
	return query.Key{
		Path:   []string{e.Key},
		Filter: query.Exists{},
	}
}

type FilterEmpty struct {
	Key string
}

func (e FilterEmpty) FilterObject(g *types.Struct) bool {
	val := pbtypes.Get(g, e.Key)
	if val == nil {
		return true
	}
	if val.Kind == nil {
		return true
	}
	switch v := val.Kind.(type) {
	case *types.Value_NullValue:
		return true
	case *types.Value_StringValue:
		return v.StringValue == ""
	case *types.Value_ListValue:
		return v.ListValue == nil || len(v.ListValue.Values) == 0
	case *types.Value_StructValue:
		return v.StructValue == nil
	case *types.Value_NumberValue:
		return v.NumberValue == 0
	case *types.Value_BoolValue:
		return !v.BoolValue
	}
	return false
}

var (
	emptyArrayValue = anyenc.MustParseJson(`[]`).MarshalTo(nil)
)

func (e FilterEmpty) AnystoreFilter() query.Filter {
	path := []string{e.Key}
	return query.Or{
		query.Key{
			Path:   path,
			Filter: query.Not{Filter: query.Exists{}},
		},
		query.Key{
			Path:   path,
			Filter: query.NewComp(query.CompOpEq, nil),
		},
		query.Key{
			Path:   path,
			Filter: query.NewComp(query.CompOpEq, ""),
		},
		query.Key{
			Path:   path,
			Filter: query.NewComp(query.CompOpEq, 0),
		},
		query.Key{
			Path:   path,
			Filter: query.NewComp(query.CompOpEq, false),
		},
		query.Key{
			Path: path,
			Filter: &query.Comp{
				CompOp:  query.CompOpEq,
				EqValue: emptyArrayValue,
			},
		},
	}
}

// all?
type FilterAllIn struct {
	Key   string
	Value *types.ListValue
}

func (l FilterAllIn) FilterObject(g *types.Struct) bool {
	val := pbtypes.Get(g, l.Key)
	if val == nil {
		return false
	}

	list, err := pbtypes.ValueListWrapper(val)
	if err != nil {
		return false
	}
	if list == nil {
		return false
	}
	exist := func(v *types.Value) bool {
		for _, lv := range list.GetValues() {
			if v.Equal(lv) {
				return true
			}
		}
		return false
	}
	for _, ev := range l.Value.Values {
		if !exist(ev) {
			return false
		}
	}
	return true
}

func (l FilterAllIn) AnystoreFilter() query.Filter {
	path := []string{l.Key}
	conds := make([]query.Filter, 0, len(l.Value.GetValues()))
	arena := &anyenc.Arena{}
	for _, v := range l.Value.GetValues() {
		conds = append(conds, query.Key{
			Path:   path,
			Filter: query.NewCompValue(query.CompOpEq, encodeScalarPbValue(arena, v)),
		})
	}
	return query.And(conds)
}

func newFilterOptionsEqual(arena *anyenc.Arena, key string, value *types.ListValue, options map[string]string) *FilterOptionsEqual {
	f := &FilterOptionsEqual{
		arena:   arena,
		Key:     key,
		Value:   value,
		Options: options,
	}
	f.compileValueFilter()
	return f
}

type FilterOptionsEqual struct {
	arena *anyenc.Arena

	Key     string
	Value   *types.ListValue
	Options map[string]string

	// valueFilter is precompiled filter without key selector
	valueFilter query.Filter
}

func (exIn *FilterOptionsEqual) FilterObject(g *types.Struct) bool {
	val := pbtypes.Get(g, exIn.Key)
	if val == nil {
		return false
	}
	list, err := pbtypes.ValueListWrapper(val)
	if err != nil {
		return false
	}
	if list == nil {
		return false
	}

	// TODO It's absolutely not clear why we filter by options CONDITIONALLY, should be filtered always
	if len(exIn.Options) > 0 {
		list.Values = slice.Filter(list.GetValues(), func(value *types.Value) bool {
			_, ok := exIn.Options[value.GetStringValue()]
			return ok
		})
	}

	if len(list.GetValues()) != len(exIn.Value.GetValues()) {
		return false
	}
	exist := func(v *types.Value) bool {
		for _, lv := range list.Values {
			if v.Equal(lv) {
				return true
			}
		}
		return false
	}
	for _, ev := range exIn.Value.GetValues() {
		if !exist(ev) {
			return false
		}
	}
	return true
}

func (exIn *FilterOptionsEqual) Ok(v *anyenc.Value) bool {
	defer exIn.arena.Reset()

	arr := v.GetArray(exIn.Key)
	// Just fall back to precompiled filter
	if len(arr) == 0 {
		return exIn.valueFilter.Ok(v.Get(exIn.Key))
	}

	// Discard deleted options
	optionList := exIn.arena.NewArray()
	var i int
	for _, arrVal := range arr {
		optionId := string(arrVal.GetStringBytes())
		_, ok := exIn.Options[optionId]
		if ok {
			optionList.SetArrayItem(i, exIn.arena.NewString(optionId))
			i++
		}
	}
	return exIn.valueFilter.Ok(optionList)
}

func (exIn *FilterOptionsEqual) compileValueFilter() {
	conds := make([]query.Filter, 0, len(exIn.Value.GetValues())+1)
	conds = append(conds, query.Size{Size: int64(len(exIn.Value.GetValues()))})
	arena := &anyenc.Arena{}
	for _, v := range exIn.Value.GetValues() {
		conds = append(conds, query.NewCompValue(query.CompOpEq, encodeScalarPbValue(arena, v)))
	}
	exIn.valueFilter = query.And(conds)
}

func (exIn *FilterOptionsEqual) IndexBounds(fieldName string, bs query.Bounds) (bounds query.Bounds) {
	return bs
}

func (exIn *FilterOptionsEqual) AnystoreFilter() query.Filter {
	return exIn
}

func (exIn *FilterOptionsEqual) String() string {
	return "{}"
}

func optionsToMap(spaceID string, key string, store ObjectStore) map[string]string {
	result := make(map[string]string)
	options, err := store.ListRelationOptions(key)
	if err != nil {
		log.Warn("nil objectStore for getting options")
		return result
	}
	for _, opt := range options {
		result[opt.Id] = opt.Text
	}

	return result
}

func makeFilterNestedIn(spaceID string, rawFilter *model.BlockContentDataviewFilter, store ObjectStore, relationKey string, nestedRelationKey string) (Filter, error) {
	rawNestedFilter := proto.Clone(rawFilter).(*model.BlockContentDataviewFilter)
	rawNestedFilter.RelationKey = nestedRelationKey
	nestedFilter, err := MakeFilter(spaceID, rawNestedFilter, store)
	if err != nil {
		return nil, fmt.Errorf("make nested filter %s -> %s: %w", relationKey, nestedRelationKey, err)
	}
	records, err := store.QueryRaw(&Filters{FilterObj: nestedFilter}, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("enrich nested filter %s: %w", nestedFilter, err)
	}

	ids := make([]string, 0, len(records))
	for _, rec := range records {
		ids = append(ids, pbtypes.GetString(rec.Details, bundle.RelationKeyId.String()))
	}
	return &FilterNestedIn{
		Key:                    relationKey,
		FilterForNestedObjects: nestedFilter,
		IDs:                    ids,
	}, nil
}

// FilterNestedIn returns true for object that has a relation pointing to any object that matches FilterForNestedObjects.
// This filter uses special machinery in able to work: it only functions when IDs field is populated by IDs of objects
// that match FilterForNestedObjects. You can't just use FilterNestedIn without populating IDs field
type FilterNestedIn struct {
	Key                    string
	FilterForNestedObjects Filter

	IDs []string
}

var _ WithNestedFilter = &FilterNestedIn{}

func (i *FilterNestedIn) FilterObject(g *types.Struct) bool {
	val := pbtypes.Get(g, i.Key)
	for _, id := range i.IDs {
		eq := FilterEq{Value: pbtypes.String(id), Cond: model.BlockContentDataviewFilter_Equal}
		if eq.filterObject(val) {
			return true
		}
	}
	return false
}

func (i *FilterNestedIn) AnystoreFilter() query.Filter {
	path := []string{i.Key}
	conds := make([]query.Filter, 0, len(i.IDs))
	for _, id := range i.IDs {
		conds = append(conds, query.Key{
			Path:   path,
			Filter: query.NewComp(query.CompOpEq, id),
		})
	}
	return query.Or(conds)
}

func (i *FilterNestedIn) IterateNestedFilters(fn func(nestedFilter Filter) error) error {
	return fn(i)
}

// See FilterNestedIn for details
type FilterNestedNotIn struct {
	Key                    string
	FilterForNestedObjects Filter

	IDs []string
}

func makeFilterNestedNotIn(spaceID string, rawFilter *model.BlockContentDataviewFilter, store ObjectStore, relationKey string, nestedRelationKey string) (Filter, error) {
	rawNestedFilter := proto.Clone(rawFilter).(*model.BlockContentDataviewFilter)
	rawNestedFilter.RelationKey = nestedRelationKey

	subQueryRawFilter := proto.Clone(rawFilter).(*model.BlockContentDataviewFilter)
	subQueryRawFilter.RelationKey = nestedRelationKey
	subQueryRawFilter.Condition = model.BlockContentDataviewFilter_Equal

	subQueryFilter, err := MakeFilter(spaceID, subQueryRawFilter, store)
	if err != nil {
		return nil, fmt.Errorf("make nested filter %s -> %s: %w", relationKey, nestedRelationKey, err)
	}
	records, err := store.QueryRaw(&Filters{FilterObj: subQueryFilter}, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("enrich nested filter: %w", err)
	}

	ids := make([]string, 0, len(records))
	for _, rec := range records {
		ids = append(ids, pbtypes.GetString(rec.Details, bundle.RelationKeyId.String()))
	}
	nestedFilter, err := MakeFilter(spaceID, rawNestedFilter, store)
	if err != nil {
		return nil, fmt.Errorf("make nested filter %s -> %s: %w", relationKey, nestedRelationKey, err)
	}
	return &FilterNestedNotIn{
		Key:                    relationKey,
		FilterForNestedObjects: nestedFilter,
		IDs:                    ids,
	}, nil
}

func (i *FilterNestedNotIn) FilterObject(g *types.Struct) bool {
	val := pbtypes.Get(g, i.Key)
	for _, id := range i.IDs {
		eq := FilterEq{Value: pbtypes.String(id), Cond: model.BlockContentDataviewFilter_Equal}
		if eq.filterObject(val) {
			return false
		}
	}
	return true
}

func (i *FilterNestedNotIn) AnystoreFilter() query.Filter {
	path := []string{i.Key}
	conds := make([]query.Filter, 0, len(i.IDs))
	for _, id := range i.IDs {
		conds = append(conds, query.Key{
			Path:   path,
			Filter: query.NewComp(query.CompOpNe, id),
		})
	}
	return query.And(conds)
}

func (i *FilterNestedNotIn) IterateNestedFilters(fn func(nestedFilter Filter) error) error {
	return fn(i)
}

type Filter2ValuesComp struct {
	Key1, Key2 string
	Cond       model.BlockContentDataviewFilterCondition
}

func (i Filter2ValuesComp) FilterObject(g *types.Struct) bool {
	val1 := pbtypes.Get(g, i.Key1)
	val2 := pbtypes.Get(g, i.Key2)
	eq := FilterEq{Value: val2, Cond: i.Cond}
	return eq.filterObject(val1)
}

func (i Filter2ValuesComp) AnystoreFilter() query.Filter {
	var op query.CompOp
	switch i.Cond {
	case model.BlockContentDataviewFilter_Equal:
		op = query.CompOpEq
	case model.BlockContentDataviewFilter_Greater:
		op = query.CompOpGt
	case model.BlockContentDataviewFilter_GreaterOrEqual:
		op = query.CompOpGte
	case model.BlockContentDataviewFilter_Less:
		op = query.CompOpLt
	case model.BlockContentDataviewFilter_LessOrEqual:
		op = query.CompOpLte
	case model.BlockContentDataviewFilter_NotEqual:
		op = query.CompOpNe
	}
	return &Anystore2ValuesComp{
		RelationKey1: i.Key1,
		RelationKey2: i.Key2,
		CompOp:       op,
	}
}

type Anystore2ValuesComp struct {
	RelationKey1, RelationKey2 string
	CompOp                     query.CompOp
	buf1, buf2                 []byte
}

func (e *Anystore2ValuesComp) Ok(v *anyenc.Value) bool {
	value1 := v.Get(e.RelationKey1)
	value2 := v.Get(e.RelationKey2)
	e.buf1 = value1.MarshalTo(e.buf1[:0])
	e.buf2 = value2.MarshalTo(e.buf2[:0])
	comp := bytes.Compare(e.buf1, e.buf2)
	switch e.CompOp {
	case query.CompOpEq:
		return comp == 0
	case query.CompOpGt:
		return comp > 0
	case query.CompOpGte:
		return comp >= 0
	case query.CompOpLt:
		return comp < 0
	case query.CompOpLte:
		return comp <= 0
	case query.CompOpNe:
		return comp != 0
	default:
		panic(fmt.Errorf("unexpected comp op: %v", e.CompOp))
	}
}

func (e *Anystore2ValuesComp) IndexBounds(_ string, bs query.Bounds) (bounds query.Bounds) {
	return bs
}

func (e *Anystore2ValuesComp) String() string {
	var comp string
	switch e.CompOp {
	case query.CompOpEq:
		comp = "$eq"
	case query.CompOpGt:
		comp = "$gt"
	case query.CompOpGte:
		comp = "$gte"
	case query.CompOpLt:
		comp = "$lt"
	case query.CompOpLte:
		comp = "$lte"
	case query.CompOpNe:
		comp = "$ne"
	default:
		panic(fmt.Errorf("unexpected comp op: %v", e.CompOp))
	}
	return fmt.Sprintf(`{"$comp_values": {"%s": ["%s", "%s"]}}`, comp, e.RelationKey1, e.RelationKey2)
}
