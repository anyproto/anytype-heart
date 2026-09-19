package text

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Documents written before the paste path validated marks may hold a mark without a Range.
// Nothing in the mark handling may panic on one.
func blockWithMarks(txt string, marks ...*model.BlockContentTextMark) *Text {
	return NewText(&model.Block{
		Id:           "test",
		Restrictions: &model.BlockRestrictions{},
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:  txt,
			Marks: &model.BlockContentTextMarks{Marks: marks},
		}},
	}).(*Text)
}

func nilRangeMark() *model.BlockContentTextMark {
	return &model.BlockContentTextMark{Type: model.BlockContentTextMark_Bold}
}

func boldMark(from, to int32) *model.BlockContentTextMark {
	return &model.BlockContentTextMark{
		Type:  model.BlockContentTextMark_Bold,
		Range: &model.Range{From: from, To: to},
	}
}

func TestMarkWithoutRange(t *testing.T) {
	t.Run("split drops the mark without range and keeps the valid one", func(t *testing.T) {
		// given
		b := blockWithMarks("hello world", nilRangeMark(), boldMark(0, 5))
		want := []*model.BlockContentTextMark{boldMark(0, 5)}

		// when
		newBlock, err := b.Split(5)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, newBlock.Model().GetText().Marks.Marks)
		assert.Empty(t, b.content.Marks.Marks)
	})

	t.Run("set text merges marks without range", func(t *testing.T) {
		// given
		b := blockWithMarks("hello")

		// when
		b.SetText("hello", &model.BlockContentTextMarks{
			Marks: []*model.BlockContentTextMark{nilRangeMark(), nilRangeMark()},
		})

		// then
		assert.Len(t, b.content.Marks.Marks, 2)
	})

	t.Run("range text paste over a mark without range", func(t *testing.T) {
		// given
		b := blockWithMarks("hello", nilRangeMark())
		copied := &model.Block{Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "ZZ",
				Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{nilRangeMark()}},
			},
		}}

		// when
		_, err := b.RangeTextPaste(1, 3, copied, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "hZZlo", b.content.Text)
	})

	t.Run("has mark for all text skips the mark without range", func(t *testing.T) {
		// given
		b := blockWithMarks("hello", nilRangeMark())

		// when
		got := b.HasMarkForAllText(&model.BlockContentTextMark{Type: model.BlockContentTextMark_Bold})

		// then
		assert.False(t, got)
	})

	t.Run("split skips a nil mark element", func(t *testing.T) {
		// given
		b := blockWithMarks("hello world", nil, boldMark(0, 5))
		want := []*model.BlockContentTextMark{boldMark(0, 5)}

		// when
		newBlock, err := b.Split(5)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, newBlock.Model().GetText().Marks.Marks)
	})

	t.Run("range text paste skips a nil mark element", func(t *testing.T) {
		// given
		b := blockWithMarks("hello", nil)
		copied := &model.Block{Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "ZZ",
				Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{nil}},
			},
		}}

		// when
		_, err := b.RangeTextPaste(1, 3, copied, false)

		// then
		require.NoError(t, err)
		assert.Equal(t, "hZZlo", b.content.Text)
	})

	t.Run("has mark for all text skips a nil mark element", func(t *testing.T) {
		// given
		b := blockWithMarks("hello", nil)

		// when
		got := b.HasMarkForAllText(&model.BlockContentTextMark{Type: model.BlockContentTextMark_Bold})

		// then
		assert.False(t, got)
	})

	t.Run("set text merges a mark without range followed by a valid one", func(t *testing.T) {
		// given
		b := blockWithMarks("hello")

		// when
		b.SetText("hello", &model.BlockContentTextMarks{
			Marks: []*model.BlockContentTextMark{nilRangeMark(), boldMark(0, 2)},
		})

		// then
		assert.Len(t, b.content.Marks.Marks, 2)
	})

	t.Run("set text merges a valid mark followed by one without range", func(t *testing.T) {
		// given
		b := blockWithMarks("hello")

		// when
		b.SetText("hello", &model.BlockContentTextMarks{
			Marks: []*model.BlockContentTextMark{boldMark(0, 2), nilRangeMark()},
		})

		// then
		assert.Len(t, b.content.Marks.Marks, 2)
	})

	t.Run("marks differ when only the left has a range", func(t *testing.T) {
		// given
		m1 := &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{boldMark(0, 1)}}
		m2 := &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{nilRangeMark()}}

		// when, then
		assert.False(t, marksEq(m1, m2))
	})

	t.Run("marks are equal when both have no range", func(t *testing.T) {
		// given
		m1 := &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{nilRangeMark()}}
		m2 := &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{nilRangeMark()}}

		// when, then
		assert.True(t, marksEq(m1, m2))
	})

	t.Run("marks differ when only one has a range", func(t *testing.T) {
		// given
		m1 := &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{nilRangeMark()}}
		m2 := &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{boldMark(0, 1)}}

		// when, then
		assert.False(t, marksEq(m1, m2))
	})
}

func TestTextContentWithoutText(t *testing.T) {
	t.Run("empty text content is materialized", func(t *testing.T) {
		// given
		content := &model.BlockContentOfText{}

		// when
		got, err := toTextContent(content)

		// then
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "", got.Text)
		assert.NotNil(t, got.Marks)
	})
}
