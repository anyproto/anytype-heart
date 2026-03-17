package stext

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/link"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/block/undo"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func newTextBlock(id, contentText string, childrenIds ...string) simple.Block {
	return text.NewText(&model.Block{
		Id: id,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: contentText,
			},
		},
		ChildrenIds: childrenIds,
	})
}

func newCodeBlock(id, contentText string, childrenIds ...string) simple.Block {
	return text.NewText(&model.Block{
		Id: id,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  contentText,
				Style: model.BlockContentText_Code,
			},
		},
		ChildrenIds:     childrenIds,
		BackgroundColor: "grey",
	})
}

func TestTextImpl_UpdateTextBlocks(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
		AddBlock(newTextBlock("1", "one")).
		AddBlock(newTextBlock("2", "two"))

	sender := mock_event.NewMockSender(t)
	tb := NewText(sb, nil, sender)
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
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)
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
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)
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
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)
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

		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)
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
	t.Run("split - when code block", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.AddBlock(
			simple.New(
				&model.Block{
					Id:          "test",
					ChildrenIds: []string{"1"},
				},
			),
		).AddBlock(newCodeBlock("1", "onetwo"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		newId, err := tb.Split(nil, pb.RpcBlockSplitRequest{
			BlockId: "1",
			Range:   &model.Range{From: 3, To: 3},
			Style:   model.BlockContentText_Checkbox,
			Mode:    pb.RpcBlockSplitRequest_BOTTOM,
		})

		// then
		require.NoError(t, err)
		require.NotEmpty(t, newId)
		r := sb.NewState()
		assert.Equal(t, []string{"1", newId}, r.Pick(r.RootId()).Model().ChildrenIds)
		assert.Equal(t, "one", r.Pick("1").Model().GetText().Text)
		assert.Equal(t, "two", r.Pick(newId).Model().GetText().Text)
		assert.Equal(t, "", r.Pick(newId).Model().BackgroundColor)
		assert.Equal(t, model.BlockContentText_Checkbox, r.Pick(newId).Model().GetText().Style)
	})
}

func TestTextImpl_Merge(t *testing.T) {
	t.Run("should merge two text blocks", func(t *testing.T) {
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
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		err := tb.Merge(nil, "1", "2")
		require.NoError(t, err)

		r := sb.NewState()
		assert.False(t, r.Exists("2"))
		require.True(t, r.Exists("1"))

		assert.Equal(t, "onetwo", r.Pick("1").Model().GetText().Text)
		assert.Equal(t, []string{"ch1", "ch2"}, r.Pick("1").Model().ChildrenIds)
	})

	t.Run("should keep children indentation when first is deeper than second", func(t *testing.T) {
		// Structure:
		//   root
		//   ├── A
		//   │   └── SubA (first - visually above B)
		//   └── B (second - being merged into SubA)
		//       ├── Ch1
		//       └── Ch2
		//
		// After merge B into SubA, Ch1 and Ch2 should become children of A
		// (B's previous sibling) to retain their indentation level.
		sb := smarttest.New("test")
		subA := newTextBlock("subA", "sub")
		b := newTextBlock("B", "two")
		b.Model().ChildrenIds = []string{"ch1", "ch2"}
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"A", "B"}})).
			AddBlock(simple.New(&model.Block{Id: "A", ChildrenIds: []string{"subA"}})).
			AddBlock(subA).
			AddBlock(b).
			AddBlock(newTextBlock("ch1", "child1")).
			AddBlock(newTextBlock("ch2", "child2"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		err := tb.Merge(nil, "subA", "B")
		require.NoError(t, err)

		r := sb.NewState()
		assert.False(t, r.Exists("B"))
		assert.Equal(t, "subtwo", r.Pick("subA").Model().GetText().Text)
		// Ch1 and Ch2 should be children of A (B's previous sibling), not subA
		assert.Equal(t, []string{"subA", "ch1", "ch2"}, r.Pick("A").Model().ChildrenIds)
		assert.Empty(t, r.Pick("subA").Model().ChildrenIds)
	})

	t.Run("shouldn't merge blocks inside header block", func(t *testing.T) {
		sb := smarttest.New("test")
		tb1 := newTextBlock("1", "one")
		tb1.Model().ChildrenIds = []string{"ch1"}
		tb2 := newTextBlock("2", "two")
		tb2.Model().ChildrenIds = []string{"ch2"}
		sb.AddBlock(simple.New(&model.Block{Id: template.HeaderLayoutId, ChildrenIds: []string{"1", "2"}})).
			AddBlock(tb1).
			AddBlock(tb2).
			AddBlock(simple.New(&model.Block{Id: "ch1"})).
			AddBlock(simple.New(&model.Block{Id: "ch2"}))

		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		err := tb.Merge(nil, "1", "2")
		require.NoError(t, err)

		r := sb.NewState()
		require.True(t, r.Exists("1"))
		require.True(t, r.Exists("2"))

		assert.Equal(t, "one", r.Pick("1").Model().GetText().Text)
		assert.Equal(t, "two", r.Pick("2").Model().GetText().Text)
		assert.Equal(t, []string{"ch1"}, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"ch2"}, r.Pick("2").Model().ChildrenIds)
	})

	// Issue #2dexn9f
	t.Run("don't set style in empty header blocks", func(t *testing.T) {
		sb := smarttest.New("test")

		tb1 := newTextBlock("title", "")
		tb1.Model().GetText().Style = model.BlockContentText_Title

		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{template.HeaderLayoutId}})).
			AddBlock(simple.New(&model.Block{Id: template.HeaderLayoutId, ChildrenIds: []string{"title"}})).
			AddBlock(tb1).
			AddBlock(newTextBlock("123", "one"))

		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		err := tb.Merge(nil, "title", "123")
		require.NoError(t, err)

		r := sb.NewState()
		require.True(t, r.Exists("title"))

		assert.Equal(t, "one", r.Pick("title").Model().GetText().Text)
		assert.Equal(t, []string{"title"}, r.Pick(template.HeaderLayoutId).Model().ChildrenIds)
		assert.Equal(t, model.BlockContentText_Title, r.Pick("title").Model().GetText().GetStyle())
	})
}

func TestTextImpl_SetMark(t *testing.T) {
	t.Run("set mark for empty", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "one")).
			AddBlock(newTextBlock("2", "two"))
		mark := &model.BlockContentTextMark{Type: model.BlockContentTextMark_Bold}
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)
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
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)
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
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)
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
	setTextApplyInterval = time.Second / 2

	t.Run("set text after interval", func(t *testing.T) {
		ctx := session.NewContext()
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", " ")).
			AddBlock(newTextBlock("2", " "))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		require.NoError(t, setText(tb, ctx, pb.RpcBlockTextSetTextRequest{
			BlockId: "1",
			Text:    "1",
		}))
		require.NoError(t, setText(tb, ctx, pb.RpcBlockTextSetTextRequest{
			BlockId: "1",
			Text:    "12",
		}))
		tb.(*textImpl).Lock()
		assert.Equal(t, " ", sb.NewState().Pick("1").Model().GetText().Text)
		tb.(*textImpl).Unlock()
		time.Sleep(time.Second)
		tb.(*textImpl).Lock()
		assert.Equal(t, "12", sb.NewState().Pick("1").Model().GetText().Text)
		tb.(*textImpl).Unlock()
		assert.Len(t, sb.Results.Applies, 1)
	})
	t.Run("set text and new op", func(t *testing.T) {
		ctx := session.NewContext()
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", " ")).
			AddBlock(newTextBlock("2", " "))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		require.NoError(t, setText(tb, ctx, pb.RpcBlockTextSetTextRequest{
			BlockId: "1",
			Text:    "1",
		}))
		require.NoError(t, setText(tb, ctx, pb.RpcBlockTextSetTextRequest{
			BlockId: "1",
			Text:    "12",
		}))
		tb.(*textImpl).Lock()
		assert.Equal(t, " ", sb.NewState().Pick("1").Model().GetText().Text)
		tb.(*textImpl).flushSetTextState(smartblock.ApplyInfo{})
		assert.Equal(t, "12", sb.NewState().Pick("1").Model().GetText().Text)
		tb.(*textImpl).Unlock()
		time.Sleep(time.Second)
		tb.(*textImpl).Lock()
		assert.Equal(t, "12", sb.NewState().Pick("1").Model().GetText().Text)
		tb.(*textImpl).Unlock()
		assert.Len(t, sb.Results.Applies, 1)
	})
	t.Run("set text two blocks", func(t *testing.T) {
		ctx := session.NewContext()
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "")).
			AddBlock(newTextBlock("2", ""))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		require.NoError(t, setText(tb, ctx, pb.RpcBlockTextSetTextRequest{
			BlockId: "1",
			Text:    "1",
		}))
		flushLocked(tb)
		require.NoError(t, setText(tb, ctx, pb.RpcBlockTextSetTextRequest{
			BlockId: "2",
			Text:    "2",
		}))
		flushLocked(tb)
		assert.Equal(t, "1", sb.NewState().Pick("1").Model().GetText().Text)
		assert.Equal(t, "2", sb.NewState().Pick("2").Model().GetText().Text)
		time.Sleep(time.Second)
		assert.Len(t, sb.Results.Applies, 2)
	})
	t.Run("flush on mention", func(t *testing.T) {
		ctx := session.NewContext()
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "")).
			AddBlock(newTextBlock("2", ""))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		require.NoError(t, setText(tb, ctx, pb.RpcBlockTextSetTextRequest{
			BlockId: "1",
			Text:    "1",
			Marks: &model.BlockContentTextMarks{
				Marks: []*model.BlockContentTextMark{
					{
						Range: &model.Range{0, 1},
						Type:  model.BlockContentTextMark_Mention,
						Param: "blockId",
					},
				},
			},
		}))

		assert.Equal(t, "1", sb.Pick("1").Model().GetText().Text)
	})
	t.Run("on error", func(t *testing.T) {
		ctx := session.NewContext()
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "")).
			AddBlock(simple.New(&model.Block{Id: "2"}))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)
		assert.Error(t, setText(tb, ctx, pb.RpcBlockTextSetTextRequest{
			BlockId: "2",
			Text:    "",
		}))
	})
	// TODO: GO-2062 Need to review tests after text shortening refactor
	// t.Run("set text greater than limit", func(t *testing.T) {
	//	//given
	//	sb := smarttest.New("test")
	//	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
	//		AddBlock(newTextBlock("1", ""))
	//	tb := NewText(sb, nil)
	//
	//	//when
	//	err := setText(tb, nil, pb.RpcBlockTextSetTextRequest{
	//		BlockId: "1",
	//		Text:    strings.Repeat("a", textSizeLimit+1),
	//	})
	//
	//	//then
	//	assert.NoError(t, err)
	//	assert.Equal(t, strings.Repeat("a", textSizeLimit), sb.NewState().Pick("1").Model().GetText().Text)
	// })
	t.Run("carriage state is saved in history", func(t *testing.T) {
		// given
		ctx := session.NewContext()
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
			AddBlock(newTextBlock("1", ""))
		tb := NewText(sb, nil, nil)
		carriageState := undo.CarriageState{BlockID: "1", RangeFrom: 2, RangeTo: 3}

		// when
		err := setText(tb, ctx, pb.RpcBlockTextSetTextRequest{
			BlockId:           carriageState.BlockID,
			SelectedTextRange: &model.Range{From: carriageState.RangeFrom, To: carriageState.RangeTo},
		})
		tb.(*textImpl).History().Add(undo.Action{Add: []simple.Block{simple.New(&model.Block{Id: "1"})}})
		action, err := tb.(*textImpl).History().Previous()

		// then
		assert.NoError(t, err)
		assert.Equal(t, carriageState, action.CarriageInfo.Before)
	})
}

func setText(tb Text, ctx session.Context, req pb.RpcBlockTextSetTextRequest) error {
	tb.(*textImpl).Lock()
	defer tb.(*textImpl).Unlock()
	return tb.SetText(ctx, req)
}

func TestTextImpl_TurnInto(t *testing.T) {
	t.Run("common text style", func(t *testing.T) {
		ctx := session.NewContext()
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "")).
			AddBlock(newTextBlock("2", ""))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)
		require.NoError(t, tb.TurnInto(ctx, model.BlockContentText_Header4, "1", "2"))
		assert.Equal(t, model.BlockContentText_Header4, sb.Doc.Pick("1").Model().GetText().Style)
		assert.Equal(t, model.BlockContentText_Header4, sb.Doc.Pick("1").Model().GetText().Style)
	})
	t.Run("apply both for parents and children", func(t *testing.T) {
		ctx := session.NewContext()
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "", "1.1")).
			AddBlock(newTextBlock("2", "", "2.2")).
			AddBlock(newTextBlock("1.1", "")).
			AddBlock(newTextBlock("2.2", ""))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)
		require.NoError(t, tb.TurnInto(ctx, model.BlockContentText_Checkbox, "1", "1.1", "2", "2.2"))
		assert.Equal(t, model.BlockContentText_Checkbox, sb.Doc.Pick("1").Model().GetText().Style)
		assert.Equal(t, model.BlockContentText_Checkbox, sb.Doc.Pick("1").Model().GetText().Style)
		assert.Equal(t, model.BlockContentText_Checkbox, sb.Doc.Pick("1.1").Model().GetText().Style)
		assert.Equal(t, model.BlockContentText_Checkbox, sb.Doc.Pick("2.2").Model().GetText().Style)
	})
	t.Run("move children up", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "", "1.1")).
			AddBlock(newTextBlock("2", "", "2.2")).
			AddBlock(newTextBlock("1.1", "")).
			AddBlock(newTextBlock("2.2", ""))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)
		require.NoError(t, tb.TurnInto(nil, model.BlockContentText_Code, "1", "1.1", "2", "2.2"))
		assert.Equal(t, model.BlockContentText_Code, sb.Doc.Pick("1").Model().GetText().Style)
		assert.Equal(t, model.BlockContentText_Code, sb.Doc.Pick("2").Model().GetText().Style)
		assert.Equal(t, model.BlockContentText_Code, sb.Doc.Pick("1.1").Model().GetText().Style)
		assert.Equal(t, model.BlockContentText_Code, sb.Doc.Pick("2.2").Model().GetText().Style)
		assert.Equal(t, []string{"1", "1.1", "2", "2.2"}, sb.Doc.Pick("test").Model().ChildrenIds)
	})
	t.Run("turn link into text", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sb := smarttest.New("test")
		os := spaceindex.NewStoreFixture(t)
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "")).
			AddBlock(link.NewLink(&model.Block{
				Id: "2",
				Content: &model.BlockContentOfLink{
					Link: &model.BlockContentLink{
						TargetBlockId: "targetId",
					},
				},
			}))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, os, sender)

		os.AddObjects(t, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:   domain.String("targetId"),
				bundle.RelationKeyName: domain.String("link name"),
			},
		})

		require.NoError(t, tb.TurnInto(nil, model.BlockContentText_Paragraph, "2"))
		secondBlockId := sb.Doc.Pick("test").Model().ChildrenIds[1]
		assert.NotEqual(t, "2", secondBlockId)
		assert.Equal(t, "link name", sb.Doc.Pick(secondBlockId).Model().GetText().Text)
	})
}

func TestTextImpl_removeInternalFlags(t *testing.T) {
	text := "text"
	rootID := "root"
	blockID := "block"

	t.Run("text is not changed", func(t *testing.T) {
		// given
		ctx := session.NewContext()
		sb := smarttest.New(rootID)
		sb.AddBlock(simple.New(&model.Block{Id: rootID, ChildrenIds: []string{blockID}})).
			AddBlock(newTextBlock(blockID, text))
		_ = sb.SetDetails(nil, []domain.Detail{{Key: bundle.RelationKeyInternalFlags, Value: domain.Int64List([]int64{0, 1, 2})}}, false)
		tb := NewText(sb, nil, nil)

		// when
		err := setText(tb, ctx, pb.RpcBlockTextSetTextRequest{
			ContextId: rootID,
			BlockId:   blockID,
			Text:      text,
		})
		flushLocked(tb)

		// then
		assert.NoError(t, err)
		assert.Len(t, sb.Details().GetInt64List(bundle.RelationKeyInternalFlags), 3)
	})

	t.Run("text is changed", func(t *testing.T) {
		// given
		ctx := session.NewContext()
		sb := smarttest.New(rootID)
		sb.AddBlock(simple.New(&model.Block{Id: rootID, ChildrenIds: []string{blockID}})).
			AddBlock(newTextBlock(blockID, text))
		_ = sb.SetDetails(nil, []domain.Detail{{Key: bundle.RelationKeyInternalFlags, Value: domain.Int64List([]int64{0, 1, 2})}}, false)
		tb := NewText(sb, nil, nil)

		// when
		err := setText(tb, ctx, pb.RpcBlockTextSetTextRequest{
			ContextId: rootID,
			BlockId:   blockID,
			Text:      text + " is changed",
		})
		flushLocked(tb)

		// then
		assert.NoError(t, err)
		assert.Empty(t, sb.Details().GetInt64List(bundle.RelationKeyInternalFlags))
	})

	t.Run("marks are changed", func(t *testing.T) {
		// given
		ctx := session.NewContext()
		sb := smarttest.New(rootID)
		sb.AddBlock(simple.New(&model.Block{Id: rootID, ChildrenIds: []string{blockID}})).
			AddBlock(newTextBlock(blockID, text))
		_ = sb.SetDetails(nil, []domain.Detail{{Key: bundle.RelationKeyInternalFlags, Value: domain.Int64List([]int64{0, 1, 2})}}, false)
		tb := NewText(sb, nil, nil)

		// when
		err := setText(tb, ctx, pb.RpcBlockTextSetTextRequest{
			ContextId: rootID,
			BlockId:   blockID,
			Text:      text,
			Marks:     &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{{Type: model.BlockContentTextMark_Bold}}},
		})
		flushLocked(tb)

		// then
		assert.NoError(t, err)
		assert.Empty(t, sb.Details().GetInt64List(bundle.RelationKeyInternalFlags))
	})

	t.Run("title is changed", func(t *testing.T) {
		// given
		ctx := session.NewContext()
		sb := smarttest.New(rootID)
		sb.AddBlock(simple.New(&model.Block{Id: rootID, ChildrenIds: []string{template.TitleBlockId}})).
			AddBlock(newTextBlock(template.TitleBlockId, text))
		_ = sb.SetDetails(nil, []domain.Detail{{Key: bundle.RelationKeyInternalFlags, Value: domain.Int64List([]int64{0, 1, 2})}}, false)
		tb := NewText(sb, nil, nil)

		// when
		err := setText(tb, ctx, pb.RpcBlockTextSetTextRequest{
			ContextId: rootID,
			BlockId:   template.TitleBlockId,
			Text:      text + " is changed",
		})
		flushLocked(tb)

		// then
		assert.NoError(t, err)
		flags := sb.Details().GetInt64List(bundle.RelationKeyInternalFlags)
		assert.Len(t, flags, 2)
		assert.NotContains(t, flags, model.InternalFlag_editorDeleteEmpty)
	})
}

func flushLocked(tb Text) {
	tb.(*textImpl).Lock()
	tb.(*textImpl).flushSetTextState(smartblock.ApplyInfo{})
	tb.(*textImpl).Unlock()
}

func newTextBlockWithStyle(id, contentText string, style model.BlockContentTextStyle, childrenIds ...string) simple.Block {
	return text.NewText(&model.Block{
		Id: id,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  contentText,
				Style: style,
			},
		},
		ChildrenIds: childrenIds,
	})
}

func TestTextImpl_TurnIntoToggleHeader(t *testing.T) {
	t.Run("collect siblings until end", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2", "3"}})).
			AddBlock(newTextBlock("1", "header")).
			AddBlock(newTextBlock("2", "paragraph1")).
			AddBlock(newTextBlock("3", "paragraph2"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader1, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_ToggleHeader1, r.Pick("1").Model().GetText().Style)
		assert.Equal(t, []string{"2", "3"}, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"1"}, r.Pick("test").Model().ChildrenIds)
	})

	t.Run("H2 stops at H1", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2", "3", "4"}})).
			AddBlock(newTextBlock("1", "toggle header")).
			AddBlock(newTextBlock("2", "paragraph")).
			AddBlock(newTextBlockWithStyle("3", "H1 header", model.BlockContentText_Header1)).
			AddBlock(newTextBlock("4", "after header"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader2, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_ToggleHeader2, r.Pick("1").Model().GetText().Style)
		assert.Equal(t, []string{"2"}, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"1", "3", "4"}, r.Pick("test").Model().ChildrenIds)
	})

	t.Run("H2 stops at H2", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2", "3", "4"}})).
			AddBlock(newTextBlock("1", "toggle header")).
			AddBlock(newTextBlock("2", "paragraph")).
			AddBlock(newTextBlockWithStyle("3", "another H2", model.BlockContentText_Header2)).
			AddBlock(newTextBlock("4", "after header"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader2, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_ToggleHeader2, r.Pick("1").Model().GetText().Style)
		assert.Equal(t, []string{"2"}, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"1", "3", "4"}, r.Pick("test").Model().ChildrenIds)
	})

	t.Run("H2 collects H3", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2", "3", "4", "5"}})).
			AddBlock(newTextBlock("1", "toggle header H2")).
			AddBlock(newTextBlock("2", "paragraph")).
			AddBlock(newTextBlockWithStyle("3", "H3 header", model.BlockContentText_Header3)).
			AddBlock(newTextBlock("4", "after H3")).
			AddBlock(newTextBlockWithStyle("5", "H2 header", model.BlockContentText_Header2))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader2, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_ToggleHeader2, r.Pick("1").Model().GetText().Style)
		// H3 and content after it should be collected, stop at H2
		assert.Equal(t, []string{"2", "3", "4"}, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"1", "5"}, r.Pick("test").Model().ChildrenIds)
	})

	t.Run("H1 collects H2 and H3", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2", "3", "4", "5", "6"}})).
			AddBlock(newTextBlock("1", "toggle header H1")).
			AddBlock(newTextBlock("2", "paragraph")).
			AddBlock(newTextBlockWithStyle("3", "H2 header", model.BlockContentText_Header2)).
			AddBlock(newTextBlockWithStyle("4", "H3 header", model.BlockContentText_Header3)).
			AddBlock(newTextBlock("5", "more content")).
			AddBlock(newTextBlockWithStyle("6", "another H1", model.BlockContentText_Header1))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader1, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_ToggleHeader1, r.Pick("1").Model().GetText().Style)
		// H2, H3 and all content should be collected, stop at H1
		assert.Equal(t, []string{"2", "3", "4", "5"}, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"1", "6"}, r.Pick("test").Model().ChildrenIds)
	})

	t.Run("H3 stops at toggle H1", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2", "3", "4"}})).
			AddBlock(newTextBlock("1", "toggle header")).
			AddBlock(newTextBlock("2", "paragraph")).
			AddBlock(newTextBlockWithStyle("3", "toggle H1", model.BlockContentText_ToggleHeader1)).
			AddBlock(newTextBlock("4", "after toggle header"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader3, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_ToggleHeader3, r.Pick("1").Model().GetText().Style)
		assert.Equal(t, []string{"2"}, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"1", "3", "4"}, r.Pick("test").Model().ChildrenIds)
	})

	t.Run("no siblings to collect", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
			AddBlock(newTextBlock("1", "header"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader1, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_ToggleHeader1, r.Pick("1").Model().GetText().Style)
		assert.Empty(t, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"1"}, r.Pick("test").Model().ChildrenIds)
	})

	t.Run("preserve existing children", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "header", "existing-child")).
			AddBlock(newTextBlock("existing-child", "existing")).
			AddBlock(newTextBlock("2", "sibling"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader1, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_ToggleHeader1, r.Pick("1").Model().GetText().Style)
		assert.Equal(t, []string{"existing-child", "2"}, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"1"}, r.Pick("test").Model().ChildrenIds)
	})

	t.Run("last block becomes toggle header", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "paragraph")).
			AddBlock(newTextBlock("2", "last block"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader1, "2")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_ToggleHeader1, r.Pick("2").Model().GetText().Style)
		assert.Empty(t, r.Pick("2").Model().ChildrenIds)
		assert.Equal(t, []string{"1", "2"}, r.Pick("test").Model().ChildrenIds)
	})

	t.Run("collect non-text blocks as well", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2", "3", "4"}})).
			AddBlock(newTextBlock("1", "header")).
			AddBlock(newTextBlock("2", "text")).
			AddBlock(simple.New(&model.Block{Id: "3", Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}})).
			AddBlock(newTextBlock("4", "more text"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader1, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_ToggleHeader1, r.Pick("1").Model().GetText().Style)
		assert.Equal(t, []string{"2", "3", "4"}, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"1"}, r.Pick("test").Model().ChildrenIds)
	})
}

func TestTextImpl_TurnIntoHeaderFromToggleHeader(t *testing.T) {
	t.Run("toggle header with children becomes header, children moved as siblings", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
			AddBlock(newTextBlockWithStyle("1", "toggle header", model.BlockContentText_ToggleHeader1, "2", "3")).
			AddBlock(newTextBlock("2", "child1")).
			AddBlock(newTextBlock("3", "child2"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_Header1, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_Header1, r.Pick("1").Model().GetText().Style)
		assert.Empty(t, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"1", "2", "3"}, r.Pick("test").Model().ChildrenIds)
	})

	t.Run("toggle header 2 with children becomes header 2", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"0", "1", "4"}})).
			AddBlock(newTextBlock("0", "before")).
			AddBlock(newTextBlockWithStyle("1", "toggle header 2", model.BlockContentText_ToggleHeader2, "2", "3")).
			AddBlock(newTextBlock("2", "child1")).
			AddBlock(newTextBlock("3", "child2")).
			AddBlock(newTextBlock("4", "after"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_Header2, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_Header2, r.Pick("1").Model().GetText().Style)
		assert.Empty(t, r.Pick("1").Model().ChildrenIds)
		// children should be placed right after the header, before "4"
		assert.Equal(t, []string{"0", "1", "2", "3", "4"}, r.Pick("test").Model().ChildrenIds)
	})

	t.Run("toggle header 3 with children becomes header 3", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
			AddBlock(newTextBlockWithStyle("1", "toggle header 3", model.BlockContentText_ToggleHeader3, "2")).
			AddBlock(newTextBlock("2", "only child"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_Header3, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_Header3, r.Pick("1").Model().GetText().Style)
		assert.Empty(t, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"1", "2"}, r.Pick("test").Model().ChildrenIds)
	})

	t.Run("toggle header without children becomes header without issues", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
			AddBlock(newTextBlockWithStyle("1", "toggle header", model.BlockContentText_ToggleHeader1))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_Header1, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_Header1, r.Pick("1").Model().GetText().Style)
		assert.Empty(t, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"1"}, r.Pick("test").Model().ChildrenIds)
	})

	t.Run("toggle header with nested children becomes header, all children unindented", func(t *testing.T) {
		// given: toggle header "1" has children "2" and "3", and "2" itself has child "2a"
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
			AddBlock(newTextBlockWithStyle("1", "toggle header", model.BlockContentText_ToggleHeader1, "2", "3")).
			AddBlock(newTextBlock("2", "child with nested", "2a")).
			AddBlock(newTextBlock("2a", "nested child")).
			AddBlock(newTextBlock("3", "child2"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_Header1, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_Header1, r.Pick("1").Model().GetText().Style)
		assert.Empty(t, r.Pick("1").Model().ChildrenIds)
		// Only direct children are moved up; nested children stay with their parent
		assert.Equal(t, []string{"1", "2", "3"}, r.Pick("test").Model().ChildrenIds)
		assert.Equal(t, []string{"2a"}, r.Pick("2").Model().ChildrenIds)
	})

	t.Run("toggle header to different header level moves children", func(t *testing.T) {
		// given: ToggleHeader1 with children is turned into Header3
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
			AddBlock(newTextBlockWithStyle("1", "toggle header 1", model.BlockContentText_ToggleHeader1, "2", "3")).
			AddBlock(newTextBlock("2", "child1")).
			AddBlock(newTextBlock("3", "child2"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_Header3, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_Header3, r.Pick("1").Model().GetText().Style)
		assert.Empty(t, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"1", "2", "3"}, r.Pick("test").Model().ChildrenIds)
	})

	t.Run("toggle header to paragraph keeps children", func(t *testing.T) {
		// given: ToggleHeader1 with children is turned into Paragraph (which CAN have children)
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
			AddBlock(newTextBlockWithStyle("1", "toggle header", model.BlockContentText_ToggleHeader1, "2", "3")).
			AddBlock(newTextBlock("2", "child1")).
			AddBlock(newTextBlock("3", "child2"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_Paragraph, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_Paragraph, r.Pick("1").Model().GetText().Style)
		// Paragraph CAN have children, so they should remain
		assert.Equal(t, []string{"2", "3"}, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"1"}, r.Pick("test").Model().ChildrenIds)
	})

	t.Run("toggle header to code moves children", func(t *testing.T) {
		// given: Code blocks also cannot have children
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
			AddBlock(newTextBlockWithStyle("1", "toggle header", model.BlockContentText_ToggleHeader1, "2")).
			AddBlock(newTextBlock("2", "child"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_Code, "1")

		// then
		require.NoError(t, err)
		r := sb.NewState()
		assert.Equal(t, model.BlockContentText_Code, r.Pick("1").Model().GetText().Style)
		assert.Empty(t, r.Pick("1").Model().ChildrenIds)
		assert.Equal(t, []string{"1", "2"}, r.Pick("test").Model().ChildrenIds)
	})
}

func newLayoutDivBlock(id string, childrenIds ...string) simple.Block {
	return simple.New(&model.Block{
		Id:          id,
		ChildrenIds: childrenIds,
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Div,
			},
		},
	})
}

func TestTextImpl_TurnIntoToggleHeaderWithDivBlocks(t *testing.T) {
	t.Run("collect from current div and sibling divs", func(t *testing.T) {
		// Tree Before:
		// root
		// -div1
		// --1
		// --H1 (converting to TH1)
		// --2
		// -div2
		// --3
		// --4
		// --H1-stop (this stops collection)
		// --5
		//
		// Tree After:
		// root
		// -div1
		// --1
		// --TH1
		// ---2
		// ---3
		// ---4
		// -div2
		// --H1-stop
		// --5

		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"div1", "div2"}})).
			AddBlock(newLayoutDivBlock("div1", "1", "H1", "2")).
			AddBlock(newLayoutDivBlock("div2", "3", "4", "H1-stop", "5")).
			AddBlock(newTextBlock("1", "before header")).
			AddBlock(newTextBlock("H1", "header to convert")).
			AddBlock(newTextBlock("2", "after header in div1")).
			AddBlock(newTextBlock("3", "first in div2")).
			AddBlock(newTextBlock("4", "second in div2")).
			AddBlock(newTextBlockWithStyle("H1-stop", "stopping header", model.BlockContentText_Header1)).
			AddBlock(newTextBlock("5", "after stopping header"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader1, "H1")

		// then
		require.NoError(t, err)
		r := sb.NewState()

		// Check toggle header style
		assert.Equal(t, model.BlockContentText_ToggleHeader1, r.Pick("H1").Model().GetText().Style)

		// Blocks are direct children of toggle header (no wrapper divs)
		assert.Equal(t, []string{"2", "3", "4"}, r.Pick("H1").Model().ChildrenIds)

		// Check div1 structure: should contain "1" and "H1" (toggle header)
		assert.Equal(t, []string{"1", "H1"}, r.Pick("div1").Model().ChildrenIds)

		// Check div2 structure: should contain "H1-stop" and "5"
		assert.Equal(t, []string{"H1-stop", "5"}, r.Pick("div2").Model().ChildrenIds)
	})

	t.Run("collect entire sibling div when no stopping header", func(t *testing.T) {
		// Tree Before:
		// root
		// -div1
		// --H1
		// --2
		// -div2
		// --3
		// --4
		//
		// Tree After:
		// root
		// -div1
		// --TH1
		// ---2
		// ---3
		// ---4
		// (div2 is removed as it becomes empty)

		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"div1", "div2"}})).
			AddBlock(newLayoutDivBlock("div1", "H1", "2")).
			AddBlock(newLayoutDivBlock("div2", "3", "4")).
			AddBlock(newTextBlock("H1", "header to convert")).
			AddBlock(newTextBlock("2", "after header")).
			AddBlock(newTextBlock("3", "first in div2")).
			AddBlock(newTextBlock("4", "second in div2"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader1, "H1")

		// then
		require.NoError(t, err)
		r := sb.NewState()

		// Blocks are direct children of toggle header
		assert.Equal(t, []string{"2", "3", "4"}, r.Pick("H1").Model().ChildrenIds)

		// div1 should only contain the toggle header
		assert.Equal(t, []string{"H1"}, r.Pick("div1").Model().ChildrenIds)

		// div2 should be unlinked (empty)
		assert.False(t, r.Exists("div2") && len(r.Pick("div2").Model().ChildrenIds) > 0)
	})

	t.Run("stop at header in current div", func(t *testing.T) {
		// Tree Before:
		// root
		// -div1
		// --H1
		// --2
		// --H1-stop
		// --3
		// -div2
		// --4
		//
		// Tree After:
		// root
		// -div1
		// --TH1
		// ---2
		// --H1-stop
		// --3
		// -div2
		// --4

		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"div1", "div2"}})).
			AddBlock(newLayoutDivBlock("div1", "H1", "2", "H1-stop", "3")).
			AddBlock(newLayoutDivBlock("div2", "4")).
			AddBlock(newTextBlock("H1", "header to convert")).
			AddBlock(newTextBlock("2", "content")).
			AddBlock(newTextBlockWithStyle("H1-stop", "stopping header", model.BlockContentText_Header1)).
			AddBlock(newTextBlock("3", "after stop")).
			AddBlock(newTextBlock("4", "in div2"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader1, "H1")

		// then
		require.NoError(t, err)
		r := sb.NewState()

		// Block "2" is a direct child of toggle header
		assert.Equal(t, []string{"2"}, r.Pick("H1").Model().ChildrenIds)

		// div1 should contain TH1, H1-stop, 3
		assert.Equal(t, []string{"H1", "H1-stop", "3"}, r.Pick("div1").Model().ChildrenIds)

		// div2 should be unchanged
		assert.Equal(t, []string{"4"}, r.Pick("div2").Model().ChildrenIds)
	})

	t.Run("no content to collect in div", func(t *testing.T) {
		// Tree Before:
		// root
		// -div1
		// --H1
		// -div2
		// --H1-stop
		//
		// Tree After: (unchanged except style)
		// root
		// -div1
		// --TH1
		// -div2
		// --H1-stop

		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"div1", "div2"}})).
			AddBlock(newLayoutDivBlock("div1", "H1")).
			AddBlock(newLayoutDivBlock("div2", "H1-stop")).
			AddBlock(newTextBlock("H1", "header to convert")).
			AddBlock(newTextBlockWithStyle("H1-stop", "stopping header", model.BlockContentText_Header1))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader1, "H1")

		// then
		require.NoError(t, err)
		r := sb.NewState()

		// Toggle header should have no children
		assert.Empty(t, r.Pick("H1").Model().ChildrenIds)

		// Structure should be preserved
		assert.Equal(t, []string{"H1"}, r.Pick("div1").Model().ChildrenIds)
		assert.Equal(t, []string{"H1-stop"}, r.Pick("div2").Model().ChildrenIds)
	})

	t.Run("H2 in div collects H3 but stops at H2", func(t *testing.T) {
		// Test header level logic with div blocks

		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"div1", "div2"}})).
			AddBlock(newLayoutDivBlock("div1", "H2", "content1")).
			AddBlock(newLayoutDivBlock("div2", "H3", "content2", "H2-stop", "content3")).
			AddBlock(newTextBlock("H2", "H2 to convert")).
			AddBlock(newTextBlock("content1", "content")).
			AddBlock(newTextBlockWithStyle("H3", "H3 header", model.BlockContentText_Header3)).
			AddBlock(newTextBlock("content2", "after H3")).
			AddBlock(newTextBlockWithStyle("H2-stop", "stopping H2", model.BlockContentText_Header2)).
			AddBlock(newTextBlock("content3", "after stop"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader2, "H2")

		// then
		require.NoError(t, err)
		r := sb.NewState()

		// Blocks are direct children: content1 from div1, H3 and content2 from div2
		assert.Equal(t, []string{"content1", "H3", "content2"}, r.Pick("H2").Model().ChildrenIds)

		// div2 should have H2-stop and content3
		assert.Equal(t, []string{"H2-stop", "content3"}, r.Pick("div2").Model().ChildrenIds)
	})

	t.Run("collect from many sibling divs", func(t *testing.T) {
		// Tree Before:
		// root
		// -div1
		// --H1 (converting to TH1)
		// --1
		// -div2
		// --2
		// -div3
		// --3
		// -div4
		// --4
		// -div5
		// --5
		// -div6
		// --6
		//
		// Tree After:
		// root
		// -div1
		// --TH1
		// ---1
		// ---2
		// ---3
		// ---4
		// ---5
		// ---6
		// (div2-div6 are removed as they become empty)

		// given
		sb := smarttest.New("root")
		sb.AddBlock(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"div1", "div2", "div3", "div4", "div5", "div6"}})).
			AddBlock(newLayoutDivBlock("div1", "H1", "1")).
			AddBlock(newLayoutDivBlock("div2", "2")).
			AddBlock(newLayoutDivBlock("div3", "3")).
			AddBlock(newLayoutDivBlock("div4", "4")).
			AddBlock(newLayoutDivBlock("div5", "5")).
			AddBlock(newLayoutDivBlock("div6", "6")).
			AddBlock(newTextBlock("H1", "header to convert")).
			AddBlock(newTextBlock("1", "in div1")).
			AddBlock(newTextBlock("2", "in div2")).
			AddBlock(newTextBlock("3", "in div3")).
			AddBlock(newTextBlock("4", "in div4")).
			AddBlock(newTextBlock("5", "in div5")).
			AddBlock(newTextBlock("6", "in div6"))
		sender := mock_event.NewMockSender(t)
		tb := NewText(sb, nil, sender)

		// when
		err := tb.TurnInto(nil, model.BlockContentText_ToggleHeader1, "H1")

		// then
		require.NoError(t, err)
		r := sb.NewState()

		assert.Equal(t, model.BlockContentText_ToggleHeader1, r.Pick("H1").Model().GetText().Style)
		assert.Equal(t, []string{"1", "2", "3", "4", "5", "6"}, r.Pick("H1").Model().ChildrenIds)
		assert.Equal(t, []string{"H1"}, r.Pick("div1").Model().ChildrenIds)
	})
}
