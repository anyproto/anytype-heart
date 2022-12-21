package structs

import "github.com/gogo/protobuf/types"

func String(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}

func Float64(i float64) *types.Value {
	return &types.Value{Kind: &types.Value_NumberValue{NumberValue: i}}
}

func Bool(b bool) *types.Value {
	return &types.Value{Kind: &types.Value_BoolValue{BoolValue: b}}
}
