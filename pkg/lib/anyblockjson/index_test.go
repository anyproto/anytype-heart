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
		"icon": { "format": "emoji", "emoji": "📚" },
		"homepage": "page-wiki-home",
		"widgets": [
			{ "target": "page-wiki-home" },
			{ "target": "type-wiki-page", "layout": "view", "limit": 6 },
			{ "target": "_favorite", "layout": "compact_list" }
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
			"widgets": [{"target": "_recent"}, {"target": "page-home"}]}`))
		require.NoError(t, err)
		assert.Equal(t, "page-home", idx.EntryPoint(), "reserved listings are skipped")
	})

	// The listing this skips is spelled `_recent`, and an object id may not
	// begin with `_`, so "skip the reserved ones" is now a statement about
	// disjoint namespaces rather than a race between two flat word lists. It
	// used to be the latter: a bundle shipping an object with id `recent` made
	// EntryPoint skip its own object, so the entry point the tooling reported
	// was not the one the installer opened.
	t.Run("the skipped targets cannot be bundle ids", func(t *testing.T) {
		for _, target := range ReservedWidgetTargets() {
			assert.True(t, IsPlatformId(target),
				"a reserved listing must live in the platform namespace, or an object can shadow it: %q", target)
		}
		for _, home := range []string{HomepageWidgets, HomepageGraph} {
			assert.True(t, IsPlatformId(home), home)
		}
	})

	t.Run("a reserved listing cannot be an entrypoint", func(t *testing.T) {
		for _, bad := range []string{"_widgets", "_graph", "_favorite", "_recent", "_all_objects"} {
			_, err := UnmarshalIndex([]byte(`{"version": 1, "entrypoint": "` + bad + `"}`))
			require.Error(t, err, bad)
		}
	})

	// the reason the entrypoint ban is a prefix and not a word list
	t.Run("no entrypoint may enter the platform namespace", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`{"version": 1, "entrypoint": "_otpage"}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/entrypoint")
		assert.Contains(t, err.Error(), "platform")
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
			"entrypoint": "page-home", "homepage": "_graph"}`))
		require.NoError(t, err)
		assert.Equal(t, "_graph", idx.SpaceHomepage())
		assert.True(t, IsReservedHomepage(idx.SpaceHomepage()))
	})

	// The bare word is what core/domain/homepage.go and builtinobjects use, and
	// setWorkspaceSettings matches it BEFORE trying to resolve an id — so while
	// the format spelled it `graph` too, an object with that id could never be
	// a homepage. Here it is an ordinary id and nothing reserved is involved.
	t.Run("the wire spelling of a reserved screen is an ordinary id", func(t *testing.T) {
		idx, err := UnmarshalIndex([]byte(`{"version": 1, "homepage": "graph"}`))
		require.NoError(t, err)
		assert.False(t, IsReservedHomepage("graph"))
		assert.Equal(t, "graph", idx.SpaceHomepage())
	})

	// what pb.Profile.SpaceDashboardId has to carry
	t.Run("a reserved screen is translated for the wire", func(t *testing.T) {
		assert.Equal(t, "graph", WireHomepage(HomepageGraph))
		assert.Equal(t, "widgets", WireHomepage(HomepageWidgets))
		assert.Equal(t, "page-home", WireHomepage("page-home"), "an object id passes through")
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
		for _, h := range []string{"_widgets", "_graph"} {
			_, err := UnmarshalIndex([]byte(`{"version": 1, "homepage": "` + h + `"}`))
			assert.NoError(t, err, h)
			assert.True(t, IsReservedHomepage(h))
		}
	})

	t.Run("an object id is not reserved", func(t *testing.T) {
		assert.False(t, IsReservedHomepage("page-wiki-home"))
		assert.False(t, IsReservedWidgetTarget("page-wiki-home"))
		assert.True(t, IsReservedWidgetTarget("_favorite"))
	})

	// The listings the importer knows are bare words; the format spells them
	// with the platform prefix so nothing a bundle ships can collide. The
	// unprefixed spelling is therefore an ordinary bundle id and reserves
	// nothing — which is the whole content of the rename.
	t.Run("the bare listing name reserves nothing", func(t *testing.T) {
		for _, bare := range []string{"favorite", "recent", "set", "collection"} {
			assert.False(t, IsReservedWidgetTarget(bare), bare)
			_, err := UnmarshalIndex([]byte(`{"version": 1, "entrypoint": "` + bare + `"}`))
			assert.NoError(t, err, "an ordinary id: %s", bare)
		}
	})

	// A `_` target that is not one of the six resolves to nothing, and a
	// widget target that resolves to nothing is dropped on install with no
	// error at all — so it has to be refused here, by name, with the
	// inventory in the message.
	// The schema states the rule too, so asserting only that this is refused
	// would stay green with the gate deleted. What the gate is FOR is the
	// message: an anonymous `does not match pattern '^[^_]'` names neither the
	// namespace nor the repair, and the failure it precedes is invisible.
	t.Run("an unknown reserved listing is refused, and named", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`{"version": 1, "widgets": [
			{"target": "page-home"}, {"target": "_favourite"}]}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/widgets/1/target")
		assert.Contains(t, err.Error(), "_favourite")
		assert.Contains(t, err.Error(), "is not a reserved listing")
		assert.Contains(t, err.Error(), "_favorite", "the message must carry the inventory")
		assert.Contains(t, err.Error(), "dropped on install without an error",
			"and must say what happens if it is not caught here")
	})

	t.Run("an unknown reserved homepage is refused", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`{"version": 1, "homepage": "_last_opened"}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/homepage")
		assert.Contains(t, err.Error(), "the only reserved homepages are")
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
	for _, target := range []string{"_favorite", "_recent", "_set", "_collection"} {
		assert.True(t, IsReservedWidgetTarget(target), target)
		assert.True(t, IsImportableWidgetTarget(target), target)
	}
	for _, target := range []string{"_all_objects", "_recent_open"} {
		assert.True(t, IsReservedWidgetTarget(target), "still names a built-in, not an object: "+target)
		assert.False(t, IsImportableWidgetTarget(target), "the importer does not know it: "+target)
	}
	assert.False(t, IsImportableWidgetTarget("page-wiki-home"))
}

// The four importable listings are the ones the pb importer knows by their
// bare, unprefixed names (widget.IsPredefinedWidgetTargetId), so the rename
// only holds together if every one of them has a wire spelling and the wiring
// applies it. Writing `_set` into a link block instead of `set` is strictly
// worse than the shadowing bug this replaces: handleLinkBlock rewrites the
// unrecognised target to addr.MissingObject and WidgetObject.Init then strips
// the link and its wrapper, so the widget vanishes with no error.
func TestIndex_WireWidgetTargets(t *testing.T) {
	want := map[string]string{
		"_favorite": "favorite", "_recent": "recent", "_set": "set", "_collection": "collection",
	}
	for target, wire := range want {
		assert.Equal(t, wire, WireWidgetTarget(target), target)
		assert.NotEqual(t, target, WireWidgetTarget(target),
			"the platform prefix must be translated away, or the importer drops the widget")
	}
	for _, target := range ReservedWidgetTargets() {
		if IsImportableWidgetTarget(target) {
			assert.Contains(t, want, target, "every importable listing needs a wire spelling")
			continue
		}
		assert.Equal(t, target, WireWidgetTarget(target),
			"a listing the importer cannot take has no wire spelling to invent")
	}
	assert.Equal(t, "page-wiki-home", WireWidgetTarget("page-wiki-home"), "an object id passes through")
}

// The index schema names the object schema by its published URL to $ref the
// shared icon definition (§2b). That URL is derived from FormatVersion
// everywhere else, so a version bump would leave this one spelling behind —
// the compiler catches it (every index test fails at once), but only this
// says which line to fix.
func TestIndexSchema_RefsThePublishedObjectSchema(t *testing.T) {
	assert.Contains(t, string(indexSchemaJSON), SchemaURL+"#/$defs/plainIcon")
}

// A bundle index carries the SAME typed icon an object does (§2b, §2c),
// restricted to the two kinds a bundle can hold. The image variant names an
// object id; the wiring resolves it to the image's name, because the
// installer looks the space icon up by name (getNewAvatarId).
//
// How this can fail: give the index its own icon shape, or let both an emoji
// and an image be present at once, and one of these breaks. The old two-key
// index accepted both with no rule for which wins — the last assertion here
// used to say "the installer prefers the image", which was a rule written
// nowhere in the schema.
func TestIndex_Icon(t *testing.T) {
	idx, err := UnmarshalIndex([]byte(`{"version": 1, "name": "Wiki",
		"icon": {"format": "file", "file": "acme-logo"}}`))
	require.NoError(t, err)
	assert.Equal(t, "acme-logo", idx.IconImageId())

	out, err := MarshalIndex(idx)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"icon": {`)
	assert.Contains(t, string(out), `"file": "acme-logo"`)

	t.Run("an emoji index icon has no image id", func(t *testing.T) {
		idx, err := UnmarshalIndex([]byte(`{"version": 1, "icon": {"format": "emoji", "emoji": "📚"}}`))
		require.NoError(t, err)
		assert.Equal(t, "📚", idx.Icon.Emoji)
		assert.Empty(t, idx.IconImageId())
	})

	t.Run("an emoji and an image are no longer both writable", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`{"version": 1,
			"icon": {"format": "emoji", "emoji": "📚", "file": "logo"}}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `property "file" is not allowed`)
	})

	t.Run("the discriminator names the alternatives when it is missing", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`{"version": 1, "icon": {"emoji": "📚"}}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/icon: missing property 'format'")
		assert.Contains(t, err.Error(), "'emoji', 'file'")
	})

	t.Run("the four-variant object icon is not a space icon", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(`{"version": 1, "icon": {"format": "icon", "name": "folder"}}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "value must be one of 'emoji', 'file'")
	})
}
