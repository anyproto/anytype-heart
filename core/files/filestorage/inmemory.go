package filestorage

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"

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
