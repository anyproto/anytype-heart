package filter

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

var (
	ErrValueMustBeListSupporting = errors.New("value must be list supporting")
)

func MakeAndFilter(protoFilters []*model.BlockContentDataviewFilter, store OptionsGetter) (AndFilters, error) {

	protoFilters = TransformQuickOption(protoFilters, nil)

	var and AndFilters
	for _, pf := range protoFilters {
		if pf.Condition != model.BlockContentDataviewFilter_None {
			f, err := MakeFilter(pf, store)
			if err != nil {
				return nil, err
			}
			and = append(and, f)
		}
	}
	return and, nil
}

func MakeFilter(rawFilter *model.BlockContentDataviewFilter, store OptionsGetter) (Filter, error) {
	parts := strings.SplitN(rawFilter.RelationKey, ".", 2)
	if len(parts) == 2 {
		rawNestedFilter := proto.Clone(rawFilter).(*model.BlockContentDataviewFilter)
		rawNestedFilter.RelationKey = parts[1]
		nestedFilter, err := MakeFilter(rawNestedFilter, store)
		if err != nil {
			return nil, fmt.Errorf("make nested filter %s: %w", parts, err)
		}
		return &NestedIn{
			Key:    parts[0],
			Filter: nestedFilter,
		}, nil
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
		return Eq{
			Key:   rawFilter.RelationKey,
			Cond:  rawFilter.Condition,
			Value: rawFilter.Value,
		}, nil
	case model.BlockContentDataviewFilter_NotEqual:
		return Not{Eq{
			Key:   rawFilter.RelationKey,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: rawFilter.Value,
		}}, nil
	case model.BlockContentDataviewFilter_Like:
		return Like{
			Key:   rawFilter.RelationKey,
			Value: rawFilter.Value,
		}, nil
	case model.BlockContentDataviewFilter_NotLike:
		return Not{Like{
			Key:   rawFilter.RelationKey,
			Value: rawFilter.Value,
		}}, nil
	case model.BlockContentDataviewFilter_In:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return In{
			Key:   rawFilter.RelationKey,
			Value: list,
		}, nil
	case model.BlockContentDataviewFilter_NotIn:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return Not{In{
			Key:   rawFilter.RelationKey,
			Value: list,
		}}, nil
	case model.BlockContentDataviewFilter_Empty:
		return Empty{
			Key: rawFilter.RelationKey,
		}, nil
	case model.BlockContentDataviewFilter_NotEmpty:
		return Not{Empty{
			Key: rawFilter.RelationKey,
		}}, nil
	case model.BlockContentDataviewFilter_AllIn:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return AllIn{
			Key:   rawFilter.RelationKey,
			Value: list,
		}, nil
	case model.BlockContentDataviewFilter_NotAllIn:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return Not{AllIn{
			Key:   rawFilter.RelationKey,
			Value: list,
		}}, nil
	case model.BlockContentDataviewFilter_ExactIn:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return ExactIn{
			Key:     rawFilter.RelationKey,
			Value:   list,
			Options: optionsToMap(rawFilter.RelationKey, store),
		}, nil
	case model.BlockContentDataviewFilter_NotExactIn:
		list, err := pbtypes.ValueListWrapper(rawFilter.Value)
		if err != nil {
			return nil, ErrValueMustBeListSupporting
		}
		return Not{ExactIn{
			Key:   rawFilter.RelationKey,
			Value: list,
		}}, nil
	case model.BlockContentDataviewFilter_Exists:
		return Exists{
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
	EnrichNestedFilter(func(nestedFilter Filter) (ids []string, err error)) error
	IterateNestedFilters(func(nestedFilter Filter) error) error
}

type Filter interface {
	FilterObject(g Getter) bool
	String() string
}

type AndFilters []Filter

var _ WithNestedFilter = AndFilters{}

func (a AndFilters) FilterObject(g Getter) bool {
	for _, f := range a {
		if !f.FilterObject(g) {
			return false
		}
	}
	return true
}

func (a AndFilters) String() string {
	var andS []string
	for _, f := range a {
		andS = append(andS, f.String())
	}
	return fmt.Sprintf("(%s)", strings.Join(andS, " AND "))
}

func (a AndFilters) EnrichNestedFilter(fn func(nestedFilter Filter) (ids []string, err error)) error {
	for _, f := range a {
		if withNested, ok := f.(WithNestedFilter); ok {
			if err := withNested.EnrichNestedFilter(fn); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a AndFilters) IterateNestedFilters(fn func(nestedFilter Filter) error) error {
	for _, f := range a {
		if withNested, ok := f.(WithNestedFilter); ok {
			err := withNested.IterateNestedFilters(fn)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type OrFilters []Filter

var _ WithNestedFilter = OrFilters{}

func (a OrFilters) FilterObject(g Getter) bool {
	if len(a) == 0 {
		return true
	}
	for _, f := range a {
		if f.FilterObject(g) {
			return true
		}
	}
	return false
}

func (a OrFilters) String() string {
	var orS []string
	for _, f := range a {
		orS = append(orS, f.String())
	}
	return fmt.Sprintf("(%s)", strings.Join(orS, " OR "))
}

func (a OrFilters) EnrichNestedFilter(fn func(nestedFilter Filter) (ids []string, err error)) error {
	for _, f := range a {
		if withNested, ok := f.(WithNestedFilter); ok {
			if err := withNested.EnrichNestedFilter(fn); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a OrFilters) IterateNestedFilters(fn func(nestedFilter Filter) error) error {
	for _, f := range a {
		if withNested, ok := f.(WithNestedFilter); ok {
			err := withNested.IterateNestedFilters(fn)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type Not struct {
	Filter
}

func (n Not) FilterObject(g Getter) bool {
	if n.Filter == nil {
		return false
	}
	return !n.Filter.FilterObject(g)
}

func (n Not) String() string {
	return fmt.Sprintf("NOT(%s)", n.Filter.String())
}

type Eq struct {
	Key   string
	Cond  model.BlockContentDataviewFilterCondition
	Value *types.Value
}

func (e Eq) FilterObject(g Getter) bool {
	return e.filterObject(g.Get(e.Key))
}

func (e Eq) filterObject(v *types.Value) bool {
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

func (e Eq) String() string {
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

type In struct {
	Key   string
	Value *types.ListValue
}

func (i In) FilterObject(g Getter) bool {
	val := g.Get(i.Key)
	for _, v := range i.Value.Values {
		eq := Eq{Value: v, Cond: model.BlockContentDataviewFilter_Equal}
		if eq.filterObject(val) {
			return true
		}
	}
	return false
}

func (i In) String() string {
	return fmt.Sprintf("%v IN(%v)", i.Key, pbtypes.Sprint(i.Value))
}

type Like struct {
	Key   string
	Value *types.Value
}

func (l Like) FilterObject(g Getter) bool {
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

func (l Like) String() string {
	return fmt.Sprintf("%v LIKE '%s'", l.Key, pbtypes.Sprint(l.Value))
}

type Exists struct {
	Key string
}

func (e Exists) FilterObject(g Getter) bool {
	val := g.Get(e.Key)
	if val == nil {
		return false
	}

	return true
}

func (e Exists) String() string {
	return fmt.Sprintf("%v EXISTS", e.Key)
}

type Empty struct {
	Key string
}

func (e Empty) FilterObject(g Getter) bool {
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

func (e Empty) String() string {
	return fmt.Sprintf("%v IS EMPTY", e.Key)
}

type AllIn struct {
	Key   string
	Value *types.ListValue
}

func (l AllIn) FilterObject(g Getter) bool {
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

func (l AllIn) String() string {
	return fmt.Sprintf("%s ALLIN(%v)", l.Key, l.Value)
}

type ExactIn struct {
	Key     string
	Value   *types.ListValue
	Options map[string]string
}

func (exIn ExactIn) FilterObject(g Getter) bool {
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

func (exIn ExactIn) String() string {
	return fmt.Sprintf("%s EXACTINN(%v)", exIn.Key, exIn.Value)
}

func optionsToMap(key string, store OptionsGetter) map[string]string {
	result := make(map[string]string)
	options, err := store.GetAggregatedOptions(key)
	if err != nil {
		log.Warn("nil objectStore for getting options")
		return result
	}
	for _, opt := range options {
		result[opt.Id] = opt.Text
	}

	return result
}

type NestedIn struct {
	Key    string
	Filter Filter

	isEnriched bool
	ids        []string
}

var _ WithNestedFilter = &NestedIn{}

func (i *NestedIn) FilterObject(g Getter) bool {
	if !i.isEnriched {
		panic("nested filter is not enriched")
	}
	val := g.Get(i.Key)
	for _, id := range i.ids {
		eq := Eq{Value: pbtypes.String(id), Cond: model.BlockContentDataviewFilter_Equal}
		if eq.filterObject(val) {
			return true
		}
	}
	return false
}

func (i *NestedIn) String() string {
	return fmt.Sprintf("%v IN(%v)", i.Key, i.ids)
}

func (i *NestedIn) EnrichNestedFilter(fn func(nestedFilter Filter) (ids []string, err error)) error {
	ids, err := fn(i.Filter)
	if err != nil {
		return err
	}
	i.ids = ids
	i.isEnriched = true
	return nil
}

func (i *NestedIn) IterateNestedFilters(fn func(nestedFilter Filter) error) error {
	return fn(i)
}
