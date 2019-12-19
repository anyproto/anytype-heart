package block

import (
	"errors"
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_OpenBlock(t *testing.T) {
	t.Run("error while open block", func(t *testing.T) {
		var (
			accountId = "123"
			blockId   = "456"
			expErr    = errors.New("test err")
		)
		fx := newServiceFixture(t, accountId)
		defer fx.ctrl.Finish()
		defer fx.tearDown()

		fx.anytype.EXPECT().GetBlock(blockId).Return(nil, expErr)

		err := fx.OpenBlock(blockId)
		require.Equal(t, expErr, err)
	})
	t.Run("should open dashboard", func(t *testing.T) {
		var (
			accountId = "123"
			blockId   = "456"
		)
		fx := newServiceFixture(t, accountId)
		defer fx.ctrl.Finish()
		defer fx.tearDown()

		mb, _ := fx.newMockBlockWithContent(blockId, &model.BlockContentOfDashboard{
			Dashboard: &model.BlockContentDashboard{},
		}, nil, nil)
		mb.EXPECT().Close()
		fx.anytype.EXPECT().GetBlock(blockId).Return(mb, nil)

		err := fx.OpenBlock(blockId)
		require.NoError(t, err)
		defer func() { require.NoError(t, fx.CloseBlock(blockId)) }()

		assert.Len(t, fx.events, 1)
		assert.Equal(t, smartBlockTypeDashboard, fx.Service.(*service).smartBlocks[blockId].Type())

	})
	t.Run("should open page", func(t *testing.T) {
		var (
			accountId = "123"
			blockId   = "456"
		)
		fx := newServiceFixture(t, accountId)
		defer fx.ctrl.Finish()
		defer fx.tearDown()

		mb, _ := fx.newMockBlockWithContent(blockId, &model.BlockContentOfPage{
			Page: &model.BlockContentPage{},
		}, nil, nil)
		mb.EXPECT().Close()
		fx.anytype.EXPECT().GetBlock(blockId).Return(mb, nil)

		err := fx.OpenBlock(blockId)
		require.NoError(t, err)
		defer func() { require.NoError(t, fx.CloseBlock(blockId)) }()

		assert.Len(t, fx.events, 1)
		assert.Equal(t, smartBlockTypePage, fx.Service.(*service).smartBlocks[blockId].Type())
	})
}

func Test_BlockTypes(t *testing.T) {
	t.Run("text block", func(t *testing.T) {
		assert.Implements(t, (*text.Block)(nil), simple.New(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{},
			},
		}))
	})
	t.Run("icon block", func(t *testing.T) {
		assert.Implements(t, (*base.IconBlock)(nil), simple.New(&model.Block{
			Content: &model.BlockContentOfIcon{
				Icon: &model.BlockContentIcon{},
			},
		}))
	})
}
