package personalfavorites

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestDiffEntry(t *testing.T) {
	base := WidgetEntry{
		Id:       "link1",
		SpaceId:  "space1",
		TargetId: "target1",
		Layout:   model.BlockContentWidget_Link,
		Limit:    10,
		ViewId:   "view1",
		AfterId:  "prev",
	}

	t.Run("no change", func(t *testing.T) {
		update, changed := diffEntry(base, base)
		assert.False(t, changed)
		assert.Nil(t, update.Layout)
		assert.Nil(t, update.Limit)
		assert.Nil(t, update.ViewId)
		assert.Nil(t, update.AfterId)
	})

	t.Run("layout only", func(t *testing.T) {
		desired := base
		desired.Layout = model.BlockContentWidget_List

		update, changed := diffEntry(base, desired)

		require.True(t, changed)
		require.NotNil(t, update.Layout)
		assert.Equal(t, model.BlockContentWidget_List, *update.Layout)
		assert.Nil(t, update.Limit)
		assert.Nil(t, update.ViewId)
		assert.Nil(t, update.AfterId)
	})

	t.Run("limit only", func(t *testing.T) {
		desired := base
		desired.Limit = 20

		update, changed := diffEntry(base, desired)

		require.True(t, changed)
		require.NotNil(t, update.Limit)
		assert.Equal(t, int32(20), *update.Limit)
	})

	t.Run("afterId only (reorder)", func(t *testing.T) {
		desired := base
		desired.AfterId = "other"

		update, changed := diffEntry(base, desired)

		require.True(t, changed)
		require.NotNil(t, update.AfterId)
		assert.Equal(t, "other", *update.AfterId)
	})

	t.Run("every field at once", func(t *testing.T) {
		desired := WidgetEntry{
			Id:       base.Id,
			SpaceId:  base.SpaceId,
			TargetId: base.TargetId,
			Layout:   model.BlockContentWidget_Tree,
			Limit:    99,
			ViewId:   "view2",
			AfterId:  "root",
		}

		update, changed := diffEntry(base, desired)

		require.True(t, changed)
		require.NotNil(t, update.Layout)
		require.NotNil(t, update.Limit)
		require.NotNil(t, update.ViewId)
		require.NotNil(t, update.AfterId)
	})
}

func TestExtractEntriesFromState(t *testing.T) {
	t.Run("empty state returns nil", func(t *testing.T) {
		st := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root"}),
		}).NewState()

		got := extractEntriesFromState(st, "spaceA")

		assert.Nil(t, got)
	})

	t.Run("linear chain of two widgets", func(t *testing.T) {
		st := buildState(t,
			widgetBlock{wrapperId: "w1", linkId: "l1", target: "targetA", layout: model.BlockContentWidget_Link, limit: 5, view: "vA"},
			widgetBlock{wrapperId: "w2", linkId: "l2", target: "targetB", layout: model.BlockContentWidget_List, limit: 10, view: "vB"},
		)

		got := extractEntriesFromState(st, "spaceA")

		want := []WidgetEntry{
			{Id: "l1", SpaceId: "spaceA", TargetId: "targetA", Layout: model.BlockContentWidget_Link, Limit: 5, ViewId: "vA", AfterId: ""},
			{Id: "l2", SpaceId: "spaceA", TargetId: "targetB", Layout: model.BlockContentWidget_List, Limit: 10, ViewId: "vB", AfterId: "l1"},
		}
		assert.Equal(t, want, got)
	})

	t.Run("non-widget siblings are dropped and don't enter the AfterId chain", func(t *testing.T) {
		st := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"stray", "w1"}}),
			"stray": simple.New(&model.Block{Id: "stray", Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: "i don't belong here"},
			}}),
			"w1": simple.New(&model.Block{Id: "w1", ChildrenIds: []string{"l1"}, Content: &model.BlockContentOfWidget{
				Widget: &model.BlockContentWidget{Layout: model.BlockContentWidget_Link},
			}}),
			"l1": simple.New(&model.Block{Id: "l1", Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{TargetBlockId: "targetA"},
			}}),
		}).NewState()

		got := extractEntriesFromState(st, "spaceA")

		require.Len(t, got, 1)
		assert.Equal(t, "l1", got[0].Id)
		assert.Empty(t, got[0].AfterId)
	})

	t.Run("wrapper with no child is skipped", func(t *testing.T) {
		st := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"w1"}}),
			"w1": simple.New(&model.Block{Id: "w1", Content: &model.BlockContentOfWidget{
				Widget: &model.BlockContentWidget{Layout: model.BlockContentWidget_Link},
			}}),
		}).NewState()

		got := extractEntriesFromState(st, "spaceA")

		assert.Nil(t, got)
	})
}

type widgetBlock struct {
	wrapperId string
	linkId    string
	target    string
	layout    model.BlockContentWidgetLayout
	limit     int32
	view      string
}

func buildState(t *testing.T, widgets ...widgetBlock) *state.State {
	t.Helper()
	rootChildren := make([]string, 0, len(widgets))
	blocks := map[string]simple.Block{}
	for _, w := range widgets {
		rootChildren = append(rootChildren, w.wrapperId)
		blocks[w.wrapperId] = simple.New(&model.Block{
			Id:          w.wrapperId,
			ChildrenIds: []string{w.linkId},
			Content: &model.BlockContentOfWidget{
				Widget: &model.BlockContentWidget{
					Layout: w.layout,
					Limit:  w.limit,
					ViewId: w.view,
				},
			},
		})
		blocks[w.linkId] = simple.New(&model.Block{
			Id: w.linkId,
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{TargetBlockId: w.target},
			},
		})
	}
	blocks["root"] = simple.New(&model.Block{Id: "root", ChildrenIds: rootChildren})
	return state.NewDoc("root", blocks).NewState()
}
