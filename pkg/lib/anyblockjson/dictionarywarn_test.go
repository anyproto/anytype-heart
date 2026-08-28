package anyblockjson

// dictionarywarn_test.go — the property dictionary spells a property the way
// every other slot in the format does (§2f): the display name for a bundled
// key, the stored key verbatim for a space-minted one — and the bundled name
// table it leans on stays unambiguous.

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

const dictHead = `"$schema":"https://schemas.anytype.io/anyblock/1/properties.schema.json","version":1,`

func readDict(t *testing.T, doc string) (*PropertyDictionary, []Issue) {
	t.Helper()
	var warns []Issue
	d, err := UnmarshalPropertyDictionaryWarn([]byte(doc), func(i Issue) { warns = append(warns, i) })
	require.NoError(t, err)
	return d, warns
}

// The dictionary spells a bundled property the way every document slot does:
// by its display name. One spelling for one concept — an object document
// says "Due date" in its `properties` map and the dictionary beside it says
// "Due date" in `installed`.
//
// How this can fail: emit the stored key here and the dictionary becomes the
// odd file out; spell the name on the way out without inverting it on the
// way in and `installed` names nothing the bundled table has.
func TestDictionary_SpellsPropertiesTheWayDocumentsDo(t *testing.T) {
	d, warns := readDict(t, `{`+dictHead+`"installed":["Due date","Creation date"]}`)

	assert.Equal(t, []string{"dueDate", "createdDate"}, d.Installed,
		"read as the stored keys they name — the wire spelling is the name, the codec keeps stored keys")
	assert.Empty(t, warns, "the canonical spelling must be silent")

	out, err := MarshalPropertyDictionary(d)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"Due date"`)
	assert.NotContains(t, string(out), `"dueDate"`, "the stored key must not survive a round trip")
}

// A stored key still names itself — an exact match wins before folding is
// consulted, which is the ladder every slot in the format follows. And the
// pre-change derived slug (`due_date`) still resolves through the fold
// layer with no compatibility table: ToSnake only inserts `_` and
// lowercases, so the old slug sits in its stored key's fold class. Both
// re-render to the canonical name.
func TestDictionary_LegacySpellingsStillNameTheirProperty(t *testing.T) {
	for _, legacy := range []string{"dueDate", "due_date"} {
		t.Run(legacy, func(t *testing.T) {
			d, warns := readDict(t, `{`+dictHead+`"installed":["`+legacy+`"]}`)

			assert.Equal(t, []string{"dueDate"}, d.Installed)
			assert.Empty(t, warns)

			out, err := MarshalPropertyDictionary(d)
			require.NoError(t, err)
			assert.Contains(t, string(out), `"Due date"`, "re-rendering settles on the canonical spelling")
		})
	}
}

// An entry is how a bundle declares a property, and its key population is
// MIXED: a bundled key gets its display name, a space-minted one is a bson
// id and must survive verbatim — the dictionary has no legend, so its
// spelling must be a pure function of the key, and the only pure spelling a
// space-minted key has is itself.
func TestDictionary_AnEntryKeyIsNamedOnlyWhenItIsBundled(t *testing.T) {
	t.Run("a bundled key travels as its name", func(t *testing.T) {
		d, warns := readDict(t, `{`+dictHead+
			`"properties":[{"property":"Due date","name":"Due date","format":"date"}]}`)

		require.Len(t, d.Properties, 1)
		assert.EqualValues(t, "dueDate", d.Properties[0].Key,
			"the name names the bundled property, so the entry defines THAT property")
		assert.Empty(t, warns)
	})

	t.Run("a space-minted key survives verbatim", func(t *testing.T) {
		const bson = "6a32d4856761631534b22f85"
		d, warns := readDict(t, `{`+dictHead+
			`"properties":[{"property":"`+bson+`","name":"Aroma notes","format":"text"}]}`)

		require.Len(t, d.Properties, 1)
		assert.EqualValues(t, bson, d.Properties[0].Key)
		assert.Empty(t, warns)

		out, err := MarshalPropertyDictionary(d)
		require.NoError(t, err)
		assert.Contains(t, string(out), `"`+bson+`"`, "nothing is ever derived from a bson id")
	})
}

// `installed` names rows to restore from the bundled table, so a key outside
// it tells a reader to install nothing. It is TOLERATED rather than refused,
// and the tolerance is about VERSION SKEW, not custom properties: the bundled
// table grows independently of the format version, so a backup written by a
// newer app lists keys this build has never heard of, and refusing them would
// make every backup unreadable one app version back.
//
// How this can fail: turn the warning into an error and a newer app's backup
// stops reading; drop the warning and a bundle that installs nothing for a
// property ships with a clean bill of health.
func TestDictionary_AKeyFromANewerAppIsToleratedAndReported(t *testing.T) {
	d, warns := readDict(t, `{`+dictHead+`"installed":["some_key_this_build_has_never_heard_of"]}`)

	assert.Equal(t, []string{"some_key_this_build_has_never_heard_of"}, d.Installed,
		"kept verbatim — it may be the newer app's")
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0].Message, "installs NOTHING for it")
	assert.Contains(t, warns[0].Message, "newer app", "the tolerance is explained, not just the fault")
}

// UnmarshalPropertyDictionary is UnmarshalPropertyDictionaryWarn with no
// sink: the same verdicts, the warnings discarded — the relationship Validate
// and ValidateWarn have.
func TestDictionary_TheSinklessDoorAgrees(t *testing.T) {
	doc := `{` + dictHead + `"installed":["Due date"]}`

	quiet, err := UnmarshalPropertyDictionary([]byte(doc))
	require.NoError(t, err)
	loud, _ := readDict(t, doc)
	assert.Equal(t, loud, quiet)
}

// The whole design rests on one fact: a bundled property's wire spelling
// names it UNIQUELY. If two bundled keys ever spelled or folded together, a
// dictionary spelling either of them would be undecidable, and the format
// would have to go back to storing camelCase.
//
// The spelling under guard is the NAME-AWARE one (bundledname.go): a key
// whose display name uniquely inverts spells the name, everything else its
// stored key. This is not a test of today's table — it is the condition
// under which a NEW bundled relation may be added: if adding one breaks
// this, the name needs to change (the audioGenre "Genre" → "Audio genre"
// rename is exactly that), not the dictionary loosened.
//
// How this can fail: name a new bundled relation "Due date", or "Due-Date"
// (one fold class with the existing name) — this fails naming the pair.
func TestDictionaryKeys_TheBundledTableStaysUnambiguous(t *testing.T) {
	keys := bundledRelationKeys()
	require.NotEmpty(t, keys)

	for spelling, owners := range keysBySpelling(keys, dictionaryKeySpelling) {
		assert.Lenf(t, owners, 1,
			"bundled properties %v all spell as %q — a dictionary naming it could mean any of them",
			owners, spelling)
	}
	for fold, owners := range keysBySpelling(keys, func(k string) string {
		return FoldKeyTerm(dictionaryKeySpelling(k))
	}) {
		assert.Lenf(t, owners, 1,
			"bundled properties %v all FOLD to %q — a near-miss spelling could not be recovered",
			owners, fold)
	}

	// and every spelling names its own key back, through the very lookup the
	// dictionary reader uses (StoredX(SpellingX(k)) == k is the round-trip
	// half of the guard)
	for _, key := range keys {
		spelling := dictionaryKeySpelling(key)
		stored, ambiguous := dictionaryStoredKey(spelling)
		assert.Emptyf(t, ambiguous, "spelling %q of %q is ambiguous", spelling, key)
		assert.Equalf(t, key, stored, "spelling %q must name %q back", spelling, key)
	}

	// the same round trip through the vocabulary door the codec uses — the
	// two readers must agree on every bundled key. A key that spells ITSELF
	// (its name is shared, unwritable or absent) resolves verbatim — the
	// vocabulary answers "not a slug" and the chain treats the term as the
	// stored key it is.
	for _, key := range keys {
		spelling := (BundledKeyVocabulary{}).PropertySlug(key)
		back, ok := (BundledKeyVocabulary{}).PropertyKey(spelling)
		if spelling == key {
			assert.Equalf(t, key, back, "verbatim spelling %q must pass through", spelling)
			continue
		}
		require.Truef(t, ok, "the vocabulary must invert its own spelling %q", spelling)
		assert.Equalf(t, key, back, "vocabulary spelling %q must name %q back", spelling, key)
	}

	// A guard that cannot fail is not a guard: this is the collision the real
	// table must never contain, and the detector must see it.
	t.Run("the detector sees a planted collision", func(t *testing.T) {
		sameName := func(string) string { return "Genre" }
		planted := keysBySpelling([]string{"genre", "audioGenre"}, sameName)
		require.Len(t, planted["Genre"], 2,
			"two keys spelling alike must land in one bucket")

		folded := keysBySpelling([]string{"gitHubStars", "githubStars"}, func(k string) string {
			return FoldKeyTerm(k)
		})
		require.Len(t, folded["githubstars"], 2,
			"two keys folding alike must land in one bucket")
	})
}

// keysBySpelling groups keys by the spelling a reader would see.
func keysBySpelling(keys []string, spell func(string) string) map[string][]string {
	out := map[string][]string{}
	for _, key := range keys {
		s := spell(key)
		out[s] = append(out[s], key)
	}
	return out
}

// The same guard for TYPES. A bundled type's spelling is what the manifest
// keys on and what a dictionary entry's `object_types` names, so an ambiguity
// here would be undecidable in two places at once.
//
// How this can fail: add a bundled type whose NAME spells onto an existing
// one's — this fails naming the pair, and the new type needs a different
// name (the space "Space" → "Space settings" rename is exactly that).
func TestDictionaryKeys_TheBundledTypeTableStaysUnambiguous(t *testing.T) {
	keys := make([]string, 0, 32)
	for _, tk := range bundle.ListTypesKeys() {
		keys = append(keys, tk.String())
	}
	require.NotEmpty(t, keys)

	for spelling, owners := range keysBySpelling(keys, TypeKeySpelling) {
		assert.Lenf(t, owners, 1,
			"bundled types %v all spell as %q — a manifest key or an object_types member "+
				"naming it could mean any of them", owners, spelling)
	}
	// the SPELLING folds stay unique. The full fold TABLE holds one known
	// collision beyond them — the name "Space" (spaceView) folds onto the
	// stored key `space` — which only degrades the forgiving layer for
	// near-misses of that one pair, both measured at 0 documents; the
	// spellings themselves stay exact and unambiguous, which is what this
	// guard protects.
	for fold, owners := range keysBySpelling(keys, func(k string) string {
		return FoldKeyTerm(TypeKeySpelling(k))
	}) {
		assert.Lenf(t, owners, 1, "bundled types %v all FOLD to %q", owners, fold)
	}
	// StoredTypeKey(TypeKeySpelling(k)) == k — for every key
	for _, key := range keys {
		assert.Equalf(t, key, StoredTypeKey(TypeKeySpelling(key)),
			"the spelling of type %q must name it back", key)
	}
	// and through the vocabulary door the codec uses
	for _, key := range keys {
		spelling := (BundledKeyVocabulary{}).TypeSlug(key)
		back, ok := (BundledKeyVocabulary{}).TypeKey(spelling)
		if spelling == key {
			assert.Equalf(t, key, back, "verbatim spelling %q must pass through", spelling)
			continue
		}
		require.Truef(t, ok, "the vocabulary must invert its own spelling %q", spelling)
		assert.Equalf(t, key, back, "vocabulary spelling %q must name type %q back", spelling, key)
	}
}

// One spelling for one concept, across the two slots that name a type outside
// a document: a dictionary entry's target types and the bundle manifest.
//
// A type DOCUMENT reaches the same answer by a different road — it spells
// through the exporter's per-document ledger and binds the term in that
// document's own `type_internal_keys` legend. The dictionary and the manifest
// have no legend, so their spelling has to be a pure function of the key.
func TestDictionary_TargetTypesSpellLikeEverythingElse(t *testing.T) {
	d, warns := readDict(t, `{`+dictHead+
		`"properties":[{"property":"Assignee","name":"Assignee","format":"objects",`+
		`"object_types":["Space member","object_type","6a83296f61fab2265263ae34"]}]}`)
	require.Empty(t, warns)
	require.Len(t, d.Properties, 1)

	assert.Equal(t, []string{"participant", "objectType", "6a83296f61fab2265263ae34"},
		d.Properties[0].ObjectTypes,
		"a name resolves to its stored type key, a legacy slug through the fold, a minted key stays verbatim")

	out, err := MarshalPropertyDictionary(d)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"Space member"`, "written back in the format's spelling")
	assert.Contains(t, string(out), `"Type"`, "objectType's name")
	assert.NotContains(t, string(out), `"objectType"`)
	assert.Contains(t, string(out), `"6a83296f61fab2265263ae34"`, "and a minted key is never renamed")
}

// bundledRelationKeys lists every relation key this build's bundled table
// holds, read from the table itself rather than a hand-kept list.
func bundledRelationKeys() []string {
	urls := bundle.ListRelationsUrls()
	out := make([]string, 0, len(urls))
	for _, url := range urls {
		key := url
		if i := strings.LastIndex(url, "/"); i >= 0 {
			key = url[i+1:]
		}
		out = append(out, strings.TrimPrefix(key, "_br"))
	}
	return out
}

// An entry may state a `property` spelling and an `internal_key`, and export
// writes both from ONE stored key — so in anything this package produced they
// agree and the precedence never matters. An AUTHOR can make them disagree,
// and then neither order is safe: the spelling is what the document's own
// values resolve through, while the internal_key is the exact key the entry
// claims to define. Honouring either silently leaves the other pointing
// somewhere else.
//
// So the disagreement is reported. The spelling still wins — the code's
// comment used to say the opposite of what authoredKey does, which is the
// contradiction this pins shut.
//
// How this can fail: let the pair disagree in silence and a type's recommended
// list points at a property no value in the document uses.
func TestDictionary_ADisagreeingIdentityPairIsReported(t *testing.T) {
	t.Run("they disagree", func(t *testing.T) {
		d, warns := readDict(t, `{`+dictHead+
			`"properties":[{"property":"Due date","internal_key":"6a32d4856761631534b22f85",`+
			`"name":"Due date","format":"date"}]}`)

		require.Len(t, warns, 1)
		assert.Contains(t, warns[0].Message, "name different properties")
		assert.EqualValues(t, "dueDate", d.Properties[0].Key,
			"the spelling wins, as authoredKey has always done")
	})

	t.Run("they agree — the pair export writes", func(t *testing.T) {
		_, warns := readDict(t, `{`+dictHead+
			`"properties":[{"property":"Due date","internal_key":"dueDate",`+
			`"name":"Due date","format":"date"}]}`)
		assert.Empty(t, warns, "the agreeing pair is the normal case and must be silent")
	})

	t.Run("only one stated", func(t *testing.T) {
		for _, entry := range []string{
			`{"property":"Due date","format":"date"}`,
			`{"internal_key":"6a32d4856761631534b22f85","format":"date"}`,
		} {
			_, warns := readDict(t, `{`+dictHead+`"properties":[`+entry+`]}`)
			assert.Empty(t, warns, entry)
		}
	})
}

// An inline option says what the option MEANS — its name, its colour, and by
// its position where it sits. Its stored key says what the option IS, and
// that is the one thing about it derivable from nothing.
//
// Everything else about an option can be reconstructed. The api key is
// regenerated from the name by the app's own rule at creation: measured over
// a 77-space export, all 514 real option api keys are reproduced by it — 470
// by the api slug and 44 by the transliterate fallback, for names like `$$`
// that slug to nothing. Not one survived a rename, so none needs to travel.
// The order is the array position (§2f). The property is the entry holding it.
//
// So `internal_key` is what an inline vocabulary was missing to be complete
// rather than merely descriptive.
//
// How this can fail: render it on an option that has none and the compact
// bare-name form disappears for every colourless option; drop it from the
// object form and a vocabulary can never state identity.
func TestDictionary_AnOptionCarriesItsStoredKey(t *testing.T) {
	d, warns := readDict(t, `{`+dictHead+
		`"properties":[{"property":"Status","name":"Status","format":"select","options":[`+
		`{"name":"To Do","color":"ice","internal_key":"63454ad0c493f68e301890db"},`+
		`{"name":"Done","color":"lime"},`+
		`"Someday"]}]}`)
	require.Empty(t, warns)
	require.Len(t, d.Properties, 1)

	opts := d.Properties[0].Options
	require.Len(t, opts, 3)
	assert.Equal(t, "63454ad0c493f68e301890db", opts[0].InternalKey)
	assert.Empty(t, opts[1].InternalKey, "an option may carry none")
	assert.Equal(t, "Someday", opts[2].Name, "and the bare-name form still means a colourless option")

	out, err := MarshalPropertyDictionary(d)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"internal_key": "63454ad0c493f68e301890db"`)
	assert.Contains(t, string(out), `"name": "To Do"`, "names with spaces survive intact")
	assert.Contains(t, string(out), `"Someday"`,
		"an option with neither colour nor key stays the compact bare name")
}
