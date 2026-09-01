package storeresolver

import (
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The space-backed key vocabulary (§3): a key's document spelling is its
// display NAME, NFC and otherwise verbatim, and the accept side inverts
// exactly what the emit side writes. Names are not unique, so the accept
// side REFUSES a shared spelling rather than picking a holder, and exposes
// the candidates through ScopedKeyVocabulary for the importer's type-scoped
// resolution. `apiObjectKey` is never read: the last test pins that a
// stored slug moves nothing here any more.

const (
	bsonPropKey = "6a7663db61fab21cd4b9e101"
	bsonTwinKey = "6a7663db61fab21cd4b9e102"
	bsonTypeKey = "6a7663db61fab21cd4b9e103"
)

// named gives a row the display name the §3 label rule reads.
func named(row spaceindex.TestObject, name string) spaceindex.TestObject {
	row[bundle.RelationKeyName] = domain.String(name)
	return row
}

// nameless strips a row's display name: such an entity has no label but its
// stored key.
func nameless(row spaceindex.TestObject) spaceindex.TestObject {
	delete(row, bundle.RelationKeyName)
	return row
}

// vocabFixture builds a resolver over exactly the rows given.
func vocabFixture(t *testing.T, objects ...spaceindex.TestObject) *Resolvers {
	index := spaceindex.NewStoreFixture(t)
	if len(objects) > 0 {
		index.AddObjects(t, objects)
	}
	return New(index)
}

func relationRow(id, key, name string) spaceindex.TestObject {
	row := spaceindex.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyRelationKey:    domain.String(key),
		bundle.RelationKeyName:           domain.String(name),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
	}
	return row
}

func typeRow(id, key, name string) spaceindex.TestObject {
	row := spaceindex.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyUniqueKey:      domain.String("ot-" + key),
		bundle.RelationKeyName:           domain.String(name),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
	}
	return row
}

func TestPropertyKeyVocabulary(t *testing.T) {
	t.Run("a name spells the BSON key, both directions", func(t *testing.T) {
		// given
		r := vocabFixture(t, relationRow("rel-manual", bsonPropKey, "Manual property"))

		// when / then
		assert.Equal(t, "Manual property", r.PropertySlug(bsonPropKey))
		key, ok := r.PropertyKey("Manual property")
		require.True(t, ok)
		assert.Equal(t, bsonPropKey, key)
	})

	t.Run("a bundled key spells its display name from the code table", func(t *testing.T) {
		r := vocabFixture(t, relationRow("rel-due", "dueDate", "Due date"))

		assert.Equal(t, "Due date", r.PropertySlug("dueDate"))
		key, ok := r.PropertyKey("Due date")
		require.True(t, ok)
		assert.Equal(t, "dueDate", key)
	})

	// chain step 2: revert the storedKey branch in PropertyKey and this
	// resolves to the BUNDLED relation instead — a write addressed at the
	// legacy relation landing on a different property.
	t.Run("an exact live stored key wins over every table", func(t *testing.T) {
		// given: a legacy relation STORED under the very string that is
		// bundled dueDate's display name
		r := vocabFixture(t,
			relationRow("rel-legacy", "Due date", "Something else"),
			relationRow("rel-due", "dueDate", "Due date"),
		)

		// when
		key, ok := r.PropertyKey("Due date")

		// then
		assert.False(t, ok, "not a name — a stored key, verbatim")
		assert.Equal(t, "Due date", key, "the bundled table is never consulted")
		// and the emit side agrees: the bundled key cannot spell a term a
		// live stored key owns, so it degrades to its own stored key
		assert.Equal(t, "dueDate", r.PropertySlug("dueDate"))
	})

	t.Run("a key the vocabulary does not know passes through both ways", func(t *testing.T) {
		r := vocabFixture(t)

		assert.Equal(t, "whatever", r.PropertySlug("whatever"))
		key, ok := r.PropertyKey("whatever")
		assert.False(t, ok)
		assert.Equal(t, "whatever", key)
	})

	// Names are NOT unique, and the vocabulary does not pretend they are:
	// collisions are per DOCUMENT (the exporter's term ledger), so both
	// holders spell the plain name — and the accept side refuses to pick.
	t.Run("a shared name is spelled by every holder and resolved by none", func(t *testing.T) {
		// given — the corpus shape: two live properties named "Projects"
		r := vocabFixture(t,
			relationRow("rel-a", bsonPropKey, "Projects"),
			relationRow("rel-b", bsonTwinKey, "Projects"),
		)

		// when / then
		assert.Equal(t, "Projects", r.PropertySlug(bsonPropKey))
		assert.Equal(t, "Projects", r.PropertySlug(bsonTwinKey),
			"both spell it: a name ambiguous space-wide is still unambiguous in nearly every document")
		_, ok := r.PropertyKey("Projects")
		assert.False(t, ok, "an ambiguous address must never resolve by store order")
		assert.Equal(t, []string{bsonPropKey, bsonTwinKey}, r.PropertyKeyCandidates("Projects"),
			"the candidates are exposed for the importer's type-scoped resolution")
	})

	t.Run("a nameless relation has no label but its stored key", func(t *testing.T) {
		r := vocabFixture(t, nameless(relationRow("rel-x", bsonPropKey, "")))

		assert.Equal(t, bsonPropKey, r.PropertySlug(bsonPropKey), "the stored key is always its own address")
	})

	// The ladder: the one spelling no entity may take is another live
	// entity's STORED KEY — verbatim-first outranks every table, so such a
	// label could never resolve to its owner anywhere.
	t.Run("a name that is someone's stored key degrades through the ladder", func(t *testing.T) {
		// given: a bson-keyed relation NAMED with the exact string another
		// relation is stored under
		r := vocabFixture(t,
			named(relationRow("rel-named", bsonPropKey, ""), "manual_property"),
			named(relationRow("rel-stored", "manual_property", ""), "Something else"),
		)

		// when / then — rung (b): the claimant's own key is a minted bson
		// id, so it takes `<name> (<tail6>)`, deterministic and invertible
		assert.Equal(t, "manual_property (b9e101)", r.PropertySlug(bsonPropKey))
		key, ok := r.PropertyKey("manual_property")
		assert.False(t, ok, "an exact stored key wins over any label")
		assert.Equal(t, "manual_property", key)
	})

	t.Run("a readable stored key is its own disambiguation", func(t *testing.T) {
		// rung (a): the claimant's own key is readable, so it spells itself
		r := vocabFixture(t,
			named(relationRow("rel-named", "producer_region", ""), "wine_region"),
			named(relationRow("rel-stored", "wine_region", ""), "Region"),
		)

		assert.Equal(t, "producer_region", r.PropertySlug("producer_region"),
			"a readable stored key needs no suffix — it is the honest spelling")
	})

	// A UI-deleted holder vacates the name namespace. Drop the
	// isUninstalled filter and the freed name is shadowed by a corpse no
	// listing shows.
	t.Run("a UI-deleted holder vacates the name namespace", func(t *testing.T) {
		// given
		corpse := relationRow("rel-corpse", bsonTwinKey, "Warranty until")
		corpse[bundle.RelationKeyIsUninstalled] = domain.Bool(true)
		r := vocabFixture(t, relationRow("rel-live", bsonPropKey, "Warranty until"), corpse)

		// when / then
		assert.Equal(t, "Warranty until", r.PropertySlug(bsonPropKey), "the live holder keeps the name")
		key, ok := r.PropertyKey("Warranty until")
		require.True(t, ok)
		assert.Equal(t, bsonPropKey, key)
		assert.Equal(t, bsonTwinKey, r.PropertySlug(bsonTwinKey), "the corpse spells nothing")
	})

	t.Run("the emitted spelling always inverts back to the key it labels", func(t *testing.T) {
		// the whole contract in one loop, over a space holding every shape
		// at once: whatever PropertySlug emits either inverts to the same
		// key or IS the key (its own address)
		r := vocabFixture(t,
			relationRow("rel-manual", bsonPropKey, "Manual property"),
			relationRow("rel-twinA", bsonTwinKey, "Shared name"),
			relationRow("rel-twinB", "6a7663db61fab21cd4b9e104", "Shared name"),
			relationRow("rel-due", "dueDate", "Due date"),
			relationRow("rel-shadow", "6a7663db61fab21cd4b9e105", "Due date"),
			relationRow("rel-legacy", "manual_alias", "Legacy"),
			named(relationRow("rel-squat", "6a7663db61fab21cd4b9e106", ""), "manual_alias"),
		)

		for _, key := range []string{
			bsonPropKey, bsonTwinKey, "6a7663db61fab21cd4b9e104", "dueDate",
			"6a7663db61fab21cd4b9e105", "manual_alias", "6a7663db61fab21cd4b9e106",
		} {
			spelling := r.PropertySlug(key)
			back, ok := r.PropertyKey(spelling)
			if !ok {
				// not a unique name: the emitted spelling is then either the
				// key itself (always an address) or a spelling the DOCUMENT
				// ledger will disambiguate — never one that inverts elsewhere
				if spelling == key {
					assert.Equal(t, key, back, "emitted %q for %q", spelling, key)
					continue
				}
				assert.Contains(t, r.PropertyKeyCandidates(spelling), key,
					"emitted the shared %q for %q — the holder must be among its candidates", spelling, key)
				continue
			}
			assert.Equal(t, key, back, "emitted %q for %q, which inverts elsewhere", spelling, key)
		}
	})
}

func TestTypeKeyVocabulary(t *testing.T) {
	t.Run("a name spells the BSON type key, both directions", func(t *testing.T) {
		r := vocabFixture(t, typeRow("type-meeting", bsonTypeKey, "Meeting note"))

		assert.Equal(t, "Meeting note", r.TypeSlug(bsonTypeKey))
		key, ok := r.TypeKey("Meeting note")
		require.True(t, ok)
		assert.Equal(t, bsonTypeKey, key)
	})

	t.Run("a bundled type spells its display name", func(t *testing.T) {
		r := vocabFixture(t, typeRow("type-objectType", "objectType", "Type"))

		assert.Equal(t, "Type", r.TypeSlug("objectType"))
		key, ok := r.TypeKey("Type")
		require.True(t, ok)
		assert.Equal(t, "objectType", key)
	})

	t.Run("an exact live stored type key wins over every table", func(t *testing.T) {
		r := vocabFixture(t,
			typeRow("type-legacy", "Task", "Legacy task"),
			typeRow("type-task", "task", "Task"),
		)

		key, ok := r.TypeKey("Task")
		assert.False(t, ok)
		assert.Equal(t, "Task", key)
		assert.Equal(t, "task", r.TypeSlug("task"),
			"and the bundled type degrades to its own stored key")
	})

	t.Run("a shared type name is spelled by every holder and resolved by none", func(t *testing.T) {
		r := vocabFixture(t,
			typeRow("type-a", bsonTypeKey, "Meeting note"),
			typeRow("type-b", "6a7663db61fab21cd4b9e107", "Meeting note"),
		)

		_, ok := r.TypeKey("Meeting note")
		assert.False(t, ok)
		assert.Equal(t, "Meeting note", r.TypeSlug(bsonTypeKey))
		assert.Equal(t, "Meeting note", r.TypeSlug("6a7663db61fab21cd4b9e107"))
		assert.Len(t, r.TypeKeyCandidates("Meeting note"), 2)
	})
}

// TestKeyVocabularyWiring pins that the resolvers ARE the vocabulary the
// Options carry — the read half and the write half must hand the codec the
// same table.
func TestKeyVocabularyWiring(t *testing.T) {
	r := vocabFixture(t)

	opts := r.Options()

	assert.Equal(t, r, opts.Keys)
}

// hiddenRelationRow is a relation the app hides from the user: invisible in
// every listing, and undeletable through the API — whatever spelling it
// occupied would be occupied forever with no visible cause, which is why it
// occupies none.
func hiddenRelationRow(id, key, name string) spaceindex.TestObject {
	row := relationRow(id, key, name)
	row[bundle.RelationKeyIsHidden] = domain.Bool(true)
	return row
}

// TestHiddenHoldersDoNotOwnNames is the one-place-for-one-rule test: v2's
// request namespace excludes hidden holders (core/api/v2/service/keys.go),
// and this vocabulary — which decides what a DOCUMENT's keys bind to — must
// agree, or the listing serves an address the write half resolves elsewhere.
func TestHiddenHoldersDoNotOwnNames(t *testing.T) {
	t.Run("a hidden twin does not make a visible holder's name ambiguous", func(t *testing.T) {
		// given — a visible relation and a hidden one both named "Severity"
		r := vocabFixture(t,
			relationRow("rel-visible", bsonPropKey, "Severity"),
			hiddenRelationRow("rel-hidden", bsonTwinKey, "Severity"),
		)

		// when
		key, ok := r.PropertyKey("Severity")

		// then
		require.True(t, ok, "the visible holder owns the name alone")
		assert.Equal(t, bsonPropKey, key)
		assert.Equal(t, "Severity", r.PropertySlug(bsonPropKey))
	})

	t.Run("a hidden entity keeps its stored key as an address", func(t *testing.T) {
		// chain step 2 is not a namespace question: the stored key is always
		// an address, and the emit side must still refuse to spell someone
		// else's entity with it
		r := vocabFixture(t,
			hiddenRelationRow("rel-hidden", "hidden_key", "Hidden"),
			named(relationRow("rel-other", bsonPropKey, ""), "hidden_key"),
		)

		key, ok := r.PropertyKey("hidden_key")
		assert.False(t, ok, "a stored key, verbatim — never the name layer")
		assert.Equal(t, "hidden_key", key)
		assert.NotEqual(t, "hidden_key", r.PropertySlug(bsonPropKey),
			"and the other holder does not emit a spelling the hidden stored key answers to")
	})

	t.Run("a custom relation sharing a bundled NAME does not capture it", func(t *testing.T) {
		// the space holds only the custom relation named "Priority" — a real
		// corpus population, 25 custom names equal a bundled relation name.
		// Both spell it; the accept side refuses to pick, and the importer's
		// type scope or the document's legend is what decides.
		r := vocabFixture(t, relationRow("rel-custom", bsonPropKey, "Priority"))

		_, ok := r.PropertyKey("Priority")
		assert.False(t, ok, "shared between the space and the bundled table: never a guess")
		assert.Equal(t, []string{bsonPropKey, "priority"}, r.PropertyKeyCandidates("Priority"),
			"the bundled binding is among the candidates")
		assert.Equal(t, "Priority", r.PropertySlug(bsonPropKey),
			"the custom holder still spells its name — collisions are per document")
		assert.Equal(t, "Priority", r.PropertySlug("priority"),
			"and so does the bundled key: the code table is its authority")
	})

	t.Run("the type namespace follows the same hidden rule", func(t *testing.T) {
		// given
		hidden := typeRow("type-hidden", "6a7663db61fab21cd4b9e107", "Invoice")
		hidden[bundle.RelationKeyIsHidden] = domain.Bool(true)
		r := vocabFixture(t, typeRow("type-visible", bsonTypeKey, "Invoice"), hidden)

		// when
		key, ok := r.TypeKey("Invoice")

		// then
		require.True(t, ok)
		assert.Equal(t, bsonTypeKey, key)
		assert.Equal(t, "Invoice", r.TypeSlug(bsonTypeKey))
	})
}

// TestAcceptHalfFolds is chain step 4 on the accept side: the forgiving
// layer, answering only when exactly one candidate remains. The extended
// fold drops case, `_`, `-`, spaces and invisible code points, so the
// legacy name-derived labels (`publish_date` for "Publish Date") land in
// their name's own class and documents already written keep resolving.
func TestAcceptHalfFolds(t *testing.T) {
	sev := func(t *testing.T) *Resolvers {
		return vocabFixture(t, relationRow("rel-sev", bsonPropKey, "Severity"))
	}

	t.Run("case, separator and legacy-label variants fold to the name", func(t *testing.T) {
		for _, input := range []string{"severity", "SEVERITY", "sever_ity", "sever-ity", " Severity "} {
			key, ok := sev(t).PropertyKey(input)
			require.True(t, ok, input)
			assert.Equal(t, bsonPropKey, key, input)
		}
	})

	t.Run("an exact stored key still wins the fold", func(t *testing.T) {
		// given — `severity` is one relation's stored KEY and folds onto
		// another's name
		r := vocabFixture(t,
			relationRow("rel-sev", bsonPropKey, "Severity"),
			relationRow("rel-legacy", "severity", "Old severity"),
		)

		// when / then — step 2, exact, before any folding
		key, ok := r.PropertyKey("severity")
		assert.False(t, ok)
		assert.Equal(t, "severity", key, "an exact stored key is never folded away")
	})

	t.Run("an ambiguous fold degrades verbatim, never guesses", func(t *testing.T) {
		// given — two live relations whose names fold together
		r := vocabFixture(t,
			relationRow("rel-a", bsonPropKey, "Mood level"),
			relationRow("rel-b", bsonTwinKey, "Moodlevel"),
		)

		// when
		key, ok := r.PropertyKey("MOOD_LEVEL")

		// then
		assert.False(t, ok)
		assert.Equal(t, "MOOD_LEVEL", key, "the term passes through — never a guess")
	})

	t.Run("a hidden holder does not answer the fold", func(t *testing.T) {
		r := vocabFixture(t, hiddenRelationRow("rel-hidden", bsonPropKey, "Severity"))

		key, ok := r.PropertyKey("severity")
		assert.False(t, ok)
		assert.Equal(t, "severity", key)
	})

	t.Run("the bundled fold table is consulted too", func(t *testing.T) {
		r := vocabFixture(t)

		// the pre-change derived slug: fold(ToSnake(key)) == fold(key), so
		// it lands in the stored key's class with no compatibility table
		key, ok := r.PropertyKey("due_date")
		require.True(t, ok)
		assert.Equal(t, "dueDate", key)
	})

	t.Run("a space claimant makes a bundled fold class ambiguous", func(t *testing.T) {
		// given — a custom relation named so that it folds together with a
		// bundled key's class: the layer must see BOTH candidates and refuse
		r := vocabFixture(t, relationRow("rel-b", bsonPropKey, "Due-Date"))

		key, ok := r.PropertyKey("due_date")
		assert.False(t, ok)
		assert.Equal(t, "due_date", key, "two candidates in one class: never a guess")
	})

	t.Run("a term nothing folds to passes through", func(t *testing.T) {
		key, ok := vocabFixture(t).PropertyKey("no_such_thing")
		assert.False(t, ok)
		assert.Equal(t, "no_such_thing", key, "a miss must return the term, not an empty key")
	})

	t.Run("the type namespace folds the same way", func(t *testing.T) {
		r := vocabFixture(t, typeRow("type-inv", bsonTypeKey, "Invoice"))

		key, ok := r.TypeKey("invoice")
		require.True(t, ok)
		assert.Equal(t, bsonTypeKey, key)
	})
}

// TestCorpseNameLifecycle — the corpse (uninstalled) story, both store
// shapes a real UI delete can leave (flag-only {isUninstalled}, and the
// prod double-flag {isUninstalled, isDeleted}).
func TestCorpseNameLifecycle(t *testing.T) {
	corpse := func(prodShape bool) spaceindex.TestObject {
		row := relationRow("rel-corpse", bsonPropKey, "Warranty until")
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
		t.Run(shape.name+": the name is severed in both directions", func(t *testing.T) {
			// given — the corpse is the ONLY holder of "Warranty until"
			r := vocabFixture(t, corpse(shape.prod))

			// then: emit degrades to the stored key…
			assert.Equal(t, bsonPropKey, r.PropertySlug(bsonPropKey))
			// …and the name no longer resolves — a document written BEFORE
			// the uninstall keeps its term verbatim on import, landing the
			// value under a string key no relation owns (executed below)
			key, ok := r.PropertyKey("Warranty until")
			assert.False(t, ok)
			assert.Equal(t, "Warranty until", key)
		})
	}

	t.Run("a pre-uninstall document imports its value onto a key no relation owns", func(t *testing.T) {
		// given — the export produced while the property was live spells the
		// name; the property has since been UI-deleted. (A REAL export also
		// carries the legend line that would resolve this — the fixture
		// drops it to show the legendless degradation.)
		index := spaceindex.NewStoreFixture(t)
		index.AddObjects(t, []spaceindex.TestObject{corpse(true)})
		doc := []byte(`{"formatVersion":"2.0","id":"obj1","type":"Page","properties":{"Name":"Doc","Warranty until":"2027-01-01"}}`)

		// when
		_, snapshot, err := anyblockjson.Unmarshal(doc, New(index).Options())

		// then — the term passes through verbatim, never a guess: the value
		// lands under the literal name string, NOT under the corpse's stored
		// key. Byte-safe, address-orphaned.
		require.NoError(t, err)
		assert.Equal(t, "2027-01-01", snapshot.Details.Fields["Warranty until"].GetStringValue())
		assert.Nil(t, snapshot.Details.Fields[bsonPropKey])
	})

	t.Run("after a recreate the corpse-era name re-aims onto the new holder", func(t *testing.T) {
		// given — P held "Warranty until" and was uninstalled; Q was minted
		// fresh under the same name
		recreated := relationRow("rel-recreated", bsonTwinKey, "Warranty until")
		r := vocabFixture(t, corpse(true), recreated)

		// when
		key, ok := r.PropertyKey("Warranty until")

		// then — same bytes, different property: a legendless document
		// exported while P was live now binds Q. This is §6's freed-name
		// hazard, pinned as current documented behavior; an EXPORTED
		// document is protected by its legend line.
		require.True(t, ok)
		assert.Equal(t, bsonTwinKey, key)
		assert.Equal(t, "Warranty until", r.PropertySlug(bsonTwinKey), "the new holder owns the spelling")
	})
}

// TestCorpseStoredKeyStillNamesItsObjects is the corpse story from the OTHER
// side, and the one that loses data: not the corpse's name, but the corpse's
// STORED KEY, which every object it ever typed or tagged still carries.
//
// The delete vacates the name namespace, so `initiative` stops being a live
// stored key — while a live entity is NAMED "initiative", which is how a
// user frees a name and reuses it. The vocabulary emits the corpse's stored
// key verbatim (nothing else is an address) and grants the live holder its
// name only where no live stored key owns the string; the DOCUMENT is what
// says which of the two it means, and the identity entry is that statement.
//
// Both arms run the real exporter over the real resolver.
func TestCorpseStoredKeyStillNamesItsObjects(t *testing.T) {
	t.Run("a type whose key the space no longer reserves", func(t *testing.T) {
		// given
		dead := typeRow("t-dead", "initiative", "Initiative")
		dead[bundle.RelationKeyIsUninstalled] = domain.Bool(true)
		r := vocabFixture(t, dead, typeRow("t-live", bsonTypeKey, "initiative"))
		require.Equal(t, "initiative", r.TypeSlug("initiative"),
			"the corpse's stored key is its own address — there is no name to spell it with")
		key, ok := r.TypeKey("initiative")
		require.True(t, ok)
		require.Equal(t, bsonTypeKey, key,
			"the corpse vacated the string, and the live holder is NAMED it — the fixture is the collision")

		snapshot := &model.SmartBlockSnapshotBase{
			Details:     &types.Struct{Fields: map[string]*types.Value{"id": strValue("obj1")}},
			ObjectTypes: []string{"ot-initiative"},
		}

		// when
		data, err := anyblockjson.Marshal(model.SmartBlockType_Page, snapshot, r.Options())
		require.NoError(t, err)

		// then
		require.NoError(t, anyblockjson.Validate(data))
		var doc struct {
			Type     string            `json:"type"`
			TypeKeys map[string]string `json:"type_internal_keys"`
		}
		require.NoError(t, json.Unmarshal(data, &doc))
		assert.Equal(t, "initiative", doc.Type)
		assert.Equal(t, map[string]string{"initiative": "initiative"}, doc.TypeKeys,
			"the document says the term is a stored key, or a reader whose space "+
				"binds the name takes it for the live holder")

		_, back, err := anyblockjson.Unmarshal(data, r.Options())
		require.NoError(t, err)
		assert.Equal(t, []string{"ot-initiative"}, back.ObjectTypes)
	})

	t.Run("a property whose key the space no longer reserves", func(t *testing.T) {
		// given — the live holder is NAMED "initiative", the exact string
		// the corpse is stored under, so the ladder gives it the suffixed
		// spelling: the plain string could never resolve to it anywhere
		dead := relationRow("rel-dead", "initiative", "Initiative")
		dead[bundle.RelationKeyIsUninstalled] = domain.Bool(true)
		r := vocabFixture(t, dead, named(relationRow("rel-live", bsonPropKey, ""), "initiative"))

		snapshot := &model.SmartBlockSnapshotBase{Details: &types.Struct{Fields: map[string]*types.Value{
			"id":         strValue("obj1"),
			"initiative": strValue("value of the deleted property"),
			bsonPropKey:  strValue("value of the live one"),
		}}}

		// when
		data, err := anyblockjson.Marshal(model.SmartBlockType_Page, snapshot, r.Options())
		require.NoError(t, err)

		// then
		require.NoError(t, anyblockjson.Validate(data))
		var doc struct {
			Properties   map[string]string `json:"properties"`
			PropertyKeys map[string]string `json:"property_internal_keys"`
		}
		require.NoError(t, json.Unmarshal(data, &doc))
		assert.Equal(t, "value of the deleted property", doc.Properties["initiative"])
		assert.Equal(t, "value of the live one", doc.Properties["initiative (b9e101)"],
			"the live holder's plain name is a stored key on this space, so it "+
				"takes the suffixed form — visibly synthetic, immutable while the key lives")
		assert.Equal(t, map[string]string{
			"initiative":          "initiative",
			"initiative (b9e101)": bsonPropKey,
		}, doc.PropertyKeys,
			"the identity entry for the stored key, and the suffix's inverse — no "+
				"shipped table binds either term")

		// and both values come home
		_, back, err := anyblockjson.Unmarshal(data, r.Options())
		require.NoError(t, err)
		assert.Equal(t, "value of the deleted property",
			back.Details.Fields["initiative"].GetStringValue())
		assert.Equal(t, "value of the live one",
			back.Details.Fields[bsonPropKey].GetStringValue())
	})
}

func strValue(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}

// The dead machinery, pinned dead: `apiObjectKey` is never read by the
// format. A stored slug neither spells its holder nor binds on the accept
// side — the name is the whole of the spelling rule, and the API surface's
// key convention is a separate decision.
func TestApiObjectKeyLeavesTheFormat(t *testing.T) {
	row := relationRow("rel-x", bsonPropKey, "Publish Date")
	row[bundle.RelationKeyApiObjectKey] = domain.String("publish_date_custom")
	r := vocabFixture(t, row)

	assert.Equal(t, "Publish Date", r.PropertySlug(bsonPropKey),
		"the name, not the stored slug")
	_, ok := r.PropertyKey("publish_date_custom")
	assert.False(t, ok, "the stored slug binds nothing")
	// the fold still forgives what the NAME's own class covers — which is
	// what keeps most legacy custom spellings resolving without the slug:
	// the old label was derived from this very name
	key, ok := r.PropertyKey("publish_date")
	require.True(t, ok)
	assert.Equal(t, bsonPropKey, key)
}

// ScopedKeyVocabulary — the capability the importer resolves shared names
// through, and diagnoses verbatim terms with.
func TestScopedKeyVocabulary(t *testing.T) {
	t.Run("TypePropertyKeys reads the space's own type row", func(t *testing.T) {
		task := typeRow("type-task", bsonTypeKey, "Task")
		task[bundle.RelationKeyRecommendedRelations] = domain.StringList([]string{"objidA", "objidGone"})
		task[bundle.RelationKeyRecommendedFeaturedRelations] = domain.StringList([]string{"objidB"})
		r := vocabFixture(t, task,
			relationRow("objidA", bsonPropKey, "Projects"),
			relationRow("objidB", bsonTwinKey, "Assignee 2"),
		)

		keys := r.TypePropertyKeys(bsonTypeKey)
		assert.ElementsMatch(t, []string{bsonPropKey, bsonTwinKey}, keys,
			"the four recommended lists, translated id→key; an id the space cannot name is dropped — "+
				"a narrower scope only degrades toward the loud error, never toward a wrong resolution")
	})

	t.Run("an uninstalled bundled type falls back to the shipped table", func(t *testing.T) {
		r := vocabFixture(t)

		keys := r.TypePropertyKeys("task")
		assert.Contains(t, keys, "assignee", "the bundled Task type declares its own properties")
	})

	t.Run("facts: a live stored key, and a glued name", func(t *testing.T) {
		r := vocabFixture(t, relationRow("rel-a", bsonPropKey, "Lists [in work]"))

		assert.True(t, r.PropertyTermFacts(bsonPropKey).LiveStoredKey)
		facts := r.PropertyTermFacts("Lists [in work] (text)")
		assert.False(t, facts.LiveStoredKey)
		assert.Equal(t, "Lists [in work]", facts.ExtendsName,
			"the eval's one real raw-name failure shape: an annotation glued onto a copied name")
		assert.Equal(t, "", r.PropertyTermFacts("Lists [in workshop]").ExtendsName,
			"a longer name sharing a prefix is not glue — the boundary rule")
	})
}

// The §3 label rule inside the space vocabulary, name-era: the rule itself
// is unit-tested in the parent package (label_test.go); what is pinned here
// is the part only this package can get wrong — which stored fact reaches
// it, and what happens when a spelling is unusable.
func TestNameLabels(t *testing.T) {
	t.Run("a non-Latin name is the spelling, verbatim", func(t *testing.T) {
		r := vocabFixture(t,
			named(relationRow("rel-due", "dueDate", ""), "Срок"),
			named(relationRow("rel-custom", bsonPropKey, ""), "Срок"),
		)

		// the space's local copy of a bundled key may carry any name; the
		// code table is the bundled key's authority in every space
		assert.Equal(t, "Due date", r.PropertySlug("dueDate"),
			"a renamed local copy does not move a spelling that ships with every reader")
		assert.Equal(t, "Срок", r.PropertySlug(bsonPropKey))
		key, ok := r.PropertyKey("Срок")
		require.True(t, ok)
		assert.Equal(t, bsonPropKey, key)
	})

	t.Run("a hidden relation spells nothing", func(t *testing.T) {
		r := vocabFixture(t, named(hiddenRelationRow("rel-secret", bsonPropKey, ""), "Secret"))

		assert.Equal(t, bsonPropKey, r.PropertySlug(bsonPropKey))
		_, ok := r.PropertyKey("Secret")
		assert.False(t, ok)
	})

	t.Run("a name the format refuses as a property spelling is never granted", func(t *testing.T) {
		// §2 refuses `id` and `type` as property spellings before any
		// resolution, so a label export would throw away is not built here
		r := vocabFixture(t, named(relationRow("rel-id", bsonPropKey, ""), "id"))

		assert.Equal(t, bsonPropKey, r.PropertySlug(bsonPropKey))
	})
}

// A UI-deleted entity vacates the NAME namespace and still answers to its
// ID. The two are different questions and the listing serves both:
//
//   - "who owns the spelling `Project`?" — a corpse must not, or a type
//     minted under the freed name is shadowed by something no listing shows.
//   - "what does this id point at?" — naming a corpse claims nothing, and
//     the store never removes the type, so the answer exists.
func TestKeyVocab_ADeletedTypeIsNamedByIdButOwnsNoName(t *testing.T) {
	corpse := typeRow("type-corpse", "retired_project", "Project")
	corpse[bundle.RelationKeyIsUninstalled] = domain.Bool(true)

	t.Run("its id resolves to its key", func(t *testing.T) {
		r := vocabFixture(t, corpse)
		key, ok := r.TypeKeyById("type-corpse")
		require.True(t, ok, "the store still holds the type; naming it claims nothing")
		assert.Equal(t, "retired_project", key)
	})

	t.Run("but it is unreachable by spelling", func(t *testing.T) {
		r := vocabFixture(t, corpse)

		key, ok := r.TypeKey("Project")
		assert.Falsef(t, ok && key == "retired_project",
			"the corpse claimed the spelling it vacated (got %q, ok=%v)", key, ok)

		byId, ok := r.TypeKeyById("type-corpse")
		require.True(t, ok)
		assert.Equal(t, "retired_project", byId, "while still being nameable by id")
	})
}

// One entity arriving on TWO rows — the shape the listing does not forbid
// and the rest of this file already guards against with first-wins (keyById,
// idByKey, and relKeyToId one file over; GetRelationByKey answers a
// duplicated relationKey with records[0]). Every SET the vocabulary
// publishes has to survive it, and the candidate list most of all: that list
// is the importer's ambiguity signal — two entries mean two live entities
// and the import stops to ask for a legend — so one entity listed twice
// would refuse a document this very exporter had just written, from a pure
// bookkeeping slip rather than from anything the space actually holds.
func TestOneEntityOnTwoRowsIsStillOneCandidate(t *testing.T) {
	t.Run("a property key carried by two rows is one candidate", func(t *testing.T) {
		// given — a legacy row and the derived one, both live, both carrying
		// the same relationKey and the same name (the relation namespace
		// reads its key off the `relationKey` detail, not off the id, so
		// nothing about the row identity keeps the two apart)
		r := vocabFixture(t,
			relationRow("legacy-row", bsonPropKey, "Manual property"),
			relationRow("rel-"+bsonPropKey, bsonPropKey, "Manual property"),
		)
		want := []string{bsonPropKey}

		// when
		got := r.PropertyKeyCandidates("Manual property")

		// then
		assert.Equal(t, want, got, "one entity, one candidate")
		key, ok := r.PropertyKey("Manual property")
		require.True(t, ok, "one entity is not an ambiguity — the name still resolves")
		assert.Equal(t, bsonPropKey, key)
	})

	t.Run("a type key carried by two rows is one candidate", func(t *testing.T) {
		// given
		r := vocabFixture(t,
			typeRow("legacy-type-row", bsonTypeKey, "Sprint"),
			typeRow("ot-"+bsonTypeKey, bsonTypeKey, "Sprint"),
		)
		want := []string{bsonTypeKey}

		// when
		got := r.TypeKeyCandidates("Sprint")

		// then
		assert.Equal(t, want, got)
		key, ok := r.TypeKey("Sprint")
		require.True(t, ok, "the type namespace has no wider scope to recover in — it must not be lost here")
		assert.Equal(t, bsonTypeKey, key)
	})

	t.Run("a property named by two of the type's four lists is one scope entry", func(t *testing.T) {
		// given — nothing declares the four recommended lists disjoint, and
		// the scope is COUNTED by the importer: a property listed as both
		// featured and ordinary would make its own type unable to single it
		// out, which is the opposite of what the scope exists for
		task := typeRow("type-task", bsonTypeKey, "Task")
		task[bundle.RelationKeyRecommendedRelations] = domain.StringList([]string{"objidA"})
		task[bundle.RelationKeyRecommendedFeaturedRelations] = domain.StringList([]string{"objidA"})
		r := vocabFixture(t, task, relationRow("objidA", bsonPropKey, "Projects"))
		want := []string{bsonPropKey}

		// when
		got := r.TypePropertyKeys(bsonTypeKey)

		// then
		assert.Equal(t, want, got, "the type declares one property, however many of its lists name it")
	})
}
