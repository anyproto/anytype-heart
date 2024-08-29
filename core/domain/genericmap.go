package domain

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gogo/protobuf/types"
	"golang.org/x/exp/constraints"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (d *GenericMap[K]) Len() int {
	if d == nil {
		return 0
	}
	return len(d.data)
}

func (d *GenericMap[K]) Set(key K, value Value) {
	// TODO Convert number value to float, convert number list value to floats
	d.data[key] = value
}

// TODO Return itself in case someone want to use chaining
func (d *GenericMap[K]) SetBool(key K, value bool) {
	d.data[key] = Bool(value)
}

func (d *GenericMap[K]) SetString(key K, value string) *GenericMap[K] {
	d.data[key] = String(value)
	return d
}

func (d *GenericMap[K]) SetInt64(key K, value int64) {
	d.data[key] = Int64(value)
}

func (d *GenericMap[K]) SetFloat(key K, value float64) {
	d.data[key] = Float64(value)
}

func (d *GenericMap[K]) SetStringList(key K, value []string) {
	d.data[key] = StringList(value)
}

func (d *GenericMap[K]) SetFloatList(key K, value []float64) {
	d.data[key] = Float64List(value)
}

func (d *GenericMap[K]) SetProtoValue(key K, value *types.Value) {
	d.Set(key, ValueFromProto(value))
}

func (d *GenericMap[K]) Delete(key K) {
	delete(d.data, key)
}

func (d *GenericMap[K]) Keys() []K {
	if d == nil {
		return nil
	}
	keys := make([]K, 0, len(d.data))
	for k := range d.data {
		keys = append(keys, k)
	}
	return keys
}

func (d *GenericMap[K]) Iterate(proc func(key K, value Value) bool) {
	if d == nil {
		return
	}
	for k, v := range d.data {
		if !proc(k, v) {
			return
		}
	}
}

func (d *GenericMap[K]) IterateSorted(proc func(key K, value Value) bool) {
	if d == nil {
		return
	}

	keys := make([]K, 0, len(d.data))
	for k := range d.data {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	for _, k := range keys {
		v := d.data[k]
		if !proc(k, v) {
			return
		}
	}
}

func (d *GenericMap[K]) GetRaw(key K) (any, bool) {
	v, ok := d.data[key]
	return v, ok
}

func (d *GenericMap[K]) Get(key K) Value {
	if d == nil {
		return Value{}
	}
	// Empty Value is ok to use
	return d.data[key]
}

func (d *GenericMap[K]) TryGet(key K) (Value, bool) {
	if d == nil {
		return Value{}, false
	}
	// Empty Value is ok to use
	v, ok := d.data[key]
	return v, ok
}

func (d *GenericMap[K]) Has(key K) bool {
	if d == nil {
		return false
	}
	_, ok := d.data[key]
	return ok
}

func (d *GenericMap[K]) TryBool(key K) (bool, bool) {
	return d.Get(key).TryBool()
}

func (d *GenericMap[K]) GetBool(key K) bool {
	return d.Get(key).Bool()
}

func (d *GenericMap[K]) TryString(key K) (string, bool) {
	return d.Get(key).TryString()
}

func (d *GenericMap[K]) GetString(key K) string {
	return d.Get(key).String()
}

func (d *GenericMap[K]) TryInt64(key K) (int64, bool) {
	return d.Get(key).TryInt64()
}

func (d *GenericMap[K]) GetInt64(key K) int64 {
	return d.Get(key).Int64()
}

func (d *GenericMap[K]) TryFloat(key K) (float64, bool) {
	return d.Get(key).TryFloat64()
}

func (d *GenericMap[K]) GetFloat(key K) float64 {
	return d.Get(key).Float64()
}

func (d *GenericMap[K]) TryStringList(key K) ([]string, bool) {
	return d.Get(key).TryStringList()
}

// TODO StringList in pbtypes return []string{singleValue} for string values
func (d *GenericMap[K]) GetStringList(key K) []string {
	return d.Get(key).StringList()
}

func (d *GenericMap[K]) TryFloatList(key K) ([]float64, bool) {
	return d.Get(key).TryFloat64List()
}

func (d *GenericMap[K]) GetFloatList(key K) []float64 {
	return d.Get(key).Float64List()
}

func (d *GenericMap[K]) TryInt64List(key K) ([]int64, bool) {
	return d.Get(key).TryInt64List()
}

func (d *GenericMap[K]) GetInt64List(key K) []int64 {
	return d.Get(key).Int64List()
}

func (d *GenericMap[K]) Copy() *GenericMap[K] {
	if d == nil {
		return nil
	}
	newData := make(map[K]Value, len(d.data))
	for k, v := range d.data {
		newData[k] = v
	}
	return &GenericMap[K]{data: newData}
}

func (d *GenericMap[K]) CopyWithoutKeys(keys ...K) *GenericMap[K] {
	if d == nil {
		return nil
	}
	newData := make(map[K]Value, len(d.data))
	for k, v := range d.data {
		if !slices.Contains(keys, k) {
			newData[k] = v
		}
	}
	return &GenericMap[K]{data: newData}
}

func (d *GenericMap[K]) CopyOnlyKeys(keys ...K) *GenericMap[K] {
	if d == nil {
		return nil
	}
	newData := make(map[K]Value, len(d.data))
	for k, v := range d.data {
		if slices.Contains(keys, k) {
			newData[k] = v
		}
	}
	return &GenericMap[K]{data: newData}
}

func (d *GenericMap[K]) Equal(other *GenericMap[K]) bool {
	if d == nil && other == nil {
		return true
	}
	if d == nil || other == nil {
		return false
	}
	if d.Len() != other.Len() {
		return false
	}
	for k, v := range d.data {
		otherV, ok := other.data[k]
		if !ok {
			return false
		}
		if !v.Equal(otherV) {
			return false
		}
	}
	return true
}

func (d *GenericMap[K]) Merge(other *GenericMap[K]) *GenericMap[K] {
	if d == nil {
		return other.Copy()
	}
	res := d.Copy()
	other.Iterate(func(k K, v Value) bool {
		res.Set(k, v)
		return true
	})
	return res
}

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

func Int64List[T constraints.Integer](v ...T) Value {
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

func (v Value) Raw() any {
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
		// TODO Not implemented
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

func (v Value) IsFloatList() bool {
	if !v.ok {
		return false
	}
	_, ok := v.value.([]float64)
	return ok
}

func (v Value) Null() bool {
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

func (v Value) TryInt64() (int64, bool) {
	if !v.ok {
		return 0, false
	}
	switch v := v.value.(type) {
	case float64:
		return int64(v), true
	default:
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
	return l, ok
}

func (v Value) StringList() []string {
	res, ok := v.TryStringList()
	if !ok {
		return nil
	}
	return res
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
	return l, ok
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
	if v, ok := v.TryStringList(); ok {
		res := make([]Value, 0, len(v))
		for _, s := range v {
			res = append(res, String(s))
		}
		return res, nil
	}
	if v, ok := v.TryFloat64List(); ok {
		res := make([]Value, 0, len(v))
		for _, f := range v {
			res = append(res, Float64(f))
		}
		return res, nil
	}
	return nil, fmt.Errorf("unsupported type: %v", v.Type())
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
		return pbtypes.FloatList(v)
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
		ok := v.Null()
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
			if v1 && v2 {
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

	return 0
}

func (v Value) Equal(other Value) bool {
	if v.ok != other.ok {
		return false
	}
	if !v.ok {
		return true
	}

	if v.Null() && other.Null() {
		return true
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

	return false
}

func (v Value) Match(nullCase func(), boolCase func(v bool), floatCase func(v float64), stringCase func(v string), stringListCase func(v []string), floatListCase func(v []float64)) {
	if !v.ok {
		return
	}
	switch v := v.value.(type) {
	case nullValue:
		nullCase()
	case bool:
		boolCase(v)
	case float64:
		floatCase(v)
	case string:
		stringCase(v)
	case []string:
		stringListCase(v)
	case []float64:
		floatListCase(v)
	}
}

// TODO Refactor, maybe remove Match function
func (v Value) IsEmpty() bool {
	if !v.ok {
		return true
	}
	var ok bool
	v.Match(
		func() {
			ok = true
		},
		func(v bool) {
			ok = !v
		}, func(v float64) {
			ok = v == 0
		}, func(v string) {
			ok = v == ""
		}, func(v []string) {
			ok = len(v) == 0
		}, func(v []float64) {
			ok = len(v) == 0
		})
	return ok
}
