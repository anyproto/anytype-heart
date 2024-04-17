package rpcstore

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"

	"github.com/anyproto/anytype-heart/core/domain"
)

type inMemoryService struct {
	store RpcStore
}

// NewInMemoryService creates new service for testing purposes
func NewInMemoryService(store RpcStore) Service {
	return &inMemoryService{
		store: store,
	}
}

func (s *inMemoryService) Name() string          { return CName }
func (s *inMemoryService) Init(_ *app.App) error { return nil }
func (s *inMemoryService) NewStore() RpcStore    { return s.store }

// NewInMemoryStore creates new in-memory store for testing purposes
func NewInMemoryStore(limit int) RpcStore {
	ts := &inMemoryStore{
		store:      make(map[cid.Cid]blocks.Block),
		files:      make(map[domain.FileId]map[cid.Cid]struct{}),
		spaceFiles: map[string]map[domain.FileId]struct{}{},
		spaceCids:  map[string]map[cid.Cid]struct{}{},
		limit:      limit,
	}
	return ts
}

type inMemoryStore struct {
	store map[cid.Cid]blocks.Block
	files map[domain.FileId]map[cid.Cid]struct{}
	limit int
	// spaceId => fileId
	spaceFiles map[string]map[domain.FileId]struct{}
	// spaceId => cid
	spaceCids map[string]map[cid.Cid]struct{}
	mu        sync.Mutex
}

func (t *inMemoryStore) isCidBinded(spaceId string, cid cid.Cid) bool {
	if _, ok := t.spaceCids[spaceId]; !ok {
		return false
	}
	_, ok := t.spaceCids[spaceId][cid]
	return ok
}

func (t *inMemoryStore) CheckAvailability(ctx context.Context, spaceId string, cids []cid.Cid) ([]*fileproto.BlockAvailability, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	checkResult := make([]*fileproto.BlockAvailability, 0, len(cids))
	for _, cid := range cids {
		status := fileproto.AvailabilityStatus_NotExists
		if _, ok := t.store[cid]; ok {
			status = fileproto.AvailabilityStatus_Exists
		}
		if t.isCidBinded(spaceId, cid) {
			status = fileproto.AvailabilityStatus_ExistsInSpace
		}

		checkResult = append(checkResult, &fileproto.BlockAvailability{
			Cid:    cid.Bytes(),
			Status: status,
		})
	}
	return checkResult, nil
}

func (t *inMemoryStore) BindCids(ctx context.Context, spaceId string, fileId domain.FileId, cids []cid.Cid) (err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var bytesToBind int
	for _, cid := range cids {
		if !t.isCidBinded(spaceId, cid) {
			bytesToBind += len(t.store[cid].RawData())
		}
	}
	if !t.isWithinLimits(bytesToBind) {
		return fileprotoerr.ErrSpaceLimitExceeded
	}

	for _, cid := range cids {
		err = t.bindCid(spaceId, fileId, cid)
		if err != nil {
			return
		}
	}
	return nil
}

func (t *inMemoryStore) bindCid(spaceId string, fileId domain.FileId, cId cid.Cid) error {
	if _, ok := t.store[cId]; !ok {
		return fmt.Errorf("cid not exists: %s", cId)
	}

	if _, ok := t.spaceFiles[spaceId]; !ok {
		t.spaceFiles[spaceId] = make(map[domain.FileId]struct{})
	}
	t.spaceFiles[spaceId][fileId] = struct{}{}

	if _, ok := t.spaceCids[spaceId]; !ok {
		t.spaceCids[spaceId] = make(map[cid.Cid]struct{})
	}
	t.spaceCids[spaceId][cId] = struct{}{}
	return nil
}

func (t *inMemoryStore) AddToFile(ctx context.Context, spaceId string, fileId domain.FileId, bs []blocks.Block) (err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var bytesToAdd int
	for _, b := range bs {
		if !t.isCidBinded(spaceId, b.Cid()) {
			bytesToAdd += len(b.RawData())
		}
	}
	if !t.isWithinLimits(bytesToAdd) {
		return fileprotoerr.ErrSpaceLimitExceeded
	}

	if _, ok := t.files[fileId]; !ok {
		t.files[fileId] = make(map[cid.Cid]struct{})
	}
	cids := t.files[fileId]
	for _, b := range bs {
		t.store[b.Cid()] = b
		cids[b.Cid()] = struct{}{}
		err = t.bindCid(spaceId, fileId, b.Cid())
		if err != nil {
			return fmt.Errorf("bind cid: %w", err)
		}
	}
	return nil
}

func (t *inMemoryStore) DeleteFiles(ctx context.Context, spaceId string, fileIds ...domain.FileId) (err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.spaceFiles[spaceId]; !ok {
		return fmt.Errorf("spaceFiles not found: %s", spaceId)
	}
	if _, ok := t.spaceCids[spaceId]; !ok {
		return fmt.Errorf("spaceCids not found: %s", spaceId)
	}

	for _, fileId := range fileIds {
		_, ok := t.spaceFiles[spaceId][fileId]
		if ok {
			delete(t.spaceFiles[spaceId], fileId)
			for cId := range t.files[fileId] {
				delete(t.spaceCids[spaceId], cId)
			}
			delete(t.files, fileId)
		}
	}
	return nil
}

func (t *inMemoryStore) SpaceInfo(ctx context.Context, spaceId string) (*fileproto.SpaceInfoResponse, error) {
	panic("not implemented")
}

func (t *inMemoryStore) AccountInfo(ctx context.Context) (*fileproto.AccountInfoResponse, error) {
	var info fileproto.AccountInfoResponse
	t.mu.Lock()
	defer t.mu.Unlock()
	info.TotalUsageBytes = uint64(t.getTotalUsage())
	info.TotalCidsCount = uint64(len(t.store))
	info.LimitBytes = uint64(t.limit)

	for spaceId, files := range t.spaceFiles {
		var spaceUsageBytes int
		for fileId := range files {
			for cid := range t.files[fileId] {
				spaceUsageBytes += len(t.store[cid].RawData())
			}
		}
		info.Spaces = append(info.Spaces, &fileproto.SpaceInfoResponse{
			SpaceId:         spaceId,
			SpaceUsageBytes: uint64(spaceUsageBytes),
			TotalUsageBytes: info.TotalUsageBytes,
			FilesCount:      uint64(len(files)),
			CidsCount:       uint64(len(t.spaceCids[spaceId])),
			LimitBytes:      info.LimitBytes,
		})
	}
	return &info, nil
}

func (t *inMemoryStore) getTotalUsage() int {
	var totalUsage int
	for _, b := range t.store {
		totalUsage += len(b.RawData())
	}
	return totalUsage
}

func (t *inMemoryStore) isWithinLimits(bytesToUpload int) bool {
	return t.getTotalUsage()+bytesToUpload <= t.limit
}

func (t *inMemoryStore) FilesInfo(ctx context.Context, spaceId string, fileIds ...domain.FileId) ([]*fileproto.FileInfo, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	infos := make([]*fileproto.FileInfo, 0, len(fileIds))
	for _, fileId := range fileIds {
		info, err := t.fileInfo(spaceId, fileId)
		if err != nil {
			return nil, fmt.Errorf("file info: %w", err)
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (t *inMemoryStore) fileInfo(spaceId string, fileId domain.FileId) (*fileproto.FileInfo, error) {
	fileChunkCids, ok := t.files[fileId]
	if !ok {
		return nil, fmt.Errorf("file not found")
	}
	if _, ok := t.spaceFiles[spaceId][fileId]; !ok {
		return nil, fmt.Errorf("file not found in space")
	}

	info := fileproto.FileInfo{
		FileId: fileId.String(),
	}
	for cId := range fileChunkCids {
		block, ok := t.store[cId]
		if !ok {
			return nil, fmt.Errorf("block not found")
		}
		info.CidsCount++
		info.UsageBytes += uint64(len(block.RawData()))
	}
	return &info, nil
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
