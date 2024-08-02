package domain

import (
	"errors"
	"fmt"
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
	d.data[key] = Float(value)
}

func (d *GenericMap[K]) SetStringList(key K, value []string) {
	d.data[key] = StringList(value)
}

func (d *GenericMap[K]) SetFloatList(key K, value []float64) {
	d.data[key] = FloatList(value)
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

func (d *GenericMap[K]) Has(key K) bool {
	if d == nil {
		return false
	}
	_, ok := d.data[key]
	return ok
}

func (d *GenericMap[K]) TryBool(key K) (bool, bool) {
	return d.Get(key).Bool()
}

func (d *GenericMap[K]) GetBool(key K) bool {
	return d.Get(key).BoolOrDefault(false)
}

func (d *GenericMap[K]) TryString(key K) (string, bool) {
	return d.Get(key).String()
}

func (d *GenericMap[K]) GetString(key K) string {
	return d.Get(key).StringOrDefault("")
}

func (d *GenericMap[K]) TryInt64(key K) (int64, bool) {
	return d.Get(key).Int64()
}

func (d *GenericMap[K]) GetInt64(key K) int64 {
	return d.Get(key).Int64OrDefault(0)
}

func (d *GenericMap[K]) TryFloat(key K) (float64, bool) {
	return d.Get(key).Float()
}

func (d *GenericMap[K]) GetFloat(key K) float64 {
	return d.Get(key).FloatOrDefault(0)
}

func (d *GenericMap[K]) TryStringList(key K) ([]string, bool) {
	return d.Get(key).StringList()
}

// TODO StringList in pbtypes return []string{singleValue} for string values
func (d *GenericMap[K]) GetStringList(key K) []string {
	return d.Get(key).StringListOrDefault(nil)
}

func (d *GenericMap[K]) TryFloatList(key K) ([]float64, bool) {
	return d.Get(key).FloatList()
}

func (d *GenericMap[K]) GetFloatList(key K) []float64 {
	return d.Get(key).FloatListOrDefault(nil)
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
		return nil
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

func Null() Value {
	return Value{ok: true, value: nullValue{}}
}

func Int64[T constraints.Integer](v T) Value {
	return Value{ok: true, value: float64(v)}
}

func Float(v float64) Value {
	return Value{ok: true, value: v}
}

func Bool(v bool) Value {
	return Value{ok: true, value: v}
}

func String(v string) Value {
	return Value{ok: true, value: v}
}

func StringList(v []string) Value {
	return Value{ok: true, value: v}
}

func FloatList(v []float64) Value {
	return Value{ok: true, value: v}
}

var ErrInvalidValue = fmt.Errorf("invalid value")

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
		return Float(value.GetNumberValue())
	case *types.Value_StructValue:
		// TODO Not implemented
	case *types.Value_ListValue:
		if list := pbtypes.ListValueToFloats(value.GetListValue()); len(list) > 0 {
			return FloatList(list)
		}
		return StringList(pbtypes.ListValueToStrings(value.GetListValue()))
	}
	return Null()
}

// TODO Remove, value should be always valid
func (v Value) Validate() error {
	return nil

	if !v.ok {
		return errors.Join(ErrInvalidValue, fmt.Errorf("value is none"))
	}

	// TODO USE type Null struct {}
	if v.value == nil {
		return nil
	}
	switch v.value.(type) {
	// TODO TEMPORARILY ALLOW HERE
	case int64:
		return nil
	case bool, string, float64, []string, []float64:
		return nil
	default:
		return errors.Join(ErrInvalidValue, fmt.Errorf("value is of invalid type %T", v.value))
	}
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

func (v Value) Bool() (bool, bool) {
	if !v.ok {
		return false, false
	}
	b, ok := v.value.(bool)
	if !ok {
		return false, false
	}
	return b, true
}

func (v Value) BoolOrDefault(def bool) bool {
	res, ok := v.Bool()
	if !ok {
		return def
	}
	return res
}

func (v Value) String() (string, bool) {
	if !v.ok {
		return "", false
	}
	s, ok := v.value.(string)
	return s, ok
}

func (v Value) StringOrDefault(def string) string {
	res, ok := v.String()
	if !ok {
		return def
	}
	return res
}

// TODO Store only floats?
func (v Value) Int64() (int64, bool) {
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

func (v Value) Int64OrDefault(def int64) int64 {
	res, ok := v.Int64()
	if !ok {
		return def
	}
	return res
}

func (v Value) Float() (float64, bool) {
	if !v.ok {
		return 0, false
	}
	switch v := v.value.(type) {
	case int:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

func (v Value) FloatOrDefault(def float64) float64 {
	res, ok := v.Float()
	if !ok {
		return def
	}
	return res
}

func (v Value) StringList() ([]string, bool) {
	if !v.ok {
		return nil, false
	}
	l, ok := v.value.([]string)
	return l, ok
}

func (v Value) StringListOrDefault(def []string) []string {
	res, ok := v.StringList()
	if !ok {
		return def
	}
	return res
}

// TODO Float list instead and []int only as helper
func (v Value) IntList() ([]int, bool) {
	if !v.ok {
		return nil, false
	}
	l, ok := v.value.([]int)
	return l, ok
}

func (v Value) IntListOrDefault(def []int) []int {
	res, ok := v.IntList()
	if !ok {
		return def
	}
	return res
}

func (v Value) FloatList() ([]float64, bool) {
	if !v.ok {
		return nil, false
	}
	l, ok := v.value.([]float64)
	return l, ok
}

func (v Value) FloatListOrDefault(def []float64) []float64 {
	res, ok := v.FloatList()
	if !ok {
		return def
	}
	return res
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
		v1, ok := v.Bool()
		v2, _ := other.Bool()
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
		v1, ok := v.String()
		v2, _ := other.String()
		if ok {
			return strings.Compare(v1, v2)
		}
	}

	{
		v1, ok := v.Float()
		v2, _ := other.Float()
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
		v1, ok := v.StringList()
		v2, _ := other.StringList()
		if ok {
			return slices.Compare(v1, v2)
		}
	}

	{
		v1, ok := v.IntList()
		v2, _ := other.IntList()
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

	{
		v1, ok1 := v.Bool()
		v2, ok2 := other.Bool()
		if ok1 != ok2 {
			return false
		}
		if ok1 {
			return v1 == v2
		}
	}

	{
		v1, ok1 := v.String()
		v2, ok2 := other.String()
		if ok1 != ok2 {
			return false
		}
		if ok1 {
			return v1 == v2
		}
	}

	{
		v1, ok1 := v.Float()
		v2, ok2 := other.Float()
		if ok1 != ok2 {
			return false
		}
		if ok1 {
			return v1 == v2
		}
	}

	{
		v1, ok1 := v.StringList()
		v2, ok2 := other.StringList()
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
		v1, ok1 := v.IntList()
		v2, ok2 := other.IntList()
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

func (v Value) Match(boolCase func(v bool), floatCase func(v float64), stringCase func(v string), stringListCase func(v []string), floatListCase func(v []float64)) {
	if !v.ok {
		return
	}
	switch v := v.value.(type) {
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

func (v Value) IsEmpty() bool {
	if !v.ok {
		return true
	}
	var ok bool
	v.Match(func(v bool) {
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
