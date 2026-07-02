package client

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RetryPolicy bounds every API call: attempts, per-attempt backoff and a
// total wall-clock budget (v1 retried block fetches forever).
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	TotalBudget time.Duration
}

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 5,
		BaseDelay:   time.Second,
		MaxDelay:    time.Minute,
		TotalBudget: 5 * time.Minute,
	}
}

// backoff returns the exponential delay before the given attempt (attempt
// numbering starts at 1 for the first retry). A Retry-After header replaces
// this value for its attempt without compounding into later ones (v1 doubled
// the server-provided value on the next failure).
func (p RetryPolicy) backoff(attempt int) time.Duration {
	delay := p.BaseDelay << (attempt - 1)
	if delay > p.MaxDelay || delay <= 0 {
		return p.MaxDelay
	}
	return delay
}

// pacer combines the steady token bucket with shared 429/529 pushback: when
// any worker is told to slow down, every worker waits.
type pacer struct {
	limiter *rate.Limiter

	mu          sync.Mutex
	pausedUntil time.Time
}

func newPacer(limit rate.Limit) *pacer {
	return &pacer{limiter: rate.NewLimiter(limit, int(limit))}
}

func (p *pacer) Wait(ctx context.Context) error {
	p.mu.Lock()
	pause := time.Until(p.pausedUntil)
	p.mu.Unlock()
	if pause > 0 {
		if err := sleepCtx(ctx, pause); err != nil {
			return err
		}
	}
	return p.limiter.Wait(ctx)
}

func (p *pacer) pushback(d time.Duration) {
	until := time.Now().Add(d)
	p.mu.Lock()
	if until.After(p.pausedUntil) {
		p.pausedUntil = until
	}
	p.mu.Unlock()
}
