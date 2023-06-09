package subscription

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEntry_SubIds(t *testing.T) {
	e := &entry{}
	e.SetSub("1", true, false)
	assert.Len(t, e.SubIds(), 1)
	e.SetSub("2", false, false)
	assert.Len(t, e.SubIds(), 2)
	e.SetSub("2", false, false)
	assert.Len(t, e.SubIds(), 2)
	assert.True(t, e.IsActive("1"))
	assert.False(t, e.IsActive("1", "2"))
	e.RemoveSubId("1")
	assert.Len(t, e.SubIds(), 1)

	e.SetSub("2", true, true)
	e.SetSub("3", true, false)
	assert.False(t, e.IsFullDetailsSent("2", "3"))
	assert.True(t, e.IsFullDetailsSent("2"))
	e.SetSub("3", true, true)
	assert.True(t, e.IsFullDetailsSent("2", "3"))

}
