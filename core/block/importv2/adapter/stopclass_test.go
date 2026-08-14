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

	t.Run("runEngine's spool-open failure during suspend carries the verdict", func(t *testing.T) {
		// given — B1 (CONFIRMED): both spool constructors fail on a dead
		// ctx, so this exit is reachable during every shutdown.
		store, err := runstore.Create(context.Background(),
			t.TempDir()+"/run-1", runstore.Manifest{RunId: "run-1"})
		require.NoError(t, err)
		defer store.Close()
		lc := &runLifecycle{store: store, spillDir: store.SpillDir()}
		s := &service{}

		// when
		result := s.runEngine(suspendCtx(), importv2.Request{}, nil, nil, lc, process.NewNoOp())

		// then
		require.Error(t, result.Err)
		assert.True(t, result.Suspended, "a shutdown-shaped failure must carry the suspend verdict")
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
