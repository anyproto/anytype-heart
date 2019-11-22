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

		// should return out of range error
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
}
