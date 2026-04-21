package personalfavorites

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/personalfavorites"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestDiffEntry(t *testing.T) {
	base := personalfavorites.WidgetEntry{
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
		desired := personalfavorites.WidgetEntry{
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

		want := []personalfavorites.WidgetEntry{
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

func TestRebuildStateFromEntries(t *testing.T) {
	t.Run("remote layout update is applied", func(t *testing.T) {
		// given: state carrying a wrapper with Layout=Link, a remote update
		// arrives changing it to List. The rebuilt state must reflect List.
		st := buildState(t,
			widgetBlock{wrapperId: "l1" + widgetWrapperSuffix, linkId: "l1", target: "targetA", layout: model.BlockContentWidget_Link, limit: 5, view: "vA"},
		).NewState()
		entries := []personalfavorites.WidgetEntry{
			{Id: "l1", SpaceId: "spaceA", TargetId: "targetA", Layout: model.BlockContentWidget_List, Limit: 5, ViewId: "vA"},
		}

		(&VirtualWidgetObject{}).rebuildStateFromEntries(st, entries)

		got := pickWidget(t, st, "l1"+widgetWrapperSuffix)
		assert.Equal(t, model.BlockContentWidget_List, got.Layout)
	})

	t.Run("remote limit update is applied", func(t *testing.T) {
		st := buildState(t,
			widgetBlock{wrapperId: "l1" + widgetWrapperSuffix, linkId: "l1", target: "targetA", layout: model.BlockContentWidget_Link, limit: 5, view: "vA"},
		).NewState()
		entries := []personalfavorites.WidgetEntry{
			{Id: "l1", SpaceId: "spaceA", TargetId: "targetA", Layout: model.BlockContentWidget_Link, Limit: 99, ViewId: "vA"},
		}

		(&VirtualWidgetObject{}).rebuildStateFromEntries(st, entries)

		got := pickWidget(t, st, "l1"+widgetWrapperSuffix)
		assert.Equal(t, int32(99), got.Limit)
	})

	t.Run("remote viewId update is applied", func(t *testing.T) {
		st := buildState(t,
			widgetBlock{wrapperId: "l1" + widgetWrapperSuffix, linkId: "l1", target: "targetA", layout: model.BlockContentWidget_Link, limit: 5, view: "vA"},
		).NewState()
		entries := []personalfavorites.WidgetEntry{
			{Id: "l1", SpaceId: "spaceA", TargetId: "targetA", Layout: model.BlockContentWidget_Link, Limit: 5, ViewId: "vB"},
		}

		(&VirtualWidgetObject{}).rebuildStateFromEntries(st, entries)

		got := pickWidget(t, st, "l1"+widgetWrapperSuffix)
		assert.Equal(t, "vB", got.ViewId)
	})

	t.Run("entry removal unlinks wrapper from root", func(t *testing.T) {
		st := buildState(t,
			widgetBlock{wrapperId: "l1" + widgetWrapperSuffix, linkId: "l1", target: "targetA", layout: model.BlockContentWidget_Link, limit: 5, view: "vA"},
		).NewState()

		(&VirtualWidgetObject{}).rebuildStateFromEntries(st, nil)

		root := st.Pick(st.RootId())
		require.NotNil(t, root)
		assert.Empty(t, root.Model().ChildrenIds)
	})

	t.Run("new entry adds wrapper and link", func(t *testing.T) {
		st := state.NewDoc("root", map[string]simple.Block{
			"root": simple.New(&model.Block{Id: "root"}),
		}).NewState()
		entries := []personalfavorites.WidgetEntry{
			{Id: "l1", SpaceId: "spaceA", TargetId: "targetA", Layout: model.BlockContentWidget_List, Limit: 7, ViewId: "vA"},
		}

		(&VirtualWidgetObject{}).rebuildStateFromEntries(st, entries)

		root := st.Pick(st.RootId())
		require.NotNil(t, root)
		require.Equal(t, []string{"l1" + widgetWrapperSuffix}, root.Model().ChildrenIds)

		wrapper := pickWidget(t, st, "l1"+widgetWrapperSuffix)
		assert.Equal(t, model.BlockContentWidget_List, wrapper.Layout)
		assert.Equal(t, int32(7), wrapper.Limit)

		link := st.Pick("l1")
		require.NotNil(t, link)
		lc, ok := link.Model().Content.(*model.BlockContentOfLink)
		require.True(t, ok)
		assert.Equal(t, "targetA", lc.Link.TargetBlockId)
	})
}

func pickWidget(t *testing.T, st *state.State, wrapperId string) *model.BlockContentWidget {
	t.Helper()
	b := st.Pick(wrapperId)
	require.NotNil(t, b, "wrapper %s missing", wrapperId)
	wc, ok := b.Model().Content.(*model.BlockContentOfWidget)
	require.True(t, ok, "block %s is not a widget wrapper (%T)", wrapperId, b.Model().Content)
	return wc.Widget
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

func buildDoc(widgets ...widgetBlock) state.Doc {
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
	return state.NewDoc("root", blocks)
}

// stubService is a minimal personalfavorites.Service for tests. It records
// Create/Delete/Update calls and answers GetWidgets via getWidgetsFn.
type stubService struct {
	getWidgetsFn func(ctx context.Context, spaceId string) ([]personalfavorites.WidgetEntry, error)

	creates []personalfavorites.WidgetEntry
	deletes []string
	updates []stubUpdate
}

type stubUpdate struct {
	id     string
	update personalfavorites.WidgetUpdate
}

var _ personalfavorites.Service = (*stubService)(nil)

func (s *stubService) Init(a *app.App) error { return nil }
func (s *stubService) Name() string          { return "stubService" }
func (s *stubService) Subscribe(params personalfavorites.SubscribeParams) (unsubscribe func()) {
	return func() {}
}
func (s *stubService) GetWidgets(ctx context.Context, spaceId string) ([]personalfavorites.WidgetEntry, error) {
	if s.getWidgetsFn != nil {
		return s.getWidgetsFn(ctx, spaceId)
	}
	return nil, nil
}
func (s *stubService) CreateWidget(ctx context.Context, entry personalfavorites.WidgetEntry) error {
	s.creates = append(s.creates, entry)
	return nil
}
func (s *stubService) DeleteWidget(ctx context.Context, id string) error {
	s.deletes = append(s.deletes, id)
	return nil
}
func (s *stubService) UpdateWidget(ctx context.Context, id string, updates personalfavorites.WidgetUpdate) error {
	s.updates = append(s.updates, stubUpdate{id: id, update: updates})
	return nil
}
func (s *stubService) OnStoreUpdate(spaceId string, changes []personalfavorites.WidgetChange) {}

// TestApply_doesNotClobberConcurrentRemoteEntry locks the invariant that
// VW.Apply must not delete an entry VW didn't have, even if the store's
// current view contains one VW hasn't observed yet. Under the original
// snapshot-sync model, syncToStore reads the store's "current" entries and
// deletes anything not in the post-Apply VW state — which propagates as a
// permanent CRDT delete for entries another peer just created.
func TestApply_doesNotClobberConcurrentRemoteEntry(t *testing.T) {
	const (
		spaceId            = "spaceA"
		existingLocalId    = "l1"
		concurrentRemoteId = "l2"
	)

	existingLocal := personalfavorites.WidgetEntry{
		Id: existingLocalId, SpaceId: spaceId, TargetId: "targetA",
		Layout: model.BlockContentWidget_Link, Limit: 5, ViewId: "vA",
	}
	concurrentRemote := personalfavorites.WidgetEntry{
		Id: concurrentRemoteId, SpaceId: spaceId, TargetId: "targetB",
		Layout: model.BlockContentWidget_List, Limit: 10, ViewId: "vB",
		AfterId: existingLocalId,
	}

	stub := &stubService{
		getWidgetsFn: func(context.Context, string) ([]personalfavorites.WidgetEntry, error) {
			return []personalfavorites.WidgetEntry{existingLocal, concurrentRemote}, nil
		},
	}

	// VW's local state only contains existingLocal — the remote hasn't been
	// observed yet.
	sb := smarttest.New("root")
	sb.SetSpaceId(spaceId)
	sb.Doc = buildDoc(widgetBlock{
		wrapperId: existingLocalId + widgetWrapperSuffix,
		linkId:    existingLocalId,
		target:    existingLocal.TargetId,
		layout:    existingLocal.Layout,
		limit:     existingLocal.Limit,
		view:      existingLocal.ViewId,
	})

	v := &VirtualWidgetObject{SmartBlock: sb, service: stub}

	// Caller changes existingLocal's layout from Link to List.
	st := v.NewState()
	wrapperId := existingLocalId + widgetWrapperSuffix
	st.Set(simple.New(&model.Block{
		Id:          wrapperId,
		ChildrenIds: []string{existingLocalId},
		Content: &model.BlockContentOfWidget{
			Widget: &model.BlockContentWidget{
				Layout: model.BlockContentWidget_List,
				Limit:  existingLocal.Limit,
				ViewId: existingLocal.ViewId,
			},
		},
	}))

	require.NoError(t, v.Apply(st))

	// Bug invariant: concurrentRemote must survive — VW never knew about it.
	assert.NotContains(t, stub.deletes, concurrentRemoteId,
		"Apply wrongly deleted a concurrent remote entry it never saw")

	// Expected local op: only the layout change on existingLocal.
	require.Len(t, stub.updates, 1)
	assert.Equal(t, existingLocalId, stub.updates[0].id)
	require.NotNil(t, stub.updates[0].update.Layout)
	assert.Equal(t, model.BlockContentWidget_List, *stub.updates[0].update.Layout)
	assert.Nil(t, stub.updates[0].update.Limit)
	assert.Nil(t, stub.updates[0].update.ViewId)
	assert.Nil(t, stub.updates[0].update.AfterId)

	// No creates were issued — nothing was added locally.
	assert.Empty(t, stub.creates)
}

func TestPushDelta(t *testing.T) {
	const spaceId = "spaceA"

	entry := func(id string, layout model.BlockContentWidgetLayout, limit int32, view, after string) personalfavorites.WidgetEntry {
		return personalfavorites.WidgetEntry{
			Id: id, SpaceId: spaceId, TargetId: "t-" + id,
			Layout: layout, Limit: limit, ViewId: view, AfterId: after,
		}
	}

	newVW := func() (*VirtualWidgetObject, *stubService) {
		stub := &stubService{}
		return &VirtualWidgetObject{service: stub}, stub
	}

	t.Run("create: entry in desired only", func(t *testing.T) {
		v, stub := newVW()
		desired := []personalfavorites.WidgetEntry{
			entry("l1", model.BlockContentWidget_Link, 5, "vA", ""),
		}

		v.pushDelta(nil, desired, spaceId)

		assert.Empty(t, stub.deletes)
		assert.Empty(t, stub.updates)
		require.Len(t, stub.creates, 1)
		assert.Equal(t, "l1", stub.creates[0].Id)
		assert.Equal(t, spaceId, stub.creates[0].SpaceId)
	})

	t.Run("delete: entry in prev only", func(t *testing.T) {
		v, stub := newVW()
		prev := []personalfavorites.WidgetEntry{
			entry("l1", model.BlockContentWidget_Link, 5, "vA", ""),
		}

		v.pushDelta(prev, nil, spaceId)

		assert.Empty(t, stub.creates)
		assert.Empty(t, stub.updates)
		assert.Equal(t, []string{"l1"}, stub.deletes)
	})

	t.Run("update: single field differs", func(t *testing.T) {
		v, stub := newVW()
		prev := []personalfavorites.WidgetEntry{
			entry("l1", model.BlockContentWidget_Link, 5, "vA", ""),
		}
		desired := []personalfavorites.WidgetEntry{
			entry("l1", model.BlockContentWidget_List, 5, "vA", ""),
		}

		v.pushDelta(prev, desired, spaceId)

		assert.Empty(t, stub.creates)
		assert.Empty(t, stub.deletes)
		require.Len(t, stub.updates, 1)
		assert.Equal(t, "l1", stub.updates[0].id)
		require.NotNil(t, stub.updates[0].update.Layout)
		assert.Equal(t, model.BlockContentWidget_List, *stub.updates[0].update.Layout)
		assert.Nil(t, stub.updates[0].update.Limit)
		assert.Nil(t, stub.updates[0].update.ViewId)
		assert.Nil(t, stub.updates[0].update.AfterId)
	})

	t.Run("reorder: only entries whose AfterId changed are updated", func(t *testing.T) {
		// prev order: l1, l2, l3  → desired order: l2, l1, l3
		// l2.AfterId: "" → "" (stays head... wait, l2 was after l1; now l2 is head).
		// So: l2.AfterId "l1" → "",  l1.AfterId "" → "l2",  l3.AfterId "l2" → "l1".
		// All three entries' AfterId changes.
		v, stub := newVW()
		prev := []personalfavorites.WidgetEntry{
			entry("l1", model.BlockContentWidget_Link, 5, "vA", ""),
			entry("l2", model.BlockContentWidget_Link, 5, "vB", "l1"),
			entry("l3", model.BlockContentWidget_Link, 5, "vC", "l2"),
		}
		desired := []personalfavorites.WidgetEntry{
			entry("l2", model.BlockContentWidget_Link, 5, "vB", ""),
			entry("l1", model.BlockContentWidget_Link, 5, "vA", "l2"),
			entry("l3", model.BlockContentWidget_Link, 5, "vC", "l1"),
		}

		v.pushDelta(prev, desired, spaceId)

		assert.Empty(t, stub.creates)
		assert.Empty(t, stub.deletes)
		require.Len(t, stub.updates, 3)
		byId := map[string]personalfavorites.WidgetUpdate{}
		for _, u := range stub.updates {
			byId[u.id] = u.update
		}
		require.NotNil(t, byId["l1"].AfterId)
		assert.Equal(t, "l2", *byId["l1"].AfterId)
		require.NotNil(t, byId["l2"].AfterId)
		assert.Equal(t, "", *byId["l2"].AfterId)
		require.NotNil(t, byId["l3"].AfterId)
		assert.Equal(t, "l1", *byId["l3"].AfterId)
		// No non-AfterId fields should be in any update.
		for _, u := range stub.updates {
			assert.Nil(t, u.update.Layout)
			assert.Nil(t, u.update.Limit)
			assert.Nil(t, u.update.ViewId)
		}
	})

	t.Run("no-op: identical prev and desired", func(t *testing.T) {
		v, stub := newVW()
		entries := []personalfavorites.WidgetEntry{
			entry("l1", model.BlockContentWidget_Link, 5, "vA", ""),
			entry("l2", model.BlockContentWidget_Link, 5, "vB", "l1"),
		}

		v.pushDelta(entries, entries, spaceId)

		assert.Empty(t, stub.creates)
		assert.Empty(t, stub.updates)
		assert.Empty(t, stub.deletes)
	})

	t.Run("empty prev with non-empty desired: only creates", func(t *testing.T) {
		v, stub := newVW()
		desired := []personalfavorites.WidgetEntry{
			entry("l1", model.BlockContentWidget_Link, 5, "vA", ""),
			entry("l2", model.BlockContentWidget_Link, 5, "vB", "l1"),
		}

		v.pushDelta(nil, desired, spaceId)

		assert.Empty(t, stub.updates)
		assert.Empty(t, stub.deletes)
		require.Len(t, stub.creates, 2)
		assert.Equal(t, "l1", stub.creates[0].Id)
		assert.Equal(t, "l2", stub.creates[1].Id)
	})

	t.Run("SpaceId is stamped on creates even if missing from desired", func(t *testing.T) {
		v, stub := newVW()
		e := entry("l1", model.BlockContentWidget_Link, 5, "vA", "")
		e.SpaceId = "" // Simulate extractEntriesFromState being called with a space or an old entry.
		desired := []personalfavorites.WidgetEntry{e}

		v.pushDelta(nil, desired, spaceId)

		require.Len(t, stub.creates, 1)
		assert.Equal(t, spaceId, stub.creates[0].SpaceId)
	})
}
