package anyblockjson

// `less` on a date matches objects that have no value for it: the filter's
// value is set and the record's is not, so domain.Value.Compare returns 1 —
// precisely what Less tests for (database/filter.go). A freshness view
// written the obvious way therefore lists every never-dated object as
// overdue.

import (
	"fmt"
	"testing"

	"github.com/gogo/protobuf/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func dateFilterDoc(filters string) string {
	return `{"version": 1, "id": "p1", "blocks": [{"type": "dataview",
		"objectId": "someSet",
		"properties": [{"key": "verifiedUntil", "format": "date"},
		               {"key": "status", "format": "select"}],
		"views": [{"name": "Needs review", "filters": [` + filters + `]}]}]}`
}

func warningsFor(t *testing.T, doc string) []Issue {
	t.Helper()
	var got []Issue
	require.NoError(t, ValidateWarn([]byte(doc), func(i Issue) { got = append(got, i) }))
	return got
}

func TestValidate_UnguardedDateLessWarns(t *testing.T) {
	for _, cond := range []string{"less", "lessOrEqual"} {
		t.Run(cond, func(t *testing.T) {
			w := warningsFor(t, dateFilterDoc(
				`{"property": "verifiedUntil", "condition": "`+cond+`", "datePreset": "today"}`))
			require.Len(t, w, 1)
			assert.Contains(t, w[0].Message, "no verifiedUntil")
			assert.Contains(t, w[0].Message, "notEmpty")
		})
	}
}

func TestValidate_GuardedDateLessIsClean(t *testing.T) {
	t.Run("notEmpty sibling in the implicit top-level AND", func(t *testing.T) {
		assert.Empty(t, warningsFor(t, dateFilterDoc(
			`{"property": "verifiedUntil", "condition": "notEmpty"},
			 {"property": "verifiedUntil", "condition": "less", "datePreset": "today"}`)))
	})

	t.Run("notEmpty in an enclosing and-group", func(t *testing.T) {
		assert.Empty(t, warningsFor(t, dateFilterDoc(
			`{"operator": "and", "filters": [
				{"property": "verifiedUntil", "condition": "notEmpty"},
				{"property": "verifiedUntil", "condition": "less", "datePreset": "today"}]}`)))
	})

	// the real shape from the wiki: the guarded pair lives inside an OR branch
	t.Run("and-group nested under an or-group", func(t *testing.T) {
		assert.Empty(t, warningsFor(t, dateFilterDoc(
			`{"operator": "or", "filters": [
				{"operator": "and", "filters": [
					{"property": "verifiedUntil", "condition": "notEmpty"},
					{"property": "verifiedUntil", "condition": "less", "datePreset": "today"}]},
				{"property": "status", "condition": "in", "value": ["Needs update"]}]}`)))
	})

	t.Run("exists guards too", func(t *testing.T) {
		assert.Empty(t, warningsFor(t, dateFilterDoc(
			`{"property": "verifiedUntil", "condition": "exists"},
			 {"property": "verifiedUntil", "condition": "less", "datePreset": "today"}`)))
	})
}

// an OR sibling guarantees nothing — the comparison is reachable without it
func TestValidate_NotEmptyUnderOrDoesNotGuard(t *testing.T) {
	w := warningsFor(t, dateFilterDoc(
		`{"operator": "or", "filters": [
			{"property": "verifiedUntil", "condition": "notEmpty"},
			{"property": "verifiedUntil", "condition": "less", "datePreset": "today"}]}`))
	require.Len(t, w, 1)
	assert.Contains(t, w[0].Message, "no verifiedUntil")
}

func TestValidate_DateFilterNonTriggers(t *testing.T) {
	t.Run("greater is unaffected", func(t *testing.T) {
		// an unset value compares as 1, and Greater tests for -1
		assert.Empty(t, warningsFor(t, dateFilterDoc(
			`{"property": "verifiedUntil", "condition": "greater", "datePreset": "today"}`)))
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
	for _, preset := range []string{"numberOfDaysAgo", "numberOfDaysNow"} {
		t.Run(preset+" without value", func(t *testing.T) {
			err := Validate([]byte(dateFilterDoc(
				`{"property": "verifiedUntil", "condition": "greater", "datePreset": "` + preset + `"}`)))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "needs a day count")
		})
		t.Run(preset+" with value", func(t *testing.T) {
			assert.NoError(t, Validate([]byte(dateFilterDoc(
				`{"property": "verifiedUntil", "condition": "greater", "datePreset": "`+preset+`", "value": 30}`))))
		})
	}

	// a zero count is explicit and legal — it just has to be written down
	t.Run("explicit zero is accepted", func(t *testing.T) {
		assert.NoError(t, Validate([]byte(dateFilterDoc(
			`{"property": "verifiedUntil", "condition": "greater", "datePreset": "numberOfDaysAgo", "value": 0}`))))
	})

	// fixed-period presets take no operand
	t.Run("fixed presets need no value", func(t *testing.T) {
		for _, preset := range []string{"today", "lastWeek", "currentMonth", "nextYear"} {
			assert.NoError(t, Validate([]byte(dateFilterDoc(
				`{"property": "verifiedUntil", "condition": "greater", "datePreset": "`+preset+`"}`))), preset)
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
		assert.Contains(t, string(data), `"datePreset": "numberOfDaysAgo"`)
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
