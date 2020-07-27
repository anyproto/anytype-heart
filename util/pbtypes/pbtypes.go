package pbtypes

import "github.com/gogo/protobuf/types"

func Float64(v float64) *types.Value {
	return &types.Value{
		Kind: &types.Value_NumberValue{NumberValue: v},
	}
}

func String(v string) *types.Value {
	return &types.Value{
		Kind: &types.Value_StringValue{StringValue: v},
	}
}

func Bool(v bool) *types.Value {
	return &types.Value{
		Kind: &types.Value_BoolValue{BoolValue: v},
	}
}

func GetFloat64(s *types.Struct, name string) float64 {
	if s == nil || s.Fields == nil {
		return 0
	}
	if v, ok := s.Fields[name]; ok {
		return v.GetNumberValue()
	}
	return 0
}
