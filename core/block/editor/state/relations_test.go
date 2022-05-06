package state

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRelationLinks_Diff(t *testing.T) {
	before := RelationLinks{
		{Id: "1"},
		{Id: "2"},
		{Id: "3"},
		{Id: "4"},
	}
	after := RelationLinks{
		{Id: "2"},
		{Id: "3"},
		{Id: "4"},
		{Id: "5"},
	}
	added, removed := after.Diff(before)
	assert.Equal(t, RelationLinks{{Id: "5"}}, added)
	assert.Equal(t, []string{"1"}, removed)
}
