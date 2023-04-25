package filestorage

import (
	"context"
	"fmt"
	"io"

	"github.com/anytypeio/any-sync/commonfile/fileblockstore"
	"github.com/dgraph-io/badger/v3"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	dshelp "github.com/ipfs/go-ipfs-ds-help"
	format "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/filestorage/rpcstore"
)

type proxyStore struct {
	cache  *flatStore
	origin rpcstore.RpcStore
	index  *FileBadgerIndex

	oldStore *badger.DB
}

func (c *proxyStore) Get(ctx context.Context, k cid.Cid) (b blocks.Block, err error) {
	b, err = c.getFromOldStore(k)
	if err == nil {
		return b, nil
	}

	log.Debug("get cid", zap.String("cid", k.String()))
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

func (c *proxyStore) getFromOldStore(k cid.Cid) (blocks.Block, error) {
	if c.oldStore == nil {
		return nil, fmt.Errorf("old store is not used")
	}
	dsKey := cidToDsKey(k)
	var b blocks.Block
	err := c.oldStore.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(dsKey))
		if err != nil {
			return err
		}
		err = item.Value(func(val []byte) error {
			b, err = blocks.NewBlockWithCid(val, k)
			return err
		})
		return err
	})
	if err == nil {
		return b, nil
	}
	if err != nil && err != badger.ErrKeyNotFound {
		log.Error("get from old store", zap.String("cid", k.String()), zap.String("key", dsKey), zap.Error(err))
	}
	return nil, err
}

func cidToDsKey(k cid.Cid) string {
	return "/blocks" + dshelp.MultihashToDsKey(k.Hash()).String()
}

func (c *proxyStore) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	remaining, oldResults := c.getManyFromOldStore(ks)
	if len(remaining) == 0 {
		return oldResults
	}
	gotFromOldStore := len(ks) - len(remaining)

	fromCache, fromOrigin, localErr := c.cache.PartitionByExistence(ctx, remaining)
	if localErr != nil {
		log.Error("proxy store hasCIDs error", zap.Error(localErr))
		fromOrigin = ks
	}
	log.Debug("get many cids", zap.Int("cached", len(fromCache)), zap.Int("origin", len(fromOrigin)))
	if len(fromOrigin) == 0 && gotFromOldStore == 0 {
		return c.cache.GetMany(ctx, fromCache)
	}
	results := make(chan blocks.Block)

	go func() {
		defer close(results)
		localResults := c.cache.GetMany(ctx, fromCache)
		originResults := c.origin.GetMany(ctx, fromOrigin)
		oOk, cOk, oldOk := true, true, true
		for {
			var cb, ob, b blocks.Block
			select {
			case b, oldOk = <-oldResults:
				if oldOk {
					results <- b
				}
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
			if !oOk && !cOk && !oldOk {
				return
			}
		}
	}()
	return results
}

func (c *proxyStore) getManyFromOldStore(ks []cid.Cid) (remaining []cid.Cid, results chan blocks.Block) {
	if c.oldStore == nil {
		return ks, nil
	}

	get := func(txn *badger.Txn, k cid.Cid, dsKey string) (blocks.Block, error) {
		item, err := txn.Get([]byte(dsKey))
		if err != nil {
			return nil, err
		}
		var b blocks.Block
		err = item.Value(func(val []byte) error {
			b, err = blocks.NewBlockWithCid(val, k)
			return err
		})
		return b, err
	}

	var bs []blocks.Block
	err := c.oldStore.View(func(txn *badger.Txn) error {
		for _, k := range ks {
			dsKey := cidToDsKey(k)
			b, err := get(txn, k, dsKey)
			if err != nil {
				remaining = append(remaining, k)
				if err != badger.ErrKeyNotFound {
					log.Error("get many from old store", zap.String("cid", k.String()), zap.String("key", dsKey), zap.Error(err))
				}
				continue
			}
			bs = append(bs, b)
		}
		return nil
	})
	if err != nil {
		log.Error("get many from old store: view tx", zap.Error(err))
		return remaining, nil
	}

	results = make(chan blocks.Block)
	go func() {
		defer close(results)
		for _, b := range bs {
			results <- b
		}
	}()
	return remaining, results
}

func (c *proxyStore) Add(ctx context.Context, bs []blocks.Block) (err error) {
	if bs, err = c.cache.NotExistsBlocks(ctx, bs); err != nil {
		return
	}
	if len(bs) == 0 {
		return nil
	}
	if err = c.cache.Add(ctx, bs); err != nil {
		return
	}
	indexCids := NewCids()
	defer indexCids.Release()
	for _, b := range bs {
		indexCids.Add(fileblockstore.CtxGetSpaceId(ctx), OpAdd, b.Cid())
	}
	return c.index.Add(indexCids)
}

func (c *proxyStore) Delete(ctx context.Context, k cid.Cid) error {
	if err := c.cache.Delete(ctx, k); err != nil {
		return err
	}
	indexCids := NewCids()
	defer indexCids.Release()
	indexCids.Add(fileblockstore.CtxGetSpaceId(ctx), OpDelete, k)
	return c.index.Add(indexCids)
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
