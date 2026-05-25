package canvas

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestNodeBlock_FillSmartIds(t *testing.T) {
	t.Run("object node fills targetObjectId", func(t *testing.T) {
		b := NewNodeBlock(&model.Block{
			Id: "block1",
			Content: &model.BlockContentOfCanvasNode{
				CanvasNode: &model.BlockContentCanvasNode{
					Type:           model.BlockContentCanvasNode_Object,
					TargetObjectId: "obj-123",
				},
			},
		}).(*nodeBlock)

		ids := b.FillSmartIds(nil)
		assert.Equal(t, []string{"obj-123"}, ids)
	})

	t.Run("text node does not fill ids", func(t *testing.T) {
		b := NewNodeBlock(&model.Block{
			Id: "block2",
			Content: &model.BlockContentOfCanvasNode{
				CanvasNode: &model.BlockContentCanvasNode{
					Type: model.BlockContentCanvasNode_Text,
					Text: "hello",
				},
			},
		}).(*nodeBlock)

		ids := b.FillSmartIds(nil)
		assert.Empty(t, ids)
	})
}

func TestNodeBlock_HasSmartIds(t *testing.T) {
	t.Run("returns true when targetObjectId is set", func(t *testing.T) {
		b := NewNodeBlock(&model.Block{
			Id: "block1",
			Content: &model.BlockContentOfCanvasNode{
				CanvasNode: &model.BlockContentCanvasNode{
					Type:           model.BlockContentCanvasNode_Object,
					TargetObjectId: "obj-123",
				},
			},
		}).(*nodeBlock)

		assert.True(t, b.HasSmartIds())
	})

	t.Run("returns false when targetObjectId is empty", func(t *testing.T) {
		b := NewNodeBlock(&model.Block{
			Id: "block2",
			Content: &model.BlockContentOfCanvasNode{
				CanvasNode: &model.BlockContentCanvasNode{
					Type: model.BlockContentCanvasNode_Text,
					Text: "some text",
				},
			},
		}).(*nodeBlock)

		assert.False(t, b.HasSmartIds())
	})
}
