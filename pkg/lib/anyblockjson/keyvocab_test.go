package anyblockjson

import (
	"testing"

	"github.com/iancoleman/strcase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
