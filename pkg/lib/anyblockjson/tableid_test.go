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
