package link

import (
	"testing"

	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLink_Diff(t *testing.T) {
	testBlock := func() *Link {
		return NewLink(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "some target"}},
		}).(*Link)
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
		b1.content.TargetBlockId = "1"
		b2.content.TargetBlockId = "1"
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
		b2.content.Style = model.BlockContentLink_Dataview
		b2.content.TargetBlockId = "42"
		b2.content.CardStyle = model.BlockContentLink_Card
		b2.content.IconSize = model.BlockContentLink_SizeMedium
		b2.content.Description = model.BlockContentLink_Content

		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetLink).BlockSetLink
		assert.NotNil(t, change.TargetBlockId)
		assert.Equal(t, "42", change.TargetBlockId.Value)

		assert.NotNil(t, change.Style)
		assert.Equal(t, model.BlockContentLink_Dataview, change.Style.Value)

		assert.NotNil(t, change.CardStyle)
		assert.Equal(t, model.BlockContentLink_Card, change.CardStyle.Value)

		assert.NotNil(t, change.IconSize)
		assert.Equal(t, model.BlockContentLink_SizeMedium, change.IconSize.Value)

		assert.NotNil(t, change.Description)
		assert.Equal(t, model.BlockContentLink_Content, change.Description.Value)
	})
	t.Run("relations", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.content.Relations = append(b2.content.Relations, "cover")

		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetLink).BlockSetLink.Relations
		assert.Len(t, change.Value, 1)
		assert.Equal(t, "cover", change.Value[0])
	})
}

func TestLink_ToText(t *testing.T) {
	t.Run("with name", func(t *testing.T) {
		b := NewLink(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "targetId"}},
		}).(*Link)
		tb := b.ToText(&types.Struct{
			Fields: map[string]*types.Value{
				"name": pbtypes.String("target name"),
			},
		})
		require.NotNil(t, tb)
		textModel := tb.Model().GetText()
		assert.Equal(t, "target name", textModel.Text)
		require.Len(t, textModel.Marks.Marks, 1)
		assert.Equal(t, "targetId", textModel.Marks.Marks[0].Param)
		assert.Equal(t, &model.Range{0, 11}, textModel.Marks.Marks[0].Range)
	})
	t.Run("with empty name", func(t *testing.T) {
		b := NewLink(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "targetId"}},
		}).(*Link)
		tb := b.ToText(nil)
		require.NotNil(t, tb)
		textModel := tb.Model().GetText()
		assert.Equal(t, "Untitled", textModel.Text)
		require.Len(t, textModel.Marks.Marks, 1)
		assert.Equal(t, "targetId", textModel.Marks.Marks[0].Param)
		assert.Equal(t, &model.Range{0, 8}, textModel.Marks.Marks[0].Range)
	})
}

func TestLink_Validate(t *testing.T) {
	t.Run("not validated", func(t *testing.T) {
		b := NewLink(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: ""}},
		}).(*Link)
		err := b.Validate()
		assert.Error(t, err)
	})
}
