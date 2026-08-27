package anyblockjson

// index.json is the only document that describes the bundle rather than
// one object: the space's name, what opens on entry, what the sidebar shows.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndex_Roundtrip(t *testing.T) {
	doc := `{
		"$schema": "https://schemas.anytype.io/anyblock/1.0/index.schema.json",
		"version": 1,
		"name": "Company Wiki",
		"description": "Everything we know, with an owner.",
		"iconEmoji": "📚",
		"homepage": "page-wiki-home",
		"widgets": [
			{ "target": "page-wiki-home" },
			{ "target": "type-wiki-page", "layout": "view", "limit": 6 },
			{ "target": "favorite", "layout": "compactList" }
		]
	}`
	idx, err := UnmarshalIndex([]byte(doc))
	require.NoError(t, err)

	assert.Equal(t, "Company Wiki", idx.Name)
	assert.Equal(t, "page-wiki-home", idx.Homepage)
	require.Len(t, idx.Widgets, 3)
	assert.Equal(t, "", idx.Widgets[0].Layout, "omitted layout stays empty; link is the default")
	assert.Equal(t, "view", idx.Widgets[1].Layout)
	assert.Equal(t, int32(6), idx.Widgets[1].Limit)

	// the install opens the first widget's target
	assert.Equal(t, "page-wiki-home", idx.EntryPoint())

	out, err := MarshalIndex(idx)
	require.NoError(t, err)
	again, err := UnmarshalIndex(out)
	require.NoError(t, err)
	out2, err := MarshalIndex(again)
	require.NoError(t, err)
	assert.Equal(t, string(out), string(out2), "export must be byte-stable (§11)")
}

func TestIndex_EntryPoint(t *testing.T) {
	t.Run("the declared entrypoint wins over widget order", func(t *testing.T) {
		idx, err := UnmarshalIndex([]byte(`{"version": 1,
			"entrypoint": "page-home",
			"widgets": [{"target": "type-task", "layout": "view"}]}`))
		require.NoError(t, err)
		assert.Equal(t, "page-home", idx.EntryPoint(),
			"reordering the sidebar must not change what opens")
	})

	t.Run("nothing declared means no entry point", func(t *testing.T) {
		idx, err := UnmarshalIndex([]byte(`{"version": 1, "name": "X"}`))
		require.NoError(t, err)
		assert.Empty(t, idx.EntryPoint())
	})

	// bundles written before entrypoint existed carried it as widgets[0]
	t.Run("falls back to the first widget naming an object", func(t *testing.T) {
		idx, err := UnmarshalIndex([]byte(`{"version": 1,
			"widgets": [{"target": "recent"}, {"target": "page-home"}]}`))
		require.NoError(t, err)
		assert.Equal(t, "page-home", idx.EntryPoint(), "reserved listings are skipped")
	})

	t.Run("a reserved listing cannot be an entrypoint", func(t *testing.T) {
		for _, bad := range []string{"widgets", "graph", "favorite", "recent"} {
			_, err := UnmarshalIndex([]byte(`{"version": 1, "entrypoint": "` + bad + `"}`))
			require.Error(t, err, bad)
		}
	})
}

// homepage is what opens on every later entry; omitting it means "the same
// page you landed on", never the widgets screen
func TestIndex_SpaceHomepage(t *testing.T) {
	t.Run("defaults to the entrypoint", func(t *testing.T) {
		idx, err := UnmarshalIndex([]byte(`{"version": 1, "entrypoint": "page-home"}`))
		require.NoError(t, err)
		assert.Equal(t, "page-home", idx.SpaceHomepage())
	})
	t.Run("an explicit value wins", func(t *testing.T) {
		idx, err := UnmarshalIndex([]byte(`{"version": 1,
			"entrypoint": "page-welcome", "homepage": "page-dashboard"}`))
		require.NoError(t, err)
		assert.Equal(t, "page-dashboard", idx.SpaceHomepage())
	})
	t.Run("a reserved homepage is still allowed, deliberately", func(t *testing.T) {
		idx, err := UnmarshalIndex([]byte(`{"version": 1,
			"entrypoint": "page-home", "homepage": "graph"}`))
		require.NoError(t, err)
		assert.Equal(t, "graph", idx.SpaceHomepage())
	})
}

func TestIndex_Validation(t *testing.T) {
	for _, tc := range []struct{ name, doc, want string }{
		{"version required", `{"name": "X"}`, "version"},
		{"unknown layout", `{"version": 1, "widgets": [{"target": "a", "layout": "grid"}]}`, "layout"},
		{"widget needs a target", `{"version": 1, "widgets": [{"layout": "link"}]}`, "target"},
		{"unknown property", `{"version": 1, "startingPage": "a"}`, "startingPage"},
		{"limit must be an integer", `{"version": 1, "widgets": [{"target": "a", "limit": 1.5}]}`, "limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := UnmarshalIndex([]byte(tc.doc))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}

	t.Run("reserved homepage names are accepted", func(t *testing.T) {
		for _, h := range []string{"widgets", "graph"} {
			_, err := UnmarshalIndex([]byte(`{"version": 1, "homepage": "` + h + `"}`))
			assert.NoError(t, err, h)
			assert.True(t, IsReservedHomepage(h))
		}
	})

	t.Run("an object id is not reserved", func(t *testing.T) {
		assert.False(t, IsReservedHomepage("page-wiki-home"))
		assert.False(t, IsReservedWidgetTarget("page-wiki-home"))
		assert.True(t, IsReservedWidgetTarget("favorite"))
	})

	// version 1 is the only version, like every other document
	t.Run("a newer version is rejected", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`{"version": 2}`))
		require.Error(t, err)
	})
}

// iconImage names an image object rather than referencing its id, because the
// installer resolves the space icon by name (getNewAvatarId).
func TestIndex_IconImage(t *testing.T) {
	idx, err := UnmarshalIndex([]byte(`{"version": 1, "name": "Wiki",
		"iconImage": "acme-logo"}`))
	require.NoError(t, err)
	assert.Equal(t, "acme-logo", idx.IconImage)

	out, err := MarshalIndex(idx)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"iconImage": "acme-logo"`)

	// both icon forms may be present; the installer prefers the image
	_, err = UnmarshalIndex([]byte(`{"version": 1, "iconEmoji": "📚", "iconImage": "logo"}`))
	assert.NoError(t, err)
}
