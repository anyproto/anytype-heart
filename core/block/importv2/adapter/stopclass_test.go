package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

// Invariant 1 extends to every ADAPTER exit: a failure on a dead run
// context is the stop itself, never an INTERNAL_ERROR notification.
func TestStopClassifiedAdapterExits(t *testing.T) {
	suspendCtx := func() context.Context {
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(importv2.ErrSuspended)
		return ctx
	}

	t.Run("a dead ctx no longer fails the spool open at all", func(t *testing.T) {
		// given — REWORKED at the fix-round blocker: this test used to pin
		// that a spool-open failure on a suspend-dead ctx carried the
		// suspend verdict (B1). Since the store-op detachment (opCtx — the
		// fix for any-store's cancelled-op connection leak), store
		// operations are ctx-IMMUNE by design: the open simply succeeds and
		// the run proceeds to its normal suspend classification. The
		// stopFatal guard on the open-failure branch remains for genuine
		// (disk-shaped) failures; its classification property is pinned by
		// the space-get sibling below.
		store, err := runstore.Create(context.Background(),
			t.TempDir()+"/run-1", runstore.Manifest{RunId: "run-1"})
		require.NoError(t, err)
		defer store.Close()

		// when
		spool, err := store.Spool(suspendCtx())

		// then
		require.NoError(t, err, "store operations are detached from run cancellation")
		require.NotNil(t, spool)
	})

	t.Run("execute's space-get failure during suspend carries the verdict", func(t *testing.T) {
		// given
		s := &service{
			config:       &config.Config{},
			spaceService: &ctxAwareSpaceGetter{},
		}
		req := &pb.RpcObjectImportRequest{
			SpaceId: "space-1", Type: model.Import_Markdown, NoProgress: true,
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{Path: []string{t.TempDir()}},
			},
		}

		// when
		result := s.execute(suspendCtx(), req, process.NewNoOp())

		// then
		require.Error(t, result.Err)
		assert.True(t, result.Suspended)
	})
}

// ctxAwareSpaceGetter fails like the real space service does when the run
// context is already dead.
type ctxAwareSpaceGetter struct{}

func (g *ctxAwareSpaceGetter) Get(ctx context.Context, spaceId string) (clientspace.Space, error) {
	return nil, ctx.Err()
}
