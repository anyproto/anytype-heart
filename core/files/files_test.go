package files

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/dgraph-io/badger/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/files/mock_fileservice"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync/mock_filesync"
	"github.com/anyproto/anytype-heart/core/filestorage/mock_filestorage"
	"github.com/anyproto/anytype-heart/pkg/lib/core/mock_core"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/mock_datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type dummySyncStatusWatcher struct{}

func (w *dummySyncStatusWatcher) Watch(spaceID string, id string, fileFunc func() []string) (new bool, err error) {
	return false, nil
}

func (w *dummySyncStatusWatcher) Init(a *app.App) error {
	return nil
}

func (w *dummySyncStatusWatcher) Name() string {
	return "dummySyncStatusWatcher"
}

func TestFileAdd(t *testing.T) {
	// Prepare fixture
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	require.NoError(t, err)

	storeProvider := mock_datastore.NewMockDatastore(t)
	storeProvider.EXPECT().LocalStorage().Return(db, nil)

	testDag := newTestDag()
	commonFileService := mock_fileservice.NewMockFileService(t)
	commonFileService.EXPECT().DAGService().Return(testDag)
	fileSyncService := mock_filesync.NewMockFileSync(t)
	spaceService := mock_space.NewMockService(t)
	coreService := mock_core.NewMockService(t)
	blockStorage := mock_filestorage.NewMockFileStorage(t)
	objectStore := objectstore.NewStoreFixture(t)

	a := new(app.App)
	a.Register(testutil.PrepareMock(a, storeProvider))
	a.Register(filestore.New())
	a.Register(testutil.PrepareMock(a, commonFileService))
	a.Register(testutil.PrepareMock(a, fileSyncService))
	a.Register(testutil.PrepareMock(a, spaceService))
	a.Register(testutil.PrepareMock(a, coreService))
	a.Register(testutil.PrepareMock(a, blockStorage))
	a.Register(objectStore)
	a.Register(&dummySyncStatusWatcher{})
	err = a.Start(context.Background())
	require.NoError(t, err)

	s := New()
	err = s.Init(a)
	require.NoError(t, err)
	// End fixture

	spaceID := "space1"
	fileName := "myFile"
	lastModifiedDate := time.Now()
	buf := strings.NewReader("it's my favorite file")

	opts := []AddOption{
		WithName(fileName),
		WithLastModifiedDate(lastModifiedDate.Unix()),
		WithReader(buf),
	}
	got, err := s.FileAdd(context.Background(), spaceID, opts...)

	require.NoError(t, err)
	assert.NotEmpty(t, got.Hash())
}
