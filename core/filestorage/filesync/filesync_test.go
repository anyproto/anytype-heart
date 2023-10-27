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
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/storage"
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

func newFixture(t *testing.T) *fixture {
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
	sender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

	fx.a.Register(fx.fileService).
		Register(filestorage.NewInMemory()).
		Register(datastore.NewInMemory()).
		Register(rpcstore.NewInMemoryService()).
		Register(fx.FileSync).
		Register(fileStoreMock).
		Register(personalSpaceIdGetter).
		Register(sender)
	require.NoError(t, fx.a.Start(ctx))
	return fx
}

type fixture struct {
	FileSync
	fileService   fileservice.FileService
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
