package anyblockjson

// Two invariants hold the format together, and the pre-freeze review found six
// violations of the first and four of the second — every one of them in a place
// the rule had never been applied, though it was written down. Instances get
// their own regression tests in prefreeze_review_test.go; these are the
// invariants themselves, so the next instance fails here without anyone having
// thought of it:
//
//	I1. Marshal never emits a document its own Validate rejects (§11).
//	I2. Validate and Unmarshal agree on every input (§12): if Validate accepts
//	    a document, Unmarshal must not fail to decode it, and vice versa.
//
// Both are driven by hostile inputs on purpose. A corpus generated from
// Marshal's own output cannot catch what Marshal gets wrong — it would agree
// with itself — and the goldens are exactly that corpus. So I1 runs over
// snapshots built from the id shapes real data and real generators produce
// (dots, slashes, non-ASCII, over-long, derived-cell-shaped, suffix-colliding),
// and I2 over hand-written documents.

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// hostileIds are the id shapes that broke the id domain, plus the ones that
// make two surfaces compete: `a.b` and `dir/file` miss the schema's charset,
// `c-1` sanitizes onto `c_1`, `block_12345` suffixes onto `12345` under
// CompactIds, `r1-c1` is what a table derives for its own cell, `dataview` is
// the id the importer pins, and the long one exceeds the 64-character bound.
var hostileIds = []string{
	"", "a.b", "a-b", "a_b", "dir/file", "блок", "c-1", "c_1", "c1", "r1",
	"r1-c1", "r1-c2", "12345", "block_12345", "dataview", "-", "_",
	strings.Repeat("x", 70), "R1-C1", "a b", "id\n2", "obj1",
}

// hostileSnapshot builds a deterministic snapshot for seed n: a root, a handful
// of text blocks, and optionally a table and a dataview, with every id drawn
// from hostileIds — including duplicates, which the snapshot graph is allowed
// to contain because it is untrusted (§11).
func hostileSnapshot(n int) *model.SmartBlockSnapshotBase {
	rnd := rand.New(rand.NewSource(int64(n)))
	pick := func() string { return hostileIds[rnd.Intn(len(hostileIds))] }

	root := &model.Block{Id: "obj1",
		Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}
	blocks := []*model.Block{root}
	add := func(b *model.Block) {
		root.ChildrenIds = append(root.ChildrenIds, b.Id)
		blocks = append(blocks, b)
	}

	for i := 0; i < 1+rnd.Intn(4); i++ {
		add(&model.Block{Id: pick(), Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{Text: fmt.Sprintf("text %d <sub>x</sub>", i)}}})
	}
	if rnd.Intn(2) == 0 {
		colIds := []string{pick(), pick()}
		rowIds := []string{pick(), pick()}
		table := &model.Block{Id: pick(),
			Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}}
		cols := &model.Block{Id: "cols" + fmt.Sprint(n),
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_TableColumns}}}
		rows := &model.Block{Id: "rows" + fmt.Sprint(n),
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_TableRows}}}
		table.ChildrenIds = []string{cols.Id, rows.Id}
		add(table)
		blocks = append(blocks, cols, rows)
		for _, id := range colIds {
			cols.ChildrenIds = append(cols.ChildrenIds, id)
			blocks = append(blocks, &model.Block{Id: id,
				Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}})
		}
		for _, rowId := range rowIds {
			rows.ChildrenIds = append(rows.ChildrenIds, rowId)
			row := &model.Block{Id: rowId,
				Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}}
			blocks = append(blocks, row)
			for _, colId := range colIds {
				cellId := rowId + "-" + colId
				row.ChildrenIds = append(row.ChildrenIds, cellId)
				blocks = append(blocks, &model.Block{Id: cellId,
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{Text: "cell " + cellId}}})
			}
		}
	}
	if rnd.Intn(3) == 0 {
		add(&model.Block{Id: pick(), Content: &model.BlockContentOfDataview{
			Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{{Id: pick(), Name: "All"}},
			}}})
	}
	// details carry the keys the import surface must refuse: if export ever
	// emitted one, Marshal's output would fail its own Validate, which is how
	// this invariant proves the two surfaces are still each other's mirror
	details := map[string]*types.Value{
		"id":             str("obj1"),
		"name":           str("hostile"),
		"spaceId":        str("bafyspace"),
		"uniqueKey":      str("ot-page"),
		"oldAnytypeID":   str("legacy1"),
		"sourceFilePath": str("/tmp/x.md"),
		"restrictions":   {Kind: &types.Value_NumberValue{NumberValue: 3}},
		"isArchived":     {Kind: &types.Value_BoolValue{BoolValue: true}},
		"":               str("empty key"),
		"a\nb":           str("newline key"),
		"dueDate":        str("next Friday"),
	}
	return &model.SmartBlockSnapshotBase{
		Blocks:  blocks,
		Details: fields(details),
	}
}

// I1: Marshal either fails loudly — §11 allows that for an over-deep tree or a
// table inside a cell — or produces a document its own Validate accepts. What
// it may never do is succeed and hand back an unimportable archive, which is a
// failure nobody sees until the archive is needed.
func TestInvariant_MarshalOutputValidates(t *testing.T) {
	variants := map[string]Options{
		"plain":   {},
		"compact": {CompactIds: true},
		"omitIds": {OmitIds: true},
	}
	for name, opts := range variants {
		t.Run(name, func(t *testing.T) {
			for n := 0; n < 300; n++ {
				snap := hostileSnapshot(n)
				data, err := Marshal(model.SmartBlockType_Page, snap, opts)
				if err != nil {
					continue // a loud failure is allowed; silent invalidity is not
				}
				require.NoError(t, Validate(data), "seed %d produced:\n%s", n, data)
			}
		})
	}
}

// The same invariant on the goldens' own fixture, which is the case the
// existing corpus covers — kept so a regression there is not mistaken for a
// hostile-input-only problem.
func TestInvariant_MarshalOutputValidates_RichFixture(t *testing.T) {
	for name, opts := range map[string]Options{
		"plain":   testOptions(),
		"compact": {CompactIds: true, ResolveFormat: testFormatResolver, ResolveOptions: testResolver},
		"omitIds": {OmitIds: true, ResolveFormat: testFormatResolver, ResolveOptions: testResolver},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := Marshal(model.SmartBlockType_Page, richSnapshot(), opts)
			require.NoError(t, err)
			require.NoError(t, Validate(data))
		})
	}
}

// hostileDocs are hand-written documents aimed at the seam between the schema's
// idea of a value and Go's: the schema says "integer", the decoder says int64,
// and JSON Schema counts 2048.0 and 1e1 as integers. Every one of these is a
// document a generator can plausibly emit.
var hostileDocs = []string{
	`{"version": 1}`,
	`{"version": 1.0}`,
	`{"version": 1e0}`,
	`{"version": 1.5}`,
	`{"version": 2}`,
	`{"version": 0}`,
	`{"version": 1, "blocks": [{"type": "file", "size": 2048}]}`,
	`{"version": 1, "blocks": [{"type": "file", "size": 2048.0}]}`,
	`{"version": 1, "blocks": [{"type": "file", "size": 1e3}]}`,
	`{"version": 1, "blocks": [{"type": "file", "size": 1e30}]}`,
	`{"version": 1, "blocks": [{"type": "file", "size": -1}]}`,
	`{"version": 1, "blocks": [{"type": "file", "size": 2048.5}]}`,
	`{"version": 1, "blocks": [{"type": "widget", "limit": 10}]}`,
	`{"version": 1, "blocks": [{"type": "widget", "limit": 1e1}]}`,
	`{"version": 1, "blocks": [{"type": "widget", "limit": 1e20}]}`,
	`{"version": 1, "blocks": [{"type": "widget", "limit": -3}]}`,
	`{"version": 1, "blocks": [{"type": "dataview", "views": [{"id": "v1", "pageSize": 50.0}]}]}`,
	`{"version": 1, "blocks": [{"type": "dataview", "views": [{"id": "v1", "pageSize": 1e19}]}]}`,
	`{"version": 1, "blocks": [{"type": "dataview", "views": [{"id": "v1", "pageSize": 0}]}]}`,
	`{"version": 1, "blocks": [{"type": "table", "columns": [{"id": "c1", "width": 120.7}], "rows": []}]}`,
	`{"version": 1, "blocks": [{"type": "table", "columns": [{"id": "c1", "width": 1e30}], "rows": []}]}`,
	`{"version": 1, "blocks": [{"type": "table", "columns": [{"id": "c1", "width": -5}], "rows": []}]}`,
	`{"version": 1, "blocks": [{"indent": 0.0, "type": "paragraph", "text": "x"}]}`,
	`{"version": 1, "blocks": [{"indent": 1e1, "type": "paragraph", "text": "x"}]}`,
	`{"version": 1, "properties": {"name": "x", "size": 9007199254740993}}`,
	`{"version": 1, "blocks": [{"type": "paragraph", "text": "<sub>x</sub>"}]}`,
	// a JSON number larger than float64 can hold. The loose surfaces have no
	// schema bound to catch it by construction (§3 accepts any number), and the
	// snapshot they decode into is a proto Struct, whose numbers are float64 —
	// so there is nowhere to put such a value, and the answer has to be a
	// path-addressed rejection rather than a decode error
	`{"version": 1, "properties": {"num": 1e400}}`,
	`{"version": 1, "properties": {"num": 1e309}}`,
	`{"version": 1, "properties": {"num": 1e308}}`,
	`{"version": 1, "store": {"k": 1e400}}`,
	`{"version": 1, "blocks": [{"type": "paragraph", "text": "x", "fields": {"w": 1e400}}]}`,
	`{"version": 1, "blocks": [{"type": "table", "columns": [{"id": "c1", "width": 1e400}], "rows": []}]}`,
	`{"version": 1, "blocks": [{"type": "dataview", "views": [{"id": "v1",
		"filters": [{"property": "p", "condition": "equal", "value": 1e400}]}]}]}`,
	`{"version": 1, "blocks": [{"type": "table", "columns": [{"id": "c1"}],
		"rows": [{"id": "r1", "cells": [["nested", {"indent": 1, "type": "paragraph", "text": "y"}]]}]}]}`,
}

// I2: whatever Validate accepts, Unmarshal must decode, and whatever Validate
// rejects, Unmarshal must reject too. A disagreement means the guarantee
// Validate offers — "this document imports" — is not true, and the failure
// arrives as a bare Go decode error with no JSON pointer, outside the
// path-addressed error contract §13 promises.
func TestInvariant_ValidateAndUnmarshalAgree(t *testing.T) {
	for _, doc := range hostileDocs {
		t.Run(doc[:min(len(doc), 60)], func(t *testing.T) {
			valErr := Validate([]byte(doc))
			_, _, unmErr := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
			assert.Equal(t, valErr == nil, unmErr == nil,
				"Validate says %v, Unmarshal says %v", valErr, unmErr)
			if valErr != nil {
				// a rejection must still be path-addressed, not a raw decode
				// error escaping from the Go layer
				assert.NotContains(t, unmErr.Error(), "decode document",
					"the reason must come from validation, not from json.Unmarshal")
			}
		})
	}
}

// Whatever Unmarshal accepts must re-export to something Validate accepts too:
// this is I1 with the input side as the generator, which is how an agent's
// document actually travels.
func TestInvariant_ImportedDocumentReExportsValid(t *testing.T) {
	for _, doc := range hostileDocs {
		if Validate([]byte(doc)) != nil {
			continue
		}
		sbType, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err, doc)
		out, err := Marshal(sbType, snap, Options{})
		require.NoError(t, err, doc)
		require.NoError(t, Validate(out), "%s re-exported as:\n%s", doc, out)

		// and the canonical form is byte-stable through another round (§11.2)
		_, snap2, err := Unmarshal(out, Options{GenerateId: seqIds("h")})
		require.NoError(t, err, doc)
		again, err := Marshal(sbType, snap2, Options{})
		require.NoError(t, err, doc)
		assert.Equal(t, string(out), string(again), "re-export must be byte-stable for %s", doc)
	}
}

// A document's ids must survive a round trip unchanged when they are already
// valid: sanitizing is for ids that need it, and renaming one that does not
// would break the "provided ids are preserved so re-exports diff cleanly"
// promise (§9).
func TestExport_ValidIdsAreNeverRenamed(t *testing.T) {
	doc := `{"version": 1, "id": "obj1", "blocks": [
		{"type": "paragraph", "id": "a_b", "text": "first"},
		{"type": "paragraph", "id": "keep-me", "text": "second"},
		{"type": "table", "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": ["x"]}]}]}`
	sbType, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	out, err := Marshal(sbType, snap, Options{})
	require.NoError(t, err)

	var got struct {
		Blocks []struct {
			Id      string `json:"id"`
			Columns []struct {
				Id string `json:"id"`
			} `json:"columns"`
			Rows []struct {
				Id string `json:"id"`
			} `json:"rows"`
		} `json:"blocks"`
	}
	require.NoError(t, json.Unmarshal(out, &got))
	require.Len(t, got.Blocks, 3)
	assert.Equal(t, "a_b", got.Blocks[0].Id)
	assert.Equal(t, "keep-me", got.Blocks[1].Id)
	assert.Equal(t, "c1", got.Blocks[2].Columns[0].Id)
	assert.Equal(t, "r1", got.Blocks[2].Rows[0].Id)
}
