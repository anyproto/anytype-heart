package main

// The Widget snapshot is how a sidebar reaches a space installed as an
// experience: CreateObjectsForExperience reads only spaceDashboardId off the
// profile, so profile.widgets never becomes a widget. Everything asserted here
// is a shape the importer requires and fails silently without — a dropped
// widget produces no error anywhere.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func sampleIndex() *anyblockjson.Index {
	return &anyblockjson.Index{
		Name:       "OKRs & Goals",
		Entrypoint: "page-okr-hub",
		Widgets: []anyblockjson.Widget{
			{Target: "page-okr-hub", Layout: "tree"},
			{Target: "type-objective", Layout: "view", Limit: 6},
			{Target: "_favorite", Layout: "compact_list"},
			{Target: "chat-goal-proposals"}, // no layout: link, the zero value
		},
	}
}

// blockIndex maps a snapshot's blocks by id, and returns the root — the block
// carrying smartblock content, which is the one anymark.AddRootBlock renames
// to the derived widgets id.
func blockIndex(t *testing.T, snap *model.SmartBlockSnapshotBase) (map[string]*model.Block, *model.Block) {
	t.Helper()
	byId := make(map[string]*model.Block, len(snap.Blocks))
	var root *model.Block
	for _, b := range snap.Blocks {
		require.NotContains(t, byId, b.Id, "block ids must be unique within the snapshot")
		byId[b.Id] = b
		if b.GetSmartblock() != nil {
			require.Nil(t, root, "exactly one block may carry smartblock content")
			root = b
		}
	}
	require.NotNil(t, root, "without a smartblock block AddRootBlock appends a second root and orphans every wrapper")
	return byId, root
}

func TestBuildWidgets_Shape(t *testing.T) {
	idx := sampleIndex()
	snap, err := buildWidgets(idx)
	require.NoError(t, err)
	require.NotNil(t, snap)

	byId, root := blockIndex(t, snap)
	assert.Equal(t, widgetsObjectId, root.Id, "the root is the snapshot's own id until the importer renames it")
	require.Len(t, snap.Blocks, 1+2*len(idx.Widgets), "root, plus a wrapper and a link per widget")

	// root children order is sidebar order: updateWidgetObject walks
	// state.Blocks(), a BFS from the root, and addWidgetBlock appends each
	// widget to the live object in that order
	require.Len(t, root.ChildrenIds, len(idx.Widgets))

	wantLayouts := []model.BlockContentWidgetLayout{
		model.BlockContentWidget_Tree,
		model.BlockContentWidget_View,
		model.BlockContentWidget_CompactList,
		model.BlockContentWidget_Link,
	}
	for i, w := range idx.Widgets {
		wrapper := byId[root.ChildrenIds[i]]
		require.NotNil(t, wrapper, "every root child must exist: an unreachable block is dropped by the BFS")

		widget := wrapper.GetWidget()
		require.NotNil(t, widget, "a root child of the widget object is a wrapper")
		assert.Equal(t, wantLayouts[i], widget.Layout, "widgets[%d] layout", i)
		assert.Equal(t, w.Limit, widget.Limit, "widgets[%d] limit", i)
		assert.Empty(t, widget.ViewId)
		assert.False(t, widget.AutoAdded)

		// addWidgetBlock reads ChildrenIds[0] and ignores the rest
		require.Len(t, wrapper.ChildrenIds, 1, "a wrapper carries exactly one link")
		link := byId[wrapper.ChildrenIds[0]]
		require.NotNil(t, link)
		require.NotNil(t, link.GetLink())
		assert.Equal(t, anyblockjson.WireWidgetTarget(w.Target), link.GetLink().TargetBlockId,
			"widgets[%d] target", i)
		assert.Empty(t, link.ChildrenIds)

		assert.Equal(t, link.Id+widgetWrapperSuffix, wrapper.Id,
			"the wrapper id convention core/block/editor/widget uses for stable wrappers")
	}

	// the object itself: a hidden dashboard, as an app export writes it
	assert.Equal(t, widgetsObjectId, snap.Details.GetFields()[detailID].GetStringValue())
	assert.Equal(t, float64(model.ObjectType_dashboard), snap.Details.GetFields()[detailLayout].GetNumberValue())
	assert.True(t, snap.Details.GetFields()[detailIsHidden].GetBoolValue())
	assert.Equal(t, []string{"ot-dashboard"}, snap.ObjectTypes)
}

// Reordering index.json reorders the sidebar and nothing else.
func TestBuildWidgets_PreservesIndexOrder(t *testing.T) {
	targets := func(idx *anyblockjson.Index) []string {
		snap, err := buildWidgets(idx)
		require.NoError(t, err)
		byId, root := blockIndex(t, snap)
		var out []string
		for _, id := range root.ChildrenIds {
			out = append(out, byId[byId[id].ChildrenIds[0]].GetLink().TargetBlockId)
		}
		return out
	}

	forward := &anyblockjson.Index{Widgets: []anyblockjson.Widget{
		{Target: "a"}, {Target: "b"}, {Target: "c"},
	}}
	reversed := &anyblockjson.Index{Widgets: []anyblockjson.Widget{
		{Target: "c"}, {Target: "b"}, {Target: "a"},
	}}
	assert.Equal(t, []string{"a", "b", "c"}, targets(forward))
	assert.Equal(t, []string{"c", "b", "a"}, targets(reversed))
}

// Ids are seeded on the widget's position, not just its target, so a bundle
// that lists the same target twice still gets two distinct wrappers rather
// than one wrapper the state silently collapses.
func TestBuildWidgets_RepeatedTargetKeepsIdsUnique(t *testing.T) {
	snap, err := buildWidgets(&anyblockjson.Index{Widgets: []anyblockjson.Widget{
		{Target: "page-home", Layout: "tree"},
		{Target: "page-home", Layout: "link"},
	}})
	require.NoError(t, err)

	// blockIndex asserts uniqueness across every block
	_, root := blockIndex(t, snap)
	require.Len(t, root.ChildrenIds, 2)
	assert.NotEqual(t, root.ChildrenIds[0], root.ChildrenIds[1])
}

// Every block id is 24 hex characters, the shape bson.NewObjectId().Hex()
// gives every block id in an app export — and derived, so re-converting an
// unchanged bundle produces identical bytes.
func TestBuildWidgets_IdsAreStableAndBsonShaped(t *testing.T) {
	first, err := buildWidgets(sampleIndex())
	require.NoError(t, err)
	second, err := buildWidgets(sampleIndex())
	require.NoError(t, err)
	assert.Equal(t, first.Blocks, second.Blocks, "conversion must be deterministic across runs")

	_, root := blockIndex(t, first)
	for _, wrapperId := range root.ChildrenIds {
		linkId := wrapperId[:len(wrapperId)-len(widgetWrapperSuffix)]
		assert.Len(t, linkId, 24)
		assert.Regexp(t, "^[0-9a-f]{24}$", linkId)
	}
}

// Nothing to show means no snapshot, rather than a widget object with an empty
// sidebar.
func TestBuildWidgets_NoWidgets(t *testing.T) {
	snap, err := buildWidgets(&anyblockjson.Index{Name: "X", Entrypoint: "page-home"})
	require.NoError(t, err)
	assert.Nil(t, snap)
}

func TestBuildWidgets_UnknownLayout(t *testing.T) {
	_, err := buildWidgets(&anyblockjson.Index{Widgets: []anyblockjson.Widget{
		{Target: "a", Layout: "grid"},
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown layout")
}

// The snapshot has to land where core/block/import/pb finds it, as a
// pb.SnapshotWithType carrying sbType Widget — that type is what
// shouldImportSnapshot admits on an EXPERIENCE import.
func TestWriteWidgets_OnDisk(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeWidgets(dir, sampleIndex(), formatPb))

	data, err := os.ReadFile(filepath.Join(dir, "objects", widgetsObjectId+".pb"))
	require.NoError(t, err)
	sw := &pb.SnapshotWithType{}
	require.NoError(t, proto.Unmarshal(data, sw))
	assert.Equal(t, model.SmartBlockType_Widget, sw.SbType)
	require.NotNil(t, sw.Snapshot.Data)
	assert.Len(t, sw.Snapshot.Data.Blocks, 1+2*len(sampleIndex().Widgets))

	t.Run("a bundle with no widgets writes nothing", func(t *testing.T) {
		empty := t.TempDir()
		require.NoError(t, writeWidgets(empty, &anyblockjson.Index{Name: "X"}, formatPb))
		_, err := os.Stat(filepath.Join(empty, "objects"))
		assert.True(t, os.IsNotExist(err))
	})
}

// The four listings the importer knows are bare words
// (widget.IsPredefinedWidgetTargetId); the format spells them in the platform
// namespace so a bundle object cannot shadow them. This is the boundary where
// the prefix has to come back off, and getting it wrong is worse than the bug
// it replaces: handleLinkBlock rewrites a target it does not recognise to
// addr.MissingObject, and WidgetObject.Init then strips the link AND its
// wrapper, so the widget disappears with nothing logged as an error.
func TestBuildWidgets_ReservedTargetsAreWrittenInTheImporterSpelling(t *testing.T) {
	idx := &anyblockjson.Index{Widgets: []anyblockjson.Widget{
		{Target: "_favorite"}, {Target: "_recent"}, {Target: "_set"}, {Target: "_collection"},
		{Target: "page-home"},
	}}
	snap, err := buildWidgets(idx)
	require.NoError(t, err)

	var targets []string
	for _, b := range snap.Blocks {
		if l := b.GetLink(); l != nil {
			targets = append(targets, l.TargetBlockId)
		}
	}
	assert.Equal(t, []string{"favorite", "recent", "set", "collection", "page-home"}, targets)
	for _, target := range targets[:4] {
		assert.True(t, widget.IsPredefinedWidgetTargetId(target),
			"handleLinkBlock leaves a target alone only for these: %q", target)
	}
	assert.False(t, widget.IsPredefinedWidgetTargetId("_favorite"),
		"the untranslated spelling is exactly what the importer does NOT know")
}
