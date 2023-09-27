package stext

import (
	"github.com/anyproto/anytype-heart/core/block/undo"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/link"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/testMock"
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

	tb := NewText(sb, nil)
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
		tb := NewText(sb, nil)
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
		tb := NewText(sb, nil)
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
		tb := NewText(sb, nil)
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

		tb := NewText(sb, nil)
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
		//given
		sb := smarttest.New("test")
		sb.AddBlock(
			simple.New(
				&model.Block{
					Id:          "test",
					ChildrenIds: []string{"1"},
				},
			),
		).AddBlock(newCodeBlock("1", "onetwo"))
		tb := NewText(sb, nil)

		//when
		newId, err := tb.Split(nil, pb.RpcBlockSplitRequest{
			BlockId: "1",
			Range:   &model.Range{From: 3, To: 3},
			Style:   model.BlockContentText_Checkbox,
			Mode:    pb.RpcBlockSplitRequest_BOTTOM,
		})

		//then
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
		tb := NewText(sb, nil)

		err := tb.Merge(nil, "1", "2")
		require.NoError(t, err)

		r := sb.NewState()
		assert.False(t, r.Exists("2"))
		require.True(t, r.Exists("1"))

		assert.Equal(t, "onetwo", r.Pick("1").Model().GetText().Text)
		assert.Equal(t, []string{"ch1", "ch2"}, r.Pick("1").Model().ChildrenIds)
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

		tb := NewText(sb, nil)

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

		tb := NewText(sb, nil)

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
		tb := NewText(sb, nil)
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
		tb := NewText(sb, nil)
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
		tb := NewText(sb, nil)
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
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", " ")).
			AddBlock(newTextBlock("2", " "))
		tb := NewText(sb, nil)

		require.NoError(t, setText(tb, nil, pb.RpcBlockTextSetTextRequest{
			BlockId: "1",
			Text:    "1",
		}))
		require.NoError(t, setText(tb, nil, pb.RpcBlockTextSetTextRequest{
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
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", " ")).
			AddBlock(newTextBlock("2", " "))
		tb := NewText(sb, nil)

		require.NoError(t, setText(tb, nil, pb.RpcBlockTextSetTextRequest{
			BlockId: "1",
			Text:    "1",
		}))
		require.NoError(t, setText(tb, nil, pb.RpcBlockTextSetTextRequest{
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
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "")).
			AddBlock(newTextBlock("2", ""))
		tb := NewText(sb, nil)

		require.NoError(t, setText(tb, nil, pb.RpcBlockTextSetTextRequest{
			BlockId: "1",
			Text:    "1",
		}))
		tb.(*textImpl).Lock()
		tb.(*textImpl).flushSetTextState(smartblock.ApplyInfo{})
		tb.(*textImpl).Unlock()
		require.NoError(t, setText(tb, nil, pb.RpcBlockTextSetTextRequest{
			BlockId: "2",
			Text:    "2",
		}))
		tb.(*textImpl).Lock()
		tb.(*textImpl).flushSetTextState(smartblock.ApplyInfo{})
		tb.(*textImpl).Unlock()
		assert.Equal(t, "1", sb.NewState().Pick("1").Model().GetText().Text)
		assert.Equal(t, "2", sb.NewState().Pick("2").Model().GetText().Text)
		time.Sleep(time.Second)
		assert.Len(t, sb.Results.Applies, 2)
	})
	t.Run("flush on mention", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "")).
			AddBlock(newTextBlock("2", ""))
		tb := NewText(sb, nil)

		require.NoError(t, setText(tb, nil, pb.RpcBlockTextSetTextRequest{
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
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "")).
			AddBlock(simple.New(&model.Block{Id: "2"}))
		tb := NewText(sb, nil)
		assert.Error(t, setText(tb, nil, pb.RpcBlockTextSetTextRequest{
			BlockId: "2",
			Text:    "",
		}))
	})
	// TODO: GO-2062 Need to review tests after text shortening refactor
	//t.Run("set text greater than limit", func(t *testing.T) {
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
	//})
	t.Run("carriage info is saved in history", func(t *testing.T) {
		//given
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1"}})).
			AddBlock(newTextBlock("1", ""))
		tb := NewText(sb, nil)
		info := undo.CarriageInfo{CarriageBlockID: "1", RangeFrom: 2, RangeTo: 3}

		//when
		err := setText(tb, nil, pb.RpcBlockTextSetTextRequest{
			BlockId:           info.CarriageBlockID,
			SelectedTextRange: &model.Range{From: info.RangeFrom, To: info.RangeTo},
		})
		tb.(*textImpl).History().Add(undo.Action{Add: []simple.Block{simple.New(&model.Block{Id: "1"})}})
		action, err := tb.(*textImpl).History().Previous()

		//then
		assert.NoError(t, err)
		assert.Equal(t, info, action.CarriageInfo)
	})
}

func setText(tb Text, ctx *session.Context, req pb.RpcBlockTextSetTextRequest) error {
	tb.(*textImpl).Lock()
	defer tb.(*textImpl).Unlock()
	return tb.SetText(ctx, req)
}

func TestTextImpl_TurnInto(t *testing.T) {
	t.Run("common text style", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "")).
			AddBlock(newTextBlock("2", ""))
		tb := NewText(sb, nil)
		require.NoError(t, tb.TurnInto(nil, model.BlockContentText_Header4, "1", "2"))
		assert.Equal(t, model.BlockContentText_Header4, sb.Doc.Pick("1").Model().GetText().Style)
		assert.Equal(t, model.BlockContentText_Header4, sb.Doc.Pick("1").Model().GetText().Style)
	})
	t.Run("apply only for parents", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "", "1.1")).
			AddBlock(newTextBlock("2", "", "2.2")).
			AddBlock(newTextBlock("1.1", "")).
			AddBlock(newTextBlock("2.2", ""))
		tb := NewText(sb, nil)
		require.NoError(t, tb.TurnInto(nil, model.BlockContentText_Checkbox, "1", "1.1", "2", "2.2"))
		assert.Equal(t, model.BlockContentText_Checkbox, sb.Doc.Pick("1").Model().GetText().Style)
		assert.Equal(t, model.BlockContentText_Checkbox, sb.Doc.Pick("1").Model().GetText().Style)
		assert.NotEqual(t, model.BlockContentText_Checkbox, sb.Doc.Pick("1.1").Model().GetText().Style)
		assert.NotEqual(t, model.BlockContentText_Checkbox, sb.Doc.Pick("2.2").Model().GetText().Style)
	})
	t.Run("move children up", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"1", "2"}})).
			AddBlock(newTextBlock("1", "", "1.1")).
			AddBlock(newTextBlock("2", "", "2.2")).
			AddBlock(newTextBlock("1.1", "")).
			AddBlock(newTextBlock("2.2", ""))
		tb := NewText(sb, nil)
		require.NoError(t, tb.TurnInto(nil, model.BlockContentText_Code, "1", "1.1", "2", "2.2"))
		assert.Equal(t, model.BlockContentText_Code, sb.Doc.Pick("1").Model().GetText().Style)
		assert.Equal(t, model.BlockContentText_Code, sb.Doc.Pick("2").Model().GetText().Style)
		assert.Equal(t, model.BlockContentText_Paragraph, sb.Doc.Pick("1.1").Model().GetText().Style)
		assert.Equal(t, model.BlockContentText_Paragraph, sb.Doc.Pick("2.2").Model().GetText().Style)
		assert.Equal(t, []string{"1", "1.1", "2", "2.2"}, sb.Doc.Pick("test").Model().ChildrenIds)
	})
	t.Run("turn link into text", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		sb := smarttest.New("test")
		os := testMock.NewMockObjectStore(ctrl)
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
		tb := NewText(sb, os)

		os.EXPECT().QueryByID([]string{"targetId"}).Return([]database.Record{
			{
				Details: &types.Struct{
					Fields: map[string]*types.Value{
						"name": pbtypes.String("link name"),
					},
				},
			},
		}, nil)

		require.NoError(t, tb.TurnInto(nil, model.BlockContentText_Paragraph, "2"))
		secondBlockId := sb.Doc.Pick("test").Model().ChildrenIds[1]
		assert.NotEqual(t, "2", secondBlockId)
		assert.Equal(t, "link name", sb.Doc.Pick(secondBlockId).Model().GetText().Text)
	})
}
