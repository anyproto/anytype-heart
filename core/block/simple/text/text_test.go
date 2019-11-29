package text

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestText_Diff(t *testing.T) {
	testBlock := func() *Text {
		return NewText(&model.Block{
			Permissions: &model.BlockPermissions{},
			Content:     &model.BlockCore{Content: &model.BlockCoreContentOfText{Text: &model.BlockContentText{}}},
		})
	}

	t.Run("no diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b1.SetText("same text", &model.BlockContentTextMarks{})
		b2.SetText("same text", &model.BlockContentTextMarks{})
		assert.Len(t, b1.Diff(b2), 0)
	})
	t.Run("base diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.Permissions.Read = true
		assert.Len(t, b1.Diff(b2), 1)
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
		b2.SetCheckable(true)
		b2.SetMarker(model.BlockContentText_Number)
		b2.SetToggleable(true)
		diff := b1.Diff(b2)
		require.Len(t, diff, 1)
		textChange := diff[0].Value.(*pb.EventMessageValueOfBlockSetText).BlockSetText
		assert.NotNil(t, textChange.Toggleable)
		assert.NotNil(t, textChange.Marker)
		assert.NotNil(t, textChange.Style)
		assert.NotNil(t, textChange.Checkable)
		assert.NotNil(t, textChange.Check)
		assert.NotNil(t, textChange.Text)
		assert.NotNil(t, textChange.Marks)
	})
}
