package stext

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
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
	err := tb.UpdateTextBlocks(nil, []string{"1", "2"}, true, func(tb text.Block) error {
		tc := tb.Model().GetText()
		require.NotNil(t, tc)
		tc.Checked = true
		return nil
	})
	require.NoError(t, err)
}

func TestTextImpl_Split(t *testing.T) {
	t.Run("top", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
			AddBlock(newTextBlock("1", "onetwo"))
		tb := NewText(sb)
		newId, err := tb.Split(nil, pb.RpcBlockSplitRequest{
			BlockId: "1",
			Range:   &model.Range{From: 3, To: 3},
			Style:   model.BlockContentText_Checkbox,
			Mode:    pb.RpcBlockSplitRequest_TOP,
		})
		require.NoError(t, err)
		require.NotEmpty(t, newId)
		r := sb.NewState()
		assert.Equal(t, []string{newId, "1"}, r.Pick(r.RootId()).Model().ChildrenIds)
		assert.Equal(t, model.BlockContentText_Checkbox, r.Pick(newId).Model().GetText().Style)
		assert.Equal(t, "one", r.Pick(newId).Model().GetText().Text)
		assert.Equal(t, "two", r.Pick("1").Model().GetText().Text)
	})
	t.Run("bottom", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
			AddBlock(newTextBlock("1", "onetwo"))
		tb := NewText(sb)
		newId, err := tb.Split(nil, pb.RpcBlockSplitRequest{
			BlockId: "1",
			Range:   &model.Range{From: 3, To: 3},
			Style:   model.BlockContentText_Checkbox,
			Mode:    pb.RpcBlockSplitRequest_BOTTOM,
		})
		require.NoError(t, err)
		require.NotEmpty(t, newId)
		r := sb.NewState()
		assert.Equal(t, []string{"1", newId}, r.Pick(r.RootId()).Model().ChildrenIds)
		assert.Equal(t, "one", r.Pick("1").Model().GetText().Text)
		assert.Equal(t, "two", r.Pick(newId).Model().GetText().Text)
	})
	t.Run("inner empty", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
			AddBlock(newTextBlock("1", "onetwo"))
		tb := NewText(sb)
		newId, err := tb.Split(nil, pb.RpcBlockSplitRequest{
			BlockId: "1",
			Range:   &model.Range{From: 3, To: 3},
			Style:   model.BlockContentText_Checkbox,
			Mode:    pb.RpcBlockSplitRequest_INNER,
		})
		require.NoError(t, err)
		require.NotEmpty(t, newId)
		r := sb.NewState()
		assert.Equal(t, []string{"1"}, r.Pick(r.RootId()).Model().ChildrenIds)
		assert.Equal(t, []string{newId}, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, "one", r.Pick("1").Model().GetText().Text)
		assert.Equal(t, "two", r.Pick(newId).Model().GetText().Text)
	})
	t.Run("inner", func(t *testing.T) {
		sb := smarttest.New("test")
		stb := newTextBlock("1", "onetwo")
		stb.Model().ChildrenIds = []string{"inner2"}
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
			AddBlock(stb).
			AddBlock(newTextBlock("inner2", "111"))

		tb := NewText(sb)
		newId, err := tb.Split(nil, pb.RpcBlockSplitRequest{
			BlockId: "1",
			Range:   &model.Range{From: 3, To: 3},
			Style:   model.BlockContentText_Checkbox,
			Mode:    pb.RpcBlockSplitRequest_INNER,
		})
		require.NoError(t, err)
		require.NotEmpty(t, newId)
		r := sb.NewState()
		assert.Equal(t, []string{"1"}, r.Pick(r.RootId()).Model().ChildrenIds)
		assert.Equal(t, []string{newId, "inner2"}, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, "one", r.Pick("1").Model().GetText().Text)
		assert.Equal(t, "two", r.Pick(newId).Model().GetText().Text)
	})
}

func TestTextImpl_Merge(t *testing.T) {
	sb := smarttest.New("test")
	tb1 := newTextBlock("1", "one")
	tb1.Model().ChildrenIds = []string{"ch1"}
	tb2 := newTextBlock("2", "two")
	tb2.Model().ChildrenIds = []string{"ch2"}
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
		AddBlock(tb1).
		AddBlock(tb2).
		AddBlock(simple.New(&model.Block{Id: "ch1"})).
		AddBlock(simple.New(&model.Block{Id: "ch2"}))
	tb := NewText(sb)

	err := tb.Merge(nil, "1", "2")
	require.NoError(t, err)

	r := sb.NewState()
	assert.False(t, r.Exists("2"))
	require.True(t, r.Exists("1"))

	assert.Equal(t, "onetwo", r.Pick("1").Model().GetText().Text)
	assert.Equal(t, []string{"ch1", "ch2"}, r.Pick("1").Model().ChildrenIds)
}

func TestTextImpl_SetMark(t *testing.T) {
	t.Run("set mark for empty", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "one")).
			AddBlock(newTextBlock("2", "two"))
		mark := &model.BlockContentTextMark{Type: model.BlockContentTextMark_Bold}
		tb := NewText(sb)
		require.NoError(t, tb.SetMark(nil, mark, "1", "2"))
		r := sb.NewState()
		tb1, _ := getText(r, "1")
		assert.True(t, tb1.HasMarkForAllText(mark))
		tb2, _ := getText(r, "2")
		assert.True(t, tb2.HasMarkForAllText(mark))
	})
	t.Run("set mark reverse", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "one")).
			AddBlock(newTextBlock("2", "two"))
		mark := &model.BlockContentTextMark{Type: model.BlockContentTextMark_Bold}
		tb := NewText(sb)
		require.NoError(t, tb.SetMark(nil, mark, "1", "2"))
		require.NoError(t, tb.SetMark(nil, mark, "1", "2"))
		r := sb.NewState()
		tb1, _ := getText(r, "1")
		assert.False(t, tb1.HasMarkForAllText(mark))
		tb2, _ := getText(r, "2")
		assert.False(t, tb2.HasMarkForAllText(mark))
	})
	t.Run("set mark partial", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "one")).
			AddBlock(newTextBlock("2", "two"))
		mark := &model.BlockContentTextMark{Type: model.BlockContentTextMark_Bold}
		tb := NewText(sb)
		require.NoError(t, tb.SetMark(nil, mark, "1"))
		require.NoError(t, tb.SetMark(nil, mark, "1", "2"))
		r := sb.NewState()
		tb1, _ := getText(r, "1")
		assert.True(t, tb1.HasMarkForAllText(mark))
		tb2, _ := getText(r, "2")
		assert.True(t, tb2.HasMarkForAllText(mark))
	})
}

func TestTextImpl_SetText(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
		AddBlock(newTextBlock("1", "")).
		AddBlock(newTextBlock("2", ""))
	tb := NewText(sb)

	require.NoError(t, tb.SetText(pb.RpcBlockSetTextTextRequest{
		BlockId: "1",
		Text:    "1",
	}))

	

}
