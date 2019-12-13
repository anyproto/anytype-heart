package block

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/gogo/protobuf/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_Normalize(t *testing.T) {

	var (
		contRow = &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Row,
			},
		}
		contColumn = &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Column,
			},
		}

		fieldsWidth = func(w float64) *types.Struct {
			return &types.Struct{
				Fields: map[string]*types.Value{
					"width": testFloatValue(w),
				},
			}
		}
	)

	t.Run("nothing to change", func(t *testing.T) {
		fx := newStateFixture(t)
		msgs, err := fx.apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 0)
		defer fx.Finish()
	})
	t.Run("clean missing children", func(t *testing.T) {
		fx := newStateFixture(t)
		defer fx.Finish()

		fx.sb.versions["root"].Model().ChildrenIds = []string{"one"}
		fx.sb.versions["one"] = simple.New(&model.Block{Id: "one", ChildrenIds: []string{"missingId"}})
		fx.get("one")
		msgs, err := fx.apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 1)
		assert.Len(t, fx.saved, 1)
		assert.Len(t, fx.sb.versions["one"].Model().ChildrenIds, 0)
	})

	t.Run("remove empty layouts", func(t *testing.T) {
		fx := newStateFixture(t)
		defer fx.Finish()

		fx.sb.versions["root"].Model().ChildrenIds = []string{"r1", "t1"}
		fx.sb.versions["r1"] = simple.New(&model.Block{Id: "r1", ChildrenIds: []string{"c1", "c2"}, Content: contRow})
		fx.sb.versions["c1"] = simple.New(&model.Block{Id: "c1", Content: contColumn})
		fx.sb.versions["c2"] = simple.New(&model.Block{Id: "c2", Content: contColumn})
		fx.sb.versions["t1"] = simple.New(&model.Block{Id: "t1"})
		fx.get("c1")
		fx.get("c2")
		msgs, err := fx.apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 4) // 3 remove + 1 change
		assert.Len(t, fx.saved, 1)
		assert.Equal(t, []string{"t1"}, fx.sb.versions["root"].Model().ChildrenIds)
		assert.Nil(t, fx.sb.versions["r1"])
		assert.Nil(t, fx.sb.versions["c1"])
		assert.Nil(t, fx.sb.versions["c2"])
	})

	t.Run("remove one column row", func(t *testing.T) {
		fx := newStateFixture(t)
		defer fx.Finish()

		fx.sb.versions["root"].Model().ChildrenIds = []string{"r1", "t1"}
		fx.sb.versions["r1"] = simple.New(&model.Block{Id: "r1", ChildrenIds: []string{"c1"}, Content: contRow})
		fx.sb.versions["c1"] = simple.New(&model.Block{Id: "c1", ChildrenIds: []string{"t2"}, Content: contColumn})
		fx.sb.versions["t1"] = simple.New(&model.Block{Id: "t1"})
		fx.sb.versions["t2"] = simple.New(&model.Block{Id: "t2"})
		fx.get("c1")
		msgs, err := fx.apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 3) // 2 remove + 1 change
		assert.Len(t, fx.saved, 1)
		assert.Equal(t, []string{"t2", "t1"}, fx.sb.versions["root"].Model().ChildrenIds)
		assert.Nil(t, fx.sb.versions["r1"])
		assert.Nil(t, fx.sb.versions["c1"])
	})

	t.Run("do not recalculate width", func(t *testing.T) {
		fx := newStateFixture(t)
		defer fx.Finish()

		fx.sb.versions["root"].Model().ChildrenIds = []string{"r1"}
		fx.sb.versions["r1"] = simple.New(&model.Block{Id: "r1", ChildrenIds: []string{"c1", "c2"}, Content: contRow})
		fx.sb.versions["c1"] = simple.New(&model.Block{Id: "c1", ChildrenIds: []string{"t1"}, Content: contColumn, Fields: fieldsWidth(0.5)})
		fx.sb.versions["c2"] = simple.New(&model.Block{Id: "c2", ChildrenIds: []string{"t2"}, Content: contColumn, Fields: fieldsWidth(0.5)})
		fx.sb.versions["t1"] = simple.New(&model.Block{Id: "t1"})
		fx.sb.versions["t2"] = simple.New(&model.Block{Id: "t2"})
		fx.get("r1")
		msgs, err := fx.apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 0)
		assert.Len(t, fx.saved, 0)
	})

	t.Run("recalculate width", func(t *testing.T) {
		fx := newStateFixture(t)
		defer fx.Finish()

		fx.sb.versions["root"].Model().ChildrenIds = []string{"r1"}
		fx.sb.versions["r1"] = simple.New(&model.Block{Id: "r1", ChildrenIds: []string{"c1", "c2"}, Content: contRow})
		fx.sb.versions["c1"] = simple.New(&model.Block{Id: "c1", ChildrenIds: []string{"t1"}, Content: contColumn, Fields: fieldsWidth(0.2)})
		fx.sb.versions["c2"] = simple.New(&model.Block{Id: "c2", ChildrenIds: []string{"t2"}, Content: contColumn, Fields: fieldsWidth(0.3)})
		fx.sb.versions["t1"] = simple.New(&model.Block{Id: "t1"})
		fx.sb.versions["t2"] = simple.New(&model.Block{Id: "t2"})
		fx.get("r1")
		msgs, err := fx.apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 2) // 2 change
		assert.Len(t, fx.saved, 2)
	})
}

func newStateFixture(t *testing.T) *stateFixture {
	ctrl := gomock.NewController(t)
	block := &testBlock{
		MockBlock: testMock.NewMockBlock(ctrl),
	}
	block.EXPECT().GetId().AnyTimes().Return("root")
	sb := &commonSmart{
		block:    block,
		versions: make(map[string]simple.Block),
	}
	sb.versions[block.GetId()] = simple.New(&model.Block{
		Id:          "root",
		ChildrenIds: []string{},
		Content: &model.BlockContentOfPage{
			Page: &model.BlockContentPage{},
		},
	})
	fx := &stateFixture{
		block: block,
		state: sb.newState(),
		sb:    sb,
		ctrl:  ctrl,
	}
	fx.block.fx = fx
	return fx
}

type stateFixture struct {
	*state
	block *testBlock
	sb    *commonSmart
	ctrl  *gomock.Controller
	saved []*model.Block
}

func (fx *stateFixture) Finish() {
	fx.ctrl.Finish()
}

type testBlock struct {
	fx *stateFixture
	*testMock.MockBlock
}

func (tb *testBlock) AddVersions(vers []*model.Block) ([]core.BlockVersion, error) {
	tb.fx.saved = vers
	return nil, nil
}
