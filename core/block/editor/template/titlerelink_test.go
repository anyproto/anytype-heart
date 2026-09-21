package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// A block Unlink detaches stays in the state until Apply drops it. Two
// layout conversions in ONE state (the API's batched set_type: page → note
// → page) therefore ran WithNoTitle and then WithTitle on a title block that
// still existed but hung from nothing — and the page committed without it.
func TestHeaderBlocksRelinkAfterDetachInTheSameState(t *testing.T) {
	newPage := func() *state.State {
		doc := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"p1"}}),
			"p1": simple.New(&model.Block{Id: "p1",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "body"}}}),
		})
		s := doc.NewState()
		InitTemplate(s, WithTitle, WithDescription)
		require.Contains(t, s.Pick(HeaderLayoutId).Model().ChildrenIds, TitleBlockId)
		require.Contains(t, s.Pick(HeaderLayoutId).Model().ChildrenIds, DescriptionBlockId)
		return s
	}

	t.Run("a title WithNoTitle detached comes back with WithTitle", func(t *testing.T) {
		s := newPage()

		InitTemplate(s, WithNoTitle)
		require.NotContains(t, s.Pick(HeaderLayoutId).Model().ChildrenIds, TitleBlockId)
		require.True(t, s.Exists(TitleBlockId), "unlinked, not removed: the case that fooled the existence check")

		InitTemplate(s, WithTitle)

		assert.Equal(t, TitleBlockId, s.Pick(HeaderLayoutId).Model().ChildrenIds[0])
	})

	t.Run("a description WithNoDescription detached comes back with WithDescription", func(t *testing.T) {
		s := newPage()

		InitTemplate(s, WithNoDescription)
		require.NotContains(t, s.Pick(HeaderLayoutId).Model().ChildrenIds, DescriptionBlockId)

		InitTemplate(s, WithDescription)

		assert.Contains(t, s.Pick(HeaderLayoutId).Model().ChildrenIds, DescriptionBlockId)
	})
}
