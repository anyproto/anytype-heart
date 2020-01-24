package block

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/history"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommonSmart_Undo(t *testing.T) {
	t.Run("not history for empty", func(t *testing.T) {
		fx := newPageFixture(t)
		defer fx.ctrl.Finish()
		defer fx.tearDown()
		histMock := testMock.NewMockHistory(fx.ctrl)
		fx.history = histMock
		histMock.EXPECT().Previous().Return(history.Action{}, history.ErrNoHistory)
		assert.Equal(t, history.ErrNoHistory, fx.Undo())
	})

	t.Run("apply action", func(t *testing.T) {
		fx := newPageFixture(t)
		defer fx.ctrl.Finish()
		defer fx.tearDown()
		histMock := testMock.NewMockHistory(fx.ctrl)
		fx.history = histMock
		action := history.Action{
			Add: []simple.Block{simple.New(&model.Block{
				Id: "add",
			})},
			Change: []history.Change{{
				Before: simple.New(&model.Block{
					Id:          "changed",
					ChildrenIds: []string{"before"},
				}),
				After: simple.New(&model.Block{
					Id:          "changed",
					ChildrenIds: []string{"after"},
				}),
			}},
			Remove: []simple.Block{simple.New(&model.Block{
				Id: "removed",
			})},
		}
		histMock.EXPECT().Previous().Return(action, nil)
		err := fx.Undo()
		require.NoError(t, err)
		assert.Nil(t, fx.versions["add"])
		assert.NotEmpty(t, fx.versions["changed"])
		assert.NotEmpty(t, fx.versions["removed"])
	})
}

func TestCommonSmart_Redo(t *testing.T) {
	t.Run("not history for empty", func(t *testing.T) {
		fx := newPageFixture(t)
		defer fx.ctrl.Finish()
		defer fx.tearDown()
		histMock := testMock.NewMockHistory(fx.ctrl)
		fx.history = histMock
		histMock.EXPECT().Next().Return(history.Action{}, history.ErrNoHistory)
		assert.Equal(t, history.ErrNoHistory, fx.Redo())
	})

	t.Run("apply action", func(t *testing.T) {
		fx := newPageFixture(t)
		defer fx.ctrl.Finish()
		defer fx.tearDown()
		histMock := testMock.NewMockHistory(fx.ctrl)
		fx.history = histMock
		action := history.Action{
			Add: []simple.Block{simple.New(&model.Block{
				Id: "add",
			})},
			Change: []history.Change{{
				Before: simple.New(&model.Block{
					Id:          "changed",
					ChildrenIds: []string{"before"},
				}),
				After: simple.New(&model.Block{
					Id:          "changed",
					ChildrenIds: []string{"after"},
				}),
			}},
			Remove: []simple.Block{simple.New(&model.Block{
				Id: "removed",
			})},
		}
		histMock.EXPECT().Next().Return(action, nil)
		err := fx.Redo()
		require.NoError(t, err)
		assert.NotEmpty(t, fx.versions["add"])
		assert.NotEmpty(t, fx.versions["changed"])
		assert.Nil(t, fx.versions["removed"])
	})
}
