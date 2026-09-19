package html

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// givenCodeBlock builds a single-Code-block doc with the given text.
func givenCodeBlock(text string) *state.State {
	return state.NewDoc("root", map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"code1"}}),
		"code1": simple.New(&model.Block{
			Id: "code1",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  text,
					Style: model.BlockContentText_Code,
				},
			},
		}),
	}).(*state.State)
}

// GO-7515: the Code style used to be emitted as `<code ...><pre>`. <pre> is the
// block-level, whitespace-preserving element and must be the OUTER one; with
// <code> outermost the HTML parser collapses the newlines.
func TestHTML_CodeBlockUsesPreOutsideCode(t *testing.T) {
	// given
	doc := givenCodeBlock("print(1)\nprint(2)")
	want := `<pre><code style="` + styleCode + `">print(1)
print(2)</code></pre>`

	// when
	got := convertHtml(doc)

	// then
	assert.Contains(t, got, want,
		"code block must be wrapped as <pre><code>, got:\n%s", got)
	assert.NotContains(t, got, "<code style=\""+styleCode+"\"><pre>",
		"the inverted <code><pre> nesting must be gone, got:\n%s", got)
	assert.NotContains(t, got, "</pre></code>",
		"the inverted </pre></code> closer must be gone, got:\n%s", got)

	// The monospace styling that external paste targets rely on is still carried
	// on a tag that wraps the whole code text.
	//
	// This is asserted with a LITERAL declaration rather than against styleCode,
	// on purpose: building the expectation out of the production constant makes
	// the test agree with whatever that constant says, so changing it to a
	// proportional font would still pass.
	assert.Contains(t, got, "font-family: monospace",
		"code must be styled monospace for external paste targets, got:\n%s", got)
	assert.Regexp(t, `<pre><code style="[^"]*font-family: monospace;?[^"]*">`, got,
		"the monospace declaration must sit on the <code> tag wrapping the code text, got:\n%s", got)
}

// The same markup is used by the HTML export path, not only the clipboard copy
// path — both read styleTags.
func TestHTML_CodeBlockExportUsesPreOutsideCode(t *testing.T) {
	// given
	doc := givenCodeBlock("print(1)\nprint(2)")
	want := `<pre><code style="` + styleCode + `">print(1)
print(2)</code></pre>`

	// when
	got := NewHTMLConverter(doc, nil).Export()

	// then
	assert.Contains(t, got, want, "export must use <pre><code>, got:\n%s", got)
}

// Copying a code block and pasting it back through the HTML slot must preserve
// both the Code style and every line break. Before the fix a two-line code block
// came back as a single Paragraph with the newline collapsed into a space.
//
// This drives the very same entry point the clipboard paste path uses
// (core/block/editor/clipboard/clipboard.go calls anymark.HTMLToBlocks on
// req.HtmlSlot).
func TestHTML_CodeBlockRoundTripsThroughHTMLSlot(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{
			name: "two lines",
			text: "print(1)\nprint(2)",
			want: "print(1)\nprint(2)",
		},
		{
			name: "leading indentation is preserved",
			text: "def f():\n    return 1",
			want: "def f():\n    return 1",
		},
		{
			name: "blank line inside the code is preserved",
			text: "a\n\nb",
			want: "a\n\nb",
		},
		{
			name: "three lines",
			text: "x\ny\nz",
			want: "x\ny\nz",
		},
		{
			name: "single line",
			text: "single",
			want: "single",
		},
		{
			// GO-7515: the outer <pre> makes the importer treat this as fenced
			// code, and the library's default post-processing right-trimmed every
			// line of the whole document, silently rewriting the code.
			name: "trailing spaces are preserved",
			text: "a  ",
			want: "a  ",
		},
		{
			name: "a line of nothing but spaces is preserved",
			text: "   ",
			want: "   ",
		},
		{
			name: "trailing spaces on every line are preserved",
			text: "line1  \nline2  ",
			want: "line1  \nline2  ",
		},
		{
			// The same default post-processing collapsed runs of blank lines.
			name: "consecutive blank lines inside the code are preserved",
			text: "a\n\n\n\nb",
			want: "a\n\n\n\nb",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			htmlSlot := convertHtml(givenCodeBlock(tc.text))

			// when
			parsed, _, err := anymark.HTMLToBlocks([]byte(htmlSlot), "")

			// then
			require.NoError(t, err)
			require.Len(t, parsed, 1, "expected a single block back, got:\n%+v", parsed)
			text := parsed[0].GetText()
			require.NotNil(t, text, "expected a text block, got:\n%+v", parsed)
			assert.Equal(t, model.BlockContentText_Code, text.Style,
				"pasted block must stay a code block, got:\n%+v", parsed)
			// Before the fix the code came back as a paragraph carrying an inline
			// Keyboard (code span) mark instead of being a code block.
			assert.Empty(t, text.GetMarks().GetMarks(),
				"a code block must not come back as an inline-marked paragraph, got:\n%+v", parsed)
			// The importer appends a single trailing newline to fenced code; the
			// claim under test is that the INNER line breaks survive.
			assert.Equal(t, tc.want, strings.TrimSuffix(text.Text, "\n"))
		})
	}
}

// The fence-aware post-processing must only spare FENCED content: ordinary
// paragraph text still has its trailing whitespace trimmed, and blank runs
// between blocks still collapse.
func TestHTML_OrdinaryTextIsStillTrimmedAroundCodeBlocks(t *testing.T) {
	// given
	const in = "<p>para trailing   </p><pre><code>x  </code></pre><p>after</p>"

	// when
	parsed, _, err := anymark.HTMLToBlocks([]byte(in), "")

	// then
	require.NoError(t, err)
	require.Len(t, parsed, 3, "got:\n%+v", parsed)
	assert.Equal(t, "para trailing", parsed[0].GetText().Text,
		"ordinary text must still be right-trimmed")
	assert.Equal(t, model.BlockContentText_Code, parsed[1].GetText().Style)
	assert.Equal(t, "x  \n", parsed[1].GetText().Text,
		"code content must keep its trailing spaces")
	assert.Equal(t, "after", parsed[2].GetText().Text)
}
