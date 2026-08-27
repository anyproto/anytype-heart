package main

// The Widget snapshot is how a sidebar reaches a space installed as an
// experience: CreateObjectsForExperience reads only spaceDashboardId off the
// profile, so profile.widgets never becomes a widget. Everything asserted here
// is a shape the importer requires and fails silently without — a dropped
// widget produces no error anywhere. The builder itself is
// anyblockjson.WidgetsSnapshot (shared with the round-trip verifier's
// reconstruction check); what this file pins is the tool's half: the archive
// placement and the importer-facing contract.

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
			{Target: "type-objective", Layout: "view", Limit: 6, ViewId: "view-board"},
			{Target: "_favorite", Layout: "compact_list"},
			{Target: "chat-goal-proposals", CardStyle: "card", IconSize: "medium",
				Description: "content", Properties: []string{"name"}},
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

func TestWidgetsSnapshot_Shape(t *testing.T) {
	idx := sampleIndex()
	snap, err := anyblockjson.WidgetsSnapshot(idx)
	require.NoError(t, err)
	require.NotNil(t, snap)

	byId, root := blockIndex(t, snap)
	assert.Equal(t, anyblockjson.WidgetsObjectId, root.Id, "the root is the snapshot's own id until the importer renames it")
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

		wc := wrapper.GetWidget()
		require.NotNil(t, wc, "a root child of the widget object is a wrapper")
		assert.Equal(t, wantLayouts[i], wc.Layout, "widgets[%d] layout", i)
		assert.Equal(t, w.Limit, wc.Limit, "widgets[%d] limit", i)
		assert.Equal(t, w.ViewId, wc.ViewId, "widgets[%d] view_id", i)
		assert.Equal(t, w.AutoAdded, wc.AutoAdded, "widgets[%d] auto_added", i)

		// addWidgetBlock reads ChildrenIds[0] and ignores the rest
		require.Len(t, wrapper.ChildrenIds, 1, "a wrapper carries exactly one link")
		link := byId[wrapper.ChildrenIds[0]]
		require.NotNil(t, link)
		require.NotNil(t, link.GetLink())
		assert.Equal(t, anyblockjson.WireWidgetTarget(w.Target), link.GetLink().TargetBlockId,
			"widgets[%d] target", i)
		assert.Empty(t, link.ChildrenIds)

		assert.Equal(t, link.Id+"-wrapper", wrapper.Id,
			"the wrapper id convention core/block/editor/widget uses for stable wrappers")
	}

	// the link child's display members ride on the last widget
	last := byId[byId[root.ChildrenIds[3]].ChildrenIds[0]].GetLink()
	assert.Equal(t, model.BlockContentLink_Card, last.CardStyle)
	assert.Equal(t, model.BlockContentLink_SizeMedium, last.IconSize)
	assert.Equal(t, model.BlockContentLink_Content, last.Description)
	assert.Equal(t, []string{"name"}, last.Relations)

	// the object itself: a hidden dashboard, as an app export writes it
	assert.Equal(t, anyblockjson.WidgetsObjectId, snap.Details.GetFields()[detailID].GetStringValue())
	assert.Equal(t, float64(model.ObjectType_dashboard), snap.Details.GetFields()[detailLayout].GetNumberValue())
	assert.True(t, snap.Details.GetFields()[detailIsHidden].GetBoolValue())
	assert.Equal(t, []string{"ot-dashboard"}, snap.ObjectTypes)
}

// The snapshot has to land where core/block/import/pb finds it, as a
// pb.SnapshotWithType carrying sbType Widget — that type is what
// shouldImportSnapshot admits on an EXPERIENCE import.
func TestWriteWidgets_OnDisk(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeWidgets(dir, sampleIndex(), formatPb))

	data, err := os.ReadFile(filepath.Join(dir, "objects", anyblockjson.WidgetsObjectId+".pb"))
	require.NoError(t, err)
	sw := &pb.SnapshotWithType{}
	require.NoError(t, proto.Unmarshal(data, sw))
	assert.Equal(t, model.SmartBlockType_Widget, sw.SbType)
	require.NotNil(t, sw.Snapshot.Data)
	assert.Len(t, sw.Snapshot.Data.Blocks, 1+2*len(sampleIndex().Widgets))

	t.Run("a bundle with no sidebar state writes nothing", func(t *testing.T) {
		empty := t.TempDir()
		require.NoError(t, writeWidgets(empty, &anyblockjson.Index{Name: "X"}, formatPb))
		_, err := os.Stat(filepath.Join(empty, "objects"))
		assert.True(t, os.IsNotExist(err))
	})

	// the auto-widget ledger is sidebar state: a bundle carrying only it
	// still writes the snapshot, details and all, so the state is in the
	// archive the day the importer starts reading it
	t.Run("the auto-widget ledger alone still writes the snapshot", func(t *testing.T) {
		ledger := t.TempDir()
		require.NoError(t, writeWidgets(ledger,
			&anyblockjson.Index{Name: "X", AutoWidgetTargets: []string{"_bin"}}, formatPb))
		data, err := os.ReadFile(filepath.Join(ledger, "objects", anyblockjson.WidgetsObjectId+".pb"))
		require.NoError(t, err)
		sw := &pb.SnapshotWithType{}
		require.NoError(t, proto.Unmarshal(data, sw))
		auto := sw.Snapshot.Data.Details.GetFields()["autoWidgetTargets"]
		require.NotNil(t, auto)
		require.Len(t, auto.GetListValue().GetValues(), 1)
		assert.Equal(t, "bin", auto.GetListValue().GetValues()[0].GetStringValue(),
			"ledger entries are written in the importer's own spelling, like link targets")
	})
}

// The listings the importer knows are bare words
// (widget.IsPredefinedWidgetTargetId); the format spells them in the platform
// namespace so a bundle object cannot shadow them. This is the boundary where
// the prefix has to come back off, and getting it wrong is worse than the bug
// it replaces: handleLinkBlock rewrites a target it does not recognise to
// addr.MissingObject, and WidgetObject.Init then strips the link AND its
// wrapper, so the widget disappears with nothing logged as an error.
func TestWidgetsSnapshot_ReservedTargetsAreWrittenInTheImporterSpelling(t *testing.T) {
	idx := &anyblockjson.Index{}
	for _, target := range anyblockjson.ReservedWidgetTargets() {
		idx.Widgets = append(idx.Widgets, anyblockjson.Widget{Target: target})
	}
	idx.Widgets = append(idx.Widgets, anyblockjson.Widget{Target: "page-home"})
	snap, err := anyblockjson.WidgetsSnapshot(idx)
	require.NoError(t, err)

	var targets []string
	for _, b := range snap.Blocks {
		if l := b.GetLink(); l != nil {
			targets = append(targets, l.TargetBlockId)
		}
	}
	require.Len(t, targets, len(idx.Widgets))
	for i, target := range targets[:len(targets)-1] {
		assert.True(t, widget.IsPredefinedWidgetTargetId(target),
			"handleLinkBlock leaves a target alone only for these: %q (from %q)", target, idx.Widgets[i].Target)
	}
	assert.Equal(t, "page-home", targets[len(targets)-1], "an object id passes through")
	assert.False(t, widget.IsPredefinedWidgetTargetId("_favorite"),
		"the untranslated spelling is exactly what the importer does NOT know")
}
