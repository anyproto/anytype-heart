package anyblockjson

// Regression tests for the confirmed findings of the pre-freeze review
// (PREFREEZE_REVIEW.md, Tier 1). The two property tests the same review asks
// for live in flat_invariants_test.go — these are the individual instances,
// hand-written so the fixture can express the failure.

import (
	"encoding/json"
	"fmt"
	"strings"
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

// ---- Tier 1 #1: one id domain ----
//
// The rule was known and written down (table.go: "Emitting one verbatim would
// make Marshal write a document its own Validate rejects, so normalize it once
// here") and then applied to table inner ids only. Every other id surface
// skipped it, in six confirmed ways. Two of them lose data.

// assertUniqueBlockIds is the snapshot-side half of the id invariant: whatever
// import mints, no two blocks may end up with the same id.
func assertUniqueBlockIds(t *testing.T, snap *model.SmartBlockSnapshotBase) {
	t.Helper()
	seen := map[string]bool{}
	for _, b := range snap.Blocks {
		require.False(t, seen[b.Id], "duplicate block id %q in the rebuilt snapshot", b.Id)
		seen[b.Id] = true
	}
}

// (a) a stored block id outside the schema's charset was written verbatim.
func TestExport_BlockIdOutsideCharsetIsSanitized(t *testing.T) {
	for _, stored := range []string{"a.b", "dir/file", "блок", strings.Repeat("x", 65)} {
		t.Run(stored, func(t *testing.T) {
			snap := &model.SmartBlockSnapshotBase{
				Blocks: []*model.Block{
					{Id: "obj1", ChildrenIds: []string{stored},
						Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
					{Id: stored, Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{Text: "hi"}}},
				},
				Details: fields(map[string]*types.Value{"id": str("obj1")}),
			}
			data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
			require.NoError(t, err)
			require.NoError(t, Validate(data), "Marshal must not emit what Validate rejects:\n%s", data)
		})
	}
}

// (c) a sanitized column id could land on a sibling paragraph's id, because
// the used-id set covered table inner ids only.
func TestExport_SanitizedColumnIdCannotTakeASiblingsId(t *testing.T) {
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "obj1", ChildrenIds: []string{"c_1", "t1"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "c_1", Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: "a paragraph that got there first"}}},
			{Id: "t1", ChildrenIds: []string{"cols", "rows"},
				Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}},
			{Id: "cols", ChildrenIds: []string{"c-1"},
				Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{
					Style: model.BlockContentLayout_TableColumns}}},
			{Id: "c-1", Content: &model.BlockContentOfTableColumn{
				TableColumn: &model.BlockContentTableColumn{}}},
			{Id: "rows", ChildrenIds: []string{"r1"},
				Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{
					Style: model.BlockContentLayout_TableRows}}},
			{Id: "r1", Content: &model.BlockContentOfTableRow{
				TableRow: &model.BlockContentTableRow{}}},
		},
		Details: fields(map[string]*types.Value{"id": str("obj1")}),
	}
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
	require.NoError(t, err)
	require.NoError(t, Validate(data), "duplicate id across two surfaces:\n%s", data)
	assert.Contains(t, string(data), "a paragraph that got there first",
		"the paragraph keeps its own id and its content")
}

// (d) under CompactIds a suffix label could equal a short id that labels as
// itself. The refs path already checked for this; the local path did not.
func TestExport_CompactLabelCannotTakeAShortBlockId(t *testing.T) {
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "obj1", ChildrenIds: []string{"block_12345", "12345"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "block_12345", Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: "long"}}},
			{Id: "12345", Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{Text: "short"}}},
		},
		Details: fields(map[string]*types.Value{"id": str("obj1")}),
	}
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{CompactIds: true})
	require.NoError(t, err)
	require.NoError(t, Validate(data), "duplicate id from a suffix label:\n%s", data)
}

// (b) a derived cell id belongs to the table whether or not the cell is
// written: the editor materializes missing cells on open, and the id it uses
// is rowId-colId. So a block claiming one is a duplicate, and validation is
// the only place that can say so.
func TestValidate_DerivedCellIdIsClaimedEvenWhenTheCellIsAbsent(t *testing.T) {
	// the trailing cell is absent from the array, so nothing in the document
	// mentions r1-c2 — an explicit null would already have been claimed
	doc := `{"version": 1, "blocks": [
		{"type": "paragraph", "id": "r1-c2", "text": "x"},
		{"type": "table",
		 "columns": [{"id": "c1"}, {"id": "c2"}],
		 "rows": [{"id": "r1", "cells": ["first"]}]}]}`
	err := Validate([]byte(doc))
	require.Error(t, err, "r1-c2 is the id the table will use when that cell is filled")
	assert.Contains(t, err.Error(), "duplicate id")

	// and the claim is not over-eager: no table, no derived ids
	require.NoError(t, Validate([]byte(`{"version": 1, "blocks": [
		{"type": "paragraph", "id": "r1-c2", "text": "x"}]}`)))
}

// (e) pinPrimaryDataview scanned top-level block ids only, so an authored
// table row named "dataview" was invisible to it — and it minted the same id
// for the dataview block *after* validation had passed. Re-export then lost
// the whole table body.
func TestImport_PrimaryDataviewDoesNotCollideWithATableRowId(t *testing.T) {
	doc := `{"version": 1, "id": "obj1", "blocks": [
		{"type": "table",
		 "columns": [{"id": "c1"}],
		 "rows": [{"id": "dataview", "cells": ["cell text"]}]},
		{"type": "dataview", "views": [{"id": "v1", "name": "All"}]}]}`
	require.NoError(t, Validate([]byte(doc)))
	sbType, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assertUniqueBlockIds(t, snap)

	out, err := Marshal(sbType, snap, Options{})
	require.NoError(t, err)
	require.NoError(t, Validate(out))
	assert.Contains(t, string(out), "cell text", "the table body must survive the round trip")
}

// (f) Options.GenerateId is the caller's, and the convert wiring derives ids
// from file paths — both halves author-controlled. genId never checked the ids
// the document itself already used.
func TestImport_GeneratedIdCannotTakeAnAuthoredId(t *testing.T) {
	doc := `{"version": 1, "blocks": [
		{"type": "paragraph", "id": "g1", "text": "authored"},
		{"type": "paragraph", "text": "needs an id"},
		{"type": "table", "columns": [{"id": "g2"}], "rows": [{"cells": ["c"]}]}]}`
	require.NoError(t, Validate([]byte(doc)))
	_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assertUniqueBlockIds(t, snap)
}
