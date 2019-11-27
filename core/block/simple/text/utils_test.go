package text

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/stretchr/testify/assert"
)

func Test_marksByTypesEq(t *testing.T) {
	newMarks := func() map[model.BlockContentTextMarkType]ranges {
		return map[model.BlockContentTextMarkType]ranges{
			model.BlockContentTextMark_Bold: {
				&model.BlockContentTextMark{
					Range: &model.Range{0, 1},
					Type:  model.BlockContentTextMark_Bold,
				},
				&model.BlockContentTextMark{
					Range: &model.Range{2, 3},
					Type:  model.BlockContentTextMark_Bold,
				},
			},
			model.BlockContentTextMark_Italic: {},
			model.BlockContentTextMark_TextColor: {
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
		assert.True(t, marksByTypesEq(newMarks(), newMarks()))
	})
	t.Run("range", func(t *testing.T) {
		m2 := newMarks()
		m2[model.BlockContentTextMark_Bold][1].Range.To = 5
		assert.False(t, marksByTypesEq(newMarks(), m2))
	})
	t.Run("param", func(t *testing.T) {
		m2 := newMarks()
		m2[model.BlockContentTextMark_TextColor][0].Param = "new"
		assert.False(t, marksByTypesEq(newMarks(), m2))
	})
	t.Run("type", func(t *testing.T) {
		m2 := newMarks()
		m2[model.BlockContentTextMark_TextColor][0].Type = model.BlockContentTextMark_Italic
		assert.False(t, marksByTypesEq(newMarks(), m2))
	})
}
