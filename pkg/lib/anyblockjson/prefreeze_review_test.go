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

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
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
	assert.NotContains(t, string(data), "added_at")
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].Message, "added_at")
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

// (d) under CompactBlockLabels a suffix label must not take an id the document
// already serves verbatim, or two things answer to one name.
//
// The finding was raised against the refs labeller, which is deleted (§9a).
// What survives it is the BLOCK half, and there the rule is live for exactly
// one reason: `mintedSuffixLabels`' census counts LOCAL ids only. A block id
// cannot alias another block id whether or not the avoid-set is consulted —
// the second subtest states that, since it is what is actually in force — but
// an OBJECT id is invisible to that census, and every object id is now spelled
// verbatim in the document, so the `fullIds` avoid-set is the only thing
// standing between a minted block and a label that already names something
// else. Nothing downstream reports the result: the document is valid, it just
// has two meanings for one string.
func TestExport_CompactLabelCannotTakeAServedId(t *testing.T) {
	const (
		shortObject = "abcde"                    // compactIdMinLen wide: spelled verbatim
		mintedBlock = "0000000000000000000abcde" // whose suffix is that same string
	)

	t.Run("a block label cannot take a short OBJECT id", func(t *testing.T) {
		// the block half against the OTHER id population, which is the half
		// its own census cannot see: mintedSuffixLabels counts local ids only,
		// so a 5-char object id spelled verbatim in the document is invisible
		// to it and only the fullIds avoid-set stands between the minted block
		// and a label that already names something else in the same document.
		snap := &model.SmartBlockSnapshotBase{
			Blocks: []*model.Block{
				{Id: "obj1", ChildrenIds: []string{mintedBlock},
					Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
				mentionBlock(mintedBlock, shortObject),
			},
			Details: fields(map[string]*types.Value{"id": str("obj1")}),
		}
		data, err := Marshal(model.SmartBlockType_Page, snap, Options{CompactBlockLabels: true})
		require.NoError(t, err)
		require.NoError(t, Validate(data), "%s", data)

		var doc struct {
			Blocks []struct {
				Id   string `json:"id"`
				Text string `json:"text"`
			} `json:"blocks"`
		}
		require.NoError(t, json.Unmarshal(data, &doc))
		require.Len(t, doc.Blocks, 1)
		assert.NotEqual(t, shortObject, doc.Blocks[0].Id,
			"the block must not label itself with an object id the document serves verbatim:\n%s", data)
		assert.Equal(t, mintedBlock, doc.Blocks[0].Id,
			"and with that label refused it stays full")
		assert.Contains(t, doc.Blocks[0].Text, shortObject,
			"the fixture only bites while the object id is spelled in the document")
	})

	t.Run("a block label cannot take a short block id", func(t *testing.T) {
		// the block half of the same rule under the shape rule that now
		// governs it. Unlike the refs half above, removing the avoid-set does
		// NOT break this: the census already covers every local id, and the
		// avoid-set is defence in depth over ids from the other population.
		assert.Empty(t, mintedSuffixLabels([]string{mintedBlock, shortObject}, compactIdMinLen, nil),
			"the census over every local id refuses the suffix")
		assert.NotEmpty(t, mintedSuffixLabels([]string{mintedBlock}, compactIdMinLen, nil),
			"and it is the collision that refuses it, not the shape")
	})
}

func mentionBlock(id, target string) *model.Block {
	return &model.Block{Id: id, Content: &model.BlockContentOfText{
		Text: &model.BlockContentText{Text: "x", Marks: &model.BlockContentTextMarks{
			Marks: []*model.BlockContentTextMark{{
				Range: &model.Range{From: 0, To: 1},
				Type:  model.BlockContentTextMark_Mention, Param: target}}}}}}
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

// ---- Tier 1 #5: property-key admission control ----
//
// §4a claims the import surface "treats supplied values as authoritative only
// where semantically safe". Nothing implemented that clause: import copied
// every supplied key onto details, skipping only id/type, so the input surface
// was strictly *wider* than the output surface — export strips
// bundle.LocalAndDerivedRelationKeys, import took them. Among them are the keys
// that decide which existing object a snapshot merges into.

// The rule, stated once: import refuses exactly what export strips, except for
// the keys it DROPS. Deriving all of it from strippedDetailKeys is what keeps
// the two surfaces from drifting apart again — a second hand-written list
// would.
//
// The exception is narrow and deliberate (§3), and covers two families. A
// TRANSIENT key describes the moment an object was written rather than the
// object; an ATTRIBUTION key names the member who wrote it, and is recovered
// from the tree root's signature on every rebuild whatever a document says.
// Either way a document carrying one is stale, not hostile, and refusing it
// would make an old export unimportable to no purpose. Everything else on the
// stripped list is either derived state or a merge-resolution vector, and
// those stay errors. The two exempt halves are asserted in
// TestTransientProperties_DroppedNotRefused and
// TestAttributionProperties_DroppedNotRefused.
func TestValidate_ImportRefusesWhatExportStrips(t *testing.T) {
	refused := 0
	for key := range strippedDetailKeys() {
		if isDroppedOnImport(key) {
			continue
		}
		refused++
		doc := fmt.Sprintf(`{"version": 1, "id": "obj1", "properties": {%q: "x"}}`, key)
		err := Validate([]byte(doc))
		require.Error(t, err, "%s is stripped on export, so it must be refused on import", key)
		assert.Contains(t, err.Error(), "/properties/"+key)
	}
	require.NotZero(t, refused, "every stripped key became droppable — the deny rule went dead")
}

// The named resolution vectors matter most: existingobject.go resolves which
// object in the victim's space a snapshot merges into from oldAnytypeID,
// uniqueKey and sourceFilePath. All three ARE bundled relations — what makes
// two of them need naming by hand is that bundle.LocalAndDerivedRelationKeys,
// the list the deny-rule derives from, does not carry them.
func TestValidate_ResolutionVectorPropertiesRefused(t *testing.T) {
	for _, key := range []string{"oldAnytypeID", "uniqueKey", "sourceFilePath"} {
		doc := fmt.Sprintf(`{"version": 1, "id": "obj1", "properties": {%q: "x"}}`, key)
		err := Validate([]byte(doc))
		require.Error(t, err, key)
		assert.Contains(t, err.Error(), "/properties/"+key)
	}

	// the comment above, pinned — the claim that these are not bundled
	// relations survived in three places, and neverWritableProperties exists
	// only because of the second half of it
	onList := map[string]bool{}
	for _, k := range bundle.LocalAndDerivedRelationKeys {
		onList[string(k)] = true
	}
	for key, wantOnList := range map[string]bool{
		"oldAnytypeID": false, "sourceFilePath": false, "uniqueKey": true,
	} {
		assert.True(t, bundle.HasRelation(domain.RelationKey(key)),
			"%s is a bundled relation", key)
		assert.Equal(t, wantOnList, onList[key],
			"%s in bundle.LocalAndDerivedRelationKeys", key)
	}
}

// The six §3 exemptions are the whole point of the exemption list: they are
// internal keys the importer meaningfully preserves, so they stay writable.
func TestValidate_ExemptedInternalPropertiesStillAccepted(t *testing.T) {
	doc := `{"version": 1, "id": "obj1", "properties": {
		"createdDate": "2026-07-06T08:44:05Z", "lastModifiedDate": "2026-07-06T08:44:05Z",
		"creator": "bafyparticipant", "isFavorite": true, "isArchived": false,
		"resolvedLayout": "basic", "name": "N"}}`
	require.NoError(t, Validate([]byte(doc)))
}

// A property key that is not a key at all — empty, or carrying a control
// character — landed on details verbatim and was written back out.
func TestValidate_PropertyKeyShape(t *testing.T) {
	t.Run("refused", func(t *testing.T) {
		for _, doc := range []string{
			`{"version": 1, "properties": {"": "empty"}}`,
			`{"version": 1, "properties": {"a\nb": "newline"}}`,
			"{\"version\": 1, \"properties\": {\"a\\u0000b\": \"nul\"}}",
			"{\"version\": 1, \"properties\": {\"a\\u007fb\": \"del\"}}",
		} {
			assert.Error(t, Validate([]byte(doc)), doc)
		}
	})
	t.Run("accepted", func(t *testing.T) {
		// the shapes real keys have: bundled lowerCamel, a bson-hex custom key,
		// and the bare names old accounts carry (ANOMALIES §7). The pattern is
		// deliberately a deny rule rather than an allowlist — an allowlist
		// would have to be verified against every key in every account before
		// export could depend on it.
		doc := `{"version": 1, "properties": {
			"dueDate": null, "68f0d9c3b3c8a94e0d0b0a12": "x", "artist": "y"}}`
		require.NoError(t, Validate([]byte(doc)))
	})
}

// A value whose shape contradicts its property's format is not stored as
// written: it reads as the format's zero forever. "next Friday" on a date is
// the case the review names. The fixtures spell the CANONICAL snake_case slug
// (§3) — this test used to pass only because it spelled the stored key, which
// made its subject dead code for every document the format itself produces.
func TestValidate_PropertyValueShapeWarns(t *testing.T) {
	warningsFor := func(t *testing.T, doc string) []Issue {
		var got []Issue
		require.NoError(t, ValidateWarn([]byte(doc), func(i Issue) { got = append(got, i) }), doc)
		return got
	}

	t.Run("a date that is not a date", func(t *testing.T) {
		got := warningsFor(t, `{"version": 1, "id": "o1", "properties": {"due_date": "next Friday"}}`)
		require.Len(t, got, 1)
		assert.Equal(t, "/properties/due_date", got[0].Path)
		assert.Contains(t, got[0].Message, "date")
	})

	t.Run("the stored spelling is an address too", func(t *testing.T) {
		// §3 chain step 1: a spelling the table does not know binds verbatim,
		// so the stored key keeps warning alongside the canonical slug
		got := warningsFor(t, `{"version": 1, "id": "o1", "properties": {"dueDate": "next Friday"}}`)
		require.Len(t, got, 1)
		assert.Equal(t, "/properties/dueDate", got[0].Path)
		assert.Contains(t, got[0].Message, "date")
	})

	t.Run("a spelling the legend binds is checked as what it resolves to", func(t *testing.T) {
		got := warningsFor(t, `{"version": 1, "id": "o1",
			"property_internal_keys": {"prio": "dueDate"}, "properties": {"prio": "next Friday"}}`)
		require.Len(t, got, 1)
		assert.Equal(t, "/properties/prio", got[0].Path)
		assert.Contains(t, got[0].Message, "date")
	})

	t.Run("a checkbox that is not a boolean", func(t *testing.T) {
		got := warningsFor(t, `{"version": 1, "id": "o1", "properties": {"done": "yes"}}`)
		require.Len(t, got, 1)
		assert.Equal(t, "/properties/done", got[0].Path)
	})

	t.Run("shapes the format does hold are quiet", func(t *testing.T) {
		// including the raw number a date out of RFC 3339 range exports as
		assert.Empty(t, warningsFor(t, `{"version": 1, "id": "o1", "properties": {
			"due_date": "2026-07-06T08:44:05Z", "created_date": 1751791445000,
			"done": true, "name": "N", "plural_name": "Ns", "tag": ["a", "b"]}}`))
	})

	t.Run("null is always a value", func(t *testing.T) {
		// §3: an explicit null records that the property was set and cleared
		assert.Empty(t, warningsFor(t, `{"version": 1, "id": "o1", "properties": {
			"due_date": null, "done": null}}`))
	})
}

// The mention attribute is snake_case like every other identifier the format
// defines, which the tag grammar had to learn: its attribute-name scanner read
// ASCII letters only, so `object_id` parsed as the attribute `object` and then
// failed on the underscore (§8.1).
func TestInline_MentionAttributeIsSnakeCase(t *testing.T) {
	md := `ping <mention object_id="bafyid">Roman</mention>`
	text, marks, err := parseInline(md)
	require.NoError(t, err)
	assert.Equal(t, "ping Roman", text)
	require.Len(t, marks, 1)
	assert.Equal(t, "bafyid", marks[0].Param)
	assert.Equal(t, md, renderInline(text, marks), "canonical form is byte-stable")

	// the previous draft's spelling is an error that names the attribute,
	// rather than a silently dropped mention
	_, _, err = parseInline(`ping <mention objectId="bafyid">Roman</mention>`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown attribute "objectId"`)
}

// The envelope `key` is the STORED identity key, written verbatim (§2) — so
// the charset it can carry is whatever the store already holds, and a closed
// allowlist over it was falsified by the first sweep that ran with one: 59
// objects in a 36 808-object account failed their own export, every one of
// them a relation option whose stored key is built from the option's *name*.
// These are the real keys, from that sweep's reports.
func TestValidate_EnvelopeKeyAcceptsRealStoredKeys(t *testing.T) {
	for _, key := range []string{
		"completion_status_Not Started",
		"challenge_resolution_status_In Progress",
		"69bbfc78877a91b1d12d1a7c_C/C++",
		"69bbfc78877a91b1d12d1a7c_C#",
		"69bbfc78877a91b1d12d1a84_$addToSet",
		"69bbfc78877a91b1d12d1a7c_.NET",
		"69aab06861fab2bc0d9afbe2_Roma Khafizianov",
		"69a56205ccba0a47d8d8eb71_тогглы",
		"69bbfc78877a91b1d12d1a7c_JavaScript/TypeScript",
	} {
		doc := fmt.Sprintf(`{"version": 1, "kind": "relation_option", "id": "o1", "internal_key": %q}`, key)
		assert.NoError(t, Validate([]byte(doc)), "stored key %q must round-trip", key)
	}
}

// What a key may never be is unreadable: empty, or carrying a control
// character. That is the same deny rule property keys get (§3), for the same
// reason — an allowlist can only be trusted after auditing every key in every
// account, and this one was not.
func TestValidate_EnvelopeKeyRejectsUnreadable(t *testing.T) {
	for _, doc := range []string{
		`{"version": 1, "kind": "object_type", "id": "t1", "internal_key": ""}`,
		"{\"version\": 1, \"kind\": \"object_type\", \"id\": \"t1\", \"key\": \"a\\u0000b\"}",
		"{\"version\": 1, \"kind\": \"object_type\", \"id\": \"t1\", \"key\": \"a\\nb\"}",
	} {
		assert.Error(t, Validate([]byte(doc)), doc)
	}
}

// ---- post-freeze review: admission keyed off the raw document spelling ----
//
// The three property checks above (deny rule, layout names, format shapes)
// were written before §3's key vocabulary arrived, and keyed off the RAW
// document spelling. The format's canonical spelling is the snake_case api
// slug, so writing "unique_key" instead of "uniqueKey" walked past all three
// — including the deny rule that stops a document from aiming itself at an
// existing object (existingobject.go resolves merge targets from oldAnytypeID,
// uniqueKey and sourceFilePath). The fix: every check runs on the STORED key a
// spelling resolves to, through the same chain import uses — the document's
// own legend, then the bundled table, then the spelling verbatim (§3).

// Every stripped key is a bundled relation with an api slug, so the canonical
// document spells the slug — and the deny rule has to hold for that spelling,
// derived from the same set as the stored-spelling test above.
func TestValidate_DeniedKeysRefusedInCanonicalSpelling(t *testing.T) {
	covered := 0
	for key := range strippedDetailKeys() {
		if isDroppedOnImport(key) {
			continue // dropped, not refused — see the note above
		}
		slug := (BundledKeyVocabulary{}).PropertySlug(key)
		if slug == key {
			continue // one spelling; TestValidate_ImportRefusesWhatExportStrips covers it
		}
		covered++
		doc := fmt.Sprintf(`{"version": 1, "id": "obj1", "properties": {%q: "x"}}`, slug)
		err := Validate([]byte(doc))
		require.Error(t, err, "%q is the canonical spelling of stripped key %q, so it must be refused", slug, key)
		assert.Contains(t, err.Error(), "/properties/"+slug)
	}
	require.NotZero(t, covered, "the bundled slug table no longer differs from the stored spellings — this test went dead")
}

// The legend is consulted before any vocabulary (§3), so without a check it
// was an unchecked rebind primitive: any spelling could be bound to any stored
// key, denied ones included — {"prio": "uniqueKey"} landed a uniqueKey detail,
// and {"myid": "id"} overwrote the envelope id itself (observed: details.id
// became ["boom"]). Admission runs on the resolved key, which the legend is
// part of resolving.
func TestValidate_LegendCannotRebindOntoInternalKeys(t *testing.T) {
	for name, doc := range map[string]string{
		"resolution vector": `{"version": 1, "id": "o1", "property_internal_keys": {"prio": "uniqueKey"}, "properties": {"prio": "ot-page"}}`,
		"envelope id":       `{"version": 1, "id": "o1", "property_internal_keys": {"myid": "id"}, "properties": {"myid": "boom"}}`,
		"stripped key":      `{"version": 1, "id": "o1", "property_internal_keys": {"s": "spaceId"}, "properties": {"s": "other"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			err := Validate([]byte(doc))
			require.Error(t, err, doc)
			_, _, unmErr := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
			require.Error(t, unmErr, "Unmarshal must reject what Validate rejects (I2)")
		})
	}

	// a legend entry that rebinds a spelling onto a HARMLESS key is the
	// feature working as specified: nothing lands on an internal key
	ok := `{"version": 1, "id": "o1", "property_internal_keys": {"prio": "6a32d4856761631534b22f85"}, "properties": {"prio": "high"}}`
	require.NoError(t, Validate([]byte(ok)))
	_, snap, err := Unmarshal([]byte(ok), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	require.Contains(t, snap.Details.Fields, "6a32d4856761631534b22f85")
}

// {"resolved_layout": "nonsense"} validated clean and imported the raw string
// onto a number-format property, where every consumer reads it with an int
// getter and silently sees "basic" — the exact silence the layout-name check
// exists to catch, dead for every canonically-spelled document.
func TestValidate_LayoutNameCheckedInCanonicalSpelling(t *testing.T) {
	err := Validate([]byte(`{"version": 1, "id": "o1", "properties": {"resolved_layout": "nonsense"}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/properties/resolved_layout")
	assert.Contains(t, err.Error(), "unknown layout")

	// a real name is accepted and lands as the stored number — the import
	// half always resolved the slug; only validation did not
	doc := `{"version": 1, "id": "o1", "properties": {"resolved_layout": "todo"}}`
	require.NoError(t, Validate([]byte(doc)))
	_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	v := snap.Details.Fields["resolvedLayout"]
	require.NotNil(t, v)
	assert.Equal(t, float64(layoutNames.value("todo")), v.GetNumberValue())
}

// A legend value is a stored key — the string that becomes a details field —
// so it obeys the same writable-key rule as a property name (§3): non-empty,
// no control characters, at most 128 characters. Export only ever records
// values that passed that rule, so the schema bound keeps admission symmetric.
func TestValidate_LegendValueMustBeAWritableKey(t *testing.T) {
	t.Run("refused", func(t *testing.T) {
		for name, doc := range map[string]string{
			"empty":        `{"version": 1, "property_internal_keys": {"p": ""}}`,
			"over-long":    fmt.Sprintf(`{"version": 1, "property_internal_keys": {"p": %q}}`, strings.Repeat("k", maxPropertyKeyLen+1)),
			"control char": `{"version": 1, "property_internal_keys": {"p": "a\nb"}}`,
		} {
			assert.Error(t, Validate([]byte(doc)), name)
		}
	})
	t.Run("accepted", func(t *testing.T) {
		// the shapes real stored keys have: bson-hex, and option keys carrying
		// the option's own name, spaces and non-ASCII included (ANOMALIES §7)
		doc := `{"version": 1, "property_internal_keys": {
			"prio": "6a32d4856761631534b22f85",
			"toggles": "69a56205ccba0a47d8d8eb71_тогглы"}}`
		require.NoError(t, Validate([]byte(doc)))
	})
}

// rebindingVocabulary stands in for a node-backed vocabulary whose space maps
// a slug to a stored key the bundled table never knew. Validate cannot see it
// (it takes no resolver, §13), so admission has to run AGAIN on the importer's
// final resolved key, at the seam where details are written.
type rebindingVocabulary struct{ BundledKeyVocabulary }

func (rebindingVocabulary) PropertyKey(slug string) (string, bool) {
	if slug == "prio" {
		return "uniqueKey", true
	}
	return BundledKeyVocabulary{}.PropertyKey(slug)
}

func TestImport_AdmissionRunsOnTheResolvedKey(t *testing.T) {
	doc := `{"version": 1, "id": "o1", "properties": {"prio": "ot-page"}}`
	require.NoError(t, Validate([]byte(doc)),
		"the bundled chain resolves prio verbatim, a legal custom key — Validate cannot know better")
	_, _, err := Unmarshal([]byte(doc), Options{Keys: rebindingVocabulary{}, GenerateId: seqIds("g")})
	require.Error(t, err, "the wider vocabulary resolves prio onto uniqueKey, which no document may set")
	assert.Contains(t, err.Error(), "/properties/prio")
	assert.Contains(t, err.Error(), "uniqueKey")
}

// unwritableSlugVocabulary produces the slug shapes a real space can mint:
// apiObjectKey is user-supplied or strcase-derived from the property name
// (objectcreator/util.go), with no length bound. buildProperties checked the
// STORED key's writability and then emitted the slug — the string that
// actually becomes the JSON property name — unchecked, so a 192-character
// slug made Marshal emit a document its own Validate rejects (I1).
type unwritableSlugVocabulary struct{ BundledKeyVocabulary }

func (unwritableSlugVocabulary) PropertySlug(key string) string {
	switch key {
	case "artist":
		return strings.Repeat("s", maxPropertyKeyLen+64)
	case "venue":
		return "ve\nue"
	case "city":
		return ""
	}
	return BundledKeyVocabulary{}.PropertySlug(key)
}

func TestExport_UnwritableSlugFallsBackToTheStoredKey(t *testing.T) {
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{Id: "obj1",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		Details: fields(map[string]*types.Value{
			"id": str("obj1"), "name": str("N"),
			"artist": str("x"), "venue": str("y"), "city": str("z"),
		}),
	}
	var warns []Issue
	data, err := Marshal(model.SmartBlockType_Page, snap,
		Options{Keys: unwritableSlugVocabulary{}, OnWarning: func(i Issue) { warns = append(warns, i) }})
	require.NoError(t, err)
	require.NoError(t, Validate(data), "Marshal must never emit what its own Validate rejects (§11):\n%s", data)

	var got struct {
		Properties map[string]any `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(data, &got))
	for _, k := range []string{"artist", "venue", "city"} {
		assert.Contains(t, got.Properties, k,
			"an unwritable slug falls back to the stored key, which is always its own address (§3)")
	}
	assert.NotEmpty(t, warns, "a vocabulary producing unwritable slugs is worth telling the caller about")
}

// The same raw-spelling defect had a fourth instance in the same loop's
// neighbourhood: the property_definitions-vs-recommended-lists ambiguity check
// indexed properties by the STORED list keys, so the canonical spelling
// "recommended_relations" carried both representations without a word.
func TestValidate_RecommendedListConflictCheckedInCanonicalSpelling(t *testing.T) {
	for _, spelling := range []string{"recommendedRelations", "recommended_relations"} {
		doc := fmt.Sprintf(`{"version": 1, "kind": "object_type", "id": "t1", "internal_key": "page",
			"type_settings": {"property_definitions": [{"property": "due_date", "format": "date"}]},
			"properties": {%q: ["a"]}}`, spelling)
		err := Validate([]byte(doc))
		require.Error(t, err, spelling)
		assert.Contains(t, err.Error(), "/properties/"+spelling)
		assert.Contains(t, err.Error(), "type_settings.property_definitions")
	}
}
