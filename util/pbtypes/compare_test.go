package pbtypes

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	types "google.golang.org/protobuf/types/known/structpb"
)

func TestStructMerge(t *testing.T) {
	t.Run("both inputs nil", func(t *testing.T) {
		got := StructMerge(nil, nil, false)
		if got != nil {
			t.Errorf("StructMerge(nil, nil, false) = %v, want %v", got, nil)
		}
	})

	t.Run("first input nil or has nil fields", func(t *testing.T) {
		st2 := &types.Struct{Fields: map[string]*types.Value{"key2": {}}}
		got := StructMerge(nil, st2, false)
		if !reflect.DeepEqual(got, CopyStruct(st2, false)) {
			t.Errorf("StructMerge(nil, st2, false) did not behave like a copy of st2")
		}

		st1WithNilFields := &types.Struct{Fields: nil}
		got = StructMerge(st1WithNilFields, st2, false)
		if !reflect.DeepEqual(got, CopyStruct(st2, false)) {
			t.Errorf("StructMerge(st1WithNilFields, st2, false) did not behave like a copy of st2")
		}
	})

	t.Run("second input nil or has nil fields", func(t *testing.T) {
		st1 := &types.Struct{Fields: map[string]*types.Value{"key1": {}}}
		got := StructMerge(st1, nil, false)
		if !reflect.DeepEqual(got, CopyStruct(st1, false)) {
			t.Errorf("StructMerge(st1, nil, false) did not behave like a copy of st1")
		}

		st2WithNilFields := &types.Struct{Fields: nil}
		got = StructMerge(st1, st2WithNilFields, false)
		if !reflect.DeepEqual(got, CopyStruct(st1, false)) {
			t.Errorf("StructMerge(st1, st2WithNilFields, false) did not behave like a copy of st1")
		}
	})

	t.Run("non-empty structs without copying values", func(t *testing.T) {
		st1 := &types.Struct{Fields: map[string]*types.Value{"key1": {}}}
		st2 := &types.Struct{Fields: map[string]*types.Value{"key2": {}}}
		got := StructMerge(st1, st2, false)
		if len(got.Fields) != 2 || got.Fields["key1"] == nil || got.Fields["key2"] == nil {
			t.Errorf("StructMerge did not correctly merge fields without copying values")
		}
	})

	t.Run("non-empty structs with copying values", func(t *testing.T) {
		st1 := &types.Struct{Fields: map[string]*types.Value{"key1": String("1")}}
		st2 := &types.Struct{Fields: map[string]*types.Value{"key2": String("2")}}
		got := StructMerge(st1, st2, true)

		require.NotSame(t, got.Fields["key2"], st2.Fields["key2"])
		require.Len(t, got.Fields, 2)

	})

	t.Run("field present in both structs", func(t *testing.T) {
		st1 := &types.Struct{Fields: map[string]*types.Value{"key": String("1")}}
		st2 := &types.Struct{Fields: map[string]*types.Value{"key": String("2")}}
		gotWithoutCopy := StructMerge(st1, st2, false)
		require.Same(t, gotWithoutCopy.Fields["key"], st2.Fields["key"])

		gotWithCopy := StructMerge(st1, st2, true)
		require.NotSame(t, gotWithCopy.Fields["key"], st2.Fields["key"])
		require.Equal(t, gotWithCopy.Fields["key"], st2.Fields["key"])

		require.Len(t, gotWithCopy.Fields, 1)
	})
}
