package anyblockjson

// Regression tests for the 4-lens review panel findings: adversarial
// round-trip defects, robustness/DoS quadratics, and the §8 resource bounds
// that close them.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Lens 2, finding A: a Link mark whose target is an object deep-link renders
// identically to an Object mark; it must normalize to one, or the parse-back
// type flip changes same-type overlap resolution.
func TestInline_ObjectDeepLinkNormalizes(t *testing.T) {
	marks := []*model.BlockContentTextMark{
		mark(mObject, 0, 2, "outerid"),
		mark(mLink, 1, 2, objectLinkPrefix+"innerlink"),
	}
	md1 := RenderInline("ab", marks)
	text1, marks1, err := ParseInline(md1)
	require.NoError(t, err)
	md2 := RenderInline(text1, marks1)
	require.Equal(t, md1, md2, "must be byte-stable")

	// standalone equivalence: the deep-link Link renders as an Object link
	asLink := RenderInline("x", []*model.BlockContentTextMark{mark(mLink, 0, 1, objectLinkPrefix+"id9")})
	asObject := RenderInline("x", []*model.BlockContentTextMark{mark(mObject, 0, 1, "id9")})
	assert.Equal(t, asObject, asLink)
}

// Lens 2, finding B: a ']' (or backtick) inside a link destination must be
// escaped, or it derails the enclosing label scan when links nest.
func TestInline_BracketInDestInsideLabel(t *testing.T) {
	for _, dest := range []string{"a]b", "a[b", "a`b`c"} {
		marks := []*model.BlockContentTextMark{
			mark(mObject, 0, 1, "z"),
			mark(mLink, 0, 1, dest),
		}
		md1 := RenderInline("k", marks)
		text1, marks1, err := ParseInline(md1)
		require.NoError(t, err, "dest %q", dest)
		assert.Equal(t, "k", text1)
		md2 := RenderInline(text1, marks1)
		require.Equal(t, md1, md2, "dest %q must be byte-stable", dest)
	}
}

// Lens 2, finding D: cyclic ChildrenIds must not recurse into a fatal stack
// overflow; the cycle is cut and the output still validates.
func TestExport_CyclicChildren(t *testing.T) {
	snap := &model.SmartBlockSnapshotBase{
		Details: fields(map[string]*types.Value{"id": str("obj1")}),
		Blocks: []*model.Block{
			{Id: "obj1", ChildrenIds: []string{"a"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "a", ChildrenIds: []string{"b", "a"},
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "a"}}},
			{Id: "b", ChildrenIds: []string{"a"},
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "b"}}},
		},
	}
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
	require.NoError(t, err)
	require.NoError(t, Validate(data))
}

// Lens 2, finding E: a block shared by two parents is emitted once; duplicate
// table column ids are dropped — the canonical output must pass validation.
func TestExport_SharedAndDuplicateIds(t *testing.T) {
	shared := &model.SmartBlockSnapshotBase{
		Details: fields(map[string]*types.Value{"id": str("obj1")}),
		Blocks: []*model.Block{
			{Id: "obj1", ChildrenIds: []string{"p1", "p2"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "p1", ChildrenIds: []string{"kid"},
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "1", Style: model.BlockContentText_Toggle}}},
			{Id: "p2", ChildrenIds: []string{"kid"},
				Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "2", Style: model.BlockContentText_Toggle}}},
			{Id: "kid", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "kid"}}},
		},
	}
	data, err := Marshal(model.SmartBlockType_Page, shared, Options{})
	require.NoError(t, err)
	require.NoError(t, Validate(data), "shared child must be emitted once")

	dupCols := &model.SmartBlockSnapshotBase{
		Details: fields(map[string]*types.Value{"id": str("obj1")}),
		Blocks: []*model.Block{
			{Id: "obj1", ChildrenIds: []string{"table1"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "table1", ChildrenIds: []string{"tcols", "trows"},
				Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}},
			{Id: "tcols", ChildrenIds: []string{"dup", "dup"},
				Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}}},
			{Id: "trows", ChildrenIds: []string{"r1"},
				Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}}},
			{Id: "dup", Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}},
			{Id: "r1", ChildrenIds: []string{"r1-dup"},
				Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{}}},
			{Id: "r1-dup", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "x"}}},
		},
	}
	data, err = Marshal(model.SmartBlockType_Page, dupCols, Options{})
	require.NoError(t, err)
	require.NoError(t, Validate(data), "duplicate column ids must be dropped")
}

// Lens 2, findings C and F: the CompactIds path guards nil inner content and
// never emits refs keys outside the schema charset.
func TestExport_CompactIdsHardening(t *testing.T) {
	contents := []model.IsBlockContent{
		&model.BlockContentOfText{},
		&model.BlockContentOfFile{},
		&model.BlockContentOfBookmark{},
		&model.BlockContentOfLink{},
		&model.BlockContentOfDataview{},
	}
	for _, c := range contents {
		snap := &model.SmartBlockSnapshotBase{
			Details: fields(map[string]*types.Value{"id": str("obj1")}),
			Blocks: []*model.Block{
				{Id: "obj1", ChildrenIds: []string{"b1"},
					Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
				{Id: "b1", Content: c},
			},
		}
		require.NotPanics(t, func() {
			_, _ = Marshal(model.SmartBlockType_Page, snap, Options{CompactIds: true})
		}, "content %T with CompactIds", c)
	}

	// a mention target with characters outside the refs-key charset stays
	// uncompacted instead of emitting an invalid legend key
	snap := &model.SmartBlockSnapshotBase{
		Details: fields(map[string]*types.Value{"id": str("obj1")}),
		Blocks: []*model.Block{
			{Id: "obj1", ChildrenIds: []string{"b1"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			textBlock("b1", model.BlockContentText_Paragraph, "hi",
				mark(mMention, 0, 2, "a`b"), mark(mMention, 0, 1, "bafyreiregularlylongobjectid")),
		},
	}
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{CompactIds: true})
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	// the odd id stays full (entity-encoded in the attribute per §8.1)
	assert.Contains(t, string(data), "a&#96;b")
	assert.Contains(t, string(data), `"ectid": "bafyreiregularlylongobjectid"`, "valid ids still compact")
}

// Lens 3: the parse boundary must stay effectively linear. Every input here
// took quadratic time before the rework (16s+ at 400KB); the generous bounds
// only trip on a reintroduced O(n²).
func TestInline_ParseIsLinearish(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	cases := []struct {
		name string
		md   string
	}{
		{"plain 1MB", strings.Repeat("lorem ipsum dolor sit amet ", 40000)},
		{"unmatched brackets", strings.Repeat("[", 200000)},
		{"unmatched emphasis", strings.Repeat("*a ", 70000)},
		{"backtick staircase", func() string {
			var b strings.Builder
			for i := 1; b.Len() < 200000; i++ {
				b.WriteString(strings.Repeat("`", i%40+1))
				b.WriteString("x")
			}
			return b.String()
		}()},
		{"nested links", strings.Repeat("[a](u)", 30000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			_, _, err := ParseInline(tc.md)
			require.NoError(t, err)
			require.Less(t, time.Since(start), 10*time.Second, "quadratic behavior reintroduced")
		})
	}
}

// Lens 3: the §8 resource bounds are deterministic local rules.
func TestInline_ResourceBounds(t *testing.T) {
	// an over-long link destination is not recognized; the mark drops on
	// export and the text stays intact
	longDest := "https://x.io/" + strings.Repeat("a", maxLinkDestLen)
	md := RenderInline("ab", []*model.BlockContentTextMark{mark(mLink, 0, 2, longDest)})
	assert.Equal(t, "ab", md, "over-long link param is dropped")

	doc := fmt.Sprintf("[a](%s)", longDest)
	text, marks, err := ParseInline(doc)
	require.NoError(t, err)
	assert.Empty(t, marks)
	assert.Equal(t, doc, text, "over-long dest stays literal")

	// an over-long emoji param is invalid and dropped
	md = RenderInline("ab", []*model.BlockContentTextMark{
		mark(mEmoji, 0, 1, strings.Repeat("😀", maxEmojiParamLen)),
	})
	assert.Equal(t, "ab", md)

	// links nested beyond the cap stay literal but still parse cleanly
	deep := strings.Repeat("[", 40) + "x" + strings.Repeat("](u)", 40)
	text, _, err = ParseInline(deep)
	require.NoError(t, err)
	assert.Contains(t, text, "x")
	canonical := RenderInline(text, nil)
	text2, marks2, err := ParseInline(canonical)
	require.NoError(t, err)
	assert.Equal(t, text, text2)
	assert.Empty(t, marks2)
}
