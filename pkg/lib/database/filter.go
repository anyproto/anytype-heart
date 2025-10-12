package database

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-store/syncpool"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/dateutil"
	"github.com/anyproto/anytype-heart/util/debug"
)

var (
	ErrValueMustBeListSupporting = errors.New("value must be list supporting")
)

func MakeFilters(protoFilters []FilterRequest, store ObjectStore) (Filter, error) {
	spaceId := getSpaceIDFromFilters(protoFilters)
	// to avoid unnecessary nested filter
	if len(protoFilters) == 1 && len(protoFilters[0].NestedFilters) > 0 && protoFilters[0].Operator != model.BlockContentDataviewFilter_No {
		return MakeFilter(spaceId, protoFilters[0], store)
	}
	return MakeFilter(spaceId, FilterRequest{
		Operator:      model.BlockContentDataviewFilter_And,
		NestedFilters: protoFilters,
	}, store)
}

func MakeFilter(spaceId string, protoFilter FilterRequest, store ObjectStore) (Filter, error) {
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

func NestedRelationKey(baseRelationKey domain.RelationKey, nestedRelationKey domain.RelationKey) domain.RelationKey {
	return baseRelationKey + "." + nestedRelationKey
}

func makeFilter(spaceID string, rawFilter FilterRequest, store ObjectStore) (Filter, error) {
	if store == nil {
		return nil, fmt.Errorf("objectStore dependency is nil")
	}
	if rawFilter.Condition == model.BlockContentDataviewFilter_None {
		return nil, nil
	}
	rawFilters := transformQuickOption(rawFilter)

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

func makeFilterByCondition(spaceID string, rawFilter FilterRequest, store ObjectStore) (Filter, error) {
	parts := strings.SplitN(string(rawFilter.RelationKey), ".", 2)
	if len(parts) == 2 {
		relationKey := domain.RelationKey(parts[0])
		nestedRelationKey := domain.RelationKey(parts[1])

		if rawFilter.Condition == model.BlockContentDataviewFilter_NotEqual || rawFilter.Condition == model.BlockContentDataviewFilter_NotIn {
			return makeFilterNestedNotIn(spaceID, rawFilter, store, relationKey, nestedRelationKey)
		} else {
			return makeFilterNestedIn(spaceID, rawFilter, store, relationKey, nestedRelationKey)
		}
	}

	// replaces "value == false" to "value != true" for expected work with checkboxes
	if rawFilter.Condition == model.BlockContentDataviewFilter_Equal {
		v, ok := rawFilter.Value.TryBool()
		if ok && !v {
			rawFilter = FilterRequest{
				RelationKey:      rawFilter.RelationKey,
				RelationProperty: rawFilter.RelationProperty,
				Condition:        model.BlockContentDataviewFilter_NotEqual,
				Value:            domain.Bool(true),
			}
		}

	}
	// replaces "value != false" to "value == true" for expected work with checkboxes
	if rawFilter.Condition == model.BlockContentDataviewFilter_NotEqual {
		v, ok := rawFilter.Value.TryBool()
		if ok && !v {
			rawFilter = FilterRequest{
				RelationKey:      rawFilter.RelationKey,
				RelationProperty: rawFilter.RelationProperty,
				Condition:        model.BlockContentDataviewFilter_Equal,
				Value:            domain.Bool(true),
			}
		}
	}

	if str, ok := rawFilter.Value.TryMapValue(); ok {
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
			Value: rawFilter.Value.String(),
		}, nil
	case model.BlockContentDataviewFilter_NotLike:
		return FilterNot{FilterLike{
			Key:   rawFilter.RelationKey,
			Value: rawFilter.Value.String(),
		}}, nil
	case model.BlockContentDataviewFilter_In:
		// hack for queries for relations containing date objects ids with format _date_YYYY-MM-DD-hh-mm-ssZ-zzzz
		// to find all date object ids of the same day we search by prefix _date_YYYY-MM-DD
		if dateObject, err := dateutil.BuildDateObjectFromId(rawFilter.Value.String()); err == nil {
			return FilterHasPrefix{
				Key:    rawFilter.RelationKey,
				Prefix: dateutil.NewDateObject(dateObject.Time(), false).Id(),
			}, nil
		}
		list, err := rawFilter.Value.TryWrapToList()
		if err != nil {
			return nil, errors.Join(ErrValueMustBeListSupporting, err)
		}
		return FilterIn{
			Key:   rawFilter.RelationKey,
			Value: list,
		}, nil
	case model.BlockContentDataviewFilter_NotIn:
		list, err := rawFilter.Value.TryWrapToList()
		if err != nil {
			return nil, errors.Join(ErrValueMustBeListSupporting, err)
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
		if list, err := wrapValueToStringList(rawFilter.Value); err == nil {
			return FilterAllIn{
				Key:     rawFilter.RelationKey,
				Strings: list,
			}, nil
		}
		if list, err := wrapValueToFloatList(rawFilter.Value); err == nil {
			return FilterAllIn{
				Key:    rawFilter.RelationKey,
				Floats: list,
			}, nil
		}
		return nil, fmt.Errorf("unsupported type: %v", rawFilter.Value.Type())
	case model.BlockContentDataviewFilter_NotAllIn:
		if list, err := wrapValueToStringList(rawFilter.Value); err == nil {
			return FilterNot{FilterAllIn{
				Key:     rawFilter.RelationKey,
				Strings: list,
			}}, nil
		}
		if list, err := wrapValueToFloatList(rawFilter.Value); err == nil {
			return FilterNot{FilterAllIn{
				Key:    rawFilter.RelationKey,
				Floats: list,
			}}, nil
		}
		return nil, fmt.Errorf("unsupported type: %v", rawFilter.Value.Type())
	case model.BlockContentDataviewFilter_ExactIn:
		list, err := wrapValueToStringList(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return newFilterOptionsEqual(&anyenc.Arena{}, rawFilter.RelationKey, list, optionsToMap(rawFilter.RelationKey, store)), nil
	case model.BlockContentDataviewFilter_NotExactIn:
		list, err := wrapValueToStringList(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return FilterNot{newFilterOptionsEqual(&anyenc.Arena{}, rawFilter.RelationKey, list, optionsToMap(rawFilter.RelationKey, store))}, nil
	case model.BlockContentDataviewFilter_Exists:
		return FilterExists{
			Key: rawFilter.RelationKey,
		}, nil
	default:
		return nil, fmt.Errorf("unexpected filter cond: %v", rawFilter.Condition)
	}
}

func wrapValueToStringList(val domain.Value) ([]string, error) {
	if v, ok := val.TryString(); ok {
		return []string{v}, nil
	}
	if v, ok := val.TryStringList(); ok {
		return v, nil
	}
	return nil, fmt.Errorf("unsupported type: %v", val.Type())
}

func wrapValueToFloatList(val domain.Value) ([]float64, error) {
	if v, ok := val.TryFloat64(); ok {
		return []float64{v}, nil
	}
	if v, ok := val.TryFloat64List(); ok {
		return v, nil
	}
	return nil, fmt.Errorf("unsupported type: %v", val.Type())
}

func makeComplexFilter(rawFilter FilterRequest, s domain.ValueMap) (Filter, error) {
	filterType := s.GetString(bundle.RelationKeyType.String())
	// TODO: rewrite to switch statement once we have more filter types
	if filterType == "valueFromRelation" {
		return Filter2ValuesComp{
			Key1: rawFilter.RelationKey,
			Key2: domain.RelationKey(s.GetString(bundle.RelationKeyRelationKey.String())),
			Cond: rawFilter.Condition,
		}, nil
	}
	return nil, fmt.Errorf("unsupported type of complex filter: %s", filterType)
}

type WithNestedFilter interface {
	IterateNestedFilters(func(nestedFilter Filter) error) error
}

type Filter interface {
	FilterObject(g *domain.Details) bool
	AnystoreFilter() query.Filter
}

type FiltersAnd []Filter

var _ WithNestedFilter = FiltersAnd{}

func (a FiltersAnd) FilterObject(g *domain.Details) bool {
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

func (fo FiltersOr) FilterObject(g *domain.Details) bool {
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

func (n FilterNot) FilterObject(g *domain.Details) bool {
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
	Key   domain.RelationKey
	Cond  model.BlockContentDataviewFilterCondition
	Value domain.Value
}

func (e FilterEq) AnystoreFilter() query.Filter {
	path := []string{string(e.Key)}
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

	// TODO: GO-5616 Remove logging
	e.logIfInvalidValue()

	return query.Key{
		Path:   path,
		Filter: query.NewCompValue(op, e.Value.ToAnyEnc(&anyenc.Arena{})),
	}
}

// TODO: GO-5616 Remove logging when we understand what filters clients pass as []string values
func (e FilterEq) logIfInvalidValue() {
	var typeS string
	switch e.Value.Raw().(type) {
	case []string:
		typeS = "[]string"
	case []float64:
		typeS = "[]float64"
	case []int64:
		typeS = "[]int64"
	}

	if typeS != "" {
		stack := debug.Stack(false)
		log.With("key", e.Key).
			With("cond", e.Cond).
			With("type", typeS).
			With("stacktrace", stack).
			Warn("Eq filter contains value of invalid type")
	}
}

func (e FilterEq) FilterObject(g *domain.Details) bool {
	return e.filterObject(g.Get(domain.RelationKey(e.Key)))
}

func (e FilterEq) filterObject(v domain.Value) bool {
	// if list := v.GetListValue(); list != nil && e.Value.GetListValue() == nil {
	isFilterValueScalar := !e.Value.IsStringList() && !e.Value.IsFloat64List()
	if isFilterValueScalar {
		if list, ok := v.TryFloat64List(); ok {
			for _, lv := range list {
				if e.filterObject(domain.Float64(lv)) {
					return true
				}
			}
			return false
		}
		if list, ok := v.TryStringList(); ok {
			for _, lv := range list {
				if e.filterObject(domain.String(lv)) {
					return true
				}
			}
			return false
		}
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
	Key    domain.RelationKey
	Prefix string
}

func (p FilterHasPrefix) FilterObject(s *domain.Details) bool {
	val := s.Get(p.Key)
	if strings.HasPrefix(val.String(), p.Prefix) {
		return true
	}

	list := val.StringList()
	for _, v := range list {
		if strings.HasPrefix(v, p.Prefix) {
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
		Path:   []string{string(p.Key)},
		Filter: query.Regexp{Regexp: re},
	}
}

// any
type FilterIn struct {
	Key   domain.RelationKey
	Value []domain.Value
}

func (i FilterIn) FilterObject(g *domain.Details) bool {
	val := g.Get(i.Key)
	for _, v := range i.Value {
		eq := FilterEq{Value: v, Cond: model.BlockContentDataviewFilter_Equal}
		if eq.filterObject(val) {
			return true
		}
	}
	return false
}

func (i FilterIn) AnystoreFilter() query.Filter {
	arena := &anyenc.Arena{}
	inVals := make([]*anyenc.Value, 0, len(i.Value))
	for _, v := range i.Value {
		inVals = append(inVals, v.ToAnyEnc(arena))
	}
	filter := query.NewInValue(inVals...)
	return query.Key{
		Path:   []string{string(i.Key)},
		Filter: filter,
	}
}

type FilterLike struct {
	Key   domain.RelationKey
	Value string
}

func (l FilterLike) FilterObject(g *domain.Details) bool {
	val, ok := g.TryString(l.Key)
	if !ok {
		return false
	}
	return strings.Contains(strings.ToLower(val), strings.ToLower(l.Value))
}

func (l FilterLike) AnystoreFilter() query.Filter {
	re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(l.Value))
	if err != nil {
		log.Errorf("failed to build anystore LIKE filter: %v", err)
	}
	return query.Key{
		Path: []string{string(l.Key)},
		Filter: query.Regexp{
			Regexp: re,
		},
	}
}

type FilterExists struct {
	Key domain.RelationKey
}

func (e FilterExists) FilterObject(g *domain.Details) bool {
	return g.Has(e.Key)
}

func (e FilterExists) AnystoreFilter() query.Filter {
	return query.Key{
		Path:   []string{string(e.Key)},
		Filter: query.Exists{},
	}
}

type FilterEmpty struct {
	Key domain.RelationKey
}

func (e FilterEmpty) FilterObject(g *domain.Details) bool {
	val := g.Get(e.Key)
	return val.IsEmpty()
}

var (
	emptyArrayValue = anyenc.MustParseJson(`[]`).MarshalTo(nil)
)

func (e FilterEmpty) AnystoreFilter() query.Filter {
	path := []string{string(e.Key)}
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
	Key     domain.RelationKey
	Strings []string
	Floats  []float64
}

func (l FilterAllIn) FilterObject(g *domain.Details) bool {
	val := g.Get(l.Key)
	if !val.Ok() {
		return false
	}

	if len(l.Strings) > 0 {
		// Single string
		{
			val, ok := g.TryString(l.Key)
			if ok && len(l.Strings) == 1 {
				return l.Strings[0] == val
			}
		}
		// TryString list
		{
			val, ok := g.TryStringList(l.Key)
			if ok {
				return lo.Every(val, l.Strings)
			}
		}
		return false
	}
	if len(l.Floats) > 0 {
		// Single float
		{
			val, ok := g.TryFloat64(l.Key)
			if ok && len(l.Floats) == 1 {
				return l.Floats[0] == val
			}
		}
		// Float64 list
		{
			val, ok := g.TryFloat64List(l.Key)
			if ok {
				return lo.Every(val, l.Floats)
			}
		}
		return false
	}
	return true
}

func (l FilterAllIn) AnystoreFilter() query.Filter {
	arena := &anyenc.Arena{}
	path := []string{string(l.Key)}
	conds := make([]query.Filter, 0, len(l.Strings)+len(l.Floats))
	for _, v := range l.Strings {
		conds = append(conds, query.Key{
			Path:   path,
			Filter: query.NewCompValue(query.CompOpEq, arena.NewString(v)),
		})
	}
	for _, v := range l.Floats {
		conds = append(conds, query.Key{
			Path:   path,
			Filter: query.NewCompValue(query.CompOpEq, arena.NewNumberFloat64(v)),
		})
	}
	return query.And(conds)
}

func newFilterOptionsEqual(arena *anyenc.Arena, key domain.RelationKey, value []string, options map[string]*domain.Details) *FilterOptionsEqual {
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

	Key     domain.RelationKey
	Value   []string
	Options map[string]*domain.Details

	// valueFilter is precompiled filter without key selector
	valueFilter query.Filter
}

func (exIn *FilterOptionsEqual) FilterObject(g *domain.Details) bool {
	// If filter is empty, it makes no sense, so to avoid confusion return false
	if len(exIn.Value) == 0 {
		return false
	}

	if val, ok := g.TryString(exIn.Key); ok {
		_, ok := exIn.Options[val]
		if !ok {
			return false
		}
		return slices.Contains(exIn.Value, val)
	}
	if val, ok := g.TryStringList(exIn.Key); ok {
		onlyOptions := lo.Filter(val, func(v string, _ int) bool {
			_, ok := exIn.Options[v]
			return ok
		})
		if len(onlyOptions) != len(exIn.Value) {
			return false
		}
		return lo.Every(onlyOptions, exIn.Value)
	}
	return false
}

func (exIn *FilterOptionsEqual) Ok(v *anyenc.Value, docBuf *syncpool.DocBuffer) bool {
	defer exIn.arena.Reset()

	arr := v.GetArray(string(exIn.Key))
	// Just fall back to precompiled filter
	if len(arr) == 0 {
		return exIn.valueFilter.Ok(v.Get(string(exIn.Key)), docBuf)
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
	return exIn.valueFilter.Ok(optionList, docBuf)
}

func (exIn *FilterOptionsEqual) compileValueFilter() {
	arena := &anyenc.Arena{}
	conds := make([]query.Filter, 0, len(exIn.Value)+1)
	conds = append(conds, query.Size{Size: int64(len(exIn.Value))})
	for _, v := range exIn.Value {
		conds = append(conds, query.NewCompValue(query.CompOpEq, arena.NewString(v)))
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

func optionsToMap(key domain.RelationKey, store ObjectStore) map[string]*domain.Details {
	result := make(map[string]*domain.Details)
	options, err := store.ListRelationOptions(key)
	if err != nil {
		log.Warnf("failed to get relation options from store: %v", err)
		return result
	}
	for _, opt := range options {
		result[opt.Id] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyName:    domain.String(opt.Text),
			bundle.RelationKeyOrderId: domain.String(opt.OrderId),
		})
	}

	return result
}

// objectsToMap collects names of objects that present in details of any objects as a value of detail with key=key
func objectsToMap(key domain.RelationKey, store ObjectStore) map[string]*domain.Details {
	names := make(map[string]*domain.Details)
	targetIdsMap := make(map[string]struct{}, 0)

	err := store.QueryIterate(Query{Filters: []FilterRequest{{
		RelationKey: key,
		Condition:   model.BlockContentDataviewFilter_NotEmpty,
	}}}, func(details *domain.Details) {
		for _, id := range details.GetStringList(key) {
			targetIdsMap[id] = struct{}{}
		}
	})

	if err != nil {
		log.Warnf("failed to get objects from store: %v", err)
		return nil
	}

	targetIds := make([]string, 0, len(targetIdsMap))
	for id := range targetIdsMap {
		targetIds = append(targetIds, id)
	}

	err = store.QueryIterate(Query{Filters: []FilterRequest{{
		RelationKey: bundle.RelationKeyId,
		Condition:   model.BlockContentDataviewFilter_In,
		Value:       domain.StringList(targetIds),
	}}}, func(details *domain.Details) {
		names[details.GetString(bundle.RelationKeyId)] = details.CopyOnlyKeys(bundle.RelationKeyName)
	})

	if err != nil {
		log.Warnf("failed to iterate over objects in store: %v", err)
		return nil
	}

	return names
}

func makeFilterNestedIn(spaceID string, rawFilter FilterRequest, store ObjectStore, relationKey domain.RelationKey, nestedRelationKey domain.RelationKey) (Filter, error) {
	rawNestedFilter := rawFilter
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
		ids = append(ids, rec.Details.GetString(bundle.RelationKeyId))
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
	Key                    domain.RelationKey
	FilterForNestedObjects Filter

	IDs []string
}

var _ WithNestedFilter = &FilterNestedIn{}

func (i *FilterNestedIn) FilterObject(g *domain.Details) bool {
	val := g.Get(i.Key)
	for _, id := range i.IDs {
		eq := FilterEq{Value: domain.String(id), Cond: model.BlockContentDataviewFilter_Equal}
		if eq.filterObject(val) {
			return true
		}
	}
	return false
}

func (i *FilterNestedIn) AnystoreFilter() query.Filter {
	arena := &anyenc.Arena{}
	values := make([]*anyenc.Value, 0, len(i.IDs))
	for _, id := range i.IDs {
		aev := domain.String(id).ToAnyEnc(arena)
		values = append(values, aev)
	}
	filter := query.NewInValue(values...)
	return query.Key{
		Path:   []string{string(i.Key)},
		Filter: filter,
	}
}

func (i *FilterNestedIn) IterateNestedFilters(fn func(nestedFilter Filter) error) error {
	return fn(i)
}

// See FilterNestedIn for details
type FilterNestedNotIn struct {
	Key                    domain.RelationKey
	FilterForNestedObjects Filter

	IDs []string
}

func negativeConditionToPositive(cond model.BlockContentDataviewFilterCondition) (model.BlockContentDataviewFilterCondition, error) {
	switch cond {
	case model.BlockContentDataviewFilter_NotEqual:
		return model.BlockContentDataviewFilter_Equal, nil
	case model.BlockContentDataviewFilter_NotIn:
		return model.BlockContentDataviewFilter_In, nil
	default:
		return 0, fmt.Errorf("condition %d is not supported", cond)
	}
}

func makeFilterNestedNotIn(spaceID string, rawFilter FilterRequest, store ObjectStore, relationKey domain.RelationKey, nestedRelationKey domain.RelationKey) (Filter, error) {
	rawNestedFilter := rawFilter
	rawNestedFilter.RelationKey = nestedRelationKey

	cond, err := negativeConditionToPositive(rawFilter.Condition)
	if err != nil {
		return nil, fmt.Errorf("convert condition: %w", err)
	}

	subQueryRawFilter := rawFilter
	subQueryRawFilter.RelationKey = nestedRelationKey
	subQueryRawFilter.Condition = cond

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
		ids = append(ids, rec.Details.GetString(bundle.RelationKeyId))
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

func (i *FilterNestedNotIn) FilterObject(g *domain.Details) bool {
	val := g.Get(i.Key)
	for _, id := range i.IDs {
		eq := FilterEq{Value: domain.String(id), Cond: model.BlockContentDataviewFilter_Equal}
		if eq.filterObject(val) {
			return false
		}
	}
	return true
}

func (i *FilterNestedNotIn) AnystoreFilter() query.Filter {
	path := []string{i.Key.String()}
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
	Key1, Key2 domain.RelationKey
	Cond       model.BlockContentDataviewFilterCondition
}

func (i Filter2ValuesComp) FilterObject(g *domain.Details) bool {
	val1 := g.Get(i.Key1)
	val2 := g.Get(i.Key2)
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
		RelationKey1: string(i.Key1),
		RelationKey2: string(i.Key2),
		CompOp:       op,
	}
}

type Anystore2ValuesComp struct {
	RelationKey1, RelationKey2 string
	CompOp                     query.CompOp
	buf1, buf2                 []byte
}

func (e *Anystore2ValuesComp) Ok(v *anyenc.Value, docBuf *syncpool.DocBuffer) bool {
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
