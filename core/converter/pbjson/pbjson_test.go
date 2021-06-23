package pbjson

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/stretchr/testify/assert"
)

func TestPbc_Convert(t *testing.T) {
	s := state.NewDoc("root", nil).(*state.State)
	template.InitTemplate(s, template.WithTitle)
	c := NewConverter(s)
	result := c.Convert()
	assert.NotEmpty(t, result)
}
