package gateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/files/mock_files"
)

func TestStallTimeout(t *testing.T) {
	t.Run("fires when nothing makes progress", func(t *testing.T) {
		// given
		fired := make(chan struct{})
		stall := newStallTimeout(20*time.Millisecond, func() { close(fired) })
		defer stall.stop()

		// then
		select {
		case <-fired:
		case <-time.After(time.Second):
			t.Fatal("expected the stall timeout to fire")
		}
	})

	// The point of the change: a large file on a slow link must stay alive as
	// long as bytes keep arriving, where a total deadline would cut it off.
	t.Run("progress keeps it armed well past the window", func(t *testing.T) {
		// given
		var fired atomic.Bool
		stall := newStallTimeout(50*time.Millisecond, func() { fired.Store(true) })
		defer stall.stop()

		// when: report progress for four windows' worth of time
		for range 20 {
			time.Sleep(10 * time.Millisecond)
			stall.progress()
		}

		// then
		assert.False(t, fired.Load(), "a transfer that keeps making progress must not be cut off")
	})

	t.Run("stop disarms it", func(t *testing.T) {
		// given
		var fired atomic.Bool
		stall := newStallTimeout(20*time.Millisecond, func() { fired.Store(true) })

		// when
		stall.stop()
		time.Sleep(100 * time.Millisecond)

		// then
		assert.False(t, fired.Load())
	})

	t.Run("progress after stop does not re-arm it", func(t *testing.T) {
		// given
		var fired atomic.Bool
		stall := newStallTimeout(20*time.Millisecond, func() { fired.Store(true) })

		// when
		stall.stop()
		stall.progress()
		time.Sleep(100 * time.Millisecond)

		// then
		assert.False(t, fired.Load())
	})
}

func TestProgressReader(t *testing.T) {
	t.Run("a read that moved bytes reports progress", func(t *testing.T) {
		// given
		var reported int
		r := &progressReader{ReadSeeker: strings.NewReader("payload"), onProgress: func() { reported++ }}

		// when
		got, err := io.ReadAll(r)

		// then
		require.NoError(t, err)
		assert.Equal(t, "payload", string(got))
		assert.Positive(t, reported)
	})

	t.Run("a read that moved nothing reports no progress", func(t *testing.T) {
		// given
		var reported int
		r := &progressReader{ReadSeeker: errReadSeeker{}, onProgress: func() { reported++ }}

		// when
		_, err := r.Read(make([]byte, 8))

		// then
		require.Error(t, err)
		assert.Zero(t, reported)
	})

	t.Run("a seek reports progress", func(t *testing.T) {
		// given
		var reported int
		r := &progressReader{ReadSeeker: strings.NewReader("payload"), onProgress: func() { reported++ }}

		// when
		_, err := r.Seek(0, io.SeekEnd)

		// then
		require.NoError(t, err)
		assert.Equal(t, 1, reported)
	})
}

type errReadSeeker struct{}

func (errReadSeeker) Read([]byte) (int, error)       { return 0, errors.New("read failed") }
func (errReadSeeker) Seek(int64, int) (int64, error) { return 0, errors.New("seek failed") }

// The bug this replaces: a total request deadline killed any transfer that ran
// longer than it, however healthy. A slow link must survive as long as bytes
// keep moving.
func TestFileHandlerSurvivesASlowButProgressingTransfer(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.fileStallTimeout = 100 * time.Millisecond

	const payload = "0123456789abcdefghij"
	body := &trickleReadSeeker{
		ReadSeeker: strings.NewReader(payload),
		chunk:      1,
		delay:      40 * time.Millisecond,
	}

	file := mock_files.NewMockFile(t)
	file.EXPECT().Reader(mock.Anything).RunAndReturn(func(ctx context.Context) (io.ReadSeeker, error) {
		body.ctx = ctx
		return body, nil
	})
	file.EXPECT().Meta().Return(&files.FileMeta{Media: "application/octet-stream", Name: "big.bin"})
	fx.fileObjectService.EXPECT().GetFileData(mock.Anything, mock.Anything).Return(file, nil)

	// when: the transfer takes ~800ms, eight stall windows
	resp, err := http.Get("http://" + fx.Addr() + "/file/fileObjectId")
	require.NoError(t, err)
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)

	// then
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, payload, string(got))
}

func TestFileHandlerCutsOffAStalledTransfer(t *testing.T) {
	// given
	fx := newFixture(t)
	fx.fileStallTimeout = 100 * time.Millisecond

	body := &wedgedReadSeeker{ReadSeeker: strings.NewReader("0123456789")}

	file := mock_files.NewMockFile(t)
	file.EXPECT().Reader(mock.Anything).RunAndReturn(func(ctx context.Context) (io.ReadSeeker, error) {
		body.ctx = ctx
		return body, nil
	})
	file.EXPECT().Meta().Return(&files.FileMeta{Media: "application/octet-stream", Name: "big.bin"})
	fx.fileObjectService.EXPECT().GetFileData(mock.Anything, mock.Anything).Return(file, nil)

	// when
	done := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + fx.Addr() + "/file/fileObjectId")
		if err != nil {
			done <- err
			return
		}
		defer resp.Body.Close()
		_, readErr := io.ReadAll(resp.Body)
		done <- readErr
	}()

	// then: the request does not hang on a reader that never delivers
	select {
	case err := <-done:
		require.Error(t, err, "expected the stalled transfer to be cut off")
	case <-time.After(10 * time.Second):
		t.Fatal("expected the stalled transfer to be cut off, it hung instead")
	}
}

// trickleReadSeeker delivers a few bytes at a time with a pause in between —
// a healthy transfer over a slow link. Like the real DAG reader it honors the
// request context, which is what a cancelled request actually stops.
type trickleReadSeeker struct {
	io.ReadSeeker
	chunk int
	delay time.Duration
	ctx   context.Context
}

func (r *trickleReadSeeker) Read(p []byte) (int, error) {
	time.Sleep(r.delay)
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) > r.chunk {
		p = p[:r.chunk]
	}
	return r.ReadSeeker.Read(p)
}

// wedgedReadSeeker can be sized but never delivers a byte, like a transfer
// whose source has gone silent.
type wedgedReadSeeker struct {
	io.ReadSeeker
	ctx context.Context
}

func (r *wedgedReadSeeker) Read([]byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}
