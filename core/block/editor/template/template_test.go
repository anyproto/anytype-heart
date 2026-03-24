package template

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestWithBookmarkBlocks(t *testing.T) {
	t.Run("empty object - no relation blocks injected", func(t *testing.T) {
		s := state.NewDoc("test", nil).NewState()
		s.Add(simple.New(&model.Block{Id: "test"}))

		WithBookmarkBlocks(s)

		want := []*model.Block{
			{Id: "test"},
		}

		assert.Equal(t, want, s.Blocks())
	})

	t.Run("extra blocks - no relation blocks injected", func(t *testing.T) {
		s := state.NewDoc("test", nil).NewState()
		s.Add(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"extra1"}}))
		s.Add(simple.New(&model.Block{Id: "extra1"}))

		WithBookmarkBlocks(s)

		want := []*model.Block{
			{Id: "test", ChildrenIds: []string{"extra1"}},
			{Id: "extra1"},
		}

		assert.Equal(t, want, s.Blocks())
	})
}
