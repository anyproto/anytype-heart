package history

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/app/testapp"
	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/threads"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockMeta"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistory_Versions(t *testing.T) {
	t.Run("no version", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown(t)
		fx.newTestSB("pageId").AddChanges("a",
			newSnapshot("s1", "", nil),
			newChange("c2", "s1", "s1"),
			newChange("c3", "s1", "c2"),
		)
		resp, err := fx.Versions("pageId", "", 0)
		require.NoError(t, err)
		assert.Len(t, resp, 3)
	})
	t.Run("chunks", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown(t)
		fx.newTestSB("pageId").AddChanges("a",
			newSnapshot("s1", "", nil),
			newChange("c2", "s1", "s1"),
			newChange("c3", "s1", "c2"),
			newSnapshot("s4", "s1", map[string]string{
				"a": "c3",
			}, "c3"),
			newChange("c5", "s4", "s4"),
		)
		resp, err := fx.Versions("pageId", "", 0)
		require.NoError(t, err)
		assert.Len(t, resp, 5)
	})
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	h := New()
	ta := testapp.New().
		With(&bs{}).
		With(h)
	a := testMock.RegisterMockAnytype(ctrl, ta)
	a.EXPECT().PredefinedBlocks().Return(threads.DerivedSmartblockIds{
		Profile: "profileId",
	}).AnyTimes()
	a.EXPECT().ObjectStore().Return(nil).AnyTimes()
	a.EXPECT().ProfileID().AnyTimes()
	a.EXPECT().LocalProfile().AnyTimes()
	mockMeta.RegisterMockMeta(ctrl, ta)
	require.NoError(t, ta.Start())
	return &fixture{
		History: h,
		ta:      ta,
		ctrl:    ctrl,
	}
}

type fixture struct {
	History
	ta   *testapp.TestApp
	ctrl *gomock.Controller
}

func (fx *fixture) newTestSB(id string) *change.TestSmartblock {
	sb := change.NewTestSmartBlock()
	testMock.GetMockAnytype(fx.ta).EXPECT().GetBlock(id).Return(sb, nil).AnyTimes()
	return sb
}

func (fx *fixture) tearDown(t *testing.T) {
	require.NoError(t, fx.ta.Close())
	fx.ctrl.Finish()
}

type bs struct{}

func (b *bs) Init(_ *app.App) (err error) {
	return
}

func (b *bs) Name() (name string) {
	return "blockService"
}

func (b *bs) ResetToState(pageId string, s *state.State) (err error) {
	return
}

func newSnapshot(id, snapshotId string, heads map[string]string, prevIds ...string) *change.Change {
	return &change.Change{
		Id: id,
		Change: &pb.Change{
			PreviousIds:    prevIds,
			LastSnapshotId: snapshotId,
			Snapshot: &pb.ChangeSnapshot{
				LogHeads: heads,
			},
		},
	}
}

func newChange(id, snapshotId string, prevIds ...string) *change.Change {
	return &change.Change{
		Id: id,
		Change: &pb.Change{
			PreviousIds:    prevIds,
			LastSnapshotId: snapshotId,
			Content:        []*pb.ChangeContent{},
		},
	}
}
