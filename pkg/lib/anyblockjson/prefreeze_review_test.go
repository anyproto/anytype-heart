package anyblockjson

// Regression tests for the confirmed findings of the pre-freeze review
// (PREFREEZE_REVIEW.md, Tier 1). The two property tests the same review asks
// for live in flat_invariants_test.go — these are the individual instances,
// hand-written so the fixture can express the failure.

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// dateOptions resolves customDate as a date property, which is what makes the
// date export path run at all — a fixture without it never reaches the code
// under test.
func dateOptions(onWarning func(Issue)) Options {
	return Options{
		ResolveFormat: testFormatResolver,
		GenerateId:    seqIds("g"),
		OnWarning:     onWarning,
	}
}

func dateSnapshot(sec float64) *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{Id: "obj1",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		Details: fields(map[string]*types.Value{
			"id":         str("obj1"),
			"customDate": {Kind: &types.Value_NumberValue{NumberValue: sec}},
		}),
	}
}

// Tier 1 #4. `customDate = 1751791445000` is milliseconds stored where seconds
// belong — a real corruption class, and one this format inherits rather than
// creates. It formatted as "57482-01-22T22:43:20Z", which parseDate cannot
// read back, so re-import stored a *string* on a date-format property: the
// value stopped being a date, permanently and quietly, and byte-stably
// thereafter, so nothing ever corrects it.
func TestExport_DateOutsideRFC3339RangeKeepsTheNumber(t *testing.T) {
	for _, sec := range []float64{1751791445000, -62167219201, 253402300800} {
		t.Run(fmt.Sprintf("%.0f", sec), func(t *testing.T) {
			var warnings []Issue
			data, err := Marshal(model.SmartBlockType_Page, dateSnapshot(sec),
				dateOptions(func(i Issue) { warnings = append(warnings, i) }))
			require.NoError(t, err)
			require.NoError(t, Validate(data))

			var doc struct {
				Properties map[string]any `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(data, &doc))
			assert.IsType(t, float64(0), doc.Properties["customDate"],
				"an unrepresentable date stays a number rather than becoming an unparseable string")

			_, back, err := Unmarshal(data, dateOptions(nil))
			require.NoError(t, err)
			got := back.Details.Fields["customDate"]
			require.NotNil(t, got)
			_, isNumber := got.GetKind().(*types.Value_NumberValue)
			require.True(t, isNumber, "the value must survive as a number, got %v", got)
			assert.Equal(t, sec, got.GetNumberValue())
			assert.NotEmpty(t, warnings, "a date this far out is worth saying out loud")
		})
	}
}

// The in-range behaviour is unchanged: a date property is an RFC 3339 string.
func TestExport_DateInsideRangeIsStillAString(t *testing.T) {
	data, err := Marshal(model.SmartBlockType_Page, dateSnapshot(1751791445), dateOptions(nil))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"customDate": "2025-07-06T08:44:05Z"`)

	_, back, err := Unmarshal(data, dateOptions(nil))
	require.NoError(t, err)
	assert.Equal(t, float64(1751791445), back.Details.Fields["customDate"].GetNumberValue())
}

// The representable range is defined by what parses back, not by taste: any
// other definition would let export write a string parseDate cannot read.
func TestFormatDate_RangeIsExactlyWhatParsesBack(t *testing.T) {
	for _, sec := range []int64{minDateSec, maxDateSec, 0, 1751791445, -1} {
		s, ok := formatDate(sec)
		require.True(t, ok, "%d must be representable", sec)
		back, parsed := parseDate(s)
		require.True(t, parsed, "%d rendered as %q, which does not parse", sec, s)
		assert.Equal(t, sec, back, "%d rendered as %q, which parses as %d", sec, s, back)
	}
	for _, sec := range []int64{minDateSec - 1, maxDateSec + 1, 1751791445000} {
		s, ok := formatDate(sec)
		assert.False(t, ok, "%d must be out of range, got %q", sec, s)
	}
}

// A file block's addedAt is a schema string with no number form to fall back
// to, so an unrepresentable timestamp is dropped with a warning rather than
// written as a string no reader can parse back.
func TestExport_FileAddedAtOutsideRangeIsOmitted(t *testing.T) {
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "obj1", ChildrenIds: []string{"f1"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "f1", Content: &model.BlockContentOfFile{File: &model.BlockContentFile{
				TargetObjectId: "file1", Name: "doc.pdf", AddedAt: 1751791445000,
			}}},
		},
		Details: fields(map[string]*types.Value{"id": str("obj1")}),
	}
	var warnings []Issue
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{
		OnWarning: func(i Issue) { warnings = append(warnings, i) }})
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	assert.NotContains(t, string(data), "addedAt")
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].Message, "addedAt")
}
