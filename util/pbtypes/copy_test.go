package pbtypes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	types "google.golang.org/protobuf/types/known/structpb"
)

func TestCopyStruct(t *testing.T) {

	t.Run("nil input struct", func(t *testing.T) {
		got := CopyStruct(nil, false)
		if got != nil {
			t.Errorf("CopyStruct(nil, false) = %v, want %v", got, nil)
		}
	})

	t.Run("struct with copyvals true", func(t *testing.T) {
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

		copy := CopyStruct(original, true)

		assert.NotSame(t, original, copy)
		assert.Equal(t, original, copy)
	})

	t.Run("struct with copyvals false", func(t *testing.T) {
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

		copy := CopyStruct(original, false)

		assert.NotSame(t, original, copy)
		for key, value := range original.Fields {
			assert.Same(t, value, copy.Fields[key])
		}
		assert.Equal(t, original, copy)
	})
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
