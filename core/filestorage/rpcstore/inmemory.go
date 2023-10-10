package rpcstore

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
)

type inMemoryService struct{}

func NewInMemoryService() Service {
	return &inMemoryService{}
}

func (s *inMemoryService) Name() string          { return CName }
func (s *inMemoryService) Init(_ *app.App) error { return nil }
func (s *inMemoryService) NewStore() RpcStore    { return NewInMemoryStore() }

func NewInMemoryStore() RpcStore {
	ts := &inMemoryStore{
		store: make(map[cid.Cid]blocks.Block),
		files: make(map[string]map[cid.Cid]struct{}),
	}
	return ts
}

type inMemoryStore struct {
	store map[cid.Cid]blocks.Block
	files map[string]map[cid.Cid]struct{}
	// TODO Add spaces
	mu sync.Mutex
}

func (t *inMemoryStore) CheckAvailability(ctx context.Context, spaceID string, cids []cid.Cid) ([]*fileproto.BlockAvailability, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	checkResult := make([]*fileproto.BlockAvailability, 0, len(cids))
	for _, cid := range cids {
		status := fileproto.AvailabilityStatus_NotExists
		if _, ok := t.store[cid]; ok {
			status = fileproto.AvailabilityStatus_Exists
		}

		checkResult = append(checkResult, &fileproto.BlockAvailability{
			Cid:    cid.Bytes(),
			Status: status,
		})
	}
	return checkResult, nil
}

func (t *inMemoryStore) BindCids(ctx context.Context, spaceID string, fileID string, cids []cid.Cid) (err error) {
	// TODO implement
	return nil
}

func (t *inMemoryStore) AddToFile(ctx context.Context, spaceId string, fileId string, bs []blocks.Block) (err error) {
	// TODO Check limits!

	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.files[fileId]; !ok {
		t.files[fileId] = make(map[cid.Cid]struct{})
	}
	cids := t.files[fileId]
	for _, b := range bs {
		t.store[b.Cid()] = b
		cids[b.Cid()] = struct{}{}
	}
	return nil
}

func (t *inMemoryStore) DeleteFiles(ctx context.Context, spaceId string, fileIds ...string) (err error) {
	panic("not implemented")
}

func (t *inMemoryStore) SpaceInfo(ctx context.Context, spaceId string) (*fileproto.SpaceInfoResponse, error) {
	var info fileproto.SpaceInfoResponse
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, b := range t.store {
		info.UsageBytes += uint64(len(b.RawData()))
	}
	info.CidsCount = uint64(len(t.store))
	// TODO info.FilesCount after implementing file storage
	info.LimitBytes = 10 * 1024 * 1024
	return &info, nil
}

func (t *inMemoryStore) FilesInfo(ctx context.Context, spaceId string, fileIds ...string) ([]*fileproto.FileInfo, error) {
	panic("not implemented")
}

func (t *inMemoryStore) NotExistsBlocks(ctx context.Context, bs []blocks.Block) (notExists []blocks.Block, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	notExists = bs[:0]
	for _, b := range bs {
		if _, ok := t.store[b.Cid()]; !ok {
			notExists = append(notExists, b)
		}
	}
	return
}

func (t *inMemoryStore) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if b, ok := t.store[k]; ok {
		return b, nil
	}
	return nil, &format.ErrNotFound{Cid: k}
}

func (t *inMemoryStore) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
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

func (t *inMemoryStore) ExistsCids(ctx context.Context, ks []cid.Cid) (exists []cid.Cid, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, k := range ks {
		if _, ok := t.store[k]; ok {
			exists = append(exists, k)
		}
	}
	return
}

func (t *inMemoryStore) Add(ctx context.Context, bs []blocks.Block) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, b := range bs {
		t.store[b.Cid()] = b
	}
	return nil
}

func (t *inMemoryStore) AddAsync(ctx context.Context, bs []blocks.Block) (successCh chan cid.Cid) {
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

func (t *inMemoryStore) Delete(ctx context.Context, c cid.Cid) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.store[c]; ok {
		delete(t.store, c)
		return nil
	}
	return &format.ErrNotFound{Cid: c}
}

func (t *inMemoryStore) DeleteMany(ctx context.Context, cids ...cid.Cid) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, c := range cids {
		if _, ok := t.store[c]; ok {
			delete(t.store, c)
		}
	}
	return nil
}

func (t *inMemoryStore) Close() (err error) {
	return nil
}
