package identity

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

type fakeClaimLedger struct {
	batches [][]ClaimLedgerRecord
}

func (l *fakeClaimLedger) RecordClaims(ctx context.Context, claims []ClaimLedgerRecord) error {
	l.batches = append(l.batches, claims)
	return nil
}

type failingOnceLedger struct {
	fakeClaimLedger
	failures int
}

func (l *failingOnceLedger) RecordClaims(ctx context.Context, claims []ClaimLedgerRecord) error {
	if l.failures > 0 {
		l.failures--
		return assert.AnError
	}
	return l.fakeClaimLedger.RecordClaims(ctx, claims)
}

func TestFlushKeepsBatchOnFailure(t *testing.T) {
	t.Run("a failed flush retries the same batch instead of dropping it", func(t *testing.T) {
		// given — E3: FlushClaims nil'd the buffer before the ledger call,
		// so a transient failure silently dropped every buffered intent.
		store := objectstore.NewStoreFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		ledger := &failingOnceLedger{failures: 1}
		service := NewService(space, store.SpaceIndex(spaceId), false, time.Unix(1700000000, 0), WithClaimLedger(ledger))
		space.EXPECT().CreateTreePayload(mock.Anything, mock.Anything).Return(treestorage.TreeStorageCreatePayload{
			RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: "tree-1", RawChange: []byte("r")},
		}, nil).Once()
		require.NoError(t, service.Claim(context.Background(), importv2.IdentityClaim{
			SourceKey: "page-1", SbType: coresb.SmartBlockTypePage,
		}))

		// when
		require.Error(t, service.FlushClaims(context.Background()))
		require.NoError(t, service.FlushClaims(context.Background()))

		// then
		require.Len(t, ledger.batches, 1)
		assert.Equal(t, "page-1", ledger.batches[0][0].SourceKey)
	})
}

type blockingLedger struct {
	mu      sync.Mutex
	batches [][]ClaimLedgerRecord
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (l *blockingLedger) RecordClaims(ctx context.Context, claims []ClaimLedgerRecord) error {
	if l.calls.Add(1) == 1 {
		close(l.entered)
		<-l.release
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.batches = append(l.batches, claims)
	return nil
}

func TestFlushOverlap(t *testing.T) {
	t.Run("overlapping flushes never panic or lose records", func(t *testing.T) {
		// given — CONFIRMED 'slice bounds out of range [4:0]': the lock
		// protected the field but not the take/write/trim transaction.
		store := objectstore.NewStoreFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		ledger := &blockingLedger{entered: make(chan struct{}), release: make(chan struct{})}
		service := NewService(space, store.SpaceIndex(spaceId), false, time.Unix(1700000000, 0), WithClaimLedger(ledger))
		counter := 0
		space.EXPECT().CreateTreePayload(mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, _ payloadcreator.PayloadCreationParams) (treestorage.TreeStorageCreatePayload, error) {
				counter++
				return treestorage.TreeStorageCreatePayload{
					RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: fmt.Sprintf("t-%d", counter), RawChange: []byte("r")},
				}, nil
			})
		for i := 0; i < 4; i++ {
			require.NoError(t, service.Claim(context.Background(), importv2.IdentityClaim{
				SourceKey: fmt.Sprintf("p-%d", i), SbType: coresb.SmartBlockTypePage,
			}))
		}

		// when: flush A blocks inside the ledger; flush B arrives while A
		// is parked. Under the broken take/unlock/write/trim shape, B
		// completed a duplicate delivery and A then panicked on the trim
		// (reproduced: slice bounds [4:0]); the fix serializes the whole
		// transaction, so B waits and then finds nothing to do.
		done := make(chan error, 2)
		go func() { done <- service.FlushClaims(context.Background()) }()
		<-ledger.entered
		go func() { done <- service.FlushClaims(context.Background()) }()
		close(ledger.release)
		require.NoError(t, <-done)
		require.NoError(t, <-done)

		// then: every claim delivered exactly once across the batches
		ledger.mu.Lock()
		total := 0
		for _, batch := range ledger.batches {
			total += len(batch)
		}
		ledger.mu.Unlock()
		assert.Equal(t, 4, total)
	})
}

func TestClaimLedger(t *testing.T) {
	t.Run("claims buffer and flush with payload bytes; matched claims carry none", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		ledger := &fakeClaimLedger{}
		service := NewService(space, store.SpaceIndex(spaceId), false, time.Unix(1700000000, 0), WithClaimLedger(ledger))
		space.EXPECT().CreateTreePayload(mock.Anything, mock.Anything).Return(treestorage.TreeStorageCreatePayload{
			RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: "tree-1", RawChange: []byte("raw-root")},
			Heads:         []string{"tree-1"},
		}, nil).Once()

		// when
		require.NoError(t, service.Claim(context.Background(), importv2.IdentityClaim{
			SourceKey: "page-1", SbType: coresb.SmartBlockTypePage,
		}))
		assert.Empty(t, ledger.batches, "claims buffer until flushed")
		require.NoError(t, service.FlushClaims(context.Background()))

		// then
		require.Len(t, ledger.batches, 1)
		require.Len(t, ledger.batches[0], 1)
		record := ledger.batches[0][0]
		assert.Equal(t, "page-1", record.SourceKey)
		assert.Equal(t, "tree-1", record.ObjectId)
		assert.False(t, record.Matched)
		assert.Equal(t, []byte("raw-root"), record.PayloadRoot)
		assert.Equal(t, []string{"tree-1"}, record.PayloadHeads)

		// and: a second flush with nothing pending is a no-op
		require.NoError(t, service.FlushClaims(context.Background()))
		assert.Len(t, ledger.batches, 1)
	})

	t.Run("a full batch flushes on its own", func(t *testing.T) {
		// given
		store := objectstore.NewStoreFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		ledger := &fakeClaimLedger{}
		service := NewService(space, store.SpaceIndex(spaceId), false, time.Unix(1700000000, 0), WithClaimLedger(ledger))
		counter := 0
		space.EXPECT().CreateTreePayload(mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, _ payloadcreator.PayloadCreationParams) (treestorage.TreeStorageCreatePayload, error) {
				counter++
				return treestorage.TreeStorageCreatePayload{
					RootRawChange: &treechangeproto.RawTreeChangeWithId{Id: fmt.Sprintf("tree-%d", counter), RawChange: []byte("r")},
				}, nil
			})

		// when: exactly claimBatchSize claims arrive
		for i := 0; i < claimBatchSize; i++ {
			require.NoError(t, service.Claim(context.Background(), importv2.IdentityClaim{
				SourceKey: fmt.Sprintf("page-%d", i), SbType: coresb.SmartBlockTypePage,
			}))
		}

		// then: the batch flushed without an explicit call
		require.Len(t, ledger.batches, 1)
		assert.Len(t, ledger.batches[0], claimBatchSize)
	})
}
