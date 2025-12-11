package filesync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	blocks "github.com/ipfs/go-block-format"
)

func newRequestsBatcher(maxBatchSize int, maxBatchWait time.Duration, requestCh chan<- blockPushManyRequest) *requestsBatcher {
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

func (b *requestsBatcher) addFile(spaceId string, fileId string, objectId string, blocks []blocks.Block) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	batch, ok := b.fileBatches[fileId]
	if !ok {
		batch = &fileBatch{
			spaceId:  spaceId,
			fileId:   fileId,
			objectId: objectId,
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
					files:            make(map[string][]blocks.Block),
					fileIdToSpaceId:  make(map[string]string),
					fileIdToObjectId: make(map[string]string),
				}
			}

			for _, block := range batch.blocks {
				ok := lastMixedBatch.addBlock(batch.spaceId, fileId, batch.objectId, block, b.maxBatchSize)
				if !ok {
					b.enqueue(lastMixedBatch)
					lastMixedBatch.reset()
					lastMixedBatch.addBlock(batch.spaceId, fileId, batch.objectId, block, b.maxBatchSize)
				}
			}
			delete(b.fileBatches, fileId)
		}
	}
	if lastMixedBatch != nil && lastMixedBatch.totalSize > 0 {
		b.enqueue(lastMixedBatch)
	}
}

type blockPushManyRequest struct {
	fileIdToObjectId map[string]string
	req              *fileproto.BlockPushManyRequest
}

type genericBatch interface {
	buildRequest() blockPushManyRequest
}

func (b *requestsBatcher) enqueue(batch genericBatch) {
	req := batch.buildRequest()
	if len(req.req.FileBlocks) > 0 {
		b.requests <- req
	}
}

type fileBatch struct {
	fileId    string
	objectId  string
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

func (b *fileBatch) buildRequest() blockPushManyRequest {
	bs := make([]*fileproto.Block, 0, len(b.blocks))
	for _, block := range b.blocks {
		bs = append(bs, &fileproto.Block{
			Cid:  block.Cid().Bytes(),
			Data: block.RawData(),
		})
	}

	return blockPushManyRequest{
		fileIdToObjectId: map[string]string{
			b.fileId: b.objectId,
		},
		req: &fileproto.BlockPushManyRequest{
			FileBlocks: []*fileproto.FileBlocks{
				{
					SpaceId: b.spaceId,
					FileId:  b.fileId,
					Blocks:  bs,
				},
			},
		},
	}
}

func (b *fileBatch) reset() {
	b.totalSize = 0
	b.blocks = nil
}

type mixedBatch struct {
	totalSize        int
	files            map[string][]blocks.Block
	fileIdToObjectId map[string]string
	fileIdToSpaceId  map[string]string
}

func (b *mixedBatch) addBlock(spaceId string, fileId string, objectId string, block blocks.Block, maxBatchSize int) bool {
	blockSize := len(block.RawData())
	if b.totalSize+blockSize > maxBatchSize {
		return false
	}
	b.totalSize += blockSize
	b.files[fileId] = append(b.files[fileId], block)
	b.fileIdToSpaceId[fileId] = spaceId
	b.fileIdToObjectId[fileId] = objectId
	return true
}

func (b *mixedBatch) buildRequest() blockPushManyRequest {
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
	return blockPushManyRequest{
		fileIdToObjectId: b.fileIdToObjectId,
		req: &fileproto.BlockPushManyRequest{
			FileBlocks: fileBlocks,
		},
	}
}

func (b *mixedBatch) reset() {
	b.totalSize = 0
	b.files = map[string][]blocks.Block{}
	b.fileIdToObjectId = map[string]string{}
	b.fileIdToSpaceId = map[string]string{}
}

type requestsBatcher struct {
	maxBatchSize int
	maxBatchWait time.Duration
	requests     chan<- blockPushManyRequest

	lock        sync.Mutex
	fileBatches map[string]*fileBatch
}
