package stext

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTextBlock(id, contentText string) simple.Block {
	return text.NewText(&model.Block{
		Id: id,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: contentText,
			},
		},
	})
}

func TestTextImpl_UpdateTextBlocks(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
		AddBlock(newTextBlock("1", "one")).
		AddBlock(newTextBlock("2", "two"))

	tb := NewText(sb)
	err := tb.UpdateTextBlocks([]string{"1", "2"}, true, func(tb text.Block) error {
		tc := tb.Model().GetText()
		require.NotNil(t, tc)
		tc.Checked = true
		return nil
	})
	require.NoError(t, err)
}

func TestTextImpl_Split(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
		AddBlock(newTextBlock("1", "onetwo"))
	tb := NewText(sb)
	newId, err := tb.Split("1", 3, model.BlockContentText_Checkbox)
	require.NoError(t, err)
	require.NotEmpty(t, newId)
	r := sb.NewState()
	assert.Equal(t, []string{newId, "1"}, r.Pick(r.RootId()).Model().ChildrenIds)
	assert.Equal(t, model.BlockContentText_Checkbox, r.Pick(newId).Model().GetText().Style)
}
