package slice

import (
	"context"
	"errors"
	"sort"
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

type testData struct {
	id int
}

func TestBatch(t *testing.T) {
	t.Run("all remote success", func(t *testing.T) {
		// given
		ctx := context.Background()
		input := []testData{{1}, {2}, {3}, {4}, {5}}
		batchCount := 2

		remote := func(ctx context.Context, s []testData) ([]int, error) {
			results := make([]int, len(s))
			for i, data := range s {
				results[i] = data.id * 10
			}
			return results, nil
		}

		local := func(ctx context.Context, s []testData) ([]int, error) {
			return nil, errors.New("local should not be called")
		}

		expected := []int{10, 20, 30, 40, 50}

		// when
		result, err := Batch(ctx, input, remote, local, batchCount)

		// then
		sort.Ints(result)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})
	t.Run("remote failed, local succeed", func(t *testing.T) {
		// given
		ctx := context.Background()
		input := []testData{{1}, {2}, {3}, {4}, {5}}
		batchCount := 1

		remote := func(ctx context.Context, s []testData) ([]int, error) {
			for _, data := range s {
				if data.id%2 != 0 {
					return nil, errors.New("remote failure")
				}
			}
			results := make([]int, len(s))
			for i, data := range s {
				results[i] = data.id * 10
			}
			return results, nil
		}

		local := func(ctx context.Context, s []testData) ([]int, error) {
			results := make([]int, len(s))
			for i, data := range s {
				results[i] = data.id * 5
			}
			return results, nil
		}

		// when
		expected := []int{5, 20, 15, 40, 25}
		result, err := Batch(ctx, input, remote, local, batchCount)

		// then
		sort.Ints(result)
		sort.Ints(expected)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})
	t.Run("all failed", func(t *testing.T) {
		// given
		ctx := context.Background()
		input := []testData{{1}, {2}, {3}}
		batchCount := 2

		remote := func(ctx context.Context, s []testData) ([]int, error) {
			return nil, errors.New("remote failure")
		}

		local := func(ctx context.Context, s []testData) ([]int, error) {
			return nil, errors.New("local failure")
		}

		// when
		result, err := Batch(ctx, input, remote, local, batchCount)

		// then
		assert.Error(t, err)
		assert.Nil(t, result)
	})
	t.Run("empty input", func(t *testing.T) {
		// given
		ctx := context.Background()
		input := []testData{}
		batchCount := 2

		remote := func(ctx context.Context, s []testData) ([]int, error) {
			return nil, errors.New("remote should not be called")
		}

		local := func(ctx context.Context, s []testData) ([]int, error) {
			return nil, errors.New("local should not be called")
		}

		// when
		result, err := Batch(ctx, input, remote, local, batchCount)

		// then
		assert.NoError(t, err)
		assert.Nil(t, result)
	})
	t.Run("batch count greater than input", func(t *testing.T) {
		// given
		ctx := context.Background()
		input := []testData{{1}, {2}}
		batchCount := 5

		remote := func(ctx context.Context, s []testData) ([]int, error) {
			results := make([]int, len(s))
			for i, data := range s {
				results[i] = data.id * 10
			}
			return results, nil
		}

		local := func(ctx context.Context, s []testData) ([]int, error) {
			return nil, errors.New("local should not be called")
		}

		expected := []int{10, 20}

		// when
		result, err := Batch(ctx, input, remote, local, batchCount)

		// then
		sort.Ints(result)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})
}
