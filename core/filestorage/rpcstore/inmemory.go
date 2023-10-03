package rpcstore

import (
	"context"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"sync"
)

type inMemoryService struct{}

func NewInMemoryService() Service {
	return &inMemoryService{}
}

func (s *inMemoryService) Name() string          { return CName }
func (s *inMemoryService) Init(_ *app.App) error { return nil }
func (s *inMemoryService) NewStore() RpcStore    { return NewInMemoryStore() }

func NewInMemoryStore() RpcStore {
	ts := &testStore{
		store: make(map[string]blocks.Block),
	}
	return ts
}

type testStore struct {
	store map[string]blocks.Block
	mu    sync.Mutex
}

func (t *testStore) CheckAvailability(ctx context.Context, spaceID string, cids []cid.Cid) (checkResult []*fileproto.BlockAvailability, err error) {
	return nil, nil
}

func (t *testStore) BindCids(ctx context.Context, spaceID string, fileID string, cids []cid.Cid) (err error) {
	return nil
}

func (t *testStore) AddToFile(ctx context.Context, spaceId string, fileId string, bs []blocks.Block) (err error) {
	panic("not implemented")
}

func (t *testStore) DeleteFiles(ctx context.Context, spaceId string, fileIds ...string) (err error) {
	panic("not implemented")
}

func (t *testStore) SpaceInfo(ctx context.Context, spaceId string) (info *fileproto.SpaceInfoResponse, err error) {
	panic("not implemented")
}

func (t *testStore) FilesInfo(ctx context.Context, spaceId string, fileIds ...string) ([]*fileproto.FileInfo, error) {
	panic("not implemented")
}

func (t *testStore) NotExistsBlocks(ctx context.Context, bs []blocks.Block) (notExists []blocks.Block, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	notExists = bs[:0]
	for _, b := range bs {
		if _, ok := t.store[b.Cid().String()]; !ok {
			notExists = append(notExists, b)
		}
	}
	return
}

func (t *testStore) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if b, ok := t.store[k.String()]; ok {
		return b, nil
	}
	return nil, &format.ErrNotFound{Cid: k}
}

func (t *testStore) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
	var result = make(chan blocks.Block)
	go func() {
		defer close(result)
		for _, k := range ks {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if b, err := t.Get(ctx, k); err == nil {
				result <- b
			}
		}
	}()
	return result
}

func (t *testStore) ExistsCids(ctx context.Context, ks []cid.Cid) (exists []cid.Cid, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, k := range ks {
		if _, ok := t.store[k.String()]; ok {
			exists = append(exists, k)
		}
	}
	return
}

func (t *testStore) Add(ctx context.Context, bs []blocks.Block) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, b := range bs {
		t.store[b.Cid().String()] = b
	}
	return nil
}

func (t *testStore) AddAsync(ctx context.Context, bs []blocks.Block) (successCh chan cid.Cid) {
	successCh = make(chan cid.Cid, len(bs))
	go func() {
		defer close(successCh)
		for _, b := range bs {
			if err := t.Add(ctx, []blocks.Block{b}); err == nil {
				successCh <- b.Cid()
			}
		}
	}()
	return successCh
}

func (t *testStore) Delete(ctx context.Context, c cid.Cid) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.store[c.String()]; ok {
		delete(t.store, c.String())
		return nil
	}
	return &format.ErrNotFound{Cid: c}
}

func (t *testStore) DeleteMany(ctx context.Context, cids ...cid.Cid) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, c := range cids {
		if _, ok := t.store[c.String()]; ok {
			delete(t.store, c.String())
		}
	}
	return nil
}

func (t *testStore) Close() (err error) {
	return nil
}
