package futures

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFutures(t *testing.T) {
	t.Run("synchronously in linear order: has value", func(t *testing.T) {
		f := New[int]()
		f.ResolveValue(42)

		got, err := f.Wait()
		require.NoError(t, err)
		assert.Equal(t, 42, got)
	})

	t.Run("synchronously in linear order: has error", func(t *testing.T) {
		f := New[int]()
		f.ResolveErr(fmt.Errorf("test error"))

		got, err := f.Wait()
		require.Error(t, err)
		assert.Equal(t, 0, got)
	})

	t.Run("one producer, multiple consumers: has value", func(t *testing.T) {
		f := New[int]()

		var wg sync.WaitGroup
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, err := f.Wait()
				require.NoError(t, err)
				assert.Equal(t, 42, got)
			}()
		}

		f.ResolveValue(42)

		wg.Wait()
	})

	t.Run("one producer, multiple consumers: has error", func(t *testing.T) {
		f := New[int]()

		var wg sync.WaitGroup
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, err := f.Wait()
				require.Error(t, err)
				assert.Equal(t, 0, got)
			}()
		}

		f.ResolveErr(fmt.Errorf("test error"))

		wg.Wait()
	})

	t.Run("multiple producers: has first resolved value", func(t *testing.T) {
		f := New[int]()

		var wg sync.WaitGroup
		for i := range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()

				f.ResolveValue(i + 1)
			}()
		}
		wg.Wait()

		got, err := f.Wait()
		require.NoError(t, err)

		assert.True(t, got >= 1 && got <= 11)
	})
}
