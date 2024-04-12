package filesync

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
)

func TestFileSync_AddFile(t *testing.T) {
	t.Run("within limits", func(t *testing.T) {
		fx := newFixture(t, 1024*1024*1024)
		defer fx.Finish(t)

		// Add file to local DAG
		fileId, fileNode := fx.givenFileAddedToDAG(t)
		spaceId := "space1"

		// Save node usage
		prevUsage, err := fx.NodeUsage(ctx)
		require.NoError(t, err)
		assert.Empty(t, prevUsage.Spaces)
		assert.Zero(t, prevUsage.TotalBytesUsage)
		assert.Zero(t, prevUsage.TotalCidsCount)

		// Add file to upload queue
		fx.givenFileUploaded(t, spaceId, fileId)

		// Check that file uploaded to in-memory node
		wantSize, _ := fileNode.Size()
		wantCids := fx.assertFileUploadedToRemoteNode(t, fileNode, int(wantSize))

		// Check that updated space usage event has been sent
		fx.waitEvent(t, 1*time.Second, func(msg *pb.EventMessage) bool {
			if usage := msg.GetFileSpaceUsage(); usage != nil {
				if usage.SpaceId == spaceId && usage.BytesUsage == wantSize {
					return true
				}
			}
			return false
		})

		// Check node usage
		currentUsage, err := fx.NodeUsage(ctx)
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

	t.Run("limit has been reached", func(t *testing.T) {
		fx := newFixture(t, 1024)
		defer fx.Finish(t)

		fileId, fileNode := fx.givenFileAddedToDAG(t)
		spaceId := "space1"

		require.NoError(t, fx.AddFile("objectId1", domain.FullFileId{SpaceId: spaceId, FileId: fileId}, true, false))
		fx.waitLimitReachedEvent(t, time.Second*5)
		fx.waitEmptyQueue(t, fx.uploadingQueue, time.Second*5)

		_, err := fx.rpcStore.Get(ctx, fileNode.Cid())
		assert.Error(t, err)

		usage, err := fx.NodeUsage(ctx)
		require.NoError(t, err)
		assert.Zero(t, usage.TotalBytesUsage)

		fx.waitCondition(t, 100*time.Millisecond, func() bool {
			return fx.retryUploadingQueue.Len() == 1 || fx.retryUploadingQueue.NumProcessedItems() > 0
		})
	})

	t.Run("file object has been deleted - stop uploading", func(t *testing.T) {
		fx := newFixture(t, 1024)
		defer fx.Finish(t)

		fileId, _ := fx.givenFileAddedToDAG(t)
		spaceId := "space1"

		fx.onUploadStarted = func(fileObjectId string) error {
			return spacestorage.ErrTreeStorageAlreadyDeleted
		}

		require.NoError(t, fx.AddFile("objectId1", domain.FullFileId{SpaceId: spaceId, FileId: fileId}, true, false))

		fx.waitEmptyQueue(t, fx.uploadingQueue, 100*time.Millisecond)
		assert.Equal(t, 1, fx.uploadingQueue.NumProcessedItems())
		fx.waitEmptyQueue(t, fx.retryUploadingQueue, 100*time.Millisecond)
		assert.Equal(t, 0, fx.retryUploadingQueue.NumProcessedItems())
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

func (fx *fixture) givenFileAddedToDAG(t *testing.T) (domain.FileId, ipld.Node) {
	buf := make([]byte, 1024*1024)
	_, err := rand.Read(buf)
	require.NoError(t, err)
	fileNode, err := fx.fileService.AddFile(ctx, bytes.NewReader(buf))
	require.NoError(t, err)
	return domain.FileId(fileNode.Cid().String()), fileNode
}

func (fx *fixture) givenFileUploaded(t *testing.T, spaceId string, fileId domain.FileId) {
	// Add file to upload queue
	err := fx.AddFile("objectId1", domain.FullFileId{SpaceId: spaceId, FileId: fileId}, true, false)
	require.NoError(t, err)

	fx.waitEmptyQueue(t, fx.uploadingQueue, time.Second*1)

	// Check remote node
	fileInfos, err := fx.rpcStore.FilesInfo(ctx, spaceId, fileId)
	require.NoError(t, err)
	require.Len(t, fileInfos, 1)
	assert.NotZero(t, fileInfos[0].UsageBytes)
}
