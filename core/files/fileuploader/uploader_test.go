package fileuploader

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/simple"
	file2 "github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/core/files/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	"github.com/anyproto/anytype-heart/core/files/filesync/mock_filesync"
	wallet2 "github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

func TestUploader_Upload(t *testing.T) {
	ctx := context.Background()
	newBlock := func(tp model.BlockContentFileType) file2.Block {
		return simple.New(&model.Block{Content: &model.BlockContentOfFile{File: &model.BlockContentFile{Type: tp}}}).(file2.Block)
	}
	t.Run("empty source", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()
		res := fx.Upload(ctx)
		require.Error(t, res.Err)
	})
	t.Run("image by block type", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()

		fileObjectId := fx.expectCreateObject()

		b := newBlock(model.BlockContentFile_Image)
		res := fx.Uploader.SetBlock(b).SetFile("./testdata/unnamed.jpg").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.FileObjectId, fileObjectId)
		assert.Equal(t, res.Name, "unnamed.jpg")
		assert.Equal(t, b.Model().GetFile().Name, "unnamed.jpg")
		assert.Equal(t, res.MIME, "image/jpeg")
	})
	t.Run("corrupted image: fall back to file", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()

		fileObjectId := fx.expectCreateObject()

		b := newBlock(model.BlockContentFile_Image)
		res := fx.Uploader.SetBlock(b).SetFile("./testdata/corrupted.jpg").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.FileObjectId, fileObjectId)
		assert.Equal(t, res.Name, "corrupted.jpg")
		assert.Equal(t, b.Model().GetFile().Name, "corrupted.jpg")
		assert.Equal(t, res.MIME, "image/jpeg")
	})
	t.Run("image type detect", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()

		fx.expectCreateObject()

		res := fx.Uploader.SetFile("./testdata/unnamed.jpg").Upload(ctx)
		require.NoError(t, res.Err)
	})
	t.Run("image to file failover", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()

		fileObjectId := fx.expectCreateObject()

		b := newBlock(model.BlockContentFile_Image)
		res := fx.Uploader.SetBlock(b).SetFile("./testdata/test.txt").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.FileObjectId, fileObjectId)
		assert.Equal(t, res.Name, "test.txt")
		assert.Equal(t, b.Model().GetFile().Name, "test.txt")
		assert.Equal(t, b.Model().GetFile().Type, model.BlockContentFile_File)
	})
	t.Run("file from url", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()

		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, "./testdata/unnamed.jpg")
		})
		serv := httptest.NewServer(mux)
		defer serv.Close()

		fileObjectId := fx.expectCreateObject()

		res := fx.Uploader.SetUrl(serv.URL + "/unnamed.jpg").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.FileObjectId, fileObjectId)
		assert.Equal(t, res.Name, "unnamed.jpg")
		res.Size = 1
		b := res.ToBlock()
		assert.Equal(t, b.Model().GetFile().Name, "unnamed.jpg")
	})
	t.Run("file from Content-Disposition", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()

		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Disposition", "form-data; name=\"fieldName\"; filename=\"filename\"")
			http.ServeFile(w, r, "./testdata/unnamed.jpg")
		})
		serv := httptest.NewServer(mux)
		defer serv.Close()

		fileObjectId := fx.expectCreateObject()

		res := fx.Uploader.SetUrl(serv.URL + "/unnamed.jpg").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.FileObjectId, fileObjectId)
		assert.Equal(t, res.Name, "filename")
		res.Size = 1
		b := res.ToBlock()
		assert.Equal(t, b.Model().GetFile().Name, "filename")
	})
	t.Run("file without url params", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()

		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, "./testdata/unnamed.jpg")
		})
		serv := httptest.NewServer(mux)
		defer serv.Close()

		fileObjectId := fx.expectCreateObject()

		res := fx.Uploader.SetUrl(serv.URL + "/unnamed.jpg?text=text").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.FileObjectId, fileObjectId)
		assert.Equal(t, res.Name, "unnamed.jpg")
		res.Size = 1
		b := res.ToBlock()
		assert.Equal(t, b.Model().GetFile().Name, "unnamed.jpg")
	})
	t.Run("bytes", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()

		fileObjectId := fx.expectCreateObject()

		inputContent := "my bytes"
		res := fx.Uploader.SetBytes([]byte(inputContent)).SetName("filename").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.FileObjectId, fileObjectId)
		assert.Equal(t, res.Name, "filename")

		fileId := domain.FileId(res.FileObjectDetails.GetString(bundle.RelationKeyFileId))
		fullId := domain.FullFileId{FileId: fileId, SpaceId: "space1"}
		variants, err := fx.fileService.GetFileVariants(ctx, fullId, res.EncryptionKeys)
		require.NoError(t, err)

		file, err := files.NewFile(fx.fileService, fullId, variants)
		require.NoError(t, err)
		reader, err := file.Reader(ctx)
		require.NoError(t, err)

		gotContent, err := io.ReadAll(reader)
		require.NoError(t, err)

		assert.Equal(t, inputContent, string(gotContent))
	})
	t.Run("preload file and discard", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()

		// Step 1: Preload file without creating object
		inputContent := "preloaded content for reuse"
		preloadedFileId, err := fx.Uploader.
			SetBytes([]byte(inputContent)).
			SetName("preloaded.txt").
			Preload(ctx)

		require.NoError(t, err)
		require.NotEmpty(t, preloadedFileId, "preload should return file ID")

		// Step 2: Verify we can create object from same preloaded file
		// Get the preloaded result to access encryption keys
		preloadResult, ok := fx.service.GetPreloadResult(preloadedFileId)
		require.True(t, ok, "should find preloaded result")
		require.NotNil(t, preloadResult)

		require.NotNil(t, preloadResult.Batch)
		err = preloadResult.Batch.Discard()
		require.NoError(t, err, "discarding batch should succeed")

		fx.service.RemovePreloadResult(preloadedFileId)
		preloadResult, ok = fx.service.GetPreloadResult(preloadedFileId)
		require.False(t, ok, "preload result should be removed")
		require.Nil(t, preloadResult)
		// upload using preloaded file
		res := fx.Uploader.SetPreloadId(preloadedFileId).Upload(context.Background())
		require.Error(t, res.Err)
	})
	t.Run("reuse existing object from preloaded file", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()

		// Step 1: Preload file without creating object
		inputContent := "preloaded content for reuse2"
		preloadedFileId, err := fx.Uploader.
			SetBytes([]byte(inputContent)).
			SetName("preloaded.txt").
			Preload(ctx)

		require.NoError(t, err)
		require.NotEmpty(t, preloadedFileId, "preload should return file ID")

		// Step 2: Verify we can create object from same preloaded file
		// Get the preloaded result to access encryption keys
		preloadResult, ok := fx.service.GetPreloadResult(preloadedFileId)
		require.True(t, ok, "should find preloaded result")
		require.NotNil(t, preloadResult)
		require.NotNil(t, preloadResult.Batch)

		// Mock the file object creation
		fileObjectId := fx.expectCreateObject()

		// upload using preloaded file
		res := fx.Uploader.SetPreloadId(preloadedFileId).Upload(context.Background())
		require.NoError(t, res.Err)
		assert.Equal(t, res.Name, "preloaded.txt")
		assert.Equal(t, res.FileObjectId, fileObjectId)
		require.NotNil(t, res.FileObjectDetails)
	})
	t.Run("upload svg image", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()

		fileObjectId := fx.expectCreateObject()

		b := newBlock(model.BlockContentFile_Image)
		res := fx.Uploader.SetBlock(b).SetFile("./testdata/test.svg").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.FileObjectId, fileObjectId)
		assert.Equal(t, res.Name, "test.svg")
		assert.Equal(t, b.Model().GetFile().Name, "test.svg")
	})
	t.Run("create object from preloaded file - simple", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()

		// Test simple case: try to create object from non-existent preloaded file
		preloadedFileId := "non-existent-file-id"

		// Try to create object from non-existent preloaded file
		uploader := fx.service.NewUploader("space1", objectorigin.None())
		createRes := uploader.
			SetPreloadId(preloadedFileId).
			SetType(model.BlockContentFile_File).
			Upload(ctx)

		// Should fail because preload result doesn't exist
		require.Error(t, createRes.Err)
		require.Contains(t, createRes.Err.Error(), "no preload result found")
	})

	t.Run("async preload - immediate return and blocking upload", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()

		// Create a large file content to simulate slow processing
		largeContent := make([]byte, 1024*1024) // 1MB
		for i := range largeContent {
			largeContent[i] = byte(i % 256)
		}

		// Start preload - should return immediately
		start := time.Now()
		preloadedFileId, err := fx.Uploader.
			SetBytes(largeContent).
			SetName("large.bin").
			Preload(ctx)

		require.NoError(t, err)
		require.NotEmpty(t, preloadedFileId)

		// Preload should return almost immediately (< 100ms)
		elapsed := time.Since(start)
		require.Less(t, elapsed, 100*time.Millisecond, "Preload should return immediately")

		// Mock that object doesn't exist yet
		fx.fileObjectService.EXPECT().
			GetObjectDetailsByFileId(mock.Anything).
			Return("", nil, filemodels.ErrObjectNotFound).Maybe()

		fx.fileObjectService.EXPECT().
			Create(mock.Anything, mock.Anything, mock.Anything).
			Return("object123", &domain.Details{}, nil).Maybe()

		// Now try to upload using the preloadId - this should block until preload completes
		uploader := fx.service.NewUploader("space1", objectorigin.None())
		createRes := uploader.
			SetPreloadId(preloadedFileId).
			SetType(model.BlockContentFile_File).
			Upload(ctx)

		// Upload should succeed after waiting for preload
		require.NoError(t, createRes.Err)
		require.NotEmpty(t, createRes.FileObjectId)

		// Wait a bit to ensure async preload has completed
		time.Sleep(100 * time.Millisecond)

		// Verify the preload result is available
		result, ok := fx.service.GetPreloadResult(preloadedFileId)
		require.True(t, ok, "preload result should be available")
		require.NotNil(t, result)
		require.Equal(t, int64(len(largeContent)), result.Size)
	})
}

func newFileServiceFixture(t *testing.T, blockStorage filestorage.FileStorage) files.Service {
	commonFileService := fileservice.New()

	fileSyncService := mock_filesync.NewMockFileSync(t)
	objectStore := objectstore.NewStoreFixture(t)

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	wallet := mock_wallet.NewMockWallet(t)
	wallet.EXPECT().Name().Return(wallet2.CName)
	wallet.EXPECT().RepoPath().Return(t.TempDir())

	a := new(app.App)
	a.Register(anystoreprovider.New())
	a.Register(commonFileService)
	a.Register(blockStorage)
	a.Register(objectStore)
	a.Register(testutil.PrepareMock(ctx, a, mock_accountservice.NewMockService(ctrl)))
	a.Register(testutil.PrepareMock(ctx, a, wallet))
	a.Register(testutil.PrepareMock(ctx, a, fileSyncService))
	a.Register(&config.Config{DisableFileConfig: true, NetworkMode: pb.RpcAccount_DefaultConfig, PeferYamuxTransport: true})

	err := a.Start(ctx)
	require.NoError(t, err)

	s := files.New()
	err = s.Init(a)
	require.NoError(t, err)

	return s
}

func newFixture(t *testing.T) *uplFixture {
	picker := mock_cache.NewMockObjectGetter(t)
	fx := &uplFixture{
		ctrl:   gomock.NewController(t),
		picker: picker,
	}

	// Create a shared storage instance
	sharedStorage := filestorage.NewInMemory()

	fx.fileService = newFileServiceFixture(t, sharedStorage)
	fx.fileObjectService = mock_fileobject.NewMockService(t)

	uploaderProvider := &service{
		fileService:       fx.fileService,
		fileStorage:       sharedStorage,
		tempDirProvider:   core.NewTempDirService(),
		picker:            picker,
		fileObjectService: fx.fileObjectService,
		preloadEntries:    make(map[string]*preloadEntry),
	}
	uploaderProvider.ctx, uploaderProvider.ctxCancel = context.WithCancel(context.Background())
	fx.service = uploaderProvider
	fx.Uploader = uploaderProvider.NewUploader("space1", objectorigin.None())
	return fx
}

type uplFixture struct {
	Uploader
	service           *service
	fileService       files.Service
	ctrl              *gomock.Controller
	picker            *mock_cache.MockObjectGetter
	fileObjectService *mock_fileobject.MockService
}

func (fx *uplFixture) tearDown() {
	fx.service.ctxCancel()
	fx.ctrl.Finish()
}

func (fx *uplFixture) expectCreateObject() string {
	fileObjectId := "fileObjectId1"

	fx.fileObjectService.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, s string, request filemodels.CreateRequest) (string, *domain.Details, error) {
		details := domain.NewDetails()
		details.SetString(bundle.RelationKeyFileId, request.FileId.String())
		return fileObjectId, details, nil
	})
	return fileObjectId
}
