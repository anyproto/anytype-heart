package anyblockjson

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// fragOptionResolver resolves option names to ids from a fixed table —
// read-only, the query-path wiring.
type fragOptionResolver map[string]string

func (r fragOptionResolver) OptionId(key domain.RelationKey, name string) (string, bool) {
	id, ok := r[name]
	return id, ok
}

func (r fragOptionResolver) OptionName(key domain.RelationKey, id string) (string, bool) {
	for name, oid := range r {
		if oid == id {
			return name, true
		}
	}
	return "", false
}

func fragFilterOpts() Options {
	return Options{
		ResolveFormat: func(key domain.RelationKey) (model.RelationFormat, bool) {
			switch string(key) {
			case "status":
				return model.RelationFormat_status, true
			case "dueDate":
				return model.RelationFormat_date, true
			case "done":
				return model.RelationFormat_checkbox, true
			}
			return 0, false
		},
		ResolveOptions: fragOptionResolver{"In progress": "opt-inprogress", "Done": "opt-done"},
	}
}

func TestUnmarshalFilters(t *testing.T) {
	t.Run("bare leaves with option-name resolution and format rehydration", func(t *testing.T) {
		// given
		raw := json.RawMessage(`[
			{"property":"done","condition":"equal","value":false},
			{"property":"status","condition":"in","value":["In progress","Done"]}]`)

		// when
		got, err := UnmarshalFilters(raw, fragFilterOpts())

		// then
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "done", got[0].RelationKey)
		assert.Equal(t, model.BlockContentDataviewFilter_Equal, got[0].Condition)
		assert.Equal(t, model.RelationFormat_checkbox, got[0].Format)
		assert.False(t, got[0].Value.GetBoolValue())
		assert.Equal(t, model.BlockContentDataviewFilter_In, got[1].Condition)
		values := got[1].Value.GetListValue().Values
		require.Len(t, values, 2)
		assert.Equal(t, "opt-inprogress", values[0].GetStringValue(), "option names resolve to ids")
		assert.Equal(t, "opt-done", values[1].GetStringValue())
	})

	t.Run("or group with date preset", func(t *testing.T) {
		raw := json.RawMessage(`[{"operator":"or","filters":[
			{"property":"dueDate","condition":"less","date_preset":"current_week"},
			{"property":"dueDate","condition":"empty"}]}]`)

		got, err := UnmarshalFilters(raw, fragFilterOpts())

		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, model.BlockContentDataviewFilter_Or, got[0].Operator)
		require.Len(t, got[0].NestedFilters, 2)
		assert.Equal(t, model.BlockContentDataviewFilter_CurrentWeek, got[0].NestedFilters[0].QuickOption)
		assert.Equal(t, model.BlockContentDataviewFilter_Empty, got[0].NestedFilters[1].Condition)
	})

	t.Run("unknown condition is a path-addressed error naming the vocabulary", func(t *testing.T) {
		raw := json.RawMessage(`[{"property":"done","condition":"equals","value":false}]`)

		_, err := UnmarshalFilters(raw, fragFilterOpts())

		require.Error(t, err)
		var ve *ValidationError
		require.True(t, errors.As(err, &ve))
		require.Len(t, ve.Issues, 1)
		assert.Equal(t, "/filters/0/condition", ve.Issues[0].Path)
		assert.Contains(t, ve.Issues[0].Message, `unknown condition "equals"`)
		assert.Contains(t, ve.Issues[0].Message, "equal, not_equal, greater")
	})

	t.Run("unknown datePreset and operator error", func(t *testing.T) {
		raw := json.RawMessage(`[{"operator":"xor","filters":[
			{"property":"dueDate","condition":"less","date_preset":"thisWeek"}]}]`)

		_, err := UnmarshalFilters(raw, fragFilterOpts())

		var ve *ValidationError
		require.True(t, errors.As(err, &ve))
		require.Len(t, ve.Issues, 2)
		assert.Equal(t, "/filters/0/operator", ve.Issues[0].Path)
		assert.Contains(t, ve.Issues[0].Message, `unknown operator "xor"`)
		assert.Equal(t, "/filters/0/filters/0/datePreset", ve.Issues[1].Path)
		assert.Contains(t, ve.Issues[1].Message, `unknown datePreset "thisWeek"`)
	})

	t.Run("counting preset without a value errors (the document rule)", func(t *testing.T) {
		raw := json.RawMessage(`[{"property":"dueDate","condition":"greater","date_preset":"number_of_days_ago"}]`)

		_, err := UnmarshalFilters(raw, fragFilterOpts())

		var ve *ValidationError
		require.True(t, errors.As(err, &ve))
		require.Len(t, ve.Issues, 1)
		assert.Equal(t, "/filters/0", ve.Issues[0].Path)
		assert.Contains(t, ve.Issues[0].Message, "needs a day count")
	})

	t.Run("unguarded date less warns on the OnWarning channel", func(t *testing.T) {
		raw := json.RawMessage(`[{"property":"dueDate","condition":"less","date_preset":"today"}]`)
		opts := fragFilterOpts()
		var warnings []Issue
		opts.OnWarning = func(i Issue) { warnings = append(warnings, i) }

		got, err := UnmarshalFilters(raw, opts)

		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Len(t, warnings, 1)
		assert.Equal(t, "/filters/0", warnings[0].Path)
		assert.Contains(t, warnings[0].Message, "also matches objects with no dueDate")
		assert.Contains(t, warnings[0].Message, "not_empty")
	})

	t.Run("guarded date less is clean", func(t *testing.T) {
		raw := json.RawMessage(`[
			{"property":"dueDate","condition":"not_empty"},
			{"property":"dueDate","condition":"less","date_preset":"today"}]`)
		opts := fragFilterOpts()
		var warnings []Issue
		opts.OnWarning = func(i Issue) { warnings = append(warnings, i) }

		_, err := UnmarshalFilters(raw, opts)

		require.NoError(t, err)
		assert.Empty(t, warnings)
	})

	t.Run("placeholder on a non-object property warns", func(t *testing.T) {
		// a WARNING here too, matching the document door: the same rule at
		// two severities would let a stored filter validate on one surface
		// and refuse on the other, and the stored pair is real data (the
		// document door's I1 arm pins that side)
		raw := json.RawMessage(`[{"property":"status","condition":"in","value":["_filter_template_2_"]}]`)
		opts := fragFilterOpts()
		var warnings []Issue
		opts.OnWarning = func(i Issue) { warnings = append(warnings, i) }

		_, err := UnmarshalFilters(raw, opts)

		require.NoError(t, err)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0].Message, "resolves to an object id")
	})

	t.Run("leaf without property errors", func(t *testing.T) {
		raw := json.RawMessage(`[{"condition":"equal","value":1}]`)

		_, err := UnmarshalFilters(raw, fragFilterOpts())

		var ve *ValidationError
		require.True(t, errors.As(err, &ve))
		assert.Equal(t, "/filters/0/property", ve.Issues[0].Path)
	})

	t.Run("filterstring output feeds straight in (the one-tree contract)", func(t *testing.T) {
		// the string form's emitted array is the same shape this codec takes
		raw := json.RawMessage(`[{"property":"done","condition":"equal","value":false},` +
			`{"operator":"or","filters":[` +
			`{"property":"dueDate","condition":"less","date_preset":"current_week"},` +
			`{"property":"dueDate","condition":"empty"}]}]`)

		got, err := UnmarshalFilters(raw, fragFilterOpts())

		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, model.BlockContentDataviewFilter_Or, got[1].Operator)
	})
}

func TestUnmarshalSorts(t *testing.T) {
	t.Run("direction, emptyPlacement and format rehydrate", func(t *testing.T) {
		raw := json.RawMessage(`[{"property":"dueDate","direction":"desc","empty_placement":"end","include_time":true}]`)

		got, err := UnmarshalSorts(raw, fragFilterOpts())

		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "dueDate", got[0].RelationKey)
		assert.Equal(t, model.BlockContentDataviewSort_Desc, got[0].Type)
		assert.Equal(t, model.BlockContentDataviewSort_End, got[0].EmptyPlacement)
		assert.Equal(t, model.RelationFormat_date, got[0].Format)
		assert.True(t, got[0].IncludeTime)
	})

	t.Run("default direction is asc", func(t *testing.T) {
		got, err := UnmarshalSorts(json.RawMessage(`[{"property":"name"}]`), Options{})

		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, model.BlockContentDataviewSort_Asc, got[0].Type)
	})

	t.Run("unknown direction errors with allowed values", func(t *testing.T) {
		_, err := UnmarshalSorts(json.RawMessage(`[{"property":"name","direction":"descending"}]`), Options{})

		var ve *ValidationError
		require.True(t, errors.As(err, &ve))
		require.Len(t, ve.Issues, 1)
		assert.Equal(t, "/sorts/0/direction", ve.Issues[0].Path)
		assert.Contains(t, ve.Issues[0].Message, `unknown direction "descending" — allowed: asc, desc, custom`)
	})

	t.Run("missing property errors", func(t *testing.T) {
		_, err := UnmarshalSorts(json.RawMessage(`[{"direction":"asc"}]`), Options{})

		var ve *ValidationError
		require.True(t, errors.As(err, &ve))
		assert.Equal(t, "/sorts/0/property", ve.Issues[0].Path)
	})
}

// The fragment door is the API v2 query surface, and it validates against the
// same $defs/filterNode and $defs/sort fragments the whole-document path holds
// a view's filters and sorts to (§12). Before it did, an unknown member was
// silently DECODED AND DROPPED — `jsonUnmarshal` is a plain json.Unmarshal —
// so a misspelled member ("directions", "datePresets") turned a stated query
// into a different, quieter one with no error, where the identical shape
// inside a whole document was a hard, path-addressed refusal.
func TestUnmarshalFilters_UnknownMemberIsRefused(t *testing.T) {
	t.Run("a filter leaf carrying an unrecognized member is refused with the member named", func(t *testing.T) {
		// given
		raw := json.RawMessage(`[{"property":"done","condition":"equal","frobnicate":true}]`)

		// when
		_, err := UnmarshalFilters(raw, fragFilterOpts())

		// then
		require.Error(t, err)
		var ve *ValidationError
		require.True(t, errors.As(err, &ve))
		assert.Contains(t, issuePaths(t, err), "/filters/0/frobnicate",
			"the refusal has to name the slot it is about (§12): %v", err)
		assert.Contains(t, err.Error(), `"frobnicate"`)
	})

	t.Run("a sort carrying an unrecognized member is refused with the member named", func(t *testing.T) {
		// given
		raw := json.RawMessage(`[{"property":"name","frobnicate":true}]`)

		// when
		_, err := UnmarshalSorts(raw, Options{})

		// then
		require.Error(t, err)
		var ve *ValidationError
		require.True(t, errors.As(err, &ve))
		require.Len(t, ve.Issues, 1)
		assert.Equal(t, "/sorts/0/frobnicate", ve.Issues[0].Path)
		assert.Contains(t, ve.Issues[0].Message, `"frobnicate"`)
	})

	t.Run("a payload that is not an array is a path-addressed refusal, not a bare decode error", func(t *testing.T) {
		// given
		raw := json.RawMessage(`{"property":"done"}`)

		// when
		_, err := UnmarshalFilters(raw, fragFilterOpts())

		// then
		var ve *ValidationError
		require.True(t, errors.As(err, &ve), "want *ValidationError, got: %v", err)
		assert.Contains(t, issuePaths(t, err), "/filters")
	})

	t.Run("a wrong-typed known member is a path-addressed refusal, not a bare decode error", func(t *testing.T) {
		// given
		raw := json.RawMessage(`[{"property":"name","include_time":"yes"}]`)

		// when
		_, err := UnmarshalSorts(raw, Options{})

		// then
		var ve *ValidationError
		require.True(t, errors.As(err, &ve), "want *ValidationError, got: %v", err)
		assert.Contains(t, issuePaths(t, err), "/sorts/0/include_time")
	})
}

// The two doors must agree: the fragment refusal for an unknown member is the
// document refusal for the identical shape, issue for issue, with only the
// path prefix differing — the fragment validates through the same schema
// fragments, so agreement is by construction and this pins it.
func TestUnmarshalFilters_FragmentAgreesWithDocument(t *testing.T) {
	cases := map[string]struct {
		member string // "filters" or "sorts"
		raw    string
	}{
		"filter leaf with unknown member":  {"filters", `[{"property":"done","condition":"equal","frobnicate":true}]`},
		"filter group with unknown member": {"filters", `[{"operator":"and","filters":[{"property":"done","condition":"equal"}],"frobnicate":true}]`},
		"sort with unknown member":         {"sorts", `[{"property":"name","frobnicate":true}]`},
		"valid filters":                    {"filters", `[{"property":"done","condition":"equal","value":false}]`},
		"valid sorts":                      {"sorts", `[{"property":"name","direction":"desc"}]`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			// given — the same array once through the fragment door, once
			// inside a whole document
			doc := `{"version":2,"type":"page","blocks":[{"type":"dataview","views":[{"` +
				tc.member + `":` + tc.raw + `}]}]}`

			// when
			var fragErr error
			if tc.member == "filters" {
				_, fragErr = UnmarshalFilters(json.RawMessage(tc.raw), fragFilterOpts())
			} else {
				_, fragErr = UnmarshalSorts(json.RawMessage(tc.raw), Options{})
			}
			docErr := Validate([]byte(doc))

			// then
			if docErr == nil {
				assert.NoError(t, fragErr, "document accepts what the fragment refuses")
				return
			}
			require.Error(t, fragErr, "fragment accepts what the document refuses: %v", docErr)
			var fragVe, docVe *ValidationError
			require.True(t, errors.As(fragErr, &fragVe))
			require.True(t, errors.As(docErr, &docVe))
			want := make([]Issue, 0, len(docVe.Issues))
			for _, iss := range docVe.Issues {
				iss.Path = strings.TrimPrefix(iss.Path, "/blocks/0/views/0")
				want = append(want, iss)
			}
			assert.Equal(t, want, fragVe.Issues)
		})
	}
}
