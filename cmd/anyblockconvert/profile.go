package main

// profile.go writes the archive's `profile` file from index.json (§2c). It is
// the one output that describes the bundle rather than an object:
// util/builtinobjects reads it with pb.Profile.Unmarshal, so it is raw
// protobuf regardless of the snapshot format.
//
// How much of it is honoured depends on which path installs the archive, and
// a bundle only ever takes one of them:
//
//   - inject() — the built-in use-case archives — reads all of it: name,
//     avatar, spaceDashboardId, and widgets (getWidgets + createWidgets).
//   - CreateObjectsForExperience — what ObjectImportExperience calls, and so
//     what every bundle this tool produces goes through — reads name, avatar
//     and spaceDashboardId, on a NEW-space install (setWorkspaceSettings
//     with isBundle=true, gated on isNewSpace, so a created space takes the
//     bundle's identity and an existing space keeps its own). It never reads
//     profile.widgets: getWidgets belongs to inject, and the one
//     createWidgets call on this path is the Markdown/AI branch's, built
//     from the manifest's dashboard page rather than from this file.
//
// The sidebar arrives as a Widget snapshot in the archive (widgets.go),
// which is how a real app export carries it.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/constant"
)

// widgetLayouts maps §2c layout names to the wire enum. Absent means link,
// the enum's own zero value.
var widgetLayouts = map[string]model.BlockContentWidgetLayout{
	"link":         model.BlockContentWidget_Link,
	"tree":         model.BlockContentWidget_Tree,
	"list":         model.BlockContentWidget_List,
	"compact_list": model.BlockContentWidget_CompactList,
	"view":         model.BlockContentWidget_View,
}

// writeProfile renders index.json as the archive's profile file.
//
// Ids stay the bundle's own: the installer maps them through oldAnytypeID
// (builtinobjects.getNewObjectId), the same relinking every other reference
// gets, so nothing here needs the post-import ids.
//
// TEMPORARY: the entry point is carried as widgets[0], because pb.Profile has
// no field of its own for it — inject reads widgets[0].targetObjectId. A
// declared entrypoint that is not the first widget is therefore not honoured,
// and anyblockvalidate warns about it rather than this reordering the sidebar
// behind the author's back.
//
// Name, Avatar and Widgets are written for symmetry with the built-in
// archives, and are inert on the path a bundle takes — see the file comment.
// The sidebar the user gets comes from widgets.go.
func writeProfile(outDir string, idx *anyblockjson.Index, names map[string]string) error {
	profile := &pb.Profile{
		Name: idx.Name,
	}

	// spaceDashboardId is the space's homepage: an object id, or a reserved
	// screen translated out of the format's `_` namespace into the bare name
	// setWorkspaceSettings switches on (WireHomepage). An omitted homepage
	// follows the entrypoint rather than defaulting to the widgets screen (§2c).
	profile.SpaceDashboardId = anyblockjson.WireHomepage(idx.SpaceHomepage())

	// the icon is referenced by id in the format and by name on the wire
	if id := idx.IconImageId(); id != "" {
		name, ok := names[id]
		if !ok {
			return fmt.Errorf("icon %q names no object in the bundle", id)
		}
		if name == "" {
			return fmt.Errorf("icon %q has no name, and the installer resolves the space icon by name", id)
		}
		profile.Avatar = name
	}

	// inert on the experience path: CreateObjectsForExperience never calls
	// getWidgets, so nothing reads these. Kept so an archive this tool produces
	// is also a valid built-in archive, where inject() does read them.
	for i, w := range idx.Widgets {
		layout, ok := widgetLayouts[w.Layout]
		if w.Layout != "" && !ok {
			return fmt.Errorf("widgets[%d]: unknown layout %q", i, w.Layout)
		}
		profile.Widgets = append(profile.Widgets, &pb.WidgetBlock{
			Layout:         layout,
			TargetObjectId: anyblockjson.WireWidgetTarget(w.Target),
			ObjectLimit:    w.Limit,
		})
	}

	data, err := profile.Marshal()
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}
	return os.WriteFile(filepath.Join(outDir, constant.ProfileFile), data, 0o644)
}
