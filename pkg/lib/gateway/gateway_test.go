package gateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/avast/retry-go/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/filedownloader/mock_filedownloader"
	"github.com/anyproto/anytype-heart/core/files/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

func TestCleanUpPathForLogging(t *testing.T) {
	t.Run("with CID in path", func(t *testing.T) {
		path := "/image/bafybeihjujzgyuzjmwc4ar7xpkobgvrxv6jmsyfhxp4mypiexlyrs2y2zu"
		got := cleanUpPathForLogging(path)
		assert.Equal(t, path, got)

		path = "/file/bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi"
		got = cleanUpPathForLogging(path)
		assert.Equal(t, path, got)
	})

	t.Run("with something else in path", func(t *testing.T) {
		path := "/file/https:/example.com/foo/bar"
		got := cleanUpPathForLogging(path)
		want := "/file/<masked invalid path>"
		assert.Equal(t, want, got)
	})
}

func TestGetImage(t *testing.T) {
	t.Run("file object id is provided", func(t *testing.T) {
		fx := newFixture(t)

		const imageData = "image data"
		const fileObjectId = "fileObjectId"

		file := mock_files.NewMockFile(t)
		file.EXPECT().Reader(mock.Anything).Return(strings.NewReader(imageData), nil)
		file.EXPECT().Meta().Return(&files.FileMeta{
			Media: "image/jpeg",
			Name:  "test image",
		})
		file.EXPECT().Name().Return("test image")

		image := mock_files.NewMockImage(t)
		image.EXPECT().GetOriginalFile().Return(file, nil)
		image.EXPECT().SpaceId().Return("space1")
		file.EXPECT().MimeType().Return("image/jpeg")

		fx.fileObjectService.EXPECT().GetImageData(mock.Anything, mock.Anything).Return(image, nil)

		path := "http://" + fx.Addr() + "/image/" + fileObjectId

		resp, err := http.Get(path)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "image/jpeg", resp.Header.Get("Content-Type"))
		assert.Equal(t, "inline; filename=\"test image\"", resp.Header.Get("Content-Disposition"))

		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, imageData, string(data))
	})
}

type fixture struct {
	*gateway
	fileService       *mock_files.MockService
	fileObjectService *mock_fileobject.MockService
}

func newFixture(t *testing.T) *fixture {
	a := new(app.App)

	fileService := mock_files.NewMockService(t)
	fileObjectService := mock_fileobject.NewMockService(t)
	fileDownloader := mock_filedownloader.NewMockService(t)
	gw := New().(*gateway)

	ctx := context.Background()
	a.Register(testutil.PrepareMock(ctx, a, fileService))
	a.Register(testutil.PrepareMock(ctx, a, fileObjectService))
	a.Register(testutil.PrepareMock(ctx, a, fileDownloader))
	a.Register(gw)
	err := a.Start(ctx)
	assert.NoError(t, err)

	t.Cleanup(func() {
		err := gw.Close(ctx)
		assert.NoError(t, err)
	})

	return &fixture{
		gateway:           gw,
		fileService:       fileService,
		fileObjectService: fileObjectService,
	}
}

type testReader struct {
	readCalled int
	seekCalled int

	reader io.ReadSeeker
}

func (r *testReader) Read(p []byte) (n int, err error) {
	r.readCalled++
	if r.readCalled <= 1 {
		return 0, fmt.Errorf("test error")
	}
	return r.reader.Read(p)
}

func (r *testReader) Seek(offset int64, whence int) (int64, error) {
	r.seekCalled++
	if r.seekCalled <= 1 {
		return 0, fmt.Errorf("test error")
	}
	return r.reader.Seek(offset, whence)
}

func TestRetryReader(t *testing.T) {
	wantData := "test data"

	reader := &testReader{reader: strings.NewReader(wantData)}

	retryOptions := []retry.Option{
		retry.Attempts(0),
		retry.Delay(10 * time.Millisecond),
		retry.MaxDelay(2 * time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	}

	retryReader := newRetryReadSeeker(reader, retryOptions...)

	data, err := io.ReadAll(retryReader)
	require.NoError(t, err)

	assert.Equal(t, wantData, string(data))

	n, err := retryReader.Seek(5, io.SeekStart)
	require.NoError(t, err)
	assert.Equal(t, int64(5), n)

	data, err = io.ReadAll(retryReader)
	require.NoError(t, err)

	assert.Equal(t, "data", string(data))

	assert.True(t, reader.readCalled > 1)
	assert.True(t, reader.seekCalled > 1)
}
