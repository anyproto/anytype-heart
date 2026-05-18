package objectcache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app/ocache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubObject struct{}

func (stubObject) Close() error                         { return nil }
func (stubObject) TryClose(time.Duration) (bool, error) { return false, nil }

func TestObjectCache_FilterNotExists(t *testing.T) {
	t.Run("returns only ids absent from cache without triggering loads", func(t *testing.T) {
		c := &objectCache{
			cache: ocache.New(func(context.Context, string) (ocache.Object, error) {
				return nil, fmt.Errorf("loadFunc must not be called by FilterNotExists")
			}),
		}
		t.Cleanup(func() { _ = c.cache.Close() })

		require.NoError(t, c.cache.Add("cached1", stubObject{}))
		require.NoError(t, c.cache.Add("cached2", stubObject{}))

		got := c.FilterNotExists([]string{"cached1", "missing1", "cached2", "missing2"})

		assert.Equal(t, []string{"missing1", "missing2"}, got)
	})

	t.Run("empty input returns empty", func(t *testing.T) {
		c := &objectCache{cache: ocache.New(func(context.Context, string) (ocache.Object, error) {
			return nil, fmt.Errorf("unexpected load")
		})}
		t.Cleanup(func() { _ = c.cache.Close() })

		assert.Empty(t, c.FilterNotExists(nil))
	})
}
