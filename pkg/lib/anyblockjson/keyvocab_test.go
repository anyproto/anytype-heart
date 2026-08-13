package anyblockjson

import (
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/iancoleman/strcase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// TestKeyVocabulary_ReverseIsATableNotACaseTransform is the guard rail on the
// one decision that looks most "simplifiable" in this whole layer: the
// reverse direction of the vocabulary is a TABLE built from the bundle, in
// both directions, and NOT a case transform. Two bundled keys prove no case
// function can do the job. Replace BundledKeyVocabulary.PropertyKey with any
// `strcase.ToLowerCamel`-shaped inversion and this test fails.
func TestKeyVocabulary_ReverseIsATableNotACaseTransform(t *testing.T) {
	vocab := BundledKeyVocabulary{}

	t.Run("mediaArtistURL: an acronym a case transform cannot restore", func(t *testing.T) {
		// given
		const key = "mediaArtistURL"
		require.True(t, bundle.HasRelation("mediaArtistURL"))

		// when
		slug := vocab.PropertySlug(key)
		back, ok := vocab.PropertyKey(slug)

		// then
		assert.Equal(t, "media_artist_url", slug)
		require.True(t, ok)
		assert.Equal(t, key, back, "the table must invert what no case transform can")
		assert.Equal(t, "mediaArtistUrl", strcase.ToLowerCamel(slug),
			"and this is what a case transform would have produced instead")
	})

	t.Run("_score: a leading underscore a case transform eats", func(t *testing.T) {
		// given
		const key = "_score"
		require.True(t, bundle.HasRelation("_score"))

		// when
		slug := vocab.PropertySlug(key)
		back, ok := vocab.PropertyKey(slug)

		// then
		assert.Equal(t, "_score", slug)
		require.True(t, ok)
		assert.Equal(t, key, back)
		assert.Equal(t, "Score", strcase.ToLowerCamel(slug),
			"a case transform loses the underscore AND capitalizes")
	})

	t.Run("every bundled key round-trips through the table", func(t *testing.T) {
		for _, key := range []string{"dueDate", "iconEmoji", "lastModifiedDate", "setOf", "name", "_final_score"} {
			back, ok := vocab.PropertyKey(vocab.PropertySlug(key))
			require.True(t, ok, key)
			assert.Equal(t, key, back)
		}
		for _, key := range []string{"objectType", "relationOption", "spaceView", "diaryEntry", "chatDerived", "page"} {
			back, ok := vocab.TypeKey(vocab.TypeSlug(key))
			require.True(t, ok, key)
			assert.Equal(t, key, back)
		}
	})
}

func TestKeyVocabulary_CustomKeysPassThrough(t *testing.T) {
	vocab := BundledKeyVocabulary{}

	// a package-only reader has no space to ask about stored slugs, so a
	// custom key is spelled — and read back — verbatim (§7.5a-5 chain step 1:
	// an exact stored key always wins over the slug layer)
	for _, key := range []string{"68b1c0aa4e1f0d0011223344", "myLegacyKey", "customStatus"} {
		assert.Equal(t, key, vocab.PropertySlug(key))
		back, ok := vocab.PropertyKey(key)
		assert.False(t, ok)
		assert.Equal(t, key, back)
	}
}

// TestDocumentSpellsSlugs pins the §7.5a surface rule at the format level: a
// document names properties and types by slug, and reading it back restores
// the stored keys. Revert any of the boundary sites (export.go, dataview.go,
// typeproperties.go, import.go) and one of these fails.
func TestDocumentSpellsSlugs(t *testing.T) {
	t.Run("properties are spelled as slugs and read back as stored keys", func(t *testing.T) {
		// given
		doc := `{"version": 1, "id": "o1", "properties": {
			"name": "A page", "icon_emoji": "🔥", "due_date": "2025-07-06T08:44:05Z",
			"customDate": "whatever"}}`

		// when — the document's slugs bind to stored keys
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)

		// then
		assert.Contains(t, snap.Details.Fields, "iconEmoji")
		assert.Contains(t, snap.Details.Fields, "dueDate")
		assert.NotContains(t, snap.Details.Fields, "due_date")
		assert.Contains(t, snap.Details.Fields, "customDate", "custom keys pass through")

		// and the export spells them back
		data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
		require.NoError(t, err)
		assert.Contains(t, string(data), `"icon_emoji"`)
		assert.Contains(t, string(data), `"due_date"`)
		assert.NotContains(t, string(data), `"iconEmoji"`)
		assert.Contains(t, string(data), `"customDate"`)
	})

	t.Run("a dataview's key slots follow the same vocabulary", func(t *testing.T) {
		doc := `{"version": 1, "id": "o1", "blocks": [{"id": "dv", "type": "dataview",
			"properties": [{"key": "due_date", "format": "date"}],
			"views": [{"id": "v1", "type": "table", "groupBy": "due_date",
				"sorts": [{"property": "due_date"}],
				"filters": [{"property": "due_date", "condition": "notEmpty"}],
				"columns": [{"property": "due_date"}]}]}]}`

		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)

		var dv *model.BlockContentDataview
		for _, b := range snap.Blocks {
			if c, ok := b.Content.(*model.BlockContentOfDataview); ok {
				dv = c.Dataview
			}
		}
		require.NotNil(t, dv)
		require.Len(t, dv.RelationLinks, 1)
		assert.Equal(t, "dueDate", dv.RelationLinks[0].Key)
		require.Len(t, dv.Views, 1)
		assert.Equal(t, "dueDate", dv.Views[0].GroupRelationKey)
		require.Len(t, dv.Views[0].Sorts, 1)
		assert.Equal(t, "dueDate", dv.Views[0].Sorts[0].RelationKey)
		require.Len(t, dv.Views[0].Filters, 1)
		assert.Equal(t, "dueDate", dv.Views[0].Filters[0].RelationKey)
		require.Len(t, dv.Views[0].Relations, 1)
		assert.Equal(t, "dueDate", dv.Views[0].Relations[0].Key)
	})

	t.Run("the envelope type is a slug too", func(t *testing.T) {
		doc := `{"version": 1, "kind": "objectType", "id": "t1", "type": "object_type"}`
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		require.Len(t, snap.ObjectTypes, 1)
		assert.Equal(t, "ot-objectType", snap.ObjectTypes[0])
	})
}

// collapsingVocab spells two stored keys as ONE slug — the shape a space
// holding a pre-mint-check shadow really has (a UI property that took
// `due_date` beside the bundled `dueDate`). No bundled fixture can produce
// it, because the bundled table is injective by construction.
type collapsingVocab struct{ a, b, slug string }

func (v collapsingVocab) PropertySlug(key string) string {
	if key == v.a || key == v.b {
		return v.slug
	}
	return key
}

func (v collapsingVocab) PropertyKey(slug string) (string, bool) {
	if slug == v.slug {
		return v.a, true
	}
	return slug, false
}

func (v collapsingVocab) TypeSlug(key string) string      { return key }
func (v collapsingVocab) TypeKey(s string) (string, bool) { return s, false }

// TestBuildPropertiesKeepsBothValuesWhenSlugsCollapse is the data-loss guard
// in export.go's buildProperties. Two stored keys spelling one JSON key would
// overwrite each other in the properties map — one value gone, no error, no
// warning. The later holder keeps its honest stored key instead, and WHICH
// holder that is may not depend on Go's map iteration order: the collapse
// pass runs over the sorted stored keys, or the canonical form is a coin flip
// on exactly the spaces that hold a shadow. Revert either the
// `spelled[slug]` branch or the `sort.Strings(keys)` and this fails (the
// second one intermittently, which is the point).
func TestBuildPropertiesKeepsBothValuesWhenSlugsCollapse(t *testing.T) {
	// given
	snapshot := &model.SmartBlockSnapshotBase{
		Details: &types.Struct{Fields: map[string]*types.Value{
			"aaaKey": pbtypes.String("value of A"),
			"zzzKey": pbtypes.String("value of Z"),
		}},
	}
	vocab := collapsingVocab{a: "aaaKey", b: "zzzKey", slug: "shared_slug"}

	// when: repeated, because map iteration order is randomized per run
	for i := 0; i < 32; i++ {
		data, err := Marshal(model.SmartBlockType_Page, snapshot, Options{Keys: vocab})

		// then
		require.NoError(t, err)
		var doc struct {
			Properties map[string]string `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(data, &doc))
		assert.Len(t, doc.Properties, 2, "no value may be lost to a collapsed spelling")
		assert.Equal(t, "value of A", doc.Properties["shared_slug"], "the first stored key keeps the slug, every run")
		assert.Equal(t, "value of Z", doc.Properties["zzzKey"], "the later one keeps its honest stored key")
	}
}

// TestBuildPropertiesRefusesASlugAnotherStoredKeyOwns is the second arm of the
// same collapse: the contested spelling is not another holder's SLUG but
// another stored key on this very object. Emitting it would bind the value to
// that key on the way back (chain step 1 — an exact stored key always wins).
func TestBuildPropertiesRefusesASlugAnotherStoredKeyOwns(t *testing.T) {
	// given
	snapshot := &model.SmartBlockSnapshotBase{
		Details: &types.Struct{Fields: map[string]*types.Value{
			"aaaKey":      pbtypes.String("value of A"),
			"shared_slug": pbtypes.String("value of the squatter"),
		}},
	}
	vocab := collapsingVocab{a: "aaaKey", b: "", slug: "shared_slug"}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snapshot, Options{Keys: vocab})

	// then
	require.NoError(t, err)
	var doc struct {
		Properties map[string]string `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Len(t, doc.Properties, 2)
	assert.Equal(t, "value of the squatter", doc.Properties["shared_slug"])
	assert.Equal(t, "value of A", doc.Properties["aaaKey"], "the slug is not emitted — its spelling is taken")
}

// TestObjectTypesIsAKeySlot pins the vocabulary decision for
// typeProperties[].objectTypes (§2a's target-type restriction). It NAMES
// types, so it is a type-key slot and speaks the one vocabulary — the same
// answer the envelope `type` gets. It was the last untranslated key slot in
// the format; revert the typeSlugs/typeKeys calls in typeproperties.go and
// this fails in both directions.
func TestObjectTypesIsAKeySlot(t *testing.T) {
	t.Run("import inverts the slug to the stored type key", func(t *testing.T) {
		// given
		doc := `{"version": 1, "kind": "objectType", "id": "t1", "key": "k",
			"typeProperties": [{"key": "owner", "name": "Owner", "format": "objects",
			 "objectTypes": ["object_type", "wikiPerson"]}]}`
		r := &recordingPropertyResolver{}

		// when
		_, _, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), ResolveProperties: r})

		// then
		require.NoError(t, err)
		require.Len(t, r.defs, 1)
		assert.Equal(t, []string{"objectType", "wikiPerson"}, r.defs[0].ObjectTypes,
			"the bundled slug inverts; an unknown term passes through (chain step 1)")
	})

	t.Run("export spells the slug", func(t *testing.T) {
		snapshot := &model.SmartBlockSnapshotBase{
			Details: &types.Struct{Fields: map[string]*types.Value{
				"recommendedRelations": pbtypes.StringList([]string{"rel-owner"}),
			}},
			ObjectTypes: []string{"ot-objectType"},
		}
		resolver := &staticPropertyResolver{def: PropertyDefinition{
			Key: "owner", Name: "Owner", Format: model.RelationFormat_object,
			ObjectTypes: []string{"objectType", "wikiPerson"},
		}}

		data, err := Marshal(model.SmartBlockType_STType, snapshot, Options{ResolveProperties: resolver})

		require.NoError(t, err)
		var doc struct {
			TypeProperties []TypeProperty `json:"typeProperties"`
		}
		require.NoError(t, json.Unmarshal(data, &doc))
		require.Len(t, doc.TypeProperties, 1)
		assert.Equal(t, []string{"object_type", "wikiPerson"}, doc.TypeProperties[0].ObjectTypes)
	})
}

// TestBuildRecommendedListsInvertsItsKeySlots: the PATCH-types channel writes
// the SAME §2a array a type document carries, so it must invert the same key
// slots through the same vocabulary. It took a bare resolver and inverted
// nothing, which made one type's property list mean two different things
// depending on which endpoint wrote it.
func TestBuildRecommendedListsInvertsItsKeySlots(t *testing.T) {
	// given
	r := &recordingPropertyResolver{}
	props := []TypeProperty{{
		Key:         "due_date",
		Name:        "Due date",
		Format:      "date",
		ObjectTypes: []string{"object_type", "wikiPerson"},
		Section:     "featured",
	}}

	// when
	lists := BuildRecommendedLists(props, Options{ResolveProperties: r})

	// then
	require.Len(t, r.defs, 1)
	assert.Equal(t, domain.RelationKey("dueDate"), r.defs[0].Key,
		"the resolver receives the def import would hand it — stored spellings")
	assert.Equal(t, []string{"objectType", "wikiPerson"}, r.defs[0].ObjectTypes)
	require.NotEmpty(t, lists)
	assert.Equal(t, "recommendedFeaturedRelations", lists[0].DetailKey)
	assert.Equal(t, []string{"dueDate"}, lists[0].Ids)
}
