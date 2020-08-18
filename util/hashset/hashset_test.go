package hashset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHashSet_Difference(t *testing.T) {
	data1 := []int{2, 3, 4, 5, 6}
	data2 := []int{1, 3, 4, 7, 8}

	s1 := New()
	for _, d := range data1 {
		s1.Add(d)
	}

	s2 := New()
	for _, d := range data2 {
		s2.Add(d)
	}

	onlyS1, onlyS2 := Difference(s1, s2)

	assert.Equal(t, 3, len(onlyS1.data))
	for _, v1 := range []int{2, 5, 6} {
		assert.True(t, onlyS1.Find(v1))
	}

	assert.Equal(t, 3, len(onlyS2.data))
	for _, v2 := range []int{1, 7, 8} {
		assert.True(t, onlyS2.Find(v2))
	}
}

func TestHashSet_Intersection(t *testing.T) {
	data1 := []int{2, 3, 4, 5, 6}
	data2 := []int{1, 3, 4, 7, 8}

	s1 := New()
	for _, d := range data1 {
		s1.Add(d)
	}

	s2 := New()
	for _, d := range data2 {
		s2.Add(d)
	}

	intersect := Intersection(s1, s2)

	assert.Equal(t, 2, len(intersect.data))
	for _, v := range []int{3, 4} {
		assert.True(t, intersect.Find(v))
	}
}
