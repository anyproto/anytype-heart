package anyblockjson

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

//
// ---- helpers ----
//

func str(s string) *types.Value {
	return &types.Value{Kind: &types.Value_StringValue{StringValue: s}}
}

func num(f float64) *types.Value {
	return &types.Value{Kind: &types.Value_NumberValue{NumberValue: f}}
}

func boolean(b bool) *types.Value {
	return &types.Value{Kind: &types.Value_BoolValue{BoolValue: b}}
}

func strList(ss ...string) *types.Value {
	vals := make([]*types.Value, 0, len(ss))
	for _, s := range ss {
		vals = append(vals, str(s))
	}
	return &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: vals}}}
}

func fields(kv map[string]*types.Value) *types.Struct {
	return &types.Struct{Fields: kv}
}

func textBlock(id string, style model.BlockContentTextStyle, text string, marks ...*model.BlockContentTextMark) *model.Block {
	t := &model.BlockContentText{Style: style, Text: text}
	if len(marks) > 0 {
		t.Marks = &model.BlockContentTextMarks{Marks: marks}
	}
	return &model.Block{Id: id, Content: &model.BlockContentOfText{Text: t}}
}

// seqIds returns a deterministic id generator for import tests.
func seqIds(prefix string) func() string {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("%s%d", prefix, n)
	}
}

type testOptionResolver struct {
	idToName map[string]string
	nameToId map[string]string
}

func (r *testOptionResolver) OptionName(_ domain.RelationKey, id string) (string, bool) {
	n, ok := r.idToName[id]
	return n, ok
}

func (r *testOptionResolver) OptionId(_ domain.RelationKey, name string) (string, bool) {
	id, ok := r.nameToId[name]
	return id, ok
}

var testResolver = &testOptionResolver{
	idToName: map[string]string{"opt1": "In progress", "opt2": "Done"},
	nameToId: map[string]string{"In progress": "opt1", "Done": "opt2"},
}

func testFormatResolver(key domain.RelationKey) (model.RelationFormat, bool) {
	switch key {
	case "customStatus":
		return model.RelationFormat_status, true
	case "customDate":
		return model.RelationFormat_date, true
	}
	return 0, false
}

func testOptions() Options {
	return Options{
		ResolveFormat:  testFormatResolver,
		ResolveOptions: testResolver,
	}
}

// richSnapshot builds a snapshot exercising every §5 block family plus
// structural blocks, properties, and store content.
func richSnapshot() *model.SmartBlockSnapshotBase {
	objectId := "bafyreiobject"
	blocks := []*model.Block{
		{
			Id: objectId,
			ChildrenIds: []string{
				"header", "b1", "b2", "b3", "b5", "b6", "b7", "b8", "b9",
				"row1", "table1", "b10", "dv1",
			},
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		},
		// structural: dropped on export (§7)
		{Id: "header", ChildrenIds: []string{"title", "descr", "featured"},
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Header}}},
		textBlock("title", model.BlockContentText_Title, "Project Phoenix"),
		textBlock("descr", model.BlockContentText_Description, "The subtitle"),
		{Id: "featured", Content: &model.BlockContentOfFeaturedRelations{FeaturedRelations: &model.BlockContentFeaturedRelations{}}},

		textBlock("b1", model.BlockContentText_Header2, "Goals"),
		textBlock("b2", model.BlockContentText_Paragraph, "Ship the new export with Roman",
			mark(mBold, 9, 19, ""), mark(mMention, 25, 30, "bafyreiroman")),
		textBlock("b3", model.BlockContentText_Marked, "Nested item"),
		textBlock("b5", model.BlockContentText_Checkbox, "Draft spec"),
		{Id: "b6", Fields: fields(map[string]*types.Value{"lang": str("go")}),
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Style: model.BlockContentText_Code, Text: "func main() {\n\tprintln(\"hi\")\n}",
			}}},
		{Id: "b7", Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{Style: model.BlockContentDiv_Dots}}},
		{Id: "b8", Content: &model.BlockContentOfFile{File: &model.BlockContentFile{
			Type: model.BlockContentFile_Image, TargetObjectId: "bafyreiimage",
			Name: "cat.png", Mime: "image/png", Size_: 2048,
			State: model.BlockContentFile_Done, AddedAt: 1751791445,
		}}},
		{Id: "b9", Content: &model.BlockContentOfBookmark{Bookmark: &model.BlockContentBookmark{
			Url: "https://anytype.io", TargetObjectId: "bafyreibookmark",
			State: model.BlockContentBookmark_Done,
		}}},
		{Id: "row1", ChildrenIds: []string{"col1", "col2"},
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Row}}},
		{Id: "col1", ChildrenIds: []string{"b11"},
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Column}}},
		{Id: "col2", ChildrenIds: []string{"b12"},
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_Column}}},
		textBlock("b11", model.BlockContentText_Paragraph, "left"),
		textBlock("b12", model.BlockContentText_Paragraph, "right"),

		// table subtree (§6.1)
		{Id: "table1", ChildrenIds: []string{"tcols", "trows"},
			Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}},
		{Id: "tcols", ChildrenIds: []string{"c1", "c2"},
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}}},
		{Id: "trows", ChildrenIds: []string{"r2", "r1"},
			Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}}},
		{Id: "c1", Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}},
		{Id: "c2", Fields: fields(map[string]*types.Value{"width": num(120)}),
			Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}},
		// r2 stored before r1, but r1 is the header: export reorders
		{Id: "r1", ChildrenIds: []string{"r1-c1", "r1-c2"},
			Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{IsHeader: true}}},
		{Id: "r2", ChildrenIds: []string{"r2-c2"},
			Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}},
		textBlock("r1-c1", model.BlockContentText_Paragraph, "Name"),
		textBlock("r1-c2", model.BlockContentText_Paragraph, "Status"),
		{Id: "r2-c2", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Style: model.BlockContentText_Checkbox, Text: "done", Checked: true,
		}}},

		{Id: "b10", Content: &model.BlockContentOfLatex{Latex: &model.BlockContentLatex{
			Processor: model.BlockContentLatex_Mermaid, Text: "graph TD; A-->B",
		}}},

		// dataview (§6.2)
		{Id: "dv1", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
			TargetObjectId: "bafyreitasks",
			RelationLinks: []*model.RelationLink{
				{Key: "name", Format: model.RelationFormat_shorttext},
				{Key: "customStatus", Format: model.RelationFormat_status},
				{Key: "dueDate", Format: model.RelationFormat_date},
			},
			Views: []*model.BlockContentDataviewView{{
				Id:               "v1",
				Type:             model.BlockContentDataviewView_Kanban,
				Name:             "By status",
				GroupRelationKey: "customStatus",
				Sorts: []*model.BlockContentDataviewSort{{
					RelationKey:    "dueDate",
					Format:         model.RelationFormat_date,
					EmptyPlacement: model.BlockContentDataviewSort_End,
					Id:             "s1",
				}},
				Filters: []*model.BlockContentDataviewFilter{
					{
						Id:          "f1",
						RelationKey: "dueDate",
						Condition:   model.BlockContentDataviewFilter_Less,
						QuickOption: model.BlockContentDataviewFilter_CurrentWeek,
						Format:      model.RelationFormat_date,
					},
					{
						Operator: model.BlockContentDataviewFilter_Or,
						NestedFilters: []*model.BlockContentDataviewFilter{
							{
								Id:          "f2",
								RelationKey: "customStatus",
								Condition:   model.BlockContentDataviewFilter_In,
								Value:       strList("opt1", "opt2"),
								Format:      model.RelationFormat_status,
							},
							{
								Id:          "f3",
								RelationKey: "done",
								Condition:   model.BlockContentDataviewFilter_Empty,
								Value:       boolean(false),
							},
						},
					},
				},
				Relations: []*model.BlockContentDataviewRelation{
					{Key: "name", IsVisible: true},
					{Key: "dueDate", IsVisible: false, Width: 120,
						Formula: model.BlockContentDataviewRelation_CountDistinct,
						Align:   model.Block_AlignRight},
				},
			}},
			GroupOrders: []*model.BlockContentDataviewGroupOrder{{
				ViewId: "v1",
				ViewGroups: []*model.BlockContentDataviewViewGroup{
					{GroupId: "g2", Index: 1, Hidden: true},
					{GroupId: "g1", Index: 0, BackgroundColor: "red"},
				},
			}},
			ObjectOrders: []*model.BlockContentDataviewObjectOrder{{
				ViewId: "v1", GroupId: "g1", ObjectIds: []string{"bafyreitask1"},
			}},
		}}},
	}
	return &model.SmartBlockSnapshotBase{
		Blocks: blocks,
		Details: fields(map[string]*types.Value{
			"id":             str("bafyreiobject"),
			"name":           str("Project Phoenix"),
			"description":    str("The subtitle"),
			"iconEmoji":      str("🔥"),
			"type":           str("bafyreitypepage"),
			"createdDate":    num(1751791445),
			"lastOpenedDate": num(1751791445), // local: stripped
			"customStatus":   str("opt1"),
			"customDate":     num(1751791445),
			"assignee":       strList("bafyreiroman"),
		}),
		ObjectTypes: []string{"ot-page"},
	}
}

func TestMarshal_ProducesValidDocument(t *testing.T) {
	data, err := Marshal(model.SmartBlockType_Page, richSnapshot(), testOptions())
	require.NoError(t, err)
	require.NoError(t, Validate(data))
}

// TestRoundTrip_ByteStable checks §11.2: Export ∘ Import is idempotent and
// byte-stable.
func TestRoundTrip_ByteStable(t *testing.T) {
	opts := testOptions()
	first, err := Marshal(model.SmartBlockType_Page, richSnapshot(), opts)
	require.NoError(t, err)
	require.NoError(t, Validate(first))

	impOpts := testOptions()
	impOpts.GenerateId = seqIds("gen")
	sbType, snap, err := Unmarshal(first, impOpts)
	require.NoError(t, err)
	assert.Equal(t, model.SmartBlockType_Page, sbType)

	second, err := Marshal(sbType, snap, opts)
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second), "Export ∘ Import must be byte-stable")

	impOpts.GenerateId = seqIds("gen2")
	sbType2, snap2, err := Unmarshal(second, impOpts)
	require.NoError(t, err)
	third, err := Marshal(sbType2, snap2, opts)
	require.NoError(t, err)
	assert.Equal(t, string(second), string(third))
}

// TestRoundTrip_State spot-checks Import(Export(S)) ≡ N(S) on the snapshot
// level (§11.1).
func TestRoundTrip_State(t *testing.T) {
	data, err := Marshal(model.SmartBlockType_Page, richSnapshot(), testOptions())
	require.NoError(t, err)
	impOpts := testOptions()
	impOpts.GenerateId = seqIds("gen")
	sbType, snap, err := Unmarshal(data, impOpts)
	require.NoError(t, err)
	assert.Equal(t, model.SmartBlockType_Page, sbType)
	assert.Equal(t, []string{"ot-page"}, snap.ObjectTypes)

	byId := map[string]*model.Block{}
	for _, b := range snap.Blocks {
		byId[b.Id] = b
	}

	// root block regenerated with the object id; structural blocks stay absent
	root := byId["bafyreiobject"]
	require.NotNil(t, root)
	for _, id := range root.ChildrenIds {
		require.NotNil(t, byId[id], "child %s missing", id)
	}

	// marks: offsets and resolved mention target (§8)
	b2 := byId["b2"].Content.(*model.BlockContentOfText).Text
	require.NotNil(t, b2.Marks)
	want := []*model.BlockContentTextMark{
		mark(mBold, 9, 19, ""),
		mark(mMention, 25, 30, "bafyreiroman"),
	}
	assert.Equal(t, want, b2.Marks.Marks)

	// code: language back into fields.lang, literal text (§5.1, §8.4)
	b6 := byId["b6"]
	assert.Equal(t, "go", b6.Fields.Fields["lang"].GetStringValue())
	assert.Equal(t, "func main() {\n\tprintln(\"hi\")\n}", b6.Content.(*model.BlockContentOfText).Text.Text)

	// table subtree rebuilt with derived cell ids, header row first (§6.1)
	table := byId["table1"]
	require.Len(t, table.ChildrenIds, 2)
	colsW, rowsW := byId[table.ChildrenIds[0]], byId[table.ChildrenIds[1]]
	assert.Equal(t, model.BlockContentLayout_TableColumns, colsW.Content.(*model.BlockContentOfLayout).Layout.Style)
	assert.Equal(t, []string{"c1", "c2"}, colsW.ChildrenIds)
	assert.Equal(t, []string{"r1", "r2"}, rowsW.ChildrenIds)
	assert.True(t, byId["r1"].Content.(*model.BlockContentOfTableRow).TableRow.IsHeader)
	assert.Equal(t, float64(120), byId["c2"].Fields.Fields["width"].GetNumberValue())
	require.NotNil(t, byId["r1-c1"])
	assert.Equal(t, "Name", byId["r1-c1"].Content.(*model.BlockContentOfText).Text.Text)
	// sparse cell r2-c1 stays absent
	assert.Nil(t, byId["r2-c1"])
	assert.True(t, byId["r2-c2"].Content.(*model.BlockContentOfText).Text.Checked)

	// dataview: cached formats rehydrated, option names resolved back (§6.2)
	dv := byId["dv1"].Content.(*model.BlockContentOfDataview).Dataview
	view := dv.Views[0]
	assert.Equal(t, model.RelationFormat_date, view.Sorts[0].Format)
	group := view.Filters[1]
	assert.Equal(t, model.BlockContentDataviewFilter_Or, group.Operator)
	assert.Equal(t, strList("opt1", "opt2"), group.NestedFilters[0].Value)
	assert.Equal(t, model.RelationFormat_status, group.NestedFilters[0].Format)
	// value dropped on presence-only conditions (§11)
	assert.Nil(t, group.NestedFilters[1].Value)
	require.Len(t, dv.GroupOrders, 1)
	assert.Equal(t, "g1", dv.GroupOrders[0].ViewGroups[0].GroupId)
	assert.Equal(t, int32(0), dv.GroupOrders[0].ViewGroups[0].Index)

	// details: dates back to unix seconds, select names to option ids
	assert.Equal(t, float64(1751791445), snap.Details.Fields["createdDate"].GetNumberValue())
	assert.Equal(t, float64(1751791445), snap.Details.Fields["customDate"].GetNumberValue())
	assert.Equal(t, strList("opt1"), snap.Details.Fields["customStatus"])
	assert.Equal(t, strList("bafyreiroman"), snap.Details.Fields["assignee"])
	// stripped local property does not resurrect
	assert.Nil(t, snap.Details.Fields["lastOpenedDate"])
}

func TestOmitIds(t *testing.T) {
	opts := testOptions()
	opts.OmitIds = true
	data, err := Marshal(model.SmartBlockType_Page, richSnapshot(), opts)
	require.NoError(t, err)
	require.NoError(t, Validate(data))

	s := string(data)
	// no block/row/column/view/sort/filter ids; id-dependent view state gone
	assert.NotContains(t, s, `"id": "b1"`)
	assert.NotContains(t, s, `"id": "r1"`)
	assert.NotContains(t, s, `"id": "c1"`)
	assert.NotContains(t, s, `"id": "v1"`)
	assert.NotContains(t, s, `"id": "s1"`)
	assert.NotContains(t, s, `"id": "f1"`)
	assert.NotContains(t, s, `"groups"`)
	assert.NotContains(t, s, `"objectOrders"`)
	// the envelope id stays (§9)
	assert.Contains(t, s, `"id": "bafyreiobject"`)

	impOpts := testOptions()
	impOpts.GenerateId = seqIds("gen")
	_, snap, err := Unmarshal(data, impOpts)
	require.NoError(t, err)
	assert.NotEmpty(t, snap.Blocks)
}

func TestCompactIds(t *testing.T) {
	opts := testOptions()
	opts.CompactIds = true
	data, err := Marshal(model.SmartBlockType_Page, richSnapshot(), opts)
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	s := string(data)

	// a refs legend appears, mentions use short labels
	assert.Contains(t, s, `"refs"`)
	assert.Contains(t, s, `"roman": "bafyreiroman"`)
	assert.Contains(t, s, `<mention objectId=\"roman\">`)
	// stripped properties leave no unused legend entries (§9a)
	assert.NotContains(t, s, `bafyreitypepage`)
	// the envelope id is never compacted (§9a)
	assert.Contains(t, s, `"id": "bafyreiobject"`)

	// importing the compact form resolves refs back to full ids
	impOpts := testOptions()
	impOpts.GenerateId = seqIds("gen")
	_, snap, err := Unmarshal(data, impOpts)
	require.NoError(t, err)
	var mention string
	for _, b := range snap.Blocks {
		if txt, ok := b.Content.(*model.BlockContentOfText); ok && txt.Text.Marks != nil {
			for _, m := range txt.Text.Marks.Marks {
				if m.Type == model.BlockContentTextMark_Mention {
					mention = m.Param
				}
			}
		}
	}
	assert.Equal(t, "bafyreiroman", mention)
}

func TestEnvelope_Variants(t *testing.T) {
	t.Run("template", func(t *testing.T) {
		snap := &model.SmartBlockSnapshotBase{
			Details:     fields(map[string]*types.Value{"id": str("tpl1")}),
			ObjectTypes: []string{"ot-template", "ot-task"},
		}
		data, err := Marshal(model.SmartBlockType_Template, snap, Options{})
		require.NoError(t, err)
		s := string(data)
		assert.Contains(t, s, `"type": "template"`)
		assert.Contains(t, s, `"templateFor": "task"`)
		assert.NotContains(t, s, `"kind"`)

		sbType, snap2, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, model.SmartBlockType_Template, sbType)
		assert.Equal(t, []string{"ot-template", "ot-task"}, snap2.ObjectTypes)
	})

	t.Run("explicit kind", func(t *testing.T) {
		snap := &model.SmartBlockSnapshotBase{
			Details: fields(map[string]*types.Value{"id": str("p1")}),
		}
		data, err := Marshal(model.SmartBlockType_ProfilePage, snap, Options{})
		require.NoError(t, err)
		assert.Contains(t, string(data), `"kind": "profilePage"`)
		sbType, _, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, model.SmartBlockType_ProfilePage, sbType)
	})

	t.Run("collection items and store", func(t *testing.T) {
		snap := &model.SmartBlockSnapshotBase{
			Details:     fields(map[string]*types.Value{"id": str("coll1")}),
			ObjectTypes: []string{"ot-collection"},
			Collections: fields(map[string]*types.Value{
				"objects": strList("bafyreitask1", "bafyreitask2"),
				"other":   str("kept"),
			}),
		}
		data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
		require.NoError(t, err)
		s := string(data)
		assert.Contains(t, s, `"items"`)
		assert.Contains(t, s, `"bafyreitask1"`)
		assert.Contains(t, s, `"store"`)

		_, snap2, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, strList("bafyreitask1", "bafyreitask2"), snap2.Collections.Fields["objects"])
		assert.Equal(t, str("kept"), snap2.Collections.Fields["other"])

		second, err := Marshal(model.SmartBlockType_Page, snap2, Options{})
		require.NoError(t, err)
		assert.Equal(t, string(data), string(second))
	})

	t.Run("root escape hatch", func(t *testing.T) {
		snap := &model.SmartBlockSnapshotBase{
			Details: fields(map[string]*types.Value{"id": str("p2")}),
			Blocks: []*model.Block{{
				Id:              "p2",
				BackgroundColor: "grey",
				Fields:          fields(map[string]*types.Value{"custom": str("x")}),
				Content:         &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
			}},
		}
		data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
		require.NoError(t, err)
		assert.Contains(t, string(data), `"root"`)
		_, snap2, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, "grey", snap2.Blocks[0].BackgroundColor)
		assert.Equal(t, "x", snap2.Blocks[0].Fields.Fields["custom"].GetStringValue())
	})
}

func TestImport_Aliases(t *testing.T) {
	doc := `{"version": 1, "blocks": [
		{"type": "heading4", "text": "deep"},
		{"type": "header4", "text": "deeper"},
		{"type": "equation", "text": "E=mc^2"},
		{"type": "embed", "processor": "youtube", "url": "https://youtu.be/x"}
	]}`
	_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	blocks := snap.Blocks[1:] // skip root
	assert.Equal(t, model.BlockContentText_Header3, blocks[0].Content.(*model.BlockContentOfText).Text.Style)
	assert.Equal(t, model.BlockContentText_Header3, blocks[1].Content.(*model.BlockContentOfText).Text.Style)
	eq := blocks[2].Content.(*model.BlockContentOfLatex).Latex
	assert.Equal(t, model.BlockContentLatex_Latex, eq.Processor)
	assert.Equal(t, "E=mc^2", eq.Text)
	yt := blocks[3].Content.(*model.BlockContentOfLatex).Latex
	assert.Equal(t, model.BlockContentLatex_Youtube, yt.Processor)
	assert.Equal(t, "https://youtu.be/x", yt.Text)
}

func TestImport_TitleAbsorption(t *testing.T) {
	doc := `{"version": 1, "blocks": [
		{"type": "title", "text": "My **Title**"},
		{"type": "description", "text": "Sub"},
		{"type": "featuredProperties"},
		{"type": "paragraph", "text": "body"}
	]}`
	_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, "My Title", snap.Details.Fields["name"].GetStringValue())
	assert.Equal(t, "Sub", snap.Details.Fields["description"].GetStringValue())
	// only the paragraph survives under the root
	require.Len(t, snap.Blocks, 2)
	assert.Equal(t, "body", snap.Blocks[1].Content.(*model.BlockContentOfText).Text.Text)

	// when the property is already set, the block is simply dropped
	doc2 := `{"version": 1, "properties": {"name": "Kept"}, "blocks": [
		{"type": "title", "text": "Ignored"}
	]}`
	_, snap2, err := Unmarshal([]byte(doc2), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, "Kept", snap2.Details.Fields["name"].GetStringValue())
}

// TestExplicitIndentZero: an explicit "indent": 0 is accepted on input and
// canonicalized away on re-export (§4 omit-default canon).
func TestExplicitIndentZero(t *testing.T) {
	doc := `{"version": 1, "blocks": [{"indent": 0, "id": "a", "type": "paragraph", "text": "x"}]}`
	sbType, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	out, err := Marshal(sbType, snap, Options{})
	require.NoError(t, err)
	assert.NotContains(t, string(out), `"indent"`)
}

// TestGeneratedDocs_ByteStable: for generated valid documents J,
// Export(Import(J)) is canonical and re-import/re-export is byte-identical
// (§11.2).
func TestGeneratedDocs_ByteStable(t *testing.T) {
	rnd := rand.New(rand.NewSource(7))
	texts := []string{
		"plain", "**bold** and *it*", "`code` span", "a\\*b", "😀 astral 𝒜",
		"<u>under</u>", "~~gone~~", "x <mention objectId=\"bafyreiperson\">P</mention>",
		"[link](https://x.io)", "line\nbreak", "_alias_", "&lt;tag&gt;",
	}
	blockGens := []func(i int) string{
		func(i int) string {
			return fmt.Sprintf(`{"type": "paragraph", "text": %q}`, texts[i%len(texts)])
		},
		func(i int) string {
			return fmt.Sprintf(`{"type": "checkbox", "checked": %v, "text": %q}`, i%2 == 0, texts[i%len(texts)])
		},
		func(i int) string {
			return fmt.Sprintf(`{"type": "heading%d", "text": "h"},{"indent": 1, "type": "paragraph", "text": %q}`,
				1+i%3, texts[i%len(texts)])
		},
		func(i int) string {
			// a deep chain: covers the real-world ~6-level maximum and beyond
			depth := 4 + i%8
			parts := []string{fmt.Sprintf(`{"type": "bulletedListItem", "text": %q}`, texts[i%len(texts)])}
			for d := 1; d <= depth; d++ {
				parts = append(parts, fmt.Sprintf(`{"indent": %d, "type": "bulletedListItem", "text": %q}`,
					d, texts[(i+d)%len(texts)]))
			}
			return strings.Join(parts, ",")
		},
		func(i int) string {
			// mixed wide/deep: siblings appearing after a pop back up
			return fmt.Sprintf(`{"type": "toggle", "text": "t"},`+
				`{"indent": 1, "type": "paragraph", "text": %q},`+
				`{"indent": 2, "type": "paragraph", "text": "deep"},`+
				`{"indent": 1, "type": "paragraph", "text": "sibling"},`+
				`{"type": "paragraph", "text": "top"}`, texts[i%len(texts)])
		},
		func(i int) string {
			return fmt.Sprintf(`{"type": "table",
				"columns": [{"id": "ca%d"}, {"id": "cb%d"}],
				"rows": [
					{"id": "ra%d", "isHeader": true, "cells": [%q]},
					{"id": "rb%d", "cells": [null, %q]}
				]}`, i, i, i, texts[i%len(texts)], i, texts[(i+1)%len(texts)])
		},
		func(i int) string {
			return fmt.Sprintf(`{"type": "dataview", "objectId": "bafyreiset%d",
				"properties": [{"key": "name", "format": "shortText"}],
				"views": [{"type": "list", "name": "v",
					"sorts": [{"property": "name", "direction": "desc"}],
					"filters": [{"property": "name", "condition": "contains", "value": "x"}]}]}`, i)
		},
		func(i int) string {
			return fmt.Sprintf(`{"type": "code", "language": "go", "text": %q}`, texts[i%len(texts)])
		},
		func(i int) string {
			return fmt.Sprintf(`{"type": "callout", "iconEmoji": "💡", "text": %q}`, texts[i%len(texts)])
		},
	}
	for i := 0; i < 300; i++ {
		var blocks []string
		for n := 1 + rnd.Intn(5); n > 0; n-- {
			blocks = append(blocks, blockGens[rnd.Intn(len(blockGens))](rnd.Intn(1000)))
		}
		doc := fmt.Sprintf(`{"version": 1, "properties": {"name": "Doc %d"}, "blocks": [%s]}`,
			i, strings.Join(blocks, ","))
		require.NoError(t, Validate([]byte(doc)), "case %d: generated doc must be valid: %s", i, doc)

		opts := testOptions()
		opts.GenerateId = seqIds(fmt.Sprintf("a%d_", i))
		sbType, snap, err := Unmarshal([]byte(doc), opts)
		require.NoError(t, err, "case %d", i)
		canonical, err := Marshal(sbType, snap, testOptions())
		require.NoError(t, err, "case %d", i)
		// Marshal must never emit output its own Validate rejects
		require.NoError(t, Validate(canonical), "case %d: canonical must validate: %s", i, canonical)

		opts2 := testOptions()
		opts2.GenerateId = seqIds(fmt.Sprintf("b%d_", i))
		sbType2, snap2, err := Unmarshal(canonical, opts2)
		require.NoError(t, err, "case %d: canonical must re-import: %s", i, canonical)
		again, err := Marshal(sbType2, snap2, testOptions())
		require.NoError(t, err, "case %d", i)
		require.Equal(t, string(canonical), string(again), "case %d: not byte-stable", i)
	}
}
