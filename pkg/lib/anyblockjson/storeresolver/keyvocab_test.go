package storeresolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
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
		m.add(bsonPropKey, "manual_property", false)
		m.add(bsonTwinKey, "manual_property", false)

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

// hiddenRelationRow is a relation the app hides from the user: invisible in
// every listing, and — this is what makes it dangerous as a slug holder —
// undeletable through the API. Whatever slug it occupies is occupied forever
// with no visible cause.
func hiddenRelationRow(id, key, slug string) spaceindex.TestObject {
	row := relationRow(id, key, slug)
	row[bundle.RelationKeyIsHidden] = domain.Bool(true)
	return row
}

// TestHiddenHoldersDoNotOwnSlugs is the one-place-for-one-rule test. The
// hidden-holder exclusion was implemented only in v2's request namespace
// (core/api/v2/service/keys.go), and this vocabulary — which decides what a
// DOCUMENT's keys bind to — still counted them. The two builders disagreeing
// on one spelling is the whole defect: the listing serves an address the
// write half resolves elsewhere, or nowhere.
func TestHiddenHoldersDoNotOwnSlugs(t *testing.T) {
	t.Run("a hidden twin does not erase a visible holder's slug", func(t *testing.T) {
		// given — the dangling-key shape: a visible relation and a hidden one
		// both slugged `severity`. GET /properties advertises `severity` (v2
		// already excludes the hidden holder); before this, keyBySlug dropped
		// BOTH, so a POST naming `severity` stored `severity` verbatim as a
		// relationLink key no relation object owns. 200 OK, no warning.
		r := vocabFixture(t,
			relationRow("rel-visible", bsonPropKey, "severity"),
			hiddenRelationRow("rel-hidden", bsonTwinKey, "severity"),
		)

		// when
		key, ok := r.PropertyKey("severity")

		// then
		require.True(t, ok, "the visible holder owns the slug alone")
		assert.Equal(t, bsonPropKey, key)
		assert.Equal(t, "severity", r.PropertySlug(bsonPropKey), "and the listing's spelling is emitted")
	})

	t.Run("a hidden squatter does not win a bundled property's own slug", func(t *testing.T) {
		// given — the wrong-property shape: installed bundled dueDate plus a
		// hidden squatter holding `due_date`. The listing serves `due_date`
		// for the bundled one and the chain resolves it to bundled dueDate,
		// so this vocabulary resolving it to the SQUATTER wrote the value
		// onto an invisible relation.
		r := vocabFixture(t,
			relationRow("rel-due", "dueDate", ""),
			hiddenRelationRow("rel-squatter", bsonPropKey, "due_date"),
		)

		// when
		key, ok := r.PropertyKey("due_date")

		// then
		require.True(t, ok)
		assert.Equal(t, "dueDate", key, "the bundled property, not the invisible squatter")
		assert.Equal(t, "due_date", r.PropertySlug("dueDate"),
			"and the emit side agrees, so the document round-trips")
	})

	t.Run("a hidden entity keeps its stored key as an address", func(t *testing.T) {
		// chain step 1 is not a namespace question: the stored key is always
		// an address, and the emit side must still refuse to spell someone
		// else's value with it
		r := vocabFixture(t,
			hiddenRelationRow("rel-hidden", "hidden_key", "hidden_slug"),
			relationRow("rel-other", bsonPropKey, "hidden_key"),
		)

		key, ok := r.PropertyKey("hidden_key")
		assert.False(t, ok, "a stored key, verbatim — never the slug layer")
		assert.Equal(t, "hidden_key", key)
		assert.Equal(t, bsonPropKey, r.PropertySlug(bsonPropKey),
			"and the other holder does not emit a spelling the hidden stored key answers to")
	})

	t.Run("the type namespace follows the same rule", func(t *testing.T) {
		// given
		hidden := typeRow("type-hidden", "6a7663db61fab21cd4b9e107", "invoice")
		hidden[bundle.RelationKeyIsHidden] = domain.Bool(true)
		r := vocabFixture(t, typeRow("type-visible", bsonTypeKey, "invoice"), hidden)

		// when
		key, ok := r.TypeKey("invoice")

		// then
		require.True(t, ok)
		assert.Equal(t, bsonTypeKey, key)
		assert.Equal(t, "invoice", r.TypeSlug(bsonTypeKey))
	})
}

// TestAcceptHalfFolds is chain step 4 on the accept side (§7.5a-3). The
// vocabulary implemented steps 1–3 and then degraded VERBATIM, while the v2
// route layer folded — so one request could resolve a term through
// /properties and store it unfolded as a dataview column key in the same
// breath. Only updateView was covered (canonicalViewKey).
func TestAcceptHalfFolds(t *testing.T) {
	// a BSON-keyed relation with a stored slug: nothing about this fixture is
	// resolvable through the bundled table, so a pass means the SPACE's fold
	// class answered
	fold := func(t *testing.T) *Resolvers {
		return vocabFixture(t, relationRow("rel-sev", bsonPropKey, "severity"))
	}

	t.Run("case and separator variants of a stored slug fold to it", func(t *testing.T) {
		for _, input := range []string{"Severity", "SEVERITY", "sever_ity", "sever-ity"} {
			key, ok := fold(t).PropertyKey(input)
			require.True(t, ok, input)
			assert.Equal(t, bsonPropKey, key, input)
		}
	})

	t.Run("an exact stored key still wins the fold", func(t *testing.T) {
		// given — `severity` is one relation's stored KEY and another's slug
		r := vocabFixture(t,
			relationRow("rel-sev", bsonPropKey, "severity"),
			relationRow("rel-legacy", "Severity", ""),
		)

		// when / then — step 1, exact, before any folding
		key, ok := r.PropertyKey("Severity")
		assert.False(t, ok)
		assert.Equal(t, "Severity", key, "an exact stored key is never folded away")
	})

	t.Run("an ambiguous fold degrades verbatim, never guesses", func(t *testing.T) {
		// given — two live relations whose slugs fold together
		r := vocabFixture(t,
			relationRow("rel-a", bsonPropKey, "mood_level"),
			relationRow("rel-b", bsonTwinKey, "moodlevel"),
		)

		// when
		key, ok := r.PropertyKey("MoodLevel")

		// then
		assert.False(t, ok)
		assert.Equal(t, "MoodLevel", key, "the term passes through — the git rule, never a guess")
	})

	t.Run("a hidden holder does not answer the fold", func(t *testing.T) {
		r := vocabFixture(t, hiddenRelationRow("rel-hidden", bsonPropKey, "severity"))

		key, ok := r.PropertyKey("Severity")
		assert.False(t, ok)
		assert.Equal(t, "Severity", key)
	})

	t.Run("the bundled fold table is consulted too", func(t *testing.T) {
		r := vocabFixture(t)

		key, ok := r.PropertyKey("DueDate")
		require.True(t, ok)
		assert.Equal(t, "dueDate", key)
	})

	t.Run("a term nothing folds to passes through", func(t *testing.T) {
		key, ok := vocabFixture(t).PropertyKey("no_such_thing")
		assert.False(t, ok)
		assert.Equal(t, "no_such_thing", key, "a miss must return the term, not an empty key")
	})

	t.Run("the type namespace folds the same way", func(t *testing.T) {
		r := vocabFixture(t, typeRow("type-inv", bsonTypeKey, "invoice"))

		key, ok := r.TypeKey("Invoice")
		require.True(t, ok)
		assert.Equal(t, bsonTypeKey, key)
	})
}

// TestCorpseSlugLifecycleInDocumentVocabulary — the corpse (uninstalled)
// story on the DOCUMENT vocabulary, both store shapes a real UI delete can
// leave (flag-only {isUninstalled}, and the prod double-flag
// {isUninstalled, isDeleted} that delete.go + detailsinject.go persist).
//
// Fixture discipline: the corpse is BSON-keyed WITH a stored apiObjectKey —
// if loadKeyMaps stopped excluding corpses, the slug would resolve and emit
// again and every assertion here flips; a readable or slug-less corpse key
// could detect neither direction.
func TestCorpseSlugLifecycleInDocumentVocabulary(t *testing.T) {
	corpse := func(prodShape bool) spaceindex.TestObject {
		row := relationRow("rel-corpse", bsonPropKey, "warranty_until")
		row[bundle.RelationKeyIsUninstalled] = domain.Bool(true)
		if prodShape {
			row[bundle.RelationKeyIsDeleted] = domain.Bool(true)
		}
		return row
	}

	for _, shape := range []struct {
		name string
		prod bool
	}{{"flag-only shape", false}, {"prod shape", true}} {
		t.Run(shape.name+": the slug is severed in both directions", func(t *testing.T) {
			// given — the corpse is the ONLY holder of warranty_until
			r := vocabFixture(t, corpse(shape.prod))

			// then: emit degrades to the stored key (a document written NOW
			// pins the address that survives a later re-mint of the slug)…
			assert.Equal(t, bsonPropKey, r.PropertySlug(bsonPropKey))
			// …and the slug no longer resolves — a document written BEFORE
			// the uninstall keeps its term verbatim on import, landing the
			// value under a string key no relation owns (executed below)
			key, ok := r.PropertyKey("warranty_until")
			assert.False(t, ok)
			assert.Equal(t, "warranty_until", key)
		})
	}

	t.Run("a pre-uninstall document imports its value onto a key no relation owns", func(t *testing.T) {
		// given — the export produced while the property was live spells the
		// slug; the property has since been UI-deleted
		index := spaceindex.NewStoreFixture(t)
		index.AddObjects(t, []spaceindex.TestObject{corpse(true)})
		doc := []byte(`{"version":1,"id":"obj1","type":"page","properties":{"name":"Doc","warranty_until":"2027-01-01"}}`)

		// when
		_, snapshot, err := anyblockjson.Unmarshal(doc, New(index).Options())

		// then — the term passes through verbatim (the git rule, never a
		// guess): the value lands under the literal slug string, NOT under
		// the corpse's stored key. Byte-safe, address-orphaned.
		require.NoError(t, err)
		assert.Equal(t, "2027-01-01", snapshot.Details.Fields["warranty_until"].GetStringValue())
		assert.Nil(t, snapshot.Details.Fields[bsonPropKey])
	})

	t.Run("after a recreate the corpse-era slug re-aims onto the new holder", func(t *testing.T) {
		// given — the vacate lean's (§8-OQ2) executed consequence: P held
		// warranty_until and was uninstalled; Q minted it fresh
		recreated := relationRow("rel-recreated", bsonTwinKey, "warranty_until")
		r := vocabFixture(t, corpse(true), recreated)

		// when
		key, ok := r.PropertyKey("warranty_until")

		// then — same bytes, different property: a document exported while P
		// was live now binds Q. Pinned as current, documented behavior; it is
		// also why the emit side must keep degrading a corpse to its stored
		// key (the assertion above), or post-uninstall exports would join
		// this re-aim class too.
		require.True(t, ok)
		assert.Equal(t, bsonTwinKey, key)
		assert.Equal(t, "warranty_until", r.PropertySlug(bsonTwinKey), "the new holder owns the spelling")
	})
}
