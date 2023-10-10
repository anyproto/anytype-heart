package database

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

var (
	ErrValueMustBeListSupporting = errors.New("value must be list supporting")
)

func MakeFiltersAnd(protoFilters []*model.BlockContentDataviewFilter, store ObjectStore) (FiltersAnd, error) {
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

func MakeFilter(spaceID string, rawFilter *model.BlockContentDataviewFilter, store ObjectStore) (Filter, error) {
	if store == nil {
		return nil, fmt.Errorf("objectStore dependency is nil")
	}
	parts := strings.SplitN(rawFilter.RelationKey, ".", 2)
	if len(parts) == 2 {
		return makeFilterNestedIn(spaceID, rawFilter, store, parts[0], parts[1])
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
	switch rawFilter.Condition {
	case model.BlockContentDataviewFilter_Equal,
		model.BlockContentDataviewFilter_Greater,
		model.BlockContentDataviewFilter_Less,
		model.BlockContentDataviewFilter_GreaterOrEqual,
		model.BlockContentDataviewFilter_LessOrEqual:
		return FilterEq{
			Key:   rawFilter.RelationKey,
			Cond:  rawFilter.Condition,
			Value: rawFilter.Value,
		}, nil
	case model.BlockContentDataviewFilter_NotEqual:
		return FilterNot{FilterEq{
			Key:   rawFilter.RelationKey,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: rawFilter.Value,
		}}, nil
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
		return FilterExactIn{
			Key:     rawFilter.RelationKey,
			Value:   list,
			Options: optionsToMap(spaceID, rawFilter.RelationKey, store),
		}, nil
	case model.BlockContentDataviewFilter_NotExactIn:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return FilterNot{FilterExactIn{
			Key:   rawFilter.RelationKey,
			Value: list,
		}}, nil
	case model.BlockContentDataviewFilter_Exists:
		return FilterExists{
			Key: rawFilter.RelationKey,
		}, nil
	default:
		return nil, fmt.Errorf("unexpected filter cond: %v", rawFilter.Condition)
	}
}

type Getter interface {
	Get(key string) *types.Value
}

type WithNestedFilter interface {
	IterateNestedFilters(func(nestedFilter Filter) error) error
}

type Filter interface {
	FilterObject(g Getter) bool
	String() string
}

type FiltersAnd []Filter

var _ WithNestedFilter = FiltersAnd{}

func (a FiltersAnd) FilterObject(g Getter) bool {
	for _, f := range a {
		if !f.FilterObject(g) {
			return false
		}
	}
	return true
}

func (a FiltersAnd) String() string {
	var andS []string
	for _, f := range a {
		andS = append(andS, f.String())
	}
	return fmt.Sprintf("(%s)", strings.Join(andS, " AND "))
}

func (a FiltersAnd) IterateNestedFilters(fn func(nestedFilter Filter) error) error {
	return iterateNestedFilters(a, fn)
}

type FiltersOr []Filter

var _ WithNestedFilter = FiltersOr{}

func (fo FiltersOr) FilterObject(g Getter) bool {
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

func (fo FiltersOr) String() string {
	var orS []string
	for _, f := range fo {
		orS = append(orS, f.String())
	}
	return fmt.Sprintf("(%s)", strings.Join(orS, " OR "))
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
	Filter
}

func (n FilterNot) FilterObject(g Getter) bool {
	if n.Filter == nil {
		return false
	}
	return !n.Filter.FilterObject(g)
}

func (n FilterNot) String() string {
	return fmt.Sprintf("NOT(%s)", n.Filter.String())
}

type FilterEq struct {
	Key   string
	Cond  model.BlockContentDataviewFilterCondition
	Value *types.Value
}

func (e FilterEq) FilterObject(g Getter) bool {
	return e.filterObject(g.Get(e.Key))
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
	}
	return false
}

func (e FilterEq) String() string {
	var eq string
	switch e.Cond {
	case model.BlockContentDataviewFilter_Equal:
		eq = "="
	case model.BlockContentDataviewFilter_Greater:
		eq = ">"
	case model.BlockContentDataviewFilter_GreaterOrEqual:
		eq = ">="
	case model.BlockContentDataviewFilter_Less:
		eq = "<"
	case model.BlockContentDataviewFilter_LessOrEqual:
		eq = "<="
	}
	return fmt.Sprintf("%s %s '%s'", e.Key, eq, pbtypes.Sprint(e.Value))
}

type FilterIn struct {
	Key   string
	Value *types.ListValue
}

func (i FilterIn) FilterObject(g Getter) bool {
	val := g.Get(i.Key)
	for _, v := range i.Value.Values {
		eq := FilterEq{Value: v, Cond: model.BlockContentDataviewFilter_Equal}
		if eq.filterObject(val) {
			return true
		}
	}
	return false
}

func (i FilterIn) String() string {
	return fmt.Sprintf("%v IN(%v)", i.Key, pbtypes.Sprint(i.Value))
}

type FilterLike struct {
	Key   string
	Value *types.Value
}

func (l FilterLike) FilterObject(g Getter) bool {
	val := g.Get(l.Key)
	if val == nil {
		return false
	}
	valStr := val.GetStringValue()
	if valStr == "" {
		return false
	}
	return strings.Contains(strings.ToLower(valStr), strings.ToLower(l.Value.GetStringValue()))
}

func (l FilterLike) String() string {
	return fmt.Sprintf("%v LIKE '%s'", l.Key, pbtypes.Sprint(l.Value))
}

type FilterExists struct {
	Key string
}

func (e FilterExists) FilterObject(g Getter) bool {
	val := g.Get(e.Key)
	if val == nil {
		return false
	}

	return true
}

func (e FilterExists) String() string {
	return fmt.Sprintf("%v EXISTS", e.Key)
}

type FilterEmpty struct {
	Key string
}

func (e FilterEmpty) FilterObject(g Getter) bool {
	val := g.Get(e.Key)
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

func (e FilterEmpty) String() string {
	return fmt.Sprintf("%v IS EMPTY", e.Key)
}

type FilterAllIn struct {
	Key   string
	Value *types.ListValue
}

func (l FilterAllIn) FilterObject(g Getter) bool {
	val := g.Get(l.Key)
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

func (l FilterAllIn) String() string {
	return fmt.Sprintf("%s ALLIN(%v)", l.Key, l.Value)
}

type FilterExactIn struct {
	Key     string
	Value   *types.ListValue
	Options map[string]string
}

func (exIn FilterExactIn) FilterObject(g Getter) bool {
	val := g.Get(exIn.Key)
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

func (exIn FilterExactIn) String() string {
	return fmt.Sprintf("%s EXACTINN(%v)", exIn.Key, exIn.Value)
}

func optionsToMap(spaceID string, key string, store ObjectStore) map[string]string {
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
	Key                    string
	FilterForNestedObjects Filter

	IDs []string
}

var _ WithNestedFilter = &FilterNestedIn{}

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

func (i *FilterNestedIn) FilterObject(g Getter) bool {
	val := g.Get(i.Key)
	for _, id := range i.IDs {
		eq := FilterEq{Value: pbtypes.String(id), Cond: model.BlockContentDataviewFilter_Equal}
		if eq.filterObject(val) {
			return true
		}
	}
	return false
}

func (i *FilterNestedIn) String() string {
	return fmt.Sprintf("%v IN(%v)", i.Key, i.IDs)
}

func (i *FilterNestedIn) IterateNestedFilters(fn func(nestedFilter Filter) error) error {
	return fn(i)
}
