package filestorage

import (
	"context"
	"fmt"
	"testing"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
)

func TestCacheStore_Add(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		cs := newPSFixture(t)
		defer cs.Finish(t)
		testBlocks := newTestBocks("1", "2", "3")
		require.NoError(t, cs.Add(ctx, testBlocks))
		for _, b := range testBlocks {
			gb, err := cs.localStore.Get(ctx, b.Cid())
			assert.NoError(t, err)
			assert.NotNil(t, gb)
		}
	})
}

func TestCacheStore_Get(t *testing.T) {
	t.Run("exists local", func(t *testing.T) {
		ctx := context.Background()
		testBlocks := newTestBocks("1", "2", "3")
		cs := newPSFixture(t)
		defer cs.Finish(t)
		require.NoError(t, cs.localStore.Add(ctx, testBlocks))
		require.NoError(t, cs.origin.Add(ctx, testBlocks))
		for _, b := range testBlocks {
			gb, err := cs.Get(ctx, b.Cid())
			assert.NoError(t, err)
			assert.NotNil(t, gb)
		}
	})
	t.Run("exists remote", func(t *testing.T) {
		ctx := context.Background()
		testBlocks := newTestBocks("1", "2", "3")
		cs := newPSFixture(t)
		defer cs.Finish(t)
		require.NoError(t, cs.localStore.Add(ctx, testBlocks[:1]))
		require.NoError(t, cs.origin.Add(ctx, testBlocks))
		for _, b := range testBlocks {
			gb, err := cs.Get(ctx, b.Cid())
			assert.NoError(t, err)
			assert.NotNil(t, gb)
		}
		for _, b := range testBlocks {
			lb, err := cs.localStore.Get(ctx, b.Cid())
			assert.NoError(t, err)
			assert.NotNil(t, lb)
		}
	})
	t.Run("exists remote, no cache flag", func(t *testing.T) {
		ctx := ContextWithDoNotCache(context.Background())
		testBlocks := newTestBocks("1", "2", "3")
		cs := newPSFixture(t)
		defer cs.Finish(t)
		require.NoError(t, cs.origin.Add(ctx, testBlocks))
		for _, b := range testBlocks {
			gb, err := cs.Get(ctx, b.Cid())
			assert.NoError(t, err)
			assert.NotNil(t, gb)
		}
		for _, b := range testBlocks {
			lb, err := cs.localStore.Get(ctx, b.Cid())
			assert.Error(t, err)
			assert.Nil(t, lb)
		}
	})
}

func TestCacheStore_GetMany(t *testing.T) {
	t.Run("all local", func(t *testing.T) {
		ctx := context.Background()
		testBlocks := newTestBocks("1", "2", "3")
		cs := newPSFixture(t)
		defer cs.Finish(t)
		require.NoError(t, cs.localStore.Add(ctx, testBlocks))
		require.NoError(t, cs.origin.Add(ctx, testBlocks))

		var cids, resCids []cid.Cid
		for _, b := range testBlocks {
			cids = append(cids, b.Cid())
		}
		ch := cs.GetMany(ctx, cids)
		func() {
			for {
				select {
				case b, ok := <-ch:
					if !ok {
						return
					} else {
						resCids = append(resCids, b.Cid())
					}
				case <-time.After(time.Second):
					assert.NoError(t, fmt.Errorf("timeout"))
					return
				}
			}
		}()
		assert.ElementsMatch(t, cids, resCids)
	})
	t.Run("partial local", func(t *testing.T) {
		ctx := context.Background()
		testBlocks := newTestBocks("1", "2", "3")
		cs := newPSFixture(t)
		defer cs.Finish(t)
		require.NoError(t, cs.localStore.Add(ctx, testBlocks[:1]))
		require.NoError(t, cs.origin.Add(ctx, testBlocks))

		var cids, resCids []cid.Cid
		for _, b := range testBlocks {
			cids = append(cids, b.Cid())
		}
		ch := cs.GetMany(ctx, cids)
		func() {
			for {
				select {
				case b, ok := <-ch:
					if !ok {
						return
					} else {
						resCids = append(resCids, b.Cid())
					}
				case <-time.After(time.Second):
					assert.NoError(t, fmt.Errorf("timeout"))
					return
				}
			}
		}()
		require.Equal(t, len(cids), len(resCids))
		for _, b := range testBlocks {
			gb, err := cs.localStore.Get(ctx, b.Cid())
			assert.NoError(t, err)
			assert.NotNil(t, gb)
		}
	})
}

func TestCacheStore_Delete(t *testing.T) {
	ctx := context.Background()
	testBlocks := newTestBocks("1", "2", "3")
	cs := newPSFixture(t)
	defer cs.Finish(t)
	require.NoError(t, cs.localStore.Add(ctx, testBlocks))
	for _, b := range testBlocks {
		require.NoError(t, cs.Delete(ctx, b.Cid()))
		gb, err := cs.localStore.Get(ctx, b.Cid())
		assert.Nil(t, gb)
		assert.True(t, format.IsNotFound(err))
	}
}

type psFixture struct {
	*proxyStore
	tmpDir    string
	flatfsDir string

	proxyCancel func()
}

func newPSFixture(t *testing.T) *psFixture {
	var err error
	fx := &psFixture{}

	fx.flatfsDir = t.TempDir()
	sender := mock_event.NewMockSender(t)
	sender.EXPECT().Broadcast(mock.Anything).Maybe()
	cache, err := newFlatStore(fx.flatfsDir, sender, time.Second)
	require.NoError(t, err)

	proxyCtx, proxyCancel := context.WithCancel(context.Background())
	fx.proxyCancel = proxyCancel
	fx.proxyStore = &proxyStore{
		backgroundCtx:    proxyCtx,
		backgroundCancel: proxyCancel,
		localStore:       cache,
		origin:           rpcstore.NewInMemoryStore(1024 * 1024),
	}
	return fx
}

func (fx *psFixture) Finish(t *testing.T) {
	fx.proxyStore.Close()
}

func newTestBocks(ids ...string) (bs []blocks.Block) {
	for _, id := range ids {
		bs = append(bs, blocks.NewBlock([]byte(id)))
	}
	return
}
