package anyblockjson

// alias_test.go — the v0.38 wire-spelling aliases (alias.go): the sixteen
// bundled keys whose STORED key says "relation" write and read "property"
// spellings, the stored keys stay their own verbatim addresses, and the
// vacated slugs bind nothing.

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The headline incoherence the alias table exists to end: one document held
// the block type `featured_properties` beside the property key
// `featured_relations` (12,609 corpus documents). The stored key
// featuredRelations now spells `featured_properties` — and because the
// alias is a bundled-table fact that ships with every reader, it needs no
// legend entry.
//
// How this can fail: drop featuredRelations from propertyKeyAliases and the
// old slug comes back; make recordPropertyKey ask the un-aliased table and
// every aliased key starts paying a legend line for a spelling every reader
// already knows.
func TestAlias_BundledPropertyKeysSpellProperty(t *testing.T) {
	// given
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{Id: "o1",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		Details: fields(map[string]*types.Value{
			"id": str("o1"),
			// relationOptionColor rather than featuredRelations: the latter
			// was this test's example until it was DEPRECATED outright (the
			// type owns an object's featured list), so it no longer travels
			// and could not demonstrate an alias.
			"relationOptionColor": str("ice"),
		}),
	}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, testOptions())
	require.NoError(t, err)

	// then
	assert.Contains(t, string(data), `"property_option_color"`)
	assert.NotContains(t, string(data), "relation_option_color",
		"the vacated slug must not survive anywhere in the document")
	assert.NotContains(t, string(data), `"property_internal_keys"`,
		"an alias is a bundled-table fact and owes no legend entry")
	require.NoError(t, Validate(data), "I1: Marshal never emits what its own Validate rejects")

	_, back, err := Unmarshal(data, testOptions())
	require.NoError(t, err)
	assert.Equal(t, str("ice"), back.Details.Fields["relationOptionColor"],
		"the alias inverts onto the stored key")
}

// The stored key itself still resolves VERBATIM — §3 chain step 2 is
// untouched by the alias layer — while the OLD derived slug binds nothing
// any more: pre-freeze, no back-compat, so `relation_option_color` is now an
// ordinary custom key that names itself.
//
// The example was `featuredRelations` until that key was DEPRECATED outright
// (an object's featured list belongs to its type), and a deprecated key is
// dropped on the way IN as well as out — so it could no longer show that a
// stored key addresses itself.
//
// How this can fail: let bundledPropertyKeyBySpelling keep answering for a
// slug whose key is aliased, and one stored key has two spellings again —
// the §15 #14 disease the alias exists to end.
func TestAlias_StoredKeyVerbatimAndTheVacatedSlugBindsNothing(t *testing.T) {
	t.Run("the stored key is its own address", func(t *testing.T) {
		doc := `{"version":1,"id":"o1","properties":{"relationOptionColor":"ice"}}`
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, str("ice"), snap.Details.Fields["relationOptionColor"])
	})

	t.Run("the vacated slug names only itself", func(t *testing.T) {
		doc := `{"version":1,"id":"o1","properties":{"relation_option_color":"ice"}}`
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, str("ice"), snap.Details.Fields["relation_option_color"],
			"a spelling no table binds passes through verbatim (§3 chain step 4)")
		assert.Nil(t, snap.Details.Fields["relationOptionColor"],
			"it must NOT land on the aliased bundled key")
	})

	t.Run("a deprecated key is dropped in both directions", func(t *testing.T) {
		doc := `{"version":1,"id":"o1","properties":{"featured_properties":["name"]}}`
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Nil(t, snap.Details.Fields["featuredRelations"],
			"the type owns an object's featured list; the object's copy does not come back")
	})
}

// The type namespace carries the same two rules: the bundled type key
// `relation` spells `property` on the wire with no legend entry, and the
// stored key still names itself verbatim on the way in.
func TestAlias_BundledTypeKeysSpellProperty(t *testing.T) {
	t.Run("export spells the alias, no legend owed", func(t *testing.T) {
		data, err := Marshal(model.SmartBlockType_Page, typedSnapshot("ot-relation"), Options{})
		require.NoError(t, err)

		doc := decodeEnvelope(t, data)
		assert.Equal(t, "property", doc.Type)
		assert.Empty(t, doc.TypeKeys, "an alias is a bundled-table fact and owes no legend entry")

		_, snap, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, []string{"ot-relation"}, snap.ObjectTypes,
			"a package-only reader inverts the alias through the shipped table")
	})

	t.Run("the stored type key still names itself verbatim", func(t *testing.T) {
		_, snap, err := Unmarshal([]byte(`{"version":1,"id":"o1","type":"relation"}`),
			Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, []string{"ot-relation"}, snap.ObjectTypes,
			"an exact stored key wins before any table (§3 chain step 2)")
	})

	t.Run("property_option inverts too", func(t *testing.T) {
		_, snap, err := Unmarshal([]byte(`{"version":1,"id":"o1","type":"property_option"}`),
			Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, []string{"ot-relationOption"}, snap.ObjectTypes)
	})
}

// The forgiving fold layer (§3 chain step 4) moves WITH the alias: a
// near-miss of the new spelling recovers, a near-miss of the old one does
// not. It cannot serve both — FoldApiKey drops case and underscores, so the
// stored key's own fold IS the old slug's, and keeping it would keep the
// old spelling alive one typo away.
//
// How this can fail: build BundledPropertyKeysByFold from the raw bundle
// fold without filtering aliased keys, and `Featured_Relations` resolves
// again.
func TestAlias_TheFoldClassMovesWithTheSpelling(t *testing.T) {
	assert.Equal(t, []string{"featuredRelations"}, BundledPropertyKeysByFold("Featured_Properties"),
		"a near-miss of the alias folds onto the key")
	assert.Empty(t, BundledPropertyKeysByFold("Featured_Relations"),
		"a near-miss of the vacated slug folds onto nothing")

	assert.Equal(t, []string{"relationOption"}, BundledTypeKeysByFold("Property_Option"))
	assert.Empty(t, BundledTypeKeysByFold("Relation_Option"))
}

// Every alias, both namespaces, through the vocabulary door: the spelling
// is the alias, and the alias names its key back. The dictionary door is
// pinned by TestDictionaryKeys_* (which sweep the whole bundled table);
// this is the table-driven statement of the sixteen renames themselves.
func TestAlias_TheWholeTableRoundTrips(t *testing.T) {
	for stored, wire := range map[string]string{
		"featuredRelations":            "featured_properties",
		"relationKey":                  "property_key",
		"relationOptionColor":          "property_option_color",
		"relationMaxCount":             "property_max_count",
		"relationReadonlyValue":        "property_readonly_value",
		"relationFormat":               "property_format",
		"relationFormatIncludeTime":    "property_format_include_time",
		"relationFormatObjectTypes":    "property_format_object_types",
		"relationDefaultValue":         "property_default_value",
		"headerRelationsLayout":        "header_properties_layout",
		"recommendedRelations":         "recommended_properties",
		"recommendedFeaturedRelations": "recommended_featured_properties",
		"recommendedHiddenRelations":   "recommended_hidden_properties",
		"recommendedFileRelations":     "recommended_file_properties",
	} {
		assert.Equal(t, wire, (BundledKeyVocabulary{}).PropertySlug(stored))
		back, ok := (BundledKeyVocabulary{}).PropertyKey(wire)
		require.True(t, ok, wire)
		assert.Equal(t, stored, back)
	}
	for stored, wire := range map[string]string{
		"relation":       "property",
		"relationOption": "property_option",
	} {
		assert.Equal(t, wire, (BundledKeyVocabulary{}).TypeSlug(stored))
		back, ok := (BundledKeyVocabulary{}).TypeKey(wire)
		require.True(t, ok, wire)
		assert.Equal(t, stored, back)
	}
}
