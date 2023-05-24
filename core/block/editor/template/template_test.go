package template

import (
	"testing"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
)

func TestWithBookmarkBlocks(t *testing.T) {
	requiredBlocks := make([]*model.Block, 0, len(bookmarkRelationKeys))
	for _, k := range bookmarkRelationKeys {
		requiredBlocks = append(requiredBlocks, makeRelationBlock(k))
	}

	t.Run("empty object", func(t *testing.T) {
		s := state.NewDoc("test", nil).NewState()
		s.Add(simple.New(&model.Block{Id: "test"}))

		WithBookmarkBlocks(s)

		want := append([]*model.Block{
			{Id: "test", ChildrenIds: bookmarkRelationKeys},
		}, requiredBlocks...)

		assert.Equal(t, want, s.Blocks())
	})

	t.Run("extra blocks", func(t *testing.T) {
		s := state.NewDoc("test", nil).NewState()
		s.Add(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"extra1"}}))
		s.Add(simple.New(&model.Block{Id: "extra1", ChildrenIds: []string{"extra2", "extra3"}}))
		s.Add(simple.New(&model.Block{Id: "extra2"}))
		s.Add(simple.New(&model.Block{Id: "extra3"}))

		WithBookmarkBlocks(s)

		want := append([]*model.Block{
			{Id: "test", ChildrenIds: append(bookmarkRelationKeys, "extra1")},
		}, append(requiredBlocks,
			&model.Block{Id: "extra1", ChildrenIds: []string{"extra2", "extra3"}},
			&model.Block{Id: "extra2"},
			&model.Block{Id: "extra3"})...)

		assert.Equal(t, want, s.Blocks())
	})

	t.Run("required relation blocks placed in chaotic order", func(t *testing.T) {
		s := state.NewDoc("test", nil).NewState()
		s.Add(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"extra1", "tag"}}))
		s.Add(simple.New(&model.Block{Id: "extra1"}))
		s.Add(simple.New(makeRelationBlock("tag")))

		WithBookmarkBlocks(s)

		want := append([]*model.Block{
			{Id: "test", ChildrenIds: append(bookmarkRelationKeys, "extra1")},
		}, append(requiredBlocks,
			&model.Block{Id: "extra1"})...)

		assert.Equal(t, want, s.Blocks())
	})
}
