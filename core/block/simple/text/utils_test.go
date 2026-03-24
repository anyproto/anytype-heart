package text

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func Test_marksEq(t *testing.T) {
	newMarks := func() *model.BlockContentTextMarks {
		return &model.BlockContentTextMarks{
			Marks: []*model.BlockContentTextMark{
				{
					Range: &model.Range{From: 0, To: 1},
					Type:  model.BlockContentTextMark_Bold,
				},
				{
					Range: &model.Range{From: 2, To: 3},
					Type:  model.BlockContentTextMark_Bold,
				},
				{
					Range: &model.Range{From: 0, To: 1},
					Type:  model.BlockContentTextMark_TextColor,
					Param: "red",
				},
				{
					Range: &model.Range{From: 2, To: 3},
					Type:  model.BlockContentTextMark_TextColor,
					Param: "green",
				},
			},
		}
	}

	t.Run("equals", func(t *testing.T) {
		assert.True(t, marksEq(newMarks(), newMarks()))
	})
	t.Run("range", func(t *testing.T) {
		m2 := newMarks()
		m2.Marks[1].Range.To = 5
		assert.False(t, marksEq(newMarks(), m2))
	})
	t.Run("param", func(t *testing.T) {
		m2 := newMarks()
		m2.Marks[3].Param = "new"
		assert.False(t, marksEq(newMarks(), m2))
	})
	t.Run("type", func(t *testing.T) {
		m2 := newMarks()
		m2.Marks[0].Type = model.BlockContentTextMark_Italic
		assert.False(t, marksEq(newMarks(), m2))
	})
}

func TestMergeAdjacentMarks(t *testing.T) {
	t.Run("merge adjacent marks", func(t *testing.T) {
		marks := []*model.BlockContentTextMark{
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
		}

		mergedMarks := mergeAdjacentMarks(marks)

		require.Len(t, mergedMarks, 2)
		assert.Equal(t, model.Range{From: 0, To: 10}, *mergedMarks[0].Range)
		assert.Equal(t, model.Range{From: 3, To: 6}, *mergedMarks[1].Range)
	})

	t.Run("adjacent identical emojis are not merged", func(t *testing.T) {
		// given
		marks := []*model.BlockContentTextMark{
			{
				Type:  model.BlockContentTextMark_Emoji,
				Param: "😀",
				Range: &model.Range{From: 0, To: 1},
			},
			{
				Type:  model.BlockContentTextMark_Emoji,
				Param: "😀",
				Range: &model.Range{From: 1, To: 2},
			},
		}

		// when
		mergedMarks := mergeAdjacentMarks(marks)

		// then
		require.Len(t, mergedMarks, 2)
		assert.Equal(t, model.Range{From: 0, To: 1}, *mergedMarks[0].Range)
		assert.Equal(t, model.Range{From: 1, To: 2}, *mergedMarks[1].Range)
	})

	t.Run("different emojis are not merged", func(t *testing.T) {
		// given
		marks := []*model.BlockContentTextMark{
			{
				Type:  model.BlockContentTextMark_Emoji,
				Param: "😀",
				Range: &model.Range{From: 0, To: 1},
			},
			{
				Type:  model.BlockContentTextMark_Emoji,
				Param: "🎉",
				Range: &model.Range{From: 1, To: 2},
			},
		}

		// when
		mergedMarks := mergeAdjacentMarks(marks)

		// then
		require.Len(t, mergedMarks, 2)
		assert.Equal(t, "😀", mergedMarks[0].Param)
		assert.Equal(t, "🎉", mergedMarks[1].Param)
	})

	t.Run("three identical emojis are all preserved", func(t *testing.T) {
		// given
		marks := []*model.BlockContentTextMark{
			{
				Type:  model.BlockContentTextMark_Emoji,
				Param: "😀",
				Range: &model.Range{From: 0, To: 1},
			},
			{
				Type:  model.BlockContentTextMark_Emoji,
				Param: "😀",
				Range: &model.Range{From: 1, To: 2},
			},
			{
				Type:  model.BlockContentTextMark_Emoji,
				Param: "😀",
				Range: &model.Range{From: 2, To: 3},
			},
		}

		// when
		mergedMarks := mergeAdjacentMarks(marks)

		// then
		require.Len(t, mergedMarks, 3)
		assert.Equal(t, model.Range{From: 0, To: 1}, *mergedMarks[0].Range)
		assert.Equal(t, model.Range{From: 1, To: 2}, *mergedMarks[1].Range)
		assert.Equal(t, model.Range{From: 2, To: 3}, *mergedMarks[2].Range)
	})

	t.Run("emojis preserved while bold marks still merge", func(t *testing.T) {
		// given
		marks := []*model.BlockContentTextMark{
			{
				Type:  model.BlockContentTextMark_Bold,
				Range: &model.Range{From: 0, To: 2},
			},
			{
				Type:  model.BlockContentTextMark_Bold,
				Range: &model.Range{From: 2, To: 6},
			},
			{
				Type:  model.BlockContentTextMark_Emoji,
				Param: "😀",
				Range: &model.Range{From: 2, To: 3},
			},
			{
				Type:  model.BlockContentTextMark_Emoji,
				Param: "😀",
				Range: &model.Range{From: 3, To: 4},
			},
		}

		// when
		mergedMarks := mergeAdjacentMarks(marks)

		// then - bold merged into one, emojis stay separate
		require.Len(t, mergedMarks, 3)

		boldCount := 0
		emojiCount := 0
		for _, m := range mergedMarks {
			if m.Type == model.BlockContentTextMark_Bold {
				boldCount++
				assert.Equal(t, model.Range{From: 0, To: 6}, *m.Range)
			}
			if m.Type == model.BlockContentTextMark_Emoji {
				emojiCount++
			}
		}
		assert.Equal(t, 1, boldCount)
		assert.Equal(t, 2, emojiCount)
	})

	t.Run("different text colors are not merged", func(t *testing.T) {
		// given
		marks := []*model.BlockContentTextMark{
			{
				Type:  model.BlockContentTextMark_TextColor,
				Param: "red",
				Range: &model.Range{From: 0, To: 3},
			},
			{
				Type:  model.BlockContentTextMark_TextColor,
				Param: "blue",
				Range: &model.Range{From: 3, To: 6},
			},
		}

		// when
		mergedMarks := mergeAdjacentMarks(marks)

		// then
		require.Len(t, mergedMarks, 2)
		assert.Equal(t, "red", mergedMarks[0].Param)
		assert.Equal(t, model.Range{From: 0, To: 3}, *mergedMarks[0].Range)
		assert.Equal(t, "blue", mergedMarks[1].Param)
		assert.Equal(t, model.Range{From: 3, To: 6}, *mergedMarks[1].Range)
	})

	t.Run("different background colors are not merged", func(t *testing.T) {
		// given
		marks := []*model.BlockContentTextMark{
			{
				Type:  model.BlockContentTextMark_BackgroundColor,
				Param: "yellow",
				Range: &model.Range{From: 0, To: 4},
			},
			{
				Type:  model.BlockContentTextMark_BackgroundColor,
				Param: "green",
				Range: &model.Range{From: 4, To: 8},
			},
		}

		// when
		mergedMarks := mergeAdjacentMarks(marks)

		// then
		require.Len(t, mergedMarks, 2)
		assert.Equal(t, "yellow", mergedMarks[0].Param)
		assert.Equal(t, model.Range{From: 0, To: 4}, *mergedMarks[0].Range)
		assert.Equal(t, "green", mergedMarks[1].Param)
		assert.Equal(t, model.Range{From: 4, To: 8}, *mergedMarks[1].Range)
	})

	t.Run("adjacent mentions with same param are merged", func(t *testing.T) {
		// given
		marks := []*model.BlockContentTextMark{
			{
				Type:  model.BlockContentTextMark_Mention,
				Param: "user123",
				Range: &model.Range{From: 0, To: 3},
			},
			{
				Type:  model.BlockContentTextMark_Mention,
				Param: "user123",
				Range: &model.Range{From: 3, To: 6},
			},
		}

		// when
		mergedMarks := mergeAdjacentMarks(marks)

		// then
		require.Len(t, mergedMarks, 1)
		assert.Equal(t, model.Range{From: 0, To: 6}, *mergedMarks[0].Range)
		assert.Equal(t, "user123", mergedMarks[0].Param)
	})

	t.Run("adjacent links with same param are merged", func(t *testing.T) {
		// given
		marks := []*model.BlockContentTextMark{
			{
				Type:  model.BlockContentTextMark_Link,
				Param: "https://example.com",
				Range: &model.Range{From: 0, To: 5},
			},
			{
				Type:  model.BlockContentTextMark_Link,
				Param: "https://example.com",
				Range: &model.Range{From: 5, To: 10},
			},
		}

		// when
		mergedMarks := mergeAdjacentMarks(marks)

		// then
		require.Len(t, mergedMarks, 1)
		assert.Equal(t, model.Range{From: 0, To: 10}, *mergedMarks[0].Range)
		assert.Equal(t, "https://example.com", mergedMarks[0].Param)
	})

	t.Run("adjacent object marks with same param are merged", func(t *testing.T) {
		// given
		marks := []*model.BlockContentTextMark{
			{
				Type:  model.BlockContentTextMark_Object,
				Param: "objectId1",
				Range: &model.Range{From: 0, To: 4},
			},
			{
				Type:  model.BlockContentTextMark_Object,
				Param: "objectId1",
				Range: &model.Range{From: 4, To: 8},
			},
		}

		// when
		mergedMarks := mergeAdjacentMarks(marks)

		// then
		require.Len(t, mergedMarks, 1)
		assert.Equal(t, model.Range{From: 0, To: 8}, *mergedMarks[0].Range)
		assert.Equal(t, "objectId1", mergedMarks[0].Param)
	})
}
