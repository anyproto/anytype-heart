//go:generate mockgen -package filesync -destination filestore_mock.go github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore FileStore

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
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
)

var ctx = context.Background()

func TestFileSync_AddFile(t *testing.T) {
	t.Run("within limits", func(t *testing.T) {
		fx := newFixture(t, 1024*1024*1024)
		defer fx.Finish(t)

		// Add file to local DAG
		buf := make([]byte, 1024*1024)
		_, err := rand.Read(buf)
		require.NoError(t, err)
		fileNode, err := fx.fileService.AddFile(ctx, bytes.NewReader(buf))
		require.NoError(t, err)
		fileId := domain.FileId(fileNode.Cid().String())
		spaceId := "space1"

		// Save node usage
		prevUsage, err := fx.NodeUsage(ctx)
		require.NoError(t, err)
		assert.Empty(t, prevUsage.Spaces)
		assert.Zero(t, prevUsage.TotalBytesUsage)
		assert.Zero(t, prevUsage.TotalCidsCount)

		fx.fileStoreMock.EXPECT().GetFileSize(fileId).Return(0, fmt.Errorf("not found"))
		fx.fileStoreMock.EXPECT().SetFileSize(fileId, gomock.Any()).Return(nil)
		fx.fileStoreMock.EXPECT().ListFileVariants(fileId).Return([]*storage.FileInfo{
			{}, // We can use just empty struct here, because we don't use any fields
		}, nil).AnyTimes()

		// Add file to upload queue
		err = fx.AddFile(spaceId, fileId, true, false)
		require.NoError(t, err)
		fx.waitEmptyQueue(t, time.Second*5)

		// Check that file uploaded to in-memory node
		wantSize, _ := fileNode.Size()
		var gotSize int
		var wantCids []string
		walker := ipld.NewWalker(ctx, ipld.NewNavigableIPLDNode(fileNode, fx.fileService.DAGService()))
		err = walker.Iterate(func(node ipld.NavigableNode) error {
			cId := node.GetIPLDNode().Cid()
			gotBlock, err := fx.rpcStore.Get(ctx, cId)
			if err != nil {
				return fmt.Errorf("node: %w", err)
			}
			wantCids = append(wantCids, cId.String())
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

		fx.fileStoreMock.EXPECT().GetFileSize(fileId).Return(0, fmt.Errorf("not found"))
		fx.fileStoreMock.EXPECT().SetFileSize(fileId, gomock.Any()).Return(nil)
		fx.fileStoreMock.EXPECT().ListFileVariants(fileId).Return([]*storage.FileInfo{
			{}, // We can use just empty struct here, because we don't use any fields
		}, nil).AnyTimes()
		require.NoError(t, fx.AddFile(spaceId, fileId, true, false))
		fx.waitLimitReachedEvent(t, time.Second*5)
		fx.waitEmptyQueue(t, time.Second*5)

		_, err = fx.rpcStore.Get(ctx, fileNode.Cid())
		assert.Error(t, err)

		usage, err := fx.NodeUsage(ctx)
		require.NoError(t, err)
		assert.Zero(t, usage.TotalBytesUsage)
	})
}

func TestFileSync_RemoveFile(t *testing.T) {
	t.Skip("https://linear.app/anytype/issue/GO-1229/fix-testfilesync-removefile")
	return
	fx := newFixture(t, 1024)
	defer fx.Finish(t)
	spaceId := "spaceId"
	fileId := domain.FileId("fileId")
	require.NoError(t, fx.RemoveFile(spaceId, fileId))
	fx.waitEmptyQueue(t, time.Second*5)
}

type personalSpaceIdStub struct {
	personalSpaceId string
}

func (s *personalSpaceIdStub) Name() string          { return "personalSpaceIdStub" }
func (s *personalSpaceIdStub) Init(a *app.App) error { return nil }
func (s *personalSpaceIdStub) PersonalSpaceID() string {
	return s.personalSpaceId
}

func newFixture(t *testing.T, limit int) *fixture {
	fx := &fixture{
		FileSync:    New(),
		fileService: fileservice.New(),
		ctrl:        gomock.NewController(t),
		a:           new(app.App),
	}

	fileStoreMock := NewMockFileStore(fx.ctrl)
	fileStoreMock.EXPECT().Name().Return(filestore.CName).AnyTimes()
	fileStoreMock.EXPECT().Init(gomock.Any()).AnyTimes()
	fileStoreMock.EXPECT().Run(gomock.Any()).AnyTimes()
	fileStoreMock.EXPECT().Close(gomock.Any()).AnyTimes()
	fx.fileStoreMock = fileStoreMock

	personalSpaceIdGetter := &personalSpaceIdStub{personalSpaceId: "space1"}

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

	fx.a.Register(fx.fileService).
		Register(localFileStorage).
		Register(datastore.NewInMemory()).
		Register(rpcstore.NewInMemoryService(fx.rpcStore)).
		Register(fx.FileSync).
		Register(fileStoreMock).
		Register(personalSpaceIdGetter).
		Register(sender)
	require.NoError(t, fx.a.Start(ctx))
	return fx
}

type fixture struct {
	FileSync
	fileService      fileservice.FileService
	fileStoreMock    *MockFileStore
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

func (f *fixture) waitEmptyQueue(t *testing.T, timeout time.Duration) {
	f.waitCondition(t, timeout, func() bool {
		ss, err := f.SyncStatus()
		require.NoError(t, err)
		return ss.QueueLen == 0
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
