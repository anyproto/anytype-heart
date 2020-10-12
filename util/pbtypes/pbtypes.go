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

func StringList(s []string) *types.Value {
	var vals []*types.Value
	for _, str := range s {
		vals = append(vals, String(str))
	}

	return &types.Value{
		Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: vals}},
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

func GetString(s *types.Struct, name string) string {
	if s == nil || s.Fields == nil {
		return ""
	}
	if v, ok := s.Fields[name]; ok {
		return v.GetStringValue()
	}
	return ""
}
