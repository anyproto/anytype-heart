package persistentqueue

import (
	"sync"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageQueue(t *testing.T) {
	t.Run("in one goroutine", func(t *testing.T) {
		q := newMessageQueue[int](10)

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
		q := newMessageQueue[int](10)

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
