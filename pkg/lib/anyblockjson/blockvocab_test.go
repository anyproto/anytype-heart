package anyblockjson

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBlockVocabularyIsTheSchemaEnum pins the exported §5 vocabulary to the
// embedded schema's own enum — the list is READ from it, so this asserts the
// reading, not a hand-kept copy. A type added to the schema appears here with
// no code change; a type that stops being in the schema disappears from every
// surface that publishes the vocabulary at the same moment.
func TestBlockVocabularyIsTheSchemaEnum(t *testing.T) {
	// given
	var doc map[string]any
	require.NoError(t, json.Unmarshal(SchemaJSON(), &doc))
	defs := doc["$defs"].(map[string]any)
	blockCore := defs["blockCore"].(map[string]any)
	props := blockCore["properties"].(map[string]any)
	typeProp := props["type"].(map[string]any)
	var want []string
	for _, v := range typeProp["enum"].([]any) {
		want = append(want, v.(string))
	}

	// when
	got := BlockTypeNames()

	// then
	require.NotEmpty(t, want)
	assert.Equal(t, want, got, "the exported vocabulary is the schema's own enum, in schema order")
	assert.Contains(t, got, "checkbox", "the type a checkbox item needs is in the vocabulary")
}

// TestAuthorableVocabularyDropsTheStructuralTypes: §7 structural blocks are
// part of the format's vocabulary but not of a body a caller can author —
// import absorbs title/description into properties and drops
// featuredProperties, so a payload naming one produces no block. A published
// enum must not offer a value that cannot survive.
func TestAuthorableVocabularyDropsTheStructuralTypes(t *testing.T) {
	// when
	authorable := AuthorableBlockTypeNames()

	// then
	assert.Len(t, authorable, len(BlockTypeNames())-len(structuralBlockTypes)-len(transparentBlockTypes))
	for _, typ := range []string{"title", "description", "featured_properties"} {
		assert.True(t, StructuralBlockType(typ), "%s is structural (§7)", typ)
		assert.NotContains(t, authorable, typ)
		assert.Contains(t, BlockTypeNames(), typ, "it is still part of the format's vocabulary")
	}
	assert.False(t, StructuralBlockType("paragraph"))
	assert.Subset(t, BlockTypeNames(), authorable)
}

// TestAuthorableVocabularyDropsTheTransparentContainers is the §7a half of
// the same rule, and the reason the two sets stay apart: a structural block
// is dropped WITH its subtree, a container is dropped and its subtree stays.
// Both are readable, neither is authorable.
func TestAuthorableVocabularyDropsTheTransparentContainers(t *testing.T) {
	// when
	authorable := AuthorableBlockTypeNames()

	// then
	for typ := range transparentBlockTypes {
		assert.True(t, TransparentBlockType(typ), "%s is a transparent container (§7a)", typ)
		assert.False(t, StructuralBlockType(typ), "%s is not structural — its subtree survives", typ)
		assert.NotContains(t, authorable, typ)
		assert.Contains(t, BlockTypeNames(), typ,
			"it stays readable: Validate must keep accepting what Unmarshal accepts (I2)")
	}
	assert.False(t, TransparentBlockType("paragraph"))
	assert.False(t, TransparentBlockType("row"), "a row is author-created and grammar-bearing (§5)")
}

// TestStructuralTypesAreDroppedOnImport is the behaviour the exclusion above
// claims: the importer reads the same map, so every structural name really
// does produce no block.
func TestStructuralTypesAreDroppedOnImport(t *testing.T) {
	// the baseline: the same document without the structural block
	_, plain, err := Unmarshal([]byte(`{"version":1,"type":"page","blocks":[{"type":"paragraph","text":"body"}]}`), Options{})
	require.NoError(t, err)

	for typ := range structuralBlockTypes {
		t.Run(typ, func(t *testing.T) {
			// given — featuredProperties carries no text of its own (§5)
			structural := fmt.Sprintf(`{"type":%q,"text":"structural"}`, typ)
			if typ == "featured_properties" {
				structural = fmt.Sprintf(`{"type":%q}`, typ)
			}
			doc := fmt.Sprintf(`{"version":1,"type":"page","blocks":[%s,{"type":"paragraph","text":"body"}]}`, structural)

			// when
			_, snapshot, err := Unmarshal([]byte(doc), Options{})

			// then
			require.NoError(t, err)
			assert.Len(t, snapshot.Blocks, len(plain.Blocks), "a structural block must not survive as a block")
			for _, b := range snapshot.Blocks {
				if text := b.GetText(); text != nil {
					assert.NotEqual(t, "structural", text.Text)
				}
			}
		})
	}
}

// TestBlockVocabularyCopiesOut: callers get their own slice — the package
// list is read at startup and shared by every surface that publishes it.
func TestBlockVocabularyCopiesOut(t *testing.T) {
	got := BlockTypeNames()
	got[0] = "clobbered"
	assert.NotEqual(t, "clobbered", BlockTypeNames()[0])
}
