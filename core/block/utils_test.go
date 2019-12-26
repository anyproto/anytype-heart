package block

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/stretchr/testify/assert"
)

func Test_findPosInSlice(t *testing.T) {
	s := []string{"1", "2", "3"}
	assert.Equal(t, 0, findPosInSlice(s, "1"))
	assert.Equal(t, 2, findPosInSlice(s, "3"))
	assert.Equal(t, -1, findPosInSlice(s, "nf"))
}

func Test_insertToSlice(t *testing.T) {
	var s []string
	s = insertToSlice(s, "1", 0)
	assert.Equal(t, []string{"1"}, s)
	s = insertToSlice(s, "0", 0)
	assert.Equal(t, []string{"0", "1"}, s)
	s = insertToSlice(s, "3", 2)
	assert.Equal(t, []string{"0", "1", "3"}, s)
	s = insertToSlice(s, "2", 2)
	assert.Equal(t, []string{"0", "1", "2", "3"}, s)
}

func Test_removeFromSlice(t *testing.T) {
	var ids = []string{"1", "2", "3"}
	assert.Equal(t, []string{"1", "3"}, removeFromSlice(ids, "2"))
}

func Test_isSmartBlock(t *testing.T) {
	assert.True(t, isSmartBlock(&model.Block{Content: &model.BlockContentOfPage{}}))
}
