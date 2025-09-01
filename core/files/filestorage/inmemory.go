package filestorage

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"

	"github.com/anyproto/anytype-heart/core/domain"
)

type inMemBlockStore struct {
	data map[string]blocks.Block
	mu   sync.Mutex
}

var _ FileStorage = (*inMemBlockStore)(nil)

// NewInMemory creates new in-memory store for testing purposes
func NewInMemory() FileStorage {
	return &inMemBlockStore{
		data: make(map[string]blocks.Block),
	}
}

func (i *inMemBlockStore) Init(a *app.App) (err error) {
	return
}

func (i *inMemBlockStore) Name() string {
	return fileblockstore.CName
}

func (i *inMemBlockStore) Run(ctx context.Context) (err error) {
	return
}

func (i *inMemBlockStore) Close(ctx context.Context) (err error) {
	return
}

func (i *inMemBlockStore) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if b := i.data[k.KeyString()]; b != nil {
		return b, nil
	}
	return nil, fileprotoerr.ErrCIDNotFound
}

func (i *inMemBlockStore) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	var result = make(chan blocks.Block, len(ks))
	defer close(result)
	for _, k := range ks {
		if b, err := i.Get(ctx, k); err == nil {
			result <- b
		}
	}
	return result
}

func (i *inMemBlockStore) Add(ctx context.Context, bs []blocks.Block) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, b := range bs {
		i.data[b.Cid().KeyString()] = b
	}
	return nil
}

func (i *inMemBlockStore) Delete(ctx context.Context, c cid.Cid) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.data, c.KeyString())
	return nil
}

func (i *inMemBlockStore) ExistsCids(ctx context.Context, ks []cid.Cid) (exists []cid.Cid, err error) {
	for _, k := range ks {
		if _, e := i.Get(ctx, k); e == nil {
			exists = append(exists, k)
		}
	}
	return
}

func (i *inMemBlockStore) NotExistsBlocks(ctx context.Context, bs []blocks.Block) (notExists []blocks.Block, err error) {
	for _, b := range bs {
		if _, e := i.Get(ctx, b.Cid()); e != nil {
			notExists = append(notExists, b)
		}
	}
	return
}

func (i *inMemBlockStore) LocalDiskUsage(ctx context.Context) (uint64, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	var size uint64
	for _, b := range i.data {
		size += uint64(len(b.RawData()))
	}
	return size, nil
}

func (i *inMemBlockStore) IterateFiles(ctx context.Context, iterFunc func(fileId domain.FullFileId)) error {
	return nil
}

func (i *inMemBlockStore) Batch(ctx context.Context) (Batch, error) {
	return &inMemBatch{store: i, pending: make([]blocks.Block, 0), deletes: make([]cid.Cid, 0)}, nil
}

// inMemBatch implements Batch for in-memory testing
type inMemBatch struct {
	store   *inMemBlockStore
	pending []blocks.Block
	deletes []cid.Cid
	mu      sync.Mutex
}

func (b *inMemBatch) Add(ctx context.Context, bs []blocks.Block) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pending = append(b.pending, bs...)
	return nil
}

func (b *inMemBatch) Delete(ctx context.Context, c cid.Cid) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.deletes = append(b.deletes, c)
	return nil
}

func (b *inMemBatch) Commit() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.store.mu.Lock()
	defer b.store.mu.Unlock()

	// Apply adds
	for _, block := range b.pending {
		b.store.data[block.Cid().KeyString()] = block
	}

	// Apply deletes
	for _, c := range b.deletes {
		delete(b.store.data, c.KeyString())
	}

	// Clear pending operations
	b.pending = make([]blocks.Block, 0)
	b.deletes = make([]cid.Cid, 0)
	return nil
}

func (b *inMemBatch) Discard() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Clear pending operations without applying them
	b.pending = make([]blocks.Block, 0)
	b.deletes = make([]cid.Cid, 0)
	return nil
}

// Get implements BlockStore interface for batch
func (b *inMemBatch) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	// First check pending blocks
	b.mu.Lock()
	for _, block := range b.pending {
		if block.Cid().Equals(k) {
			b.mu.Unlock()
			return block, nil
		}
	}
	// Check if it's in deletes
	for _, c := range b.deletes {
		if c.Equals(k) {
			b.mu.Unlock()
			return nil, format.ErrNotFound{Cid: k}
		}
	}
	b.mu.Unlock()

	// Fall back to store
	return b.store.Get(ctx, k)
}

// GetMany implements BlockStore interface for batch
func (b *inMemBatch) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	ch := make(chan blocks.Block)
	go func() {
		defer close(ch)
		for _, k := range ks {
			blk, err := b.Get(ctx, k)
			if err == nil {
				ch <- blk
			}
		}
	}()
	return ch
}

// ExistsCids implements BlockStoreLocal interface for batch
func (b *inMemBatch) ExistsCids(ctx context.Context, ks []cid.Cid) (exists []cid.Cid, err error) {
	for _, k := range ks {
		if _, err := b.Get(ctx, k); err == nil {
			exists = append(exists, k)
		}
	}
	return exists, nil
}

// NotExistsBlocks implements BlockStoreLocal interface for batch
func (b *inMemBatch) NotExistsBlocks(ctx context.Context, bs []blocks.Block) (notExists []blocks.Block, err error) {
	for _, block := range bs {
		if _, err := b.Get(ctx, block.Cid()); err != nil {
			notExists = append(notExists, block)
		}
	}
	return notExists, nil
}

// PartitionByExistence implements localStore interface for batch
func (b *inMemBatch) PartitionByExistence(ctx context.Context, ks []cid.Cid) (exist []cid.Cid, notExist []cid.Cid, err error) {
	for _, k := range ks {
		if _, err := b.Get(ctx, k); err == nil {
			exist = append(exist, k)
		} else {
			notExist = append(notExist, k)
		}
	}
	return exist, notExist, nil
}

// Close implements localStore interface for batch
func (b *inMemBatch) Close() error {
	return nil
}

func (i *inMemBlockStore) NewLocalStoreGarbageCollector() LocalStoreGarbageCollector {
	return &inMemGarbageCollector{store: i, using: map[string]struct{}{}}
}

type inMemGarbageCollector struct {
	store *inMemBlockStore
	using map[string]struct{}
}

func (i *inMemGarbageCollector) MarkAsUsing(cids []cid.Cid) {
	for _, c := range cids {
		i.using[c.KeyString()] = struct{}{}
	}
}

func (i *inMemGarbageCollector) CollectGarbage(ctx context.Context) error {
	i.store.mu.Lock()
	defer i.store.mu.Unlock()
	for k := range i.store.data {
		if _, ok := i.using[k]; !ok {
			delete(i.store.data, k)
		}
	}
	return nil
}
