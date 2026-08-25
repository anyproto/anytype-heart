package anyblockjson

// dictionarywarn_test.go — the property dictionary spells a property the way
// every other slot in the format does (§2f).

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"

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

// The dictionary was the last place in the format that spelled a property in
// camelCase: within ONE real exported bundle, an object document said
// `created_date` while the dictionary beside it said `createdDate`. It now
// says `created_date` too, so the format has one spelling for one concept.
//
// How this can fail: emit the stored key here and the dictionary becomes the
// odd file again; slug it on the way out without inverting it on the way in
// and `installed` names nothing the bundled table has.
func TestDictionary_SpellsPropertiesTheWayDocumentsDo(t *testing.T) {
	d, warns := readDict(t, `{`+dictHead+`"installed":["due_date","created_date"]}`)

	assert.Equal(t, []string{"dueDate", "createdDate"}, d.Installed,
		"read as the stored keys they name — the wire spelling is the slug, the codec keeps stored keys")
	assert.Empty(t, warns, "the canonical spelling must be silent")

	out, err := MarshalPropertyDictionary(d)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"due_date"`)
	assert.NotContains(t, string(out), `"dueDate"`, "camelCase must not survive a round trip")
}

// A stored key still names itself — an exact match wins before folding is
// consulted, which is the ladder every slot in the format follows. A bundle
// written before the dictionary spoke slugs therefore keeps reading, and
// re-rendering normalizes it.
func TestDictionary_TheStoredSpellingStillNamesItsProperty(t *testing.T) {
	d, warns := readDict(t, `{`+dictHead+`"installed":["dueDate"]}`)

	assert.Equal(t, []string{"dueDate"}, d.Installed)
	assert.Empty(t, warns)

	out, err := MarshalPropertyDictionary(d)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"due_date"`, "re-rendering settles on the canonical spelling")
}

// An entry is how a bundle declares a property, and its key population is
// MIXED: a bundled key gets the slug, a space-minted one is a bson id and
// must survive verbatim. `bundle.ApiSlug` is `strcase.ToSnake`, which turns
// `6a32d4856761631534b22f85` into `6_a_32_d_4856761631534_b_22_f_85` — 515 of
// 6,426 entry keys in a 77-space export are bson ids, so slugging entry keys
// unconditionally would corrupt one in twelve.
func TestDictionary_AnEntryKeyIsSluggedOnlyWhenItIsBundled(t *testing.T) {
	t.Run("a bundled key travels as its slug", func(t *testing.T) {
		d, warns := readDict(t, `{`+dictHead+
			`"properties":[{"property":"due_date","name":"Due date","format":"date"}]}`)

		require.Len(t, d.Properties, 1)
		assert.EqualValues(t, "dueDate", d.Properties[0].Key,
			"the slug names the bundled property, so the entry defines THAT property")
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
		assert.Contains(t, string(out), `"`+bson+`"`)
		assert.NotContains(t, string(out), "6_a_32", "ToSnake must not reach a bson id")
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
	doc := `{` + dictHead + `"installed":["due_date"]}`

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
// This is that guard, and since v0.38 the spelling under guard is the
// ALIAS-AWARE one (alias.go): the sixteen relation-spelled keys write their
// property aliases, everything else its derived slug. It is not a test of
// today's table — it is the condition under which a NEW bundled relation OR
// a new alias may be added: if adding one breaks this, the key (or the
// alias) needs a different name, not a looser dictionary.
//
// How this can fail: add `dueDate` beside an existing `due_date`, or
// `gitHubStars` beside `githubStars` — both fold to one spelling; or alias a
// key onto a spelling some other key already derives — and this fails naming
// the pair.
func TestDictionaryKeys_TheBundledTableStaysUnambiguous(t *testing.T) {
	keys := bundledRelationKeys()
	require.NotEmpty(t, keys)

	for spelling, owners := range keysBySpelling(keys, dictionaryKeySpelling) {
		assert.Lenf(t, owners, 1,
			"bundled properties %v all spell as %q — a dictionary naming it could mean any of them",
			owners, spelling)
	}
	for fold, owners := range keysBySpelling(keys, func(k string) string {
		return bundle.FoldApiKey(dictionaryKeySpelling(k))
	}) {
		assert.Lenf(t, owners, 1,
			"bundled properties %v all FOLD to %q — a near-miss spelling could not be recovered",
			owners, fold)
	}

	// and every spelling names its own key back, through the very lookup the
	// dictionary reader uses — aliased keys included (StoredX(SpellingX(k))
	// == k is the round-trip half of the guard)
	for _, key := range keys {
		spelling := dictionaryKeySpelling(key)
		stored, ambiguous := dictionaryStoredKey(spelling)
		assert.Emptyf(t, ambiguous, "spelling %q of %q is ambiguous", spelling, key)
		assert.Equalf(t, key, stored, "spelling %q must name %q back", spelling, key)
	}

	// the same round trip through the vocabulary door the codec uses — the
	// two readers must agree on every bundled key
	for _, key := range keys {
		spelling := (BundledKeyVocabulary{}).PropertySlug(key)
		back, ok := (BundledKeyVocabulary{}).PropertyKey(spelling)
		require.Truef(t, ok, "the vocabulary must invert its own spelling %q", spelling)
		assert.Equalf(t, key, back, "vocabulary spelling %q must name %q back", spelling, key)
	}

	// A guard that cannot fail is not a guard: this is the collision the real
	// table must never contain, and the detector must see it.
	t.Run("the detector sees a planted collision", func(t *testing.T) {
		planted := keysBySpelling([]string{"dueDate", "due_date", "tag"}, bundle.ApiSlug)
		require.Len(t, planted["due_date"], 2,
			"two keys spelling alike must land in one bucket")
		assert.Len(t, planted["tag"], 1)

		folded := keysBySpelling([]string{"gitHubStars", "githubStars"}, func(k string) string {
			return bundle.FoldApiKey(bundle.ApiSlug(k))
		})
		require.Len(t, folded["githubstars"], 2,
			"two keys folding alike must land in one bucket")

		// and an ALIAS-shaped collision: an alias that lands on a spelling
		// another key derives must land in that key's bucket, or the alias
		// loops above are grouping by a function that cannot see one
		hostileAlias := func(k string) string {
			if k == "featuredRelations" {
				return "due_date" // a hypothetical alias squatting a derived slug
			}
			return dictionaryKeySpelling(k)
		}
		aliased := keysBySpelling([]string{"featuredRelations", "dueDate"}, hostileAlias)
		require.Len(t, aliased["due_date"], 2,
			"an alias and the slug it squats must land in one bucket")
	})
}

// The alias table itself carries three obligations of its own (alias.go),
// each with a failure the spelling loops above cannot name:
//
//   - an alias must belong to a BUNDLED key — aliasing a key the table does
//     not hold aliases nothing, and MarshalPropertyDictionary would refuse
//     the key anyway;
//   - an alias must not BE a bundled stored key — a stored key resolves
//     verbatim before any alias (§3 chain step 2), so such an alias could
//     never name its own key;
//   - aliases must be pairwise distinct, spelling and fold alike — two keys
//     behind one alias is the two-answers disease with no derived slug to
//     blame.
func TestDictionaryKeys_TheAliasTableStaysUnambiguous(t *testing.T) {
	type table struct {
		aliases  map[string]string
		isStored func(string) bool
		what     string
	}
	for _, tb := range []table{
		{propertyKeyAliases, func(k string) bool { return bundle.HasRelation(domain.RelationKey(k)) }, "property"},
		{typeKeyAliases, func(k string) bool { return bundle.HasObjectTypeByKey(domain.TypeKey(k)) }, "type"},
	} {
		t.Run(tb.what, func(t *testing.T) {
			require.NotEmpty(t, tb.aliases)
			seen := map[string]string{}
			seenFold := map[string]string{}
			for key, alias := range tb.aliases {
				assert.Truef(t, tb.isStored(key),
					"alias %q is declared for %q, which is not a bundled %s key", alias, key, tb.what)
				assert.Falsef(t, tb.isStored(alias),
					"alias %q IS a bundled stored key: verbatim-first would answer before the alias ever could", alias)
				if first, dup := seen[alias]; dup {
					t.Errorf("alias %q is declared for both %q and %q", alias, first, key)
				}
				seen[alias] = key
				fold := bundle.FoldApiKey(alias)
				if first, dup := seenFold[fold]; dup {
					t.Errorf("aliases of %q and %q fold together (%q)", first, key, fold)
				}
				seenFold[fold] = key
			}
		})
	}
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
// here would be undecidable in two places at once. Alias-aware since v0.38,
// like the property guard above: `relation` spells `property`,
// `relationOption` spells `property_option` (alias.go).
//
// How this can fail: add a bundled type whose key spells onto an existing
// one's — this fails naming the pair, and the new type needs a different key.
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
	for fold, owners := range keysBySpelling(keys, func(k string) string {
		return bundle.FoldApiKey(TypeKeySpelling(k))
	}) {
		assert.Lenf(t, owners, 1, "bundled types %v all FOLD to %q", owners, fold)
	}
	// StoredTypeKey(TypeKeySpelling(k)) == k — for every key, the aliased
	// two included
	for _, key := range keys {
		assert.Equalf(t, key, StoredTypeKey(TypeKeySpelling(key)),
			"the spelling of type %q must name it back", key)
	}
	// and through the vocabulary door the codec uses
	for _, key := range keys {
		spelling := (BundledKeyVocabulary{}).TypeSlug(key)
		back, ok := (BundledKeyVocabulary{}).TypeKey(spelling)
		require.Truef(t, ok, "the vocabulary must invert its own spelling %q", spelling)
		assert.Equalf(t, key, back, "vocabulary spelling %q must name type %q back", spelling, key)
	}
}

// One spelling for one concept, across the two slots that name a type outside
// a document: a dictionary entry's target types and the bundle manifest.
//
// A type DOCUMENT reaches the same answer by a different road — it slugs
// through the exporter's per-document ledger and binds the term in that
// document's own `type_internal_keys` legend. The dictionary and the manifest have no
// legend, so their spelling has to be a pure function of the key.
func TestDictionary_TargetTypesSpellLikeEverythingElse(t *testing.T) {
	d, warns := readDict(t, `{`+dictHead+
		`"properties":[{"property":"assignee","name":"Assignee","format":"objects",`+
		`"object_types":["participant","object_type","6a83296f61fab2265263ae34"]}]}`)
	require.Empty(t, warns)
	require.Len(t, d.Properties, 1)

	assert.Equal(t, []string{"participant", "objectType", "6a83296f61fab2265263ae34"},
		d.Properties[0].ObjectTypes,
		"slugs resolve to stored type keys; a space-minted key stays verbatim")

	out, err := MarshalPropertyDictionary(d)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"object_type"`, "written back in the format's spelling")
	assert.NotContains(t, string(out), `"objectType"`)
	assert.Contains(t, string(out), `"6a83296f61fab2265263ae34"`, "and a minted key is never slugged")
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
			`"properties":[{"property":"due_date","internal_key":"6a32d4856761631534b22f85",`+
			`"name":"Due date","format":"date"}]}`)

		require.Len(t, warns, 1)
		assert.Contains(t, warns[0].Message, "name different properties")
		assert.EqualValues(t, "dueDate", d.Properties[0].Key,
			"the spelling wins, as authoredKey has always done")
	})

	t.Run("they agree — the pair export writes", func(t *testing.T) {
		_, warns := readDict(t, `{`+dictHead+
			`"properties":[{"property":"due_date","internal_key":"dueDate",`+
			`"name":"Due date","format":"date"}]}`)
		assert.Empty(t, warns, "the agreeing pair is the normal case and must be silent")
	})

	t.Run("only one stated", func(t *testing.T) {
		for _, entry := range []string{
			`{"property":"due_date","format":"date"}`,
			`{"internal_key":"6a32d4856761631534b22f85","format":"date"}`,
		} {
			_, warns := readDict(t, `{`+dictHead+`"properties":[`+entry+`]}`)
			assert.Empty(t, warns, entry)
		}
	})
}
