package persistentqueue

import (
	"sort"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
)

func TestPriorityQueue(t *testing.T) {
	lessFunc := func(one, other int) bool {
		return one > other
	}
	t.Run("consecutive insertions", func(t *testing.T) {
		pq := newPriorityQueue[int](lessFunc)

		const n = 100
		for i := 0; i < n; i++ {
			pq.push(i)
		}

		for i := 0; i < n; i++ {
			got, ok := pq.pop()
			require.True(t, ok)
			want := n - 1 - i
			require.Equal(t, want, got)
		}
	})

	t.Run("property testing", func(t *testing.T) {
		f := func(input []int) bool {
			want := slices.Clone(input)
			// descending order
			sort.Slice(want, func(i, j int) bool {
				return want[i] > want[j]
			})
			pq := newPriorityQueue[int](lessFunc)
			for _, in := range input {
				pq.push(in)
			}

			got := make([]int, 0, len(input))
			for range input {
				gotItem, ok := pq.pop()
				if !ok {
					return false
				}
				got = append(got, gotItem)
			}

			return assert.Equal(t, want, got)
		}

		err := quick.Check(f, nil)
		require.NoError(t, err)
	})

	t.Run("initWith: property testing", func(t *testing.T) {
		f := func(input []int) bool {
			want := slices.Clone(input)
			// descending order
			sort.Slice(want, func(i, j int) bool {
				return want[i] > want[j]
			})
			pq := newPriorityQueue[int](lessFunc)
			pq.initWith(input)

			got := make([]int, 0, len(input))
			for range input {
				gotItem, ok := pq.pop()
				if !ok {
					return false
				}
				got = append(got, gotItem)
			}

			return assert.Equal(t, want, got)
		}

		err := quick.Check(f, nil)
		require.NoError(t, err)
	})
}
