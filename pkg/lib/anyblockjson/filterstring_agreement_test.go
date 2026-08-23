package anyblockjson

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
)

// The compact filter grammar's compiler lives in a SUBPACKAGE (§6.2.1,
// §13), so this package's vocabulary invariant — every identifier the format
// defines is snake_case (§1) — could not see the tokens it emits. It drifted:
// the compiler wrote "notEqual", "greaterOrEqual", "allIn", "notEmpty" and a
// `datePreset` member while the reader had already migrated to `not_equal`,
// `greater_or_equal`, `all_in`, `not_empty` and `date_preset`, so
// UnmarshalFilters rejected documents its own compiler produced.
//
// Nothing structural stops that happening again — the two vocabularies are
// declared in different packages, one as parser output and one as a reader
// table — so the guard has to be a test that crosses the boundary. It asserts
// the only thing that matters: whatever the compiler emits, this package
// accepts.
func TestFilterString_CompilerOutputIsAcceptedByThisPackage(t *testing.T) {
	// one input per condition the grammar can emit, plus the date presets,
	// so a token that drifts has nowhere to hide
	// one input per condition the grammar can emit, plus the date presets,
	// so a token that drifts has nowhere to hide. NOTE the asymmetry the
	// migration deliberately kept: what a user TYPES stays camelCase
	// (`daysAgo`, `currentWeek`) because it is the compact syntax's own
	// vocabulary, served as an EBNF grammar; only the JSON it compiles TO is
	// this format's snake_case (§1).
	inputs := []string{
		`status = "Done"`,
		`status != "Done"`,
		`count > 3`,
		`count < 3`,
		`count >= 3`,
		`count <= 3`,
		`name CONTAINS "spec"`,
		`name NOT CONTAINS "draft"`,
		`status IN ("Done", "In progress")`,
		`status NOT IN ("Done")`,
		`tag HAS ALL ("a", "b")`,
		`tag NOT HAS ALL ("a")`,
		`name IS EMPTY`,
		`name IS NOT EMPTY`,
		`name EXISTS`,
		`due_date = today()`,
		`due_date > daysAgo(7)`,
		`due_date < daysFromNow(3)`,
		`due_date = currentWeek()`,
		`due_date = lastMonth()`,
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			raw, err := filterstring.Parse(in, filterstring.Options{})
			require.NoError(t, err, "the compiler must accept its own documented grammar (§6.2.1)")
			require.NotEmpty(t, raw)

			got, err := UnmarshalFilters(raw, Options{})
			// the assertion that would have caught the drift: this package
			// reads what that package writes
			require.NoError(t, err, "this package must accept what its own filter compiler emits: %s", string(raw))
			assert.NotEmpty(t, got)
		})
	}
}

// The label rule (§3, label.go) mints spellings out of user text — a stored
// api slug or a display name — and the ONE thing every spelling it mints has
// to be is a key this format can write everywhere it names keys. The
// narrowest of those surfaces is the compact filter string, whose grammar
// lives in the subpackage: `key = identifier`, letters of any script, digits,
// `_`, never a leading digit, never a reserved word (§6.2.1). That is not a
// style rule — a property whose label starts with a digit cannot be filtered
// on at all, which is the state of every bson-keyed property today.
//
// The two halves are declared in different packages on purpose (the grammar
// belongs to the parser, the label rule to the format), so nothing structural
// keeps them in step — the same shape as the vocabulary drift the test above
// exists for. This one asserts the PROPERTY rather than a case list, so it
// keeps holding when either side grows: whatever comes out of the label rule,
// when it is not empty, parses as a key and comes back byte-identical.
func TestKeyLabel_EveryLabelIsAKeyTheFilterGrammarAccepts(t *testing.T) {
	// hostile by hand: every rune class the rule can meet, plus the real
	// production names that motivated it
	inputs := []string{
		"", " ", "#", "\U0001F389", "50% done", "1221312425", "007", "_", "__", "-", "--",
		"All", "and", "NOT", "in", "Is", "Has", "Contains", "Empty", "exists", "true", "FALSE",
		"id", "type", "Publish Date", "More Information", "Active competitors",
		"Website", "Rating", "Contact Type", "Cost & type", "What's missing",
		"Killme ", "Date \U0001F4C5`", "Priority \U0001F4CC", "Email \U0001F4E7 ", "Company \U0001F3EC",
		"Тоггл", "日本語のプロパティ", "tiếng Việt", "العربية", "עברית", "İstanbul",
		"क्षत्रिय", "Ünïcødé", "naïve café", "C#", "C++", "C/C++", "@home", "a.b",
		"mediaArtistURL", "_score", "due_date", "iconEmoji", "  spaced  name ",
		"\t\n\v", "a\nb", " ", "­", "​", "é", "ß", "ﬁ", "Ⅻ", "½", "①",
		strings.Repeat("a", 200), strings.Repeat("é ", 90), "x" + strings.Repeat("_", 50),
	}
	// plus generated ones, so the property is not pinned to the list: a
	// deterministic walk over an alphabet holding every class at once
	alphabet := []rune("aZ_-. 0б日ế\U0001F389#/\\\"'\t́é½Ⅻ")
	rnd := rand.New(rand.NewSource(7383))
	for i := 0; i < 4000; i++ {
		var b strings.Builder
		for n := rnd.Intn(12); n >= 0; n-- {
			b.WriteRune(alphabet[rnd.Intn(len(alphabet))])
		}
		inputs = append(inputs, b.String())
	}

	const storedKey = "6a7663db61fab21cd4b9e101"
	for _, in := range inputs {
		// both stored facts feed the rule, and both namespaces use it
		for _, label := range []string{
			PropertyLabel(storedKey, in, ""),
			PropertyLabel(storedKey, "", in),
			TypeLabel(storedKey, in, ""),
			TypeLabel(storedKey, "", in),
		} {
			if label == "" {
				continue // "no label" is always a legal answer: the stored key is one
			}
			raw, err := filterstring.Parse(label+` = "x"`, filterstring.Options{})
			require.NoErrorf(t, err, "label %q minted from %q is not a key this format can write", label, in)

			var got []struct {
				Property string `json:"property"`
			}
			require.NoError(t, json.Unmarshal(raw, &got))
			require.Len(t, got, 1)
			assert.Equalf(t, label, got[0].Property,
				"the parser must read the label back byte-for-byte (minted from %q)", in)
		}
	}
}
