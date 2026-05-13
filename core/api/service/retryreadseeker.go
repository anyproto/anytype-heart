package service

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/avast/retry-go/v4"
)

// blockFetchRetryOptions returns the retry options the API uses when reading
// file/image blocks from storage. Mirrors the gateway's options: infinite
// attempts bounded by ctx cancellation, exponential backoff between 200 ms
// and 2 s, only the last error is reported.
func blockFetchRetryOptions(ctx context.Context) []retry.Option {
	return []retry.Option{
		retry.Context(ctx),
		retry.Attempts(0),
		retry.Delay(200 * time.Millisecond),
		retry.MaxDelay(2 * time.Second),
		retry.DelayType(retry.BackOffDelay),
		retry.LastErrorOnly(true),
	}
}

// retryReadSeeker wraps an io.ReadSeeker so transient errors on Read/Seek
// (typically block-fetch failures while streaming an image variant) are
// retried with the supplied backoff options. EOF is never retried.
//
// Mirrors pkg/lib/gateway/gateway.go's retryReadSeeker so API and gateway
// behave the same on flaky underlying storage.
type retryReadSeeker struct {
	reader  io.ReadSeeker
	options []retry.Option
}

var _ io.ReadSeeker = (*retryReadSeeker)(nil)

func newRetryReadSeeker(reader io.ReadSeeker, options ...retry.Option) *retryReadSeeker {
	options = append(options, retry.RetryIf(func(err error) bool {
		return !errors.Is(err, io.EOF)
	}))
	return &retryReadSeeker{
		reader:  reader,
		options: options,
	}
}

func (r *retryReadSeeker) Read(p []byte) (int, error) {
	return retry.DoWithData(func() (int, error) {
		return r.reader.Read(p)
	}, r.options...)
}

func (r *retryReadSeeker) Seek(offset int64, whence int) (int64, error) {
	return retry.DoWithData(func() (int64, error) {
		return r.reader.Seek(offset, whence)
	}, r.options...)
}
