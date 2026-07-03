package spacev2

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func testFactory(created *atomic.Int32) controllerFactory {
	return func(spaceId string) (*controller, error) {
		created.Add(1)
		return newController(spaceId, newFakeBackend(spaceinfo.AccountStatusActive), controllerOptions{
			retryMin: 2 * time.Millisecond,
			retryMax: 10 * time.Millisecond,
		}), nil
	}
}

func TestRegistryGetOrCreateIdempotent(t *testing.T) {
	// given
	var created atomic.Int32
	r := newRegistry(testFactory(&created))
	t.Cleanup(func() { require.NoError(t, r.close(context.Background())) })

	// when: many concurrent callers race for the same space
	const callers = 16
	ctrls := make([]*controller, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, err := r.getOrCreate("spaceA")
			assert.NoError(t, err)
			ctrls[i] = c
		}(i)
	}
	wg.Wait()

	// then: exactly one controller exists
	assert.Equal(t, int32(1), created.Load())
	for i := 1; i < callers; i++ {
		assert.Same(t, ctrls[0], ctrls[i])
	}
	assert.Same(t, ctrls[0], r.get("spaceA"))
	assert.Nil(t, r.get("spaceB"))
}

func TestRegistryFactoryErrorNotCached(t *testing.T) {
	// given: a factory that fails once, then succeeds
	var calls atomic.Int32
	factoryErr := errors.New("no space view yet")
	r := newRegistry(func(spaceId string) (*controller, error) {
		if calls.Add(1) == 1 {
			return nil, factoryErr
		}
		return newController(spaceId, newFakeBackend(spaceinfo.AccountStatusActive), controllerOptions{
			retryMin: 2 * time.Millisecond,
			retryMax: 10 * time.Millisecond,
		}), nil
	})
	t.Cleanup(func() { require.NoError(t, r.close(context.Background())) })

	// when
	_, err := r.getOrCreate("spaceA")

	// then: the failure surfaces but poisons nothing
	require.ErrorIs(t, err, factoryErr)
	assert.Nil(t, r.get("spaceA"))

	// and: the next attempt succeeds
	c, err := r.getOrCreate("spaceA")
	require.NoError(t, err)
	require.NotNil(t, c)
}

func TestRegistryCloseAll(t *testing.T) {
	// given
	var created atomic.Int32
	r := newRegistry(testFactory(&created))
	a, err := r.getOrCreate("spaceA")
	require.NoError(t, err)
	b, err := r.getOrCreate("spaceB")
	require.NoError(t, err)
	require.Len(t, r.all(), 2)

	// when
	require.NoError(t, r.close(context.Background()))

	// then: all controllers are closed and new ones are refused
	assert.Equal(t, StateClosed, a.State())
	assert.Equal(t, StateClosed, b.State())
	_, err = r.getOrCreate("spaceC")
	assert.ErrorIs(t, err, ErrClosed)

	// and: closing again is a no-op
	require.NoError(t, r.close(context.Background()))
}
