package storeresolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The space-backed key vocabulary (ADDRESSING §7.5a). Both directions are
// pinned here, and the EMIT side is the half that had nothing: it returned
// bundle.ApiSlug unconditionally, never consulting the space, so a document
// could label a value with a key denoting a different entity while the API's
// own listing (servedKey, which applies the same guards) advertised another.
//
// Fixtures are BSON-keyed with a stored apiObjectKey, or stored keys the
// bundled table resolves elsewhere. A key the bundled table happens to invert
// cannot tell the two vocabularies apart.

const (
	bsonPropKey = "6a7663db61fab21cd4b9e101"
	bsonTwinKey = "6a7663db61fab21cd4b9e102"
	bsonTypeKey = "6a7663db61fab21cd4b9e103"
)

// vocabFixture builds a resolver over exactly the rows given.
func vocabFixture(t *testing.T, objects ...spaceindex.TestObject) *Resolvers {
	index := spaceindex.NewStoreFixture(t)
	if len(objects) > 0 {
		index.AddObjects(t, objects)
	}
	return New(index)
}

func relationRow(id, key, slug string) spaceindex.TestObject {
	row := spaceindex.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyRelationKey:    domain.String(key),
		bundle.RelationKeyName:           domain.String(id),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
	}
	if slug != "" {
		row[bundle.RelationKeyApiObjectKey] = domain.String(slug)
	}
	return row
}

func typeRow(id, key, slug string) spaceindex.TestObject {
	row := spaceindex.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyUniqueKey:      domain.String("ot-" + key),
		bundle.RelationKeyName:           domain.String(id),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
	}
	if slug != "" {
		row[bundle.RelationKeyApiObjectKey] = domain.String(slug)
	}
	return row
}

func TestPropertyKeyVocabulary(t *testing.T) {
	t.Run("a stored slug spells the BSON key, both directions", func(t *testing.T) {
		// given
		r := vocabFixture(t, relationRow("rel-manual", bsonPropKey, "manual_property"))

		// when / then
		assert.Equal(t, "manual_property", r.PropertySlug(bsonPropKey))
		key, ok := r.PropertyKey("manual_property")
		require.True(t, ok)
		assert.Equal(t, bsonPropKey, key)
	})

	t.Run("a bundled key spells its derived slug with no stored detail", func(t *testing.T) {
		r := vocabFixture(t, relationRow("rel-due", "dueDate", ""))

		assert.Equal(t, "due_date", r.PropertySlug("dueDate"))
		key, ok := r.PropertyKey("due_date")
		require.True(t, ok)
		assert.Equal(t, "dueDate", key)
	})

	// TestPropertyKeyVocabulary/chain step 1: revert the storedKey branch in
	// PropertyKey and this resolves to the BUNDLED relation instead — a write
	// addressed at the legacy relation landing on a different property.
	t.Run("an exact live stored key wins over the bundled slug layer", func(t *testing.T) {
		// given: a legacy relation STORED under `due_date`, which is also
		// bundled dueDate's derived slug
		r := vocabFixture(t,
			relationRow("rel-legacy", "due_date", ""),
			relationRow("rel-due", "dueDate", ""),
		)

		// when
		key, ok := r.PropertyKey("due_date")

		// then
		assert.False(t, ok, "not a slug — a stored key, verbatim")
		assert.Equal(t, "due_date", key, "chain step 1: the bundled table is never consulted")
	})

	t.Run("a key the vocabulary does not know passes through both ways", func(t *testing.T) {
		r := vocabFixture(t)

		assert.Equal(t, "whatever", r.PropertySlug("whatever"))
		key, ok := r.PropertyKey("whatever")
		assert.False(t, ok)
		assert.Equal(t, "whatever", key)
	})

	// keyMaps.add's twin-slug drop, BOTH directions. The invariant is that
	// slugByKey holds a spelling only for an UNAMBIGUOUS holder: clearing just
	// the reverse map left the first holder still claiming a slug the accept
	// side refuses to invert.
	t.Run("add drops a twin slug from both directions", func(t *testing.T) {
		// given
		m := newKeyMaps(nil)

		// when
		m.add(bsonPropKey, "manual_property")
		m.add(bsonTwinKey, "manual_property")

		// then
		assert.Equal(t, "", m.keyBySlug["manual_property"], "neither holder wins the reverse direction")
		assert.NotContains(t, m.slugByKey, bsonPropKey, "and the first holder stops claiming the spelling")
		assert.NotContains(t, m.slugByKey, bsonTwinKey)
		assert.True(t, m.storedKey[bsonPropKey], "both stay addressable by their stored keys")
		assert.True(t, m.storedKey[bsonTwinKey])
	})

	t.Run("twin slugs resolve to neither, and neither is emitted", func(t *testing.T) {
		// given
		r := vocabFixture(t,
			relationRow("rel-manual", bsonPropKey, "manual_property"),
			relationRow("rel-twin", bsonTwinKey, "manual_property"),
		)

		// when / then
		_, ok := r.PropertyKey("manual_property")
		assert.False(t, ok, "an ambiguous address must never resolve by store order")
		assert.Equal(t, bsonPropKey, r.PropertySlug(bsonPropKey), "the FIRST holder keeps the honest stored key too")
		assert.Equal(t, bsonTwinKey, r.PropertySlug(bsonTwinKey))
	})

	// The emit side's three round-trip guards, one per chain step.
	t.Run("a slug a live stored key answers to is not emitted", func(t *testing.T) {
		// given: someone's stored key IS another's slug — step 1 wins on
		// input, so emitting the slug would label the wrong row
		r := vocabFixture(t,
			relationRow("rel-manual", bsonPropKey, "manual_property"),
			relationRow("rel-legacy", "manual_property", ""),
		)

		assert.Equal(t, bsonPropKey, r.PropertySlug(bsonPropKey))
	})

	t.Run("a bundled key whose derived slug is squatted keeps its stored key", func(t *testing.T) {
		// given: the §7.5a-6 shadow — a UI property took `due_date`, so the
		// spelling is ambiguous on input and cannot label the bundled value
		r := vocabFixture(t,
			relationRow("rel-due", "dueDate", ""),
			relationRow("rel-squatter", bsonPropKey, "due_date"),
		)

		assert.Equal(t, "dueDate", r.PropertySlug("dueDate"),
			"the bundled value must not be labeled with the squatter's address")
	})

	t.Run("a stored slug the bundled table resolves elsewhere is not emitted", func(t *testing.T) {
		// same shadow seen from the squatter's side, and with the bundled
		// relation NOT installed — nothing in the space reveals the clash
		r := vocabFixture(t, relationRow("rel-squatter", bsonPropKey, "due_date"))

		assert.Equal(t, bsonPropKey, r.PropertySlug(bsonPropKey))
	})

	// loadKeyMaps' corpse filter (§7.5-requirement-2): a UI-deleted entity
	// vacates the slug namespace here as everywhere else. Drop the
	// isUninstalled filter and the live holder loses its slug to a twin that
	// no listing shows and no route resolves.
	t.Run("a UI-deleted holder vacates the slug namespace", func(t *testing.T) {
		// given
		corpse := relationRow("rel-corpse", bsonTwinKey, "manual_property")
		corpse[bundle.RelationKeyIsUninstalled] = domain.Bool(true)
		r := vocabFixture(t, relationRow("rel-manual", bsonPropKey, "manual_property"), corpse)

		// when / then
		assert.Equal(t, "manual_property", r.PropertySlug(bsonPropKey), "the live holder keeps the slug")
		key, ok := r.PropertyKey("manual_property")
		require.True(t, ok)
		assert.Equal(t, bsonPropKey, key)
		assert.Equal(t, bsonTwinKey, r.PropertySlug(bsonTwinKey), "the corpse spells nothing")
	})

	t.Run("the emitted slug always inverts back to the key it labels", func(t *testing.T) {
		// the whole contract in one loop, over a space holding every shape at
		// once: whatever PropertySlug emits, PropertyKey must invert
		r := vocabFixture(t,
			relationRow("rel-manual", bsonPropKey, "manual_property"),
			relationRow("rel-twinA", bsonTwinKey, "shared_slug"),
			relationRow("rel-twinB", "6a7663db61fab21cd4b9e104", "shared_slug"),
			relationRow("rel-due", "dueDate", ""),
			relationRow("rel-squatter", "6a7663db61fab21cd4b9e105", "due_date"),
			relationRow("rel-legacy", "manual_alias", ""),
			relationRow("rel-aliased", "6a7663db61fab21cd4b9e106", "manual_alias"),
		)

		for _, key := range []string{
			bsonPropKey, bsonTwinKey, "6a7663db61fab21cd4b9e104", "dueDate",
			"6a7663db61fab21cd4b9e105", "manual_alias", "6a7663db61fab21cd4b9e106",
		} {
			slug := r.PropertySlug(key)
			back, ok := r.PropertyKey(slug)
			if !ok {
				// not a slug: the emitted spelling must then be the key itself,
				// which is always an address (chain step 1)
				assert.Equal(t, key, back, "emitted %q for %q", slug, key)
				continue
			}
			assert.Equal(t, key, back, "emitted %q for %q, which inverts elsewhere", slug, key)
		}
	})
}

func TestTypeKeyVocabulary(t *testing.T) {
	t.Run("a stored slug spells the BSON type key, both directions", func(t *testing.T) {
		r := vocabFixture(t, typeRow("type-meeting", bsonTypeKey, "meeting_note"))

		assert.Equal(t, "meeting_note", r.TypeSlug(bsonTypeKey))
		key, ok := r.TypeKey("meeting_note")
		require.True(t, ok)
		assert.Equal(t, bsonTypeKey, key)
	})

	t.Run("a bundled type spells its derived slug", func(t *testing.T) {
		r := vocabFixture(t, typeRow("type-objectType", "objectType", ""))

		assert.Equal(t, "object_type", r.TypeSlug("objectType"))
		key, ok := r.TypeKey("object_type")
		require.True(t, ok)
		assert.Equal(t, "objectType", key)
	})

	t.Run("an exact live stored type key wins over the bundled slug layer", func(t *testing.T) {
		r := vocabFixture(t,
			typeRow("type-legacy", "object_type", ""),
			typeRow("type-objectType", "objectType", ""),
		)

		key, ok := r.TypeKey("object_type")
		assert.False(t, ok)
		assert.Equal(t, "object_type", key)
	})

	t.Run("a bundled type whose derived slug is squatted keeps its stored key", func(t *testing.T) {
		// the type namespace has the same hole as the property one
		r := vocabFixture(t,
			typeRow("type-objectType", "objectType", ""),
			typeRow("type-squatter", bsonTypeKey, "object_type"),
		)

		assert.Equal(t, "objectType", r.TypeSlug("objectType"))
	})

	t.Run("twin type slugs resolve to neither, and neither is emitted", func(t *testing.T) {
		r := vocabFixture(t,
			typeRow("type-a", bsonTypeKey, "meeting_note"),
			typeRow("type-b", "6a7663db61fab21cd4b9e107", "meeting_note"),
		)

		_, ok := r.TypeKey("meeting_note")
		assert.False(t, ok)
		assert.Equal(t, bsonTypeKey, r.TypeSlug(bsonTypeKey))
		assert.Equal(t, "6a7663db61fab21cd4b9e107", r.TypeSlug("6a7663db61fab21cd4b9e107"))
	})
}

// TestKeyVocabularyWiring pins that the resolvers ARE the vocabulary the
// Options carry — the read half and the write half must hand the codec the
// same table, which is what Wave 1 shipped broken on the write side.
func TestKeyVocabularyWiring(t *testing.T) {
	r := vocabFixture(t)

	opts := r.Options()

	assert.Equal(t, r, opts.Keys)
}
