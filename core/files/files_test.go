package files

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/pkg/lib/core/mock_core"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type dummySyncStatusWatcher struct{}

func (w *dummySyncStatusWatcher) Watch(spaceID string, id string, fileFunc func() []string) (new bool, err error) {
	return false, nil
}
func (w *dummySyncStatusWatcher) Init(a *app.App) error { return nil }
func (w *dummySyncStatusWatcher) Name() string          { return "dummySyncStatusWatcher" }

type personalSpaceIdStub struct {
	personalSpaceId string
}

func (s *personalSpaceIdStub) Name() string          { return "personalSpaceIdStub" }
func (s *personalSpaceIdStub) Init(a *app.App) error { return nil }
func (s *personalSpaceIdStub) PersonalSpaceID() string {
	return s.personalSpaceId
}

type spaceResolverStub struct {
	spaceId string
}

func (s *spaceResolverStub) Name() string          { return "spaceResolverStub" }
func (s *spaceResolverStub) Init(a *app.App) error { return nil }
func (s *spaceResolverStub) ResolveSpaceID(objectID string) (string, error) {
	return s.spaceId, nil
}

func TestFileAdd(t *testing.T) {
	// Prepare fixture
	dataStoreProvider := datastore.NewInMemory()

	blockStorage := filestorage.NewInMemory()

	rpcStorage := rpcstore.NewInMemoryService()
	commonFileService := fileservice.New()

	fileSyncService := filesync.New()

	spaceId := "space1"
	personalSpaceIdGetter := &personalSpaceIdStub{personalSpaceId: spaceId}
	spaceIdResolver := &spaceResolverStub{spaceId: spaceId}

	coreService := mock_core.NewMockService(t)
	objectStore := objectstore.NewStoreFixture(t)

	eventSender := mock_event.NewMockSender(t)

	ctx := context.Background()
	a := new(app.App)
	a.Register(dataStoreProvider)
	a.Register(filestore.New())
	a.Register(commonFileService)
	a.Register(fileSyncService)
	a.Register(testutil.PrepareMock(ctx, a, coreService))
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(blockStorage)
	a.Register(objectStore)
	a.Register(personalSpaceIdGetter)
	a.Register(spaceIdResolver)
	a.Register(&dummySyncStatusWatcher{})
	a.Register(rpcStorage)
	err := a.Start(ctx)
	require.NoError(t, err)

	s := New()
	err = s.Init(a)
	require.NoError(t, err)
	// End fixture

	fileName := "myFile"
	lastModifiedDate := time.Now()
	buf := strings.NewReader("it's my favorite file")
	eventSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

	opts := []AddOption{
		WithName(fileName),
		WithLastModifiedDate(lastModifiedDate.Unix()),
		WithReader(buf),
	}
	got, err := s.FileAdd(context.Background(), spaceId, opts...)

	require.NoError(t, err)
	assert.NotEmpty(t, got.Hash())

	// TODO Check that file is in RpcStore (Cloud Storage)
	// TODO Check that file is in BlockStore (DAG)
}
