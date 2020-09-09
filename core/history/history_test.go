package history

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/anytypeio/go-anytype-middleware/util/testMock/mockMeta"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistory_Versions(t *testing.T) {
	t.Run("no version", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()
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
		defer fx.tearDown()
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
	a := testMock.NewMockService(ctrl)
	m := mockMeta.NewMockService(ctrl)
	a.EXPECT().PredefinedBlocks().Return(core.PredefinedBlockIds{
		Profile: "profileId",
	}).AnyTimes()
	a.EXPECT().PageStore().Return(nil).AnyTimes()
	return &fixture{
		History: NewHistory(a, new(bs), m),
		anytype: a,
		meta:    m,
		ctrl:    ctrl,
	}
}

type fixture struct {
	History
	anytype *testMock.MockService
	meta    *mockMeta.MockService
	ctrl    *gomock.Controller
}

func (fx *fixture) newTestSB(id string) *change.TestSmartblock {
	sb := change.NewTestSmartBlock()
	fx.anytype.EXPECT().GetBlock(id).Return(sb, nil).AnyTimes()
	return sb
}

func (fx *fixture) tearDown() {
	fx.ctrl.Finish()
}

type bs struct{}

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
