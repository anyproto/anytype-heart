package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// httpFetcher downloads pre-signed file URLs with the same retry/timeout
// discipline as API calls (v1 used a bare http.DefaultClient with neither).
// Reads are ctx-interruptible: a half-open connection dies with the run.
type httpFetcher struct {
	transport Transport
	retry     RetryPolicy
	// stallTimeout bounds the gap between read progress, not the total
	// download (large files are legitimate).
	stallTimeout time.Duration
}

type FetcherOption func(*httpFetcher)

func WithFetcherTransport(transport Transport) FetcherOption {
	return func(f *httpFetcher) { f.transport = transport }
}

func WithFetcherRetryPolicy(policy RetryPolicy) FetcherOption {
	return func(f *httpFetcher) { f.retry = policy }
}

func WithFetcherStallTimeout(d time.Duration) FetcherOption {
	return func(f *httpFetcher) { f.stallTimeout = d }
}

func NewFileFetcher(opts ...FetcherOption) FileFetcher {
	// The stall guard only arms once headers arrive; a host that accepts
	// the connection but never answers must not hang the run, so the header
	// phase gets its own bound. No total cap — large files are legitimate.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	f := &httpFetcher{
		transport:    &http.Client{Transport: transport},
		retry:        DefaultRetryPolicy(),
		stallTimeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

func (f *httpFetcher) Fetch(ctx context.Context, url string, dst io.Writer) error {
	deadline := time.Now().Add(f.retry.TotalBudget)
	var lastErr error
	for attempt := 0; attempt < f.retry.MaxAttempts; attempt++ {
		if attempt > 0 {
			delay := f.retry.backoff(attempt)
			if retryAfter := retryAfterOf(lastErr); retryAfter > 0 {
				delay = retryAfter
			}
			if time.Now().Add(delay).After(deadline) {
				break
			}
			if err := sleepCtx(ctx, delay); err != nil {
				return err
			}
		}
		lastErr = f.fetchOnce(ctx, url, dst)
		if lastErr == nil {
			return nil
		}
		if !isRetryable(lastErr) || ctx.Err() != nil {
			return lastErr
		}
		// A partially written dst cannot be retried into blindly; the
		// caller passes a fresh writer per attempt via Resettable, or we
		// refuse to retry after partial progress.
		if resettable, ok := dst.(Resettable); ok {
			if err := resettable.Reset(); err != nil {
				return fmt.Errorf("reset download target: %w", err)
			}
		} else if written, ok := lastErr.(*partialWriteError); ok && written.bytes > 0 {
			return lastErr
		}
	}
	return fmt.Errorf("download retries exhausted: %w", lastErr)
}

// Resettable lets a download target rewind between retry attempts.
type Resettable interface {
	Reset() error
}

type partialWriteError struct {
	bytes int64
	err   error
}

func (e *partialWriteError) Error() string { return e.err.Error() }
func (e *partialWriteError) Unwrap() error { return e.err }

func (f *httpFetcher) fetchOnce(ctx context.Context, url string, dst io.Writer) error {
	fetchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build download request: %w", err)
	}
	res, err := f.transport.Do(req)
	if err != nil {
		return &transportError{err: err}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errorFromResponse(res)
	}

	// The stall guard cancels the request context when no bytes arrive for
	// stallTimeout — unlike v1's monitor, this interrupts a hung read.
	progress := make(chan struct{}, 1)
	guardDone := make(chan struct{})
	go func() {
		defer close(guardDone)
		timer := time.NewTimer(f.stallTimeout)
		defer timer.Stop()
		for {
			select {
			case <-fetchCtx.Done():
				return
			case <-progress:
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(f.stallTimeout)
			case <-timer.C:
				cancel()
				return
			}
		}
	}()

	written, err := io.Copy(dst, progressReader{r: res.Body, progress: progress})
	cancel()
	<-guardDone
	if err != nil {
		return &partialWriteError{bytes: written, err: fmt.Errorf("download body: %w", &transportError{err: err})}
	}
	return nil
}

type progressReader struct {
	r        io.Reader
	progress chan struct{}
}

func (p progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		select {
		case p.progress <- struct{}{}:
		default:
		}
	}
	return n, err
}
