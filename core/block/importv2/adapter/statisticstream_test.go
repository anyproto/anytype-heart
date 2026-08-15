package adapter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
)

// The §15 push producer, end to end over the REAL engine: a whole import
// must leave a coherent statistic trail — the phases in order, the
// materialize counters reaching their denominator, and a terminal event
// that is not stuck behind the coalescing window.

func TestImportStatisticStream(t *testing.T) {
	t.Run("a real import pushes phases in order and finishes at its total", func(t *testing.T) {
		// given: the real runEngine over a mock space, one markdown page
		fx := newLifecycleFixture(t)
		spc := mock_clientspace.NewMockSpace(t)
		var minted atomic.Int64
		spc.EXPECT().CreateTreePayload(mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, _ payloadcreator.PayloadCreationParams) (treestorage.TreeStorageCreatePayload, error) {
				return treestorage.TreeStorageCreatePayload{RootRawChange: &treechangeproto.RawTreeChangeWithId{
					Id: fmt.Sprintf("obj-%03d", minted.Add(1)), RawChange: []byte("raw"),
				}}, nil
			}).Maybe()
		spc.EXPECT().DeriveTreePayload(mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, params payloadcreator.PayloadDerivationParams) (treestorage.TreeStorageCreatePayload, error) {
				return treestorage.TreeStorageCreatePayload{RootRawChange: &treechangeproto.RawTreeChangeWithId{
					Id: "drv-" + params.Key.Marshal(), RawChange: []byte("raw"),
				}}, nil
			}).Maybe()
		spc.EXPECT().CreateTreeObjectWithPayload(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
			func(ctx context.Context, payload treestorage.TreeStorageCreatePayload, initFunc smartblock.InitFunc) (smartblock.SmartBlock, error) {
				id := payload.RootRawChange.Id
				sb := smarttest.New(id)
				if initCtx := initFunc(id); initCtx.State != nil {
					require.NoError(t, sb.Apply(initCtx.State))
				}
				return sb, nil
			}).Maybe()
		fx.service.spaceService = &fakeSpaceGetter{spc: spc}
		fx.service.objectStore = objectstore.NewStoreFixture(t)
		fx.service.installer = fakeInstaller{}
		fx.service.engineRunner = fx.service.runEngine

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "page.md"), []byte("# Hello"), 0o600))
		req := &pb.RpcObjectImportRequest{
			SpaceId:    "space-1",
			Type:       model.Import_Markdown,
			NoProgress: true,
			Params: &pb.RpcObjectImportRequestParamsOfMarkdownParams{
				MarkdownParams: &pb.RpcObjectImportRequestMarkdownParams{
					Path: []string{dir}, NoCollection: true,
				},
			},
		}

		// when
		fx.service.Import(req)
		fx.waitRuns()

		// then: the phase trail is monotone and complete
		events := fx.statistics()
		require.NotEmpty(t, events)
		var phases []pb.EventImportStatisticPhase
		for _, e := range events {
			if len(phases) == 0 || phases[len(phases)-1] != e.Phase {
				phases = append(phases, e.Phase)
			}
		}
		assert.Equal(t, []pb.EventImportStatisticPhase{
			pb.EventImportStatistic_Scanning,
			pb.EventImportStatistic_Fetching,
			pb.EventImportStatistic_Analyzing,
			pb.EventImportStatistic_Fetching,
			pb.EventImportStatistic_Creating,
			pb.EventImportStatistic_Finalizing,
		}, phases)

		// and: the terminal event is flushed past the window, complete, and
		// says cancel would now remove what was created
		last := events[len(events)-1]
		assert.Equal(t, int64(1), last.PagesTotal, "the spool census fixes pass 3's denominator")
		assert.Equal(t, int64(1), last.PagesDone, "materialization reached it")
		assert.Equal(t, int64(1), last.ObjectsCreated)
		assert.True(t, last.TotalsKnown)
		assert.Equal(t, pb.EventImportStatistic_RemovesCreated, last.CancelEffect)
		assert.Equal(t, model.Import_Markdown, last.ImportType)
		assert.NotEmpty(t, last.ImportId, "a durable run is pollable by its id")

		// and: the SCANNING events are the only ones that admit no total —
		// §15.3's count-up, never a fake bar
		for _, e := range events {
			assert.Equal(t, e.Phase != pb.EventImportStatistic_Scanning, e.TotalsKnown,
				"totals become known at the pass-1/pass-2 boundary and stay known")
			assert.LessOrEqual(t, e.PagesDone, e.PagesTotal,
				"a definition must never push done past a denominator that is the claim count")
		}
	})
}
