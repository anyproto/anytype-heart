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
	"regexp"
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
	// the envelope key is a STORED identity key written verbatim (§2), and a
	// closed charset over it was falsified by a 36 808-object sweep: relation
	// options carry their option *name* in the key, spaces and all. I1 never
	// covered this slot, which is exactly why the bad rule shipped.
	storedKeys := []string{
		"", "page", "task", "completion_status_Not Started",
		"69bbfc78877a91b1d12d1a7c_C/C++", "69a56205ccba0a47d8d8eb71_тогглы",
		"69bbfc78877a91b1d12d1a84_$addToSet", "opt-" + strings.Repeat("x", 80),
	}
	return &model.SmartBlockSnapshotBase{
		Blocks:  blocks,
		Details: fields(details),
		Key:     storedKeys[rnd.Intn(len(storedKeys))],
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
	`{"version": 1, "blocks": [{"type": "dataview", "views": [{"id": "v1", "page_size": 50.0}]}]}`,
	`{"version": 1, "blocks": [{"type": "dataview", "views": [{"id": "v1", "page_size": 1e19}]}]}`,
	`{"version": 1, "blocks": [{"type": "dataview", "views": [{"id": "v1", "page_size": 0}]}]}`,
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
	// admission runs on the RESOLVED stored key (§3): the canonical slug
	// spelling of a denied key, a property_keys legend rebinding a harmless
	// spelling onto one, and the layout-name check behind the same resolution.
	// These pin WHERE the rule lives as much as that it exists — a "fix" that
	// moves the deny rule into import alone makes Validate accept what
	// Unmarshal rejects, and this corpus is what catches that.
	`{"version": 1, "properties": {"unique_key": "ot-page"}}`,
	`{"version": 1, "properties": {"space_id": "other"}}`,
	`{"version": 1, "properties": {"old_anytype_id": "legacy-1"}}`,
	`{"version": 1, "properties": {"source_file_path": "/x/y"}}`,
	`{"version": 1, "properties": {"resolved_layout": "nonsense"}}`,
	`{"version": 1, "properties": {"resolved_layout": "todo"}}`,
	`{"version": 1, "property_keys": {"prio": "uniqueKey"}, "properties": {"prio": "ot-page"}}`,
	`{"version": 1, "property_keys": {"myid": "id"}, "properties": {"myid": "boom"}}`,
	`{"version": 1, "property_keys": {"s": "spaceId"}, "properties": {"s": "other"}}`,
	// a benign rebind is the legend working as specified, and flows through
	`{"version": 1, "property_keys": {"prio": "6a32d4856761631534b22f85"}, "properties": {"prio": "high"}}`,
	// a legend value is a stored key and obeys the writable-key rule (§3)
	`{"version": 1, "property_keys": {"p": ""}}`,
	`{"version": 1, "property_keys": {"p": "a\nb"}}`,
	`{"version": 1, "property_keys": {"p": "` + strings.Repeat("k", 129) + `"}}`,
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

// I3: every identifier the format defines is snake_case (§1 Naming). The rule
// is worth a test rather than a review habit — the vocabulary grows one enum
// value at a time, and a camelCase addition would be invisible until a
// generating model tripped over it, which is the failure the rename fixed.
//
// It covers the Go name tables as well as the schema: some vocabulary (the
// layout names) exists only in Go, which is exactly where a stray name hides.
func TestInvariant_VocabularyIsSnakeCase(t *testing.T) {
	// platform identifiers are quoted, not translated (§1 Naming)
	platform := map[string]bool{"allObjects": true, "recentOpen": true}
	snake := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

	check := func(t *testing.T, where, ident string) {
		t.Helper()
		if platform[ident] || strings.HasPrefix(ident, "$") {
			return
		}
		assert.Regexp(t, snake, ident, "%s: %q is not snake_case", where, ident)
	}

	for _, schema := range [][]byte{schemaJSON, indexSchemaJSON} {
		var doc any
		require.NoError(t, json.Unmarshal(schema, &doc))
		var walk func(node any)
		walk = func(node any) {
			switch n := node.(type) {
			case map[string]any:
				for key, v := range n {
					switch key {
					case "properties":
						if props, ok := v.(map[string]any); ok {
							for name, sub := range props {
								check(t, "schema property", name)
								walk(sub)
							}
							continue
						}
					case "enum":
						if list, ok := v.([]any); ok {
							for _, e := range list {
								if s, isStr := e.(string); isStr {
									check(t, "schema enum", s)
								}
							}
							continue
						}
					}
					walk(v)
				}
			case []any:
				for _, v := range n {
					walk(v)
				}
			}
		}
		walk(doc)
	}

	for name, values := range map[string][]string{
		"kind":            namesOf(kindNames.toName),
		"textStyle":       namesOf(textStyleNames.toName),
		"fileType":        namesOf(fileTypeNames.toName),
		"processor":       namesOf(processorNames.toName),
		"widgetLayout":    namesOf(widgetLayoutNames.toName),
		"viewType":        namesOf(viewTypeNames.toName),
		"condition":       namesOf(conditionNames.toName),
		"date_preset":     namesOf(datePresetNames.toName),
		"aggregation":     namesOf(aggregationNames.toName),
		"format":          namesOf(formatNames.toName),
		"layout":          namesOf(layoutNames.toName),
		"card_style":      namesOf(cardStyleNames.toName),
		"card_size":       namesOf(cardSizeNames.toName),
		"list_size":       namesOf(listSizeNames.toName),
		"empty_placement": namesOf(emptyPlacementNames.toName),
	} {
		for _, v := range values {
			check(t, name+" name table", v)
		}
	}
}

func namesOf[T comparable](m map[T]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
