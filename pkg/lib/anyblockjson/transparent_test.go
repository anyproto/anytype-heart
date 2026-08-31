package anyblockjson

// transparent_test.go covers §7a — transparent containers. A `Layout/Div`
// (the editor's fan-out wrapper) and a content-less block contribute
// containment and nothing else, so export lifts their children to the
// container's own depth and writes nothing for the container.
//
// The package had no test and no golden carrying one of these before the
// rule existed: `grep '"group"' *_test.go testdata/` matched only dataview
// `group_by`. Every case here is new ground.

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

// divBlock builds the wrapper state.wrapChildrenToDiv mints — content
// Layout/Div, id prefixed `div-` exactly as newDiv() spells it. The prefix
// is fixture realism ONLY: the rule keys on content (see the `authored
// group` cases, whose ids carry no prefix and lift just the same).
func divBlock(id string, children ...string) *model.Block {
	return &model.Block{Id: id, ChildrenIds: children,
		Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Div}}}
}

// contentless builds the other §7a shape: a block whose content oneof is
// unset (legacy accounts hold these around a relation object's "used in"
// dataview).
func contentless(id string, children ...string) *model.Block {
	return &model.Block{Id: id, ChildrenIds: children}
}

// withChildren links children under an existing block fixture.
func withChildren(b *model.Block, children ...string) *model.Block {
	b.ChildrenIds = children
	return b
}

func layoutBlock(id string, style model.BlockContentLayoutStyle, children ...string) *model.Block {
	return &model.Block{Id: id, ChildrenIds: children,
		Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: style}}}
}

// pageOf wraps blocks into a page snapshot whose root lists rootChildren.
func pageOf(rootChildren []string, blocks ...*model.Block) *model.SmartBlockSnapshotBase {
	all := []*model.Block{{Id: "obj1", ChildrenIds: rootChildren,
		Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}}
	return &model.SmartBlockSnapshotBase{
		Blocks:  append(all, blocks...),
		Details: fields(map[string]*types.Value{"id": str("obj1")}),
	}
}

// blockLines renders the served blocks as "<indent> <id> <type>" lines —
// the flat run's structure, which is exactly what the lift changes.
func blockLines(t *testing.T, data []byte) []string {
	t.Helper()
	var doc struct {
		Blocks []struct {
			Indent int    `json:"indent"`
			Id     string `json:"id"`
			Type   string `json:"type"`
		} `json:"blocks"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	out := make([]string, 0, len(doc.Blocks))
	for _, b := range doc.Blocks {
		out = append(out, fmt.Sprintf("%d %s %s", b.Indent, b.Id, b.Type))
	}
	return out
}

func TestMarshal_TransparentContainersAreLifted(t *testing.T) {
	for _, tc := range []struct {
		name string
		snap *model.SmartBlockSnapshotBase
		want []string
	}{
		{
			name: "a container at indent 0 lifts its children to indent 0",
			snap: pageOf([]string{"div-1", "p3"},
				divBlock("div-1", "p1", "p2"),
				textBlock("p1", model.BlockContentText_Paragraph, "one"),
				textBlock("p2", model.BlockContentText_Paragraph, "two"),
				textBlock("p3", model.BlockContentText_Paragraph, "three")),
			want: []string{"0 p1 paragraph", "0 p2 paragraph", "0 p3 paragraph"},
		},
		{
			name: "nested containers collapse fully — a chain of three removes three levels",
			snap: pageOf([]string{"div-1"},
				divBlock("div-1", "div-2"),
				divBlock("div-2", "div-3"),
				divBlock("div-3", "p1"),
				textBlock("p1", model.BlockContentText_Paragraph, "one")),
			want: []string{"0 p1 paragraph"},
		},
		{
			name: "a childless container emits nothing",
			snap: pageOf([]string{"div-1", "p1"},
				divBlock("div-1"),
				textBlock("p1", model.BlockContentText_Paragraph, "one")),
			want: []string{"0 p1 paragraph"},
		},
		{
			name: "a content-less block with children is a container too",
			snap: pageOf([]string{"legacy"},
				contentless("legacy", "p1"),
				textBlock("p1", model.BlockContentText_Paragraph, "one")),
			want: []string{"0 p1 paragraph"},
		},
		{
			name: "a container under real nesting keeps the parent's depth for its children",
			snap: pageOf([]string{"t1"},
				withChildren(textBlock("t1", model.BlockContentText_Toggle, "toggle"), "div-1"),
				divBlock("div-1", "p1"),
				textBlock("p1", model.BlockContentText_Paragraph, "one")),
			want: []string{"0 t1 toggle", "1 p1 paragraph"},
		},
		{
			// the decoy: row and column are author-created and grammar-
			// bearing (§5). A rule that lifted "any layout block" would
			// flatten this into two bare paragraphs.
			name: "row and column are NOT transparent",
			snap: pageOf([]string{"row1"},
				layoutBlock("row1", model.BlockContentLayout_Row, "col1", "col2"),
				layoutBlock("col1", model.BlockContentLayout_Column, "p1"),
				layoutBlock("col2", model.BlockContentLayout_Column, "p2"),
				textBlock("p1", model.BlockContentText_Paragraph, "left"),
				textBlock("p2", model.BlockContentText_Paragraph, "right")),
			want: []string{
				"0 row1 row", "1 col1 column", "2 p1 paragraph",
				"1 col2 column", "2 p2 paragraph",
			},
		},
		{
			// the live I1 hole this closes: a Layout_Row with more than 40
			// columns normalizes to row → div → columns, which Marshal used
			// to emit and its own Validate rejected
			// ("a row block can only contain column blocks, got group").
			name: "row > container > column says row > column",
			snap: pageOf([]string{"row1"},
				layoutBlock("row1", model.BlockContentLayout_Row, "div-1"),
				divBlock("div-1", "col1", "col2"),
				layoutBlock("col1", model.BlockContentLayout_Column, "p1"),
				layoutBlock("col2", model.BlockContentLayout_Column, "p2"),
				textBlock("p1", model.BlockContentText_Paragraph, "left"),
				textBlock("p2", model.BlockContentText_Paragraph, "right")),
			want: []string{
				"0 row1 row", "1 col1 column", "2 p1 paragraph",
				"1 col2 column", "2 p2 paragraph",
			},
		},
		{
			name: "a structural block under a top-level container is dropped, not preserved",
			snap: pageOf([]string{"div-1"},
				divBlock("div-1", "title", "p1"),
				textBlock("title", model.BlockContentText_Title, "The title"),
				textBlock("p1", model.BlockContentText_Paragraph, "one")),
			want: []string{"0 p1 paragraph"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := Marshal(model.SmartBlockType_Page, tc.snap, testOptions())
			require.NoError(t, err)
			// I1: Marshal never emits what its own Validate rejects
			require.NoError(t, Validate(data))
			assert.Equal(t, tc.want, blockLines(t, data))
			assert.NotContains(t, string(data), `"group"`, "export emits no group, ever")
		})
	}
}

// TestMarshal_ContainerAttributesAreDroppedWithAWarning pins the accepted
// loss: a container's own align/background/fields go with it. The warning is
// the only trace, and it costs nothing on real data — every one of the 7,303
// wrappers in the production corpus carries no attribute at all.
func TestMarshal_ContainerAttributesAreDroppedWithAWarning(t *testing.T) {
	div := divBlock("div-1", "p1")
	div.BackgroundColor = "red"
	div.Align = model.Block_AlignCenter
	div.Fields = fields(map[string]*types.Value{"custom": str("kept nowhere")})
	snap := pageOf([]string{"div-1"}, div, textBlock("p1", model.BlockContentText_Paragraph, "one"))

	var warnings []Issue
	opts := testOptions()
	opts.OnWarning = func(i Issue) { warnings = append(warnings, i) }

	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	assert.Equal(t, []string{"0 p1 paragraph"}, blockLines(t, data))
	assert.NotContains(t, string(data), "red")
	assert.NotContains(t, string(data), "kept nowhere")

	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].Message, "div-1")
	assert.Contains(t, warnings[0].Message, "attributes on it are dropped")
}

// TestMarshal_ContainerAttributesWarnOnceUnderCompaction: the id census runs
// the block emit a SECOND time on a throwaway exporter (§9a), so every
// warning the emit raises is raised twice unless the probe's sink is
// silenced. A caller shown each issue twice stops trusting the count.
func TestMarshal_ContainerAttributesWarnOnceUnderCompaction(t *testing.T) {
	div := divBlock("div-1", "p1")
	div.BackgroundColor = "red"
	snap := pageOf([]string{"div-1"}, div, textBlock("p1", model.BlockContentText_Paragraph, "one"))

	var warnings []Issue
	opts := testOptions()
	opts.CompactIds = true // this is what turns the census probe on
	opts.OnWarning = func(i Issue) { warnings = append(warnings, i) }

	_, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	assert.Len(t, warnings, 1, "the census probe must not report the emit's issues a second time")
}

// TestMarshal_ContainerAttributesSilentWhenBare is the other half: a bare
// container — what normalization actually mints — warns about nothing.
func TestMarshal_ContainerAttributesSilentWhenBare(t *testing.T) {
	snap := pageOf([]string{"div-1"}, divBlock("div-1", "p1"),
		textBlock("p1", model.BlockContentText_Paragraph, "one"))

	var warnings []Issue
	opts := testOptions()
	opts.OnWarning = func(i Issue) { warnings = append(warnings, i) }

	_, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	assert.Empty(t, warnings)
}

// TestMarshal_ContainerCycleTerminates: the lift skips blockToJSON, which is
// where the emit-once mark is set, so the lift has to set it itself. Without
// that, a ChildrenIds cycle through a chain of containers recurses until the
// stack gives out — an untrusted snapshot crashing the process.
func TestMarshal_ContainerCycleTerminates(t *testing.T) {
	snap := pageOf([]string{"div-1"},
		divBlock("div-1", "div-2", "p1"),
		divBlock("div-2", "div-1"), // back-edge
		textBlock("p1", model.BlockContentText_Paragraph, "one"))

	data, err := Marshal(model.SmartBlockType_Page, snap, testOptions())
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	assert.Equal(t, []string{"0 p1 paragraph"}, blockLines(t, data))
}

// TestMarshal_ContainerSharedByTwoParents: the same mark, read back. A
// container listed by two parents must lift once — its children are blocks,
// and emitting them twice is a document with duplicate ids that Validate
// rejects (I1).
func TestMarshal_ContainerSharedByTwoParents(t *testing.T) {
	snap := pageOf([]string{"t1", "t2"},
		withChildren(textBlock("t1", model.BlockContentText_Toggle, "first"), "div-1"),
		withChildren(textBlock("t2", model.BlockContentText_Toggle, "second"), "div-1"),
		divBlock("div-1", "p1"),
		textBlock("p1", model.BlockContentText_Paragraph, "one"))

	data, err := Marshal(model.SmartBlockType_Page, snap, testOptions())
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	assert.Equal(t, []string{"0 t1 toggle", "1 p1 paragraph", "0 t2 toggle"}, blockLines(t, data))
}

// TestMarshal_ContainerInsideATableCell: the lift lives in appendBlocksFlat,
// which is also what walks a cell's descendants — so cells get the rule for
// free. "For free" is still a claim that needs a test.
func TestMarshal_ContainerInsideATableCell(t *testing.T) {
	snap := tableSnapshot(
		&model.BlockContentOfText{Text: &model.BlockContentText{Style: model.BlockContentText_Paragraph, Text: "cell"}},
		divBlock("div-1", "p1"),
		textBlock("p1", model.BlockContentText_Paragraph, "under the container"),
	)
	data, err := Marshal(model.SmartBlockType_Page, snap, testOptions())
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	assert.NotContains(t, string(data), `"group"`)
	// the cell renders as its array form: the cell block at 0, the lifted
	// paragraph at 1 — the depth the container held
	assert.Contains(t, string(data), `"indent": 1`)
	assert.Contains(t, string(data), "under the container")
}

// TestMarshal_ContainerAsACellRendersEmpty is the one place the lift cannot
// run: a cell is a position, not a run, so there is nowhere to lift to.
func TestMarshal_ContainerAsACellRendersEmpty(t *testing.T) {
	snap := tableSnapshot(nil, textBlock("p1", model.BlockContentText_Paragraph, "lost with the cell"))

	var warnings []Issue
	opts := testOptions()
	opts.OnWarning = func(i Issue) { warnings = append(warnings, i) }

	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	assert.NotContains(t, string(data), "lost with the cell")
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].Message, "a cell cannot be lifted")
}

// TestMarshalBlockSubtree_ContainerRoot: the fragment surface follows the
// rule at every level, its ROOT included — a deliberate divergence from §7's
// structural carve-out, because no read surface ever serves a container id,
// so no caller can address one except out of a stale cache.
func TestMarshalBlockSubtree_ContainerRoot(t *testing.T) {
	t.Run("a container root marshals as its lifted children", func(t *testing.T) {
		subtree := []*model.Block{
			divBlock("div-1", "p1", "p2"),
			textBlock("p1", model.BlockContentText_Paragraph, "one"),
			textBlock("p2", model.BlockContentText_Paragraph, "two"),
		}
		data, err := MarshalBlockSubtree(subtree, testOptions())
		require.NoError(t, err)
		assert.Equal(t, []string{"0 p1 paragraph", "0 p2 paragraph"}, blockLines(t, data))
	})
	t.Run("a childless container root marshals as an empty run", func(t *testing.T) {
		data, err := MarshalBlockSubtree([]*model.Block{divBlock("div-1")}, testOptions())
		require.NoError(t, err)
		assert.Empty(t, blockLines(t, data))
	})
}

// TestMarshal_WrappedPrimaryDataviewPinsUnderOmitIds is the live corruption
// this repairs on 160 real objects: their own dataview sits at indent 1
// inside a wrapper, so §7's primary-dataview pin — which fires only at
// indent 0 — never fires, the id `dataview` is lost under OmitIds, and the
// editor adds a SECOND, empty dataview on open. Lifted, the pin fires.
func TestMarshal_WrappedPrimaryDataviewPinsUnderOmitIds(t *testing.T) {
	snap := pageOf([]string{"div-1"},
		divBlock("div-1", "dv1"),
		&model.Block{Id: "dv1", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
			RelationLinks: []*model.RelationLink{{Key: "name", Format: model.RelationFormat_shorttext}},
			Views:         []*model.BlockContentDataviewView{{Id: "v1", Type: model.BlockContentDataviewView_Table, Name: "All"}},
		}}})

	opts := testOptions()
	opts.OmitIds = true
	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	assert.Equal(t, []string{"0  dataview"}, blockLines(t, data), "the dataview must be served at indent 0")

	_, reimported, err := Unmarshal(data, opts)
	require.NoError(t, err)
	var dvId string
	for _, b := range reimported.Blocks {
		if b.GetDataview() != nil {
			dvId = b.Id
		}
	}
	assert.Equal(t, dataviewBlockId, dvId,
		"the object's own dataview must come back at the editor's fixed id, or the editor adds a second one")
}

// TestMarshal_TransparentContainersGolden freezes the served bytes for a
// document carrying containers in every shape the corpus has: at indent 0,
// nested, childless, attributed, content-less, inside a row, and inside a
// table cell.
func TestMarshal_TransparentContainersGolden(t *testing.T) {
	attributed := divBlock("div-attributed", "p4")
	attributed.BackgroundColor = "red"
	attributed.Align = model.Block_AlignCenter

	snap := pageOf(
		[]string{"div-top", "div-empty", "legacy", "row1", "div-attributed", "table1"},
		divBlock("div-top", "h1", "div-nested"),
		textBlock("h1", model.BlockContentText_Header1, "Heading"),
		divBlock("div-nested", "p1", "p2"),
		textBlock("p1", model.BlockContentText_Paragraph, "one"),
		textBlock("p2", model.BlockContentText_Paragraph, "two"),
		divBlock("div-empty"),
		contentless("legacy", "p3"),
		textBlock("p3", model.BlockContentText_Paragraph, "three"),
		layoutBlock("row1", model.BlockContentLayout_Row, "div-inrow"),
		divBlock("div-inrow", "col1"),
		layoutBlock("col1", model.BlockContentLayout_Column, "p5"),
		textBlock("p5", model.BlockContentText_Paragraph, "in a column"),
		attributed,
		textBlock("p4", model.BlockContentText_Paragraph, "four"),
		// a table whose single cell holds a container below it
		&model.Block{Id: "table1", ChildrenIds: []string{"tcols", "trows"},
			Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}},
		layoutBlock("tcols", model.BlockContentLayout_TableColumns, "c1"),
		layoutBlock("trows", model.BlockContentLayout_TableRows, "r1"),
		&model.Block{Id: "c1", Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}},
		&model.Block{Id: "r1", ChildrenIds: []string{"r1-c1"},
			Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}},
		&model.Block{Id: "r1-c1", ChildrenIds: []string{"div-incell"},
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Style: model.BlockContentText_Paragraph, Text: "cell"}}},
		divBlock("div-incell", "p6"),
		textBlock("p6", model.BlockContentText_Paragraph, "under the cell's container"),
	)

	opts := testOptions()
	opts.OnWarning = func(Issue) {} // the attributed container warns; not the subject here
	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	require.False(t, strings.Contains(string(data), `"group"`))
	checkGolden(t, "containers.json", data)
}

//
// ---- import (§7a) ----
//

// importedTree renders a snapshot's block graph as "<indent> <type>" lines
// in document order, so a lift can be read off the rebuilt tree.
func importedTree(t *testing.T, s *model.SmartBlockSnapshotBase) []string {
	t.Helper()
	byId := map[string]*model.Block{}
	child := map[string]bool{}
	for _, b := range s.Blocks {
		byId[b.Id] = b
		for _, c := range b.ChildrenIds {
			child[c] = true
		}
	}
	var root *model.Block
	for _, b := range s.Blocks {
		if !child[b.Id] {
			root = b
			break
		}
	}
	var out []string
	var walk func(ids []string, depth int)
	walk = func(ids []string, depth int) {
		for _, id := range ids {
			b := byId[id]
			if b == nil {
				continue
			}
			out = append(out, fmt.Sprintf("%d %s", depth, blockKind(b)))
			walk(b.ChildrenIds, depth+1)
		}
	}
	require.NotNil(t, root)
	walk(root.ChildrenIds, 0)
	return out
}

// blockKind names a model block the way the JSON type does, closely enough
// to read a tree by.
func blockKind(b *model.Block) string {
	switch c := b.Content.(type) {
	case *model.BlockContentOfText:
		return "text:" + orEmpty(c.Text).Text
	case *model.BlockContentOfLayout:
		switch orEmpty(c.Layout).Style {
		case model.BlockContentLayout_Row:
			return "row"
		case model.BlockContentLayout_Column:
			return "column"
		case model.BlockContentLayout_Div:
			return "DIV"
		}
		return "layout"
	case *model.BlockContentOfDataview:
		return "dataview"
	case nil:
		return "CONTENT-LESS"
	}
	return fmt.Sprintf("%T", b.Content)
}

func TestUnmarshal_TransparentContainersAreLifted(t *testing.T) {
	for _, tc := range []struct {
		name   string
		blocks string
		want   []string
	}{
		{
			name:   "a container contributes no block and its children re-base",
			blocks: `{"type":"group"},{"indent":1,"type":"paragraph","text":"one"},{"indent":1,"type":"paragraph","text":"two"}`,
			want:   []string{"0 text:one", "0 text:two"},
		},
		{
			name:   "nested containers re-base recursively",
			blocks: `{"type":"group"},{"indent":1,"type":"group"},{"indent":2,"type":"paragraph","text":"deep"}`,
			want:   []string{"0 text:deep"},
		},
		{
			name:   "a childless container is simply gone",
			blocks: `{"type":"group"},{"type":"paragraph","text":"after"}`,
			want:   []string{"0 text:after"},
		},
		{
			name:   "only the container's own subtree re-bases",
			blocks: `{"type":"toggle","text":"t"},{"indent":1,"type":"group"},{"indent":2,"type":"paragraph","text":"inside"},{"type":"paragraph","text":"after"}`,
			want:   []string{"0 text:t", "1 text:inside", "0 text:after"},
		},
		{
			name:   "attributes on a container are ignored",
			blocks: `{"type":"group","background_color":"red","align":"center","fields":{"x":"y"}},{"indent":1,"type":"paragraph","text":"one"}`,
			want:   []string{"0 text:one"},
		},
		{
			// the decoy again, on the read side
			name:   "row and column are rebuilt, not lifted",
			blocks: `{"type":"row"},{"indent":1,"type":"column"},{"indent":2,"type":"paragraph","text":"left"}`,
			want:   []string{"0 row", "1 column", "2 text:left"},
		},
		{
			name:   "row > group > column reads as row > column",
			blocks: `{"type":"row"},{"indent":1,"type":"group"},{"indent":2,"type":"column"},{"indent":3,"type":"paragraph","text":"left"}`,
			want:   []string{"0 row", "1 column", "2 text:left"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := fmt.Sprintf(`{"version":2,"type":"page","blocks":[%s]}`, tc.blocks)
			require.NoError(t, Validate([]byte(doc)))

			_, snapshot, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
			require.NoError(t, err)
			assert.Equal(t, tc.want, importedTree(t, snapshot))
			for _, b := range snapshot.Blocks {
				assert.NotEqual(t, "DIV", blockKind(b),
					"import must not mint a Layout_Div: no read would ever show it, and normalization never removes one that has children")
			}
		})
	}
}

// TestUnmarshal_ContainerLiftRunsBeforeTheStructuralRules pins §7a's
// ordering. The lift is positional, so a lifted title is at indent 0 for
// every purpose: absorbed into properties.name exactly as a title written at
// indent 0 is, and the wrapped primary dataview reaches the position §7's pin
// requires.
func TestUnmarshal_ContainerLiftRunsBeforeTheStructuralRules(t *testing.T) {
	t.Run("a wrapped title is absorbed into the name property", func(t *testing.T) {
		doc := `{"version":2,"type":"page","blocks":[
			{"type":"group"},
			{"indent":1,"type":"title","text":"Wrapped title"},
			{"indent":1,"type":"paragraph","text":"body"}]}`
		_, snapshot, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, "Wrapped title", snapshot.Details.Fields["name"].GetStringValue())
		assert.Equal(t, []string{"0 text:body"}, importedTree(t, snapshot))
	})
	t.Run("a wrapped primary dataview is pinned to the editor's fixed id", func(t *testing.T) {
		doc := `{"version":2,"type":"page","blocks":[
			{"type":"group"},
			{"indent":1,"type":"dataview","views":[{"type":"table","name":"All"}]}]}`
		_, snapshot, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		var dvId string
		for _, b := range snapshot.Blocks {
			if b.GetDataview() != nil {
				dvId = b.Id
			}
		}
		assert.Equal(t, dataviewBlockId, dvId,
			"the pin fires only at indent 0, which is where the lift puts it")
	})
}

// TestUnmarshal_ContainerInsideATableCell covers the second of the three
// flatSubtree entry points. An unfixed cell path is worse than shipping
// nothing: it mints a real Layout_Div inside a table that no read ever shows.
func TestUnmarshal_ContainerInsideATableCell(t *testing.T) {
	doc := `{"version":2,"type":"page","blocks":[{"type":"table",
		"columns":[{"id":"c1"}],
		"rows":[{"id":"r1","cells":[[
			{"type":"paragraph","text":"cell"},
			{"indent":1,"type":"group"},
			{"indent":2,"type":"paragraph","text":"under the container"}]]}]}]}`
	require.NoError(t, Validate([]byte(doc)))

	_, snapshot, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	for _, b := range snapshot.Blocks {
		require.NotEqual(t, "DIV", blockKind(b), "a container inside a cell must not become a Layout_Div")
	}
	// the paragraph re-bases to the cell's own first level
	var cell *model.Block
	for _, b := range snapshot.Blocks {
		if b.Id == "r1-c1" {
			cell = b
		}
	}
	require.NotNil(t, cell)
	require.Len(t, cell.ChildrenIds, 1)
	byId := map[string]*model.Block{}
	for _, b := range snapshot.Blocks {
		byId[b.Id] = b
	}
	assert.Equal(t, "text:under the container", blockKind(byId[cell.ChildrenIds[0]]))
}

// TestUnmarshalBlocks_ContainerOnTheWritePath covers the third entry point —
// the API's block-write surface. A container pasted through it used to mint a
// Layout_Div straight into a live object.
func TestUnmarshalBlocks_ContainerOnTheWritePath(t *testing.T) {
	run := []json.RawMessage{
		json.RawMessage(`{"type":"group"}`),
		json.RawMessage(`{"indent":1,"type":"paragraph","text":"one"}`),
		json.RawMessage(`{"indent":1,"type":"paragraph","text":"two"}`),
	}
	blocks, topIds, err := UnmarshalBlocks(run, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	require.Len(t, blocks, 2, "the container contributes no block")
	assert.Equal(t, []string{blocks[0].Id, blocks[1].Id}, topIds,
		"both paragraphs are top-level in the run — they take the container's position")
	for _, b := range blocks {
		assert.NotEqual(t, "DIV", blockKind(b))
	}
}

// TestUnmarshalBlock_LoneContainerIsRefused: this entry point's contract is
// exactly one block. Returning zero would leave a replaceBlock silently
// replacing nothing.
func TestUnmarshalBlock_LoneContainerIsRefused(t *testing.T) {
	_, err := UnmarshalBlock(json.RawMessage(`{"type":"group"}`), "b1", Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transparent container")
	assert.Contains(t, err.Error(), "/blocks/0/type")
}

// TestValidate_ContainmentIsJudgedOnTheLiftedTree is §7a's containment rule:
// the grammar is checked against the tree import builds, and the message names the effective
// parent — or it reads as wrong to whoever wrote the group.
func TestValidate_ContainmentIsJudgedOnTheLiftedTree(t *testing.T) {
	for _, tc := range []struct {
		name   string
		blocks string
		want   string // "" = valid
	}{
		{
			// this document is REJECTED before §7a, and it is one
			// wrapChildrenToDiv produces from a row with 41 columns
			name:   "row > group > column is valid: it says row > column",
			blocks: `{"type":"row"},{"indent":1,"type":"group"},{"indent":2,"type":"column"}`,
		},
		{
			name:   "row > group (childless) is valid: a row with no columns is a legal document",
			blocks: `{"type":"row"},{"indent":1,"type":"group"}`,
		},
		{
			name:   "row > group > paragraph is reported against the row",
			blocks: `{"type":"row"},{"indent":1,"type":"group"},{"indent":2,"type":"paragraph","text":"x"}`,
			want:   "nested under a group inside a row — a row block can only contain column blocks, got paragraph",
		},
		{
			name:   "divider > group > paragraph is reported against the divider",
			blocks: `{"type":"divider"},{"indent":1,"type":"group"},{"indent":2,"type":"paragraph","text":"x"}`,
			want:   "nested under a group inside a divider block — divider blocks cannot have children",
		},
		{
			name:   "the direct message is unchanged when no container is between",
			blocks: `{"type":"row"},{"indent":1,"type":"paragraph","text":"x"}`,
			want:   "a row block can only contain column blocks, got paragraph",
		},
		{
			name:   "a container under a leaf is exempt: it becomes nothing",
			blocks: `{"type":"divider"},{"indent":1,"type":"group"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := fmt.Sprintf(`{"version":2,"type":"page","blocks":[%s]}`, tc.blocks)
			err := Validate([]byte(doc))
			if tc.want == "" {
				require.NoError(t, err)
				// I2: whatever Validate accepts, Unmarshal reads
				_, _, uerr := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
				require.NoError(t, uerr)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// TestValidate_ContainerCannotBeACellBlock: a cell is a position, not a run,
// so it is the one spelling of a container the format cannot read back —
// and Validate has to say so, or it would accept what Unmarshal refuses.
func TestValidate_ContainerCannotBeACellBlock(t *testing.T) {
	doc := `{"version":2,"type":"page","blocks":[{"type":"table",
		"columns":[{"id":"c1"}],
		"rows":[{"id":"r1","cells":[[{"type":"group"},{"indent":1,"type":"paragraph","text":"x"}]]}]}]}`

	err := Validate([]byte(doc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a cell block cannot be a group")

	_, _, uerr := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.Error(t, uerr, "Validate and Unmarshal must agree (I2)")
}

// TestUnmarshal_ContainerRoundTripsToNothing is guarantee 2 (§11) on a
// document carrying containers: `group` is a readable input token that no
// export produces, so the canonical form of a document with one is the same
// document without it.
func TestUnmarshal_ContainerRoundTripsToNothing(t *testing.T) {
	doc := []byte(`{"version":2,"type":"page","blocks":[` +
		`{"type":"group"},{"indent":1,"type":"paragraph","text":"one"},` +
		`{"type":"paragraph","text":"two"}]}`)

	sbType, snapshot, err := Unmarshal(doc, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	first, err := Marshal(sbType, snapshot, Options{OmitIds: true})
	require.NoError(t, err)

	_, again, err := Unmarshal(first, Options{GenerateId: seqIds("h")})
	require.NoError(t, err)
	second, err := Marshal(sbType, again, Options{OmitIds: true})
	require.NoError(t, err)

	assert.Equal(t, string(first), string(second))
	assert.NotContains(t, string(first), "group")
	assert.Equal(t, []string{"0  paragraph", "0  paragraph"}, blockLines(t, first))
}
