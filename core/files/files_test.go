package files

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

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

type fixture struct {
	Service

	eventSender       *mock_event.MockSender
	commonFileService fileservice.FileService
	fileSyncService   filesync.FileSync
	rpcStore          rpcstore.RpcStore
}

const spaceId = "space1"

func newFixture(t *testing.T) *fixture {
	dataStoreProvider := datastore.NewInMemory()

	blockStorage := filestorage.NewInMemory()

	rpcStore := rpcstore.NewInMemoryStore(1024)
	rpcStoreService := rpcstore.NewInMemoryService(rpcStore)
	commonFileService := fileservice.New()

	fileSyncService := filesync.New()

	personalSpaceIdGetter := &personalSpaceIdStub{personalSpaceId: spaceId}
	spaceIdResolver := &spaceResolverStub{spaceId: spaceId}

	objectStore := objectstore.NewStoreFixture(t)

	eventSender := mock_event.NewMockSender(t)

	ctx := context.Background()
	a := new(app.App)
	a.Register(dataStoreProvider)
	a.Register(filestore.New())
	a.Register(commonFileService)
	a.Register(fileSyncService)
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(blockStorage)
	a.Register(objectStore)
	a.Register(personalSpaceIdGetter)
	a.Register(spaceIdResolver)
	a.Register(rpcStoreService)
	err := a.Start(ctx)
	require.NoError(t, err)

	s := New()
	err = s.Init(a)
	require.NoError(t, err)
	return &fixture{
		Service:           s,
		eventSender:       eventSender,
		commonFileService: commonFileService,
		fileSyncService:   fileSyncService,
		rpcStore:          rpcStore,
	}
}

func TestIsFileExistOnNode(t *testing.T) {
	t.Run("file not exist", func(t *testing.T) {
		fx := newFixture(t)

		fileId := domain.FileId("bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku")
		got, err := fx.IsFileExistOnNode(context.Background(), fileId)
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("file exists", func(t *testing.T) {
		fx := newFixture(t)

		ctx := context.Background()
		buf := strings.NewReader("file content")

		node, err := fx.commonFileService.AddFile(ctx, buf)
		require.NoError(t, err)

		fileId := domain.FileId(node.Cid().String())

		got, err := fx.IsFileExistOnNode(context.Background(), fileId)
		require.NoError(t, err)
		assert.True(t, got)
	})
}

func TestFileAdd(t *testing.T) {
	fx := newFixture(t)

	uploaded := make(chan struct{})
	fx.fileSyncService.OnUploaded(func(fileId domain.FileId) error {
		close(uploaded)
		return nil
	})

	fileName := "myFile"
	lastModifiedDate := time.Now()
	fileContent := "it's my favorite file"
	buf := strings.NewReader(fileContent)
	fx.eventSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

	opts := []AddOption{
		WithName(fileName),
		WithLastModifiedDate(lastModifiedDate.Unix()),
		WithReader(buf),
	}
	got, err := fx.FileAdd(context.Background(), spaceId, opts...)

	require.NoError(t, err)
	assert.NotEmpty(t, got.FileId)

	t.Run("want to decrypt file content", func(t *testing.T) {
		reader, err := got.File.Reader(context.Background())
		require.NoError(t, err)

		gotContent, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, fileContent, string(gotContent))

	})

	t.Run("want to store encrypted content in DAG", func(t *testing.T) {
		encryptedContent, err := fx.commonFileService.GetFile(context.Background(), cid.MustParse(got.File.Info().Hash))
		require.NoError(t, err)
		gotEncryptedContent, err := io.ReadAll(encryptedContent)
		require.NoError(t, err)
		assert.NotEqual(t, fileContent, string(gotEncryptedContent))

	})

	t.Run("check that file is uploaded to backup node", func(t *testing.T) {
		err = fx.fileSyncService.AddFile(spaceId, got.FileId, true, false)
		require.NoError(t, err)
		<-uploaded
		infos, err := fx.rpcStore.FilesInfo(context.Background(), spaceId, got.FileId)
		require.NoError(t, err)

		require.Len(t, infos, 1)

		assert.Equal(t, got.FileId.String(), infos[0].FileId)
	})
}
