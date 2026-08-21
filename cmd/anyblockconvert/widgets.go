package main

// widgets.go writes the archive's Widget snapshot from index.json (§2c) — the
// sidebar, as the path a bundle actually takes carries it.
//
// A bundle is installed with ObjectImportExperience, which reaches
// builtinobjects.CreateObjectsForExperience. That function reads the `profile`
// file for exactly one field, SpaceDashboardId (via setWorkspaceSettings); it
// never calls getWidgets or createWidgets, which belong to inject(), the
// built-in-archive path. So on this path profile.Widgets is inert and the
// sidebar has to arrive the way a real app export carries it: as a snapshot
// with sbType Widget, which core/block/import/pb.shouldImportSnapshot admits
// precisely when the import type is EXPERIENCE.
//
// The snapshot is one root block plus, per widget, a wrapper carrying the
// widget content and a link child carrying the target — the shape
// widget.createBlock builds in a live space, and the shape
// objectcreator.addWidgetBlock reads back on import.

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	// widgetsObjectId is the snapshot's own id. Nothing derives from it: the
	// importer replaces it with the space's derived Widgets id
	// (objectid.widget.GetIDAndPayload returns spc.DerivedIDs().Widgets), and
	// the root block is renamed to match before the state is built. It only has
	// to be stable and not collide with an object in the bundle.
	widgetsObjectId = "widgets"

	// widgetWrapperSuffix is the convention core/block/editor/widget uses when
	// a wrapper's id has to be derived from its link's rather than random
	// (widget.createBlock), so that two devices creating the same widget do not
	// end up with two wrappers.
	widgetWrapperSuffix = "-wrapper"
)

// writeWidgets renders index.json's widgets as the archive's Widget snapshot.
// A bundle declaring no widgets gets no snapshot rather than an empty one.
func writeWidgets(outDir string, idx *anyblockjson.Index, format outputFormat) error {
	snap, err := buildWidgets(idx)
	if err != nil {
		return err
	}
	if snap == nil {
		return nil
	}
	return writeSnapshot(outDir, widgetsObjectId, model.SmartBlockType_Widget, snap, format)
}

// buildWidgets assembles the Widget snapshot, or nil when there is nothing to
// put in the sidebar.
//
// Four things about the block graph are load-bearing, none of them obvious:
//
//   - The root block must carry smartblock content. objectcreator.setRootBlock
//     hands the blocks to anymark.AddRootBlock, which renames the first block
//     with that content to the derived widgets id. Without one, AddRootBlock
//     *appends* a second root instead, the state's root becomes that new block,
//     and every wrapper is orphaned.
//   - Every wrapper must be reachable from the root. updateWidgetObject walks
//     state.Blocks(), which is a breadth-first traversal from the root — a block
//     the root does not reach is simply not there.
//   - That traversal is also why root.ChildrenIds order is sidebar order:
//     addWidgetBlock appends each widget to the existing widget object in
//     traversal order (InsertTo with Block_Inner appends, despite the
//     prependChildrenIds it calls). So the order here is index.json's order.
//   - A wrapper gets exactly one child. addWidgetBlock reads ChildrenIds[0] and
//     ignores the rest.
func buildWidgets(idx *anyblockjson.Index) (*model.SmartBlockSnapshotBase, error) {
	if len(idx.Widgets) == 0 {
		return nil, nil
	}

	root := &model.Block{
		Id:      widgetsObjectId,
		Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
	}
	blocks := make([]*model.Block, 0, 1+2*len(idx.Widgets))
	blocks = append(blocks, root)

	for i, w := range idx.Widgets {
		layout, ok := widgetLayouts[w.Layout]
		if w.Layout != "" && !ok {
			return nil, fmt.Errorf("widgets[%d]: unknown layout %q", i, w.Layout)
		}
		linkId := widgetBlockId(i, w.Target)
		wrapperId := linkId + widgetWrapperSuffix

		root.ChildrenIds = append(root.ChildrenIds, wrapperId)
		blocks = append(blocks, &model.Block{
			Id:          wrapperId,
			ChildrenIds: []string{linkId},
			Content: &model.BlockContentOfWidget{Widget: &model.BlockContentWidget{
				Layout: layout,
				Limit:  w.Limit,
				// viewId names a view inside the target and only a client can
				// know which; createWidgets leaves it empty on the inject path
				// too, and the app falls back to the target's default view.
				ViewId:    "",
				AutoAdded: false,
			}},
		}, &model.Block{
			Id: linkId,
			// the target is the bundle's own object id, relinked on import like
			// every other reference (common.UpdateLinksToObjects); a reserved
			// listing is translated out of the format's `_` namespace into the
			// bare word the importer knows (WireWidgetTarget) and then passes
			// through untouched, because handleLinkBlock returns early for
			// widget.IsPredefinedWidgetTargetId. Anything else that does not
			// resolve is rewritten to _missing_object and then stripped — link
			// and wrapper both — by WidgetObject.Init, which is why
			// anyblockbatch.CheckIndexTargets rejects such a bundle up front.
			Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
				TargetBlockId: anyblockjson.WireWidgetTarget(w.Target),
				// all four are their enums' zero values, so they cost no wire
				// bytes; they are spelled out because this is the shape an app
				// export writes and the shape a reader will compare against.
				Style:       model.BlockContentLink_Page,
				CardStyle:   model.BlockContentLink_Text,
				Description: model.BlockContentLink_None,
				IconSize:    model.BlockContentLink_SizeNone,
			}},
		})
	}

	return &model.SmartBlockSnapshotBase{
		Blocks: blocks,
		Details: &types.Struct{Fields: map[string]*types.Value{
			detailID:     strVal(widgetsObjectId),
			detailLayout: numVal(float64(model.ObjectType_dashboard)),
			// the widget object is never listed anywhere as an object
			detailIsHidden: boolVal(true),
		}},
		ObjectTypes: []string{bundle.TypeKeyDashboard.URL()},
	}, nil
}

// widgetBlockId mints a link block's id: 24 hex characters, the shape
// bson.NewObjectId().Hex() gives every block id in an app export.
//
// Derived from the widget's position rather than drawn at random, because this
// tool is deterministic by design — re-converting an unchanged bundle produces
// identical bytes (see batch.optionLocalKey, which does the same for options).
// Seeding on the position rather than the target alone is what makes the ids
// unique even when a bundle lists the same target twice.
func widgetBlockId(i int, target string) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("widget\x00%d\x00%s", i, target)))
	return hex.EncodeToString(sum[:])[:24]
}
