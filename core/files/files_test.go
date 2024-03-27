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

const spaceId = "space1"

func newFixture(t *testing.T) *fixture {
	fileStore := filestore.New()
	dataStoreProvider := datastore.NewInMemory()

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
		fileStore:         fileStore,
	}
}

func TestFileAdd(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

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
	got, err := fx.FileAdd(ctx, spaceId, opts...)
	require.NoError(t, err)
	assert.NotEmpty(t, got.FileId)
	got.Commit()

	t.Run("expect decrypting content", func(t *testing.T) {
		file, err := fx.FileByHash(ctx, domain.FullFileId{FileId: got.FileId, SpaceId: spaceId})
		require.NoError(t, err)

		reader, err := file.Reader(ctx)
		require.NoError(t, err)

		gotContent, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, fileContent, string(gotContent))

	})

	t.Run("expect that encrypted content stored in DAG", func(t *testing.T) {
		file, err := fx.FileByHash(ctx, domain.FullFileId{FileId: got.FileId, SpaceId: spaceId})
		require.NoError(t, err)

		contentCid := cid.MustParse(file.Info().Hash)
		encryptedContent, err := fx.commonFileService.GetFile(ctx, contentCid)
		require.NoError(t, err)
		gotEncryptedContent, err := io.ReadAll(encryptedContent)
		require.NoError(t, err)
		assert.NotEqual(t, fileContent, string(gotEncryptedContent))
	})

	t.Run("check that file is uploaded to backup node", func(t *testing.T) {
		err = fx.fileSyncService.AddFile(domain.FullFileId{SpaceId: spaceId, FileId: got.FileId}, true, false)
		require.NoError(t, err)
		<-uploaded
		infos, err := fx.rpcStore.FilesInfo(ctx, spaceId, got.FileId)
		require.NoError(t, err)

		require.Len(t, infos, 1)

		assert.Equal(t, got.FileId.String(), infos[0].FileId)
	})
}

func TestFileAddWithCustomKeys(t *testing.T) {
	t.Run("with valid keys expect use them", func(t *testing.T) {
		fx := newFixture(t)
		ctx := context.Background()

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

		customKeys := map[string]string{
			encryptionKeyPath(schema.LinkFile): "bweokjjonr756czpdoymdfwzromqtqb27z44tmcb2vv322y2v62ja",
		}

		opts := []AddOption{
			WithName(fileName),
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
	fileName := "myFile"
	lastModifiedDate := time.Now()
	fileContent := "it's my favorite file"
	buf := strings.NewReader(fileContent)
	opts := []AddOption{
		WithName(fileName),
		WithLastModifiedDate(lastModifiedDate.Unix()),
		WithReader(buf),
	}
	got, err := fx.FileAdd(context.Background(), spaceId, opts...)
	require.NoError(t, err)
	got.Commit()
	return got
}

func givenCustomEncryptionKeys() map[string]string {
	return map[string]string{
		encryptionKeyPath(schema.LinkImageOriginal):  "bweokjjonr756czpdoymdfwzromqtqb27z44tmcb2vv322y2v62ja",
		encryptionKeyPath(schema.LinkImageLarge):     "bweokjjonr756czpdoymdfwzromqtqb27z44tmcb2vv322y2v62ja",
		encryptionKeyPath(schema.LinkImageSmall):     "bear36qgxpvnsqis2omwqi33zcrjo6arxhokpqr3bnh2oqphxkiba",
		encryptionKeyPath(schema.LinkImageThumbnail): "bcewq7zoa6cbbev6nxkykrrclvidriuglgags67zbdda53wfnn6eq",
		encryptionKeyPath(schema.LinkImageExif):      "bdoiogvdd5bayrezafzf2lvgh3xxjk7ru4yq2frpxhjgmx26ih6sq",
	}
}
