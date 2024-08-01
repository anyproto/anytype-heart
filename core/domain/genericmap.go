package domain

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/exp/slices"
)

func (d *GenericMap[K]) Len() int {
	return len(d.data)
}

func (d *GenericMap[K]) Set(key K, value any) {
	// TODO Convert number value to float, convert number list value to floats

	// TODO TEMP panic
	v := SomeValue(value)
	if err := v.Validate(); err != nil {
		panic(err)
	}
	d.data[key] = value
}

func (d *GenericMap[K]) Delete(key K) {
	delete(d.data, key)
}

func (d *GenericMap[K]) Keys() []K {
	keys := make([]K, 0, len(d.data))
	for k := range d.data {
		keys = append(keys, k)
	}
	return keys
}

func (d *GenericMap[K]) Iterate(proc func(K, any) bool) {
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
	v, ok := d.data[key]
	return Value{ok, v}
}

func (d *GenericMap[K]) Has(key K) bool {
	_, ok := d.data[key]
	return ok
}

func (d *GenericMap[K]) GetBool(key K) (bool, bool) {
	return d.Get(key).Bool()
}

func (d *GenericMap[K]) GetBoolOrDefault(key K, def bool) bool {
	return d.Get(key).BoolOrDefault(def)
}

func (d *GenericMap[K]) GetString(key K) (string, bool) {
	return d.Get(key).String()
}

func (d *GenericMap[K]) GetStringOrDefault(key K, def string) string {
	return d.Get(key).StringOrDefault(def)
}

func (d *GenericMap[K]) GetInt64(key K) (int64, bool) {
	return d.Get(key).Int64()
}

func (d *GenericMap[K]) GetInt64OrDefault(key K, def int64) int64 {
	return d.Get(key).Int64OrDefault(def)
}

func (d *GenericMap[K]) GetFloat(key K) (float64, bool) {
	return d.Get(key).Float()
}

func (d *GenericMap[K]) GetFloatOrDefault(key K, def float64) float64 {
	return d.Get(key).FloatOrDefault(def)
}

func (d *GenericMap[K]) GetStringList(key K) ([]string, bool) {
	return d.Get(key).StringList()
}

// TODO StringList in pbtypes return []string{singleValue} for string values
func (d *GenericMap[K]) GetStringListOrDefault(key K, def []string) []string {
	return d.Get(key).StringListOrDefault(def)
}

func (d *GenericMap[K]) GetFloatList(key K) ([]float64, bool) {
	return d.Get(key).FloatList()
}

func (d *GenericMap[K]) GetFloatListOrDefault(key K, def []float64) []float64 {
	return d.Get(key).FloatListOrDefault(def)
}

func (d *GenericMap[K]) ShallowCopy() *GenericMap[K] {
	newData := make(map[K]any, len(d.data))
	for k, v := range d.data {
		newData[k] = v
	}
	return &GenericMap[K]{data: newData}
}

func (d *GenericMap[K]) CopyWithoutKeys(keys ...K) *GenericMap[K] {
	newData := make(map[K]any, len(d.data))
	for k, v := range d.data {
		if !slices.Contains(keys, k) {
			newData[k] = v
		}
	}
	return &GenericMap[K]{data: newData}
}

func (d *GenericMap[K]) CopyOnlyWithKeys(keys ...K) *GenericMap[K] {
	newData := make(map[K]any, len(d.data))
	for k, v := range d.data {
		if slices.Contains(keys, k) {
			newData[k] = v
		}
	}
	return &GenericMap[K]{data: newData}
}

func (d *GenericMap[K]) Equal(other *GenericMap[K]) bool {
	if d.Len() != other.Len() {
		return false
	}
	for k, v := range d.data {
		otherV, ok := other.data[k]
		if !ok {
			return false
		}
		if !SomeValue(v).EqualAny(otherV) {
			return false
		}
	}
	return true
}

func (d *GenericMap[K]) Merge(other *GenericMap[K]) *GenericMap[K] {
	res := d.ShallowCopy()
	other.Iterate(func(k K, v any) bool {
		res.Set(k, v)
		return true
	})
	return res
}

type Value struct {
	ok    bool
	value any
}

type ValueType int

const (
	ValueTypeNone ValueType = iota
	ValueTypeBool
	ValueTypeString
	ValueTypeFloat
	ValueTypeStringList
	ValueTypeFloatList
)

func SomeValue(value any) Value {
	return Value{ok: true, value: value}
}

var ErrInvalidValue = fmt.Errorf("invalid value")

func (v Value) Validate() error {
	if !v.ok {
		return errors.Join(ErrInvalidValue, fmt.Errorf("value is none"))
	}
	switch v.value.(type) {
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

func (v Value) EqualAny(other any) bool {
	return v.Equal(Value{ok: true, value: other})
}

func (v Value) Type() ValueType {
	if !v.ok {
		return ValueTypeNone
	}
	switch v.value.(type) {
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
