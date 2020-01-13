package block

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
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
}
