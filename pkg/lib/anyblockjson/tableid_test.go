package anyblockjson

// A cell's id is rowId + "-" + colId, and the editor recovers the column from
// it with SplitN(id, "-", 2) (table.ParseCellID — the basis of every column
// insert/delete/move, HTML export and table normalization). A row or column
// id containing "-" therefore corrupts column identity, which is why the
// schema pins authored ones to [A-Za-z0-9_]{1,64}. Generated ids must honour
// the same rule, and Options.GenerateId belongs to the caller.

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

// A table's grid belongs to the table whether the ids were authored or
// generated (§6.1). Only the authored half was ever claimed, so a generated
// row or column left every cell id it implies free — free for an authored
// block to be sitting on already, and free for the next generated id to take.
// Either way the document imported to two blocks with one id, a snapshot no
// editor can resolve.
//
// Both fixtures calibrate themselves: they import the table alone to learn
// what the generator will call the row and the column, and only then build the
// collision. Hard-coding the generator's answer would make the test pass
// vacuously the day the number of genId calls changes.
func TestImport_GeneratedGridIsClaimed(t *testing.T) {
	tableOnly := `{"version": 1, "id": "p1", "blocks": [
		{"type": "table", "columns": [{}], "rows": [{"cells": ["cell"]}]},
		{"type": "paragraph", "text": "trailing"}]}`
	mints := 0
	_, probe, err := Unmarshal([]byte(tableOnly), Options{
		GenerateId: func() string { mints++; return fmt.Sprintf("g%d", mints) }})
	require.NoError(t, err)
	var rowId, colId string
	for _, b := range probe.Blocks {
		switch b.Content.(type) {
		case *model.BlockContentOfTableRow:
			rowId = b.Id
		case *model.BlockContentOfTableColumn:
			colId = b.Id
		}
	}
	require.NotEmpty(t, rowId, "the fixture's row id is generated")
	require.NotEmpty(t, colId, "the fixture's column id is generated")
	derived := rowId + "-" + colId
	require.Contains(t, blockIds(t, probe, "p1"), derived, "the cell block is built at the derived id")
	require.NotZero(t, mints, "the fixture generates ids")

	noDuplicateIds := func(t *testing.T, snap *model.SmartBlockSnapshotBase) {
		t.Helper()
		seen := map[string]bool{}
		for _, b := range snap.Blocks {
			assert.False(t, seen[b.Id], "duplicate block id %q — the generated grid landed on a taken id", b.Id)
			seen[b.Id] = true
		}
	}

	t.Run("an authored block is already sitting on the grid", func(t *testing.T) {
		doc := fmt.Sprintf(`{"version": 1, "id": "p1", "blocks": [
			{"type": "paragraph", "id": %q, "text": "authored"},
			{"type": "table", "columns": [{}], "rows": [{"cells": ["cell"]}]}]}`, derived)
		require.NoError(t, Validate([]byte(doc)), "the document itself is legal: %s", doc)

		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		noDuplicateIds(t, snap)
		// the authored block keeps its id and the generated grid moves off it:
		// a derived id has no spelling of its own, so the generated side — the
		// only side with a free choice — is the one that yields
		assert.Contains(t, blockIds(t, snap, "p1"), derived, "the authored block keeps its id")
		assert.NotEqual(t, derived, cellIdOf(t, snap), "the cell moved off the authored id")
	})

	t.Run("a later generated id lands on the grid", func(t *testing.T) {
		// the same document and the same generator, except that its LAST
		// answer — the trailing paragraph's — is the cell id the table just
		// derived. A generator is the caller's (the convert wiring derives ids
		// from file paths), so its answers are not the package's to trust.
		n := 0
		_, snap, err := Unmarshal([]byte(tableOnly), Options{GenerateId: func() string {
			n++
			if n == mints {
				return derived
			}
			return fmt.Sprintf("g%d", n)
		}})
		require.NoError(t, err)
		noDuplicateIds(t, snap)
		assert.Equal(t, derived, cellIdOf(t, snap),
			"the table's own cell is unmoved — nothing else claimed its id first")
	})
}

// cellIdOf returns the id of the single table cell in a snapshot: the one
// child of the one row.
func cellIdOf(t *testing.T, snap *model.SmartBlockSnapshotBase) string {
	t.Helper()
	for _, b := range snap.Blocks {
		if _, ok := b.Content.(*model.BlockContentOfTableRow); ok {
			require.Len(t, b.ChildrenIds, 1, "the fixture's row holds one cell")
			return b.ChildrenIds[0]
		}
	}
	t.Fatal("no table row in the snapshot")
	return ""
}

// A snapshot's block list is not its block tree: unlinked subtrees survive in
// it, and a table among them is not in the document at all. Its grid of
// derived ids therefore claims nothing — reserving it renamed a block the
// document DOES contain on the authority of one nobody can see, which is
// exactly the rename §9 promises never happens to an id that is already legal.
func TestExport_UnreachableTableReservesNoIds(t *testing.T) {
	reachable := []*model.Block{
		{Id: "root", ChildrenIds: []string{"r9-c9"},
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
		textBlock("r9-c9", model.BlockContentText_Paragraph, "in the document"),
	}
	// an orphan table: nothing links to "orphan", so the emit never arrives
	orphan := []*model.Block{
		{Id: "orphan", ChildrenIds: []string{"cols9", "rows9"},
			Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}},
		{Id: "cols9", ChildrenIds: []string{"c9"},
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}}},
		{Id: "rows9", ChildrenIds: []string{"r9"},
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}}},
		{Id: "c9", Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}},
		{Id: "r9", Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}},
	}
	details := fields(map[string]*types.Value{"id": str("root")})

	withOrphan, err := Marshal(model.SmartBlockType_Page, &model.SmartBlockSnapshotBase{
		Blocks: append(append([]*model.Block{}, reachable...), orphan...), Details: details}, testOptions())
	require.NoError(t, err)
	assert.Contains(t, string(withOrphan), `"id": "r9-c9"`,
		"the reachable block keeps the id it was authored with:\n%s", withOrphan)

	without, err := Marshal(model.SmartBlockType_Page, &model.SmartBlockSnapshotBase{
		Blocks: reachable, Details: details}, testOptions())
	require.NoError(t, err)
	assert.Equal(t, string(without), string(withOrphan),
		"a block the document does not contain may not change the document")
}

// The fragment export's entry point is the caller's subtree[0], not the root
// indexBlocks infers — and an inferred root is what a fragment slice least
// deserves to be judged by: it is "the first block nobody references", which a
// slice carrying its own parent, or any spare entry, moves somewhere else.
// Reserve from the wrong entry point and the table that IS emitted reserves
// nothing, so the plain block on its derived cell id keeps that id and Marshal
// writes a duplicate — a run its own Validate rejects (I1).
func TestMarshalBlockSubtree_ReservesFromTheCallersRoot(t *testing.T) {
	subtree := []*model.Block{
		// [0] is what the caller addresses; it holds the table and a plain
		// block spelling the (r1,c1) cell id the table derives
		{Id: "holder", ChildrenIds: []string{"tbl", "r1-c1"},
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "holder"}}},
		{Id: "tbl", ChildrenIds: []string{"cols", "rows"},
			Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}},
		{Id: "cols", ChildrenIds: []string{"c1"},
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}}},
		{Id: "rows", ChildrenIds: []string{"r1"},
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}}},
		{Id: "c1", Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}},
		{Id: "r1", Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}},
		textBlock("r1-c1", model.BlockContentText_Paragraph, "sits on a cell id"),
		// a spare entry the emit never visits, and the caller's own parent —
		// between them, "the first block nobody references" is neither the
		// table's owner nor anything that leads to it
		textBlock("spare", model.BlockContentText_Paragraph, "unreferenced"),
		{Id: "parent", ChildrenIds: []string{"holder"},
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "parent"}}},
	}
	fragment, err := MarshalBlockSubtree(subtree, Options{})
	require.NoError(t, err)

	// the run is validated the way a fragment is: as the blocks of a document
	var env struct {
		Blocks json.RawMessage `json:"blocks"`
	}
	require.NoError(t, json.Unmarshal(fragment, &env))
	doc, err := json.Marshal(map[string]any{"version": FormatVersion, "blocks": env.Blocks})
	require.NoError(t, err)
	assert.NoError(t, Validate(doc), "Marshal must not emit what Validate rejects:\n%s", fragment)
	assert.Contains(t, string(fragment), `"r1-c1_2"`, "the plain block yields to the grid:\n%s", fragment)
}

// The mirror of the export rule on the import side: a generated id may not
// land on a cell id the document's table already implies, materialized or not.
func TestImport_GeneratedIdAvoidsUnwrittenCellId(t *testing.T) {
	doc := `{"version": 1, "id": "p1", "blocks": [
		{"type": "table", "id": "tbl", "columns": [{"id": "c1"}], "rows": [{"id": "r1"}]},
		{"type": "paragraph", "text": "x"}]}`
	_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: func() string { return "r1-c1" }})
	require.NoError(t, err)

	ids := map[string]bool{}
	for _, b := range snap.Blocks {
		assert.False(t, ids[b.Id], "duplicate block id %q", b.Id)
		ids[b.Id] = true
	}
	assert.False(t, ids["r1-c1"], "a generated id took the (r1,c1) cell id")
}

// Sanitizing is for ids that need it. A generated id that is already a legal
// row/column id keeps its name — nothing renames it, and above all not the
// disambiguation pass, which used to find the id taken by the generator's own
// claim and hand back `<id>_2` for every row and column ever minted.
func TestImport_GeneratedTableInnerIdsKeepTheirName(t *testing.T) {
	doc := `{"version": 1, "blocks": [{"type": "table",
		"columns": [{}, {}], "rows": [{"cells": ["a", "b"]}, {"cells": ["c", "d"]}]}]}`

	t.Run("legal generated ids are untouched", func(t *testing.T) {
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		var inner []string
		for _, b := range snap.Blocks {
			switch b.Content.(type) {
			case *model.BlockContentOfTableRow, *model.BlockContentOfTableColumn:
				inner = append(inner, b.Id)
			}
		}
		require.Len(t, inner, 4)
		for _, id := range inner {
			assert.Regexp(t, `^g\d+$`, id, "a generated id that needs no sanitizing keeps its name")
		}
	})

	t.Run("the default generator's shape survives", func(t *testing.T) {
		_, snap, err := Unmarshal([]byte(doc), Options{})
		require.NoError(t, err)
		for _, b := range snap.Blocks {
			switch b.Content.(type) {
			case *model.BlockContentOfTableRow, *model.BlockContentOfTableColumn:
				assert.Regexp(t, `^[0-9a-f]{24}$`, b.Id, "24 hex chars, like every other minted id")
			}
		}
	})

	t.Run("a sanitized id takes the sanitized name, nothing more", func(t *testing.T) {
		// a dashed generator (the convert wiring's shape): every id sanitizes
		// to a name nothing else holds, so the suffix pass has no work to do
		n := 0
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: func() string {
			n++
			return fmt.Sprintf("x-%d", n)
		}})
		require.NoError(t, err)
		seen := map[string]bool{}
		for _, b := range snap.Blocks {
			switch b.Content.(type) {
			case *model.BlockContentOfTableRow, *model.BlockContentOfTableColumn:
				assert.False(t, seen[b.Id], "duplicate table inner id %q", b.Id)
				seen[b.Id] = true
				assert.Regexp(t, `^x_\d+$`, b.Id, "sanitized, and not suffixed on top of it")
			}
		}
		require.Len(t, seen, 4)
	})
}
