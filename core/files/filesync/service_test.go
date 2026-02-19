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
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/kanban/mock_kanban"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

type dummyCollectionService struct {
}

func (s *dummyCollectionService) SubscribeForCollection(collectionID string, subscriptionID string) ([]string, <-chan []string, error) {
	return nil, nil, nil
}

func (s *dummyCollectionService) UnsubscribeFromCollection(collectionID string, subscriptionID string) error {
	return nil
}

func (s *dummyCollectionService) Name() string          { return "dummyCollectionService" }
func (s *dummyCollectionService) Init(a *app.App) error { return nil }

func newFixtureNotStarted(t *testing.T, limit int) *fixture {
	fx := &fixture{
		fileSync:    New().(*fileSync),
		fileService: fileservice.New(),
		ctrl:        gomock.NewController(t),
		a:           new(app.App),
	}

	sender := mock_event.NewMockSender(t)
	sender.EXPECT().Broadcast(mock.Anything).Run(func(e *pb.Event) {
		fx.eventsLock.Lock()
		defer fx.eventsLock.Unlock()
		fx.events = append(fx.events, e)
	}).Maybe()

	fx.rpcStore = rpcstore.NewInMemoryStore(limit)
	localFileStorage := filestorage.NewInMemory()
	fx.localFileStorage = localFileStorage

	ctrl := gomock.NewController(t)
	wallet := mock_wallet.NewMockWallet(t)
	wallet.EXPECT().RepoPath().Return(t.TempDir()).Maybe()

	kanbanService := mock_kanban.NewMockService(t)

	spaceService := mock_space.NewMockService(t)
	spaceService.EXPECT().TechSpaceId().Return("techSpaceId").Maybe()

	fx.subService = subscription.NewInternalTestService(t)

	fx.subService.AddObjects(t, "techSpaceId", []spaceindex.TestObject{
		{
			bundle.RelationKeyId:             domain.String("spaceView1"),
			bundle.RelationKeyTargetSpaceId:  domain.String("space1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(model.ObjectType_spaceView),
		},
	})

	collectionService := &dummyCollectionService{}

	dbProvider, err := anystoreprovider.NewInPath(t.TempDir())
	require.NoError(t, err)

	fx.a.Register(fx.fileService).
		Register(localFileStorage).
		Register(dbProvider).
		Register(rpcstore.NewInMemoryService(fx.rpcStore)).
		Register(fx.fileSync).
		Register(fx.subService).
		Register(testutil.PrepareMock(ctx, fx.a, sender)).
		Register(testutil.PrepareMock(ctx, fx.a, kanbanService)).
		Register(collectionService).
		Register(testutil.PrepareMock(ctx, fx.a, mock_accountservice.NewMockService(ctrl))).
		Register(testutil.PrepareMock(ctx, fx.a, wallet)).
		Register(testutil.PrepareMock(ctx, fx.a, spaceService)).
		Register(&config.Config{DisableFileConfig: true, NetworkMode: pb.RpcAccount_DefaultConfig, PeferYamuxTransport: true})

	return fx
}

func newFixture(t *testing.T, limit int) *fixture {
	fx := newFixtureNotStarted(t, limit)

	err := fx.a.Start(ctx)
	require.NoError(t, err)

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
	subService       *subscription.InternalTestService
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
