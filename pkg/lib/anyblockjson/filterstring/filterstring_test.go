package filterstring

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parse is the test harness: parse with opts and return the emitted JSON.
func parse(t *testing.T, input string, opts Options) string {
	t.Helper()
	out, err := Parse(input, opts)
	require.NoError(t, err)
	return string(out)
}

// parseErr asserts the parse fails and returns the offset-addressed error.
func parseErr(t *testing.T, input string, opts Options) *Error {
	t.Helper()
	_, err := Parse(input, opts)
	require.Error(t, err)
	var pe *Error
	require.True(t, errors.As(err, &pe), "every parse error is *Error, got %T", err)
	return pe
}

func TestParse_Grammar(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // the §6.2 structured filters array, compact JSON
	}{
		{
			name:  "single equal leaf",
			input: `done = false`,
			want:  `[{"property":"done","condition":"equal","value":false}]`,
		},
		{
			name:  "not equal number",
			input: `priority != 3`,
			want:  `[{"property":"priority","condition":"not_equal","value":3}]`,
		},
		{
			name:  "all comparison operators",
			input: `a > 1 AND b < 2 AND c >= 3 AND d <= 4`,
			want: `[{"property":"a","condition":"greater","value":1},` +
				`{"property":"b","condition":"less","value":2},` +
				`{"property":"c","condition":"greater_or_equal","value":3},` +
				`{"property":"d","condition":"less_or_equal","value":4}]`,
		},
		{
			name:  "contains and not contains",
			input: `name CONTAINS "report" AND name NOT CONTAINS "draft"`,
			want: `[{"property":"name","condition":"contains","value":"report"},` +
				`{"property":"name","condition":"not_contains","value":"draft"}]`,
		},
		{
			name:  "in list",
			input: `status IN ("In progress", "Blocked")`,
			want:  `[{"property":"status","condition":"in","value":["In progress","Blocked"]}]`,
		},
		{
			name:  "not in list",
			input: `status NOT IN ("Done")`,
			want:  `[{"property":"status","condition":"not_in","value":["Done"]}]`,
		},
		{
			name:  "has all and not has all",
			input: `tags HAS ALL ("urgent", "q3") OR tags NOT HAS ALL ("later")`,
			want: `[{"operator":"or","filters":[` +
				`{"property":"tags","condition":"all_in","value":["urgent","q3"]},` +
				`{"property":"tags","condition":"not_all_in","value":["later"]}]}]`,
		},
		{
			name:  "set literal is exactIn",
			input: `tags = ("a", "b")`,
			want:  `[{"property":"tags","condition":"exact_in","value":["a","b"]}]`,
		},
		{
			name:  "negated set literal is notExactIn",
			input: `tags != ("a")`,
			want:  `[{"property":"tags","condition":"not_exact_in","value":["a"]}]`,
		},
		{
			name:  "is empty and is not empty",
			input: `assignee IS EMPTY OR assignee IS NOT EMPTY`,
			want: `[{"operator":"or","filters":[` +
				`{"property":"assignee","condition":"empty"},` +
				`{"property":"assignee","condition":"not_empty"}]}]`,
		},
		{
			name:  "exists",
			input: `assignee EXISTS`,
			want:  `[{"property":"assignee","condition":"exists"}]`,
		},
		{
			name:  "valueless date preset",
			input: `dueDate < currentWeek()`,
			want:  `[{"property":"dueDate","condition":"less","date_preset":"current_week"}]`,
		},
		{
			name:  "counting preset keeps its operand as value",
			input: `lastModifiedDate > daysAgo(7)`,
			want:  `[{"property":"lastModifiedDate","condition":"greater","value":7,"date_preset":"number_of_days_ago"}]`,
		},
		{
			name:  "daysFromNow maps to numberOfDaysNow",
			input: `dueDate <= daysFromNow(0)`,
			want:  `[{"property":"dueDate","condition":"less_or_equal","value":0,"date_preset":"number_of_days_now"}]`,
		},
		{
			name:  "keywords are case-insensitive",
			input: `done = false and name contains "x" or assignee is empty`,
			want: `[{"operator":"or","filters":[` +
				`{"operator":"and","filters":[` +
				`{"property":"done","condition":"equal","value":false},` +
				`{"property":"name","condition":"contains","value":"x"}]},` +
				`{"property":"assignee","condition":"empty"}]}]`,
		},
		{
			name:  "string escapes",
			input: `name = "say \"hi\"\n"`,
			want:  `[{"property":"name","condition":"equal","value":"say \"hi\"\n"}]`,
		},
		{
			name:  "negative number",
			input: `balance < -5`,
			want:  `[{"property":"balance","condition":"less","value":-5}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parse(t, tt.input, Options{}))
		})
	}
}

func TestParse_Precedence(t *testing.T) {
	t.Run("AND binds tighter than OR", func(t *testing.T) {
		want := `[{"operator":"or","filters":[` +
			`{"operator":"and","filters":[` +
			`{"property":"a","condition":"equal","value":1},` +
			`{"property":"b","condition":"equal","value":2}]},` +
			`{"operator":"and","filters":[` +
			`{"property":"c","condition":"equal","value":3},` +
			`{"property":"d","condition":"equal","value":4}]}]}]`
		assert.Equal(t, want, parse(t, `a = 1 AND b = 2 OR c = 3 AND d = 4`, Options{}))
	})

	t.Run("parentheses group OR under AND as bare top-level siblings", func(t *testing.T) {
		// the worked SPEC example: top-level implicit AND, group only for OR
		want := `[{"property":"done","condition":"equal","value":false},` +
			`{"operator":"or","filters":[` +
			`{"property":"dueDate","condition":"less","date_preset":"current_week"},` +
			`{"property":"dueDate","condition":"empty"}]}]`
		assert.Equal(t, want, parse(t, `done = false AND (dueDate < currentWeek() OR dueDate IS EMPTY)`, Options{}))
	})

	t.Run("redundant parentheses flatten (canonical form)", func(t *testing.T) {
		want := `[{"property":"a","condition":"equal","value":1},` +
			`{"property":"b","condition":"equal","value":2},` +
			`{"property":"c","condition":"equal","value":3}]`
		assert.Equal(t, want, parse(t, `(a = 1 AND b = 2) AND (c = 3)`, Options{}))
	})

	t.Run("nested OR in OR flattens", func(t *testing.T) {
		want := `[{"operator":"or","filters":[` +
			`{"property":"a","condition":"equal","value":1},` +
			`{"property":"b","condition":"equal","value":2},` +
			`{"property":"c","condition":"equal","value":3}]}]`
		assert.Equal(t, want, parse(t, `a = 1 OR (b = 2 OR c = 3)`, Options{}))
	})

	t.Run("single leaf in parentheses", func(t *testing.T) {
		assert.Equal(t, `[{"property":"a","condition":"equal","value":1}]`,
			parse(t, `(a = 1)`, Options{}))
	})
}

func TestParse_DateConversion(t *testing.T) {
	dateFormat := Options{ResolveFormat: func(key string) (string, bool) {
		if key == "dueDate" {
			return "date", true
		}
		return "text", true
	}}

	t.Run("RFC 3339 date-only converts to unix on a date property", func(t *testing.T) {
		// 2026-08-01T00:00:00Z = 1785542400
		assert.Equal(t, `[{"property":"dueDate","condition":"less","value":1785542400}]`,
			parse(t, `dueDate < "2026-08-01"`, dateFormat))
	})

	t.Run("full RFC 3339 timestamp converts", func(t *testing.T) {
		assert.Equal(t, `[{"property":"dueDate","condition":"greater","value":1785596400}]`,
			parse(t, `dueDate > "2026-08-01T15:00:00Z"`, dateFormat))
	})

	t.Run("a text property keeps date-looking strings verbatim", func(t *testing.T) {
		assert.Equal(t, `[{"property":"name","condition":"equal","value":"2026-08-01"}]`,
			parse(t, `name = "2026-08-01"`, dateFormat))
	})

	t.Run("non-RFC-3339 string on a date property errors", func(t *testing.T) {
		pe := parseErr(t, `dueDate < "next tuesday"`, dateFormat)
		assert.Equal(t, 10, pe.Offset)
		assert.Contains(t, pe.Message, `is not an RFC 3339 date`)
		assert.Contains(t, pe.Hint, "currentWeek()")
	})

	t.Run("preset on a non-date property errors", func(t *testing.T) {
		pe := parseErr(t, `name = currentWeek()`, dateFormat)
		assert.Contains(t, pe.Message, `currentWeek() is a date preset, but property "name" has format "text"`)
	})

	t.Run("without a format resolver strings stay verbatim", func(t *testing.T) {
		assert.Equal(t, `[{"property":"dueDate","condition":"less","value":"2026-08-01"}]`,
			parse(t, `dueDate < "2026-08-01"`, Options{}))
	})
}

func TestParse_ErrorPositions(t *testing.T) {
	t.Run("unknown property key with did-you-mean", func(t *testing.T) {
		pe := parseErr(t, `done = false AND dueDat < currentWeek()`,
			Options{KnownKeys: []string{"done", "dueDate", "status", "name"}})
		assert.Equal(t, 17, pe.Offset)
		assert.Equal(t, "dueDat", pe.Token)
		assert.Contains(t, pe.Message, `unknown property key "dueDat"`)
		assert.Contains(t, pe.Message, "known property keys: done, dueDate, name, status")
		assert.Equal(t, "did you mean dueDate?", pe.Hint)
		assert.Contains(t, pe.Error(), `parse error at offset 17 near "dueDat"`)
	})

	t.Run("unknown option name with did-you-mean", func(t *testing.T) {
		opts := Options{
			ResolveFormat: func(key string) (string, bool) { return "select", true },
			KnownOptions: func(key string) ([]string, bool) {
				return []string{"In progress", "Blocked", "Done"}, true
			},
		}
		pe := parseErr(t, `status IN ("In progres")`, opts)
		assert.Equal(t, 11, pe.Offset)
		assert.Equal(t, `"In progres"`, pe.Token)
		assert.Contains(t, pe.Message, `property "status" has no option named "In progres"`)
		assert.Contains(t, pe.Message, "a query never creates options")
		assert.Equal(t, "did you mean In progress?", pe.Hint)
	})

	t.Run("unterminated string", func(t *testing.T) {
		pe := parseErr(t, `name = "unclosed`, Options{})
		assert.Equal(t, 7, pe.Offset)
		assert.Contains(t, pe.Message, "unterminated string literal")
	})

	t.Run("missing closing parenthesis", func(t *testing.T) {
		pe := parseErr(t, `(a = 1 AND b = 2`, Options{})
		assert.Equal(t, 16, pe.Offset)
		assert.Equal(t, "", pe.Token) // EOF
		assert.Contains(t, pe.Message, "expected ) to close the group opened at offset 0")
		assert.Contains(t, pe.Error(), "at end of input")
	})

	t.Run("missing condition after key", func(t *testing.T) {
		pe := parseErr(t, `done`, Options{})
		assert.Contains(t, pe.Message, `expected a condition after property "done"`)
		assert.Contains(t, pe.Hint, "IS EMPTY")
	})

	t.Run("trailing garbage after a complete filter", func(t *testing.T) {
		pe := parseErr(t, `a = 1 b = 2`, Options{})
		assert.Equal(t, 6, pe.Offset)
		assert.Equal(t, "b", pe.Token)
		assert.Contains(t, pe.Message, "expected AND, OR or end of input")
	})

	t.Run("unknown function with did-you-mean", func(t *testing.T) {
		pe := parseErr(t, `dueDate < currentWek()`, Options{})
		assert.Equal(t, 10, pe.Offset)
		assert.Contains(t, pe.Message, `unknown function "currentWek"`)
		assert.Equal(t, "did you mean currentWeek?", pe.Hint)
	})

	t.Run("counting preset without operand", func(t *testing.T) {
		pe := parseErr(t, `dueDate > daysAgo()`, Options{})
		assert.Contains(t, pe.Message, "daysAgo takes a day count, e.g. daysAgo(7)")
	})

	t.Run("plain preset with an operand", func(t *testing.T) {
		pe := parseErr(t, `dueDate < yesterday(5)`, Options{})
		assert.Contains(t, pe.Message, "yesterday takes no arguments")
	})

	t.Run("preset inside a value list", func(t *testing.T) {
		pe := parseErr(t, `dueDate IN (today())`, Options{})
		assert.Contains(t, pe.Message, "date presets cannot appear inside a value list")
	})

	t.Run("reserved word as key", func(t *testing.T) {
		pe := parseErr(t, `in = 1`, Options{})
		assert.Contains(t, pe.Message, `"in" is a reserved word, not a property key`)
	})

	t.Run("empty filter", func(t *testing.T) {
		pe := parseErr(t, `   `, Options{})
		assert.Contains(t, pe.Message, "empty filter")
	})

	t.Run("empty value list", func(t *testing.T) {
		pe := parseErr(t, `status IN ()`, Options{})
		assert.Contains(t, pe.Message, "a value list needs at least one value")
	})

	t.Run("bare word in value position", func(t *testing.T) {
		pe := parseErr(t, `status = Done`, Options{})
		assert.Equal(t, 9, pe.Offset)
		assert.Contains(t, pe.Message, `unexpected bare word "Done" in value position`)
		assert.Contains(t, pe.Hint, "double-quoted strings")
	})

	t.Run("free-standing NOT is not in the grammar", func(t *testing.T) {
		pe := parseErr(t, `NOT (done = true)`, Options{})
		assert.Contains(t, pe.Message, "reserved word")
	})

	t.Run("set literal after ordering operator", func(t *testing.T) {
		pe := parseErr(t, `priority > (1, 2)`, Options{})
		assert.Contains(t, pe.Message, "a value list is only allowed after = or != (set literal), not after >")
	})

	t.Run("HAS without ALL", func(t *testing.T) {
		pe := parseErr(t, `tags HAS ("a")`, Options{})
		assert.Contains(t, pe.Message, "expected ALL after HAS")
	})

	t.Run("NOT followed by nothing usable", func(t *testing.T) {
		pe := parseErr(t, `tags NOT = 1`, Options{})
		assert.Contains(t, pe.Message, "expected CONTAINS, IN or HAS ALL after NOT")
		assert.Contains(t, pe.Hint, "IS NOT EMPTY")
	})
}

func TestParse_Bounds(t *testing.T) {
	t.Run("a paren bomb is an ordinary parse error, never a crash", func(t *testing.T) {
		// the historical failure mode: unbounded recursion → goroutine stack
		// overflow → runtime FATAL that kills the whole process
		pe := parseErr(t, strings.Repeat("(", 100_000), Options{})
		assert.Contains(t, pe.Message, "maximum is 4096")
	})

	t.Run("a balanced deep nest under the length cap hits the depth bound", func(t *testing.T) {
		input := strings.Repeat("(", 1000) + "a = 1" + strings.Repeat(")", 1000)
		pe := parseErr(t, input, Options{})
		assert.Contains(t, pe.Message, "filter groups nest at most 32 deep")
	})

	t.Run("nesting beyond 32 groups is rejected with an offset", func(t *testing.T) {
		input := strings.Repeat("(", 40) + "a = 1" + strings.Repeat(")", 40)
		pe := parseErr(t, input, Options{})
		assert.Equal(t, 32, pe.Offset, "the 33rd open paren is the offender")
		assert.Contains(t, pe.Message, "filter groups nest at most 32 deep")
	})

	t.Run("nesting at exactly 32 groups parses", func(t *testing.T) {
		input := strings.Repeat("(", 32) + "a = 1" + strings.Repeat(")", 32)
		assert.Equal(t, `[{"property":"a","condition":"equal","value":1}]`,
			parse(t, input, Options{}))
	})

	t.Run("input beyond 4096 bytes is rejected before lexing", func(t *testing.T) {
		pe := parseErr(t, "name = \""+strings.Repeat("x", 5000)+"\"", Options{})
		assert.Contains(t, pe.Message, "the maximum is 4096")
	})

	t.Run("counting presets bound the day count", func(t *testing.T) {
		pe := parseErr(t, `dueDate > daysAgo(999999999)`, Options{})
		assert.Contains(t, pe.Message, "daysAgo takes a whole day count between 0 and 36500")
	})

	t.Run("an unterminated string echoes at most 32 runes", func(t *testing.T) {
		pe := parseErr(t, `a = "`+strings.Repeat("x", 1000), Options{})
		assert.Contains(t, pe.Message, "unterminated string literal")
		assert.True(t, strings.HasSuffix(pe.Token, "…"), "long tokens are truncated, got %q", pe.Token)
		assert.Less(t, len(pe.Error()), 200, "the error must not mirror the input back")
	})
}

func TestParse_UnicodeKeys(t *testing.T) {
	t.Run("non-ASCII property keys are identifiers", func(t *testing.T) {
		assert.Equal(t, `[{"property":"café","condition":"equal","value":1}]`,
			parse(t, `café = 1`, Options{}))
		assert.Equal(t, `[{"property":"дата","condition":"empty"}]`,
			parse(t, `дата IS EMPTY`, Options{}))
	})

	t.Run("an unknown non-ASCII key reports the full key, not a stray byte", func(t *testing.T) {
		pe := parseErr(t, `café = 1`, Options{KnownKeys: []string{"status"}})
		assert.Equal(t, "café", pe.Token)
		assert.Contains(t, pe.Message, `unknown property key "café"`)
	})

	t.Run("an unexpected rune is reported as the rune the caller wrote", func(t *testing.T) {
		pe := parseErr(t, `a © 1`, Options{})
		assert.Equal(t, 2, pe.Offset)
		assert.Equal(t, "©", pe.Token)
		assert.Contains(t, pe.Message, `unexpected character "©"`)
	})
}

func TestParse_Steering(t *testing.T) {
	t.Run("single quotes get the double-quote hint", func(t *testing.T) {
		pe := parseErr(t, `severity = 'High'`, Options{})
		assert.Equal(t, 11, pe.Offset)
		assert.Contains(t, pe.Hint, `string values use double quotes`)
	})

	t.Run("a reserved-word key steers to the structured filters array", func(t *testing.T) {
		pe := parseErr(t, `all = true`, Options{})
		assert.Contains(t, pe.Message, `"all" is a reserved word`)
		assert.Contains(t, pe.Hint, "structured filters array")
	})

	t.Run("a known key the syntax cannot spell steers to the structured form", func(t *testing.T) {
		pe := parseErr(t, `due IS EMPTY`, Options{KnownKeys: []string{"due-date"}})
		assert.Contains(t, pe.Hint, "did you mean due-date?")
		assert.Contains(t, pe.Hint, `property key "due-date" cannot be written in the compact filter string`)
		assert.Contains(t, pe.Hint, "structured filters array")
	})
}

func TestParse_PresetConditions(t *testing.T) {
	t.Run("a preset with != is rejected — the engine would drop it", func(t *testing.T) {
		pe := parseErr(t, `dueDate != today()`, Options{})
		assert.Equal(t, 11, pe.Offset, "the error addresses the preset, not the closing paren")
		assert.Equal(t, "today", pe.Token)
		assert.Contains(t, pe.Message, "a date preset cannot be used with not_equal")
		assert.Contains(t, pe.Hint, "negate by range")
	})

	t.Run("a preset with CONTAINS addresses the preset name", func(t *testing.T) {
		pe := parseErr(t, `name CONTAINS today()`, Options{})
		assert.Equal(t, 14, pe.Offset)
		assert.Equal(t, "today", pe.Token)
		assert.Contains(t, pe.Message, "a date preset cannot be used with contains")
	})

	t.Run("a preset inside a value list addresses the preset name", func(t *testing.T) {
		pe := parseErr(t, `dueDate IN ("x", today())`, Options{})
		assert.Equal(t, 17, pe.Offset)
		assert.Equal(t, "today", pe.Token)
		assert.Contains(t, pe.Message, "date presets cannot appear inside a value list")
	})
}

func TestParse_EmitsValidJSON(t *testing.T) {
	// every grammar example must emit a decodable §6.2 array
	for _, example := range Examples {
		t.Run(example, func(t *testing.T) {
			out, err := Parse(example, Options{})
			require.NoError(t, err)
			var nodes []map[string]any
			require.NoError(t, json.Unmarshal(out, &nodes))
			require.NotEmpty(t, nodes)
		})
	}
}
