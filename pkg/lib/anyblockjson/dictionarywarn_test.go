package anyblockjson

// dictionarywarn_test.go — the property dictionary spells a property the way
// every other slot in the format does (§2f).

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
			`"properties":[{"key":"due_date","name":"Due date","format":"date"}]}`)

		require.Len(t, d.Properties, 1)
		assert.EqualValues(t, "dueDate", d.Properties[0].Key,
			"the slug names the bundled property, so the entry defines THAT property")
		assert.Empty(t, warns)
	})

	t.Run("a space-minted key survives verbatim", func(t *testing.T) {
		const bson = "6a32d4856761631534b22f85"
		d, warns := readDict(t, `{`+dictHead+
			`"properties":[{"key":"`+bson+`","name":"Aroma notes","format":"text"}]}`)

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

// The whole design rests on one fact: a bundled property's slug names it
// UNIQUELY. If two bundled keys ever slugged or folded together, a dictionary
// spelling either of them would be undecidable, and the format would have to
// go back to storing camelCase.
//
// This is that guard. It is not a test of today's table — it is the condition
// under which a NEW bundled relation may be added: if adding one breaks this,
// the relation needs a different key, not a looser dictionary.
//
// How this can fail: add `dueDate` beside an existing `due_date`, or
// `gitHubStars` beside `githubStars` — both fold to one spelling, and this
// fails naming the pair.
func TestDictionaryKeys_TheBundledTableStaysUnambiguous(t *testing.T) {
	keys := bundledRelationKeys()
	require.NotEmpty(t, keys)

	for spelling, owners := range keysBySpelling(keys, bundle.ApiSlug) {
		assert.Lenf(t, owners, 1,
			"bundled properties %v all spell as %q — a dictionary naming it could mean any of them",
			owners, spelling)
	}
	for fold, owners := range keysBySpelling(keys, func(k string) string {
		return bundle.FoldApiKey(bundle.ApiSlug(k))
	}) {
		assert.Lenf(t, owners, 1,
			"bundled properties %v all FOLD to %q — a near-miss spelling could not be recovered",
			owners, fold)
	}

	// and every slug names its own key back, through the very lookup the
	// dictionary reader uses
	for _, key := range keys {
		slug := bundle.ApiSlug(key)
		stored, ambiguous := dictionaryStoredKey(slug)
		assert.Emptyf(t, ambiguous, "slug %q of %q is ambiguous", slug, key)
		assert.Equalf(t, key, stored, "slug %q must name %q back", slug, key)
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
