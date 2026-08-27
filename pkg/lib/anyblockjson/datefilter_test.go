package anyblockjson

// `less` on a date matches objects that have no value for it: the filter's
// value is set and the record's is not, so domain.Value.Compare returns 1 —
// precisely what Less tests for (database/filter.go). A freshness view
// written the obvious way therefore lists every never-dated object as
// overdue.

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gogo/protobuf/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func dateFilterDoc(filters string) string {
	return `{"version": 1, "id": "p1", "blocks": [{"type": "dataview",
		"object_id": "someSet",
		"properties": [{"property": "verifiedUntil", "format": "date"},
		               {"property": "status", "format": "select"}],
		"views": [{"name": "Needs review", "filters": [` + filters + `]}]}]}`
}

func warningsFor(t *testing.T, doc string) []Issue {
	t.Helper()
	var got []Issue
	require.NoError(t, ValidateWarn([]byte(doc), func(i Issue) { got = append(got, i) }))
	return got
}

func TestValidate_UnguardedDateLessWarns(t *testing.T) {
	for _, cond := range []string{"less", "less_or_equal"} {
		t.Run(cond, func(t *testing.T) {
			w := warningsFor(t, dateFilterDoc(
				`{"property": "verifiedUntil", "condition": "`+cond+`", "date_preset": "today"}`))
			require.Len(t, w, 1)
			assert.Contains(t, w[0].Message, "no verifiedUntil")
			assert.Contains(t, w[0].Message, "not_empty")
		})
	}
}

func TestValidate_GuardedDateLessIsClean(t *testing.T) {
	t.Run("not_empty sibling in the implicit top-level AND", func(t *testing.T) {
		assert.Empty(t, warningsFor(t, dateFilterDoc(
			`{"property": "verifiedUntil", "condition": "not_empty"},
			 {"property": "verifiedUntil", "condition": "less", "date_preset": "today"}`)))
	})

	t.Run("not_empty in an enclosing and-group", func(t *testing.T) {
		assert.Empty(t, warningsFor(t, dateFilterDoc(
			`{"operator": "and", "filters": [
				{"property": "verifiedUntil", "condition": "not_empty"},
				{"property": "verifiedUntil", "condition": "less", "date_preset": "today"}]}`)))
	})

	// the real shape from the wiki: the guarded pair lives inside an OR branch
	t.Run("and-group nested under an or-group", func(t *testing.T) {
		assert.Empty(t, warningsFor(t, dateFilterDoc(
			`{"operator": "or", "filters": [
				{"operator": "and", "filters": [
					{"property": "verifiedUntil", "condition": "not_empty"},
					{"property": "verifiedUntil", "condition": "less", "date_preset": "today"}]},
				{"property": "status", "condition": "in", "value": ["Needs update"]}]}`)))
	})

	t.Run("exists guards too", func(t *testing.T) {
		assert.Empty(t, warningsFor(t, dateFilterDoc(
			`{"property": "verifiedUntil", "condition": "exists"},
			 {"property": "verifiedUntil", "condition": "less", "date_preset": "today"}`)))
	})
}

// `… OR dueDate IS EMPTY` deliberately INCLUDES the undated objects, so the
// "also matches objects with no X" warning would contradict the filter's own
// text — the canonical worked example (`done = false AND (dueDate <
// currentWeek() OR dueDate IS EMPTY)`) must not warn on every execution.
func TestValidate_EmptySiblingUnderOrSuppressesTheWarning(t *testing.T) {
	t.Run("empty on the same property under the same OR", func(t *testing.T) {
		assert.Empty(t, warningsFor(t, dateFilterDoc(
			`{"operator": "or", "filters": [
				{"property": "verifiedUntil", "condition": "less", "date_preset": "current_week"},
				{"property": "verifiedUntil", "condition": "empty"}]}`)))
	})

	t.Run("empty on a DIFFERENT property does not suppress", func(t *testing.T) {
		w := warningsFor(t, dateFilterDoc(
			`{"operator": "or", "filters": [
				{"property": "verifiedUntil", "condition": "less", "date_preset": "current_week"},
				{"property": "status", "condition": "empty"}]}`))
		require.Len(t, w, 1)
		assert.Contains(t, w[0].Message, "no verifiedUntil")
	})
}

// an OR sibling guarantees nothing — the comparison is reachable without it
func TestValidate_NotEmptyUnderOrDoesNotGuard(t *testing.T) {
	w := warningsFor(t, dateFilterDoc(
		`{"operator": "or", "filters": [
			{"property": "verifiedUntil", "condition": "not_empty"},
			{"property": "verifiedUntil", "condition": "less", "date_preset": "today"}]}`))
	require.Len(t, w, 1)
	assert.Contains(t, w[0].Message, "no verifiedUntil")
}

func TestValidate_DateFilterNonTriggers(t *testing.T) {
	t.Run("greater is unaffected", func(t *testing.T) {
		// an unset value compares as 1, and Greater tests for -1
		assert.Empty(t, warningsFor(t, dateFilterDoc(
			`{"property": "verifiedUntil", "condition": "greater", "date_preset": "today"}`)))
	})

	t.Run("less on a non-date property", func(t *testing.T) {
		assert.Empty(t, warningsFor(t, dateFilterDoc(
			`{"property": "status", "condition": "less", "value": "x"}`)))
	})
}

// numberOfDaysAgo / numberOfDaysNow read their operand from `value`
// (getDateRange calls f.Value.Int64()). With no value the count is 0, so
// "edited in the last 30 days" silently becomes "edited today".
func TestValidate_CountingPresetNeedsValue(t *testing.T) {
	for _, preset := range []string{"number_of_days_ago", "number_of_days_now"} {
		t.Run(preset+" without value", func(t *testing.T) {
			err := Validate([]byte(dateFilterDoc(
				`{"property": "verifiedUntil", "condition": "greater", "date_preset": "` + preset + `"}`)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "needs a day count")
		})
		t.Run(preset+" with value", func(t *testing.T) {
			assert.NoError(t, Validate([]byte(dateFilterDoc(
				`{"property": "verifiedUntil", "condition": "greater", "date_preset": "`+preset+`", "value": 30}`))))
		})
	}

	// a zero count is explicit and legal — it just has to be written down
	t.Run("explicit zero is accepted", func(t *testing.T) {
		assert.NoError(t, Validate([]byte(dateFilterDoc(
			`{"property": "verifiedUntil", "condition": "greater", "date_preset": "number_of_days_ago", "value": 0}`))))
	})

	// fixed-period presets take no operand
	t.Run("fixed presets need no value", func(t *testing.T) {
		for _, preset := range []string{"today", "last_week", "current_month", "next_year"} {
			assert.NoError(t, Validate([]byte(dateFilterDoc(
				`{"property": "verifiedUntil", "condition": "greater", "date_preset": "`+preset+`"}`))), preset)
		}
	})
}

// export must not elide a zero count, or the round trip loses which day the
// filter meant and the document stops validating
func TestRoundtrip_ZeroDayCountSurvives(t *testing.T) {
	for _, days := range []int64{0, 30} {
		snapshot := dataviewSnapshot()
		snapshot.Blocks[1].GetDataview().Views[0].Filters = []*model.BlockContentDataviewFilter{{
			RelationKey: "due",
			Condition:   model.BlockContentDataviewFilter_Greater,
			QuickOption: model.BlockContentDataviewFilter_NumberOfDaysAgo,
			Format:      model.RelationFormat_date,
			Value:       &types.Value{Kind: &types.Value_NumberValue{NumberValue: float64(days)}},
		}}
		data, err := Marshal(model.SmartBlockType_Page, snapshot, testOptions())
		require.NoError(t, err)
		assert.Contains(t, string(data), `"date_preset": "number_of_days_ago"`)
		assert.Contains(t, string(data), `"value": `+fmt.Sprint(days))

		require.NoError(t, Validate(data), "exported document must validate")

		_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		var got *model.BlockContentDataviewFilter
		for _, b := range back.Blocks {
			if dv := b.GetDataview(); dv != nil && len(dv.Views) > 0 && len(dv.Views[0].Filters) > 0 {
				got = dv.Views[0].Filters[0]
			}
		}
		require.NotNil(t, got)
		assert.Equal(t, float64(days), got.Value.GetNumberValue())
	}
}

// The day count is only ever read where the preset's day range is actually
// applied. transformDateFilter computes the range for every date filter but
// substitutes it for six conditions only — equal, in, less, greater,
// less_or_equal, greater_or_equal (pkg/lib/database/quickoptions.go); every
// other condition returns the filter unchanged, so the preset is inert and a
// missing count means nothing at all rather than "today".
//
// This is where the rule collided with export: `value` is dropped on
// presence-only leaves (§11), so a stored filter with `empty` and a counting
// preset marshalled into a document the package's own validation rejected. A
// 36 808-object sweep found two of them.
func TestValidate_CountingPresetOnlyWhereTheRangeApplies(t *testing.T) {
	t.Run("applied: a missing count is still an error", func(t *testing.T) {
		for _, cond := range []string{"equal", "in", "less", "greater", "less_or_equal", "greater_or_equal"} {
			err := Validate([]byte(dateFilterDoc(
				`{"property": "verifiedUntil", "condition": "` + cond + `", "date_preset": "number_of_days_ago"}`)))
			require.Error(t, err, cond)
			assert.Contains(t, err.Error(), "needs a day count", cond)
		}
	})

	t.Run("not applied: the preset is inert, so nothing is missing", func(t *testing.T) {
		// presence-only leaves are the ones export strips the value from
		for _, cond := range []string{"empty", "not_empty", "exists", "not_equal", "not_in"} {
			assert.NoError(t, Validate([]byte(dateFilterDoc(
				`{"property": "verifiedUntil", "condition": "`+cond+`", "date_preset": "number_of_days_ago"}`))), cond)
		}
	})
}

// I1 for the same pair: a stored filter with a presence-only condition and a
// counting preset must marshal into a document Validate accepts.
func TestExport_CountingPresetOnPresenceOnlyLeaf(t *testing.T) {
	for _, cond := range []model.BlockContentDataviewFilterCondition{
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
		model.BlockContentDataviewFilter_Exists,
	} {
		snapshot := dataviewSnapshot()
		snapshot.Blocks[1].GetDataview().Views[0].Filters = []*model.BlockContentDataviewFilter{{
			RelationKey: "due",
			Condition:   cond,
			QuickOption: model.BlockContentDataviewFilter_NumberOfDaysAgo,
			Format:      model.RelationFormat_date,
		}}
		data, err := Marshal(model.SmartBlockType_Page, snapshot, testOptions())
		require.NoError(t, err)
		assert.NoError(t, Validate(data), "Marshal must not emit what Validate rejects (%v):\n%s", cond, data)
	}
}

// bareDateFilterDoc is dateFilterDoc without the properties list — the shape
// a hand-written dataview actually has, where the only thing that says what
// `due_date` is, is the bundled table.
func bareDateFilterDoc(filters string) string {
	return `{"version": 1, "id": "p1", "blocks": [{"type": "dataview",
		"object_id": "someSet",
		"views": [{"name": "Needs review", "filters": [` + filters + `]}]}]}`
}

// The day count is only read on a DATE filter. transformDateFilter returns a
// filter of any other format untouched — before getDateRange is reached at
// all (pkg/lib/database/quickoptions.go) — so a counting preset on a text or
// select property is stored UI state that decides nothing, and the count it
// does not carry is not missing. Demanding one there rejected a document the
// app runs exactly as written.
//
// The fixture reaches the date path through the format and nothing else:
// every case below is the same leaf under the same condition, and only the
// property's resolved format moves. Where it resolves to date the error is
// still there, which is what shows the check ran.
func TestValidate_CountingPresetOnlyOnADateProperty(t *testing.T) {
	const leaf = `{"property": "%s", "condition": "greater", "date_preset": "number_of_days_ago"}`

	t.Run("a declared non-date property is inert", func(t *testing.T) {
		// status is declared `select` two lines above the filter
		assert.NoError(t, Validate([]byte(dateFilterDoc(fmt.Sprintf(leaf, "status")))))
	})

	t.Run("the declaration outranks the bundled table", func(t *testing.T) {
		// the same key the bundle calls a date, declared as text by the
		// block that owns the filter — impDvFormat reads the properties
		// list first, so this filter imports as a text filter
		doc := `{"version": 1, "blocks": [{"type": "dataview",
			"properties": [{"property": "due_date", "format": "text"}],
			"views": [{"filters": [` + fmt.Sprintf(leaf, "due_date") + `]}]}]}`
		assert.NoError(t, Validate([]byte(doc)))
	})

	t.Run("a property the bundle knows as a date still errors", func(t *testing.T) {
		// no properties list at all: `due_date` resolves through the §3
		// chain to dueDate, whose bundled format is date, which is the
		// format import attaches — the rule has to reach that document
		err := Validate([]byte(bareDateFilterDoc(fmt.Sprintf(leaf, "due_date"))))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "needs a day count")
	})

	t.Run("a declared date property still errors", func(t *testing.T) {
		err := Validate([]byte(dateFilterDoc(fmt.Sprintf(leaf, "verifiedUntil"))))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "needs a day count")
	})

	t.Run("an unknown property is not assumed to be a date", func(t *testing.T) {
		// neither declared nor bundled: import gives it format 0, which is
		// not date, so the preset is inert there too
		assert.NoError(t, Validate([]byte(bareDateFilterDoc(fmt.Sprintf(leaf, "whenever")))))
	})
}

// The same gate on the fragment surface (API v2 filters): the format comes
// from the space through Options.ResolveFormat — via the reader's vocabulary,
// the way import resolves the same term — instead of from a properties list.
// ResolveProperties is a different seam and answers nothing here; a fixture
// wired to it leaves the format unresolved and the rule never runs.
func TestUnmarshalFilters_CountingPresetOnlyOnADateProperty(t *testing.T) {
	countingLeaf := func(prop string) json.RawMessage {
		return json.RawMessage(`[{"property":"` + prop + `","condition":"greater","date_preset":"number_of_days_ago"}]`)
	}

	t.Run("a non-date property is inert", func(t *testing.T) {
		_, err := UnmarshalFilters(countingLeaf("status"), fragFilterOpts())
		assert.NoError(t, err)
	})

	t.Run("a date property still errors", func(t *testing.T) {
		_, err := UnmarshalFilters(countingLeaf("dueDate"), fragFilterOpts())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "needs a day count")
	})

	t.Run("the documented slug reaches the same resolver", func(t *testing.T) {
		// `due_date` is what the API surface documents; it resolves to
		// dueDate through the bundled vocabulary before the format is
		// looked up, exactly as importer.filterFromJSON resolves it
		_, err := UnmarshalFilters(countingLeaf("due_date"), fragFilterOpts())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "needs a day count")
	})
}

// A preset under a condition that does not apply is inert: transformDateFilter
// substitutes the range for six conditions and returns every other filter
// unchanged, so the view matches on the condition alone and the preset is UI
// state. That is worth saying — the author wrote "the last week" and got
// something else — but it is a WARNING, because export has to stay lossless:
// stored filters carry these pairs, and refusing them would make an
// unexportable object out of every one that has one (§11, I1).
func TestValidate_InertPresetWarns(t *testing.T) {
	for _, cond := range []string{"not_equal", "not_in", "empty", "not_empty", "exists", "contains"} {
		t.Run(cond, func(t *testing.T) {
			doc := dateFilterDoc(
				`{"property": "verifiedUntil", "condition": "` + cond + `", "date_preset": "current_week"}`)
			w := warningsFor(t, doc) // warningsFor requires Validate to pass
			require.Len(t, w, 1)
			assert.Equal(t, "/blocks/0/views/0/filters/0", w[0].Path)
			assert.Contains(t, w[0].Message, `date_preset "current_week" is ignored`)
			assert.Contains(t, w[0].Message, cond)
			assert.Contains(t, w[0].Message, "greater_or_equal", "the message names the conditions that do apply")
		})
	}

	t.Run("a leaf with no condition", func(t *testing.T) {
		// an absent condition is proto None, which substitutes nothing either
		w := warningsFor(t, dateFilterDoc(`{"property": "verifiedUntil", "date_preset": "today"}`))
		require.Len(t, w, 1)
		assert.Contains(t, w[0].Message, "a leaf with no condition")
	})

	t.Run("the six that apply say nothing", func(t *testing.T) {
		for _, cond := range []string{"equal", "in", "greater", "greater_or_equal"} {
			assert.Empty(t, warningsFor(t, dateFilterDoc(
				`{"property": "verifiedUntil", "condition": "`+cond+`", "date_preset": "current_week"}`)), cond)
		}
		// less and less_or_equal apply too; their one warning is the
		// unguarded-comparison trap, not this rule
		for _, cond := range []string{"less", "less_or_equal"} {
			w := warningsFor(t, dateFilterDoc(
				`{"property": "verifiedUntil", "condition": "`+cond+`", "date_preset": "current_week"}`))
			require.Len(t, w, 1, cond)
			assert.NotContains(t, w[0].Message, "is ignored", cond)
		}
	})

	t.Run("the document still imports, preset and all", func(t *testing.T) {
		doc := dateFilterDoc(`{"property": "verifiedUntil", "condition": "empty", "date_preset": "current_week"}`)
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		var got *model.BlockContentDataviewFilter
		for _, b := range snap.Blocks {
			if dv := b.GetDataview(); dv != nil && len(dv.Views) > 0 && len(dv.Views[0].Filters) > 0 {
				got = dv.Views[0].Filters[0]
			}
		}
		require.NotNil(t, got)
		assert.Equal(t, model.BlockContentDataviewFilter_CurrentWeek, got.QuickOption,
			"a warning must not cost the document the field it warned about")
	})
}

// I1 for the same pairing, from the export side: Marshal writes a preset
// beside a presence-only condition because the stored filter has one, so the
// verdict on its own output may be a warning and may never be a refusal.
func TestExport_InertPresetWarnsButNeverRejects(t *testing.T) {
	for _, cond := range []model.BlockContentDataviewFilterCondition{
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
		model.BlockContentDataviewFilter_Exists,
		model.BlockContentDataviewFilter_NotEqual,
	} {
		snapshot := dataviewSnapshot()
		snapshot.Blocks[1].GetDataview().Views[0].Filters = []*model.BlockContentDataviewFilter{{
			RelationKey: "due",
			Condition:   cond,
			QuickOption: model.BlockContentDataviewFilter_CurrentWeek,
			Format:      model.RelationFormat_date,
		}}
		data, err := Marshal(model.SmartBlockType_Page, snapshot, testOptions())
		require.NoError(t, err)
		require.Contains(t, string(data), `"date_preset": "current_week"`, "export keeps the preset")

		var warnings []Issue
		require.NoError(t, ValidateWarn(data, func(i Issue) { warnings = append(warnings, i) }),
			"Marshal must not emit what Validate rejects (%v):\n%s", cond, data)
		require.Len(t, warnings, 1, "%v: %v", cond, warnings)
		assert.Contains(t, warnings[0].Message, "is ignored")
	}
}

// The day-count rule reads the OPERAND, not the member. It used to check only
// that "value" was present, so `"value": null` — and a string, and a boolean —
// passed both Validate and Unmarshal (no I2 break: both accepted) while the
// engine read the very 0 the message warns about: domain.Value.Int64 answers
// 0 for every kind that is not a number. The bound is the one the compact
// grammar already applies to daysAgo(n) (§6.2.1); two forms of one filter
// language admit the same filters.
func TestValidate_CountingPresetOperandIsADayCount(t *testing.T) {
	leaf := func(value string) string {
		return dateFilterDoc(`{"property": "verifiedUntil", "condition": "greater",
			"date_preset": "number_of_days_ago", "value": ` + value + `}`)
	}

	t.Run("refused", func(t *testing.T) {
		for name, tc := range map[string]struct{ value, says string }{
			"null":           {"null", "null counts as 0 days"},
			"a string":       {`"30"`, "a string counts as 0 days"},
			"a boolean":      {"true", "a boolean counts as 0 days"},
			"an array":       {"[30]", "an array counts as 0 days"},
			"a fraction":     {"3.5", "3.5 is not a whole day count"},
			"a negative":     {"-1", "-1 is not a whole day count"},
			"past the bound": {"36501", "36501 is not a whole day count"},
		} {
			t.Run(name, func(t *testing.T) {
				// when
				err := Validate([]byte(leaf(tc.value)))

				// then
				require.Error(t, err, "an operand the engine reads as 0 is the trap the message describes")
				var ve *ValidationError
				require.ErrorAs(t, err, &ve)
				require.Len(t, ve.Issues, 1)
				assert.Equal(t, "/blocks/0/views/0/filters/0/value", ve.Issues[0].Path,
					"the fault is the operand, not the leaf that carries it (§13)")
				assert.Contains(t, ve.Issues[0].Message, tc.says)

				// and I2: Unmarshal reaches the same verdict
				_, _, err = Unmarshal([]byte(leaf(tc.value)), Options{GenerateId: seqIds("g")})
				require.Error(t, err)
				assert.Equal(t, ve.Issues, err.(*ValidationError).Issues)
			})
		}
	})

	// a number no float64 can hold is refused, once: checkNumbers owns that
	// fault and addresses the same pointer, and one fault is one issue (§12)
	t.Run("a number out of float64 range is somebody else's issue", func(t *testing.T) {
		err := Validate([]byte(leaf("1e400")))
		var ve *ValidationError
		require.ErrorAs(t, err, &ve)
		require.Len(t, ve.Issues, 1)
		assert.Equal(t, "/blocks/0/views/0/filters/0/value", ve.Issues[0].Path)
		assert.Contains(t, ve.Issues[0].Message, "out of range: values must fit a 64-bit float")
	})

	t.Run("accepted", func(t *testing.T) {
		for _, value := range []string{"0", "1", "30", "36500"} {
			t.Run(value, func(t *testing.T) {
				require.NoError(t, Validate([]byte(leaf(value))))
				_, _, err := Unmarshal([]byte(leaf(value)), Options{GenerateId: seqIds("g")})
				require.NoError(t, err)
			})
		}
	})

	// the gate is unchanged: an operand nothing reads is not a fault
	t.Run("still inert where the preset is", func(t *testing.T) {
		assert.NoError(t, Validate([]byte(dateFilterDoc(
			`{"property": "status", "condition": "greater",
			  "date_preset": "number_of_days_ago", "value": null}`))),
			"a non-date property never reaches the preset's range")
	})
}

// Export's half of the same rule: a stored operand that is not a day count
// has no written form, so it may not travel verbatim — that document is one
// this package's own Validate refuses (§11, I1). It is written as the count
// the query engine reads out of it, and the caller is told.
func TestExport_CountingPresetOperandIsWritableOrReported(t *testing.T) {
	marshal := func(t *testing.T, value *types.Value) (string, []Issue) {
		t.Helper()
		var warned []Issue
		root := &model.Block{Id: "o1", ChildrenIds: []string{"dv"},
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}
		snap := &model.SmartBlockSnapshotBase{Blocks: []*model.Block{root, {Id: "dv",
			Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				RelationLinks: []*model.RelationLink{{Key: "dueDate", Format: model.RelationFormat_date}},
				Views: []*model.BlockContentDataviewView{{Id: "v1",
					Filters: []*model.BlockContentDataviewFilter{{
						RelationKey: "dueDate",
						Condition:   model.BlockContentDataviewFilter_Greater,
						QuickOption: model.BlockContentDataviewFilter_NumberOfDaysAgo,
						Format:      model.RelationFormat_date,
						Value:       value,
					}}}},
			}}}}}
		data, err := Marshal(model.SmartBlockType_Page, snap,
			Options{OnWarning: func(i Issue) { warned = append(warned, i) }})
		require.NoError(t, err)
		require.NoError(t, Validate(data), "Marshal may never emit what its own Validate refuses:\n%s", data)
		return string(data), warned
	}

	t.Run("a whole count in range travels untouched", func(t *testing.T) {
		data, warned := marshal(t, &types.Value{Kind: &types.Value_NumberValue{NumberValue: 30}})
		assert.Contains(t, data, `"value": 30`)
		assert.Empty(t, warned)
	})

	t.Run("no operand is the stored shape of today, and stays quiet", func(t *testing.T) {
		data, warned := marshal(t, nil)
		assert.Contains(t, data, `"value": 0`)
		assert.Empty(t, warned)
	})

	t.Run("a string operand is written as the count the engine reads", func(t *testing.T) {
		data, warned := marshal(t, str("a week"))
		assert.Contains(t, data, `"value": 0`)
		require.Len(t, warned, 1)
		assert.Contains(t, warned[0].Message, `carries a week as its day count`)
		assert.Contains(t, warned[0].Message, "0 is written instead")
	})

	t.Run("a count past the bound is pinned to it and reported", func(t *testing.T) {
		data, warned := marshal(t, &types.Value{Kind: &types.Value_NumberValue{NumberValue: 40000}})
		assert.Contains(t, data, `"value": 36500`)
		require.Len(t, warned, 1)
		assert.Contains(t, warned[0].Message, "36500 is written instead")
	})

	t.Run("a fraction is truncated the way the engine truncates it", func(t *testing.T) {
		data, warned := marshal(t, &types.Value{Kind: &types.Value_NumberValue{NumberValue: 3.7}})
		assert.Contains(t, data, `"value": 3`)
		require.Len(t, warned, 1)
	})
}
