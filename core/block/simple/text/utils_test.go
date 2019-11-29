package text

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/stretchr/testify/assert"
)

func Test_marksEq(t *testing.T) {
	newMarks := func() *model.BlockContentTextMarks {
		return &model.BlockContentTextMarks{
			Marks: []*model.BlockContentTextMark{
				&model.BlockContentTextMark{
					Range: &model.Range{0, 1},
					Type:  model.BlockContentTextMark_Bold,
				},
				&model.BlockContentTextMark{
					Range: &model.Range{2, 3},
					Type:  model.BlockContentTextMark_Bold,
				},
				&model.BlockContentTextMark{
					Range: &model.Range{0, 1},
					Type:  model.BlockContentTextMark_TextColor,
					Param: "red",
				},
				&model.BlockContentTextMark{
					Range: &model.Range{2, 3},
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
