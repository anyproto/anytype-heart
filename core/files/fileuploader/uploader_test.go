package fileuploader

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

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
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject/filemodels"
	"github.com/anyproto/anytype-heart/core/files/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/files/filesync"
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
}

func newFileServiceFixture(t *testing.T) files.Service {
	blockStorage := filestorage.NewInMemory()

	rpcStore := rpcstore.NewInMemoryStore(1024)
	rpcStoreService := rpcstore.NewInMemoryService(rpcStore)
	commonFileService := fileservice.New()
	fileSyncService := filesync.New()
	objectStore := objectstore.NewStoreFixture(t)
	eventSender := mock_event.NewMockSender(t)
	eventSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

	ctx := context.Background()
	ctrl := gomock.NewController(t)
	wallet := mock_wallet.NewMockWallet(t)
	wallet.EXPECT().Name().Return(wallet2.CName)
	wallet.EXPECT().RepoPath().Return(t.TempDir())

	a := new(app.App)
	a.Register(anystoreprovider.New())
	a.Register(commonFileService)
	a.Register(fileSyncService)
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(blockStorage)
	a.Register(objectStore)
	a.Register(rpcStoreService)
	a.Register(testutil.PrepareMock(ctx, a, mock_accountservice.NewMockService(ctrl)))
	a.Register(testutil.PrepareMock(ctx, a, wallet))
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
	fx.fileService = newFileServiceFixture(t)
	fx.fileObjectService = mock_fileobject.NewMockService(t)

	uploaderProvider := &service{
		fileService:       fx.fileService,
		tempDirProvider:   core.NewTempDirService(),
		picker:            picker,
		fileObjectService: fx.fileObjectService,
	}
	fx.Uploader = uploaderProvider.NewUploader("space1", objectorigin.None())
	return fx
}

type uplFixture struct {
	Uploader
	fileService       files.Service
	ctrl              *gomock.Controller
	picker            *mock_cache.MockObjectGetter
	fileObjectService *mock_fileobject.MockService
}

func (fx *uplFixture) tearDown() {
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
