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
