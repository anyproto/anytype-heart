package main

// The profile file is the only output describing the bundle rather than an
// object, and the only way an installed space gets a name, an entry point and
// a sidebar. builtinobjects reads it with pb.Profile.Unmarshal, so it must
// decode as one.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/constant"
)

func readBack(t *testing.T, dir string) *pb.Profile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, constant.ProfileFile))
	require.NoError(t, err)
	p := &pb.Profile{}
	require.NoError(t, p.Unmarshal(data), "the installer reads this with pb.Profile.Unmarshal")
	return p
}

func TestWriteProfile(t *testing.T) {
	dir := t.TempDir()
	idx := &anyblockjson.Index{
		Name:       "Company Wiki",
		Entrypoint: "page-home",
		Widgets: []anyblockjson.Widget{
			{Target: "page-home", Layout: "tree"},
			{Target: "type-page", Layout: "view", Limit: 6},
			{Target: "favorite", Layout: "compact_list"},
			{Target: "chat-requests"}, // no layout: link, the zero value
		},
	}
	require.NoError(t, writeProfile(dir, idx, nil))

	p := readBack(t, dir)
	assert.Equal(t, "Company Wiki", p.Name)
	// an omitted homepage follows the entrypoint, never the widgets screen
	assert.Equal(t, "page-home", p.SpaceDashboardId)

	require.Len(t, p.Widgets, 4)
	assert.Equal(t, model.BlockContentWidget_Tree, p.Widgets[0].Layout)
	assert.Equal(t, model.BlockContentWidget_View, p.Widgets[1].Layout)
	assert.Equal(t, int32(6), p.Widgets[1].ObjectLimit)
	assert.Equal(t, model.BlockContentWidget_CompactList, p.Widgets[2].Layout)
	assert.Equal(t, model.BlockContentWidget_Link, p.Widgets[3].Layout, "absent layout is link")

	// reserved targets pass through untouched; the installer knows them
	assert.Equal(t, "favorite", p.Widgets[2].TargetObjectId)
	// TEMPORARY: inject opens widgets[0], which is why validate warns when
	// the declared entrypoint is not first
	assert.Equal(t, idx.EntryPoint(), p.Widgets[0].TargetObjectId)
}

func TestWriteProfile_Homepage(t *testing.T) {
	t.Run("explicit homepage wins over the entrypoint", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, writeProfile(dir, &anyblockjson.Index{
			Entrypoint: "page-welcome", Homepage: "page-dashboard",
		}, nil))
		assert.Equal(t, "page-dashboard", readBack(t, dir).SpaceDashboardId)
	})
	t.Run("a reserved homepage passes through", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, writeProfile(dir, &anyblockjson.Index{
			Entrypoint: "page-home", Homepage: "graph",
		}, nil))
		assert.Equal(t, "graph", readBack(t, dir).SpaceDashboardId)
	})
}

// iconImage is an object id in the format and the image's *name* on the wire
func TestWriteProfile_IconImage(t *testing.T) {
	dir := t.TempDir()
	names := map[string]string{"file-logo": "acme-logo"}
	require.NoError(t, writeProfile(dir, &anyblockjson.Index{
		Name: "X", IconImage: "file-logo",
	}, names))
	assert.Equal(t, "acme-logo", readBack(t, dir).Avatar)

	t.Run("an unknown id fails rather than shipping a blank icon", func(t *testing.T) {
		err := writeProfile(t.TempDir(), &anyblockjson.Index{IconImage: "file-missing"}, names)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "names no object")
	})
	t.Run("a nameless object fails: the installer resolves by name", func(t *testing.T) {
		err := writeProfile(t.TempDir(), &anyblockjson.Index{IconImage: "file-x"},
			map[string]string{"file-x": ""})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "has no name")
	})
}

func TestWriteProfile_UnknownLayout(t *testing.T) {
	err := writeProfile(t.TempDir(), &anyblockjson.Index{
		Widgets: []anyblockjson.Widget{{Target: "a", Layout: "grid"}},
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown layout")
}
