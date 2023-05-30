package pbtypes

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
)

func TestCopyStruct(t *testing.T) {
	original := &types.Struct{
		Fields: map[string]*types.Value{
			"field1": {
				Kind: &types.Value_StringValue{
					StringValue: "test string",
				},
			},
			"field2": {
				Kind: &types.Value_NumberValue{
					NumberValue: 123.456,
				},
			},
		},
	}

	copy := CopyStruct(original)

	assert.NotSame(t, original, copy)
	assert.Equal(t, original, copy)
}

func TestCopyValue(t *testing.T) {
	original := &types.Value{
		Kind: &types.Value_StructValue{
			StructValue: &types.Struct{
				Fields: map[string]*types.Value{
					"field1": {
						Kind: &types.Value_StringValue{
							StringValue: "test string",
						},
					},
				},
			},
		},
	}

	copy := CopyVal(original)

	assert.NotSame(t, original, copy)
	assert.Equal(t, original, copy)
}

func TestCopyListValue(t *testing.T) {
	original := &types.ListValue{
		Values: []*types.Value{
			{
				Kind: &types.Value_BoolValue{
					BoolValue: true,
				},
			},
			{
				Kind: &types.Value_NullValue{
					NullValue: types.NullValue_NULL_VALUE,
				},
			},
		},
	}

	copy := CopyListVal(original)

	assert.NotSame(t, original, copy)
	assert.Equal(t, original, copy)
}
