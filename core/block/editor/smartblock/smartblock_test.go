package smartblock

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmartBlock_Init(t *testing.T) {
	fx := newFixture(t)
	defer fx.tearDown()
	fx.init([]*model.Block{{Id: "one"}})
	assert.Equal(t, "one", fx.RootId())
}

func TestSmartBlock_Show(t *testing.T) {
	fx := newFixture(t)
	defer fx.tearDown()
	fx.init([]*model.Block{{Id: "1", ChildrenIds: []string{"2"}}, {Id: "2"}})
	var event *pb.Event
	fx.SetEventFunc(func(e *pb.Event) {
		event = e
	})

	err := fx.Show()
	require.NoError(t, err)

	require.NotNil(t, event)
	require.Len(t, event.Messages, 1)
	msg := event.Messages[0].GetBlockShow()
	require.NotNil(t, msg)
	assert.Len(t, msg.Blocks, 2)
	assert.Equal(t, "1", msg.RootId)
}

func TestSmartBlock_Apply(t *testing.T) {
	t.Run("no flags", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.tearDown()
		fx.init([]*model.Block{{Id: "1"}})
		s := fx.NewState()
		s.Add(simple.New(&model.Block{Id: "2"}))
		require.NoError(t, s.InsertTo("1", model.Block_Inner, "2"))

		fx.source.EXPECT().WriteVersion(source.Version{
			Blocks: []*model.Block{{Id: "1", Restrictions: &model.BlockRestrictions{}, ChildrenIds: []string{"2"}}, {Id: "2"}},
		})
		var event *pb.Event
		fx.SetEventFunc(func(e *pb.Event) {
			event = e
		})
		err := fx.Apply(s)
		require.NoError(t, err)
		assert.Equal(t, 1, fx.History().Len())
		assert.NotNil(t, event)
	})

}

type fixture struct {
	t        *testing.T
	ctrl     *gomock.Controller
	source   *testMock.MockSource
	snapshot *testMock.MockSmartBlockSnapshot
	SmartBlock
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	return &fixture{
		SmartBlock: New(),
		t:          t,
		ctrl:       ctrl,
		source:     testMock.NewMockSource(ctrl),
		snapshot:   testMock.NewMockSmartBlockSnapshot(ctrl),
	}
}

func (fx *fixture) tearDown() {
	fx.ctrl.Finish()
}

func (fx *fixture) init(blocks []*model.Block) {
	sb := &core.SmartBlockVersion{
		Snapshot: fx.snapshot,
	}
	fx.source.EXPECT().ReadVersion().Return(sb, nil)
	fx.source.EXPECT().Id().Return(blocks[0].Id).AnyTimes()
	fx.snapshot.EXPECT().Blocks().Return(blocks, nil)

	err := fx.Init(fx.source)
	require.NoError(fx.t, err)
}
