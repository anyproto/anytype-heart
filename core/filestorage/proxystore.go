package filestorage

import (
	"context"
	"fmt"
	"io"

	"github.com/dgraph-io/badger/v3"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	dshelp "github.com/ipfs/go-ipfs-ds-help"
	format "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
)

const CtxKeyRemoteLoadDisabled = "object_remote_load_disabled"

type proxyStore struct {
	localStore *flatStore
	origin     rpcstore.RpcStore

	oldStore *badger.DB
}

func (c *proxyStore) Get(ctx context.Context, k cid.Cid) (b blocks.Block, err error) {
	b, err = c.getFromOldStore(k)
	if err == nil {
		return b, nil
	}

	log.Debug("get cid", zap.String("cid", k.String()))
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
		return nil, fmt.Errorf("remote load disabled")
	}
	if b, err = c.origin.Get(ctx, k); err != nil {
		log.Debug("proxyStore remote cid error", zap.String("cid", k.String()), zap.Error(err))
		return
	}
	if addErr := c.localStore.Add(ctx, []blocks.Block{b}); addErr != nil {
		log.Error("block fetched from origin but got error for add to localStore", zap.Error(addErr))
	}
	return
}

func (c *proxyStore) getFromOldStore(k cid.Cid) (blocks.Block, error) {
	if c.oldStore == nil {
		return nil, fmt.Errorf("old store is not used")
	}
	dsKey := cidToOldDsKey(k)
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

func cidToOldDsKey(k cid.Cid) string {
	return "/blocks" + dshelp.MultihashToDsKey(k.Hash()).String()
}

func (c *proxyStore) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	remaining, oldResults := c.getManyFromOldStore(ks)
	if len(remaining) == 0 {
		return oldResults
	}
	gotFromOldStore := len(ks) - len(remaining)

	fromCache, fromOrigin, localErr := c.localStore.PartitionByExistence(ctx, remaining)
	if localErr != nil {
		log.Error("proxy store hasCIDs error", zap.Error(localErr))
		fromOrigin = ks
	}
	log.Debug("get many cids", zap.Int("cached", len(fromCache)), zap.Int("origin", len(fromOrigin)))
	if len(fromOrigin) == 0 && gotFromOldStore == 0 {
		return c.localStore.GetMany(ctx, fromCache)
	}
	results := make(chan blocks.Block)

	go func() {
		defer close(results)
		localResults := c.localStore.GetMany(ctx, fromCache)
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
					if addErr := c.localStore.Add(ctx, []blocks.Block{ob}); addErr != nil {
						log.Error("add block to localStore error", zap.Error(addErr))
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
	results = make(chan blocks.Block)

	if c.oldStore == nil {
		close(results)
		return ks, results
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
			dsKey := cidToOldDsKey(k)
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
		close(results)
		return remaining, results
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
	if c.oldStore != nil {
		if err := c.oldStore.Close(); err != nil {
			log.Error("error while closing old store", zap.Error(err))
		}
	}
	if err := c.localStore.Close(); err != nil {
		log.Error("error while closing localStore store", zap.Error(err))
	}
	if closer, ok := c.origin.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
