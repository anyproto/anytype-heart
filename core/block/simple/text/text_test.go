package text

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestText_Diff(t *testing.T) {
	testBlock := func() *Text {
		return NewText(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfText{Text: &model.BlockContentText{}},
		}).(*Text)
	}
	t.Run("type error", func(t *testing.T) {
		b1 := testBlock()
		b2 := base.NewBase(&model.Block{})
		_, err := b1.Diff(b2)
		assert.Error(t, err)
	})
	t.Run("no diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b1.SetText("same text", &model.BlockContentTextMarks{})
		b2.SetText("same text", &model.BlockContentTextMarks{})
		d, err := b1.Diff(b2)
		require.NoError(t, err)
		assert.Len(t, d, 0)
	})
	t.Run("base diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.Restrictions.Read = true
		d, err := b1.Diff(b2)
		require.NoError(t, err)
		assert.Len(t, d, 1)
	})
	t.Run("content diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.SetText("text", &model.BlockContentTextMarks{
			Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{1, 2},
					Type:  model.BlockContentTextMark_Italic,
				},
			},
		})
		b2.SetStyle(model.BlockContentText_Header2)
		b2.SetChecked(true)
		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		textChange := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetText).BlockSetText
		assert.NotNil(t, textChange.Style)
		assert.NotNil(t, textChange.Checked)
		assert.NotNil(t, textChange.Text)
		assert.NotNil(t, textChange.Marks)
	})
}

func TestText_Split(t *testing.T) {
	testBlock := func() *Text {
		return NewText(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text: "1234567890",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Type: model.BlockContentTextMark_Bold,
							Range: &model.Range{
								From: 0,
								To:   10,
							},
						},
						{
							Type: model.BlockContentTextMark_Italic,
							Range: &model.Range{
								From: 6,
								To:   10,
							},
						},
						{
							Type: model.BlockContentTextMark_BackgroundColor,
							Range: &model.Range{
								From: 3,
								To:   4,
							},
						},
					},
				},
			}},
		}).(*Text)
	}
	t.Run("should split block", func(t *testing.T) {
		b := testBlock()
		newBlock, err := b.Split(5)
		require.NoError(t, err)
		nb := newBlock.(*Text)
		assert.Equal(t, "12345", nb.content.Text)
		assert.Equal(t, "67890", b.content.Text)
		require.Len(t, b.content.Marks.Marks, 2)
		require.Len(t, nb.content.Marks.Marks, 2)
		assert.Equal(t, model.Range{0, 5}, *nb.content.Marks.Marks[0].Range)
		assert.Equal(t, model.Range{3, 4}, *nb.content.Marks.Marks[1].Range)
		assert.Equal(t, model.Range{0, 5}, *b.content.Marks.Marks[0].Range)
		assert.Equal(t, model.Range{1, 5}, *b.content.Marks.Marks[1].Range)
	})
	t.Run("out of range", func(t *testing.T) {
		b := testBlock()
		_, err := b.Split(11)
		require.Equal(t, ErrOutOfRange, err)
	})
	t.Run("start pos", func(t *testing.T) {
		b := testBlock()
		_, err := b.Split(0)
		require.NoError(t, err)
	})
	t.Run("end pos", func(t *testing.T) {
		b := testBlock()
		_, err := b.Split(10)
		require.NoError(t, err)
	})
}

func TestText_normalizeMarks(t *testing.T) {
	b := NewText(&model.Block{
		Restrictions: &model.BlockRestrictions{},
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text: "1234567890",
			Marks: &model.BlockContentTextMarks{
				Marks: []*model.BlockContentTextMark{
					{
						Type: model.BlockContentTextMark_Bold,
						Range: &model.Range{
							From: 0,
							To:   5,
						},
					},
					{
						Type: model.BlockContentTextMark_Bold,
						Range: &model.Range{
							From: 5,
							To:   10,
						},
					},
					{
						Type: model.BlockContentTextMark_BackgroundColor,
						Range: &model.Range{
							From: 3,
							To:   4,
						},
					},
					{
						Type: model.BlockContentTextMark_BackgroundColor,
						Range: &model.Range{
							From: 4,
							To:   5,
						},
					},
					{
						Type: model.BlockContentTextMark_BackgroundColor,
						Range: &model.Range{
							From: 4,
							To:   6,
						},
					},
				},
			},
		}},
	}).(*Text)

	b.normalizeMarks()

	require.Len(t, b.content.Marks.Marks, 2)
	assert.Equal(t, model.Range{From: 0, To: 10}, *b.content.Marks.Marks[0].Range)
	assert.Equal(t, model.Range{From: 3, To: 6}, *b.content.Marks.Marks[1].Range)
}

func TestText_Merge(t *testing.T) {
	testBlock := func() *Text {
		return NewText(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Text: "1234567890",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{
							Type: model.BlockContentTextMark_Bold,
							Range: &model.Range{
								From: 0,
								To:   5,
							},
						},
						{
							Type: model.BlockContentTextMark_Bold,
							Range: &model.Range{
								From: 5,
								To:   10,
							},
						},
						{
							Type: model.BlockContentTextMark_BackgroundColor,
							Range: &model.Range{
								From: 3,
								To:   4,
							},
						},
					},
				},
			}},
		}).(*Text)
	}

	t.Run("should merge two blocks", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		err := b1.Merge(b2)
		require.NoError(t, err)
		assert.Equal(t, "12345678901234567890", b1.content.Text)

		require.Len(t, b1.content.Marks.Marks, 3)
		assert.Equal(t, model.Range{From: 0, To: 20}, *b1.content.Marks.Marks[0].Range)
		assert.Equal(t, model.Range{From: 3, To: 4}, *b1.content.Marks.Marks[1].Range)
		assert.Equal(t, model.Range{From: 13, To: 14}, *b1.content.Marks.Marks[2].Range)
	})
}

func TestText_SetMarkForAllText(t *testing.T) {
	b := NewText(&model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "1234567890",
			},
		},
	})
	tb := b.(Block)
	tb.SetMarkForAllText(&model.BlockContentTextMark{
		Type: model.BlockContentTextMark_Bold,
	})
	require.Len(t, tb.Model().GetText().Marks.Marks, 1)
	assert.Equal(t, &model.BlockContentTextMark{
		Type:  model.BlockContentTextMark_Bold,
		Range: &model.Range{From: 0, To: 10},
	}, tb.Model().GetText().Marks.Marks[0])
	tb.SetMarkForAllText(&model.BlockContentTextMark{
		Type: model.BlockContentTextMark_Italic,
	})
	require.Len(t, tb.Model().GetText().Marks.Marks, 2)
	assert.Equal(t, &model.BlockContentTextMark{
		Type:  model.BlockContentTextMark_Italic,
		Range: &model.Range{From: 0, To: 10},
	}, tb.Model().GetText().Marks.Marks[1])
	tb.SetMarkForAllText(&model.BlockContentTextMark{
		Type: model.BlockContentTextMark_Bold,
	})
	assert.Len(t, tb.Model().GetText().Marks.Marks, 2)
}

func TestText_RemoveMarkType(t *testing.T) {
	b := NewText(&model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "1234567890",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{Type: model.BlockContentTextMark_Bold, Range: &model.Range{To: 10}},
						{Type: model.BlockContentTextMark_Italic, Range: &model.Range{To: 5}},
					},
				},
			},
		},
	}).(Block)
	b.RemoveMarkType(model.BlockContentTextMark_Bold)
	assert.Len(t, b.Model().GetText().Marks.Marks, 1)
	assert.Equal(t, model.BlockContentTextMark_Italic, b.Model().GetText().Marks.Marks[0].Type)
}

func TestText_HasMarkForAllText(t *testing.T) {
	b := NewText(&model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "1234567890",
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{
						{Type: model.BlockContentTextMark_Bold, Range: &model.Range{To: 10}},
						{Type: model.BlockContentTextMark_Italic, Range: &model.Range{To: 5}},
					},
				},
			},
		},
	}).(Block)
	assert.False(t, b.HasMarkForAllText(&model.BlockContentTextMark{Type: model.BlockContentTextMark_Italic}))
	assert.True(t, b.HasMarkForAllText(&model.BlockContentTextMark{Type: model.BlockContentTextMark_Bold}))
}
