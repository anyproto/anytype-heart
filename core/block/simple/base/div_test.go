package base

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiv_Diff(t *testing.T) {
	testBlock := func() *Div {
		return NewDiv(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}},
		}).(*Div)
	}
	t.Run("type error", func(t *testing.T) {
		b1 := testBlock()
		b2 := NewBase(&model.Block{})
		_, err := b1.Diff(b2)
		assert.Error(t, err)
	})
	t.Run("no diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b1.content.Style = model.BlockContentDiv_Dots
		b2.content.Style = model.BlockContentDiv_Dots
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
		b2.SetStyle(model.BlockContentDiv_Dots)

		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetDiv).BlockSetDiv
		assert.NotNil(t, change.Style)
	})
}
