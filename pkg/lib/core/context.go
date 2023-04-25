package core

import (
	"context"
	"sync"
)

type ThreadLoadProgress struct {
	RecordsMissingLocally int
	RecordsLoaded         int
	RecordsLoadedSpent    float64

	RecordsFailedToLoad int
	lk                  sync.Mutex
}

type contextKey string

const ThreadLoadProgressContextKey contextKey = "threadload"
const ThreadLoadSkipMissingRecords contextKey = "threadSkipMissingRecords"

// DeriveContext returns a new context with value "progress" derived from
// the given one.
func (p *ThreadLoadProgress) DeriveContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, ThreadLoadProgressContextKey, p)
}

func (p *ThreadLoadProgress) IncrementLoadedRecords(spentSeconds float64) {
	p.lk.Lock()
	defer p.lk.Unlock()
	p.RecordsLoaded++
	p.RecordsLoadedSpent += spentSeconds

}

func (p *ThreadLoadProgress) IncrementFailedRecords() {
	p.lk.Lock()
	defer p.lk.Unlock()
	p.RecordsFailedToLoad++
}

func (p *ThreadLoadProgress) IncrementMissingRecord() {
	p.lk.Lock()
	defer p.lk.Unlock()
	p.RecordsMissingLocally++
}

// Value returns the current progress.
func (p *ThreadLoadProgress) Value() ThreadLoadProgress {
	p.lk.Lock()
	defer p.lk.Unlock()
	return ThreadLoadProgress{
		RecordsMissingLocally: p.RecordsMissingLocally,
		RecordsLoaded:         p.RecordsLoaded,
		RecordsLoadedSpent:    p.RecordsLoadedSpent,
		RecordsFailedToLoad:   p.RecordsFailedToLoad,
	}
}
