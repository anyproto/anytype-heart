package pbjson

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
)

func TestPbc_Convert(t *testing.T) {
	s := state.NewDoc("root", nil).(*state.State)
	template.InitTemplate(s, template.WithTitle)
	c := NewConverter(s)
	result := c.Convert(0)
	assert.NotEmpty(t, result)
}
