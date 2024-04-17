package files

import (
	"context"
	"fmt"
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
	"github.com/anyproto/anytype-heart/pkg/lib/mill/schema"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	Service

	eventSender       *mock_event.MockSender
	commonFileService fileservice.FileService
	fileSyncService   filesync.FileSync
	rpcStore          rpcstore.RpcStore
	fileStore         filestore.FileStore
}

const (
	spaceId         = "space1"
	testFileName    = "myFile"
	testFileContent = "it's my favorite file"
)

func newFixture(t *testing.T) *fixture {
	fileStore := filestore.New()
	dataStoreProvider, err := datastore.NewInMemory()
	require.NoError(t, err)

	blockStorage := filestorage.NewInMemory()

	rpcStore := rpcstore.NewInMemoryStore(1024)
	rpcStoreService := rpcstore.NewInMemoryService(rpcStore)
	commonFileService := fileservice.New()
	fileSyncService := filesync.New()
	objectStore := objectstore.NewStoreFixture(t)
	eventSender := mock_event.NewMockSender(t)

	ctx := context.Background()
	a := new(app.App)
	a.Register(dataStoreProvider)
	a.Register(fileStore)
	a.Register(commonFileService)
	a.Register(fileSyncService)
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(blockStorage)
	a.Register(objectStore)
	a.Register(rpcStoreService)
	err = a.Start(ctx)
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
		fileStore:         fileStore,
	}
}

func TestFileAdd(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	uploaded := make(chan struct{})
	fx.fileSyncService.OnUploaded(func(objectId string) error {
		close(uploaded)
		return nil
	})

	lastModifiedDate := time.Now()
	buf := strings.NewReader(testFileContent)
	fx.eventSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

	opts := []AddOption{
		WithName(testFileName),
		WithLastModifiedDate(lastModifiedDate.Unix()),
		WithReader(buf),
	}
	got, err := fx.FileAdd(ctx, spaceId, opts...)
	require.NoError(t, err)
	assert.NotEmpty(t, got.FileId)
	got.Commit()

	t.Run("expect decrypting content", func(t *testing.T) {
		file, err := fx.FileByHash(ctx, domain.FullFileId{FileId: got.FileId, SpaceId: spaceId})
		require.NoError(t, err)

		assertFileMeta(t, got, file)

		reader, err := file.Reader(ctx)
		require.NoError(t, err)

		gotContent, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, testFileContent, string(gotContent))

	})

	t.Run("expect that encrypted content stored in DAG", func(t *testing.T) {
		file, err := fx.FileByHash(ctx, domain.FullFileId{FileId: got.FileId, SpaceId: spaceId})
		require.NoError(t, err)

		contentCid := cid.MustParse(file.Info().Hash)
		encryptedContent, err := fx.commonFileService.GetFile(ctx, contentCid)
		require.NoError(t, err)
		gotEncryptedContent, err := io.ReadAll(encryptedContent)
		require.NoError(t, err)
		assert.NotEqual(t, testFileContent, string(gotEncryptedContent))
	})

	t.Run("check that file is uploaded to backup node", func(t *testing.T) {
		err = fx.fileSyncService.AddFile("objectId1", domain.FullFileId{SpaceId: spaceId, FileId: got.FileId}, true, false)
		require.NoError(t, err)
		<-uploaded
		infos, err := fx.rpcStore.FilesInfo(ctx, spaceId, got.FileId)
		require.NoError(t, err)

		require.Len(t, infos, 1)

		assert.Equal(t, got.FileId.String(), infos[0].FileId)
	})
}

func TestIndexFile(t *testing.T) {
	t.Run("with encryption keys available", func(t *testing.T) {
		fx := newFixture(t)

		fileResult := testAddFile(t, fx)

		// Delete from index
		err := fx.fileStore.DeleteFile(fileResult.FileId)
		require.NoError(t, err)

		err = fx.fileStore.AddFileKeys(*fileResult.EncryptionKeys)
		require.NoError(t, err)

		// Index
		file, err := fx.FileByHash(context.Background(), domain.FullFileId{FileId: fileResult.FileId, SpaceId: spaceId})
		require.NoError(t, err)

		assertFileMeta(t, fileResult, file)
	})

	t.Run("with encryption keys not available", func(t *testing.T) {
		fx := newFixture(t)

		fileResult := testAddFile(t, fx)

		// Delete from index
		err := fx.fileStore.DeleteFile(fileResult.FileId)
		require.NoError(t, err)

		_, err = fx.FileByHash(context.Background(), domain.FullFileId{FileId: fileResult.FileId, SpaceId: spaceId})
		require.Error(t, err)
	})
}

func assertFileMeta(t *testing.T, fileResult *AddResult, file File) {
	assert.Equal(t, fileResult.FileId, file.FileId())
	assert.Equal(t, fileResult.MIME, file.Meta().Media)
	assert.Equal(t, testFileName, file.Meta().Name)
	assert.Equal(t, int64(len(testFileContent)), file.Meta().Size)

	now := time.Now()
	assert.True(t, now.Sub(time.Unix(file.Meta().LastModifiedDate, 0)) < time.Second)
	assert.True(t, now.Sub(file.Meta().Added) < time.Second)
}

func TestFileAddWithCustomKeys(t *testing.T) {
	t.Run("with valid keys expect use them", func(t *testing.T) {
		fx := newFixture(t)
		ctx := context.Background()

		uploaded := make(chan struct{})
		fx.fileSyncService.OnUploaded(func(objectId string) error {
			close(uploaded)
			return nil
		})

		lastModifiedDate := time.Now()
		buf := strings.NewReader(testFileContent)
		fx.eventSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

		customKeys := map[string]string{
			encryptionKeyPath(schema.LinkFile): "bweokjjonr756czpdoymdfwzromqtqb27z44tmcb2vv322y2v62ja",
		}

		opts := []AddOption{
			WithName(testFileName),
			WithLastModifiedDate(lastModifiedDate.Unix()),
			WithReader(buf),
			WithCustomEncryptionKeys(customKeys),
		}
		got, err := fx.FileAdd(ctx, spaceId, opts...)
		require.NoError(t, err)
		assert.NotEmpty(t, got.FileId)
		got.Commit()

		assertCustomEncryptionKeys(t, fx, got, customKeys)
	})

	t.Run("with invalid keys expect generate new ones", func(t *testing.T) {
		for i, customKeys := range []map[string]string{
			nil,
			{"invalid": "key"},
			{encryptionKeyPath(schema.LinkFile): "not-an-aes-key"},
		} {
			t.Run(fmt.Sprintf("case %d", i+1), func(t *testing.T) {
				fx := newFixture(t)
				ctx := context.Background()

				uploaded := make(chan struct{})
				fx.fileSyncService.OnUploaded(func(objectId string) error {
					close(uploaded)
					return nil
				})

				lastModifiedDate := time.Now()
				buf := strings.NewReader(testFileContent)
				fx.eventSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

				opts := []AddOption{
					WithName(testFileName),
					WithLastModifiedDate(lastModifiedDate.Unix()),
					WithReader(buf),
					WithCustomEncryptionKeys(customKeys),
				}
				got, err := fx.FileAdd(ctx, spaceId, opts...)
				require.NoError(t, err)
				assert.NotEmpty(t, got.FileId)
				got.Commit()

				encKeys, err := fx.fileStore.GetFileKeys(got.FileId)
				require.NoError(t, err)
				assert.NotEmpty(t, encKeys)
				assert.NotEqual(t, customKeys, encKeys)
			})
		}
	})
}

func TestAddFilesConcurrently(t *testing.T) {
	testAddConcurrently(t, func(t *testing.T, fx *fixture) *AddResult {
		return testAddFile(t, fx)
	})
}

func testAddConcurrently(t *testing.T, addFunc func(t *testing.T, fx *fixture) *AddResult) {
	fx := newFixture(t)

	const numTimes = 5
	gotCh := make(chan *AddResult, numTimes)

	for i := 0; i < numTimes; i++ {
		go func() {
			got := addFunc(t, fx)
			gotCh <- got
		}()
	}

	var prev *AddResult
	for i := 0; i < numTimes; i++ {
		got := <-gotCh

		if prev == nil {
			// The first file should be new
			assert.False(t, got.IsExisting)
			prev = got
		} else {
			assert.Equal(t, prev.FileId, got.FileId)
			assert.Equal(t, prev.EncryptionKeys, got.EncryptionKeys)
			assert.Equal(t, prev.MIME, got.MIME)
			assert.Equal(t, prev.Size, got.Size)
			assert.True(t, got.IsExisting)
		}
	}
}

func testAddFile(t *testing.T, fx *fixture) *AddResult {
	lastModifiedDate := time.Now()
	buf := strings.NewReader(testFileContent)
	opts := []AddOption{
		WithName(testFileName),
		WithLastModifiedDate(lastModifiedDate.Unix()),
		WithReader(buf),
	}
	got, err := fx.FileAdd(context.Background(), spaceId, opts...)
	require.NoError(t, err)
	got.Commit()
	return got
}
