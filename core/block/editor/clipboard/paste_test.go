package clipboard

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"testing"
)

const newText = "brand new text"

func TestPasteCtrl_Exec(t *testing.T) {
	t.Run("Single range. Last block has target id", func(t *testing.T) {

		s := state.NewDoc("root1", map[string]simple.Block{
			"root1": base.NewBase(&model.Block{
				Id:          "root1",
				ChildrenIds: []string{"1"},
			}),
			"1": text.NewText(&model.Block{
				Id: "1",
				Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{
						Text: "oldText",
					},
				},
			}),
		}).NewState()
		ps := state.NewDoc("root2", map[string]simple.Block{}).NewState()
		ps.Add(base.NewBase(&model.Block{
			Id:          "root2",
			ChildrenIds: []string{"2"},
		}))
		ps.Add(text.NewText(&model.Block{
			Id: "2",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: newText,
				},
			},
		}))
		ctrl := pasteCtrl{
			s:  s,
			ps: ps,
		}
		assert.NotNil(t, ctrl)

		assert.NoError(t, ctrl.Exec(&pb.RpcBlockPasteRequest{
			FocusedBlockId: "1",
			IsPartOfBlock:  false,
			TextSlot:       newText,
		}))

		b := s.Get("1")
		assert.NotNil(t, b)
		assert.Equal(t, b.Model().Id, "1")

		text, ok := b.Model().Content.(*model.BlockContentOfText)

		assert.True(t, ok)
		assert.NotNil(t, text)
		assert.Equal(t, text.Text.Text, newText)
	})
}
