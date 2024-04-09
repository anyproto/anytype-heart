package history

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/commonspace/objecttreebuilder"
	"github.com/anyproto/any-sync/commonspace/objecttreebuilder/mock_objecttreebuilder"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
)

// todo: reimplement

func TestHistory_GetBlocksModifiers(t *testing.T) {
	objectID := "objectId"
	spaceID := "spaceId"
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("object without blocks", func(t *testing.T) {
		history := New()
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectID,
			SpaceID:  spaceID,
		}, "versionId", nil)
		if err != nil {
			return
		}
		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 0)
	})
	t.Run("object with 1 created block", func(t *testing.T) {
		spaceService := mock_space.NewMockService(t)
		space := mock_clientspace.NewMockSpace(t)
		treeBuilder := mock_objecttreebuilder.NewMockTreeBuilder(ctrl)
		smartBlockTest := smarttest.New(objectID)
		bl := &model.Block{Id: "blockId", Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}}
		smartBlockTest.AddBlock(simple.New(bl))
		treeBuilder.EXPECT().BuildHistoryTree(context.Background(), objectID, objecttreebuilder.HistoryTreeOpts{
			BeforeId: "",
			Include:  true,
		}).Return(smartBlockTest, nil)
		space.EXPECT().TreeBuilder().Return(treeBuilder)
		spaceService.EXPECT().Get(context.Background(), spaceID).Return(space, nil)
		history := history{
			spaceService: spaceService,
		}
		blocksModifiers, err := history.GetBlocksModifiers(domain.FullID{
			ObjectID: objectID,
			SpaceID:  spaceID,
		}, "versionId", []*model.Block{bl})
		if err != nil {
			return
		}
		assert.Nil(t, err)
		assert.Len(t, blocksModifiers, 0)
	})
	t.Run("object with 1 modified block", func(t *testing.T) {

	})
	t.Run("object with simple blocks changes by 1 participant", func(t *testing.T) {

	})
	t.Run("object with modified blocks changes by 1 participant", func(t *testing.T) {

	})
	t.Run("object with modified blocks changes by 1 participant", func(t *testing.T) {

	})
	t.Run("object with moved block changes by 1 participant", func(t *testing.T) {

	})
	t.Run("object with block changes by 2 participants", func(t *testing.T) {

	})
}
