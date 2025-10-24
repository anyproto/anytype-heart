package filestorage

import (
	"context"

	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
)

// BlockStoreBatch combines localStore interface with batch operations
// This is what's returned by flatStore.Batch() and implements all read/write operations
// with temp directory support via the anyproto fork of flatfs
type BlockStoreBatch interface {
	localStore
	Commit() error
	Discard() error
}

// batchProxy wraps a proxyStore and batch to implement the Batch interface
type batchProxy struct {
	*proxyStore
	batch BlockStoreBatch
}

// newBatchProxy creates a new batch that uses its own proxyStore with temp directory support
func newBatchProxy(flatStore *flatStore, origin rpcstore.RpcStore) (*batchProxy, error) {
	// Create a batch from flatStore that can read from both temp and main directories
	batch, err := flatStore.Batch(context.Background())
	if err != nil {
		return nil, err
	}

	// Create a new proxyStore with the batch as the local store
	// The batch already implements all necessary read methods with temp dir support
	proxy := newProxyStore(batch, origin)

	return &batchProxy{
		proxyStore: proxy,
		batch:      batch,
	}, nil
}

// Commit commits the batch, moving files from temp to main directory
func (b *batchProxy) Commit() error {
	return b.batch.Commit()
}

// Discard discards the batch, removing temp directory contents
func (b *batchProxy) Discard() error {
	return b.batch.Discard()
}
