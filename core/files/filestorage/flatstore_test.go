package filestorage

import (
	"context"
	"fmt"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
)

func TestFlatstoreGarbageCollect(t *testing.T) {
	sender := mock_event.NewMockSender(t)
	sender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

	s, err := newFlatStore(t.TempDir(), sender, 0)
	require.NoError(t, err)

	ctx := context.Background()

	testBlocks := generateTestBlocks(5)
	err = s.Add(ctx, testBlocks)
	require.NoError(t, err)

	gc := newFlatStoreGarbageCollector(s)
	gc.MarkAsUsing([]cid.Cid{testBlocks[0].Cid(), testBlocks[1].Cid()})
	err = gc.CollectGarbage(ctx)
	require.NoError(t, err)

	for _, b := range testBlocks[:2] {
		_, err = s.Get(ctx, b.Cid())
		require.NoError(t, err)
	}
	for _, b := range testBlocks[2:] {
		_, err = s.Get(ctx, b.Cid())
		require.Error(t, err)
	}
}

func generateTestBlocks(num int) []blocks.Block {
	bs := make([]blocks.Block, 0, num)
	for i := 0; i < num; i++ {
		bs = append(bs, blocks.NewBlock([]byte(fmt.Sprintf("test%d", i))))
	}
	return bs
}
