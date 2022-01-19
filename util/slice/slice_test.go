package slice

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_FindPos(t *testing.T) {
	s := []string{"1", "2", "3"}
	assert.Equal(t, 0, FindPos(s, "1"))
	assert.Equal(t, 2, FindPos(s, "3"))
	assert.Equal(t, -1, FindPos(s, "nf"))
}

func Test_Insert(t *testing.T) {
	var s []string
	s = Insert(s, 0, "1")
	assert.Equal(t, []string{"1"}, s)
	s = Insert(s, 0, "0")
	assert.Equal(t, []string{"0", "1"}, s)
	s = Insert(s, 2, "3")
	assert.Equal(t, []string{"0", "1", "3"}, s)
	s = Insert(s, 2, "2")
	assert.Equal(t, []string{"0", "1", "2", "3"}, s)
	s = Insert(s, 3, "2.1", "2.2", "2.3")
	assert.Equal(t, []string{"0", "1", "2", "2.1", "2.2", "2.3", "3"}, s)
}

func Test_Remove(t *testing.T) {
	var ids = []string{"1", "2", "3"}
	assert.Equal(t, []string{"1", "3"}, Remove(ids, "2"))
}

func TestHasPrefix(t *testing.T) {
	assert.True(t, HasPrefix([]string{"1", "2"}, []string{"1", "2"}))
	assert.True(t, HasPrefix([]string{"1", "2"}, []string{"1"}))
	assert.False(t, HasPrefix([]string{"1"}, []string{"1", "2"}))
	assert.True(t, HasPrefix([]string{"1"}, nil))
	assert.False(t, HasPrefix([]string{"1", "2"}, []string{"1", "3"}))
}
