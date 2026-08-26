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
// The snapshot itself is anyblockjson.WidgetsSnapshot — one root block plus,
// per widget, a wrapper carrying the widget content and a link child carrying
// the target, the shape widget.createBlock builds in a live space and
// objectcreator.addWidgetBlock reads back on import. It lives in the package
// rather than here because the round-trip verifier holds the SAME function's
// output against the widget object it omits: one builder, so the tool that
// installs a sidebar and the check that promises nothing was lost cannot
// drift apart.

import (
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// writeWidgets renders index.json's sidebar state as the archive's Widget
// snapshot. A bundle declaring none gets no snapshot rather than an empty
// one.
func writeWidgets(outDir string, idx *anyblockjson.Index, format outputFormat) error {
	snap, err := anyblockjson.WidgetsSnapshot(idx)
	if err != nil {
		return err
	}
	if snap == nil {
		return nil
	}
	return writeSnapshot(outDir, anyblockjson.WidgetsObjectId, model.SmartBlockType_Widget, snap, format)
}
