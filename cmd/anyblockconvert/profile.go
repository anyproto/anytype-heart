package main

// profile.go writes the archive's `profile` file from index.json (§2c). It is
// the one output that describes the bundle rather than an object, and the only
// way an installed space gets a name, an entry point and a sidebar:
// util/builtinobjects reads it with pb.Profile.Unmarshal, so it is raw
// protobuf regardless of the snapshot format.

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
	"link":        model.BlockContentWidget_Link,
	"tree":        model.BlockContentWidget_Tree,
	"list":        model.BlockContentWidget_List,
	"compactList": model.BlockContentWidget_CompactList,
	"view":        model.BlockContentWidget_View,
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
func writeProfile(outDir string, idx *anyblockjson.Index, names map[string]string) error {
	profile := &pb.Profile{
		Name: idx.Name,
	}

	// spaceDashboardId is the space's homepage: an object id, or a reserved
	// screen name passed through untouched. An omitted homepage follows the
	// entrypoint rather than defaulting to the widgets screen (§2c).
	profile.SpaceDashboardId = idx.SpaceHomepage()

	// the icon is referenced by id in the format and by name on the wire
	if idx.IconImage != "" {
		name, ok := names[idx.IconImage]
		if !ok {
			return fmt.Errorf("iconImage %q names no object in the bundle", idx.IconImage)
		}
		if name == "" {
			return fmt.Errorf("iconImage %q has no name, and the installer resolves the space icon by name", idx.IconImage)
		}
		profile.Avatar = name
	}

	for i, w := range idx.Widgets {
		layout, ok := widgetLayouts[w.Layout]
		if w.Layout != "" && !ok {
			return fmt.Errorf("widgets[%d]: unknown layout %q", i, w.Layout)
		}
		profile.Widgets = append(profile.Widgets, &pb.WidgetBlock{
			Layout:         layout,
			TargetObjectId: w.Target,
			ObjectLimit:    w.Limit,
		})
	}

	data, err := profile.Marshal()
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}
	return os.WriteFile(filepath.Join(outDir, constant.ProfileFile), data, 0o644)
}
