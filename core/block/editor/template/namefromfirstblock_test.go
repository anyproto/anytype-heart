package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestWithNameFromFirstBlock(t *testing.T) {
	newDoc := func() *state.State {
		doc := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"p1", "p2"}}),
			"p1": simple.New(&model.Block{Id: "p1", ChildrenIds: []string{"c1"},
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "Title line"}}}),
			"c1": simple.New(&model.Block{Id: "c1",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "nested"}}}),
			"p2": simple.New(&model.Block{Id: "p2",
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "body"}}}),
		})
		return doc.NewState()
	}

	t.Run("the first block's children move up to where it was", func(t *testing.T) {
		s := newDoc()

		WithNameFromFirstBlock(s)

		assert.Equal(t, "Title line", s.Details().GetString(bundle.RelationKeyName))
		assert.Equal(t, []string{"c1", "p2"}, s.Pick("root").Model().ChildrenIds)
	})

	t.Run("the children survive when the first block is already mutable in the state", func(t *testing.T) {
		// an earlier edit in the same state (the API's batched PATCH: an
		// update_block, then a set_type) made p1 a mutable copy; unlinking
		// its children then empties the very list the reinsertion reads
		s := newDoc()
		s.Get("p1").Model().GetText().Text = "Edited title"
		require.Equal(t, []string{"c1"}, s.Get("p1").Model().ChildrenIds)

		WithNameFromFirstBlock(s)

		assert.Equal(t, "Edited title", s.Details().GetString(bundle.RelationKeyName))
		assert.Equal(t, []string{"c1", "p2"}, s.Pick("root").Model().ChildrenIds,
			"the nested block must not vanish with its parent")
		assert.NotNil(t, s.Pick("c1"))
	})

	t.Run("an object that already has a name keeps its first block", func(t *testing.T) {
		s := newDoc()
		s.SetDetail(bundle.RelationKeyName, domain.String("Named"))

		WithNameFromFirstBlock(s)

		assert.Equal(t, "Named", s.Details().GetString(bundle.RelationKeyName))
		assert.Equal(t, []string{"p1", "p2"}, s.Pick("root").Model().ChildrenIds)
	})
}
