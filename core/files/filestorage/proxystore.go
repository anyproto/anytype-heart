package filestorage

import (
	"context"
	"fmt"
	"io"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
)

type ctxKey string

const CtxKeyRemoteLoadDisabled = ctxKey("object_remote_load_disabled")
const CtxDoNotCache = ctxKey("do_not_cache")

var ErrRemoteLoadDisabled = fmt.Errorf("remote load disabled")

func ContextWithDoNotCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, CtxDoNotCache, true)
}

type proxyStore struct {
	localStore *flatStore
	origin     rpcstore.RpcStore

	backgroundCtx    context.Context
	backgroundCancel context.CancelFunc
}

func newProxyStore(localStore *flatStore, origin rpcstore.RpcStore) *proxyStore {
	ctx, cancel := context.WithCancel(context.Background())
	return &proxyStore{
		localStore:       localStore,
		origin:           origin,
		backgroundCtx:    ctx,
		backgroundCancel: cancel,
	}
}

func (c *proxyStore) Get(ctx context.Context, k cid.Cid) (b blocks.Block, err error) {
	if b, err = c.localStore.Get(ctx, k); err != nil {
		if format.IsNotFound(err) {
			err = nil
			log.Debug("proxyStore local cid not found", zap.String("cid", k.String()))
		} else {
			return
		}
	} else {
		return
	}
	v, ok := ctx.Value(CtxKeyRemoteLoadDisabled).(bool)
	if ok && v {
		return nil, ErrRemoteLoadDisabled
	}
	if b, err = c.origin.Get(ctx, k); err != nil {
		log.Debug("proxyStore remote cid error", zap.String("cid", k.String()), zap.Error(err))
		return
	}

	if dontCache, ok := ctx.Value(CtxDoNotCache).(bool); !ok || !dontCache {
		if addErr := c.localStore.Add(ctx, []blocks.Block{b}); addErr != nil {
			log.Error("block fetched from origin but got error for add to localStore", zap.Error(addErr))
		}
	}
	return
}

func (c *proxyStore) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	fromCache, fromOrigin, localErr := c.localStore.PartitionByExistence(ctx, ks)
	if localErr != nil {
		log.Error("proxy store hasCIDs error", zap.Error(localErr))
		fromOrigin = ks
	}
	log.Debug("get many cids", zap.Int("cached", len(fromCache)), zap.Int("origin", len(fromOrigin)))
	if len(fromOrigin) == 0 {
		return c.localStore.GetMany(ctx, fromCache)
	}
	results := make(chan blocks.Block)

	go func() {
		defer close(results)
		localResults := c.localStore.GetMany(ctx, fromCache)
		originResults := c.origin.GetMany(ctx, fromOrigin)
		oOk, cOk := true, true
		for {
			var cb, ob blocks.Block
			select {
			case cb, cOk = <-localResults:
				if cOk {
					results <- cb
				}
			case ob, oOk = <-originResults:
				if oOk {
					if addErr := c.localStore.Add(ctx, []blocks.Block{ob}); addErr != nil {
						log.Error("add block to localStore error", zap.Error(addErr))
					}
					results <- ob
				}
			case <-ctx.Done():
				return
			case <-c.backgroundCtx.Done():
				return
			}
			if !oOk && !cOk {
				return
			}
		}
	}()
	return results
}

func (c *proxyStore) Add(ctx context.Context, bs []blocks.Block) (err error) {
	if bs, err = c.localStore.NotExistsBlocks(ctx, bs); err != nil {
		return
	}
	if len(bs) == 0 {
		return nil
	}
	return c.localStore.Add(ctx, bs)
}

func (c *proxyStore) Delete(ctx context.Context, k cid.Cid) error {
	return c.localStore.Delete(ctx, k)
}

func (c *proxyStore) ExistsCids(ctx context.Context, ks []cid.Cid) (exist []cid.Cid, err error) {
	exist, _, err = c.localStore.PartitionByExistence(ctx, ks)
	return
}

func (c *proxyStore) NotExistsBlocks(ctx context.Context, bs []blocks.Block) (notExists []blocks.Block, err error) {
	return c.localStore.NotExistsBlocks(ctx, bs)
}

func (c *proxyStore) Close() error {
	if c.backgroundCancel != nil {
		c.backgroundCancel()
	}
	if err := c.localStore.Close(); err != nil {
		log.Error("error while closing localStore store", zap.Error(err))
	}
	if closer, ok := c.origin.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
