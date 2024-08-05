package database

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/anyproto/any-store/encoding"
	"github.com/anyproto/any-store/query"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
	"github.com/valyala/fastjson"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrValueMustBeListSupporting = errors.New("value must be list supporting")
)

func MakeFiltersAnd(protoFilters []FilterRequest, store ObjectStore) (FiltersAnd, error) {
	if store == nil {
		return FiltersAnd{}, fmt.Errorf("objectStore dependency is nil")
	}
	spaceID := getSpaceIDFromFilters(protoFilters)
	protoFilters = TransformQuickOption(protoFilters, nil)

	var and FiltersAnd
	for _, pf := range protoFilters {
		if pf.Condition != model.BlockContentDataviewFilter_None {
			f, err := MakeFilter(spaceID, pf, store)
			if err != nil {
				return nil, err
			}
			and = append(and, f)
		}
	}
	return and, nil
}

func NestedRelationKey(baseRelationKey domain.RelationKey, nestedRelationKey domain.RelationKey) string {
	return fmt.Sprintf("%s.%s", baseRelationKey.String(), nestedRelationKey.String())
}

func MakeFilter(spaceID string, rawFilter FilterRequest, store ObjectStore) (Filter, error) {
	if store == nil {
		return nil, fmt.Errorf("objectStore dependency is nil")
	}
	parts := strings.SplitN(rawFilter.RelationKey, ".", 2)
	if len(parts) == 2 {
		return makeFilterNestedIn(spaceID, rawFilter, store, domain.RelationKey(parts[0]), domain.RelationKey(parts[1]))
	}

	// replaces "value == false" to "value != true" for expected work with checkboxes
	if rawFilter.Condition == model.BlockContentDataviewFilter_Equal {
		v, ok := rawFilter.Value.Bool()
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
		v, ok := rawFilter.Value.Bool()
		if ok && !v {
			rawFilter = FilterRequest{
				RelationKey:      rawFilter.RelationKey,
				RelationProperty: rawFilter.RelationProperty,
				Condition:        model.BlockContentDataviewFilter_Equal,
				Value:            domain.Bool(true),
			}
		}
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
			Value: rawFilter.Value.StringOrDefault(""),
		}, nil
	case model.BlockContentDataviewFilter_NotLike:
		return FilterNot{FilterLike{
			Key:   rawFilter.RelationKey,
			Value: rawFilter.Value.StringOrDefault(""),
		}}, nil
	case model.BlockContentDataviewFilter_In:
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
			Key: domain.RelationKey(rawFilter.RelationKey),
		}, nil
	case model.BlockContentDataviewFilter_NotEmpty:
		return FilterNot{FilterEmpty{
			Key: domain.RelationKey(rawFilter.RelationKey),
		}}, nil
	case model.BlockContentDataviewFilter_AllIn:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		if v := pbtypes.ListValueToFloats(list); len(v) > 0 {
			return FilterAllIn{
				Key:    domain.RelationKey(rawFilter.RelationKey),
				Floats: v,
			}, nil
		}
		if v := pbtypes.ListValueToStrings(list); len(v) > 0 {
			return FilterAllIn{
				Key:     domain.RelationKey(rawFilter.RelationKey),
				Strings: v,
			}, nil
		}
		return nil, fmt.Errorf("unexpected filter value type: %v", list)
	case model.BlockContentDataviewFilter_NotAllIn:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		var filter FilterAllIn
		if v := pbtypes.ListValueToFloats(list); len(v) > 0 {
			filter = FilterAllIn{
				Key:    domain.RelationKey(rawFilter.RelationKey),
				Floats: v,
			}
		}
		if v := pbtypes.ListValueToStrings(list); len(v) > 0 {
			filter = FilterAllIn{
				Key:     domain.RelationKey(rawFilter.RelationKey),
				Strings: v,
			}
		}
		return FilterNot{filter}, nil
	case model.BlockContentDataviewFilter_ExactIn:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return FilterOptionsEqual{
			Key:     domain.RelationKey(rawFilter.RelationKey),
			Value:   pbtypes.ListValueToStrings(list),
			Options: optionsToMap(spaceID, domain.RelationKey(rawFilter.RelationKey), store),
		}, nil
	case model.BlockContentDataviewFilter_NotExactIn:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return FilterNot{FilterOptionsEqual{
			Key:   domain.RelationKey(rawFilter.RelationKey),
			Value: pbtypes.ListValueToStrings(list),
		}}, nil
	case model.BlockContentDataviewFilter_Exists:
		return FilterExists{
			Key: domain.RelationKey(rawFilter.RelationKey),
		}, nil
	default:
		return nil, fmt.Errorf("unexpected filter cond: %v", rawFilter.Condition)
	}
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
	Value domain.Value
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
		return query.Or{
			query.Key{
				Path:   path,
				Filter: query.NewComp(query.CompOpNe, e.Value.Raw()),
			},
			query.Key{
				Path:   path,
				Filter: query.Not{Filter: query.Exists{}},
			},
		}
	}
	return query.Key{
		Path:   path,
		Filter: query.NewComp(op, e.Value.Raw()),
	}
}

func (e FilterEq) FilterObject(g *domain.Details) bool {
	return e.filterObject(g.Get(domain.RelationKey(e.Key)))
}

func (e FilterEq) filterObject(v domain.Value) bool {
	// if list := v.GetListValue(); list != nil && e.Value.GetListValue() == nil {
	isFilterValueScalar := !e.Value.IsStringList() && !e.Value.IsFloatList()
	if isFilterValueScalar {
		if list, ok := v.FloatList(); ok {
			for _, lv := range list {
				if e.filterObject(domain.Float(lv)) {
					return true
				}
			}
			return false
		}
		if list, ok := v.StringList(); ok {
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

// any
type FilterIn struct {
	Key   string
	Value *types.ListValue
}

func (i FilterIn) FilterObject(g *domain.Details) bool {
	val := g.Get(domain.RelationKey(i.Key))
	for _, v := range i.Value.Values {
		eq := FilterEq{Value: domain.ValueFromProto(v), Cond: model.BlockContentDataviewFilter_Equal}
		if eq.filterObject(val) {
			return true
		}
	}
	return false
}

func (i FilterIn) AnystoreFilter() query.Filter {
	path := []string{i.Key}
	conds := make([]query.Filter, 0, len(i.Value.GetValues()))
	for _, v := range i.Value.GetValues() {
		conds = append(conds, query.Key{
			Path:   path,
			Filter: query.NewComp(query.CompOpEq, pbtypes.ProtoToAny(v)),
		})
	}
	return query.Or(conds)
}

type FilterLike struct {
	Key   string
	Value string
}

func (l FilterLike) FilterObject(g *domain.Details) bool {
	val, ok := g.TryString(domain.RelationKey(l.Key))
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
		Path: []string{l.Key},
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
				EqValue: encoding.AppendJSONValue(nil, fastjson.MustParse(`[]`)),
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
		// String list
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
			val, ok := g.TryFloat(l.Key)
			if ok && len(l.Floats) == 1 {
				return l.Floats[0] == val
			}
		}
		// Float list
		{
			val, ok := g.TryFloatList(l.Key)
			if ok {
				return lo.Every(val, l.Floats)
			}
		}
		return false
	}
	return true
}

func (l FilterAllIn) AnystoreFilter() query.Filter {
	path := []string{string(l.Key)}
	conds := make([]query.Filter, 0, len(l.Strings)+len(l.Floats))
	for _, v := range l.Strings {
		conds = append(conds, query.Key{
			Path:   path,
			Filter: query.NewComp(query.CompOpEq, v),
		})
	}
	for _, v := range l.Floats {
		conds = append(conds, query.Key{
			Path:   path,
			Filter: query.NewComp(query.CompOpEq, v),
		})
	}
	return query.And(conds)
}

type FilterOptionsEqual struct {
	Key     domain.RelationKey
	Value   []string
	Options map[string]string
}

func (exIn FilterOptionsEqual) FilterObject(g *domain.Details) bool {
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

func (exIn FilterOptionsEqual) AnystoreFilter() query.Filter {
	path := []string{string(exIn.Key)}
	conds := make([]query.Filter, 0, len(exIn.Value)+1)
	conds = append(conds, query.Key{
		Path:   path,
		Filter: query.Size{Size: int64(len(exIn.Value))},
	})
	for _, v := range exIn.Value {
		conds = append(conds, query.Key{
			Path:   path,
			Filter: query.NewComp(query.CompOpEq, v),
		})
	}
	return query.And(conds)
}

func optionsToMap(spaceID string, key domain.RelationKey, store ObjectStore) map[string]string {
	result := make(map[string]string)
	options, err := ListRelationOptions(store, spaceID, key)
	if err != nil {
		log.Warn("nil objectStore for getting options")
		return result
	}
	for _, opt := range options {
		result[opt.Id] = opt.Text
	}

	return result
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

func makeFilterNestedIn(spaceID string, rawFilter FilterRequest, store ObjectStore, relationKey domain.RelationKey, nestedRelationKey domain.RelationKey) (Filter, error) {
	rawNestedFilter := rawFilter
	rawNestedFilter.RelationKey = string(nestedRelationKey)
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
	path := []string{string(i.Key)}
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
