package filesync

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	wallet2 "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

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
	ctrl := gomock.NewController(t)
	wallet := mock_wallet.NewMockWallet(t)
	wallet.EXPECT().Name().Return(wallet2.CName)
	wallet.EXPECT().RepoPath().Return("repo/path")

	fx.a.Register(fx.fileService).
		Register(localFileStorage).
		Register(dataStoreProvider).
		Register(rpcstore.NewInMemoryService(fx.rpcStore)).
		Register(fx.fileSync).
		Register(fileStore).
		Register(sender).
		Register(testutil.PrepareMock(ctx, fx.a, mock_accountservice.NewMockService(ctrl))).
		Register(testutil.PrepareMock(ctx, fx.a, wallet)).
		Register(&config.Config{DisableFileConfig: true, NetworkMode: pb.RpcAccount_DefaultConfig, PeferYamuxTransport: true})

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
	rpcStore         *rpcstore.InMemoryStore
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

type queueLen interface {
	Len() int
}

func (f *fixture) waitEmptyQueue(t *testing.T, queue queueLen, timeout time.Duration) {
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
