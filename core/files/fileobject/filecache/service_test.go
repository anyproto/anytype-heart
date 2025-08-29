package filecache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueue(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		q, err := newQueue[int](3)
		require.NoError(t, err)

		q.push(0)

		got := q.getNext()
		require.NotNil(t, got)
		assert.Equal(t, 0, *got)
	})

	t.Run("close queue", func(t *testing.T) {
		q, err := newQueue[int](3)
		require.NoError(t, err)

		const n = 10
		waiters := make(chan struct{}, n)
		for range n {
			go func() {
				q.getNext()
				waiters <- struct{}{}
			}()
		}

		q.close()

		timeout := time.After(50 * time.Millisecond)
		for range n {
			select {
			case <-waiters:
			case <-timeout:
				t.Fatal("timeout")
			}
		}
	})

	t.Run("drop old", func(t *testing.T) {
		q, err := newQueue[int](3)
		require.NoError(t, err)

		for i := range 10 {
			q.push(i)
		}

		var gotTasks []int
		for range 3 {
			got := q.getNext()
			require.NotNil(t, got)
			gotTasks = append(gotTasks, *got)
		}

		got := q.pop()
		assert.Nil(t, got)
	})

	t.Run("multiple consumers", func(t *testing.T) {
		const n = 10

		q, err := newQueue[int](n)
		require.NoError(t, err)

		for i := range n {
			q.push(i)
		}

		resultsCh := make(chan int, n)
		for range n {
			go func() {
				got := q.getNext()
				require.NotNil(t, got)
				resultsCh <- *got
			}()
		}

		want := make([]int, n)
		for i := range want {
			want[i] = i
		}

		var got []int
		timeout := time.After(50 * time.Millisecond)
		for range n {
			select {
			case res := <-resultsCh:
				got = append(got, res)
			case <-timeout:
				t.Fatal("timeout")
			}
		}

		assert.ElementsMatch(t, want, got)
	})
}
