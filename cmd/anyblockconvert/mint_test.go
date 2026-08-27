package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
)

// A property an author declared by NAME or spelling gets a fresh internal key
// minted, the way the app mints one when a user creates a property
// (objectcreator/relation.go: bson.NewObjectId().Hex()). A property that
// STATED its internal_key keeps it exactly — that is the whole point of
// stating one: a bundle re-imported elsewhere reproduces the same stored key.
//
// This tool used to key the relation by the spelling in both cases, so a
// generated bundle imported with a stored key no real space would have.
//
// How this can fail: mint for a stated key and re-import stops reproducing
// the space; key by the spelling and a generated bundle diverges from what
// the app creates.
func TestMint_ASpellingGetsAFreshKeyAStatedKeyIsKept(t *testing.T) {
	b := newBatch(nil, nil)

	authored := anyblockjson.PropertyDefinition{Key: "cooking_time", Name: "Cooking Time"}
	id, ok := b.PropertyId(authored)
	require.True(t, ok)
	assert.NotContains(t, id, "cooking_time", "the spelling must not become the stored key")

	minted, ok := b.PropertyKey("cooking_time")
	require.True(t, ok, "the spelling must be bound in the batch vocabulary")
	assert.True(t, looksLikeMintedKey(minted), "a minted key is a bson id, got %q", minted)

	t.Run("the same spelling resolves to the same key across the batch", func(t *testing.T) {
		// this is what makes minting safe: a recipe document carrying
		// "cooking_time": 90 must land on the very key the dictionary minted,
		// not write a detail no relation object describes
		again, _ := b.PropertyId(authored)
		assert.Equal(t, id, again)

		key2, _ := b.PropertyKey("cooking_time")
		assert.Equal(t, minted, key2)
		assert.Equal(t, "cooking_time", b.PropertySlug(minted), "and the binding inverts")
	})

	t.Run("a stated internal key is reproduced exactly", func(t *testing.T) {
		stated := anyblockjson.PropertyDefinition{
			Key: "6a32d4856761631534b22f85", Name: "Project", KeyIsInternal: true}
		id, ok := b.PropertyId(stated)
		require.True(t, ok)
		assert.Contains(t, id, "6a32d4856761631534b22f85")
	})

	t.Run("a bundled property is never minted", func(t *testing.T) {
		id, ok := b.PropertyId(anyblockjson.PropertyDefinition{Key: "dueDate", Name: "Due date"})
		require.True(t, ok)
		assert.Contains(t, id, "dueDate", "a bundled key resolves to its bundled url")
	})
}
