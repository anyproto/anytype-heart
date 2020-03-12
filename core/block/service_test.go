package block

/**
import (
	"errors"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/bookmark"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/file"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
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

		fx.anytype.EXPECT().GetBlockWithBatcher(blockId).Return(nil, expErr)

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
		fx.anytype.EXPECT().GetBlockWithBatcher(blockId).Return(mb, nil)

		err := fx.OpenBlock(blockId)
		require.NoError(t, err)
		defer func() { require.NoError(t, fx.CloseBlock(blockId)) }()

		assert.Len(t, fx.events, 1)
		assert.Equal(t, smartBlockTypeDashboard, fx.Service.(*service).openedBlocks[blockId].Type())

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
		fx.anytype.EXPECT().GetBlockWithBatcher(blockId).Return(mb, nil)

		err := fx.OpenBlock(blockId)
		require.NoError(t, err)
		defer func() { require.NoError(t, fx.CloseBlock(blockId)) }()

		assert.Len(t, fx.events, 1)
		assert.Equal(t, smartBlockTypePage, fx.Service.(*service).openedBlocks[blockId].Type())
	})
}

func TestService_pickBlock(t *testing.T) {
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
	fx.anytype.EXPECT().GetBlockWithBatcher(blockId).Return(mb, nil)

	// send command without open
	fx.Redo(pb.RpcBlockRedoRequest{ContextId: blockId})
	require.Len(t, fx.Service.(*service).openedBlocks, 1)
	fx.Service.(*service).openedBlocks[blockId].lastUsage = time.Now().Add(-time.Hour)
	fx.Service.(*service).cleanupBlocks()
	require.Len(t, fx.Service.(*service).openedBlocks, 0)
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
	t.Run("file block", func(t *testing.T) {
		assert.Implements(t, (*file.Block)(nil), simple.New(&model.Block{
			Content: &model.BlockContentOfFile{
				File: &model.BlockContentFile{},
			},
		}))
	})
	t.Run("link block", func(t *testing.T) {
		assert.Implements(t, (*link.Block)(nil), simple.New(&model.Block{
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{},
			},
		}))
	})
	t.Run("page block", func(t *testing.T) {
		assert.Implements(t, (*base.PageBlock)(nil), simple.New(&model.Block{
			Content: &model.BlockContentOfPage{
				Page: &model.BlockContentPage{},
			},
		}))
	})
	t.Run("bookmark block", func(t *testing.T) {
		assert.Implements(t, (*bookmark.Block)(nil), simple.New(&model.Block{
			Content: &model.BlockContentOfBookmark{
				Bookmark: &model.BlockContentBookmark{},
			},
		}))
	})
}
*/
