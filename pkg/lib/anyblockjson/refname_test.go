package anyblockjson

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
)

// refNameNormalize (refs.go) is the identifier normalization that used to
// mint KEY labels, surviving for its one remaining surface: the informative
// `#name` reference suffix (§9). Key spellings are raw names now and need no
// normalization; the suffix still does, because its grammar is what makes
// the `#` split safe. The rules — and these pins — are unchanged from the
// key-label era on purpose: the suffix is informative and trimmed unread,
// and keeping the bytes stable keeps every already-written reference
// identical on its next export.
//
// Every case here is a decision with a plausible alternative, and the
// alternative is named in the case's own comment — a table that only pinned
// the happy path would pass with a transliterating normalizer.
func TestRefNameNormalize(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		why  string
	}{
		{"Publish Date", "publish_date", "a name separates its own words, and this one does"},
		{"Original creation date", "original_creation_date", ""},
		// camelCase is a KEY phenomenon; a person types "Due Date". Hump
		// splitting is what turned "P2P Sync" into `p_2_p_sync`.
		{"iconEmoji", "iconemoji", "a camelCase NAME is a key someone pasted into a name field"},
		{"mediaArtistURL", "mediaartisturl", "no acronym rule can tell SDKs from XMLParser anyway"},
		{"P2P Sync", "p2p_sync", "a letter and a digit are one word"},
		{"GitHub Stars", "github_stars", ""},
		{"Platform SDKs", "platform_sdks", ""},
		// a leading `_` run is CONTENT: 20 production relations from two
		// integrations namespace themselves this way.
		{"__amemory_salience", "__amemory_salience", "a namespace prefix survives"},
		{"trailing_", "trailing", "a trailing run is still a gap between a word and nothing"},
		{"a__b", "a_b", "an interior run still collapses"},
		{"___", "", "underscores alone name nothing"},
		{"  spaced  name ", "spaced_name", "separator runs collapse and edges trim"},

		// non-Latin scripts are kept, never transliterated
		{"Тоггл", "тоггл", "no transliteration"},
		{"日本語のプロパティ", "日本語のプロパティ", "no transliteration"},
		{"tiếng Việt", "tiếng_việt", "no transliteration"},

		// emoji, punctuation and symbols are separators, not letters
		{"Priority 📌", "priority", ""},
		{"C#", "c", "the suffix grammar admits no `#` — the split guarantee"},
		{"#", "", "nothing left to name means no suffix, never a dangling `#`"},
		{"", "", ""},

		// the leading-`_` escape for the two grammar faults — kept for byte
		// stability with every reference already written
		{"50% done", "_50_done", "identStart is a letter or `_`, never a digit"},
		{"All", "_all", "`all` is a reserved word of the filter grammar"},

		// A combining mark modifies the letter before it: neither a
		// separator (which would cut the word at every virama) nor
		// droppable (this script writes its VOWELS as marks, and
		// मिल/मूल/मल/मैल would all become मल).
		{"क्षत्रिय", "क्षत्रिय", "marks are kept: they carry the word"},
		{"हिन्दी", "हिन्दी", ""},
		{"İstanbul", "istanbul", "lowercasing İ leaves a combining dot behind"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, refNameNormalize(tc.in), tc.why)
		})
	}

	// two visually identical names can be different byte sequences, and two
	// exports of one reference must not differ by normalization form
	t.Run("NFD and NFC forms of one name normalize to one suffix", func(t *testing.T) {
		nfc := "Ünïcødé"
		nfd := norm.NFD.String(nfc)
		assert.NotEqual(t, nfc, nfd, "the fixture has to actually differ in bytes")
		assert.Equal(t, refNameNormalize(nfc), refNameNormalize(nfd))
		assert.Equal(t, "ünïcødé", refNameNormalize(nfd))
	})

	// the control that makes the marks rows meaningful: four words that
	// differ ONLY by their marks must stay four different suffixes
	t.Run("marks distinguish words rather than collapsing them", func(t *testing.T) {
		seen := map[string]string{}
		for _, name := range []string{"मिल", "मूल", "मल", "मैल"} {
			label := refNameNormalize(name)
			if prev, clash := seen[label]; clash {
				t.Fatalf("%q and %q both normalize to %q — the mark carries the meaning", prev, name, label)
			}
			seen[label] = name
		}
		assert.Len(t, seen, 4, "four words, four suffixes")
	})
}

// The suffix grammar's whole contract: whatever refNameNormalize mints, when
// it is not empty, is a bare identifier the filter grammar accepts — which
// is a strictly narrower shape than "contains no `#`", so the split
// guarantee (§9) rides along. Asserted as a PROPERTY over hostile input
// rather than as a case list, so it keeps holding when either side grows.
func TestRefNameNormalize_EverySuffixIsABareIdentifier(t *testing.T) {
	inputs := []string{
		"", " ", "#", "\U0001F389", "50% done", "007", "_", "__", "-",
		"All", "NOT", "in", "id", "type", "Cost & type", "What's missing",
		"Тоггл", "日本語のプロパティ", "क्षत्रिय", "C#", "C++", "a.b", "a#b#c",
		"\t\n\v", "a\nb", "­", "​", "é", "ß", "ﬁ", "Ⅻ", "½", "①",
		strings.Repeat("a", 200), strings.Repeat("é ", 90),
	}
	alphabet := []rune("aZ_-. 0б日ế\U0001F389#/\\\"'\t́é½Ⅻ")
	rnd := rand.New(rand.NewSource(7383))
	for i := 0; i < 4000; i++ {
		var b strings.Builder
		for n := rnd.Intn(12); n >= 0; n-- {
			b.WriteRune(alphabet[rnd.Intn(len(alphabet))])
		}
		inputs = append(inputs, b.String())
	}
	for _, in := range inputs {
		label := refNameNormalize(in)
		if label == "" {
			continue // "no suffix" is always a legal answer
		}
		require.Truef(t, filterstring.IsBareKey(label),
			"suffix %q minted from %q is not a bare identifier", label, in)
		require.NotContainsf(t, label, "#", "minted from %q — the split guarantee", in)
	}
}
