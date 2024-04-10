package filesync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/queue"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
)

var ctx = context.Background()

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

		buf := make([]byte, 1024*1024)
		_, err := rand.Read(buf)
		require.NoError(t, err)
		fileNode, err := fx.fileService.AddFile(ctx, bytes.NewReader(buf))
		require.NoError(t, err)
		fileId := domain.FileId(fileNode.Cid().String())
		spaceId := "space1"

		require.NoError(t, fx.AddFile("objectId1", domain.FullFileId{SpaceId: spaceId, FileId: fileId}, true, false))
		fx.waitLimitReachedEvent(t, time.Second*5)
		fx.waitEmptyQueue(t, fx.uploadingQueue, time.Second*5)

		_, err = fx.rpcStore.Get(ctx, fileNode.Cid())
		assert.Error(t, err)

		usage, err := fx.NodeUsage(ctx)
		require.NoError(t, err)
		assert.Zero(t, usage.TotalBytesUsage)
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

func TestFileSync_RemoveFile(t *testing.T) {
	fx := newFixture(t, 1024*1024*1024)
	defer fx.Finish(t)
	spaceId := "spaceId"

	fileId, _ := fx.givenFileAddedToDAG(t)
	fx.givenFileUploaded(t, spaceId, fileId)

	require.NoError(t, fx.RemoveFile(domain.FullFileId{SpaceId: spaceId, FileId: fileId}))

	fx.waitEmptyQueue(t, fx.removingQueue, time.Second*1)

	_, err := fx.rpcStore.FilesInfo(ctx, spaceId, fileId)
	require.Error(t, err)
}

func newFixture(t *testing.T, limit int) *fixture {
	fx := &fixture{
		fileSync:    New().(*fileSync),
		fileService: fileservice.New(),
		ctrl:        gomock.NewController(t),
		a:           new(app.App),
	}

	fileStore := filestore.New()

	sender := mock_event.NewMockSender(t)
	sender.EXPECT().Name().Return("event")
	sender.EXPECT().Init(mock.Anything).Return(nil)
	sender.EXPECT().Broadcast(mock.Anything).Run(func(e *pb.Event) {
		fx.eventsLock.Lock()
		defer fx.eventsLock.Unlock()
		fx.events = append(fx.events, e)
	}).Maybe()

	fx.rpcStore = rpcstore.NewInMemoryStore(limit)
	localFileStorage := filestorage.NewInMemory()
	fx.localFileStorage = localFileStorage

	dataStoreProvider, err := datastore.NewInMemory()
	require.NoError(t, err)

	fx.a.Register(fx.fileService).
		Register(localFileStorage).
		Register(dataStoreProvider).
		Register(rpcstore.NewInMemoryService(fx.rpcStore)).
		Register(fx.fileSync).
		Register(fileStore).
		Register(sender)
	require.NoError(t, fx.a.Start(ctx))
	return fx
}

type fixture struct {
	*fileSync
	fileService      fileservice.FileService
	localFileStorage fileblockstore.BlockStoreLocal
	ctrl             *gomock.Controller
	a                *app.App
	tmpDir           string
	rpcStore         rpcstore.RpcStore
	eventsLock       sync.Mutex
	events           []*pb.Event
}

func (f *fixture) waitLimitReachedEvent(t *testing.T, timeout time.Duration) {
	f.waitEvent(t, timeout, func(msg *pb.EventMessage) bool {
		return msg.GetFileLimitReached() != nil
	})
}

func (f *fixture) waitEvent(t *testing.T, timeout time.Duration, pred func(msg *pb.EventMessage) bool) {
	f.waitCondition(t, timeout, func() bool {
		f.eventsLock.Lock()
		defer f.eventsLock.Unlock()

		for _, e := range f.events {
			for _, msg := range e.Messages {
				if pred(msg) {
					return true
				}
			}
		}
		return false
	})
}

func (f *fixture) waitEmptyQueue(t *testing.T, queue *queue.Queue[*QueueItem], timeout time.Duration) {
	f.waitCondition(t, timeout, func() bool {
		return queue.Len() == 0
	})
}

func (f *fixture) waitCondition(t *testing.T, timeout time.Duration, pred func() bool) {
	retryTime := time.Millisecond * 10
	for i := 0; i < int(timeout/retryTime); i++ {
		time.Sleep(retryTime)
		if pred() {
			return
		}
	}
	require.False(t, true, "condition is not met: timeout")
}

func (f *fixture) Finish(t *testing.T) {
	defer os.RemoveAll(f.tmpDir)
	require.NoError(t, f.a.Close(ctx))
}
