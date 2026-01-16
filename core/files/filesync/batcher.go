package filesync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	blocks "github.com/ipfs/go-block-format"
)

// requestsBatcher batches file upload requests to one or many BlockPushMany requests
// Multiple small files could be batched together. In contrast, large files will be uploaded within multiple requests
type requestsBatcher struct {
	maxBatchSize int
	maxBatchWait time.Duration
	requests     chan<- blockPushManyRequest

	lock        sync.Mutex
	fileBatches map[string]*fileBatch
}

// newRequestsBatcher create a new instance of a batcher
// - maxBatchSize controls the maximum data that can be uploaded at once
// - maxBatchWait controls maximum wait time for a batch to be uploaded. It's required to avoid waiting for batches to be full before sending a request
// - requestsCh is channel where requests are sent
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
			// We don't use mixedBatch here because mixedBatch is designed to combine multiple small files. But in this
			// case we are trying to upload a large file that be scattered to multiple upload requests.
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

	// Use mixedBatch to group multiple file batches together
	var lastMixedBatch *mixedBatch
	for fileId, batch := range b.fileBatches {
		// If it's time to send a batch to the server
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

func (b *requestsBatcher) enqueue(batch genericBatch) {
	req := batch.buildRequest()
	if len(req.req.FileBlocks) > 0 {
		b.requests <- req
	}
}

type blockPushManyRequest struct {
	fileIdToObjectId map[string]string
	req              *fileproto.BlockPushManyRequest
}

type genericBatch interface {
	buildRequest() blockPushManyRequest
}

type fileBatch struct {
	fileId    string
	objectId  string
	spaceId   string
	totalSize int
	blocks    []blocks.Block
	createdAt time.Time
}

// addBlock tries to add a block. It returns true if the block fits within current batch
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

// mixedBatch groups multiple fileBatches together
type mixedBatch struct {
	totalSize        int
	files            map[string][]blocks.Block
	fileIdToObjectId map[string]string
	fileIdToSpaceId  map[string]string
}

// addBlock tries to add block to the mixed batch. It returns true if the block fits within current batch
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
