package filesync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filesync/filequeue"
)

func TestFileSync_AddFile(t *testing.T) {
	t.Run("within limits", func(t *testing.T) {
		fx := newFixture(t, 1024*1024*1024)
		defer fx.Finish(t)

		// Add file to local DAG
		fileId, fileNode := fx.givenFileAddedToDAG(t)
		spaceId := "space1"

		// Save node usage
		prevUsage, err := fx.getAndUpdateNodeUsage(ctx)
		require.NoError(t, err)
		assert.Empty(t, prevUsage.Spaces)
		assert.Zero(t, prevUsage.TotalBytesUsage)
		assert.Zero(t, prevUsage.TotalCidsCount)

		// Add file to upload queue
		fx.givenFileUploaded(t, spaceId, fileId)

		// Check that file uploaded to in-memory node
		wantSize, _ := fileNode.Size()
		wantCids := fx.assertFileUploadedToRemoteNode(t, fileNode, int(wantSize))

		// Check node usage
		currentUsage, err := fx.getAndUpdateNodeUsage(ctx)
		require.NoError(t, err)
		assert.Equal(t, int(wantSize), currentUsage.TotalBytesUsage)
		assert.Equal(t, len(wantCids), currentUsage.TotalCidsCount)
		assert.Equal(t, []SpaceStat{
			{
				SpaceId:           spaceId,
				FileCount:         1,
				CidsCount:         len(wantCids),
				TotalBytesUsage:   currentUsage.TotalBytesUsage,
				SpaceBytesUsage:   currentUsage.TotalBytesUsage, // Equals to total because we got only one space
				AccountBytesLimit: currentUsage.AccountBytesLimit,
			},
		}, currentUsage.Spaces)
	})

	// t.Run("limit has been reached", func(t *testing.T) {
	// 	fx := newFixture(t, 1024)
	// 	defer fx.Finish(t)
	//
	// 	fileId, fileNode := fx.givenFileAddedToDAG(t)
	// 	spaceId := "space1"
	//
	// 	req := AddFileRequest{
	// 		FileObjectId:   "objectId1",
	// 		FileId:         domain.FullFileId{SpaceId: spaceId, FileId: fileId},
	// 		UploadedByUser: true,
	// 		Imported:       false,
	// 	}
	// 	require.NoError(t, fx.AddFile(req))
	// 	fx.waitLimitReachedEvent(t, time.Second*5)
	// 	fx.waitEmptyQueue(t, fx.uploadingQueue, time.Second*5)
	//
	// 	_, err := fx.rpcStore.Get(ctx, fileNode.Cid())
	// 	assert.Error(t, err)
	//
	// 	usage, err := fx.NodeUsage(ctx)
	// 	require.NoError(t, err)
	// 	assert.Zero(t, usage.TotalBytesUsage)
	//
	// 	fx.waitCondition(t, 100*time.Millisecond, func() bool {
	// 		return fx.retryUploadingQueue.Len() == 1 || fx.retryUploadingQueue.NumProcessedItems() > 0
	// 	})
	// })
	//
	// t.Run("file object has been deleted - stop uploading", func(t *testing.T) {
	// 	fx := newFixture(t, 1024*1024*1024)
	// 	defer fx.Finish(t)
	//
	// 	fileId, _ := fx.givenFileAddedToDAG(t)
	// 	spaceId := "space1"
	//
	// 	fx.onUploadStarted = func(objectId string, fileId domain.FullFileId) error {
	// 		return spacestorage.ErrTreeStorageAlreadyDeleted
	// 	}
	//
	// 	req := AddFileRequest{
	// 		FileObjectId:        "objectId1",
	// 		FileId:              domain.FullFileId{SpaceId: spaceId, FileId: fileId},
	// 		UploadedByUser:      true,
	// 		Imported:            false,
	// 		PrioritizeVariantId: "",
	// 		Score:               0,
	// 	}
	// 	require.NoError(t, fx.AddFile(req))
	//
	// 	fx.waitEmptyQueue(t, fx.uploadingQueue, 100*time.Millisecond)
	// 	assert.Equal(t, 1, fx.uploadingQueue.NumProcessedItems())
	// 	fx.waitEmptyQueue(t, fx.retryUploadingQueue, 100*time.Millisecond)
	// 	assert.Equal(t, 0, fx.retryUploadingQueue.NumProcessedItems())
	// })
}

func (fx *fixture) assertFileUploadedToRemoteNode(t *testing.T, fileNode ipld.Node, wantSize int) []cid.Cid {
	var gotSize int
	var wantCids []cid.Cid
	walker := ipld.NewWalker(ctx, ipld.NewNavigableIPLDNode(fileNode, fx.fileService.DAGService()))
	err := walker.Iterate(func(node ipld.NavigableNode) error {
		cId := node.GetIPLDNode().Cid()
		gotBlock, err := fx.rpcStore.Get(ctx, cId)
		if err != nil {
			return fmt.Errorf("node: %w", err)
		}
		wantCids = append(wantCids, cId)
		gotSize += len(gotBlock.RawData())
		wantBlock, err := fx.localFileStorage.Get(ctx, cId)
		if err != nil {
			return fmt.Errorf("local: %w", err)
		}
		require.Equal(t, wantBlock.RawData(), gotBlock.RawData())
		return nil
	})
	if !errors.Is(err, ipld.EndOfDag) {
		require.NoError(t, err)
	}
	assert.Equal(t, int(wantSize), gotSize)
	return wantCids
}

func (fx *fixture) givenFileAddedToDAG(t *testing.T) (domain.FileId, ipld.Node) {
	return fx.givenFileWithSizeAddedToDAG(t, 1024*1024)
}

func (fx *fixture) givenFileUploaded(t *testing.T, spaceId string, fileId domain.FileId) {
	// Add file to upload queue
	req := AddFileRequest{
		FileObjectId:   "objectId1",
		FileId:         domain.FullFileId{SpaceId: spaceId, FileId: fileId},
		UploadedByUser: true,
		Imported:       false,
	}
	err := fx.AddFile(req)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	it, err := fx.queue.GetNext(ctx, filequeue.GetNextRequest[FileInfo]{
		Subscribe:   true,
		StoreFilter: filterByState(FileStateDone),
		Filter:      func(info FileInfo) bool { return info.State == FileStateDone },
	})

	require.NoError(t, err)

	err = fx.queue.ReleaseAndUpdate(it)
	require.NoError(t, err)

	// Check remote node
	fileInfos, err := fx.rpcStore.FilesInfo(ctx, spaceId, fileId)
	require.NoError(t, err)
	require.Len(t, fileInfos, 1)
	assert.NotZero(t, fileInfos[0].UsageBytes)
}

func (fx *fixture) givenFileWithSizeAddedToDAG(t *testing.T, size int) (domain.FileId, ipld.Node) {
	buf := make([]byte, size)
	_, err := rand.Read(buf)
	require.NoError(t, err)
	fileNode, err := fx.fileService.AddFile(ctx, bytes.NewReader(buf))
	require.NoError(t, err)
	return domain.FileId(fileNode.Cid().String()), fileNode
}

// func TestUpload(t *testing.T) {
// 	t.Run("with file absent on node, upload it", func(t *testing.T) {
// 		fx := newFixture(t, 1024*1024*1024)
// 		defer fx.Finish(t)
//
// 		spaceId := "space1"
// 		fileId, _ := fx.givenFileAddedToDAG(t)
//
// 		err := fx.uploadFileHandleLimits(ctx, &QueueItem{SpaceId: spaceId, FileId: fileId})
// 		require.NoError(t, err)
//
// 		assert.True(t, fx.rpcStore.Stats().BlocksAdded() > 0)
// 		assert.True(t, fx.rpcStore.Stats().CidsBinded() == 0)
// 	})
//
// 	t.Run("with already uploaded file, bind cids", func(t *testing.T) {
// 		fx := newFixture(t, 1024*1024*1024)
// 		defer fx.Finish(t)
//
// 		spaceId := "space1"
// 		fileId, _ := fx.givenFileAddedToDAG(t)
//
// 		err := fx.uploadFileHandleLimits(ctx, &QueueItem{SpaceId: spaceId, FileId: fileId})
// 		require.NoError(t, err)
//
// 		assert.True(t, fx.rpcStore.Stats().BlocksAdded() > 0)
// 		assert.True(t, fx.rpcStore.Stats().CidsBinded() == 0)
//
// 		err = fx.uploadFileHandleLimits(ctx, &QueueItem{SpaceId: spaceId, FileId: fileId})
// 		require.NoError(t, err)
//
// 		assert.True(t, fx.rpcStore.Stats().CidsBinded() == fx.rpcStore.Stats().BlocksAdded())
// 	})
//
// 	t.Run("with too large file, upload limit is reached", func(t *testing.T) {
// 		fx := newFixture(t, 1024)
// 		defer fx.Finish(t)
//
// 		spaceId := "space1"
// 		fileId, _ := fx.givenFileAddedToDAG(t)
//
// 		err := fx.uploadFileHandleLimits(ctx, &QueueItem{SpaceId: spaceId, FileId: fileId})
// 		var errLimit *errLimitReached
// 		require.ErrorAs(t, err, &errLimit)
// 	})
//
// 	t.Run("with multiple files are uploading simultaneously, only one will be uploaded, other unbinded", func(t *testing.T) {
// 		// Limit is exact size of test file. It doesn't equal to raw data because of DAG overhead
// 		fileSize := 10 * 1024 * 1024
// 		limit := fileSize*2 - 1 // Place only for one file
// 		fx := newFixture(t, limit)
// 		defer fx.Finish(t)
//
// 		spaceId := "space1"
//
// 		var (
// 			errorsLock sync.Mutex
// 			errors     []error
// 			wg         sync.WaitGroup
// 			fileIds    []domain.FileId
// 		)
//
// 		numberOfFiles := 3
// 		for i := 0; i < numberOfFiles; i++ {
// 			fileId, _ := fx.givenFileWithSizeAddedToDAG(t, fileSize)
// 			fileIds = append(fileIds, fileId)
// 		}
//
// 		for _, fileId := range fileIds {
// 			wg.Add(1)
// 			go func(fileId domain.FileId) {
// 				defer wg.Done()
//
// 				err := fx.uploadFileHandleLimits(ctx, &QueueItem{SpaceId: spaceId, FileId: fileId})
// 				if err != nil {
// 					errorsLock.Lock()
// 					errors = append(errors, err)
// 					errorsLock.Unlock()
// 				}
//
// 			}(fileId)
// 		}
// 		wg.Wait()
//
// 		for _, err := range errors {
// 			var errLimit *errLimitReached
// 			require.ErrorAs(t, err, &errLimit)
// 		}
//
// 		assert.True(t, fx.rpcStore.Stats().BlocksAdded() > 0)
// 		assert.True(t, fx.rpcStore.Stats().CidsBinded() == 0)
//
// 		filesInfo, err := fx.rpcStore.FilesInfo(ctx, spaceId, fileIds...)
// 		require.NoError(t, err)
//
// 		// Check invariants:
// 		// Number of deleted files <= number of limit reached errors
// 		assert.LessOrEqual(t, int(fx.rpcStore.Stats().FilesDeleted()), len(errors))
// 		// Number of uploaded files == number of tried files - number of errors
// 		assert.Equal(t, len(filesInfo), numberOfFiles-len(errors))
// 	})
// }

//
// func TestBlocksAvailabilityResponseMarshalUnmarshal(t *testing.T) {
// 	resp := &blocksAvailabilityResponse{
// 		bytesToUpload: 123,
// 		bytesToBind:   234,
// 		cidsToBind: map[cid.Cid]struct{}{
// 			cid.MustParse("bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku"): {},
// 		},
// 	}
//
// 	data, err := json.Marshal(resp)
// 	require.NoError(t, err)
//
// 	unmarshaledResp := &blocksAvailabilityResponse{}
// 	err = json.Unmarshal(data, &unmarshaledResp)
// 	require.NoError(t, err)
//
// 	assert.Equal(t, resp, unmarshaledResp)
// }
