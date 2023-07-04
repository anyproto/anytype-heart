//go:generate mockgen -package file_test -destination block_service_mock_test.go github.com/anyproto/anytype-heart/core/block/editor/file BlockService

package file_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/file"
	"github.com/anyproto/anytype-heart/core/block/getblock/mock_getblock"
	"github.com/anyproto/anytype-heart/core/block/simple"
	file2 "github.com/anyproto/anytype-heart/core/block/simple/file"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/testMock"
)

func TestUploader_Upload(t *testing.T) {
	ctx := session.NewContext(context.Background(), "space1")
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
		im := fx.newImage("123")
		fx.fileService.EXPECT().ImageAdd(gomock.Any(), gomock.Any()).Return(im, nil)
		im.EXPECT().GetOriginalFile(gomock.Any()).Return(fx.file, nil)
		b := newBlock(model.BlockContentFile_Image)
		fx.picker.EXPECT().PickBlock(mock.Anything, mock.Anything).Return(nil, nil)
		fx.file.EXPECT().Meta().Return(&files.FileMeta{Media: "image/jpg"}).AnyTimes()
		res := fx.Uploader.SetBlock(b).SetFile("./testdata/unnamed.jpg").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.Hash, "123")
		assert.Equal(t, res.Name, "unnamed.jpg")
		assert.Equal(t, b.Model().GetFile().Name, "unnamed.jpg")
		assert.Equal(t, res.MIME, "image/jpg")
	})
	t.Run("image type detect", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()
		im := fx.newImage("123")
		fx.picker.EXPECT().PickBlock(mock.Anything, mock.Anything).Return(nil, nil)
		fx.fileService.EXPECT().ImageAdd(gomock.Any(), gomock.Any()).Return(im, nil)
		im.EXPECT().GetOriginalFile(gomock.Any())
		res := fx.Uploader.AutoType(true).SetFile("./testdata/unnamed.jpg").Upload(ctx)
		require.NoError(t, res.Err)
	})
	t.Run("image to file failover", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()
		meta := &files.FileMeta{
			Media: "text/text",
			Name:  "test.txt",
			Size:  3,
			Added: time.Now(),
		}
		// fx.anytype.EXPECT().ImageAdd(gomock.Any(), gomock.Any()).Return(nil, image.ErrFormat)
		fx.picker.EXPECT().PickBlock(mock.Anything, mock.Anything).Return(nil, nil)
		fx.fileService.EXPECT().FileAdd(gomock.Any(), gomock.Any()).Return(fx.newFile("123", meta), nil)
		b := newBlock(model.BlockContentFile_Image)
		res := fx.Uploader.SetBlock(b).SetFile("./testdata/test.txt").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.Hash, "123")
		assert.Equal(t, res.Name, "test.txt")
		assert.Equal(t, b.Model().GetFile().Name, "test.txt")
		assert.Equal(t, b.Model().GetFile().Type, model.BlockContentFile_File)
	})
	t.Run("file from url", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, "./testdata/unnamed.jpg")
		})
		serv := httptest.NewServer(mux)
		defer serv.Close()

		fx := newFixture(t)
		defer fx.tearDown()
		im := fx.newImage("123")
		fx.picker.EXPECT().PickBlock(mock.Anything, mock.Anything).Return(nil, nil)
		fx.fileService.EXPECT().ImageAdd(gomock.Any(), gomock.Any()).Return(im, nil)
		im.EXPECT().GetOriginalFile(gomock.Any())
		res := fx.Uploader.AutoType(true).SetUrl(serv.URL + "/unnamed.jpg").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.Hash, "123")
		assert.Equal(t, res.Name, "unnamed.jpg")
		res.Size = 1
		b := res.ToBlock()
		assert.Equal(t, b.Model().GetFile().Name, "unnamed.jpg")
	})
	t.Run("file from Content-Disposition", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Disposition", "form-data; name=\"fieldName\"; filename=\"filename\"")
			http.ServeFile(w, r, "./testdata/unnamed.jpg")
		})
		serv := httptest.NewServer(mux)
		defer serv.Close()

		fx := newFixture(t)
		defer fx.tearDown()
		im := fx.newImage("123")
		fx.picker.EXPECT().PickBlock(mock.Anything, mock.Anything).Return(nil, nil)
		fx.fileService.EXPECT().ImageAdd(gomock.Any(), gomock.Any()).Return(im, nil)
		im.EXPECT().GetOriginalFile(gomock.Any())
		res := fx.Uploader.AutoType(true).SetUrl(serv.URL + "/unnamed.jpg").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.Hash, "123")
		assert.Equal(t, res.Name, "filename")
		res.Size = 1
		b := res.ToBlock()
		assert.Equal(t, b.Model().GetFile().Name, "filename")
	})
	t.Run("file without url params", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, "./testdata/unnamed.jpg")
		})
		serv := httptest.NewServer(mux)
		defer serv.Close()

		fx := newFixture(t)
		defer fx.tearDown()
		im := fx.newImage("123")
		fx.picker.EXPECT().PickBlock(mock.Anything, mock.Anything).Return(nil, nil)
		fx.fileService.EXPECT().ImageAdd(gomock.Any(), gomock.Any()).Return(im, nil)
		im.EXPECT().GetOriginalFile(gomock.Any())
		res := fx.Uploader.AutoType(true).SetUrl(serv.URL + "/unnamed.jpg?text=text").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.Hash, "123")
		assert.Equal(t, res.Name, "unnamed.jpg")
		res.Size = 1
		b := res.ToBlock()
		assert.Equal(t, b.Model().GetFile().Name, "unnamed.jpg")
	})
	t.Run("bytes", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()
		fx.picker.EXPECT().PickBlock(mock.Anything, mock.Anything).Return(nil, nil)
		fx.fileService.EXPECT().FileAdd(gomock.Any(), gomock.Any()).Return(fx.newFile("123", &files.FileMeta{}), nil)
		res := fx.Uploader.SetBytes([]byte("my bytes")).SetName("filename").Upload(ctx)
		require.NoError(t, res.Err)
		assert.Equal(t, res.Hash, "123")
		assert.Equal(t, res.Name, "filename")
	})
}

func newFixture(t *testing.T) *uplFixture {
	picker := mock_getblock.NewMockPicker(t)
	fx := &uplFixture{
		ctrl:   gomock.NewController(t),
		picker: picker,
	}
	fx.fileService = testMock.NewMockFileService(fx.ctrl)
	fx.blockService = NewMockBlockService(fx.ctrl)

	fx.Uploader = file.NewUploader(fx.blockService, fx.fileService, core.NewTempDirService(nil), picker)
	fx.file = testMock.NewMockFile(fx.ctrl)
	fx.file.EXPECT().Hash().Return("123").AnyTimes()
	return fx
}

type uplFixture struct {
	file.Uploader
	blockService *MockBlockService
	file         *testMock.MockFile
	fileService  *testMock.MockFileService
	ctrl         *gomock.Controller
	picker       *mock_getblock.MockPicker
}

func (fx *uplFixture) newImage(hash string) *testMock.MockImage {
	im := testMock.NewMockImage(fx.ctrl)
	im.EXPECT().Hash().Return(hash).AnyTimes()
	return im
}

func (fx *uplFixture) newFile(hash string, meta *files.FileMeta) *testMock.MockFile {
	f := testMock.NewMockFile(fx.ctrl)
	f.EXPECT().Hash().Return(hash).AnyTimes()
	f.EXPECT().Meta().Return(meta).AnyTimes()
	return f
}

func (fx *uplFixture) tearDown() {
	fx.ctrl.Finish()
}
