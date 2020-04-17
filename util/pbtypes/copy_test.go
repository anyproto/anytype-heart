package pbtypes

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
)

func TestCopyStruct(t *testing.T) {
	s := &types.Struct{
		Fields: map[string]*types.Value{
			"string": String("string"),
			"bool":   Bool(true),
		},
	}
	c := CopyStruct(s)
	assert.True(t, s.Equal(c))
}
