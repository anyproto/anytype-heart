package md

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// textBlockWithMarks builds one text block carrying the given marks.
func textBlockWithMarks(id, text string, style model.BlockContentTextStyle, marks ...*model.BlockContentTextMark) simple.Block {
	return simple.New(&model.Block{
		Id: id,
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:  text,
			Style: style,
			Marks: &model.BlockContentTextMarks{Marks: marks},
		}},
	})
}

// GO-7514: a code span cannot hold a line break — CommonMark converts line
// endings inside a span to spaces. Emitting one anyway put the widened delimiter
// at the start of a line with the content on the NEXT line, which reads as a
// fenced code block opener with a clean info string. The fence then swallowed
// every following block, so two objects came back as one.
func TestMD_MultilineCodeSpanDoesNotSwallowFollowingBlocks(t *testing.T) {
	for _, style := range []struct {
		name  string
		style model.BlockContentTextStyle
	}{
		{"paragraph", model.BlockContentText_Paragraph},
		{"quote", model.BlockContentText_Quote},
	} {
		t.Run(style.name, func(t *testing.T) {
			// given: a block whose code span spans a line break and whose content
			// holds a backtick run, so the delimiter widens to three
			const marked = "x\na``b"
			blocks := map[string]simple.Block{
				"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"b1", "b2"}}),
				"b1": textBlockWithMarks("b1", marked, style.style,
					mark(0, int32(len([]rune(marked))), model.BlockContentTextMark_Keyboard, "")),
				"b2": textBlockWithMarks("b2", "AFTER", model.BlockContentText_Paragraph),
			}
			s := state.NewDoc("root", blocks).(*state.State)

			// when
			md := string(NewMDConverter(s, &testFileNamer{}, false).Convert(model.SmartBlockType_Page))
			parsed, _, err := anymark.MarkdownToBlocks([]byte(md), "", nil)

			// then: the following block must survive as its own block
			require.NoError(t, err)
			require.Len(t, parsed, 2,
				"a multiline code span must not swallow the next block, exported markdown was %q, got:\n%+v", md, parsed)
			assert.Equal(t, "AFTER", parsed[1].GetText().Text,
				"the following block must be intact, exported markdown was %q", md)

			// and: the export must not contain a line-initial fence opener
			assert.NotContains(t, md, "```x",
				"a backtick fence opener with a clean info string must never be emitted, got %q", md)

			// The marked text itself is only asserted for Paragraph. A multi-line
			// Quote loses its continuation lines on reimport for ANY content —
			// "> x   \n> y   " comes back as just "x" with no marks involved at
			// all — which is a pre-existing importer defect (the importer is
			// byte-identical to develop here), not something this change owns.
			if style.style == model.BlockContentText_Paragraph {
				assert.Equal(t, marked, parsed[0].GetText().Text,
					"the marked text must round-trip exactly, exported markdown was %q", md)
			}
		})
	}
}

// The mark is split into one span per line, so each line carries its own code
// formatting and the line break is carried by the block instead.
func TestMD_MultilineCodeSpanIsSplitPerLine(t *testing.T) {
	// given
	const marked = "x\ny"
	want := "`x`\n`y`   \n"

	// when
	got := exportStyled(marked, model.BlockContentText_Paragraph, keyboardMark(marked))

	// then
	assert.Equal(t, want, got)
}

func TestMD_SplitCodeSpanMarks(t *testing.T) {
	kbd := func(from, to int32) *model.BlockContentTextMark {
		return mark(from, to, model.BlockContentTextMark_Keyboard, "")
	}
	for _, tc := range []struct {
		name  string
		text  string
		marks []*model.BlockContentTextMark
		want  []*model.Range
	}{
		{
			name:  "single line mark is untouched",
			text:  "abc",
			marks: []*model.BlockContentTextMark{kbd(0, 3)},
			want:  []*model.Range{{From: 0, To: 3}},
		},
		{
			name:  "mark spanning one break is split in two",
			text:  "ab\ncd",
			marks: []*model.BlockContentTextMark{kbd(0, 5)},
			want:  []*model.Range{{From: 0, To: 2}, {From: 3, To: 5}},
		},
		{
			name:  "mark spanning two breaks is split in three",
			text:  "a\nb\nc",
			marks: []*model.BlockContentTextMark{kbd(0, 5)},
			want:  []*model.Range{{From: 0, To: 1}, {From: 2, To: 3}, {From: 4, To: 5}},
		},
		{
			name:  "consecutive breaks produce no empty span",
			text:  "a\n\nb",
			marks: []*model.BlockContentTextMark{kbd(0, 4)},
			want:  []*model.Range{{From: 0, To: 1}, {From: 3, To: 4}},
		},
		{
			name:  "a non-keyboard mark spanning a break is untouched",
			text:  "ab\ncd",
			marks: []*model.BlockContentTextMark{mark(0, 5, model.BlockContentTextMark_Bold, "")},
			want:  []*model.Range{{From: 0, To: 5}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			text := &model.BlockContentText{
				Text:  tc.text,
				Marks: &model.BlockContentTextMarks{Marks: tc.marks},
			}

			// when
			got := splitCodeSpanMarks(text)

			// then
			require.Len(t, got, len(tc.want))
			for i, want := range tc.want {
				assert.Equal(t, *want, *got[i].Range, "mark %d", i)
			}
		})
	}
}

// GFM resolves table cell boundaries BEFORE inline parsing, so a pipe inside a
// table cell has to stay escaped even within a code span — it is the one
// character a code span does not make literal. Unescaped, it split the cell in
// half and the neighbouring cell's content was discarded outright.
func TestMD_PipeStaysEscapedInsideATableCellCodeSpan(t *testing.T) {
	// given
	s := tableWithCell("ls | wc", keyboardMark("ls | wc"))

	// when
	md := string(NewMDConverter(s, &testFileNamer{}, false).Convert(model.SmartBlockType_Page))
	parsed, _, err := anymark.MarkdownToBlocks([]byte(md), "", nil)

	// then
	require.NoError(t, err)
	assert.Contains(t, md, `\|`,
		"a pipe inside a table cell code span must stay escaped, got %q", md)

	var cells []string
	for _, b := range parsed {
		if tb := b.GetText(); tb != nil && tb.Text != "" {
			cells = append(cells, tb.Text)
		}
	}
	require.Len(t, cells, 4,
		"the row must keep both cells, exported markdown was %q, got cells %q", md, cells)
	assert.Contains(t, cells[3], "second",
		"the neighbouring cell must not be discarded, exported markdown was %q, got cells %q", md, cells)
	assert.Contains(t, cells[2], "wc",
		"the command cell must not be split, exported markdown was %q, got cells %q", md, cells)
}

// The pipe exception is scoped to table cells: everywhere else a pipe inside a
// code span is ordinary literal content and must NOT be escaped.
func TestMD_PipeIsNotEscapedInACodeSpanOutsideATable(t *testing.T) {
	// given / when
	got := exportStyled("ls | wc", model.BlockContentText_Paragraph, keyboardMark("ls | wc"))

	// then
	assert.Equal(t, "`ls | wc`   \n", got)
}

// GO-7514: the converter is reused across every block of a document, so the
// per-block code span state must be reset in Init. Without the reset, a block
// following a code-span-bearing block inherited its ranges and stopped being
// escaped. Every other test builds a fresh converter per case, so this is the
// only one that can see it.
func TestMD_CodeSpanStateDoesNotLeakBetweenBlocksOfOneDocument(t *testing.T) {
	// given: a code-span block followed by a block of literal punctuation
	blocks := map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"b1", "b2"}}),
		"b1": textBlockWithMarks("b1", "code", model.BlockContentText_Paragraph,
			keyboardMark("code")),
		"b2": textBlockWithMarks("b2", "*a* z", model.BlockContentText_Paragraph),
	}
	s := state.NewDoc("root", blocks).(*state.State)
	want := "`code`   \n\\*a\\* z   \n"

	// when: both blocks go through ONE converter
	md := string(NewMDConverter(s, &testFileNamer{}, false).Convert(model.SmartBlockType_Page))

	// then
	assert.Equal(t, want, md)

	parsed, _, err := anymark.MarkdownToBlocks([]byte(md), "", nil)
	require.NoError(t, err)
	require.Len(t, parsed, 2)
	for _, m := range parsed[1].GetText().GetMarks().GetMarks() {
		assert.NotEqual(t, model.BlockContentTextMark_Italic, m.Type,
			"the second block's literal asterisks must not become italic, exported markdown was %q", md)
	}
}

// The code span range is half-open: the rune at its start IS inside, the rune at
// its end is NOT. Every other fixture marks the whole text, so neither boundary
// is otherwise exercised.
func TestMD_CodeSpanRangeBoundaries(t *testing.T) {
	t.Run("punctuation before the span is still escaped", func(t *testing.T) {
		// given: "*a* " is literal, only "x" is a code span
		const text = "*a* x"
		want := "\\*a\\* `x`   \n"

		// when
		got := exportStyled(text, model.BlockContentText_Paragraph,
			mark(4, 5, model.BlockContentTextMark_Keyboard, ""))

		// then
		assert.Equal(t, want, got)

		parsed, _, err := anymark.MarkdownToBlocks([]byte(got), "", nil)
		require.NoError(t, err)
		require.NotEmpty(t, parsed)
		for _, m := range parsed[0].GetText().GetMarks().GetMarks() {
			assert.NotEqual(t, model.BlockContentTextMark_Italic, m.Type,
				"asterisks before the span must not become italic, exported markdown was %q", got)
		}
	})

	t.Run("punctuation after the span is still escaped", func(t *testing.T) {
		// given: only "x" is a code span; the backtick right after it is literal
		const text = "x`y"
		want := "`x`\\`y   \n"

		// when
		got := exportStyled(text, model.BlockContentText_Paragraph,
			mark(0, 1, model.BlockContentTextMark_Keyboard, ""))

		// then: the trailing backtick must not be swallowed into the delimiter
		assert.Equal(t, want, got)
	})

	t.Run("imported mark range covers exactly the span", func(t *testing.T) {
		// given
		const text = "ab x cd"

		// when
		got := exportStyled(text, model.BlockContentText_Paragraph,
			mark(3, 4, model.BlockContentTextMark_Keyboard, ""))
		parsed, _, err := anymark.MarkdownToBlocks([]byte(got), "", nil)

		// then
		require.NoError(t, err)
		require.NotEmpty(t, parsed)
		tb := parsed[0].GetText()
		assert.Equal(t, text, tb.Text, "exported markdown was %q", got)
		require.Len(t, tb.GetMarks().GetMarks(), 1, "exported markdown was %q", got)
		gotMark := tb.GetMarks().GetMarks()[0]
		assert.Equal(t, model.BlockContentTextMark_Keyboard, gotMark.Type)
		assert.Equal(t, model.Range{From: 3, To: 4}, *gotMark.Range,
			"the code span must come back covering exactly the same range, exported markdown was %q", got)
	})
}

// tableWithCell builds a 2x2 GFM table whose second-row first cell holds the
// given text and marks, and whose neighbour holds "second".
func tableWithCell(cellText string, marks ...*model.BlockContentTextMark) *state.State {
	blocks := map[string]simple.Block{
		"root":    simple.New(&model.Block{Id: "root", ChildrenIds: []string{"table"}}),
		"table":   simple.New(&model.Block{Id: "table", ChildrenIds: []string{"columns", "rows"}, Content: &model.BlockContentOfTable{Table: &model.BlockContentTable{}}}),
		"columns": simple.New(&model.Block{Id: "columns", ChildrenIds: []string{"col1", "col2"}, Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableColumns}}}),
		"col1":    simple.New(&model.Block{Id: "col1", Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}}),
		"col2":    simple.New(&model.Block{Id: "col2", Content: &model.BlockContentOfTableColumn{TableColumn: &model.BlockContentTableColumn{}}}),
		"rows":    simple.New(&model.Block{Id: "rows", ChildrenIds: []string{"row1", "row2"}, Content: &model.BlockContentOfLayout{Layout: &model.BlockContentLayout{Style: model.BlockContentLayout_TableRows}}}),
		"row1":    simple.New(&model.Block{Id: "row1", ChildrenIds: []string{"row1-col1", "row1-col2"}, Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{IsHeader: true}}}),
		"row1-col1": simple.New(&model.Block{Id: "row1-col1",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "H1"}}}),
		"row1-col2": simple.New(&model.Block{Id: "row1-col2",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "H2"}}}),
		"row2": simple.New(&model.Block{Id: "row2", ChildrenIds: []string{"row2-col1", "row2-col2"}, Content: &model.BlockContentOfTableRow{TableRow: &model.BlockContentTableRow{IsHeader: false}}}),
		"row2-col1": simple.New(&model.Block{Id: "row2-col1",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: cellText, Marks: &model.BlockContentTextMarks{Marks: marks}}}}),
		"row2-col2": simple.New(&model.Block{Id: "row2-col2",
			Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "second"}}}),
	}
	return state.NewDoc("root", blocks).(*state.State)
}

// Init must reset the per-block code span state. The converter reuses one
// marksWriter for every block of a document, so without the reset the state
// accumulates: codeRanges leaks into the next block and changes its escaping
// (see TestMD_CodeSpanStateDoesNotLeakBetweenBlocksOfOneDocument), while
// codeSpans keeps every previous block's marks alive for the whole export.
// The codeSpans growth is not observable in the output — the map is keyed by
// mark pointer, so stale entries are never read — so it is asserted directly.
func TestMD_MarksWriterResetsPerBlockState(t *testing.T) {
	// given: a writer that has just rendered a block with two code spans
	h := &MD{}
	withSpans := &model.BlockContentText{
		Text: "ab cd",
		Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
			mark(0, 2, model.BlockContentTextMark_Keyboard, ""),
			mark(3, 5, model.BlockContentTextMark_Keyboard, ""),
		}},
	}
	mw := h.marksWriter(withSpans)
	require.Len(t, mw.codeRanges, 2, "precondition: the first block registers its spans")
	require.Len(t, mw.codeSpans, 2, "precondition: the first block registers its spans")

	// when: the same writer is reused for a block with no marks at all
	mw = h.marksWriter(&model.BlockContentText{Text: "plain"})

	// then
	assert.Empty(t, mw.codeRanges, "codeRanges must not leak into the next block")
	assert.Empty(t, mw.codeSpans, "codeSpans must not leak into the next block")

	// and: reused for a block with one span, only that span is registered
	mw = h.marksWriter(&model.BlockContentText{
		Text: "xy",
		Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
			mark(0, 2, model.BlockContentTextMark_Keyboard, ""),
		}},
	})
	assert.Len(t, mw.codeRanges, 1)
	assert.Len(t, mw.codeSpans, 1)
}
