package persistentqueue

import (
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageQueue(t *testing.T) {
	lessFunc := func(one, other int) bool {
		return one > other
	}

	t.Run("in one goroutine", func(t *testing.T) {
		q := newPriorityMessageQueue[int](lessFunc)

		for i := 0; i < 100; i++ {
			err := q.add(i)
			require.NoError(t, err)
		}

		for i := 0; i < 100; i++ {
			got, err := q.waitOne()
			require.NoError(t, err)

			want := 99 - i
			assert.Equal(t, want, got)
		}
	})

	t.Run("in multiple goroutines", func(t *testing.T) {
		q := newPriorityMessageQueue[int](lessFunc)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := q.add(i)
				require.NoError(t, err)
			}()
		}

		results := make(chan int, 100)
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, err := q.waitOne()
				require.NoError(t, err)

				results <- got
			}()
		}
		wg.Wait()

		close(results)

		resultsSlice := lo.ChannelToSlice(results)

		want := make([]int, 100)
		for i := 0; i < 100; i++ {
			want[i] = i
		}

		assert.ElementsMatch(t, want, resultsSlice)
	})
}

type testItemWithPriority struct {
	Value     int
	Timestamp int64
	Priority  int
}

func TestMessageQueueWithComplexPriority(t *testing.T) {
	lessFunc := func(one, other testItemWithPriority) bool {
		if one.Priority != other.Priority {
			return one.Priority > other.Priority
		}
		return one.Timestamp < other.Timestamp
	}

	q := newPriorityMessageQueue[testItemWithPriority](lessFunc)

	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := q.add(testItemWithPriority{
				Value:     i,
				Timestamp: time.Now().UnixMilli(),
				Priority:  rand.Intn(10),
			})
			require.NoError(t, err)
		}()
	}
	wg.Wait()

	got := make([]testItemWithPriority, 0, n)
	for i := 0; i < n; i++ {
		it, err := q.waitOne()
		require.NoError(t, err)

		got = append(got, it)
	}

	gotIsSorted := sort.SliceIsSorted(got, func(i, j int) bool {
		one, other := got[i], got[j]
		return lessFunc(one, other)
	})

	assert.True(t, gotIsSorted)
}
