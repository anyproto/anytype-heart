package anyblockjson

// index.json (§2c) is the only document that describes the bundle rather than
// one object: the space's name, what opens on entry, what the sidebar shows.

import (
	"errors"
	"strconv"
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

	// an index shares the format version and its rules with object documents
	// (§10): a newer one is rejected with both versions named, not with a
	// generic schema constraint failure
	t.Run("a newer version is rejected, naming both versions", func(t *testing.T) {
		// given a bundle index from a future format version, carrying a key
		// this reader has never heard of
		data := []byte(`{"version": 2, "name": "Wiki", "futureKey": true}`)

		// when
		_, err := UnmarshalIndex(data)

		// then
		require.Error(t, err)
		var ve *ValidationError
		require.True(t, errors.As(err, &ve))
		assert.True(t, ve.NewerFormat, "must be flagged as a newer format, not a constraint failure")
		assert.Contains(t, err.Error(), "newer version")
		assert.Contains(t, err.Error(), "2")
		assert.Contains(t, err.Error(), strconv.Itoa(FormatVersion))
		// the version gate ran before the schema, so the unknown key never
		// produced an issue of its own
		assert.NotContains(t, err.Error(), "futureKey")
	})

	t.Run("a missing version is rejected", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`{"name": "Wiki"}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version is required")
	})

	t.Run("a non-object index is rejected cleanly", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`[1, 2, 3]`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index must be a JSON object")
	})
}

// Not every reserved listing survives import. handleLinkBlock leaves a target
// alone only when widget.IsPredefinedWidgetTargetId knows it; anything else it
// cannot resolve becomes addr.MissingObject, and WidgetObject.Init then strips
// the link and its wrapper. So allObjects and recentOpen — real targets in a
// live space — would cost the author a widget with no error to explain it,
// which is why the two questions are asked separately.
func TestIndex_ImportableWidgetTargets(t *testing.T) {
	for _, target := range []string{"favorite", "recent", "set", "collection"} {
		assert.True(t, IsReservedWidgetTarget(target), target)
		assert.True(t, IsImportableWidgetTarget(target), target)
	}
	for _, target := range []string{"allObjects", "recentOpen"} {
		assert.True(t, IsReservedWidgetTarget(target), "still names a built-in, not an object: "+target)
		assert.False(t, IsImportableWidgetTarget(target), "the importer does not know it: "+target)
	}
	assert.False(t, IsImportableWidgetTarget("page-wiki-home"))
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
