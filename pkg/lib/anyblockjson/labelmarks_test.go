package anyblockjson

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
)

// In Devanagari, Thai, Bengali, Tamil, Khmer and Myanmar the VOWELS are
// combining marks. Dropping them does not shorten a word, it changes it —
// and it does so in both directions at once:
//
//	मिल मूल मल मैल   (mil, mūl, mal, mail — four words)  all became  मल
//	हिन्दी and हिंदी (two legal spellings of ONE word)    became      हनद and हद
//
// So the population this rule exists to serve — non-Latin names, which the old
// transliterating path turned into `ri_ben_yu_nopuropatei` — was the one it
// served worst. NFC rescues Latin, Greek, Cyrillic and Vietnamese because
// precomposed forms exist for them; these scripts have none.
//
// The fix is in the GRAMMAR (§6.2.1, UAX #31 ID_Continue admits Mn and Mc),
// not in the normalizer: marks are now identifier parts, so the label keeps
// them and the normalizer deletes a rule instead of adding one.
//
// These fail only if marks stop surviving: each asserts the label equals the
// name, so a rule that dropped, reordered or duplicated a mark fails on the
// value, and the parse assertion catches a label the filter grammar cannot
// carry.
func TestPropertyLabel_CombiningMarksAreKept(t *testing.T) {
	for _, name := range []string{
		"मिल", "मूल", "मल", "मैल", // four distinct Hindi words
		"नाम", "किताब", "हिन्दी", "हिंदी",
		"ชื่อ",  // Thai
		"பெயர்", // Tamil
		"নাম",   // Bengali
	} {
		t.Run(name, func(t *testing.T) {
			label := PropertyLabel("storedKey", "", name)
			assert.Equal(t, name, label,
				"a word whose vowels are marks must label itself, not a consonant skeleton")
			require.True(t, filterstring.IsBareKey(label),
				"and the filter grammar must carry it (§6.2.1)")
		})
	}
}

// the control that makes the test above meaningful: four words that differ
// ONLY by their marks must produce four DIFFERENT labels, and two spellings
// of one word must not be forced apart.
func TestPropertyLabel_MarksDistinguishWordsRatherThanCollapsingThem(t *testing.T) {
	seen := map[string]string{}
	for _, name := range []string{"मिल", "मूल", "मल", "मैल"} {
		label := PropertyLabel("k", "", name)
		if prev, clash := seen[label]; clash {
			t.Fatalf("%q and %q both label %q — the mark carries the meaning", prev, name, label)
		}
		seen[label] = name
	}
	assert.Len(t, seen, 4, "four words, four labels")
}
