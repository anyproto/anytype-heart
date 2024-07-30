package domain

import (
	"fmt"

	"github.com/gogo/protobuf/types"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type Tombstone struct{}

type GenericMap[K ~string] struct {
	data map[K]any
}

type Details = GenericMap[RelationKey]

// TODO Helpers for frequent fields like id, spaceId, etc?

func NewDetails() *Details {
	return &GenericMap[RelationKey]{data: make(map[RelationKey]any, 20)}
}

func NewDetailsFromProto(st *types.Struct) *Details {
	data := make(map[RelationKey]any, len(st.GetFields()))
	for k, v := range st.GetFields() {
		data[RelationKey(k)] = pbtypes.ProtoToAny(v)
	}
	return &GenericMap[RelationKey]{data: data}
}

func NewDetailsWithSize(size int) *Details {
	return &GenericMap[RelationKey]{data: make(map[RelationKey]any, size)}
}

func NewDetailsFromMap(data map[RelationKey]any) *Details {
	return &GenericMap[RelationKey]{data: data}
}

func (d *GenericMap[K]) ToProto() *types.Struct {
	res := &types.Struct{
		Fields: make(map[string]*types.Value, len(d.data)),
	}
	for k, v := range d.data {
		res.Fields[string(k)] = pbtypes.AnyToProto(v)
	}
	return res
}

func JsonToProto(v *fastjson.Value) (*Details, error) {
	obj, err := v.Object()
	if err != nil {
		return nil, fmt.Errorf("is object: %w", err)
	}
	res := NewDetailsWithSize(obj.Len())
	var visitErr error
	obj.Visit(func(k []byte, v *fastjson.Value) {
		if visitErr != nil {
			return
		}
		// key is copied
		val, err := JsonValueToProto(v)
		if err != nil {
			visitErr = err
		}
		res.Set(RelationKey(k), val)
	})
	return res, visitErr
}

func JsonValueToProto(val *fastjson.Value) (any, error) {
	switch val.Type() {
	case fastjson.TypeNumber:
		return val.Float64()
	case fastjson.TypeString:
		v, err := val.StringBytes()
		if err != nil {
			return nil, fmt.Errorf("string: %w", err)
		}
		return string(v), nil
	case fastjson.TypeTrue:
		return true, nil
	case fastjson.TypeFalse:
		return false, nil
	case fastjson.TypeArray:
		arrVals, err := val.Array()
		if err != nil {
			return nil, fmt.Errorf("array: %w", err)
		}
		// Assume string as default type
		if len(arrVals) == 0 {
			return []string{}, nil
		}

		firstVal := arrVals[0]
		if firstVal.Type() == fastjson.TypeString {
			res := make([]string, 0, len(arrVals))
			for _, arrVal := range arrVals {
				v, err := arrVal.StringBytes()
				if err != nil {
					return nil, fmt.Errorf("array item: string: %w", err)
				}
				res = append(res, string(v))
			}
			return res, nil
		} else if firstVal.Type() == fastjson.TypeNumber {
			res := make([]float64, 0, len(arrVals))
			for _, arrVal := range arrVals {
				v, err := arrVal.Float64()
				if err != nil {
					return nil, fmt.Errorf("array item: number: %w", err)
				}
				res = append(res, v)
			}
			return res, nil
		} else {
			return nil, fmt.Errorf("unsupported array type %s", firstVal.Type())
		}
	}
	// TODO What is the matter with nil value?
	return nil, nil
}

func ProtoToJson(arena *fastjson.Arena, details *Details) *fastjson.Value {
	obj := arena.NewObject()
	details.Iterate(func(k RelationKey, v any) bool {
		obj.Set(string(k), ProtoValueToJson(arena, v))
		return true
	})
	return obj
}

func ProtoValueToJson(arena *fastjson.Arena, v any) *fastjson.Value {
	if v == nil {
		return arena.NewNull()
	}
	switch v := v.(type) {
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
			lst.SetArrayItem(i, ProtoValueToJson(arena, it))
		}
		return lst
	case []float64:
		lst := arena.NewArray()
		for i, it := range v {
			lst.SetArrayItem(i, ProtoValueToJson(arena, it))
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
		st1.Iterate(func(k RelationKey, v any) bool {
			diff.Set(k, Tombstone{})
			return true
		})
		return diff
	}

	st2.Iterate(func(k2 RelationKey, v2 any) bool {
		v1 := st1.Get(k2)
		if !v1.Ok() || !v1.EqualAny(v2) {
			if diff == nil {
				diff = NewDetails()
			}
			diff.Set(k2, v2)
		}
		return true
	})

	st1.Iterate(func(k RelationKey, _ any) bool {
		if !st2.Has(k) {
			if diff == nil {
				diff = NewDetails()
			}
			diff.Set(k, Tombstone{})
		}
		return true
	})

	return diff
}
