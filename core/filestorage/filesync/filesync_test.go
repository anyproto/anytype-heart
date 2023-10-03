//go:generate mockgen -package filesync -destination filestore_mock.go github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore FileStore

package filesync

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/dgraph-io/badger/v3"
	"github.com/ipfs/go-cid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore/mock_rpcstore"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

func TestFileSync_AddFile(t *testing.T) {
	fx := newFixture(t)
	defer fx.Finish(t)
	var buf = make([]byte, 1024*1024)
	_, err := rand.Read(buf)
	require.NoError(t, err)
	n, err := fx.fileService.AddFile(ctx, bytes.NewReader(buf))
	require.NoError(t, err)
	fileId := n.Cid().String()
	spaceId := "space1"

	fx.fileStoreMock.EXPECT().GetSyncStatus(fileId).Return(int(syncstatus.StatusNotSynced), nil)
	fx.fileStoreMock.EXPECT().GetFileSize(fileId).Return(0, fmt.Errorf("not found"))
	fx.fileStoreMock.EXPECT().SetFileSize(fileId, gomock.Any()).Return(nil)
	fx.fileStoreMock.EXPECT().ListByTarget(fileId).Return([]*storage.FileInfo{
		{}, // We can use just empty struct here, because we don't use any fields
	}, nil).AnyTimes()
	// TODO Test when limit is reached
	fx.rpcStore.EXPECT().CheckAvailability(gomock.Any(), spaceId, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, cids []cid.Cid) ([]*fileproto.BlockAvailability, error) {
		res := lo.Map(cids, func(c cid.Cid, _ int) *fileproto.BlockAvailability {
			return &fileproto.BlockAvailability{
				Cid:    c.Bytes(),
				Status: fileproto.AvailabilityStatus_NotExists,
			}
		})
		return res, nil
	})
	// fx.rpcStore.EXPECT().BindCids(gomock.Any(), spaceId, fileId, gomock.Any()).Return(nil)
	fx.rpcStore.EXPECT().SpaceInfo(gomock.Any(), spaceId).Return(&fileproto.SpaceInfoResponse{LimitBytes: 2 * 1024 * 1024}, nil).AnyTimes()
	fx.rpcStore.EXPECT().AddToFile(gomock.Any(), spaceId, fileId, gomock.Any()).AnyTimes()
	require.NoError(t, fx.AddFile(spaceId, fileId, false, false))
	fx.waitEmptyQueue(t, time.Second*5)
}

func TestFileSync_RemoveFile(t *testing.T) {
	t.Skip("https://linear.app/anytype/issue/GO-1229/fix-testfilesync-removefile")
	return
	fx := newFixture(t)
	defer fx.Finish(t)
	spaceId := "spaceId"
	fileId := "fileId"
	fx.rpcStore.EXPECT().DeleteFiles(gomock.Any(), spaceId, fileId).Return(nil)
	require.NoError(t, fx.RemoveFile(spaceId, fileId))
	fx.waitEmptyQueue(t, time.Second*5)
}

func newFixture(t *testing.T) *fixture {
	fx := &fixture{
		FileSync:    New(),
		fileService: fileservice.New(),
		ctrl:        gomock.NewController(t),
		a:           new(app.App),
	}
	var err error
	bp := &badgerProvider{}
	fx.tmpDir, err = os.MkdirTemp("", "*")
	require.NoError(t, err)
	bp.db, err = badger.Open(badger.DefaultOptions(fx.tmpDir))
	require.NoError(t, err)

	fx.rpcStore = mock_rpcstore.NewMockRpcStore(fx.ctrl)
	fx.rpcStore.EXPECT().SpaceInfo(gomock.Any(), "space1").Return(&fileproto.SpaceInfoResponse{LimitBytes: 2 * 1024 * 1024}, nil).AnyTimes()

	mockRpcStoreService := mock_rpcstore.NewMockService(fx.ctrl)
	mockRpcStoreService.EXPECT().Name().Return(rpcstore.CName).AnyTimes()
	mockRpcStoreService.EXPECT().Init(gomock.Any()).AnyTimes()
	mockRpcStoreService.EXPECT().NewStore().Return(fx.rpcStore)

	fileStoreMock := NewMockFileStore(fx.ctrl)
	fileStoreMock.EXPECT().Name().Return(filestore.CName).AnyTimes()
	fileStoreMock.EXPECT().Init(gomock.Any()).AnyTimes()
	fileStoreMock.EXPECT().Run(gomock.Any()).AnyTimes()
	fileStoreMock.EXPECT().Close(gomock.Any()).AnyTimes()
	fx.fileStoreMock = fileStoreMock

	spaceService := mock_space.NewMockService(t)
	spaceService.EXPECT().AccountId().Return("space1").Maybe()

	sender := mock_event.NewMockSender(t)
	sender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

	fx.a.Register(fx.fileService).
		Register(filestorage.NewInMemory()).
		Register(bp).
		Register(mockRpcStoreService).
		Register(fx.FileSync).
		Register(fileStoreMock).
		Register(testutil.PrepareRunnableMock(ctx, fx.a, spaceService)).
		Register(testutil.PrepareMock(fx.a, sender))
	require.NoError(t, fx.a.Start(ctx))
	return fx
}

type fixture struct {
	FileSync
	fileService   fileservice.FileService
	rpcStore      *mock_rpcstore.MockRpcStore
	fileStoreMock *MockFileStore
	ctrl          *gomock.Controller
	a             *app.App
	tmpDir        string
}

func (f *fixture) waitEmptyQueue(t *testing.T, timeout time.Duration) {
	retryTime := time.Millisecond * 10
	for i := 0; i < int(timeout/retryTime); i++ {
		time.Sleep(retryTime)
		ss, err := f.SyncStatus()
		require.NoError(t, err)
		if ss.QueueLen == 0 {
			return
		}
	}
	require.False(t, true, "queue is not empty: timeout")
}

func (f *fixture) Finish(t *testing.T) {
	defer os.RemoveAll(f.tmpDir)
	require.NoError(t, f.a.Close(ctx))
}

type badgerProvider struct {
	db *badger.DB
}

func (b *badgerProvider) Init(a *app.App) (err error) {
	return nil
}

func (b *badgerProvider) Name() (name string) {
	return datastore.CName
}

func (b *badgerProvider) Run(ctx context.Context) (err error) {
	return nil
}

func (b *badgerProvider) Close(ctx context.Context) (err error) {
	return b.db.Close()
}

func (b *badgerProvider) LocalStorage() (*badger.DB, error) {
	return b.db, nil
}

func (b *badgerProvider) SpaceStorage() (*badger.DB, error) {
	return b.db, nil
}
