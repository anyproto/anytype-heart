package file

import (
	"image"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploader_Do(t *testing.T) {
	var (
		fileHash = "12345"
		fileMeta = &core.FileMeta{
			Media: "text/plain; charset=utf-8",
			Name:  "test.txt",
			Size:  10,
			Added: time.Now(),
		}
	)

	testFilepath, _ := filepath.Abs("./testdata/test.txt")
	t.Run("success local file", func(t *testing.T) {
		f := NewFile(&model.Block{
			Id:      "test",
			Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}},
		}).(*File)
		fx := newFixture(t, f)
		defer fx.ctrl.Finish()

		file := testMock.NewMockFile(fx.ctrl)
		file.EXPECT().Hash().Return(fileHash).AnyTimes()
		file.EXPECT().Meta().Return(fileMeta).AnyTimes()

		fx.anytype.EXPECT().FileAdd(gomock.Any(), gomock.Any()).Return(file, nil)

		fx.mu.Lock()
		err := f.Upload(fx.anytype, fx, testFilepath, "", false)
		fx.mu.Unlock()
		require.NoError(t, err)

		select {
		case <-time.After(time.Second * 2):
			t.Error("upload timeout")
			return
		case <-fx.done:
		}
	})

	t.Run("success url", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("12345"))
		}))
		defer ts.Close()

		f := NewFile(&model.Block{
			Id:      "test",
			Content: &model.BlockContentOfFile{File: &model.BlockContentFile{}},
		}).(*File)
		fx := newFixture(t, f)
		defer fx.ctrl.Finish()

		file := testMock.NewMockFile(fx.ctrl)
		file.EXPECT().Hash().Return(fileHash).AnyTimes()
		file.EXPECT().Meta().Return(fileMeta).AnyTimes()

		fx.anytype.EXPECT().FileAdd(gomock.Any(), gomock.Any()).Return(file, nil)

		fx.mu.Lock()
		err := f.Upload(fx.anytype, fx, "", ts.URL+"/http.txt", false)
		fx.mu.Unlock()
		require.NoError(t, err)

		select {
		case <-time.After(time.Second * 2):
			t.Error("upload timeout")
			return
		case <-fx.done:
		}
	})

	t.Run("image file fallback", func(t *testing.T) {
		f := NewFile(&model.Block{
			Id:      "test",
			Content: &model.BlockContentOfFile{File: &model.BlockContentFile{Type: model.BlockContentFile_Image}},
		}).(*File)
		fx := newFixture(t, f)
		defer fx.ctrl.Finish()

		fx.anytype.EXPECT().ImageAdd(gomock.Any(), gomock.Any()).Return(nil, image.ErrFormat)

		file := testMock.NewMockFile(fx.ctrl)
		file.EXPECT().Hash().Return(fileHash).AnyTimes()
		file.EXPECT().Meta().Return(fileMeta).AnyTimes()

		fx.anytype.EXPECT().FileAdd(gomock.Any(), gomock.Any()).Return(file, nil)

		fx.mu.Lock()
		err := f.Upload(fx.anytype, fx, testFilepath, "", false)
		fx.mu.Unlock()
		require.NoError(t, err)

		select {
		case <-time.After(time.Second * 2):
			t.Error("upload timeout")
			return
		case <-fx.done:
		}
	})
	t.Run("image success", func(t *testing.T) {
		f := NewFile(&model.Block{
			Id:      "test",
			Content: &model.BlockContentOfFile{File: &model.BlockContentFile{Type: model.BlockContentFile_Image}},
		}).(*File)
		fx := newFixture(t, f)
		defer fx.ctrl.Finish()

		file := testMock.NewMockImage(fx.ctrl)
		file.EXPECT().Hash().Return(fileHash).AnyTimes()

		fx.anytype.EXPECT().ImageAdd(gomock.Any(), gomock.Any()).Return(file, nil)

		fx.mu.Lock()
		err := f.Upload(fx.anytype, fx, testFilepath, "", false)
		fx.mu.Unlock()
		require.NoError(t, err)

		select {
		case <-time.After(time.Second * 2):
			t.Error("upload timeout")
			return
		case <-fx.done:
		}
	})
}

func newFixture(t *testing.T, file *File) *fixture {
	ctrl := gomock.NewController(t)
	return &fixture{
		ctrl:    ctrl,
		anytype: testMock.NewMockService(ctrl),
		file:    file,
		t:       t,
		done:    make(chan struct{}),
	}
}

type fixture struct {
	ctrl    *gomock.Controller
	anytype *testMock.MockService
	file    *File
	t       *testing.T
	done    chan struct{}
	mu      sync.Mutex
}

func (f *fixture) UpdateFileBlock(id string, apply func(f Block)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(f.t, f.file.Id, id)
	apply(f.file)
	if f.file.content.State == model.BlockContentFile_Done || f.file.content.State == model.BlockContentFile_Error {
		close(f.done)
	}
	return nil
}
