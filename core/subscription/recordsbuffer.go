package subscription

import (
	"context"
	"errors"
	"sync"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

var errRecordsBufferClosed = errors.New("records buffer closed")

// recordsBuffer coalesces store update records by object id: a pending older
// version of an object is replaced by the newer one, so the buffer holds at
// most one record per distinct object no matter how fast updates arrive. This
// bounds the memory of the change pipeline during mass indexing, when the
// consumer (recordsHandler) drains it once per batch interval
type recordsBuffer struct {
	mu     sync.Mutex
	byId   map[string]int
	batch  []database.Record
	signal chan struct{}
	closed bool
}

func newRecordsBuffer() *recordsBuffer {
	return &recordsBuffer{
		byId:   make(map[string]int),
		signal: make(chan struct{}, 1),
	}
}

// Add queues the record, replacing a pending older version of the same object.
// It never blocks
func (b *recordsBuffer) Add(rec database.Record) error {
	id := rec.Details.GetString(bundle.RelationKeyId)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return errRecordsBufferClosed
	}
	if i, ok := b.byId[id]; ok {
		b.batch[i] = rec
	} else {
		b.batch = append(b.batch, rec)
		b.byId[id] = len(b.batch) - 1
	}
	b.mu.Unlock()
	select {
	case b.signal <- struct{}{}:
	default:
	}
	return nil
}

// Wait blocks until at least one record is queued or the context is canceled,
// then returns the whole pending batch. The returned slice is owned by the
// caller
func (b *recordsBuffer) Wait(ctx context.Context) ([]database.Record, error) {
	for {
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return nil, errRecordsBufferClosed
		}
		if len(b.batch) > 0 {
			batch := b.batch
			b.batch = make([]database.Record, 0, len(batch))
			clear(b.byId)
			b.mu.Unlock()
			return batch, nil
		}
		b.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-b.signal:
		}
	}
}

func (b *recordsBuffer) Close() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	select {
	case b.signal <- struct{}{}:
	default:
	}
}
