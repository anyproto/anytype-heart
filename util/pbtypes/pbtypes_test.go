package pbtypes

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	types "google.golang.org/protobuf/types/known/structpb"
)

func TestGet(t *testing.T) {
	nString := String("nString")
	st := &types.Struct{Fields: map[string]*types.Value{
		"string": String("string"),
		"struct": Struct(&types.Struct{Fields: map[string]*types.Value{
			"nString": nString,
		}}),
	}}

	assert.Equal(t, st.Fields["string"], Get(st, "string"))
	assert.Equal(t, nString, Get(st, "struct", "nString"))
	assert.Nil(t, Get(st, "some", "thing"))
}

func TestStructIterate(t *testing.T) {
	st := &types.Struct{
		Fields: map[string]*types.Value{
			"one": String("one"),
			"two": Int64(2),
			"three": Struct(&types.Struct{
				Fields: map[string]*types.Value{
					"child": String("childVal"),
				},
			}),
		},
	}
	var paths [][]string
	StructIterate(st, func(p []string, _ *types.Value) {
		paths = append(paths, p)
	})
	assert.Len(t, paths, 4)
	assert.Contains(t, paths, []string{"three", "child"})
	assert.Contains(t, paths, []string{"two"})
}

func TestCopyStructFields(t *testing.T) {
	t.Run("not nil struct", func(t *testing.T) {
		src := &types.Struct{
			Fields: map[string]*types.Value{
				"one": String("one"),
				"two": Int64(2),
				"three": Struct(&types.Struct{
					Fields: map[string]*types.Value{
						"child": String("childVal"),
					},
				}),
			},
		}
		newStruct := CopyStructFields(src, "one", "three")
		assert.Len(t, newStruct.Fields, 2)
		assert.Equal(t, newStruct.Fields["one"], src.Fields["one"])
		assert.Equal(t, newStruct.Fields["three"], src.Fields["three"])
	})
	t.Run("nil struct", func(t *testing.T) {
		newStruct := CopyStructFields(nil, "one", "three")
		assert.NotNil(t, newStruct.Fields)
		newStruct = CopyStructFields(&types.Struct{}, "one", "three")
		assert.NotNil(t, newStruct.Fields)
	})
}

func TestStructEqualKeys(t *testing.T) {
	st1 := &types.Struct{Fields: map[string]*types.Value{
		"k1": String("1"),
		"k2": String("1"),
	}}
	assert.True(t, StructEqualKeys(st1, &types.Struct{Fields: map[string]*types.Value{
		"k1": String("1"),
		"k2": String("1"),
	}}))
	assert.False(t, StructEqualKeys(st1, &types.Struct{Fields: map[string]*types.Value{
		"k1": String("1"),
		"k3": String("1"),
	}}))
	assert.False(t, StructEqualKeys(st1, &types.Struct{Fields: map[string]*types.Value{
		"k1": String("1"),
	}}))
	assert.False(t, StructEqualKeys(st1, &types.Struct{}))
	assert.False(t, StructEqualKeys(st1, nil))
}

func TestNormalizeValue(t *testing.T) {

	tests := []struct {
		name       string
		input      *types.Value
		normalized *types.Value
	}{
		{"nil", nil, nil},
		{"nil kind", &types.Value{}, &types.Value{Kind: &types.Value_NullValue{NullValue: types.NullValue_NULL_VALUE}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			NormalizeValue(tt.input)
			assert.Equal(t, tt.normalized, tt.input, "normalized value should be equal to input")
		})
	}
}

func TestValidateValue(t *testing.T) {
	type args struct {
		t *types.Value
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{"nil value", args{nil}, assert.NoError},
		{"nil kind", args{&types.Value{}}, assert.Error},
		{"empty string", args{&types.Value{Kind: &types.Value_StringValue{}}}, assert.NoError},
		{"non empty string", args{&types.Value{Kind: &types.Value_StringValue{StringValue: "123"}}}, assert.NoError},
		{"nil struct value", args{&types.Value{Kind: &types.Value_StructValue{}}}, assert.NoError},                                 // StructValue is optional. So nil means it's not set
		{"nil struct value map", args{&types.Value{Kind: &types.Value_StructValue{StructValue: &types.Struct{}}}}, assert.NoError}, // it's initialized automatically
		{"non-nil struct value", args{&types.Value{Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: map[string]*types.Value{}}}}}, assert.NoError},
		{"nil struct map value", args{&types.Value{Kind: &types.Value_StructValue{StructValue: &types.Struct{Fields: map[string]*types.Value{"k": nil}}}}}, assert.Error}, // it's valid but it hard to support on JS and has some problems converting to JSON: https://github.com/golang/protobuf/issues/1258#issuecomment-750436666
		{"list nil value", args{&types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: []*types.Value{nil}}}}}, assert.Error},                      // it's valid but it hard to support on JS and has some problems converting to JSON: https://github.com/golang/protobuf/issues/1258#issuecomment-750436666

	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(t, ValidateValue(tt.args.t), fmt.Sprintf("ValidateValue(%v)", tt.args.t))
		})
	}
}

func TestIsEmptyValueOrAbsent(t *testing.T) {
	data := &types.Struct{Fields: map[string]*types.Value{
		"structValue":      {Kind: &types.Value_StructValue{StructValue: &types.Struct{}}},
		"stringValue":      {Kind: &types.Value_StringValue{StringValue: "42"}},
		"emptyStringValue": {Kind: &types.Value_StringValue{StringValue: ""}},
		"numberValue":      {Kind: &types.Value_NumberValue{NumberValue: 42}},
		"emptyNumberValue": {Kind: &types.Value_NumberValue{NumberValue: 0}},
		"boolValue":        {Kind: &types.Value_BoolValue{BoolValue: true}},
		"emptyBoolValue":   {Kind: &types.Value_BoolValue{BoolValue: false}},
		"listValue": {Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: []*types.Value{{
			Kind: &types.Value_StringValue{StringValue: "Hello"}}}}}},
		"emptyListValue": {Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: []*types.Value{}}}},
		"nullValue":      Null(),
	}}

	tests := []struct {
		name      string
		s         *types.Struct
		fieldName string
		expected  bool
	}{
		{"NilStruct", nil, "field", true},
		{"NilFields", &types.Struct{}, "nilField", true},
		{"StructValue", data, "structValue", false},
		{"AbsentField", data, "nonExistentField", true},
		{"EmptyStringValue", data, "emptyStringValue", true},
		{"NonEmptyStringValue", data, "stringValue", false},
		{"ZeroNumberValue", data, "emptyNumberValue", false},
		{"NonZeroNumberValue", data, "numberValue", false},
		{"FalseBoolValue", data, "emptyBoolValue", false},
		{"TrueBoolValue", data, "boolValue", false},
		{"EmptyListValue", data, "emptyListValue", true},
		{"NullValue", data, "nullValue", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEmptyValueOrAbsent(tt.s, tt.fieldName); got != tt.expected {
				t.Errorf("IsEmptyValueOrAbsent(%v, %v) = %v, want %v", tt.s, tt.fieldName, got, tt.expected)
			}
		})
	}
}
