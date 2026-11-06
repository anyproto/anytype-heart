package spaceloader

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/clientspace"
)

type fakeSpaceServiceProvider struct {
	openErr error
	opens   atomic.Int32
}

func (f *fakeSpaceServiceProvider) open(ctx context.Context) (clientspace.Space, error) {
	f.opens.Add(1)
	return nil, f.openErr
}

func (f *fakeSpaceServiceProvider) onLoad(sp clientspace.Space, loadErr error) error {
	return nil
}

func TestLoadRetry(t *testing.T) {
	t.Run("collection not found terminates the load instead of retrying forever", func(t *testing.T) {
		// given
		provider := &fakeSpaceServiceProvider{openErr: fmt.Errorf("build space: %w", anystore.ErrCollectionNotFound)}
		ls := &loadingSpace{
			retryTimeout:         time.Millisecond * 10,
			spaceServiceProvider: provider,
			loadCh:               make(chan struct{}),
		}

		// when
		go ls.loadRetry(context.Background())

		// then
		select {
		case <-ls.loadCh:
		case <-time.After(time.Second * 2):
			t.Fatal("load did not terminate on collection not found")
		}
		require.ErrorIs(t, ls.getLoadErr(), anystore.ErrCollectionNotFound)
		assert.Equal(t, int32(1), provider.opens.Load(), "must not retry a corrupt local store")
	})

	t.Run("generic build error keeps retrying", func(t *testing.T) {
		// given
		provider := &fakeSpaceServiceProvider{openErr: errors.New("transient network error")}
		ls := &loadingSpace{
			retryTimeout:         time.Millisecond * 10,
			spaceServiceProvider: provider,
			loadCh:               make(chan struct{}),
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// when
		go ls.loadRetry(ctx)

		// then
		assert.Eventually(t, func() bool { return provider.opens.Load() > 1 }, time.Second*5, time.Millisecond*20)
		cancel()
		select {
		case <-ls.loadCh:
		case <-time.After(time.Second * 2):
			t.Fatal("load did not terminate on context cancel")
		}
	})
}
