package embed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/block/simple/test"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestLatex_Diff(t *testing.T) {
	testBlock := func() *Latex {
		return NewLatex(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{}},
		}).(*Latex)
	}
	t.Run("type error", func(t *testing.T) {
		b1 := testBlock()
		b2 := base.NewBase(&model.Block{})
		_, err := b1.Diff("", b2)
		assert.Error(t, err)
	})
	t.Run("no diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b1.content.Text = "1"
		b2.content.Text = "1"
		d, err := b1.Diff("", b2)
		require.NoError(t, err)
		assert.Len(t, d, 0)
	})
	t.Run("base diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.Restrictions.Read = true
		d, err := b1.Diff("", b2)
		require.NoError(t, err)
		assert.Len(t, d, 1)
	})
	t.Run("content diff", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.content.Text = "42"

		diff, err := b1.Diff("", b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockSetLatex{
			BlockSetLatex: &pb.EventBlockSetLatex{
				Id:   b1.Id,
				Text: &pb.EventBlockSetLatexText{Value: "42"},
			},
		}), diff)
	})
	t.Run("content diff processor", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.content.Processor = model.BlockContentLatex_Mermaid

		diff, err := b1.Diff("", b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockSetLatex{
			BlockSetLatex: &pb.EventBlockSetLatex{
				Id: b1.Id,
				Processor: &pb.EventBlockSetLatexProcessor{
					Value: model.BlockContentLatex_Mermaid,
				},
			},
		}), diff)
	})
}
