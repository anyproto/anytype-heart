package clipboard

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func markedBlock(id string, txt string, marks ...*model.BlockContentTextMark) *model.Block {
	return &model.Block{
		Id: id,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  txt,
				Marks: &model.BlockContentTextMarks{Marks: marks},
			},
		},
	}
}

// grin is U+1F600, a single rune that takes two UTF-16 code units
const grin = "\U0001F600"

func TestSanitizeMarks(t *testing.T) {
	boldAt := func(from, to int32) *model.BlockContentTextMark {
		return &model.BlockContentTextMark{
			Type:  model.BlockContentTextMark_Bold,
			Range: &model.Range{From: from, To: to},
		}
	}
	linkAt := func(from, to int32) *model.BlockContentTextMark {
		return &model.BlockContentTextMark{
			Type:  model.BlockContentTextMark_Link,
			Param: "https://example.com/x",
			Range: &model.Range{From: from, To: to},
		}
	}

	for _, tc := range []struct {
		name  string
		text  string
		marks []*model.BlockContentTextMark
		want  []*model.BlockContentTextMark
	}{
		// unrepairable - dropped
		{
			name:  "mark without range is dropped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{{Type: model.BlockContentTextMark_Bold}},
			want:  []*model.BlockContentTextMark{},
		},
		{
			name:  "nil mark is dropped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{nil},
			want:  []*model.BlockContentTextMark{},
		},
		{
			name:  "reversed range is dropped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(4, 2)},
			want:  []*model.BlockContentTextMark{},
		},
		{
			// only the reversed-range rule rejects this one, range.from is valid on its own
			name:  "negative range.to is dropped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(0, -1)},
			want:  []*model.BlockContentTextMark{},
		},
		{
			name:  "range entirely before the text is dropped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(-3, -1)},
			want:  []*model.BlockContentTextMark{},
		},
		{
			name:  "range entirely past the end of the text is dropped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(7, 9)},
			want:  []*model.BlockContentTextMark{},
		},
		{
			name:  "range starting one past the end of the text is dropped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(6, 6)},
			want:  []*model.BlockContentTextMark{},
		},

		// repairable - clipped, mark and param kept
		{
			name:  "range overrunning the text is clipped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{linkAt(0, 6)},
			want:  []*model.BlockContentTextMark{linkAt(0, 5)},
		},
		{
			name:  "range starting before the text is clipped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{linkAt(-1, 3)},
			want:  []*model.BlockContentTextMark{linkAt(0, 3)},
		},
		{
			name:  "range overrunning the text on both ends is clipped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{linkAt(-2, 99)},
			want:  []*model.BlockContentTextMark{linkAt(0, 5)},
		},

		// already applicable - kept untouched
		{
			name:  "mark covering the whole text is kept",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(0, 5)},
			want:  []*model.BlockContentTextMark{boldAt(0, 5)},
		},
		{
			name:  "interior zero width range is kept",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(2, 2)},
			want:  []*model.BlockContentTextMark{boldAt(2, 2)},
		},
		{
			name:  "zero width range at the end of the text is kept",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(5, 5)},
			want:  []*model.BlockContentTextMark{boldAt(5, 5)},
		},
		{
			// this is what SetMarkForAllText produces on an empty block
			name:  "zero width range on empty text is kept",
			text:  "",
			marks: []*model.BlockContentTextMark{boldAt(0, 0)},
			want:  []*model.BlockContentTextMark{boldAt(0, 0)},
		},
		{
			name:  "only the unrepairable mark is dropped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(0, 2), {Type: model.BlockContentTextMark_Italic}, boldAt(3, 5)},
			want:  []*model.BlockContentTextMark{boldAt(0, 2), boldAt(3, 5)},
		},

		// UTF-16: the emoji is 1 rune / 4 bytes / 2 UTF-16 code units
		{
			name:  "mark covering a surrogate pair is kept",
			text:  grin,
			marks: []*model.BlockContentTextMark{boldAt(0, 2)},
			want:  []*model.BlockContentTextMark{boldAt(0, 2)},
		},
		{
			name:  "mark past a surrogate pair is clipped to the UTF-16 length",
			text:  grin,
			marks: []*model.BlockContentTextMark{boldAt(0, 3)},
			want:  []*model.BlockContentTextMark{boldAt(0, 2)},
		},
		{
			name:  "mark starting inside a surrogate pair is kept",
			text:  grin,
			marks: []*model.BlockContentTextMark{boldAt(1, 2)},
			want:  []*model.BlockContentTextMark{boldAt(1, 2)},
		},
		{
			name:  "clipping uses UTF-16 units not bytes or runes",
			text:  grin + "a",
			marks: []*model.BlockContentTextMark{boldAt(0, 4)},
			want:  []*model.BlockContentTextMark{boldAt(0, 3)},
		},
		{
			name:  "range starting past the end measured in UTF-16 units is dropped",
			text:  grin,
			marks: []*model.BlockContentTextMark{boldAt(3, 4)},
			want:  []*model.BlockContentTextMark{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			b := markedBlock("b", tc.text, tc.marks...)

			// when
			sanitizeMarks(b)

			// then
			assert.Equal(t, tc.want, b.GetText().Marks.Marks)
			assert.Equal(t, tc.text, b.GetText().Text)
		})
	}

	t.Run("block without marks container is left alone", func(t *testing.T) {
		// given
		b := &model.Block{Id: "b", Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{Text: "hello"},
		}}

		// when
		sanitizeMarks(b)

		// then
		require.Nil(t, b.GetText().Marks)
	})

	t.Run("non-text block is left alone", func(t *testing.T) {
		// given
		b := &model.Block{Id: "b", Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}}

		// when
		sanitizeMarks(b)

		// then
		require.Nil(t, b.GetText())
	})
}

func TestPasteAny_InvalidMarks(t *testing.T) {
	t.Run("mark without range does not reach the document", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
		cb := newFixture(t, sb)
		pasted := markedBlock("pasted", "hello", &model.BlockContentTextMark{Type: model.BlockContentTextMark_Bold})

		// when
		ids, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{AnySlot: []*model.Block{pasted}}, "")

		// then
		require.NoError(t, err)
		require.Len(t, ids, 1)
		stored := sb.Pick(ids[0]).Model().GetText()
		assert.Equal(t, "hello", stored.Text)
		assert.Empty(t, stored.Marks.Marks)
	})

	t.Run("block stays editable after pasting a mark without range", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
		ids, _, _, _, err := newFixture(t, sb).Paste(nil, &pb.RpcBlockPasteRequest{
			AnySlot: []*model.Block{markedBlock("pasted", "hello", &model.BlockContentTextMark{Type: model.BlockContentTextMark_Bold})},
		}, "")
		require.NoError(t, err)
		require.Len(t, ids, 1)

		// when: the user edits that block afterwards
		_, _, _, _, err = newFixture(t, sb).Paste(nil, &pb.RpcBlockPasteRequest{
			FocusedBlockId:    ids[0],
			SelectedTextRange: &model.Range{From: 1, To: 3},
			IsPartOfBlock:     true,
			AnySlot:           []*model.Block{markedBlock("x", "ZZ")},
		}, "")

		// then
		require.NoError(t, err)
		assert.Equal(t, "hZZlo", sb.Pick(ids[0]).Model().GetText().Text)
	})

	t.Run("two marks without range do not panic and do not reach the document", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
		cb := newFixture(t, sb)
		pasted := markedBlock("pasted", "hello",
			&model.BlockContentTextMark{Type: model.BlockContentTextMark_Bold},
			&model.BlockContentTextMark{Type: model.BlockContentTextMark_Bold},
		)

		// when
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			FocusedBlockId:    "1",
			SelectedTextRange: &model.Range{From: 4, To: 4},
			IsPartOfBlock:     true,
			AnySlot:           []*model.Block{pasted},
		}, "")

		// then
		require.NoError(t, err)
		assert.Equal(t, "aaaahello", sb.Pick("1").Model().GetText().Text)
		assert.Empty(t, sb.Pick("1").Model().GetText().Marks.Marks)
	})

	t.Run("clipped mark reaches the document with its range and param", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
		cb := newFixture(t, sb)
		want := []*model.BlockContentTextMark{{
			Type:  model.BlockContentTextMark_Link,
			Param: "https://example.com/x",
			Range: &model.Range{From: 0, To: 5},
		}}

		// when
		ids, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			AnySlot: []*model.Block{markedBlock("pasted", "hello", &model.BlockContentTextMark{
				Type:  model.BlockContentTextMark_Link,
				Param: "https://example.com/x",
				Range: &model.Range{From: -1, To: 99},
			})},
		}, "")

		// then
		require.NoError(t, err)
		require.Len(t, ids, 1)
		assert.Equal(t, want, sb.Pick(ids[0]).Model().GetText().Marks.Marks)
	})

	t.Run("every block of the slot is sanitized, not only the first", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
		cb := newFixture(t, sb)
		first := markedBlock("first", "hello", &model.BlockContentTextMark{
			Type: model.BlockContentTextMark_Bold, Range: &model.Range{From: 0, To: 5},
		})
		second := markedBlock("second", "world", &model.BlockContentTextMark{
			Type: model.BlockContentTextMark_Bold, // no range
		})

		// when
		ids, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			AnySlot: []*model.Block{first, second},
		}, "")

		// then
		require.NoError(t, err)
		require.Len(t, ids, 2)
		assert.Len(t, sb.Pick(ids[0]).Model().GetText().Marks.Marks, 1)
		assert.Empty(t, sb.Pick(ids[1]).Model().GetText().Marks.Marks)
	})

	t.Run("nested child blocks are sanitized too", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
		cb := newFixture(t, sb)
		parent := markedBlock("parent", "parent text")
		parent.ChildrenIds = []string{"child"}
		child := markedBlock("child", "child text", &model.BlockContentTextMark{
			Type: model.BlockContentTextMark_Bold, // no range
		})

		// when
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			AnySlot: []*model.Block{parent, child},
		}, "")

		// then
		require.NoError(t, err)
		var childMarks []*model.BlockContentTextMark
		for _, id := range sb.Pick("test").Model().ChildrenIds {
			collectMarksOf(sb, id, "child text", &childMarks)
		}
		assert.Empty(t, childMarks, "the mark without a range must not reach the nested block")
	})

	t.Run("param bearing marks keep their param through the paste", func(t *testing.T) {
		// given: params carry the colour, the mention target and the object target,
		// so losing one silently changes what the mark points at or looks like
		sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
		cb := newFixture(t, sb)
		want := []*model.BlockContentTextMark{
			{Type: model.BlockContentTextMark_TextColor, Param: "red", Range: &model.Range{From: 0, To: 2}},
			{Type: model.BlockContentTextMark_BackgroundColor, Param: "blue", Range: &model.Range{From: 2, To: 4}},
			{Type: model.BlockContentTextMark_Mention, Param: "mentionedObjectId", Range: &model.Range{From: 4, To: 6}},
			{Type: model.BlockContentTextMark_Object, Param: "targetObjectId", Range: &model.Range{From: 6, To: 8}},
		}

		// when
		ids, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			AnySlot: []*model.Block{markedBlock("pasted", "hello123", want...)},
		}, "")

		// then
		require.NoError(t, err)
		require.Len(t, ids, 1)
		got := sb.Pick(ids[0]).Model().GetText().Marks.Marks
		require.Len(t, got, len(want))
		for i, w := range want {
			assert.Equal(t, w.Type, got[i].Type)
			assert.Equal(t, w.Param, got[i].Param, "param of the %s mark must survive", w.Type)
			assert.Equal(t, w.Range, got[i].Range)
		}
	})

	t.Run("param survives when the mark range is clipped", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
		cb := newFixture(t, sb)
		want := []*model.BlockContentTextMark{{
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
			Range: &model.Range{From: 0, To: 5},
		}}

		// when
		ids, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			AnySlot: []*model.Block{markedBlock("pasted", "hello", &model.BlockContentTextMark{
				Type:  model.BlockContentTextMark_TextColor,
				Param: "red",
				Range: &model.Range{From: -2, To: 99},
			})},
		}, "")

		// then
		require.NoError(t, err)
		require.Len(t, ids, 1)
		assert.Equal(t, want, sb.Pick(ids[0]).Model().GetText().Marks.Marks)
	})

	t.Run("valid mark survives the paste", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
		cb := newFixture(t, sb)
		want := []*model.BlockContentTextMark{{
			Type:  model.BlockContentTextMark_Bold,
			Range: &model.Range{From: 0, To: 5},
		}}

		// when
		ids, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			AnySlot: []*model.Block{markedBlock("pasted", "hello", want[0])},
		}, "")

		// then
		require.NoError(t, err)
		require.Len(t, ids, 1)
		assert.Equal(t, want, sb.Pick(ids[0]).Model().GetText().Marks.Marks)
	})
}

func TestPasteAny_MalformedBlocks(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block *model.Block
	}{
		{
			name:  "title block without fields",
			block: withId(markedBlock("", "hello"), template.TitleBlockId),
		},
		{
			name:  "description block without fields",
			block: withId(markedBlock("", "hello"), template.DescriptionBlockId),
		},
		{
			name:  "text content without text",
			block: &model.Block{Id: "t", Content: &model.BlockContentOfText{}},
		},
		{
			name:  "block without content",
			block: &model.Block{Id: "c"},
		},
		{
			name:  "dataview block without dataview content",
			block: &model.Block{Id: "d", Content: &model.BlockContentOfDataview{}},
		},
		{
			name:  "file block without file content",
			block: &model.Block{Id: "f", Content: &model.BlockContentOfFile{}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
			cb := newFixture(t, sb)

			// when
			_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{AnySlot: []*model.Block{tc.block}}, "")

			// then
			require.NoError(t, err)
		})
	}

	t.Run("nil block in any slot", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
		cb := newFixture(t, sb)

		// when
		ids, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			AnySlot: []*model.Block{nil, markedBlock("pasted", "hello")},
		}, "")

		// then
		require.NoError(t, err)
		require.Len(t, ids, 1)
		assert.Equal(t, "hello", sb.Pick(ids[0]).Model().GetText().Text)
	})

	for _, tc := range []struct {
		name      string
		blockId   string
		detailKey string
		want      string
	}{
		{
			name:      "title block is unbound from its detail and keeps its text",
			blockId:   template.TitleBlockId,
			detailKey: "name",
			want:      "copied title",
		},
		{
			name:      "description block is unbound from its detail and keeps its text",
			blockId:   template.DescriptionBlockId,
			detailKey: "description",
			want:      "copied description",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given: a block still bound to a detail, as Copy hands it over
			sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
			cb := newFixture(t, sb)
			b := withId(markedBlock("", tc.want), tc.blockId)
			b.Fields = &types.Struct{Fields: map[string]*types.Value{
				text.DetailsKeyFieldName: pbtypes.StringList([]string{tc.detailKey}),
				"keepMe":                 pbtypesString("value"),
			}}

			// when
			ids, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{AnySlot: []*model.Block{b}}, "")

			// then: still bound, the pasted block would render the target object's detail,
			// which this page does not have, and the copied text would be lost
			require.NoError(t, err)
			require.Len(t, ids, 1)
			assert.Equal(t, tc.want, sb.Pick(ids[0]).Model().GetText().Text)
			assert.NotContains(t, b.Fields.Fields, text.DetailsKeyFieldName)
			assert.Contains(t, b.Fields.Fields, "keepMe")
		})
	}
}

func withId(b *model.Block, id string) *model.Block {
	b.Id = id
	return b
}

func pbtypesString(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}

// collectMarksOf walks the pasted tree and collects the marks of the block holding wantText
func collectMarksOf(sb *smarttest.SmartTest, id string, wantText string, out *[]*model.BlockContentTextMark) {
	b := sb.Pick(id)
	if b == nil {
		return
	}
	if txt := b.Model().GetText(); txt != nil && txt.Text == wantText && txt.Marks != nil {
		*out = append(*out, txt.Marks.Marks...)
	}
	for _, c := range b.Model().ChildrenIds {
		collectMarksOf(sb, c, wantText, out)
	}
}
