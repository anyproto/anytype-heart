package anyblockjson

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

// The label rule leaves bundled keys alone — their spelling is the derived
// api slug, in every space and offline (§3). That is only safe if the api
// slug is already a key this format can write everywhere, and nothing in
// `pkg/lib/bundle` knows about the §6.2.1 grammar: `ApiSlug` is
// `strcase.ToSnake` and no more, so a bundled key added later as `50Percent`
// or `all` would produce a slug no filter string can name — silently, since
// the label rule would pass it straight through.
//
// It holds today for all 223 (194 relations, 29 types), and this asserts it
// rather than assuming it, at the moment such a key would be added.
func TestBundledSlugsAreKeysTheFilterGrammarAccepts(t *testing.T) {
	var keys []string
	for _, u := range bundle.ListRelationsUrls() {
		keys = append(keys, strings.TrimPrefix(u, addr.BundledRelationURLPrefix))
	}
	for _, k := range bundle.ListTypesKeys() {
		keys = append(keys, string(k))
	}
	require.NotEmpty(t, keys)
	for _, key := range keys {
		slug := bundle.ApiSlug(key)
		_, err := filterstring.Parse(slug+` = "x"`, filterstring.Options{})
		require.NoErrorf(t, err, "bundled key %q spells %q, which is not a key this format can write", key, slug)
	}
}

// The label rule (§3): what a document spells for a key the bundled table
// does not speak for. Every case here is a decision with a plausible
// alternative, and the alternative is named in the case's own comment — a
// table that only pinned the happy path would pass with the transliterating
// normalizer this change replaced.
func TestNormalizeKeyLabel(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		why  string
	}{
		{"Publish Date", "publish_date", "the api slug's own transform, so an ASCII name lands where GO-7458's backfill would put it"},
		{"Original creation date", "original_creation_date", ""},
		{"iconEmoji", "icon_emoji", "humps split, exactly as ApiSlug does — `iconemoji` would be the no-transform answer"},
		{"mediaArtistURL", "media_artist_url", "the acronym case ApiSlug is pinned on"},
		{"  spaced  name ", "spaced_name", "separator runs collapse and edges trim"},
		{"Cost & type", "cost_type", ""},
		{"What's missing", "what_s_missing", ""},

		// the whole point of the change: non-Latin scripts are ALREADY legal
		// in the §6.2.1 grammar, so they are kept, not transliterated.
		// ApiSlugFromName answers `toggl`, `tieng_viet` and
		// `ri_ben_yu_nopuropatei` here — unguessable and unreadable at once.
		{"Тоггл", "тоггл", "no transliteration"},
		{"日本語のプロパティ", "日本語のプロパティ", "no transliteration"},
		{"tiếng Việt", "tiếng_việt", "no transliteration"},
		{"العربية", "العربية", "no transliteration"},

		// emoji, punctuation and symbols are separators, not letters
		{"Priority 📌", "priority", ""},
		{"Email 📧 ", "email", ""},
		{"C#", "c", ""},
		{"@home", "home", ""},
		{"tag/tag", "tag_tag", ""},

		// nothing left to name
		{"#", "", "the four production relations that store `#` as their api slug"},
		{"🎉", "", ""},
		{"", "", ""},

		// the two ways this construction can fail the grammar, and the one
		// escape for both. Dropping the leading digits instead would turn
		// `50% done` into `done`, which is a DIFFERENT (bundled) property.
		{"50% done", "_50_done", "identStart is a letter or `_`, never a digit"},
		{"1221312425", "_1221312425", ""},
		{"All", "_all", "`all` is a reserved word of the filter grammar"},
		{"NOT", "_not", ""},
		{"in", "_in", ""},

		// a combining mark modifies the letter before it — as a separator it
		// would cut the word at every virama (`क_ष_त_र_य`)
		{"क्षत्रिय", "कषतरय", "marks drop, they do not separate"},
		{"İstanbul", "istanbul", "lowercasing İ leaves a combining dot behind"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeKeyLabel(tc.in), tc.why)
		})
	}

	// Pitfall stated in §3: two visually identical names can be different
	// byte sequences, and a reader matches a label byte-for-byte. Export is
	// safe by construction (it writes the same string as label and as legend
	// key), but a hand-edited document in the other form must still land on
	// the same label. This fails the moment the NFC pass is dropped.
	t.Run("NFD and NFC forms of one name normalize to one label", func(t *testing.T) {
		nfc := "Ünïcødé"
		nfd := norm.NFD.String(nfc)
		assert.NotEqual(t, nfc, nfd, "the fixture has to actually differ in bytes")
		assert.Equal(t, normalizeKeyLabel(nfc), normalizeKeyLabel(nfd))
		assert.Equal(t, "ünïcødé", normalizeKeyLabel(nfd))
	})

}

// The ladder itself — which of the two stored facts the label comes from.
// Each case fails loudly if the branches are reordered: swapping slug and
// name would spell `manual_property` as `manual_property_renamed`, and
// dropping the `slug == key` arm would spell a bson-slugged relation
// `_6a7663db61fab21cd4b9e101`, one character longer than the key it already
// had.
func TestPropertyLabel(t *testing.T) {
	const bson = "6a7663db61fab21cd4b9e101"

	t.Run("a conforming stored slug is the label, verbatim", func(t *testing.T) {
		assert.Equal(t, "manual_property", PropertyLabel(bson, "manual_property", "Manual property renamed"))
	})

	t.Run("a stored slug the grammar refuses is normalized", func(t *testing.T) {
		assert.Equal(t, "_50_done", PropertyLabel(bson, "50_done", "50% done"))
	})

	t.Run("no usable slug falls to the display name", func(t *testing.T) {
		assert.Equal(t, "publish_date", PropertyLabel(bson, "", "Publish Date"))
	})

	t.Run("a slug that merely repeats the stored key is not a slug", func(t *testing.T) {
		// production rows store the bson id as their own apiObjectKey; the
		// name is the only thing left that can name them
		assert.Equal(t, "recipe", PropertyLabel(bson, bson, "Recipe"))
	})

	t.Run("nothing derivable leaves the stored key as its own label", func(t *testing.T) {
		assert.Equal(t, "", PropertyLabel(bson, "#", "#"))
		assert.Equal(t, "", PropertyLabel(bson, "", ""))
	})

	t.Run("a label equal to the stored key is not a label", func(t *testing.T) {
		// nothing is gained and the legend rule would owe an identity entry
		// either way
		assert.Equal(t, "", PropertyLabel("website", "", "Website"))
	})

	// the schema's propertyNames bound (§3), which the label rule enforces
	// rather than the exporter's after-the-fact warning. Truncating would
	// invent a spelling nobody chose and could collide two long names onto
	// one label; the stored key is right there.
	t.Run("a label past the key bound is refused, never truncated", func(t *testing.T) {
		assert.Equal(t, "", PropertyLabel(bson, "", strings.Repeat("a", maxPropertyKeyLen+1)))
		assert.Equal(t, "", PropertyLabel(bson, strings.Repeat("a", maxPropertyKeyLen+1), ""))
		assert.Len(t, PropertyLabel(bson, "", strings.Repeat("a", maxPropertyKeyLen)), maxPropertyKeyLen)
	})

	// §2 refuses `id` and `type` as property SPELLINGS before any
	// resolution, so minting one produces a label export throws away with a
	// warning. The type namespace has no such reservation — its home
	// surface is a value, not a member name.
	t.Run("the two reserved property spellings are never minted", func(t *testing.T) {
		assert.Equal(t, "", PropertyLabel(bson, "", "id"))
		assert.Equal(t, "", PropertyLabel(bson, "", "Type"))
		assert.Equal(t, "id", TypeLabel(bson, "", "id"))
		assert.Equal(t, "type", TypeLabel(bson, "", "Type"))
	})
}
