package subscription

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
)

func TestEntry_SubIds(t *testing.T) {
	e := newEntry("id", &types.Struct{})
	e.SetSub("1", true, false)
	assert.Len(t, e.SubIds(), 1)
	e.SetSub("2", false, false)
	assert.Len(t, e.SubIds(), 2)
	e.SetSub("2", false, false)
	assert.Len(t, e.SubIds(), 2)
	assert.Contains(t, e.GetActive(), "1")
	assert.NotContains(t, e.GetActive(), []string{"1", "2"})
	e.RemoveSubId("1")
	assert.Len(t, e.SubIds(), 1)

	e.SetSub("2", true, true)
	e.SetSub("3", true, false)
	assert.Contains(t, e.GetFullDetailsSent(), "2")
	assert.NotContains(t, e.GetFullDetailsSent(), "3")
	e.SetSub("3", true, true)
	assert.NotContains(t, e.GetFullDetailsSent(), []string{"2", "3"})

}
