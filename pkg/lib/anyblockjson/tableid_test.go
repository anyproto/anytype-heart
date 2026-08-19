package anyblockjson

// A cell's id is rowId + "-" + colId, and the editor recovers the column from
// it with SplitN(id, "-", 2) (table.ParseCellID — the basis of every column
// insert/delete/move, HTML export and table normalization). A row or column
// id containing "-" therefore corrupts column identity, which is why the
// schema pins authored ones to [A-Za-z0-9_]{1,64}. Generated ids must honour
// the same rule, and Options.GenerateId belongs to the caller.

import (
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const tableDoc = `{"version": 1, "id": "p1", "blocks": [{"type": "table",
	"columns": [{}, {}],
	"rows": [{"cells": ["a", "b"]}]}]}`

// the convert wiring derives ids from file paths, so its generator is full of
// dashes — the package cannot assume otherwise
func TestImport_GeneratedTableIdsAreSeparatorFree(t *testing.T) {
	n := 0
	_, snap, err := Unmarshal([]byte(tableDoc), Options{
		GenerateId: func() string {
			n++
			return "pages-doc-kickoff-onboarding-" + string(rune('0'+n))
		},
	})
	require.NoError(t, err)

	var rows, cols, cells []string
	for _, b := range snap.Blocks {
		switch b.Content.(type) {
		case *model.BlockContentOfTableRow:
			rows = append(rows, b.Id)
		case *model.BlockContentOfTableColumn:
			cols = append(cols, b.Id)
		}
	}
	require.Len(t, rows, 1)
	require.Len(t, cols, 2)
	for _, id := range append(append([]string{}, rows...), cols...) {
		assert.NotContains(t, id, "-", "row/column id must be separator-free")
		assert.Regexp(t, `^[A-Za-z0-9_]{1,64}$`, id)
	}

	// and the cells built from them carry exactly one separator
	for _, row := range rows {
		for _, cellId := range blockById(snap, row).ChildrenIds {
			cells = append(cells, cellId)
			assert.Equal(t, 1, strings.Count(cellId, "-"),
				"cell id must split into exactly rowId and colId")
			rowId, colId, _ := strings.Cut(cellId, "-")
			assert.Equal(t, row, rowId)
			assert.Contains(t, cols, colId)
		}
	}
	assert.Len(t, cells, 2)
}

func blockById(snap *model.SmartBlockSnapshotBase, id string) *model.Block {
	for _, b := range snap.Blocks {
		if b.Id == id {
			return b
		}
	}
	return nil
}

// distinct source ids must stay distinct after sanitizing
func TestImport_SanitizedTableIdsStayUnique(t *testing.T) {
	ids := []string{"a-b", "a_b", "a.b", "a b"} // all sanitize to "a_b"
	n := 0
	_, snap, err := Unmarshal([]byte(`{"version": 1, "id": "p1", "blocks": [{"type": "table",
		"columns": [{}, {}], "rows": [{"cells": ["x", "y"]}, {"cells": ["z"]}]}]}`),
		Options{GenerateId: func() string {
			id := ids[n%len(ids)]
			n++
			return id
		}})
	require.NoError(t, err)

	seen := map[string]bool{}
	for _, b := range snap.Blocks {
		switch b.Content.(type) {
		case *model.BlockContentOfTableRow, *model.BlockContentOfTableColumn:
			assert.False(t, seen[b.Id], "duplicate table inner id %q", b.Id)
			seen[b.Id] = true
			assert.Regexp(t, `^[A-Za-z0-9_]{1,64}$`, b.Id)
		}
	}
}

// Marshal must never emit a document its own Validate rejects: stored tables
// predating this rule carry dashed row/column ids.
func TestExport_DashedTableIdsAreNormalized(t *testing.T) {
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "root", ChildrenIds: []string{"tbl"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "tbl", ChildrenIds: []string{"cols", "rows"},
				Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}},
			{Id: "cols", ChildrenIds: []string{"pages-doc-12"},
				Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}}},
			{Id: "rows", ChildrenIds: []string{"pages-doc-17"},
				Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}}},
			{Id: "pages-doc-12", Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}},
			{Id: "pages-doc-17", ChildrenIds: []string{"pages-doc-17-pages-doc-12"},
				Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}},
			textBlock("pages-doc-17-pages-doc-12", model.BlockContentText_Paragraph, "cell"),
		},
		Details: fields(map[string]*types.Value{"id": str("root")}),
	}
	data, err := Marshal(model.SmartBlockType_Page, snap, testOptions())
	require.NoError(t, err)

	assert.NoError(t, Validate(data), "Marshal must not emit what Validate rejects")
	assert.Contains(t, string(data), `"id": "pages_doc_17"`)
	assert.Contains(t, string(data), `"id": "pages_doc_12"`)
	assert.Contains(t, string(data), "cell", "cell content survives the relabel")

	// and the normalized document round-trips
	_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	again, err := Marshal(model.SmartBlockType_Page, back, testOptions())
	require.NoError(t, err)
	assert.Equal(t, string(data), string(again), "export must be byte-stable (§11)")
}

// A cell's id is derived, so a table owns every rowId-colId of its grid
// whether or not that cell is materialized (§4, §6.1): validation claims the
// whole grid, and the editor materializes the missing cell at exactly that id
// the first time it is filled. Export has to reserve the same ids, or a
// perfectly legal snapshot — every block id unique — marshals into a document
// Validate rejects.
func TestExport_UnwrittenDerivedCellIdIsReserved(t *testing.T) {
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "root", ChildrenIds: []string{"tbl", "r1-c1"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "tbl", ChildrenIds: []string{"cols", "rows"},
				Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}},
			{Id: "cols", ChildrenIds: []string{"c1"},
				Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}}},
			{Id: "rows", ChildrenIds: []string{"r1"},
				Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}}},
			{Id: "c1", Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}},
			// the (r1,c1) cell is not materialized: the row has no children
			{Id: "r1", Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}},
			textBlock("r1-c1", model.BlockContentText_Paragraph, "sibling"),
		},
		Details: fields(map[string]*types.Value{"id": str("root")}),
	}
	// the fixture is not a bad fixture: every stored id in it is unique
	seen := map[string]bool{}
	for _, b := range snap.Blocks {
		require.False(t, seen[b.Id], "fixture id %q is not unique", b.Id)
		seen[b.Id] = true
	}

	data, err := Marshal(model.SmartBlockType_Page, snap, testOptions())
	require.NoError(t, err)
	assert.NoError(t, Validate(data), "Marshal must not emit what Validate rejects")

	// the table's ids are what the derived id is made of, so they stay; the
	// plain block spelling a derived cell id is the one that yields (§4)
	assert.Contains(t, string(data), `"id": "c1"`)
	assert.Contains(t, string(data), `"id": "r1"`)
	assert.Contains(t, string(data), `"id": "r1-c1_2"`)
	assert.Contains(t, string(data), "sibling", "the block keeps its content")
}

// A cell rendered through the string shorthand never reaches blockToJSON,
// which is where the emit-once mark is set (§11). Without the mark, a block
// that is both a cell and a child somewhere else is written twice — once as
// the cell's text and once as a block carrying the derived cell id, which is a
// duplicate id in the output.
//
// The mark has to be READ as well as written, and which of the two failures
// shows up depends on which parent the walk reaches first — so both orders are
// tested. Setting the mark without consulting it covers only the order where
// the cell comes first; reach the other parent first and the cell writes the
// block a SECOND time, and one stored block imports back as two.
func TestExport_StringShorthandCellIsEmittedOnce(t *testing.T) {
	// one block, two parents: the row's cell and a child of a top-level block
	sharedCellSnapshot := func(rootChildren ...string) *model.SmartBlockSnapshotBase {
		return &model.SmartBlockSnapshotBase{
			Blocks: []*model.Block{
				{Id: "root", ChildrenIds: rootChildren,
					Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
				{Id: "tbl", ChildrenIds: []string{"cols", "rows"},
					Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}},
				{Id: "cols", ChildrenIds: []string{"c1"},
					Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}}},
				{Id: "rows", ChildrenIds: []string{"r1"},
					Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}}},
				{Id: "c1", Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}},
				{Id: "r1", ChildrenIds: []string{"r1-c1"},
					Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}},
				{Id: "holder", ChildrenIds: []string{"r1-c1"},
					Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "holder"}}},
				textBlock("r1-c1", model.BlockContentText_Paragraph, "shared"),
			},
			Details: fields(map[string]*types.Value{"id": str("root")}),
		}
	}
	for name, snap := range map[string]*model.SmartBlockSnapshotBase{
		"the cell is reached first":         sharedCellSnapshot("tbl", "holder"),
		"the other parent is reached first": sharedCellSnapshot("holder", "tbl"),
	} {
		t.Run(name, func(t *testing.T) {
			data, err := Marshal(model.SmartBlockType_Page, snap, testOptions())
			require.NoError(t, err)
			assert.NoError(t, Validate(data), "Marshal must not emit what Validate rejects")
			assert.Equal(t, 1, strings.Count(string(data), "shared"),
				"the block is emitted once:\n%s", data)

			// and the count the document states is the count that comes back:
			// a second emission is a block import has to build, not a phrase
			// that happens to appear twice
			_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
			require.NoError(t, err)
			var shared int
			for _, b := range back.Blocks {
				if t, ok := b.Content.(*model.BlockContentOfText); ok && t.Text.GetText() == "shared" {
					shared++
				}
			}
			assert.Equal(t, 1, shared, "one stored block, one imported block:\n%s", data)
		})
	}
}
