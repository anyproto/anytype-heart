package rpcstore

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"

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
func NewInMemoryStore(limit int) *InMemoryStore {
	ts := &InMemoryStore{
		store:      make(map[cid.Cid]blocks.Block),
		files:      make(map[domain.FileId]map[cid.Cid]struct{}),
		spaceFiles: map[string]map[domain.FileId]struct{}{},
		spaceCids:  map[string]map[cid.Cid]struct{}{},
		stats:      &InMemoryStoreStats{},
		limit:      limit,
	}
	return ts
}

var _ RpcStore = (*InMemoryStore)(nil)

type InMemoryStore struct {
	store map[cid.Cid]blocks.Block
	files map[domain.FileId]map[cid.Cid]struct{}
	limit int
	// spaceId => fileId
	spaceFiles map[string]map[domain.FileId]struct{}
	// spaceId => cid
	spaceCids map[string]map[cid.Cid]struct{}
	mu        sync.Mutex

	stats *InMemoryStoreStats
}

type InMemoryStoreStats struct {
	cidsBinded   uint64
	blocksAdded  uint64
	filesDeleted uint64
}

func (s *InMemoryStoreStats) CidsBinded() uint64 {
	return atomic.LoadUint64(&s.cidsBinded)
}

func (s *InMemoryStoreStats) BlocksAdded() uint64 {
	return atomic.LoadUint64(&s.blocksAdded)
}

func (s *InMemoryStoreStats) FilesDeleted() uint64 {
	return atomic.LoadUint64(&s.filesDeleted)
}

func (s *InMemoryStoreStats) bindCid() {
	atomic.AddUint64(&s.cidsBinded, 1)
}

func (s *InMemoryStoreStats) addBlock() {
	atomic.AddUint64(&s.blocksAdded, 1)
}

func (s *InMemoryStoreStats) deleteFile() {
	atomic.AddUint64(&s.filesDeleted, 1)
}

func (t *InMemoryStore) Stats() *InMemoryStoreStats {
	return t.stats
}

func (t *InMemoryStore) isCidBinded(spaceId string, cid cid.Cid) bool {
	if _, ok := t.spaceCids[spaceId]; !ok {
		return false
	}
	_, ok := t.spaceCids[spaceId][cid]
	return ok
}

func (t *InMemoryStore) CheckAvailability(ctx context.Context, spaceId string, cids []cid.Cid) ([]*fileproto.BlockAvailability, error) {
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

func (t *InMemoryStore) BindCids(ctx context.Context, spaceId string, fileId domain.FileId, cids []cid.Cid) (err error) {
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
		t.stats.bindCid()
	}
	return nil
}

func (t *InMemoryStore) bindCid(spaceId string, fileId domain.FileId, cId cid.Cid) error {
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

func (t *InMemoryStore) AddToFileMany(ctx context.Context, req *fileproto.BlockPushManyRequest) (err error) {
	for _, fb := range req.FileBlocks {
		bs := make([]blocks.Block, 0, len(fb.Blocks))
		for _, b := range fb.Blocks {
			c, err := cid.Cast(b.Cid)
			if err != nil {
				return fmt.Errorf("cast cid: %w", err)
			}
			newBl, err := blocks.NewBlockWithCid(b.Data, c)
			if err != nil {
				return fmt.Errorf("new block: %w", err)
			}
			bs = append(bs, *newBl)
		}
		err = t.AddToFile(ctx, fb.SpaceId, domain.FileId(fb.FileId), bs)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *InMemoryStore) AddToFile(ctx context.Context, spaceId string, fileId domain.FileId, bs []blocks.Block) (err error) {
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
		t.stats.addBlock()
	}
	return nil
}

func (t *InMemoryStore) IterateFiles(ctx context.Context, iterFunc func(fileId domain.FullFileId)) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	for spaceId, spaceFiles := range t.spaceFiles {
		for fileId := range spaceFiles {
			iterFunc(domain.FullFileId{SpaceId: spaceId, FileId: fileId})
		}
	}
	return nil
}

func (t *InMemoryStore) DeleteFiles(ctx context.Context, spaceId string, fileIds ...domain.FileId) (err error) {
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
			t.stats.deleteFile()
		}
	}
	return nil
}

func (t *InMemoryStore) SpaceInfo(ctx context.Context, spaceId string) (*fileproto.SpaceInfoResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	var spaceUsageBytes int
	files := t.spaceFiles[spaceId]
	for fileId := range files {
		for cid := range t.files[fileId] {
			spaceUsageBytes += len(t.store[cid].RawData())
		}
	}
	return &fileproto.SpaceInfoResponse{
		SpaceId:         spaceId,
		SpaceUsageBytes: uint64(spaceUsageBytes),
		TotalUsageBytes: uint64(t.getTotalUsage()),
		FilesCount:      uint64(len(files)),
		CidsCount:       uint64(len(t.spaceCids[spaceId])),
		LimitBytes:      uint64(t.limit),
	}, nil
}

func (t *InMemoryStore) SetLimit(limit int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.limit = limit
}

func (t *InMemoryStore) AccountInfo(ctx context.Context) (*fileproto.AccountInfoResponse, error) {
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

func (t *InMemoryStore) getTotalUsage() int {
	var totalUsage int
	for _, b := range t.store {
		totalUsage += len(b.RawData())
	}
	return totalUsage
}

func (t *InMemoryStore) isWithinLimits(bytesToUpload int) bool {
	return t.getTotalUsage()+bytesToUpload <= t.limit
}

func (t *InMemoryStore) FilesInfo(ctx context.Context, spaceId string, fileIds ...domain.FileId) ([]*fileproto.FileInfo, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	infos := make([]*fileproto.FileInfo, 0, len(fileIds))
	for _, fileId := range fileIds {
		info, err := t.fileInfo(spaceId, fileId)
		if err != nil {
			log.Error("file info", zap.String("fileId", fileId.String()), zap.Error(err))
			continue
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (t *InMemoryStore) fileInfo(spaceId string, fileId domain.FileId) (*fileproto.FileInfo, error) {
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

func (t *InMemoryStore) NotExistsBlocks(ctx context.Context, bs []blocks.Block) (notExists []blocks.Block, err error) {
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

func (t *InMemoryStore) Get(ctx context.Context, k cid.Cid) (blocks.Block, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if b, ok := t.store[k]; ok {
		return b, nil
	}
	return nil, &format.ErrNotFound{Cid: k}
}

func (t *InMemoryStore) GetMany(ctx context.Context, ks []cid.Cid) <-chan blocks.Block {
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

func (t *InMemoryStore) ExistsCids(ctx context.Context, ks []cid.Cid) (exists []cid.Cid, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, k := range ks {
		if _, ok := t.store[k]; ok {
			exists = append(exists, k)
		}
	}
	return
}

func (t *InMemoryStore) Add(ctx context.Context, bs []blocks.Block) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, b := range bs {
		t.store[b.Cid()] = b
	}
	return nil
}

func (t *InMemoryStore) AddAsync(ctx context.Context, bs []blocks.Block) (successCh chan cid.Cid) {
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

func (t *InMemoryStore) Delete(ctx context.Context, c cid.Cid) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.store[c]; ok {
		delete(t.store, c)
		return nil
	}
	return &format.ErrNotFound{Cid: c}
}

func (t *InMemoryStore) DeleteMany(ctx context.Context, cids ...cid.Cid) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, c := range cids {
		if _, ok := t.store[c]; ok {
			delete(t.store, c)
		}
	}
	return nil
}

func (t *InMemoryStore) Close() (err error) {
	return nil
}
