package domain

import (
	"fmt"

	"github.com/anyproto/any-store/anyenc"
	"github.com/gogo/protobuf/types"
)

type GenericMap[K ~string] struct {
	data map[K]Value
}

// Detail is Key-Value pair
type Detail struct {
	Key   RelationKey
	Value Value
}

type Details = GenericMap[RelationKey]

func NewDetails() *Details {
	return &GenericMap[RelationKey]{data: make(map[RelationKey]Value, 20)}
}

func NewDetailsFromProto(st *types.Struct) *Details {
	data := make(map[RelationKey]Value, len(st.GetFields()))
	d := &GenericMap[RelationKey]{data: data}
	for k, v := range st.GetFields() {
		d.SetProtoValue(RelationKey(k), v)
	}
	return d
}

func NewDetailsFromMap(details map[RelationKey]Value) *Details {
	return &GenericMap[RelationKey]{
		data: details,
	}
}

func NewDetailsWithSize(size int) *Details {
	return &GenericMap[RelationKey]{data: make(map[RelationKey]Value, size)}
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

func NewDetailsFromAnyEnc(v *anyenc.Value) (*Details, error) {
	obj, err := v.Object()
	if err != nil {
		return nil, fmt.Errorf("is object: %w", err)
	}
	res := NewDetailsWithSize(obj.Len())
	var visitErr error
	obj.Visit(func(k []byte, v *anyenc.Value) {
		if visitErr != nil {
			return
		}
		// key is copied
		err := jsonValueToAny(res, RelationKey(k), v)
		if err != nil {
			visitErr = err
		}
	})
	return res, visitErr
}

func jsonValueToAny(d *Details, key RelationKey, val *anyenc.Value) error {
	switch val.Type() {
	case anyenc.TypeNumber:
		v, err := val.Float64()
		if err != nil {
			return fmt.Errorf("number: %w", err)
		}
		d.SetFloat(key, v)
		return nil

	case anyenc.TypeString:
		v, err := val.StringBytes()
		if err != nil {
			return fmt.Errorf("string: %w", err)
		}
		d.SetString(key, string(v))
		return nil
	case anyenc.TypeTrue:
		d.SetBool(key, true)
		return nil
	case anyenc.TypeFalse:
		d.SetBool(key, false)
		return nil
	case anyenc.TypeArray:
		arrVals, err := val.Array()
		if err != nil {
			return fmt.Errorf("array: %w", err)
		}
		// Assume string as default type
		if len(arrVals) == 0 {
			d.SetStringList(key, nil)
			return nil
		}

		firstVal := arrVals[0]
		if firstVal.Type() == anyenc.TypeString {
			res := make([]string, 0, len(arrVals))
			for _, arrVal := range arrVals {
				v, err := arrVal.StringBytes()
				if err != nil {
					return fmt.Errorf("array item: string: %w", err)
				}
				res = append(res, string(v))
			}
			d.SetStringList(key, res)
			return nil
		} else if firstVal.Type() == anyenc.TypeNumber {
			res := make([]float64, 0, len(arrVals))
			for _, arrVal := range arrVals {
				v, err := arrVal.Float64()
				if err != nil {
					return fmt.Errorf("array item: number: %w", err)
				}
				res = append(res, v)
			}
			d.SetFloatList(key, res)
			return nil
		} else {
			return fmt.Errorf("unsupported array type %s", firstVal.Type())
		}
	}
	d.Set(key, Null())
	return nil
}

func (d *GenericMap[K]) ToAnyEnc(arena *anyenc.Arena) *anyenc.Value {
	obj := arena.NewObject()
	d.Iterate(func(k K, v Value) bool {
		obj.Set(string(k), v.ToAnyEnc(arena))
		return true
	})
	return obj
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

// StructDiff returns pb struct which contains:
// - st2 fields that not exist in st1
// - st2 fields that not equal to ones exist in st1
// - nil map value for st1 fields not exist in st2
// In case st1 and st2 are equal returns nil
func StructDiff(st1, st2 *Details) *Details {
	var diff *Details
	if st1 == nil {
		return st2
	}
	if st2 == nil {
		diff = NewDetails()
		st1.Iterate(func(k RelationKey, v Value) bool {
			// TODO This is not correct, Null value could be a valid value. Just rewrite this diff and generate events logic
			diff.Set(k, Null())
			return true
		})
		return diff
	}

	st2.Iterate(func(k2 RelationKey, v2 Value) bool {
		v1 := st1.Get(k2)
		if !v1.Ok() || !v1.Equal(v2) {
			if diff == nil {
				diff = NewDetails()
			}
			diff.Set(k2, v2)
		}
		return true
	})

	st1.Iterate(func(k RelationKey, _ Value) bool {
		if !st2.Has(k) {
			if diff == nil {
				diff = NewDetails()
			}
			diff.Set(k, Null())
		}
		return true
	})

	return diff
}

func DetailsListToProtos(dets []*Details) []*types.Struct {
	res := make([]*types.Struct, 0, len(dets))
	for _, d := range dets {
		res = append(res, d.ToProto())
	}
	return res
}
