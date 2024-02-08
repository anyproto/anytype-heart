package fileuploader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/getblock/mock_getblock"
	"github.com/anyproto/anytype-heart/core/block/simple"
	file2 "github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/testMock"
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

		fx.expectImageAdd()
		fileObjectId := fx.expectCreateObject()

		b := newBlock(model.BlockContentFile_Image)
		res := fx.Uploader.SetBlock(b).SetFile("./testdata/unnamed.jpg").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.FileObjectId, fileObjectId)
		assert.Equal(t, res.Name, "unnamed.jpg")
		assert.Equal(t, b.Model().GetFile().Name, "unnamed.jpg")
		assert.Equal(t, res.MIME, "image/jpg")
	})
	t.Run("image type detect", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()

		fx.expectImageAdd()
		fx.expectCreateObject()

		res := fx.Uploader.AutoType(true).SetFile("./testdata/unnamed.jpg").Upload(ctx)
		require.NoError(t, res.Err)
	})
	t.Run("image to file failover", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()

		fx.expectFileAdd()
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

		fx.expectImageAdd()
		fileObjectId := fx.expectCreateObject()

		res := fx.Uploader.AutoType(true).SetUrl(serv.URL + "/unnamed.jpg").Upload(ctx)
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

		fx.expectImageAdd()
		fileObjectId := fx.expectCreateObject()

		res := fx.Uploader.AutoType(true).SetUrl(serv.URL + "/unnamed.jpg").Upload(ctx)
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

		fx.expectImageAdd()
		fileObjectId := fx.expectCreateObject()

		res := fx.Uploader.AutoType(true).SetUrl(serv.URL + "/unnamed.jpg?text=text").Upload(ctx)
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

		fx.expectFileAdd()
		fileObjectId := fx.expectCreateObject()

		res := fx.Uploader.SetBytes([]byte("my bytes")).SetName("filename").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.FileObjectId, fileObjectId)
		assert.Equal(t, res.Name, "filename")
	})
}

func newFixture(t *testing.T) *uplFixture {
	picker := mock_getblock.NewMockObjectGetter(t)
	fx := &uplFixture{
		ctrl:   gomock.NewController(t),
		picker: picker,
	}
	// TODO Use full-fledged file service (from files' package tests)
	fx.fileService = testMock.NewMockFileService(fx.ctrl)
	fx.fileObjectService = mock_fileobject.NewMockService(t)

	uploaderProvider := &service{
		fileService:       fx.fileService,
		tempDirProvider:   core.NewTempDirService(),
		picker:            picker,
		fileObjectService: fx.fileObjectService,
	}
	fx.Uploader = uploaderProvider.NewUploader("space1", objectorigin.None())
	fx.file = testMock.NewMockFile(fx.ctrl)
	fx.file.EXPECT().FileId().Return(domain.FileId("123")).AnyTimes()
	return fx
}

type uplFixture struct {
	Uploader
	file              *testMock.MockFile
	fileService       *testMock.MockFileService
	ctrl              *gomock.Controller
	picker            *mock_getblock.MockObjectGetter
	fileObjectService *mock_fileobject.MockService
}

func (fx *uplFixture) newImage(fileId domain.FileId) *testMock.MockImage {
	im := testMock.NewMockImage(fx.ctrl)
	im.EXPECT().FileId().Return(fileId).AnyTimes()
	return im
}

func (fx *uplFixture) newFile(fileId domain.FileId, meta *files.FileMeta) *testMock.MockFile {
	f := testMock.NewMockFile(fx.ctrl)
	f.EXPECT().FileId().Return(fileId).AnyTimes()
	f.EXPECT().Meta().Return(meta).AnyTimes()
	return f
}

func (fx *uplFixture) tearDown() {
	fx.ctrl.Finish()
}

func (fx *uplFixture) expectImageAdd() {
	// Lock mutex to reflect the behavior of the file service
	lock := &sync.Mutex{}
	lock.Lock()
	fx.fileService.EXPECT().ImageAdd(gomock.Any(), gomock.Any(), gomock.Any()).Return(&files.AddResult{
		FileId: "123",
		MIME:   "image/jpg",
		EncryptionKeys: &domain.FileEncryptionKeys{
			FileId:         "123",
			EncryptionKeys: map[string]string{},
		},
		Lock: lock,
	}, nil)
}

func (fx *uplFixture) expectFileAdd() {
	// Lock mutex to reflect the behavior of the file service
	lock := &sync.Mutex{}
	lock.Lock()
	fx.fileService.EXPECT().FileAdd(gomock.Any(), gomock.Any(), gomock.Any()).Return(&files.AddResult{
		FileId: "123",
		MIME:   "text/text",
		Size:   3,
		EncryptionKeys: &domain.FileEncryptionKeys{
			FileId:         "123",
			EncryptionKeys: map[string]string{},
		},
		Lock: lock,
	}, nil)
}

func (fx *uplFixture) expectCreateObject() string {
	fileObjectId := "fileObjectId1"
	fx.fileObjectService.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).Return(fileObjectId, &types.Struct{Fields: map[string]*types.Value{}}, nil)
	return fileObjectId
}
