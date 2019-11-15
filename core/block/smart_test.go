package block

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/testMock"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommonSmart_Open(t *testing.T) {
	t.Run("should send fullscreen event on open", func(t *testing.T) {
		fx := newFixture(t, "")
		defer fx.tearDown()

		sb := &commonSmart{
			s:              fx.Service.(*service),
			versionsChange: func(vers []core.BlockVersion) {},
		}

		mblock, _ := fx.newMockBlockWithContent(
			"1",
			&model.BlockContentOfPage{Page: &model.BlockContentPage{}},
			[]string{"2", "3"},
			map[string]core.BlockVersion{
				"2": fx.newMockVersion(&model.Block{Id: "2"}),
				"3": fx.newMockVersion(&model.Block{Id: "3"}),
			},
		)
		block := &blockWrapper{MockBlock: mblock}

		err := sb.Open(block)
		require.NoError(t, err)

		defer func() {
			err := sb.Close()
			require.NoError(t, err)
			assert.True(t, block.cancelBlockVersionsCalled)
			assert.True(t, block.cancelClientEventsCalled)
		}()

		require.Len(t, fx.events, 1)
		event := fx.events[0]
		require.IsType(t, (*pb.EventMessageOfBlockShowFullscreen)(nil), event.Message)
		showFullscreen := event.Message.(*pb.EventMessageOfBlockShowFullscreen).BlockShowFullscreen
		assert.Equal(t, showFullscreen.RootId, "1")
		assert.Len(t, showFullscreen.Blocks, 3)
	})
}

func TestCommonSmart_Create(t *testing.T) {
	t.Run("should create block", func(t *testing.T) {
		fx := newFixture(t, "")
		defer fx.tearDown()

		sb := &commonSmart{
			s: fx.Service.(*service),
		}

		mblock, _ := fx.newMockBlockWithContent(
			"1",
			&model.BlockContentOfPage{Page: &model.BlockContentPage{}},
			[]string{"2", "3"},
			map[string]core.BlockVersion{
				"2": fx.newMockVersion(&model.Block{Id: "2"}),
				"3": fx.newMockVersion(&model.Block{Id: "3"}),
			},
		)
		block := &blockWrapper{MockBlock: mblock}

		err := sb.Open(block)
		require.NoError(t, err)
		defer func() {
			err := sb.Close()
			require.NoError(t, err)
		}()

		req := pb.RpcBlockCreateRequest{
			Block: &model.Block{
				Content: &model.BlockContentOfPage{Page: &model.BlockContentPage{}},
			},
			TargetId:  "3",
			Position:  model.Block_BEFORE,
			ContextId: "1",
			ParentId:  "1",
		}
		newBlockId := "23"
		newBlock, _ := fx.newMockBlockWithContent(newBlockId, req.Block.Content, nil, nil)
		block.EXPECT().NewBlock(*req.Block).Return(newBlock, nil)
		newBlockVer, _ := newBlock.GetCurrentVersion()

		var versToSave []*model.Block
		block.EXPECT().AddVersions(&matcher{name: "AddVersions", f: func(x interface{}) bool {
			versToSave = x.([]*model.Block)
			return true
		}}).Return([]core.BlockVersion{newBlockVer}, nil)

		id, err := sb.Create(req)
		require.NoError(t, err)
		assert.Equal(t, newBlockId, id)

		require.Len(t, versToSave, 2)
		assert.Equal(t, "23", versToSave[0].Id)
		assert.Equal(t, []string{"2", "23", "3"}, versToSave[1].ChildrenIds)

		assert.Len(t, fx.events, 3)
	})
}

type blockWrapper struct {
	*testMock.MockBlock
	clientEventsChan          chan<- proto.Message
	blockVersionsChan         chan<- []core.BlockVersion
	cancelClientEventsCalled  bool
	cancelBlockVersionsCalled bool
}

func (bw *blockWrapper) SubscribeClientEvents(ch chan<- proto.Message) (func(), error) {
	bw.clientEventsChan = ch
	return func() {
		bw.cancelClientEventsCalled = true
		close(bw.clientEventsChan)
	}, nil
}

func (bw *blockWrapper) SubscribeNewVersionsOfBlocks(v string, ch chan<- []core.BlockVersion) (func(), error) {
	bw.blockVersionsChan = ch
	return func() {
		bw.cancelBlockVersionsCalled = true
		close(bw.blockVersionsChan)
	}, nil
}
