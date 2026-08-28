package anyblockjson

// flat_rules_test.go pins the specific flat-encoding rules the two invariants
// in flat_invariants_test.go rest on: leaf containment, the depth bound, the
// table-cell rules, the float-form indent (`1.0` read as 0, which passed
// validation and then imported as something else), and the property-message
// wording. Each was violated at least once before it was pinned.
//
// The invariants themselves live next door and are driven by hostile inputs;
// these are instances, expressed as the documents that broke them.

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

// tableSnapshot builds root → table (one column, one row) whose single cell
// is cellContent with cellChildren nested under it.
func tableSnapshot(cellContent model.IsBlockContent, cellChildren ...*model.Block) *model.SmartBlockSnapshotBase {
	cellChildIds := make([]string, 0, len(cellChildren))
	for _, c := range cellChildren {
		cellChildIds = append(cellChildIds, c.Id)
	}
	blocks := []*model.Block{
		{Id: "obj1", ChildrenIds: []string{"table1"},
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
		{Id: "table1", ChildrenIds: []string{"tcols", "trows"},
			Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}},
		{Id: "tcols", ChildrenIds: []string{"c1"},
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}}},
		{Id: "trows", ChildrenIds: []string{"r1"},
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}}},
		{Id: "c1", Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}},
		{Id: "r1", ChildrenIds: []string{"r1-c1"},
			Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}},
		{Id: "r1-c1", ChildrenIds: cellChildIds, Content: cellContent},
	}
	return &model.SmartBlockSnapshotBase{
		Blocks:  append(blocks, cellChildren...),
		Details: fields(map[string]*types.Value{"id": str("obj1")}),
	}
}

// innerTable returns a minimal valid table subtree rooted at id.
func innerTable(id string) []*model.Block {
	return []*model.Block{
		{Id: id, ChildrenIds: []string{id + "cols", id + "rows"},
			Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}},
		{Id: id + "cols", Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}}},
		{Id: id + "rows", Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}}},
	}
}

// Finding 1: cells cannot contain tables (the schema's recursion cut) — a
// snapshot violating that must fail Marshal loudly rather than produce a
// document Validate rejects.
func TestMarshal_TableInCellErrors(t *testing.T) {
	t.Run("table among cell descendants", func(t *testing.T) {
		inner := innerTable("inner")
		snap := tableSnapshot(
			&model.BlockContentOfText{Text: &model.BlockContentText{Text: "cell"}},
			inner...,
		)
		_, err := Marshal(model.SmartBlockType_Page, snap, Options{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cells cannot contain tables")
		assert.Contains(t, err.Error(), "r1-c1")
	})

	t.Run("cell block is a table", func(t *testing.T) {
		inner := innerTable("inner")
		snap := tableSnapshot(inner[0].Content, nil...)
		// graft the inner table's wrappers under the cell id
		snap.Blocks[len(snap.Blocks)-1].ChildrenIds = inner[0].ChildrenIds
		snap.Blocks = append(snap.Blocks, inner[1], inner[2])
		_, err := Marshal(model.SmartBlockType_Page, snap, Options{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cells cannot contain tables")
	})
}

// Finding 2: the schema's integer type admits integer-valued floats
// (1.0, 1e0); Validate and Unmarshal must agree on every such input, and
// V1/V2 must fire on float-form violations.
func TestIndent_FloatForms(t *testing.T) {
	agree := func(t *testing.T, doc string) (valErr, unmErr error) {
		valErr = Validate([]byte(doc))
		_, _, unmErr = Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		assert.Equal(t, valErr == nil, unmErr == nil,
			"Validate (%v) and Unmarshal (%v) must agree", valErr, unmErr)
		return valErr, unmErr
	}

	t.Run("V2 fires on float indent under a leaf", func(t *testing.T) {
		doc := `{"version": 1, "blocks": [{"type": "divider"}, {"indent": 1.0, "type": "paragraph", "text": "x"}]}`
		valErr, _ := agree(t, doc)
		require.Error(t, valErr)
		assert.Contains(t, valErr.Error(), "divider blocks cannot have children")
	})

	t.Run("V1 fires on float jump", func(t *testing.T) {
		doc := `{"version": 1, "blocks": [{"type": "paragraph", "text": "a"}, {"indent": 5.0, "type": "paragraph", "text": "b"}]}`
		valErr, _ := agree(t, doc)
		require.Error(t, valErr)
		assert.Contains(t, valErr.Error(), "indent 5 follows indent 0")
	})

	t.Run("valid float forms import with the right depth and canonicalize", func(t *testing.T) {
		for _, form := range []string{"1.0", "1e0"} {
			doc := fmt.Sprintf(`{"version": 1, "blocks": [
				{"id": "a", "type": "toggle", "text": "t"},
				{"indent": %s, "id": "b", "type": "paragraph", "text": "x"}
			]}`, form)
			require.NoError(t, Validate([]byte(doc)), form)
			sbType, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
			require.NoError(t, err, form)
			byId := map[string]*model.Block{}
			for _, b := range snap.Blocks {
				byId[b.Id] = b
			}
			assert.Equal(t, []string{"b"}, byId["a"].ChildrenIds, form)
			out, err := Marshal(sbType, snap, Options{})
			require.NoError(t, err, form)
			assert.Contains(t, string(out), `"indent": 1`, form)
		}
	})
}

// Finding 3: a snapshot nested deeper than the F4 bound must fail Marshal
// loudly; at the bound it must marshal to a document Validate accepts.
func TestMarshal_DepthBound(t *testing.T) {
	chain := func(depth int) *model.SmartBlockSnapshotBase {
		blocks := []*model.Block{{Id: "obj1", ChildrenIds: []string{"n0"},
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}}
		for i := 0; i <= depth; i++ {
			b := textBlock(fmt.Sprintf("n%d", i), model.BlockContentText_Toggle, fmt.Sprintf("level %d", i))
			if i < depth {
				b.ChildrenIds = []string{fmt.Sprintf("n%d", i+1)}
			}
			blocks = append(blocks, b)
		}
		return &model.SmartBlockSnapshotBase{Blocks: blocks}
	}

	t.Run("depth 40 errors", func(t *testing.T) {
		_, err := Marshal(model.SmartBlockType_Page, chain(40), Options{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds the format bound 32")
		assert.Contains(t, err.Error(), "n33")
	})

	t.Run("depth 32 marshals and validates", func(t *testing.T) {
		data, err := Marshal(model.SmartBlockType_Page, chain(32), Options{})
		require.NoError(t, err)
		require.NoError(t, Validate(data))
	})
}

// M3 (C11): with an OnWarning sink the read path degrades content the format
// can't represent (here: over-deep nesting) to a warning instead of failing
// the whole document, and the degraded output still validates.
func TestMarshal_OnWarningDegradesOverDeep(t *testing.T) {
	deepChain := func(depth int) *model.SmartBlockSnapshotBase {
		blocks := []*model.Block{{Id: "obj1", ChildrenIds: []string{"n0"},
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}}
		for i := 0; i <= depth; i++ {
			b := textBlock(fmt.Sprintf("n%d", i), model.BlockContentText_Toggle, fmt.Sprintf("level %d", i))
			if i < depth {
				b.ChildrenIds = []string{fmt.Sprintf("n%d", i+1)}
			}
			blocks = append(blocks, b)
		}
		return &model.SmartBlockSnapshotBase{Blocks: blocks}
	}

	// without a sink the over-deep document still fails loudly (canonical export)
	_, err := Marshal(model.SmartBlockType_Page, deepChain(40), Options{})
	require.Error(t, err)

	// with a sink it degrades: clamp + warn, and the result validates
	var warnings []Issue
	data, err := Marshal(model.SmartBlockType_Page, deepChain(40),
		Options{OnWarning: func(i Issue) { warnings = append(warnings, i) }})
	require.NoError(t, err, "the read must succeed with a warning sink")
	require.NotEmpty(t, warnings, "the clamp emits a warning")
	assert.Contains(t, warnings[0].Message, "clamped")
	require.NoError(t, Validate(data), "clamped indents stay within the format bound")
}

// Finding 4: unknown properties are rejected with a message naming the key;
// `children` additionally gets the flat-migration hint.
func TestValidate_UnknownPropertyMessages(t *testing.T) {
	err := Validate([]byte(`{"version": 1, "blocks": [{"type": "toggle", "children": [{"type": "paragraph"}]}]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `/blocks/0/children: property "children" is not allowed — the flat format has no children; nest with indent instead`)

	err = Validate([]byte(`{"version": 1, "blocks": [{"type": "paragraph", "banana": 1}]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `/blocks/0/banana: property "banana" is not allowed`)
}

// Finding 5: the F10 array-form cell round-trips byte-stable.
func TestRoundTrip_CellArrayForm(t *testing.T) {
	snap := tableSnapshot(
		&model.BlockContentOfText{Text: &model.BlockContentText{Style: model.BlockContentText_Toggle, Text: "cell"}},
		textBlock("child1", model.BlockContentText_Paragraph, "nested"),
	)
	first, err := Marshal(model.SmartBlockType_Page, snap, Options{})
	require.NoError(t, err)
	require.NoError(t, Validate(first))
	assert.Contains(t, string(first), `"cells": [`)
	assert.Contains(t, string(first), `"indent": 1`)

	sbType, snap2, err := Unmarshal(first, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	byId := map[string]*model.Block{}
	for _, b := range snap2.Blocks {
		byId[b.Id] = b
	}
	cell := byId["r1-c1"]
	require.NotNil(t, cell)
	require.Len(t, cell.ChildrenIds, 1)
	child := byId[cell.ChildrenIds[0]]
	require.NotNil(t, child)
	assert.Equal(t, "nested", child.Content.(*model.BlockContentOfText).Text.Text)

	second, err := Marshal(sbType, snap2, Options{})
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))
}

// Finding 6: every V2 leaf type actually drops children on export, and the
// validation leaf set matches the export behavior (drift alarm).
func TestLeafTypes_ExportAgreement(t *testing.T) {
	// factories keyed by JSON type; each returns the leaf block (children
	// attached by the test) plus any extra blocks its subtree needs
	leafFactories := map[string]func() (*model.Block, []*model.Block){
		"embed": func() (*model.Block, []*model.Block) {
			return &model.Block{Id: "leaf", Content: &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{Text: "x"}}}, nil
		},
		"bookmark": func() (*model.Block, []*model.Block) {
			return &model.Block{Id: "leaf", Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{Url: "https://x.io"}}}, nil
		},
		"link": func() (*model.Block, []*model.Block) {
			return &model.Block{Id: "leaf", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{TargetBlockId: "obj"}}}, nil
		},
		"divider": func() (*model.Block, []*model.Block) {
			return &model.Block{Id: "leaf", Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}}, nil
		},
		"table": func() (*model.Block, []*model.Block) {
			blocks := innerTable("leaf")
			return blocks[0], blocks[1:]
		},
		"property": func() (*model.Block, []*model.Block) {
			return &model.Block{Id: "leaf", Content: &model.BlockContentOfRelation{Relation: &model.BlockContentRelation{Key: "name"}}}, nil
		},
		"dataview": func() (*model.Block, []*model.Block) {
			return &model.Block{Id: "leaf", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{}}}, nil
		},
		"icon": func() (*model.Block, []*model.Block) {
			return &model.Block{Id: "leaf", Content: &model.BlockContentOfIcon{Icon: &model.BlockContentIcon{Name: "smile"}}}, nil
		},
		"table_of_contents": func() (*model.Block, []*model.Block) {
			return &model.Block{Id: "leaf", Content: &model.BlockContentOfTableOfContents{TableOfContents: &model.BlockContentTableOfContents{}}}, nil
		},
		"featured_properties": func() (*model.Block, []*model.Block) {
			return &model.Block{Id: "leaf", Content: &model.BlockContentOfFeaturedRelations{FeaturedRelations: &model.BlockContentFeaturedRelations{}}}, nil
		},
		"chat": func() (*model.Block, []*model.Block) {
			return &model.Block{Id: "leaf", Content: &model.BlockContentOfChat{Chat: &model.BlockContentChat{}}}, nil
		},
	}

	// the validation leaf set must equal the factory set plus the equation
	// input alias (which exports as embed)
	wantLeafSet := map[string]bool{"equation": true}
	for typ := range leafFactories {
		wantLeafSet[typ] = true
	}
	assert.Equal(t, wantLeafSet, leafBlockTypes,
		"leafBlockTypes drifted from the export withChildren=false set — update both together")

	// maxIndentInBlocks reads the deepest indent in an exported document
	maxIndent := func(t *testing.T, data []byte) int {
		var doc struct {
			Blocks []struct {
				Indent int `json:"indent"`
			} `json:"blocks"`
		}
		require.NoError(t, json.Unmarshal(data, &doc))
		out := 0
		for _, b := range doc.Blocks {
			if b.Indent > out {
				out = b.Indent
			}
		}
		return out
	}

	// wrap each leaf in a toggle so structural top-level dropping (§7,
	// featuredProperties) does not interfere; a surviving grandchild would
	// appear at indent 2
	build := func(leaf *model.Block, extra []*model.Block) *model.SmartBlockSnapshotBase {
		child := textBlock("grandchild", model.BlockContentText_Paragraph, "under leaf")
		leaf.ChildrenIds = append(leaf.ChildrenIds, child.Id)
		wrapper := textBlock("w1", model.BlockContentText_Toggle, "wrap")
		wrapper.ChildrenIds = []string{leaf.Id}
		blocks := []*model.Block{
			{Id: "obj1", ChildrenIds: []string{"w1"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			wrapper, leaf, child,
		}
		return &model.SmartBlockSnapshotBase{Blocks: append(blocks, extra...)}
	}

	for typ, factory := range leafFactories {
		t.Run(typ, func(t *testing.T) {
			leaf, extra := factory()
			data, err := Marshal(model.SmartBlockType_Page, build(leaf, extra), Options{})
			require.NoError(t, err)
			require.NoError(t, Validate(data))
			assert.Equal(t, 1, maxIndent(t, data), "children of a %s block must be dropped on export", typ)
		})
	}

	t.Run("control: paragraph keeps children", func(t *testing.T) {
		leaf := textBlock("leaf", model.BlockContentText_Paragraph, "parent")
		data, err := Marshal(model.SmartBlockType_Page, build(leaf, nil), Options{})
		require.NoError(t, err)
		require.NoError(t, Validate(data))
		assert.Equal(t, 2, maxIndent(t, data))
	})
}

// Finding 7: lenient clamps land on the containment checks — a clamped
// block still errors under a leaf parent or a row (V2/V3 evaluate on the
// clamped indents).
func TestNormalizeIndent_ContainmentStillErrors(t *testing.T) {
	t.Run("clamped under a leaf", func(t *testing.T) {
		doc := `{"version": 1, "blocks": [{"type": "divider"}, {"indent": 5, "type": "paragraph", "text": "x"}]}`
		_, _, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), NormalizeIndent: true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "divider blocks cannot have children")
	})

	t.Run("clamped non-column under a row", func(t *testing.T) {
		doc := `{"version": 1, "blocks": [{"type": "row"}, {"indent": 7, "type": "paragraph", "text": "x"}]}`
		_, _, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), NormalizeIndent: true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "a row block can only contain column blocks")
	})
}

// Finding 8: the prefix property holds at real-world depth (≥ 6), not just
// the depth-2 rich fixture.
func TestValidate_PrefixProperty_Deep(t *testing.T) {
	var parts []string
	parts = append(parts, `{"id": "d0", "type": "toggle", "text": "level 0"}`)
	for d := 1; d <= 8; d++ {
		parts = append(parts, fmt.Sprintf(`{"indent": %d, "id": "d%d", "type": "toggle", "text": "level %d"}`, d, d, d))
	}
	parts = append(parts, `{"id": "top", "type": "paragraph", "text": "back to top"}`)
	doc := `{"version": 1, "blocks": [` + strings.Join(parts, ",") + `]}`

	sbType, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	canonical, err := Marshal(sbType, snap, Options{})
	require.NoError(t, err)

	var parsed struct {
		Blocks []json.RawMessage `json:"blocks"`
	}
	require.NoError(t, json.Unmarshal(canonical, &parsed))
	require.Len(t, parsed.Blocks, 10)
	for n := 0; n <= len(parsed.Blocks); n++ {
		blockParts := make([]string, 0, n)
		for _, b := range parsed.Blocks[:n] {
			blockParts = append(blockParts, string(b))
		}
		prefix := `{"version": 1, "blocks": [` + strings.Join(blockParts, ",") + `]}`
		require.NoError(t, Validate([]byte(prefix)), "prefix of %d blocks", n)
	}
}
