package pbtypes

import "github.com/gogo/protobuf/types"

func Float64(v float64) *types.Value {
	return &types.Value{
		Kind: &types.Value_NumberValue{NumberValue: v},
	}
}
