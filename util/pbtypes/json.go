package pbtypes

import (
	"bytes"
	"fmt"

	"github.com/gogo/protobuf/types"
	"github.com/valyala/fastjson"
)

func JsonToProto(v *fastjson.Value) (*types.Struct, error) {
	obj, err := v.Object()
	if err != nil {
		return nil, fmt.Errorf("is object: %w", err)
	}
	res := &types.Struct{
		Fields: make(map[string]*types.Value, obj.Len()),
	}
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
		res.Fields[string(k)] = val
	})
	return res, visitErr
}

func JsonValueToProto(val *fastjson.Value) (*types.Value, error) {
	switch val.Type() {
	case fastjson.TypeNumber:
		v, err := val.Float64()
		if err != nil {
			return nil, fmt.Errorf("float64: %w", err)
		}
		return Float64(v), nil
	case fastjson.TypeString:
		v, err := val.StringBytes()
		if err != nil {
			return nil, fmt.Errorf("string: %w", err)
		}
		return String(string(v)), nil
	case fastjson.TypeTrue:
		return Bool(true), nil
	case fastjson.TypeFalse:
		return Bool(false), nil
	case fastjson.TypeArray:
		vals, err := val.Array()
		if err != nil {
			return nil, fmt.Errorf("array: %w", err)
		}
		lst := make([]*types.Value, 0, len(vals))
		for i, v := range vals {
			val, err := JsonValueToProto(v)
			if err != nil {
				return nil, fmt.Errorf("array item %d: %w", i, err)
			}
			lst = append(lst, val)
		}
		return &types.Value{
			Kind: &types.Value_ListValue{
				ListValue: &types.ListValue{
					Values: lst,
				},
			},
		}, nil
	}
	return Null(), nil
}

func ProtoToJson(arena *fastjson.Arena, details *types.Struct) *fastjson.Value {
	obj := arena.NewObject()
	for k, v := range details.Fields {
		obj.Set(k, ProtoValueToJson(arena, v))
	}
	return obj
}

func ProtoValueToJson(arena *fastjson.Arena, v *types.Value) *fastjson.Value {
	if v == nil {
		return arena.NewNull()
	}
	switch v.Kind.(type) {
	case *types.Value_StringValue:
		return arena.NewString(v.GetStringValue())
	case *types.Value_NumberValue:
		return arena.NewNumberFloat64(v.GetNumberValue())
	case *types.Value_BoolValue:
		if v.GetBoolValue() {
			return arena.NewTrue()
		} else {
			return arena.NewFalse()
		}
	case *types.Value_ListValue:
		lst := arena.NewArray()
		for i, v := range v.GetListValue().Values {
			lst.SetArrayItem(i, ProtoValueToJson(arena, v))
		}
		return lst
	default:
		return arena.NewNull()
	}
}

type JsonDiffType int

const (
	JsonDiffTypeAdd JsonDiffType = iota
	JsonDiffTypeRemove
	JsonDiffTypeUpdate
)

type JsonDiff struct {
	Type  JsonDiffType
	Key   string
	Value *fastjson.Value
}

func DiffJson(a *fastjson.Value, b *fastjson.Value) ([]JsonDiff, error) {
	objA, err := a.Object()
	if err != nil {
		return nil, fmt.Errorf("param a is not an object: %w", err)
	}
	objB, err := b.Object()
	if err != nil {
		return nil, fmt.Errorf("param b is not an object: %w", err)
	}

	var diffs []JsonDiff
	existsA := make(map[string]struct{}, objA.Len())

	objA.Visit(func(key []byte, v *fastjson.Value) {
		existsA[string(key)] = struct{}{}
	})

	var (
		stop     bool
		visitErr error
	)
	objB.Visit(func(key []byte, v *fastjson.Value) {
		if stop {
			return
		}
		strKey := string(key)
		if _, ok := existsA[strKey]; ok {
			eq, err := compareValue(a.Get(strKey), v)
			if err != nil {
				visitErr = err
				stop = true
			}
			if !eq {
				diffs = append(diffs, JsonDiff{
					Type:  JsonDiffTypeUpdate,
					Key:   strKey,
					Value: v, // Holden value, be cautious
				})
			}
			delete(existsA, strKey)
		} else {
			diffs = append(diffs, JsonDiff{
				Type:  JsonDiffTypeAdd,
				Key:   strKey,
				Value: v, // Holden value, be cautious
			})
		}
	})
	if visitErr != nil {
		return nil, fmt.Errorf("visit b: %w", visitErr)
	}

	for key := range existsA {
		diffs = append(diffs, JsonDiff{
			Type: JsonDiffTypeRemove,
			Key:  key,
		})
	}
	return diffs, nil
}

func compareValue(a *fastjson.Value, b *fastjson.Value) (bool, error) {
	if a.Type() != b.Type() {
		// Return true, as we have checked that types are equal
		return false, nil
	}
	switch a.Type() {
	case fastjson.TypeNull:
		return true, nil
	case fastjson.TypeNumber:
		af, err := a.Float64()
		if err != nil {
			return false, fmt.Errorf("a: get float64: %w", err)
		}
		bf, err := b.Float64()
		if err != nil {
			return false, fmt.Errorf("b: get float64: %w", err)
		}
		return af == bf, nil
	case fastjson.TypeString:
		as, err := a.StringBytes()
		if err != nil {
			return false, fmt.Errorf("a: get string: %w", err)
		}
		bs, err := b.StringBytes()
		if err != nil {
			return false, fmt.Errorf("b: get string: %w", err)
		}
		return bytes.Compare(as, bs) == 0, nil
	case fastjson.TypeTrue, fastjson.TypeFalse:
		// Return true, as we have checked that types are equal
		return true, nil
	case fastjson.TypeArray:
		aa, err := a.Array()
		if err != nil {
			return false, fmt.Errorf("a: get array: %w", err)
		}
		ba, err := b.Array()
		if err != nil {
			return false, fmt.Errorf("b: get array: %w", err)
		}
		if len(aa) != len(ba) {
			return false, nil
		}
		for i := range aa {
			eq, err := compareValue(aa[i], ba[i])
			if err != nil {
				return false, err
			}
			if !eq {
				return false, nil
			}
		}
		return true, nil
	case fastjson.TypeObject:
		ao, err := a.Object()
		if err != nil {
			return false, fmt.Errorf("a: get object: %w", err)
		}
		bo, err := b.Object()
		if err != nil {
			return false, fmt.Errorf("b: get object: %w", err)
		}
		if ao.Len() != bo.Len() {
			return false, nil
		}
		var (
			eq       bool
			stop     bool
			visitErr error
		)
		ao.Visit(func(k []byte, va *fastjson.Value) {
			if stop {
				return
			}
			vb := bo.Get(string(k))
			// TODO Test nil values
			if vb == nil {
				eq = false
				stop = true
				return
			}
			eq, visitErr = compareValue(va, vb)
			if visitErr != nil {
				stop = true
				return
			}
			if !eq {
				stop = true
			}
		})
		if visitErr != nil {
			return false, fmt.Errorf("compare objects: %w", visitErr)
		}
		return eq, nil
	}
	return false, nil
}

func JsonArrayToStrings(arr []*fastjson.Value) []string {
	res := make([]string, 0, len(arr))
	for _, v := range arr {
		res = append(res, string(v.GetStringBytes()))
	}
	return res
}

func StringsToJsonArray(arena *fastjson.Arena, arr []string) *fastjson.Value {
	res := arena.NewArray()
	for i, v := range arr {
		res.SetArrayItem(i, arena.NewString(v))
	}
	return res
}
