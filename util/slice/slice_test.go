package slice

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testStruct struct {
	id  string
	val int
}

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

func Test_Intersection(t *testing.T) {
	var res []string
	res = Intersection([]string{"1"}, []string{"1"})
	assert.Equal(t, []string{"1"}, res)
	res = Intersection([]string{"2", "1"}, []string{"1", "2"})
	assert.Equal(t, []string{"1", "2"}, res)
	res = Intersection(nil, []string{"1", "2"})
	assert.Nil(t, res)
	res = Intersection(nil, nil)
	assert.Nil(t, res)
	res = Intersection([]string{"2", "3", "4"}, []string{"1", "2"})
	assert.Equal(t, []string{"2"}, res)
	res = Intersection([]string{"1", "2"}, []string{"10", "10", "10", "2", "5", "1"})
	assert.Equal(t, []string{"1", "2"}, res)
}

func Test_RemoveMut(t *testing.T) {
	var ids = []string{"1", "2", "3"}
	assert.Equal(t, []string{"1", "3"}, RemoveMut(ids, "2"))
}

func Test_Remove(t *testing.T) {
	var ids = []string{"1", "2", "3"}
	assert.Equal(t, []string{"1", "3"}, Remove(ids, "2"))
	assert.Equal(t, []string{"1", "2", "3"}, ids)
}

func Test_RemoveN(t *testing.T) {
	var ids = []string{"1", "2", "3", "4", "1", "2", "0", "7"}
	assert.Equal(t, []string{"2", "3", "4", "2", "0", "7"}, RemoveN(ids, "1"))
	assert.Equal(t, []string{"3", "4", "0", "7"}, RemoveN(ids, "1", "2"))
	assert.Equal(t, []string{"3", "0", "7"}, RemoveN(ids, "2", "4", "1"))
}

func TestHasPrefix(t *testing.T) {
	assert.True(t, HasPrefix([]string{"1", "2"}, []string{"1", "2"}))
	assert.True(t, HasPrefix([]string{"1", "2"}, []string{"1"}))
	assert.False(t, HasPrefix([]string{"1"}, []string{"1", "2"}))
	assert.True(t, HasPrefix([]string{"1"}, nil))
	assert.False(t, HasPrefix([]string{"1", "2"}, []string{"1", "3"}))
}

func TestUnion(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, Union([]string{}, []string{"a", "b", "c"}))
	assert.Equal(t, []string{"a", "b", "c"}, Union([]string{"a"}, []string{"a", "b", "c"}))
	assert.Equal(t, []string{"a", "b", "c"}, Union([]string{"a"}, []string{"b", "c"}))
	assert.Equal(t, []string{"a", "b", "c"}, Union([]string{"a", "b", "c"}, []string{}))
}

func TestChangeElement(t *testing.T) {
	ts := testStruct{id: "b", val: 3}
	result := ReplaceFirstBy([]testStruct{{id: "a", val: 1}, {id: "b", val: 2}}, ts, func(s testStruct) bool {
		return s.id == ts.id
	})
	assert.Equal(t, []testStruct{{id: "a", val: 1}, {id: "b", val: 3}}, result)

	tsPtr := &testStruct{id: "a", val: 3}
	resultPtr := ReplaceFirstBy([]*testStruct{{id: "a", val: 1}, {id: "b", val: 2}}, tsPtr, func(s *testStruct) bool {
		return s.id == tsPtr.id
	})
	assert.Equal(t, []*testStruct{{id: "a", val: 3}, {id: "b", val: 2}}, resultPtr)

	resultFood := ReplaceFirstBy([]string{"apple", "carrot", "bacon"}, "banana", func(f string) bool {
		return f == "bacon"
	})
	assert.Equal(t, []string{"apple", "carrot", "banana"}, resultFood)
}

func TestUnsortedEquals(t *testing.T) {
	assert.True(t, UnsortedEqual([]string{"a", "b", "c"}, []string{"a", "b", "c"}))
	assert.True(t, UnsortedEqual([]string{"a", "b", "c"}, []string{"c", "a", "b"}))
	assert.False(t, UnsortedEqual([]int{1, 2, 3}, []int{2, 2, 3}))
	assert.False(t, UnsortedEqual([]string{"a", "b", "c"}, []string{"a", "b"}))
	assert.False(t, UnsortedEqual([]string{"a", "b", "c"}, []string{"a", "b", "c", "d"}))
}

func TestMergeUniqBy(t *testing.T) {
	strEqual := func(v1, v2 string) bool {
		return v1 == v2
	}
	assert.Equal(t, MergeUniqBy([]string{"a", "b", "c"}, []string{"a", "b"}, strEqual), []string{"a", "b", "c"})
	assert.Equal(t, MergeUniqBy([]string{}, []string{"a", "b"}, strEqual), []string{"a", "b"})
	assert.Equal(t, MergeUniqBy([]string{"a", "b", "c"}, []string{"z", "d"}, strEqual), []string{"a", "b", "c", "z", "d"})
}
