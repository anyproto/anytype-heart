package service

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flakyReader fails the first `failuresBeforeSuccess` Read calls with the
// supplied error, then succeeds and delegates to delegate.
type flakyReader struct {
	delegate           io.ReadSeeker
	failureErr         error
	readsBeforeSuccess int
	seeksBeforeSuccess int
	readsObserved      int
	seeksObserved      int
}

func (f *flakyReader) Read(p []byte) (int, error) {
	f.readsObserved++
	if f.readsObserved <= f.readsBeforeSuccess {
		return 0, f.failureErr
	}
	return f.delegate.Read(p)
}

func (f *flakyReader) Seek(offset int64, whence int) (int64, error) {
	f.seeksObserved++
	if f.seeksObserved <= f.seeksBeforeSuccess {
		return 0, f.failureErr
	}
	return f.delegate.Seek(offset, whence)
}

type fixedReader struct {
	data []byte
	pos  int64
}

func (r *fixedReader) Read(p []byte) (int, error) {
	if r.pos >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += int64(n)
	return n, nil
}

func (r *fixedReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		r.pos = offset
	case io.SeekCurrent:
		r.pos += offset
	case io.SeekEnd:
		r.pos = int64(len(r.data)) + offset
	}
	return r.pos, nil
}

func TestRetryReadSeeker(t *testing.T) {
	fastRetry := []retry.Option{
		retry.Attempts(5),
		retry.Delay(time.Millisecond),
		retry.MaxDelay(time.Millisecond),
		retry.DelayType(retry.FixedDelay),
		retry.LastErrorOnly(true),
	}

	t.Run("retries transient read errors and eventually succeeds", func(t *testing.T) {
		flaky := &flakyReader{
			delegate:           &fixedReader{data: []byte("hello world")},
			failureErr:         errors.New("transient block-fetch error"),
			readsBeforeSuccess: 2,
		}
		rs := newRetryReadSeeker(flaky, fastRetry...)

		buf := make([]byte, 11)
		n, err := rs.Read(buf)

		require.NoError(t, err)
		assert.Equal(t, 11, n)
		assert.Equal(t, "hello world", string(buf))
		assert.Equal(t, 3, flaky.readsObserved, "should observe 2 failures + 1 success")
	})

	t.Run("does not retry on io.EOF", func(t *testing.T) {
		flaky := &flakyReader{
			delegate:           &fixedReader{data: []byte{}}, // immediately EOF
			failureErr:         errors.New("won't see this"),
			readsBeforeSuccess: 0,
		}
		rs := newRetryReadSeeker(flaky, fastRetry...)

		buf := make([]byte, 4)
		n, err := rs.Read(buf)

		assert.Equal(t, io.EOF, err)
		assert.Equal(t, 0, n)
		assert.Equal(t, 1, flaky.readsObserved, "EOF must not be retried")
	})

	t.Run("retries seek errors", func(t *testing.T) {
		flaky := &flakyReader{
			delegate:           &fixedReader{data: []byte("hello")},
			failureErr:         errors.New("transient seek error"),
			seeksBeforeSuccess: 1,
		}
		rs := newRetryReadSeeker(flaky, fastRetry...)

		off, err := rs.Seek(2, io.SeekStart)

		require.NoError(t, err)
		assert.Equal(t, int64(2), off)
		assert.Equal(t, 2, flaky.seeksObserved, "should observe 1 failure + 1 success")
	})

	t.Run("gives up after attempts exhausted", func(t *testing.T) {
		flaky := &flakyReader{
			delegate:           &fixedReader{data: []byte("hello")},
			failureErr:         errors.New("permanent failure"),
			readsBeforeSuccess: 100,
		}
		rs := newRetryReadSeeker(flaky, fastRetry...)

		buf := make([]byte, 4)
		_, err := rs.Read(buf)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "permanent failure")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // already cancelled

		flaky := &flakyReader{
			delegate:           &fixedReader{data: []byte("hello")},
			failureErr:         errors.New("transient"),
			readsBeforeSuccess: 100,
		}
		rs := newRetryReadSeeker(flaky, blockFetchRetryOptions(ctx)...)

		buf := make([]byte, 4)
		_, err := rs.Read(buf)
		require.Error(t, err)
	})
}
