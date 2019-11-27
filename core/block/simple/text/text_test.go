package text

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestText_AddMark(t *testing.T) {
	t.Run("out of range validation", func(t *testing.T) {
		block := NewText(&model.Block{
			Content: &model.BlockCore{Content: &model.BlockCoreContentOfText{Text: &model.BlockContentText{
				Text: "012345678901234567890123456789",
			}}},
		})
		outOfRangeMarks := []*model.BlockContentTextMark{
			{
				Range: &model.Range{-1, 10},
				Type:  model.BlockContentTextMark_Bold,
			},
			{
				Range: &model.Range{2, 1},
				Type:  model.BlockContentTextMark_Bold,
			},
			{
				Range: &model.Range{2, 31},
				Type:  model.BlockContentTextMark_Bold,
			},
			{},
		}
		for _, m := range outOfRangeMarks {
			err := block.AddMark(m)
			assert.Equal(t, ErrOutOfRange, err)
		}
	})

	testBlock := func() *Text {
		return NewText(&model.Block{
			Content: &model.BlockCore{Content: &model.BlockCoreContentOfText{Text: &model.BlockContentText{
				Text: "012345678901234567890123456789",
				Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
					{
						Range: &model.Range{To: 10},
						Type:  model.BlockContentTextMark_Bold,
					},
				}},
			}}},
		})
	}

	testBlockColor := func() *Text {
		return NewText(&model.Block{
			Content: &model.BlockCore{Content: &model.BlockCoreContentOfText{Text: &model.BlockContentText{
				Text: "012345678901234567890123456789",
			}}},
		})
	}

	t.Run("toggle existing", func(t *testing.T) {
		block := testBlock()
		assert.Len(t, block.content.Marks.Marks, 1)

		err := block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{To: 10},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)
		assert.Len(t, block.content.Marks.Marks, 0)
	})

	t.Run("toggle left side", func(t *testing.T) {
		block := testBlock()
		assert.Len(t, block.content.Marks.Marks, 1)

		err := block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{To: 5},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)
		require.Len(t, block.content.Marks.Marks, 1)
		assert.Equal(t, model.BlockContentTextMark_Bold, block.content.Marks.Marks[0].Type)
		assert.Equal(t, &model.Range{From: 5, To: 10}, block.content.Marks.Marks[0].Range)
	})
	t.Run("toggle right side", func(t *testing.T) {
		block := testBlock()
		assert.Len(t, block.content.Marks.Marks, 1)

		err := block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 5, To: 10},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)
		require.Len(t, block.content.Marks.Marks, 1)
		assert.Equal(t, model.BlockContentTextMark_Bold, block.content.Marks.Marks[0].Type)
		assert.Equal(t, &model.Range{From: 0, To: 5}, block.content.Marks.Marks[0].Range)
	})
	t.Run("toggle center", func(t *testing.T) {
		block := testBlock()
		assert.Len(t, block.content.Marks.Marks, 1)

		err := block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 3, To: 6},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)
		require.Len(t, block.content.Marks.Marks, 2)
		assert.Equal(t, &model.Range{From: 0, To: 3}, block.content.Marks.Marks[0].Range)
		assert.Equal(t, &model.Range{From: 6, To: 10}, block.content.Marks.Marks[1].Range)
	})
	t.Run("overlap inner", func(t *testing.T) {
		block := testBlock()
		err := block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 12, To: 14},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 16, To: 18},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 22, To: 28},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)
		assert.Len(t, block.content.Marks.Marks, 4)

		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 14, To: 22},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)
		require.Len(t, block.content.Marks.Marks, 2)
		assert.Equal(t, &model.Range{From: 12, To: 28}, block.content.Marks.Marks[1].Range)
	})

	t.Run("overlap outer", func(t *testing.T) {
		block := testBlock()
		err := block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 12, To: 14},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 22, To: 28},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)
		assert.Len(t, block.content.Marks.Marks, 3)

		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 11, To: 29},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)
		assert.Len(t, block.content.Marks.Marks, 2)
		assert.Equal(t, &model.Range{From: 11, To: 29}, block.content.Marks.Marks[1].Range)
	})
	t.Run("merge with param", func(t *testing.T) {
		block := testBlockColor()
		err := block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 12, To: 14},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 16, To: 18},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "green",
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 20, To: 25},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
		})
		require.NoError(t, err)
		assert.Len(t, block.content.Marks.Marks, 3)

		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 9, To: 21},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
		})
		require.NoError(t, err)
		assert.Len(t, block.content.Marks.Marks, 1)
		assert.Equal(t, &model.Range{From: 9, To: 25}, block.content.Marks.Marks[0].Range)
	})
	t.Run("merge left param; right drop", func(t *testing.T) {
		block := testBlockColor()
		err := block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 12, To: 14},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 16, To: 18},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "green",
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 20, To: 25},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "green",
		})
		require.NoError(t, err)
		assert.Len(t, block.content.Marks.Marks, 3)

		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 9, To: 28},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
		})
		require.NoError(t, err)
		assert.Len(t, block.content.Marks.Marks, 1)
		assert.Equal(t, &model.Range{From: 9, To: 28}, block.content.Marks.Marks[0].Range)
	})
	t.Run("merge left param; right split", func(t *testing.T) {
		block := testBlockColor()
		err := block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 12, To: 14},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 16, To: 18},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "green",
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 20, To: 25},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "green",
		})
		require.NoError(t, err)
		assert.Len(t, block.content.Marks.Marks, 3)

		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 9, To: 22},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
		})
		require.NoError(t, err)
		assert.Len(t, block.content.Marks.Marks, 2)
		assert.Equal(t, &model.Range{From: 9, To: 22}, block.content.Marks.Marks[0].Range)
		assert.Equal(t, &model.Range{From: 22, To: 25}, block.content.Marks.Marks[1].Range)
	})
	t.Run("merge right param; left drop", func(t *testing.T) {
		block := testBlockColor()
		err := block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 12, To: 14},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 16, To: 18},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "green",
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 20, To: 25},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "green",
		})
		require.NoError(t, err)
		assert.Len(t, block.content.Marks.Marks, 3)

		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 9, To: 22},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "green",
		})
		require.NoError(t, err)
		assert.Len(t, block.content.Marks.Marks, 1)
		assert.Equal(t, &model.Range{From: 9, To: 25}, block.content.Marks.Marks[0].Range)
	})
	t.Run("merge right param; left split", func(t *testing.T) {
		block := testBlockColor()
		err := block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 12, To: 14},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 16, To: 18},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "green",
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 20, To: 25},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "green",
		})
		require.NoError(t, err)
		assert.Len(t, block.content.Marks.Marks, 3)

		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 13, To: 28},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "green",
		})
		require.NoError(t, err)
		assert.Len(t, block.content.Marks.Marks, 2)
		assert.Equal(t, &model.Range{From: 12, To: 13}, block.content.Marks.Marks[0].Range)
		assert.Equal(t, &model.Range{From: 13, To: 28}, block.content.Marks.Marks[1].Range)
	})
	t.Run("split center", func(t *testing.T) {
		block := testBlockColor()
		err := block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 10, To: 20},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 13, To: 18},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "green",
		})
		require.NoError(t, err)

		require.Len(t, block.content.Marks.Marks, 3)
		assert.Equal(t, &model.Range{From: 10, To: 13}, block.content.Marks.Marks[0].Range)
		assert.Equal(t, "red", block.content.Marks.Marks[0].Param)
		assert.Equal(t, &model.Range{From: 13, To: 18}, block.content.Marks.Marks[1].Range)
		assert.Equal(t, "green", block.content.Marks.Marks[1].Param)
		assert.Equal(t, &model.Range{From: 18, To: 20}, block.content.Marks.Marks[2].Range)
		assert.Equal(t, "red", block.content.Marks.Marks[2].Param)
	})
	t.Run("left color", func(t *testing.T) {
		block := testBlockColor()
		err := block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 10, To: 20},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 10, To: 15},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "green",
		})
		require.NoError(t, err)

		require.Len(t, block.content.Marks.Marks, 2)
		assert.Equal(t, &model.Range{From: 10, To: 15}, block.content.Marks.Marks[0].Range)
		assert.Equal(t, "green", block.content.Marks.Marks[0].Param)
		assert.Equal(t, &model.Range{From: 15, To: 20}, block.content.Marks.Marks[1].Range)
		assert.Equal(t, "red", block.content.Marks.Marks[1].Param)

		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 8, To: 12},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
		})
		require.NoError(t, err)

		require.Len(t, block.content.Marks.Marks, 3)
		assert.Equal(t, &model.Range{From: 8, To: 12}, block.content.Marks.Marks[0].Range)
		assert.Equal(t, "red", block.content.Marks.Marks[0].Param)
		assert.Equal(t, &model.Range{From: 12, To: 15}, block.content.Marks.Marks[1].Range)
		assert.Equal(t, "green", block.content.Marks.Marks[1].Param)
		assert.Equal(t, &model.Range{From: 15, To: 20}, block.content.Marks.Marks[2].Range)
		assert.Equal(t, "red", block.content.Marks.Marks[2].Param)
	})
	t.Run("right color", func(t *testing.T) {
		block := testBlockColor()
		err := block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 10, To: 20},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 15, To: 22},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "green",
		})
		require.NoError(t, err)

		require.Len(t, block.content.Marks.Marks, 2)
		assert.Equal(t, &model.Range{From: 10, To: 15}, block.content.Marks.Marks[0].Range)
		assert.Equal(t, "red", block.content.Marks.Marks[0].Param)
		assert.Equal(t, &model.Range{From: 15, To: 22}, block.content.Marks.Marks[1].Range)
		assert.Equal(t, "green", block.content.Marks.Marks[1].Param)

		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 20, To: 25},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
		})
		require.NoError(t, err)

		require.Len(t, block.content.Marks.Marks, 3)
		assert.Equal(t, &model.Range{From: 10, To: 15}, block.content.Marks.Marks[0].Range)
		assert.Equal(t, "red", block.content.Marks.Marks[0].Param)
		assert.Equal(t, &model.Range{From: 15, To: 20}, block.content.Marks.Marks[1].Range)
		assert.Equal(t, "green", block.content.Marks.Marks[1].Param)
		assert.Equal(t, &model.Range{From: 20, To: 25}, block.content.Marks.Marks[2].Range)
		assert.Equal(t, "red", block.content.Marks.Marks[2].Param)
	})
	t.Run("replace color", func(t *testing.T) {
		block := testBlockColor()
		err := block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 10, To: 20},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "red",
		})
		require.NoError(t, err)
		err = block.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 10, To: 20},
			Type:  model.BlockContentTextMark_TextColor,
			Param: "green",
		})
		require.NoError(t, err)
		require.Len(t, block.content.Marks.Marks, 1)
		assert.Equal(t, &model.Range{From: 10, To: 20}, block.content.Marks.Marks[0].Range)
		assert.Equal(t, "green", block.content.Marks.Marks[0].Param)
	})
}

func TestText_SetText(t *testing.T) {
	testBlock := func() *Text {
		return NewText(&model.Block{
			Content: &model.BlockCore{Content: &model.BlockCoreContentOfText{Text: &model.BlockContentText{}}},
		})
	}

	t.Run("ErrOutOfRange", func(t *testing.T) {
		b := testBlock()
		var testRanges = []model.Range{
			{-1, 0},
			{0, -1},
			{0, 2},
		}
		for _, r := range testRanges {
			assert.Equal(t, ErrOutOfRange, b.SetText("123", r))
		}
	})

	t.Run("add text to empty block", func(t *testing.T) {
		b := testBlock()
		text := "1"
		err := b.SetText(text, model.Range{})
		require.NoError(t, err)
		assert.Equal(t, text, b.content.Text)
	})

	t.Run("add text to end of block", func(t *testing.T) {
		b := testBlock()
		b.content.Text = "1"
		err := b.SetText("2", model.Range{From: 1, To: 1})
		require.NoError(t, err)
		assert.Equal(t, "12", b.content.Text)
	})

	t.Run("replace text in block", func(t *testing.T) {
		b := testBlock()
		b.content.Text = "1"
		err := b.SetText("222", model.Range{
			From: 0,
			To:   1,
		})
		require.NoError(t, err)
		assert.Equal(t, "222", b.content.Text)
	})

	t.Run("prepend text, move marks", func(t *testing.T) {
		b := testBlock()
		b.content.Text = "1234567890"
		err := b.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 5, To: 6},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)
		err = b.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 8, To: 10},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)

		err = b.SetText("000", model.Range{})
		require.NoError(t, err)

		assert.Equal(t, "0001234567890", b.content.Text)
		assert.Equal(t, model.Range{From: 8, To: 9}, *b.content.Marks.Marks[0].Range)
		assert.Equal(t, model.Range{From: 11, To: 13}, *b.content.Marks.Marks[1].Range)
	})

	t.Run("insert into mark", func(t *testing.T) {
		b := testBlock()
		b.content.Text = "1234567890"
		err := b.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 5, To: 8},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)
		err = b.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 9, To: 10},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)

		err = b.SetText("0000", model.Range{5, 5})
		require.NoError(t, err)

		assert.Equal(t, "12345000067890", b.content.Text)
		assert.Equal(t, model.Range{From: 5, To: 12}, *b.content.Marks.Marks[0].Range)
		assert.Equal(t, model.Range{From: 13, To: 14}, *b.content.Marks.Marks[1].Range)
	})

	t.Run("insert over mark", func(t *testing.T) {
		b := testBlock()
		b.content.Text = "1234567890"
		err := b.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 5, To: 8},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)
		err = b.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 9, To: 10},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)

		err = b.SetText("XXXXX", model.Range{4, 8})
		require.NoError(t, err)

		assert.Equal(t, "1234XXXXX90", b.content.Text)
		require.Len(t, b.content.Marks.Marks, 1)
		assert.Equal(t, model.Range{From: 10, To: 11}, *b.content.Marks.Marks[0].Range)
	})

	t.Run("marks overlap right", func(t *testing.T) {
		b := testBlock()
		b.content.Text = "1234567890"
		err := b.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 5, To: 8},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)

		err = b.SetText("XXXXX", model.Range{4, 7})
		require.NoError(t, err)

		assert.Equal(t, "1234XXXXX890", b.content.Text)
		require.Len(t, b.content.Marks.Marks, 1)
		assert.Equal(t, model.Range{From: 9, To: 10}, *b.content.Marks.Marks[0].Range)
	})
	t.Run("marks overlap left", func(t *testing.T) {
		b := testBlock()
		b.content.Text = "1234567890"
		err := b.AddMark(&model.BlockContentTextMark{
			Range: &model.Range{From: 5, To: 8},
			Type:  model.BlockContentTextMark_Bold,
		})
		require.NoError(t, err)

		err = b.SetText("XXXXX", model.Range{6, 9})
		require.NoError(t, err)

		assert.Equal(t, "123456XXXXX0", b.content.Text)
		require.Len(t, b.content.Marks.Marks, 1)
		assert.Equal(t, model.Range{From: 5, To: 6}, *b.content.Marks.Marks[0].Range)
	})
}
