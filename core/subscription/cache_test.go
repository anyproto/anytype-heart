package subscription

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEntry_SubIds(t *testing.T) {
	e := &entry{}
	e.AddSubId("1", true)
	assert.Len(t, e.SubIds(), 1)
	e.AddSubId("2", false)
	assert.Len(t, e.SubIds(), 2)
	e.AddSubId("2", false)
	assert.Len(t, e.SubIds(), 2)
	assert.True(t, e.IsActive())
	e.RemoveSubId("1")
	assert.Len(t, e.SubIds(), 1)
}
