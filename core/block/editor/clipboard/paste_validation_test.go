package clipboard

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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

func TestDropInvalidMarks(t *testing.T) {
	boldAt := func(from, to int32) *model.BlockContentTextMark {
		return &model.BlockContentTextMark{
			Type:  model.BlockContentTextMark_Bold,
			Range: &model.Range{From: from, To: to},
		}
	}

	for _, tc := range []struct {
		name  string
		text  string
		marks []*model.BlockContentTextMark
		want  []*model.BlockContentTextMark
	}{
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
			name:  "negative range.from is dropped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(-1, 3)},
			want:  []*model.BlockContentTextMark{},
		},
		{
			name:  "negative offsets are dropped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(-3, -1)},
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
			name:  "reversed range is dropped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(4, 2)},
			want:  []*model.BlockContentTextMark{},
		},
		{
			name:  "range.to past the end of the text is dropped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(0, 6)},
			want:  []*model.BlockContentTextMark{},
		},
		{
			name:  "range.from at the end of the text is dropped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(5, 5)},
			want:  []*model.BlockContentTextMark{},
		},
		{
			name:  "any mark on empty text is dropped",
			text:  "",
			marks: []*model.BlockContentTextMark{boldAt(0, 0)},
			want:  []*model.BlockContentTextMark{},
		},
		{
			name:  "mark covering the whole text is kept",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(0, 5)},
			want:  []*model.BlockContentTextMark{boldAt(0, 5)},
		},
		{
			name:  "only the invalid mark is dropped",
			text:  "hello",
			marks: []*model.BlockContentTextMark{boldAt(0, 2), {Type: model.BlockContentTextMark_Italic}, boldAt(3, 5)},
			want:  []*model.BlockContentTextMark{boldAt(0, 2), boldAt(3, 5)},
		},
		{
			// the emoji is 1 rune / 4 bytes / 2 UTF-16 code units: offsets are UTF-16 units
			name:  "mark covering a surrogate pair is kept",
			text:  grin,
			marks: []*model.BlockContentTextMark{boldAt(0, 2)},
			want:  []*model.BlockContentTextMark{boldAt(0, 2)},
		},
		{
			name:  "mark past a surrogate pair is dropped",
			text:  grin,
			marks: []*model.BlockContentTextMark{boldAt(0, 3)},
			want:  []*model.BlockContentTextMark{},
		},
		{
			name:  "mark starting inside a surrogate pair is kept",
			text:  grin,
			marks: []*model.BlockContentTextMark{boldAt(1, 2)},
			want:  []*model.BlockContentTextMark{boldAt(1, 2)},
		},
		{
			name:  "mark past the end of text measured in UTF-16 units is dropped",
			text:  grin + "a",
			marks: []*model.BlockContentTextMark{boldAt(0, 4)},
			want:  []*model.BlockContentTextMark{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			b := markedBlock("b", tc.text, tc.marks...)

			// when
			dropInvalidMarks(b)

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
		dropInvalidMarks(b)

		// then
		require.Nil(t, b.GetText().Marks)
	})

	t.Run("non-text block is left alone", func(t *testing.T) {
		// given
		b := &model.Block{Id: "b", Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}}

		// when
		dropInvalidMarks(b)

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

	t.Run("title block keeps other fields and loses the details key", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
		cb := newFixture(t, sb)
		b := withId(markedBlock("", "hello"), template.TitleBlockId)
		b.Fields = &types.Struct{Fields: map[string]*types.Value{
			text.DetailsKeyFieldName: pbtypesString("name"),
			"keepMe":                 pbtypesString("value"),
		}}

		// when
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{AnySlot: []*model.Block{b}}, "")

		// then
		require.NoError(t, err)
		assert.NotContains(t, b.Fields.Fields, text.DetailsKeyFieldName)
		assert.Contains(t, b.Fields.Fields, "keepMe")
	})
}

func withId(b *model.Block, id string) *model.Block {
	b.Id = id
	return b
}

func pbtypesString(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}
