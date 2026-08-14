package identity

import (
	"context"
	"fmt"
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
