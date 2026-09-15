package gateway

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
)

// The mux routes every method to the handler and ServeContent only skips the
// body for HEAD, so without a guard a CORS preflight on a multi-gigabyte object
// streams a multi-gigabyte body — and holds one of the 32 shared slots to do it.
func TestFileHandlerMethods(t *testing.T) {
	// No GetFileData expectation: reaching the file service at all fails these.
	t.Run("a preflight is answered without touching the file", func(t *testing.T) {
		fx := newFixture(t)

		req, err := http.NewRequest(http.MethodOptions, "http://"+fx.Addr()+"/file/fileObjectId", nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
		assert.Empty(t, body)
		assert.NotEmpty(t, resp.Header.Get("Access-Control-Allow-Headers"),
			"a preflight that does not allow Range is useless to a download")
	})

	t.Run("a write method is refused without touching the file", func(t *testing.T) {
		fx := newFixture(t)

		resp, err := http.Post("http://"+fx.Addr()+"/file/fileObjectId", "text/plain", strings.NewReader("x"))
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Allow"), "GET")
	})
}

// The client reads these from a cross-origin fetch. Without exposing them the
// filename this package encodes, and the headers a resume needs, are invisible.
func TestFileHandlerExposesDownloadHeaders(t *testing.T) {
	fx := newFixture(t)

	file := mock_files.NewMockFile(t)
	file.EXPECT().Reader(mock.Anything).Return(strings.NewReader("payload"), nil)
	file.EXPECT().Meta().Return(&files.FileMeta{Media: "application/octet-stream", Name: "report.pdf"})
	fx.fileObjectService.EXPECT().GetFileData(mock.Anything, mock.Anything).Return(file, nil)

	resp, err := http.Get("http://" + fx.Addr() + "/file/fileObjectId")
	require.NoError(t, err)
	defer resp.Body.Close()

	exposed := resp.Header.Get("Access-Control-Expose-Headers")
	for _, header := range []string{"Content-Disposition", "Content-Length", "Accept-Ranges", "Content-Range"} {
		assert.Contains(t, exposed, header)
	}
}

// A seek into a large file fetches the previous block to recover the IV, which
// is real network I/O. If the stall window expires during it, ServeContent maps
// the seek error to 416 — telling a resuming client its partial file is invalid
// and inviting it to throw away gigabytes over what is a transient stall.
func TestFileHandlerStalledSeekIsNotRangeNotSatisfiable(t *testing.T) {
	fx := newFixture(t)
	fx.fileStallTimeout = 100 * time.Millisecond

	body := &stalledSeeker{ReadSeeker: strings.NewReader("0123456789")}

	file := mock_files.NewMockFile(t)
	file.EXPECT().Reader(mock.Anything).RunAndReturn(func(ctx context.Context) (io.ReadSeeker, error) {
		body.ctx = ctx
		return body, nil
	})
	file.EXPECT().Meta().Return(&files.FileMeta{Media: "application/octet-stream", Name: "big.bin"})
	fx.fileObjectService.EXPECT().GetFileData(mock.Anything, mock.Anything).Return(file, nil)

	req, err := http.NewRequest(http.MethodGet, "http://"+fx.Addr()+"/file/fileObjectId", nil)
	require.NoError(t, err)
	req.Header.Set("Range", "bytes=5-")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.NotEqual(t, http.StatusRequestedRangeNotSatisfiable, resp.StatusCode,
		"a transient stall must not be reported as a permanent range error")
	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)
}

// stalledSeeker can be sized, but a seek to a real offset never completes —
// like a block fetch whose source has gone silent.
type stalledSeeker struct {
	io.ReadSeeker
	ctx context.Context
}

func (r *stalledSeeker) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekStart && offset > 0 {
		<-r.ctx.Done()
		return 0, r.ctx.Err()
	}
	return r.ReadSeeker.Seek(offset, whence)
}
