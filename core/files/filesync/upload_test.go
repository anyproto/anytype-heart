package filesync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-store/query"
	"github.com/globalsign/mgo/bson"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filesync/filequeue"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
)

func TestFileSync_AddFile(t *testing.T) {
	t.Run("within limits", func(t *testing.T) {
		for _, size := range []int{1024, 1024 * 1024, 5 * 1024 * 1024} {
			t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
				fx := newFixture(t, 1024*1024*1024)
				defer fx.Finish(t)

				// Add file to local DAG
				fileId, fileNode := fx.givenFileAddedToDAG(t, 1024)
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
		}
	})

	t.Run("limit has been reached", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		fx := newFixture(t, 1024)
		defer fx.Finish(t)

		fileId, fileNode := fx.givenFileAddedToDAG(t, 1024)
		spaceId := "space1"

		req := AddFileRequest{
			FileObjectId:   "objectId1",
			FileId:         domain.FullFileId{SpaceId: spaceId, FileId: fileId},
			UploadedByUser: true,
			Imported:       false,
		}
		require.NoError(t, fx.AddFile(req))
		fx.waitLimitReachedEvent(t, time.Second)

		_, err := fx.rpcStore.Get(ctx, fileNode.Cid())
		assert.Error(t, err)

		usage, err := fx.NodeUsage(ctx)
		require.NoError(t, err)
		assert.Zero(t, usage.TotalBytesUsage)

		it, err := fx.queue.GetNext(ctx, filequeue.GetNextRequest[FileInfo]{
			Subscribe:   true,
			StoreFilter: filterByState(FileStateLimited),
			Filter:      func(info FileInfo) bool { return info.State == FileStateLimited },
		})

		require.NoError(t, err)
		assert.Equal(t, req.FileId.FileId, it.FileId)
		assert.Equal(t, req.FileObjectId, it.ObjectId)

		err = fx.queue.ReleaseAndUpdate(it.ObjectId, it)
		require.NoError(t, err)
	})

	t.Run("upload multiple files concurrently", func(t *testing.T) {
		fx := newFixture(t, 1024*1024*1024)
		defer fx.Finish(t)

		var wg sync.WaitGroup
		var cidsCount atomic.Int64
		var totalSize atomic.Int64
		spaceId := "space1"

		for i := 0; i < 10; i++ {
			wg.Add(1)

			go func() {
				defer wg.Done()

				// Add file to local DAG
				fileId, fileNode := fx.givenFileAddedToDAG(t, 1024)

				// Add file to upload queue
				fx.givenFileUploaded(t, spaceId, fileId)

				// Check that file uploaded to in-memory node
				wantSize, _ := fileNode.Size()
				wantCids := fx.assertFileUploadedToRemoteNode(t, fileNode, int(wantSize))

				cidsCount.Add(int64(len(wantCids)))
				totalSize.Add(int64(wantSize))
			}()
		}

		wg.Wait()

		// Check node usage
		currentUsage, err := fx.getAndUpdateNodeUsage(ctx)
		require.NoError(t, err)

		assert.Equal(t, totalSize.Load(), int64(currentUsage.TotalBytesUsage))

		assert.Equal(t, []SpaceStat{
			{
				SpaceId:           spaceId,
				FileCount:         10,
				CidsCount:         int(cidsCount.Load()),
				TotalBytesUsage:   currentUsage.TotalBytesUsage,
				SpaceBytesUsage:   currentUsage.TotalBytesUsage, // Equals to total because we got only one space
				AccountBytesLimit: currentUsage.AccountBytesLimit,
			},
		}, currentUsage.Spaces)
	})

	t.Run("upload multiple files concurrently: limits reached", func(t *testing.T) {
		fx := newFixture(t, 1024+512)
		defer fx.Finish(t)

		var wg sync.WaitGroup
		var uploaded atomic.Int64
		var limited atomic.Int64
		spaceId := "space1"

		for i := 0; i < 10; i++ {
			wg.Add(1)

			go func() {
				defer wg.Done()

				// Add file to local DAG
				fileId, fileNode := fx.givenFileAddedToDAG(t, 1024)

				// Add file to upload queue
				state := fx.givenFileUploadedOrLimited(t, spaceId, fileId)
				if state == FileStateDone {
					// Check that file uploaded to in-memory node
					wantSize, _ := fileNode.Size()
					fx.assertFileUploadedToRemoteNode(t, fileNode, int(wantSize))
					uploaded.Inc()
				} else {
					limited.Inc()
				}
			}()
		}

		wg.Wait()

		// Check node usage
		currentUsage, err := fx.getAndUpdateNodeUsage(ctx)
		require.NoError(t, err)

		assert.Equal(t, int64(1), uploaded.Load())
		assert.Equal(t, int64(9), limited.Load())

		assert.Equal(t, []SpaceStat{
			{
				SpaceId:           spaceId,
				FileCount:         1,
				CidsCount:         1,
				TotalBytesUsage:   currentUsage.TotalBytesUsage,
				SpaceBytesUsage:   currentUsage.TotalBytesUsage, // Equals to total because we got only one space
				AccountBytesLimit: currentUsage.AccountBytesLimit,
			},
		}, currentUsage.Spaces)
	})
}

func TestFileSync_AddFile_SkipWhenNotLocal(t *testing.T) {
	fx := newFixture(t, 1024*1024*1024)
	defer fx.Finish(t)

	nonExistentFileId := domain.FileId("bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku")
	req := AddFileRequest{
		FileObjectId:   "objectId1",
		FileId:         domain.FullFileId{SpaceId: "space1", FileId: nonExistentFileId},
		UploadedByUser: true,
	}
	err := fx.AddFile(req)
	require.NoError(t, err)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = fx.queue.GetNext(timeoutCtx, filequeue.GetNextRequest[FileInfo]{
		Subscribe:   true,
		StoreFilter: filterByState(FileStatePendingUpload),
		Filter:      func(info FileInfo) bool { return info.FileId == nonExistentFileId },
	})
	assert.Error(t, err, "file without local blocks should not be queued")
}

func TestFileSync_MarkUploaded(t *testing.T) {
	t.Run("marks pending file as done", func(t *testing.T) {
		fx := newFixture(t, 1024*1024*1024)
		defer fx.Finish(t)

		fileId, _ := fx.givenFileAddedToDAG(t, 1024)
		objectId := "objectId1"
		spaceId := "space1"

		require.NoError(t, fx.AddFile(AddFileRequest{
			FileObjectId: objectId,
			FileId:       domain.FullFileId{SpaceId: spaceId, FileId: fileId},
		}))

		require.NoError(t, fx.MarkUploaded(objectId))

		it, err := fx.queue.GetById(objectId)
		require.NoError(t, err)
		assert.Equal(t, FileStateDone, it.State)

		err = fx.queue.Release(objectId)
		require.NoError(t, err)
	})

	t.Run("marks missing-blocks file as done", func(t *testing.T) {
		fx := newFixture(t, 1024*1024*1024)
		defer fx.Finish(t)

		fileId, fileNode := fx.givenFileAddedToDAG(t, 1024)
		objectId := "objectId1"
		spaceId := "space1"

		require.NoError(t, fx.AddFile(AddFileRequest{
			FileObjectId: objectId,
			FileId:       domain.FullFileId{SpaceId: spaceId, FileId: fileId},
		}))

		// Simulate missing blocks by deleting root and processing
		err := fx.localFileStorage.Delete(ctx, fileNode.Cid())
		require.NoError(t, err)

		it, err := fx.queue.GetById(objectId)
		require.NoError(t, err)
		it.State = FileStateMissingBlocks
		require.NoError(t, fx.queue.ReleaseAndUpdate(objectId, it))

		require.NoError(t, fx.MarkUploaded(objectId))

		it, err = fx.queue.GetById(objectId)
		require.NoError(t, err)
		assert.Equal(t, FileStateDone, it.State)

		err = fx.queue.Release(objectId)
		require.NoError(t, err)
	})

	t.Run("no error when file not in queue", func(t *testing.T) {
		fx := newFixture(t, 1024*1024*1024)
		defer fx.Finish(t)

		require.NoError(t, fx.MarkUploaded("nonExistentObject"))
	})
}

func TestFileSync_UploadTransitionsToMissingBlocks(t *testing.T) {
	fx := newFixture(t, 1024*1024*1024)
	defer fx.Finish(t)

	spaceId := "space1"
	fileId, fileNode := fx.givenFileAddedToDAG(t, 1024)

	// Delete root block from local storage so walkDAG fails with errBlockNotFound
	err := fx.localFileStorage.Delete(ctx, fileNode.Cid())
	require.NoError(t, err)

	it := FileInfo{
		FileId:       fileId,
		SpaceId:      spaceId,
		ObjectId:     "objectId1",
		State:        FileStatePendingUpload,
		ScheduledAt:  time.Now(),
		CidsToUpload: map[cid.Cid]struct{}{},
		CidsToBind:   map[cid.Cid]struct{}{},
	}

	result, err := fx.processFilePendingUpload(ctx, it)
	require.NoError(t, err)
	assert.Equal(t, FileStateMissingBlocks, result.State)
	assert.Equal(t, fileId, result.FileId)
}

func TestFileSync_TransientAllocateErrorReschedules(t *testing.T) {
	t.Run("transient SpaceInfo error must not flip file to Limited", func(t *testing.T) {
		transientErr := errors.New("lock already taken, locked nodes: [0]")

		fx := newFixtureNotStarted(t, 1024*1024*1024)
		// Inject the transient error before start so spaceUsage's initial
		// Update fails and its cache stays empty — subsequent allocateFile
		// calls will retry SpaceInfo and hit the same error.
		fx.rpcStore.SetSpaceInfoError(transientErr)

		// Capture filesyncstatus updates: the user-visible regression in GO-7275
		// was a stray filesyncstatus.Limited emitted via OnStatusUpdated.
		var statusMu sync.Mutex
		statuses := map[string][]filesyncstatus.Status{}
		fx.OnStatusUpdated(func(objectId string, _ domain.FullFileId, status filesyncstatus.Status) error {
			statusMu.Lock()
			defer statusMu.Unlock()
			statuses[objectId] = append(statuses[objectId], status)
			return nil
		})

		require.NoError(t, fx.a.Start(ctx))
		defer fx.Finish(t)

		// Wait until the space view subscription has registered spaceUsage
		// for "space1" so getSpace returns a valid usage tracker.
		spaceId := "space1"
		fx.waitCondition(t, 2*time.Second, func() bool {
			_, err := fx.limitManager.getSpace(spaceId)
			return err == nil
		})

		// Pre-populate CidsToUpload so checkBlocksAvailability short-circuits
		// without walking the local DAG.
		dummyCid, err := cid.Parse("bafybeihqbmekus5fwgtlybi7qdjmwo7d2o2aksjth4fqabzcduswc7o6re")
		require.NoError(t, err)

		objectId := "objectId1"
		it := FileInfo{
			FileId:              domain.FileId("bafybeihqbmekus5fwgtlybi7qdjmwo7d2o2aksjth4fqabzcduswc7o6re"),
			SpaceId:             spaceId,
			ObjectId:            objectId,
			State:               FileStatePendingUpload,
			ScheduledAt:         time.Now(),
			AddedByUser:         true,
			BytesToUploadOrBind: 1024,
			CidsToUpload:        map[cid.Cid]struct{}{dummyCid: {}},
			CidsToBind:          map[cid.Cid]struct{}{},
		}

		result, processErr := fx.processFilePendingUpload(ctx, it)

		require.Error(t, processErr, "transient allocateFile error must propagate to caller")
		assert.ErrorIs(t, processErr, transientErr, "error chain should preserve underlying transient error")
		assert.NotEqual(t, FileStateLimited, result.State, "transient error must not flip file to Limited")
		assert.Equal(t, FileStatePendingUpload, result.State, "file should stay PendingUpload for retry")

		fx.eventsLock.Lock()
		for _, e := range fx.events {
			for _, msg := range e.Messages {
				assert.Nil(t, msg.GetFileLimitReached(), "FileLimitReached event must not be broadcast for transient errors")
			}
		}
		fx.eventsLock.Unlock()

		statusMu.Lock()
		defer statusMu.Unlock()
		for _, s := range statuses[objectId] {
			assert.NotEqual(t, filesyncstatus.Limited, s, "filesyncstatus.Limited must not be emitted for transient errors")
		}
	})
}

func TestFileSync_NoSyncingStatusWhenLimitReached(t *testing.T) {
	fx := newFixtureNotStarted(t, 1024)
	spaceId := "space1"

	var mu sync.Mutex
	statuses := map[string][]filesyncstatus.Status{}
	fx.OnStatusUpdated(func(objectId string, _ domain.FullFileId, status filesyncstatus.Status) error {
		mu.Lock()
		statuses[objectId] = append(statuses[objectId], status)
		mu.Unlock()
		return nil
	})

	require.NoError(t, fx.a.Start(ctx))
	defer fx.Finish(t)

	fileId, _ := fx.givenFileAddedToDAG(t, 1024)
	objectId := "limitedObject"
	require.NoError(t, fx.AddFile(AddFileRequest{
		FileObjectId:   objectId,
		FileId:         domain.FullFileId{SpaceId: spaceId, FileId: fileId},
		UploadedByUser: true,
	}))

	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	it, err := fx.queue.GetNext(waitCtx, filequeue.GetNextRequest[FileInfo]{
		Subscribe:   true,
		StoreFilter: filterByState(FileStateLimited),
		Filter:      func(info FileInfo) bool { return info.ObjectId == objectId && info.State == FileStateLimited },
	})
	require.NoError(t, err)
	require.NoError(t, fx.queue.ReleaseAndUpdate(it.ObjectId, it))

	mu.Lock()
	defer mu.Unlock()
	for _, s := range statuses[objectId] {
		assert.NotEqual(t, filesyncstatus.Syncing, s, "limited file should never get Syncing status")
	}
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

func (fx *fixture) givenFileAddedToDAG(t *testing.T, size int) (domain.FileId, ipld.Node) {
	return fx.givenFileWithSizeAddedToDAG(t, size)
}

func (fx *fixture) givenFileUploaded(t *testing.T, spaceId string, fileId domain.FileId) {
	// Add file to upload queue
	req := AddFileRequest{
		FileObjectId:   bson.NewObjectId().Hex(),
		FileId:         domain.FullFileId{SpaceId: spaceId, FileId: fileId},
		UploadedByUser: true,
		Imported:       false,
	}
	err := fx.AddFile(req)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	it, err := fx.queue.GetNext(ctx, filequeue.GetNextRequest[FileInfo]{
		Subscribe: true,
		StoreFilter: query.And{
			filterByFileId(fileId.String()),
			filterByState(FileStateDone),
		},
		Filter: func(info FileInfo) bool { return info.FileId == fileId && info.State == FileStateDone },
	})

	require.NoError(t, err)
	assert.Equal(t, fileId, it.FileId)

	err = fx.queue.ReleaseAndUpdate(it.ObjectId, it)
	require.NoError(t, err)

	// Check remote node
	fileInfos, err := fx.rpcStore.FilesInfo(ctx, spaceId, fileId)
	require.NoError(t, err)
	require.Len(t, fileInfos, 1)
	assert.NotZero(t, fileInfos[0].UsageBytes)
}

func (fx *fixture) givenFileUploadedOrLimited(t *testing.T, spaceId string, fileId domain.FileId) FileState {
	// Add file to upload queue
	req := AddFileRequest{
		FileObjectId:   bson.NewObjectId().Hex(),
		FileId:         domain.FullFileId{SpaceId: spaceId, FileId: fileId},
		UploadedByUser: true,
		Imported:       false,
	}
	err := fx.AddFile(req)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	it, err := fx.queue.GetNext(ctx, filequeue.GetNextRequest[FileInfo]{
		Subscribe: true,
		StoreFilter: query.And{
			filterByFileId(fileId.String()),
			query.Or{
				filterByState(FileStateDone),
				filterByState(FileStateLimited),
			},
		},
		Filter: func(info FileInfo) bool {
			return info.FileId == fileId && (info.State == FileStateLimited || info.State == FileStateDone)
		},
	})

	require.NoError(t, err)
	assert.Equal(t, fileId, it.FileId)

	state := it.State

	err = fx.queue.ReleaseAndUpdate(it.ObjectId, it)
	require.NoError(t, err)

	// Check remote node
	fileInfos, err := fx.rpcStore.FilesInfo(ctx, spaceId, fileId)
	require.NoError(t, err)
	if state == FileStateDone {
		require.Len(t, fileInfos, 1)
		assert.NotZero(t, fileInfos[0].UsageBytes)
	} else {
		require.Len(t, fileInfos, 0)
	}
	return state
}

func (fx *fixture) givenFileWithSizeAddedToDAG(t *testing.T, size int) (domain.FileId, ipld.Node) {
	buf := make([]byte, size)
	_, err := rand.Read(buf)
	require.NoError(t, err)
	fileNode, err := fx.fileService.AddFile(ctx, bytes.NewReader(buf))
	require.NoError(t, err)
	return domain.FileId(fileNode.Cid().String()), fileNode
}
