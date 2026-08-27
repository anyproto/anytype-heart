package anyblockjson

// widgetobject_test.go — the sidebar's object, and why a bundle carries
// index.widgets instead of one (§2c).

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// two ids the shape every object id in a real export has (isObjectIdShaped)
const (
	widgetTargetA = "bafyreidft7aqr2fgy6g57hme4rmcdkynf24cd2jfhlyr3duxjevj6vewsu"
	widgetTargetB = "bafyreicy4lwi5kigqgfclic3qtqvxeuu5rgtb4edhwqwcswmpdkghutxk4"
)

func widgetWrapper(id, target string, wc *model.BlockContentWidget, lc *model.BlockContentLink) []*model.Block {
	if wc == nil {
		wc = &model.BlockContentWidget{}
	}
	if lc == nil {
		lc = &model.BlockContentLink{}
	}
	lc.TargetBlockId = target
	return []*model.Block{
		{Id: id, ChildrenIds: []string{id + "-link"},
			Content: &model.BlockContentOfWidget{Widget: wc}},
		{Id: id + "-link", Content: &model.BlockContentOfLink{Link: lc}},
	}
}

// widgetSnapshot assembles a widget object the way a real export holds one:
// a smartblock root over wrapper-and-link pairs, with the constant details
// every corpus document carries.
func widgetSnapshot(extraDetails map[string]*types.Value, pairs ...[]*model.Block) *model.SmartBlockSnapshotBase {
	root := &model.Block{Id: "widget-root",
		Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}
	blocks := []*model.Block{root}
	for _, pair := range pairs {
		root.ChildrenIds = append(root.ChildrenIds, pair[0].Id)
		blocks = append(blocks, pair...)
	}
	det := map[string]*types.Value{
		"id": str("widget-root"), "isHidden": boolean(true),
		"layout":         num(float64(model.ObjectType_dashboard)),
		"resolvedLayout": num(float64(model.ObjectType_dashboard)),
		"createdDate":    num(0),
	}
	for k, v := range extraDetails {
		det[k] = v
	}
	return &model.SmartBlockSnapshotBase{Blocks: blocks, Details: fields(det),
		ObjectTypes: []string{"ot-dashboard"}}
}

// Measured over a 77-space export, the widget object reduces to exactly what
// index.json now states: 218 wrapper-and-link pairs in perfect regularity,
// the auto-widget ledger and switch, the constant hidden-dashboard details,
// and the object's own timestamps. The predicate is FAIL-CLOSED: a member
// this package cannot account for keeps the document.
//
// How this can fail: make the default arm return true and an unaccounted
// detail disappears with the document; drop a block-shape check and a
// sidebar richer than the pair is silently flattened into the index.
func TestWidgetObject_OmittedOnlyWhenTheIndexSaysItAll(t *testing.T) {
	t.Run("a plain widget object is omitted", func(t *testing.T) {
		assert.True(t, OmittedWidgetObject(model.SmartBlockType_Widget, widgetSnapshot(nil,
			widgetWrapper("w1", widgetTargetA,
				&model.BlockContentWidget{Layout: model.BlockContentWidget_Tree, Limit: 6}, nil),
			widgetWrapper("w2", "chat", nil, nil))))
	})

	t.Run("only the widget kind is eligible", func(t *testing.T) {
		assert.False(t, OmittedWidgetObject(model.SmartBlockType_Page, widgetSnapshot(nil)))
	})

	t.Run("the full member set is accounted for", func(t *testing.T) {
		assert.True(t, OmittedWidgetObject(model.SmartBlockType_Widget, widgetSnapshot(
			map[string]*types.Value{
				"autoWidgetTargets":  strList("bin", widgetTargetA),
				"autoWidgetDisabled": boolean(true),
				"lastModifiedDate":   num(1.7e9),
				"name":               str(""),
			},
			widgetWrapper("w1", widgetTargetA,
				&model.BlockContentWidget{Layout: model.BlockContentWidget_View, Limit: 6,
					ViewId: "view-1", AutoAdded: true},
				&model.BlockContentLink{CardStyle: model.BlockContentLink_Card,
					IconSize:    model.BlockContentLink_SizeMedium,
					Description: model.BlockContentLink_Content,
					Relations:   []string{"name"}}))))
	})

	t.Run("the editor's header scaffolding is accepted, empty title and binding included", func(t *testing.T) {
		// 11 of 77 corpus documents carry it: a Header layout over one EMPTY
		// title block whose fields hold the editor's `_detailsKey` binding.
		// §7 drops all of it from every document, so nothing is lost.
		snap := widgetSnapshot(nil, widgetWrapper("w1", widgetTargetA, nil, nil))
		title := &model.Block{Id: "title", Fields: fields(map[string]*types.Value{
			"_detailsKey": strList("name", "done")}),
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Style: model.BlockContentText_Title}}}
		header := &model.Block{Id: "header", ChildrenIds: []string{"title"},
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Header}}}
		snap.Blocks = append(snap.Blocks, header, title)
		snap.Blocks[0].ChildrenIds = append([]string{"header"}, snap.Blocks[0].ChildrenIds...)
		assert.True(t, OmittedWidgetObject(model.SmartBlockType_Widget, snap))

		t.Run("but a title with text is content", func(t *testing.T) {
			title.GetText().Text = "My sidebar"
			assert.False(t, OmittedWidgetObject(model.SmartBlockType_Widget, snap),
				"fail closed: a widget object with real text must travel")
		})
	})

	refusals := map[string]*model.SmartBlockSnapshotBase{
		"an unforeseen detail": widgetSnapshot(map[string]*types.Value{
			"somethingNobodyPlannedFor": str("x")}),
		"a non-empty name": widgetSnapshot(map[string]*types.Value{
			"name": str("My sidebar")}),
		"a layout other than dashboard": widgetSnapshot(map[string]*types.Value{
			"layout": num(float64(model.ObjectType_basic))}),
		// the corpus's two strays: bare words no client constant defines.
		// Written into an index they would read as object ids naming
		// nothing, and the widget would be dropped on install with no error
		"a target the index cannot spell": widgetSnapshot(nil,
			widgetWrapper("w1", "lists", nil, nil)),
		"a dangling target": widgetSnapshot(nil,
			widgetWrapper("w1", "_missing_object", nil, nil)),
		"a ledger entry the index cannot spell": widgetSnapshot(map[string]*types.Value{
			"autoWidgetTargets": strList("bookmark")}),
		"a limit outside the index schema's range": widgetSnapshot(nil,
			widgetWrapper("w1", widgetTargetA, &model.BlockContentWidget{Limit: 101}, nil)),
		"a wrapper attribute the pair cannot carry": widgetSnapshot(nil,
			func() []*model.Block {
				pair := widgetWrapper("w1", widgetTargetA, nil, nil)
				pair[0].BackgroundColor = "red"
				return pair
			}()),
	}
	for name, snap := range refusals {
		t.Run(name+" keeps the document", func(t *testing.T) {
			assert.False(t, OmittedWidgetObject(model.SmartBlockType_Widget, snap))
		})
	}

	t.Run("a wrapper with two children keeps the document", func(t *testing.T) {
		snap := widgetSnapshot(nil, widgetWrapper("w1", widgetTargetA, nil, nil))
		extra := &model.Block{Id: "second-link",
			Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: widgetTargetB}}}
		snap.Blocks = append(snap.Blocks, extra)
		snap.Blocks[1].ChildrenIds = append(snap.Blocks[1].ChildrenIds, "second-link")
		assert.False(t, OmittedWidgetObject(model.SmartBlockType_Widget, snap),
			"addWidgetBlock reads ChildrenIds[0] and ignores the rest, so the rest is content the lift would lose")
	})

	t.Run("an unreachable block keeps the document", func(t *testing.T) {
		snap := widgetSnapshot(nil, widgetWrapper("w1", widgetTargetA, nil, nil))
		snap.Blocks = append(snap.Blocks, textBlock("stray", model.BlockContentText_Paragraph, "note"))
		assert.False(t, OmittedWidgetObject(model.SmartBlockType_Widget, snap))
	})

	t.Run("a widget block that is not a root child keeps the document", func(t *testing.T) {
		snap := widgetSnapshot(nil)
		snap.Blocks[0].ChildrenIds = []string{"p"}
		snap.Blocks = append(snap.Blocks,
			&model.Block{Id: "p", ChildrenIds: []string{"w1"},
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}})
		snap.Blocks = append(snap.Blocks, widgetWrapper("w1", widgetTargetA, nil, nil)...)
		assert.False(t, OmittedWidgetObject(model.SmartBlockType_Widget, snap))
	})
}

// The lift is one place saying which block member becomes which index field,
// so a composer cannot drop the document while carrying fewer of them than
// the omission assumed were carried.
func TestIndexFromWidgetObject(t *testing.T) {
	snap := widgetSnapshot(
		map[string]*types.Value{
			"autoWidgetTargets":  strList("bin", "favorite", widgetTargetB),
			"autoWidgetDisabled": boolean(true),
		},
		widgetWrapper("w1", widgetTargetA,
			&model.BlockContentWidget{Layout: model.BlockContentWidget_View, Limit: 6,
				ViewId: "view-1", AutoAdded: true},
			&model.BlockContentLink{CardStyle: model.BlockContentLink_Card,
				IconSize:    model.BlockContentLink_SizeMedium,
				Description: model.BlockContentLink_Content,
				Relations:   []string{"name"}}),
		widgetWrapper("w2", "chat", &model.BlockContentWidget{Limit: 6}, nil))

	var idx Index
	IndexFromWidgetObject(&idx, snap)

	want := []Widget{
		{Target: widgetTargetA, Layout: "view", Limit: 6, ViewId: "view-1", AutoAdded: true,
			CardStyle: "card", IconSize: "medium", Description: "content", Properties: []string{"name"}},
		// the wire word is translated into the `_` namespace; the defaults
		// (link layout, text card style, …) stay empty, the §4 omit canon
		{Target: "_chat", Limit: 6},
	}
	assert.Equal(t, want, idx.Widgets)
	assert.Equal(t, []string{"_bin", "_favorite", widgetTargetB}, idx.AutoWidgetTargets,
		"ledger entries are targets and get the same translation")
	assert.True(t, idx.AutoWidgetDisabled)
}

// WidgetsSnapshot is the builder both cmd/anyblockconvert and the round-trip
// verifier use; the lift must read its output back into the very index it
// was built from, or the two sides have drifted.
func TestWidgetsSnapshot_TheLiftReadsItBack(t *testing.T) {
	idx := &Index{
		Widgets: []Widget{
			{Target: widgetTargetA, Layout: "tree", Limit: 6},
			{Target: "_chat", Limit: 6, AutoAdded: true},
			{Target: "_all_objects", CardStyle: "card", IconSize: "medium",
				Description: "content", Properties: []string{"name"}},
			{Target: widgetTargetB, Layout: "view", ViewId: "view-9"},
		},
		AutoWidgetTargets:  []string{"_bin", widgetTargetA},
		AutoWidgetDisabled: true,
	}
	snap, err := WidgetsSnapshot(idx)
	require.NoError(t, err)
	require.NotNil(t, snap)

	assert.True(t, OmittedWidgetObject(model.SmartBlockType_Widget, snap),
		"the omission predicate must admit the very snapshot the rebuild writes")

	var back Index
	IndexFromWidgetObject(&back, snap)
	assert.Equal(t, idx.Widgets, back.Widgets)
	assert.Equal(t, idx.AutoWidgetTargets, back.AutoWidgetTargets)
	assert.Equal(t, idx.AutoWidgetDisabled, back.AutoWidgetDisabled)

	t.Run("deterministic across runs", func(t *testing.T) {
		again, err := WidgetsSnapshot(idx)
		require.NoError(t, err)
		assert.Equal(t, snap.Blocks, again.Blocks,
			"re-converting an unchanged bundle must produce identical bytes")
	})
}

// Nothing to show means no snapshot — but the ledger alone is something to
// show: it is the state that stops a restored client re-adding widgets the
// user deleted.
func TestWidgetsSnapshot_Empty(t *testing.T) {
	snap, err := WidgetsSnapshot(&Index{Name: "X", Entrypoint: "page-home"})
	require.NoError(t, err)
	assert.Nil(t, snap)

	snap, err = WidgetsSnapshot(&Index{AutoWidgetDisabled: true})
	require.NoError(t, err)
	require.NotNil(t, snap)
	assert.True(t, snap.Details.GetFields()["autoWidgetDisabled"].GetBoolValue())
	require.Len(t, snap.Blocks, 1, "just the root; there are no widgets to hang under it")
}

func TestWidgetsSnapshot_UnknownVocabulary(t *testing.T) {
	for name, w := range map[string]Widget{
		"layout":      {Target: "a", Layout: "grid"},
		"card_style":  {Target: "a", CardStyle: "poster"},
		"icon_size":   {Target: "a", IconSize: "huge"},
		"description": {Target: "a", Description: "excerpt"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := WidgetsSnapshot(&Index{Widgets: []Widget{w}})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown")
		})
	}
}

// The residual predicate is what the round-trip comparator consults, so its
// scope is pinned: the two object timestamps whatever their value, a name
// only when EMPTY — a non-empty name keeps the whole document instead, and
// the comparator must report it if it ever goes missing anyway.
func TestWidgetObjectResidualKey(t *testing.T) {
	assert.True(t, WidgetObjectResidualKey("createdDate", num(0)))
	assert.True(t, WidgetObjectResidualKey("lastModifiedDate", num(1.7e9)))
	assert.True(t, WidgetObjectResidualKey("name", str("")))
	assert.False(t, WidgetObjectResidualKey("name", str("My sidebar")))
	assert.False(t, WidgetObjectResidualKey("autoWidgetTargets", strList("bin")),
		"the ledger is lifted state, not residue: it travels in the index and comes back")
	assert.False(t, WidgetObjectResidualKey("isHidden", boolean(true)))
}
