package pbtypes

import (
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
