package pbtypes

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
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
