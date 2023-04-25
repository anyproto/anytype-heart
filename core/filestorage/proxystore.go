package filestorage

import (
	"context"
	"io"

	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-libipfs/blocks"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/filestorage/rpcstore"
)

type proxyStore struct {
	cache  *flatStore
	origin rpcstore.RpcStore
}

func (c *proxyStore) Get(ctx context.Context, k cid.Cid) (b blocks.Block, err error) {
	if b, err = c.cache.Get(ctx, k); err != nil {
		if format.IsNotFound(err) {
			err = nil
			log.Debug("proxyStore local cid not found", zap.String("cid", k.String()))
		} else {
			return
		}
	} else {
		return
	}
	if b, err = c.origin.Get(ctx, k); err != nil {
		log.Debug("proxyStore remote cid error", zap.String("cid", k.String()), zap.Error(err))
		return
	}
	if addErr := c.cache.Add(ctx, []blocks.Block{b}); addErr != nil {
		log.Error("block fetched from origin but got error for add to cache", zap.Error(addErr))
	}
	return
}

func (c *proxyStore) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	fromCache, fromOrigin, localErr := c.cache.PartitionByExistence(ctx, ks)
	if localErr != nil {
		log.Error("proxy store hasCIDs error", zap.Error(localErr))
		fromOrigin = ks
	}
	log.Debug("get many cids", zap.Int("cached", len(fromCache)), zap.Int("origin", len(fromOrigin)))
	if len(fromOrigin) == 0 {
		return c.cache.GetMany(ctx, fromCache)
	}
	results := make(chan blocks.Block)

	go func() {
		defer close(results)
		localResults := c.cache.GetMany(ctx, fromCache)
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
					if addErr := c.cache.Add(ctx, []blocks.Block{ob}); addErr != nil {
						log.Error("add block to cache error", zap.Error(addErr))
					}
					results <- ob
				}
			case <-ctx.Done():
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
	if bs, err = c.cache.NotExistsBlocks(ctx, bs); err != nil {
		return
	}
	if len(bs) == 0 {
		return nil
	}
	return c.cache.Add(ctx, bs)
}

func (c *proxyStore) Delete(ctx context.Context, k cid.Cid) error {
	return c.cache.Delete(ctx, k)
}

func (c *proxyStore) ExistsCids(ctx context.Context, ks []cid.Cid) (exist []cid.Cid, err error) {
	exist, _, err = c.cache.PartitionByExistence(ctx, ks)
	return
}

func (c *proxyStore) NotExistsBlocks(ctx context.Context, bs []blocks.Block) (notExists []blocks.Block, err error) {
	return c.cache.NotExistsBlocks(ctx, bs)
}

func (c *proxyStore) Close() error {
	if err := c.cache.Close(); err != nil {
		log.Error("error while closing cache store", zap.Error(err))
	}
	if closer, ok := c.origin.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
