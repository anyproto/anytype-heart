package notion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func tagProperty(id, name string) propertySchema {
	return propertySchema{Id: id, Type: "multi_select", Name: name}
}

func TestTagRedirectIsSpaceWide(t *testing.T) {
	t.Run("every database's Tags property redirects onto the one bundled relation", func(t *testing.T) {
		// given — two databases each carrying a "Tags" multi_select, which the
		// committed cassette also contains (ids Bfgr and yq%7B~). A shared
		// vocabulary across every type is the whole point of tags, and the one
		// bundled target kept for exactly that reason.
		store := newPropertiesStore()
		first := tagProperty("Bfgr", "Tags")
		second := tagProperty("yq%7B~", "Tags")
		store.noteName(first.Name)
		store.noteName(second.Name)

		// when
		firstDef, firstCreated := store.resolveRelation(first)
		secondDef, secondCreated := store.resolveRelation(second)

		// then
		require.NotNil(t, firstDef)
		require.NotNil(t, secondDef)
		assert.Equal(t, bundle.RelationKeyTag.String(), firstDef.key)
		assert.Equal(t, bundle.RelationKeyTag.String(), secondDef.key,
			"a later database's Tags must not mint a private relation with its own option pool")
		assert.False(t, firstCreated, "the bundled tag relation is never emitted as a new object")
		assert.False(t, secondCreated)
	})

	t.Run("an exact Tag property still wins over Tags", func(t *testing.T) {
		// given — v1's precedence rule: "Tags" only redirects when no property
		// named exactly "Tag" exists anywhere
		store := newPropertiesStore()
		store.noteName("Tag")
		store.noteName("Tags")

		// when
		tagsDef, _ := store.resolveRelation(tagProperty("p1", "Tags"))
		tagDef, _ := store.resolveRelation(tagProperty("p2", "Tag"))

		// then
		require.NotNil(t, tagsDef)
		require.NotNil(t, tagDef)
		assert.NotEqual(t, bundle.RelationKeyTag.String(), tagsDef.key,
			"Tags must not take the bundled relation when an exact Tag exists")
		assert.Equal(t, bundle.RelationKeyTag.String(), tagDef.key)
	})

	t.Run("same-named properties in different databases stay separate", func(t *testing.T) {
		// given — the c6e29db27 rule: a property belongs to its own database
		store := newPropertiesStore()
		first := propertySchema{Id: "catA", Type: "select", Name: "Category"}
		second := propertySchema{Id: "catB", Type: "select", Name: "Category"}

		// when
		firstDef, _ := store.resolveRelation(first)
		secondDef, _ := store.resolveRelation(second)

		// then
		require.NotNil(t, firstDef)
		require.NotNil(t, secondDef)
		assert.NotEqual(t, firstDef.key, secondDef.key)
	})
}
