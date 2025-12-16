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
