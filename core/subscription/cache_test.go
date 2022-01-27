package subscription

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEntry_SubIds(t *testing.T) {
	e := &entry{}
	e.SetSub("1", true)
	assert.Len(t, e.SubIds(), 1)
	e.SetSub("2", false)
	assert.Len(t, e.SubIds(), 2)
	e.SetSub("2", false)
	assert.Len(t, e.SubIds(), 2)
	assert.True(t, e.IsActive("1"))
	assert.False(t, e.IsActive("1", "2"))
	e.RemoveSubId("1")
	assert.Len(t, e.SubIds(), 1)
}
