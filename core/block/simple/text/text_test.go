package text

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
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
		textChange := diff[0].Value.(*pb.EventMessageValueOfBlockSetText).BlockSetText
		assert.NotNil(t, textChange.Style)
		assert.NotNil(t, textChange.Checked)
		assert.NotNil(t, textChange.Text)
		assert.NotNil(t, textChange.Marks)
	})
}
