package anyblockjson

import (
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
