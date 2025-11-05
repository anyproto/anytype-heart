package domain

import (
	"iter"
	"sort"

	"github.com/anyproto/any-store/anyenc"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"
	"golang.org/x/exp/slices"
)

type GenericMap[K ~string] struct {
	data map[K]Value
}

func NewGenericMap[K ~string]() *GenericMap[K] {
	return &GenericMap[K]{data: make(map[K]Value)}
}

func (d *GenericMap[K]) Len() int {
	if d == nil {
		return 0
	}
	return len(d.data)
}

func (d *GenericMap[K]) Set(key K, value Value) *GenericMap[K] {
	d.data[key] = value
	return d
}

func (d *GenericMap[K]) SetNull(key K) *GenericMap[K] {
	d.Set(key, Null())
	return d
}

func (d *GenericMap[K]) SetBool(key K, value bool) *GenericMap[K] {
	d.data[key] = Bool(value)
	return d
}

func (d *GenericMap[K]) SetString(key K, value string) *GenericMap[K] {
	d.data[key] = String(value)
	return d
}

func (d *GenericMap[K]) SetInt64(key K, value int64) *GenericMap[K] {
	d.data[key] = Int64(value)
	return d
}

func (d *GenericMap[K]) SetFloat64(key K, value float64) *GenericMap[K] {
	d.data[key] = Float64(value)
	return d
}

func (d *GenericMap[K]) SetStringList(key K, value []string) *GenericMap[K] {
	d.data[key] = StringList(value)
	return d
}

func (d *GenericMap[K]) SetFloat64List(key K, value []float64) *GenericMap[K] {
	d.data[key] = Float64List(value)
	return d
}

func (d *GenericMap[K]) SetInt64List(key K, value []int64) *GenericMap[K] {
	d.data[key] = Int64List(value)
	return d
}

func (d *GenericMap[K]) SetProtoValue(key K, value *types.Value) *GenericMap[K] {
	d.Set(key, ValueFromProto(value))
	return d
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

func (d *GenericMap[K]) Iterate() iter.Seq2[K, Value] {
	return func(proc func(key K, value Value) bool) {
		if d == nil {
			return
		}
		for k, v := range d.data {
			if !proc(k, v) {
				return
			}
		}
	}
}

func (d *GenericMap[K]) IterateSorted() iter.Seq2[K, Value] {
	return func(proc func(key K, value Value) bool) {
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
}

func (d *GenericMap[K]) IterateKeys() iter.Seq[K] {
	return func(proc func(key K) bool) {
		if d == nil {
			return
		}
		for k := range d.data {
			if !proc(k) {
				return
			}
		}
	}
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

func (d *GenericMap[K]) GetNull(key K) bool {
	return d.Get(key).IsNull()
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

func (d *GenericMap[K]) TryFloat64(key K) (float64, bool) {
	return d.Get(key).TryFloat64()
}

func (d *GenericMap[K]) GetFloat64(key K) float64 {
	return d.Get(key).Float64()
}

func (d *GenericMap[K]) TryStringList(key K) ([]string, bool) {
	return d.Get(key).TryStringList()
}

func (d *GenericMap[K]) GetStringList(key K) []string {
	return d.Get(key).StringList()
}

func (d *GenericMap[K]) WrapToStringList(key K) []string {
	return d.Get(key).WrapToStringList()
}

func (d *GenericMap[K]) TryFloat64List(key K) ([]float64, bool) {
	return d.Get(key).TryFloat64List()
}

func (d *GenericMap[K]) GetFloat64List(key K) []float64 {
	return d.Get(key).Float64List()
}

func (d *GenericMap[K]) TryInt64List(key K) ([]int64, bool) {
	return d.Get(key).TryInt64List()
}

func (d *GenericMap[K]) GetInt64List(key K) []int64 {
	return d.Get(key).Int64List()
}

func (d *GenericMap[K]) TryMapValue(key K) (ValueMap, bool) {
	return d.Get(key).TryMapValue()
}

func (d *GenericMap[K]) GetMapValue(key K) ValueMap {
	return d.Get(key).MapValue()
}

func (d *GenericMap[K]) Copy() *GenericMap[K] {
	if d == nil {
		return NewGenericMap[K]()
	}
	newData := make(map[K]Value, len(d.data))
	for k, v := range d.data {
		newData[k] = v
	}
	return &GenericMap[K]{data: newData}
}

func (d *GenericMap[K]) CopyWithoutKeys(keys ...K) *GenericMap[K] {
	if d == nil {
		return NewGenericMap[K]()
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
		return NewGenericMap[K]()
	}
	newData := make(map[K]Value, len(keys))
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
	// One is nil, other is not
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
	for k, v := range other.Iterate() {
		res.Set(k, v)
	}
	return res
}

func (d *GenericMap[K]) MarshalJSON() ([]byte, error) {
	proto := d.ToProto()
	m := &jsonpb.Marshaler{}
	out, err := m.MarshalToString(proto)
	return []byte(out), err
}

func (d *GenericMap[K]) ToProto() *types.Struct {
	if d == nil {
		return &types.Struct{Fields: map[string]*types.Value{}}
	}
	res := &types.Struct{
		Fields: make(map[string]*types.Value, len(d.data)),
	}
	for k, v := range d.data {
		res.Fields[string(k)] = v.ToProto()
	}
	return res
}

func (d *GenericMap[K]) ToAnyEnc(arena *anyenc.Arena) *anyenc.Value {
	obj := arena.NewObject()
	for k, v := range d.Iterate() {
		obj.Set(string(k), v.ToAnyEnc(arena))
	}
	return obj
}
