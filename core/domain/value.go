package domain

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/anyproto/any-store/anyenc"
	"golang.org/x/exp/constraints"
	types "google.golang.org/protobuf/types/known/structpb"

	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type nullValue struct{}

type Value struct {
	ok    bool
	value any
}

type ValueType int

const (
	ValueTypeNone ValueType = iota
	ValueTypeNull
	ValueTypeBool
	ValueTypeString
	ValueTypeFloat
	ValueTypeStringList
	ValueTypeFloatList
	ValueTypeMap = 7
)

func Invalid() Value {
	return Value{ok: false}
}

func Null() Value {
	return Value{ok: true, value: nullValue{}}
}

func Int64[T constraints.Integer](v T) Value {
	return Value{ok: true, value: float64(v)}
}

func Int64List[T constraints.Integer](v []T) Value {
	conv := make([]float64, len(v))
	for i, val := range v {
		conv[i] = float64(val)
	}
	return Value{ok: true, value: conv}
}

func Float64(v float64) Value {
	return Value{ok: true, value: v}
}

func Bool(v bool) Value {
	return Value{ok: true, value: v}
}

func String[T ~string](v T) Value {
	return Value{ok: true, value: string(v)}
}

func StringList(v []string) Value {
	return Value{ok: true, value: v}
}

func Float64List(v []float64) Value {
	return Value{ok: true, value: v}
}

func ValueList(vs []Value) Value {
	// Prefer string list
	if len(vs) == 0 {
		return StringList(nil)
	}
	if vs[0].IsString() {
		strs := make([]string, 0, len(vs))
		for _, v := range vs {
			strs = append(strs, v.String())
		}
		return StringList(strs)
	}
	if vs[0].IsFloat64() {
		floats := make([]float64, 0, len(vs))
		for _, v := range vs {
			floats = append(floats, v.Float64())
		}
		return Float64List(floats)
	}
	return StringList(nil)
}

type ValueMap = *GenericMap[string]

func NewValueMap(m map[string]Value) Value {
	vmap := &GenericMap[string]{data: m}
	return Value{ok: true, value: vmap}
}

func (v Value) Raw() any {
	_, ok := v.value.(nullValue)
	if ok {
		return nil
	}
	return v.value
}

func ValueFromProto(value *types.Value) Value {
	if value == nil {
		return Null()
	}
	switch value.Kind.(type) {
	case *types.Value_NullValue:
		return Null()
	case *types.Value_BoolValue:
		return Bool(value.GetBoolValue())
	case *types.Value_StringValue:
		return String(value.GetStringValue())
	case *types.Value_NumberValue:
		return Float64(value.GetNumberValue())
	case *types.Value_StructValue:
		val := value.GetStructValue()
		m := make(map[string]Value, len(val.GetFields()))
		for k, v := range val.GetFields() {
			m[k] = ValueFromProto(v)
		}
		return NewValueMap(m)
	case *types.Value_ListValue:
		if list := pbtypes.ListValueToFloats(value.GetListValue()); len(list) > 0 {
			return Float64List(list)
		}
		return StringList(pbtypes.ListValueToStrings(value.GetListValue()))
	}
	return Null()
}

func (v Value) Ok() bool {
	return v.ok
}

func (v Value) IsStringList() bool {
	if !v.ok {
		return false
	}
	_, ok := v.value.([]string)
	return ok
}

func (v Value) IsFloat64List() bool {
	if !v.ok {
		return false
	}
	_, ok := v.value.([]float64)
	return ok
}

func (v Value) IsNull() bool {
	if !v.ok {
		return false
	}
	_, ok := v.value.(nullValue)
	return ok
}

func (v Value) IsBool() bool {
	if !v.ok {
		return false
	}
	_, ok := v.value.(bool)
	return ok
}

func (v Value) TryBool() (bool, bool) {
	if !v.ok {
		return false, false
	}
	b, ok := v.value.(bool)
	if !ok {
		return false, false
	}
	return b, true
}

func (v Value) Bool() bool {
	res, ok := v.TryBool()
	if !ok {
		return false
	}
	return res
}

func (v Value) IsString() bool {
	if !v.ok {
		return false
	}
	_, ok := v.value.(string)
	return ok
}

func (v Value) TryString() (string, bool) {
	if !v.ok {
		return "", false
	}
	s, ok := v.value.(string)
	return s, ok
}

func (v Value) String() string {
	res, ok := v.TryString()
	if !ok {
		return ""
	}
	return res
}

func (v Value) IsInt64() bool {
	return v.IsFloat64()
}

func (v Value) TryInt64() (int64, bool) {
	res, ok := v.TryFloat64()
	if ok {
		return int64(res), true
	} else {
		return 0, false
	}
}

func (v Value) Int64() int64 {
	res, ok := v.TryInt64()
	if !ok {
		return 0
	}
	return res
}

func (v Value) IsFloat64() bool {
	if !v.ok {
		return false
	}
	_, ok := v.value.(float64)
	return ok
}

func (v Value) TryFloat64() (float64, bool) {
	if !v.ok {
		return 0, false
	}
	switch v := v.value.(type) {
	case float64:
		return v, true
	default:
		return 0, false
	}
}

func (v Value) Float64() float64 {
	res, ok := v.TryFloat64()
	if !ok {
		return 0
	}
	return res
}

func (v Value) TryStringList() ([]string, bool) {
	if !v.ok {
		return nil, false
	}
	l, ok := v.value.([]string)
	return slices.Clone(l), ok
}

func (v Value) StringList() []string {
	res, ok := v.TryStringList()
	if !ok {
		return nil
	}
	return res
}

func (v Value) TryWrapToStringList() ([]string, bool) {
	res, ok := v.TryStringList()
	if ok {
		return res, true
	}
	s, ok := v.TryString()
	if ok {
		if s == "" {
			return []string{}, true
		} else {
			return []string{s}, true
		}
	}
	return nil, false
}

func (v Value) WrapToStringList() []string {
	res, ok := v.TryWrapToStringList()
	if ok {
		return res
	}
	return nil
}

func (v Value) IsInt64List() bool {
	return v.IsFloat64List()
}

func (v Value) TryInt64List() ([]int64, bool) {
	if !v.ok {
		return nil, false
	}
	l, ok := v.value.([]float64)
	if !ok {
		return nil, false
	}
	res := make([]int64, len(l))
	for i, v := range l {
		res[i] = int64(v)
	}
	return res, true
}

func (v Value) Int64List() []int64 {
	res, ok := v.TryInt64List()
	if !ok {
		return nil
	}
	return res
}

func (v Value) TryFloat64List() ([]float64, bool) {
	if !v.ok {
		return nil, false
	}
	l, ok := v.value.([]float64)
	return slices.Clone(l), ok
}

func (v Value) Float64List() []float64 {
	res, ok := v.TryFloat64List()
	if !ok {
		return nil
	}
	return res
}

func (v Value) WrapToList() []Value {
	list, err := v.TryWrapToList()
	if err != nil {
		return nil
	}
	return list
}

func (v Value) TryWrapToList() ([]Value, error) {
	if v, ok := v.TryString(); ok {
		return []Value{String(v)}, nil
	}
	if v, ok := v.TryFloat64(); ok {
		return []Value{Float64(v)}, nil
	}

	list, ok := v.TryListValues()
	if ok {
		return list, nil
	}
	return nil, fmt.Errorf("unsupported type: %v", v.Type())
}

func (v Value) TryListValues() ([]Value, bool) {
	if v, ok := v.TryStringList(); ok {
		res := make([]Value, 0, len(v))
		for _, s := range v {
			res = append(res, String(s))
		}
		return res, true
	}
	if v, ok := v.TryFloat64List(); ok {
		res := make([]Value, 0, len(v))
		for _, f := range v {
			res = append(res, Float64(f))
		}
		return res, true
	}
	return nil, false
}

func (v Value) IsMapValue() bool {
	if !v.ok {
		return false
	}
	_, ok := v.value.(ValueMap)
	return ok
}

func (v Value) MapValue() ValueMap {
	if !v.ok {
		return nil
	}
	m, ok := v.value.(ValueMap)
	if !ok {
		return nil
	}
	return m
}

func (v Value) TryMapValue() (ValueMap, bool) {
	if !v.ok {
		return nil, false
	}
	m, ok := v.value.(ValueMap)
	return m, ok
}

func (v Value) Type() ValueType {
	if !v.ok {
		return ValueTypeNone
	}
	switch v.value.(type) {
	case nullValue:
		return ValueTypeNull
	case bool:
		return ValueTypeBool
	case string:
		return ValueTypeString
	case float64:
		return ValueTypeFloat
	case []string:
		return ValueTypeStringList
	case []float64:
		return ValueTypeFloatList
	case ValueMap:
		return ValueTypeMap
	default:
		return ValueTypeNone
	}
}

func (v Value) ToProto() *types.Value {
	if !v.ok {
		return pbtypes.Null()
	}
	switch v := v.value.(type) {
	case nullValue:
		return pbtypes.Null()
	case bool:
		return pbtypes.Bool(v)
	case string:
		return pbtypes.String(v)
	case float64:
		return pbtypes.Float64(v)
	case []string:
		return pbtypes.StringList(v)
	case []float64:
		return pbtypes.Float64List(v)
	case ValueMap:
		s := &types.Struct{Fields: make(map[string]*types.Value, v.Len())}
		for k, val := range v.Iterate() {
			s.Fields[k] = val.ToProto()
		}
		return pbtypes.Struct(s)
	default:
		panic("integrity violation")
	}
}

func (v Value) Compare(other Value) int {
	if !v.ok && other.ok {
		return -1
	}
	if v.ok && !other.ok {
		return 1
	}
	if !v.ok {
		return 0
	}

	if v.Type() < other.Type() {
		return -1
	}
	if v.Type() > other.Type() {
		return 1
	}

	{
		// Two null values are always equal
		ok := v.IsNull()
		if ok {
			return 0
		}
	}

	{
		v1, ok := v.TryBool()
		v2, _ := other.TryBool()
		if ok {
			if !v1 && v2 {
				return -1
			}
			if v1 == v2 {
				return 0
			}
			return 1
		}
	}

	{
		v1, ok := v.TryString()
		v2, _ := other.TryString()
		if ok {
			return strings.Compare(v1, v2)
		}
	}

	{
		v1, ok := v.TryFloat64()
		v2, _ := other.TryFloat64()
		if ok {
			if v1 < v2 {
				return -1
			}
			if v1 > v2 {
				return 1
			}
			return 0
		}
	}

	{
		v1, ok := v.TryStringList()
		v2, _ := other.TryStringList()
		if ok {
			return slices.Compare(v1, v2)
		}
	}

	{
		v1, ok := v.TryFloat64List()
		v2, _ := other.TryFloat64List()
		if ok {
			return slices.Compare(v1, v2)
		}
	}

	{
		v1, ok := v.TryMapValue()
		v2, _ := other.TryMapValue()
		if ok {
			return compareMaps(v1, v2)
		}
	}

	return 0
}

func compareMaps(a, b ValueMap) int {
	if a.Len() < b.Len() {
		return -1
	} else if a.Len() > b.Len() {
		return 1
	}

	keysA := a.Keys()
	keysB := b.Keys()
	sort.Strings(keysA)
	sort.Strings(keysB)

	// keys first
	if res := slices.Compare(keysA, keysB); res != 0 {
		return res
	}

	for _, k := range keysA {
		if res := a.Get(k).Compare(b.Get(k)); res != 0 {
			return res
		}
	}

	return 0
}

func (v Value) Equal(other Value) bool {
	if v.ok != other.ok {
		return false
	}
	if !v.ok {
		return true
	}

	if v.IsNull() && other.IsNull() {
		return true
	}

	if v.Type() != other.Type() {
		return false
	}

	{
		v1, ok1 := v.TryBool()
		v2, ok2 := other.TryBool()
		if ok1 != ok2 {
			return false
		}
		if ok1 {
			return v1 == v2
		}
	}

	{
		v1, ok1 := v.TryString()
		v2, ok2 := other.TryString()
		if ok1 != ok2 {
			return false
		}
		if ok1 {
			return v1 == v2
		}
	}

	{
		v1, ok1 := v.TryFloat64()
		v2, ok2 := other.TryFloat64()
		if ok1 != ok2 {
			return false
		}
		if ok1 {
			return v1 == v2
		}
	}

	{
		v1, ok1 := v.TryStringList()
		v2, ok2 := other.TryStringList()
		if ok1 != ok2 {
			return false
		}
		if ok1 {
			if len(v1) != len(v2) {
				return false
			}
			return slices.Equal(v1, v2)
		}
	}

	{
		v1, ok1 := v.TryFloat64List()
		v2, ok2 := other.TryFloat64List()
		if ok1 != ok2 {
			return false
		}
		if ok1 {
			if len(v1) != len(v2) {
				return false
			}
			return slices.Equal(v1, v2)
		}
	}

	{
		v1, ok := v.TryMapValue()
		v2, _ := other.TryMapValue()
		if ok {
			return v1.Equal(v2)
		}
	}

	return false
}

type ValueMatcher struct {
	Null        func()
	Bool        func(v bool)
	Float64     func(v float64)
	Int64       func(v int64)
	String      func(v string)
	StringList  func(v []string)
	Float64List func(v []float64)
	Int64List   func(v []int64)
	MapValue    func(valueMap ValueMap)
}

func (v Value) Match(matcher ValueMatcher) {
	if !v.ok {
		return
	}
	if v.IsNull() && matcher.Null != nil {
		matcher.Null()
	}
	if v.IsBool() && matcher.Bool != nil {
		matcher.Bool(v.Bool())
	}
	if v.IsFloat64() && matcher.Float64 != nil {
		matcher.Float64(v.Float64())
	}
	if v.IsInt64() && matcher.Int64 != nil {
		matcher.Int64(v.Int64())
	}
	if v.IsString() && matcher.String != nil {
		matcher.String(v.String())
	}
	if v.IsStringList() && matcher.StringList != nil {
		matcher.StringList(v.StringList())
	}
	if v.IsFloat64List() && matcher.Float64List != nil {
		matcher.Float64List(v.Float64List())
	}
	if v.IsInt64List() && matcher.Int64List != nil {
		matcher.Int64List(v.Int64List())
	}
	if v.IsMapValue() && matcher.MapValue != nil {
		matcher.MapValue(v.MapValue())
	}
}

func (v Value) IsEmpty() bool {
	if !v.ok {
		return true
	}
	var ok bool
	v.Match(ValueMatcher{
		Null: func() {
			ok = true
		},
		Bool: func(v bool) {
			ok = !v
		},
		Float64: func(v float64) {
			ok = v == 0
		},
		String: func(v string) {
			ok = v == ""
		},
		StringList: func(v []string) {
			ok = len(v) == 0
		},
		Float64List: func(v []float64) {
			ok = len(v) == 0
		},
		MapValue: func(v ValueMap) {
			ok = v.Len() == 0
		},
	})
	return ok
}

func (v Value) ToAnyEnc(arena *anyenc.Arena) *anyenc.Value {
	switch v := v.value.(type) {
	case nullValue:
		return arena.NewNull()
	case string:
		return arena.NewString(v)
	case float64:
		return arena.NewNumberFloat64(v)
	case bool:
		if v {
			return arena.NewTrue()
		} else {
			return arena.NewFalse()
		}
	case []string:
		lst := arena.NewArray()
		for i, it := range v {
			lst.SetArrayItem(i, arena.NewString(it))
		}
		return lst
	case []float64:
		lst := arena.NewArray()
		for i, it := range v {
			lst.SetArrayItem(i, arena.NewNumberFloat64(it))
		}
		return lst
	default:
		return arena.NewNull()
	}
}
