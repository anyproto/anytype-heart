package anyblockjson

// documentkind_test.go — telling the format's three grammars apart (§2c, §2f).

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A reader handed a file with no filename to go by has to place it. The
// declared `$schema` is the author's own statement and outranks inference;
// shape answers for a document that declares none.
//
// How this can fail: match `$schema` exactly instead of by suffix and every
// document written against another format version becomes unplaceable; infer
// from a member two grammars share and ordinary objects start being read as
// dictionaries.
func TestDocumentKind_PlacesTheThreeGrammars(t *testing.T) {
	t.Run("the declaration is believed", func(t *testing.T) {
		for _, tc := range []struct{ doc, want string }{
			{`{"$schema": "` + SchemaURL + `", "version": 1}`, KindObject},
			{`{"$schema": "` + IndexSchemaURL + `", "version": 1}`, KindIndex},
			{`{"$schema": "` + PropertiesSchemaURL + `", "version": 1}`, KindPropertyDictionary},
		} {
			assert.Equal(t, tc.want, DocumentKind([]byte(tc.doc)), tc.doc)
		}
	})

	// `$schema` is decorative for VALIDITY — only `version` gates the format
	// — so a bundle written against a later schema still reads, and it must
	// still be placeable. Matching the whole URL would break that.
	t.Run("the version segment is ignored", func(t *testing.T) {
		assert.Equal(t, KindIndex, DocumentKind([]byte(
			`{"$schema": "https://schemas.anytype.io/anyblock/9/index.schema.json", "version": 1}`)))
	})

	t.Run("a schema nobody publishes decides nothing", func(t *testing.T) {
		// an author reached for `.../relation.schema.json`, which does not
		// exist; it must fall through to shape rather than be believed
		assert.Equal(t, KindObject, DocumentKind([]byte(
			`{"$schema": "https://schemas.anytype.io/anyblock/1/relation.schema.json",
			  "version": 1, "kind": "property", "internal_key": "estimate"}`)))
	})

	t.Run("shape places a document that declares nothing", func(t *testing.T) {
		for _, tc := range []struct{ name, doc, want string }{
			{"installed is a dictionary's alone", `{"version": 1, "installed": ["done"]}`, KindPropertyDictionary},
			{"and so is a properties ARRAY", `{"version": 1, "properties": [{"property": "k", "format": "text"}]}`, KindPropertyDictionary},
			{"a properties MAP is an object's", `{"version": 1, "properties": {"name": "Note"}}`, KindObject},
			{"a manifest is an index's", `{"version": 1, "manifest": {"properties": "properties.json"}}`, KindIndex},
			{"so are widgets", `{"version": 1, "widgets": [{"target": "page-home"}]}`, KindIndex},
		} {
			assert.Equal(t, tc.want, DocumentKind([]byte(tc.doc)), tc.name)
		}
	})
}

// Handed to the wrong reader, a correct document used to be walked through
// the ways it fails to be something it never claimed to be: an index was told
// `/name: property "name" is not allowed` — on the field whose whole job is
// to name the space — and a dictionary `/properties: got array, want object`.
// Both send an author to repair a file that is already right.
//
// How this can fail: report the misroute on a document that carries no
// evidence at all, and `{"version": 2}` stops getting the newer-format
// verdict it needs.
func TestDocumentKind_TheWrongReaderSaysSo(t *testing.T) {
	index := `{"$schema": "` + IndexSchemaURL + `", "version": 1, "name": "Company Wiki"}`
	dict := `{"$schema": "` + PropertiesSchemaURL + `", "version": 1, "installed": ["done"]}`
	object := `{"$schema": "` + SchemaURL + `", "version": 1, "properties": {"name": "Note"}}`

	t.Run("an index read as an object", func(t *testing.T) {
		err := Validate([]byte(index))
		require.Error(t, err)
		assert.Contains(t, err.(*ValidationError).Issues[0].String(), "this is a bundle index")
		assert.Contains(t, err.(*ValidationError).Issues[0].String(), "UnmarshalIndex")
		assert.NotContains(t, err.Error(), `"name" is not allowed`,
			"the old verdict blamed the field that names the space")
	})

	t.Run("a dictionary read as an object", func(t *testing.T) {
		err := Validate([]byte(dict))
		require.Error(t, err)
		assert.Contains(t, err.(*ValidationError).Issues[0].String(), "this is a property dictionary")
		assert.NotContains(t, err.Error(), "want object")
	})

	t.Run("an object read as an index, or as a dictionary", func(t *testing.T) {
		_, err := UnmarshalIndex([]byte(object))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "an object document")

		_, err = UnmarshalPropertyDictionary([]byte(object))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "an object document")
	})

	t.Run("each grammar still reads its own", func(t *testing.T) {
		require.NoError(t, Validate([]byte(object)))
		_, err := UnmarshalIndex([]byte(index))
		require.NoError(t, err)
		_, err = UnmarshalPropertyDictionary([]byte(dict))
		require.NoError(t, err)
	})

	// `{"version": 1}` is a legal start to all three grammars. A reader that
	// guessed here would override its caller on no evidence.
	t.Run("a document with no evidence is left to its caller", func(t *testing.T) {
		_, err := UnmarshalPropertyDictionary([]byte(`{"version": 2}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "newer version",
			"the version gate must still be what answers")

		require.NoError(t, Validate([]byte(`{"version": 1}`)))
	})

	// The identity a reader dispatches on must not become the thing that
	// decides validity: a stale or invented $schema stays decorative.
	t.Run("a stale schema url is still valid", func(t *testing.T) {
		require.NoError(t, Validate([]byte(
			`{"$schema": "https://schemas.anytype.io/anyblock/9/object.schema.json",
			  "version": 1, "blocks": [{"type": "paragraph", "text": "fine"}]}`)))
	})
}
