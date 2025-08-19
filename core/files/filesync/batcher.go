package filesync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	blocks "github.com/ipfs/go-block-format"
)

type fileBatch struct {
	fileId    string
	spaceId   string
	totalSize int
	blocks    []blocks.Block
	createdAt time.Time
}

func (b *fileBatch) addBlock(block blocks.Block, maxBatchSize int) bool {
	blockSize := len(block.RawData())
	if b.totalSize+blockSize > maxBatchSize {
		return false
	}
	if b.createdAt.IsZero() {
		b.createdAt = time.Now()
	}
	b.totalSize += blockSize
	b.blocks = append(b.blocks, block)
	return true
}

func (b *fileBatch) buildRequest() *fileproto.BlockPushManyRequest {
	bs := make([]*fileproto.Block, 0, len(b.blocks))
	for _, block := range b.blocks {
		bs = append(bs, &fileproto.Block{
			Cid:  block.Cid().Bytes(),
			Data: block.RawData(),
		})
	}
	return &fileproto.BlockPushManyRequest{
		FileBlocks: []*fileproto.FileBlocks{
			{
				SpaceId: b.spaceId,
				FileId:  b.fileId,
				Blocks:  bs,
			},
		},
	}
}

func (b *fileBatch) reset() {
	b.totalSize = 0
	b.blocks = nil
}

type mixedBatch struct {
	totalSize       int
	files           map[string][]blocks.Block
	fileIdToSpaceId map[string]string
}

func (b *mixedBatch) addBlock(spaceId string, fileId string, block blocks.Block, maxBatchSize int) bool {
	blockSize := len(block.RawData())
	if b.totalSize+blockSize > maxBatchSize {
		return false
	}
	b.totalSize += blockSize
	b.files[fileId] = append(b.files[fileId], block)
	b.fileIdToSpaceId[fileId] = spaceId
	return true
}

func (b *mixedBatch) buildRequest() *fileproto.BlockPushManyRequest {
	fileBlocks := make([]*fileproto.FileBlocks, 0, len(b.files))
	for fileId, bs := range b.files {
		reqBlocks := make([]*fileproto.Block, 0, len(bs))
		for _, block := range bs {
			reqBlocks = append(reqBlocks, &fileproto.Block{
				Cid:  block.Cid().Bytes(),
				Data: block.RawData(),
			})
		}
		fileBlocks = append(fileBlocks, &fileproto.FileBlocks{
			SpaceId: b.fileIdToSpaceId[fileId],
			FileId:  fileId,
			Blocks:  reqBlocks,
		})
	}
	return &fileproto.BlockPushManyRequest{
		FileBlocks: fileBlocks,
	}
}

func (b *mixedBatch) reset() {
	b.totalSize = 0
	for fileId := range b.files {
		delete(b.files, fileId)
	}
	for fileId := range b.fileIdToSpaceId {
		delete(b.fileIdToSpaceId, fileId)
	}
}

type requestsBatcher struct {
	maxBatchSize int
	maxBatchWait time.Duration
	requests     chan<- *fileproto.BlockPushManyRequest

	lock        sync.Mutex
	fileBatches map[string]*fileBatch
}

func newRequestsBatcher(maxBatchSize int, maxBatchWait time.Duration, requestCh chan<- *fileproto.BlockPushManyRequest) *requestsBatcher {
	return &requestsBatcher{
		maxBatchSize: maxBatchSize,
		maxBatchWait: maxBatchWait,
		fileBatches:  make(map[string]*fileBatch),
		requests:     requestCh,
	}
}

func (b *requestsBatcher) run(ctx context.Context) {
	ticker := time.NewTicker(b.maxBatchWait)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.tick()
		}
	}
}

func (b *requestsBatcher) addFile(spaceId string, fileId string, blocks []blocks.Block) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	batch, ok := b.fileBatches[fileId]
	if !ok {
		batch = &fileBatch{
			spaceId: spaceId,
			fileId:  fileId,
		}
	}

	for _, block := range blocks {
		ok = batch.addBlock(block, b.maxBatchSize)
		if !ok {
			b.enqueue(batch)
			batch.reset()
			ok = batch.addBlock(block, b.maxBatchSize)
			if !ok {
				return fmt.Errorf("block size is too big")
			}
		}
	}
	if batch.totalSize+b.maxBatchSize/10 > b.maxBatchSize {
		b.enqueue(batch)
		batch.reset()
	}

	if batch.totalSize > 0 {
		b.fileBatches[fileId] = batch
	}
	return nil
}

func (b *requestsBatcher) tick() {
	b.lock.Lock()
	defer b.lock.Unlock()

	// TODO Think about optimized batching using max-heap for files or any other approach
	var lastMixedBatch *mixedBatch
	for fileId, batch := range b.fileBatches {
		if time.Since(batch.createdAt) > b.maxBatchWait {
			if lastMixedBatch == nil {
				lastMixedBatch = &mixedBatch{
					files:           make(map[string][]blocks.Block),
					fileIdToSpaceId: make(map[string]string),
				}
			}

			for _, block := range batch.blocks {
				ok := lastMixedBatch.addBlock(batch.spaceId, fileId, block, b.maxBatchSize)
				if !ok {
					b.enqueue(lastMixedBatch)
					lastMixedBatch.reset()
					lastMixedBatch.addBlock(batch.spaceId, fileId, block, b.maxBatchSize)
				}
			}
			delete(b.fileBatches, fileId)
		}
	}
	if lastMixedBatch != nil && lastMixedBatch.totalSize > 0 {
		b.enqueue(lastMixedBatch)
	}
}

type genericBatch interface {
	buildRequest() *fileproto.BlockPushManyRequest
}

func (b *requestsBatcher) enqueue(batch genericBatch) {
	req := batch.buildRequest()
	if len(req.FileBlocks) > 0 {
		b.requests <- req
	}
}
