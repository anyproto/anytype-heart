package clipboard

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// kalmanGain is the formula from the GO-7330 report: the Markdown parser read
// its subscripts as an emphasis pair and dropped them.
// longFormulaHead is long enough that the parser starts a new block at the line
// break that follows it, the way anymark's soft length limit works.
var longFormulaHead = `$\alpha` + strings.Repeat(` \beta_k`, 200)

const kalmanGain = `$\tilde{\mathbf{y}}_k = \mathbf{z}_k - \mathbf{H}\hat{\mathbf{x}}_{k|k-1}$`

func codeBlockPage(text string) *smarttest.SmartTest {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"code"}}))
	sb.AddBlock(simple.New(&model.Block{Id: "code", Content: &model.BlockContentOfText{
		Text: &model.BlockContentText{
			Text:  text,
			Style: model.BlockContentText_Code,
			Marks: &model.BlockContentTextMarks{},
		},
	}}))
	return sb
}

func TestStandaloneFormula(t *testing.T) {
	type want struct {
		formula string
		ok      bool
	}
	tests := []struct {
		name string
		in   string
		want want
	}{
		{name: "inline formula", in: `$\alpha_k$`, want: want{formula: `$\alpha_k$`, ok: true}},
		{name: "display formula", in: `$$\alpha_k$$`, want: want{formula: `$$\alpha_k$$`, ok: true}},
		{name: "single-line display formula", in: `$$\alpha_k$$`, want: want{formula: `$$\alpha_k$$`, ok: true}},
		{name: "pipes are allowed, a table needs a second line", in: `$\alpha_{k|k-1}$`, want: want{formula: `$\alpha_{k|k-1}$`, ok: true}},
		{name: "surrounding whitespace is trimmed", in: "  $\\alpha_k$\n", want: want{formula: `$\alpha_k$`, ok: true}},
		{name: "kalman gain formula", in: kalmanGain, want: want{formula: kalmanGain, ok: true}},

		{name: "formula inside a sentence", in: `The value $\alpha_k$ holds.`, want: want{}},
		{name: "dollar amount then bold then formula", in: `Costs $5; **evaluate** $\alpha_k$.`, want: want{}},
		{name: "text that merely starts and ends with a dollar", in: `$5 for \alpha and **bold** costs $10$`, want: want{}},
		{name: "no control sequence", in: `$**bold**$`, want: want{}},
		{name: "backslash that is not a control sequence", in: `$x\!*y*$`, want: want{}},
		{name: "text that only ends with a dollar", in: `value \alpha$`, want: want{}},
		{name: "text that only starts with a dollar", in: `$\alpha value`, want: want{}},
		{name: "formula across several lines", in: "$\\alpha\nx_k$", want: want{}},
		{name: "formula split by a carriage return", in: "$\\alpha\rx_k$", want: want{}},
		{name: "brackets could be a link", in: `$\alpha [x] y$`, want: want{}},
		{name: "backtick could be a code span", in: "$\\alpha `x` y$", want: want{}},
		{name: "angle brackets could be html", in: `$\alpha <x> y$`, want: want{}},
		{name: "currency anchors around a windows path and a link", in: `$5 for [Windows setup](https://example.com) in C:\Temp; payable in US$`, want: want{}},
		{name: "shell variables", in: `$HOME and $PWD`, want: want{}},
		{name: "plain text", in: `just some prose`, want: want{}},
		{name: "bare delimiters", in: `$$`, want: want{}},
		{name: "empty", in: ``, want: want{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// when
			formula, ok := standaloneFormula(tt.in)

			// then
			assert.Equal(t, tt.want.ok, ok)
			assert.Equal(t, tt.want.formula, formula)
		})
	}
}

func TestPasteText_standaloneFormulaKeptVerbatim(t *testing.T) {
	type want struct {
		blocks []string
		marks  int
	}
	tests := []struct {
		name     string
		textSlot string
		want     want
	}{
		{
			name:     "kalman gain formula keeps every subscript",
			textSlot: kalmanGain,
			want:     want{blocks: []string{kalmanGain}},
		},
		{
			name:     "formula with a spacing command",
			textSlot: `$d_k\!\left(\tilde{\mathbf{y}}_k\right)$`,
			want:     want{blocks: []string{`$d_k\!\left(\tilde{\mathbf{y}}_k\right)$`}},
		},
		{
			name:     "single-line display formula",
			textSlot: `$$\sum_{i=1}^{n} w_i * x_i$$`,
			want:     want{blocks: []string{`$$\sum_{i=1}^{n} w_i * x_i$$`}},
		},
		{
			name:     "copied with a trailing newline",
			textSlot: kalmanGain + "\n",
			want:     want{blocks: []string{kalmanGain}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			sb := createPage(t, createBlocks([]string{}, []string{""}, emptyMarks))

			// when
			pasteText(t, sb, "1", model.Range{}, nil, tt.textSlot)

			// then
			checkBlockText(t, sb, tt.want.blocks)
			cIds := sb.Pick("test").Model().ChildrenIds
			require.Len(t, cIds, 1)
			assert.Len(t, sb.Pick(cIds[0]).Model().GetText().Marks.GetMarks(), tt.want.marks)
		})
	}
}

// Anything that is not a formula on its own must reach the Markdown parser
// exactly as it did before, marks and all. These are the cases that a wider
// formula test would quietly break.
func TestPasteText_mixedContentStillParsedAsMarkdown(t *testing.T) {
	type want struct {
		blocks []string
		marks  []model.BlockContentTextMarkType
	}
	tests := []struct {
		name     string
		textSlot string
		want     want
	}{
		{
			name:     "dollar amount then bold then math",
			textSlot: `Costs $5; **evaluate** $x_k$.`,
			want:     want{blocks: []string{`Costs $5; evaluate $x_k$.`}, marks: []model.BlockContentTextMarkType{model.BlockContentTextMark_Bold}},
		},
		{
			name:     "backslash-escaped dollars around bold",
			textSlot: `\$HOME **and** \$PWD`,
			want:     want{blocks: []string{`\$HOME and \$PWD`}, marks: []model.BlockContentTextMarkType{model.BlockContentTextMark_Bold}},
		},
		{
			name:     "code spans around bold",
			textSlot: "Use `$HOME` and **then** `$PWD`.",
			want: want{blocks: []string{`Use $HOME and then $PWD.`}, marks: []model.BlockContentTextMarkType{
				model.BlockContentTextMark_Keyboard, model.BlockContentTextMark_Keyboard, model.BlockContentTextMark_Bold,
			}},
		},
		{
			name:     "text that merely starts and ends with a dollar",
			textSlot: `$5 for \alpha and **bold** costs $10$`,
			want:     want{blocks: []string{`$5 for \alpha and bold costs $10$`}, marks: []model.BlockContentTextMarkType{model.BlockContentTextMark_Bold}},
		},
		{
			name:     "currency anchors around a windows path and a link",
			textSlot: `$5 for [Windows setup](https://example.com) in C:\Temp; payable in US$`,
			want:     want{blocks: []string{`$5 for Windows setup in C:\Temp; payable in US$`}, marks: []model.BlockContentTextMarkType{model.BlockContentTextMark_Link}},
		},
		{
			name:     "currency anchors around a windows path and a code span",
			textSlot: "$5 for `code` in C:\\Temp; payable in US$",
			want:     want{blocks: []string{`$5 for code in C:\Temp; payable in US$`}, marks: []model.BlockContentTextMarkType{model.BlockContentTextMark_Keyboard}},
		},
		{
			name:     "emphasis with no formula in sight",
			textSlot: `a _b_ and **c** and $5`,
			want: want{blocks: []string{`a b and c and $5`}, marks: []model.BlockContentTextMarkType{
				model.BlockContentTextMark_Italic, model.BlockContentTextMark_Bold,
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			sb := createPage(t, createBlocks([]string{}, []string{""}, emptyMarks))

			// when
			pasteText(t, sb, "1", model.Range{}, nil, tt.textSlot)

			// then
			checkBlockText(t, sb, tt.want.blocks)
			cIds := sb.Pick("test").Model().ChildrenIds
			require.Len(t, cIds, 1)
			var got []model.BlockContentTextMarkType
			for _, m := range sb.Pick(cIds[0]).Model().GetText().Marks.GetMarks() {
				got = append(got, m.Type)
			}
			assert.ElementsMatch(t, tt.want.marks, got)
		})
	}
}

func TestPasteText_intoCodeBlockKeepsContentVerbatim(t *testing.T) {
	type want struct {
		text string
	}
	tests := []struct {
		name     string
		textSlot string
		want     want
	}{
		{name: "shell variables", textSlot: "echo $HOME\nx=$(pwd)\necho ${VAR}", want: want{text: "echo $HOME\nx=$(pwd)\necho ${VAR}"}},
		{name: "positional arguments", textSlot: "awk '{print $1, $2}'", want: want{text: "awk '{print $1, $2}'"}},
		{name: "underscores and asterisks", textSlot: "sed -i 's/_old_/_new_/g' *.txt", want: want{text: "sed -i 's/_old_/_new_/g' *.txt"}},
		{name: "indentation with tabs", textSlot: "if true; then\n\techo hi\nfi", want: want{text: "if true; then\n\techo hi\nfi"}},
		{name: "latex formula", textSlot: kalmanGain, want: want{text: kalmanGain}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			sb := codeBlockPage("")

			// when
			pasteText(t, sb, "code", model.Range{}, nil, tt.textSlot)

			// then
			b := sb.Pick("code")
			require.NotNil(t, b)
			assert.Equal(t, tt.want.text, b.Model().GetText().Text)
			assert.Equal(t, model.BlockContentText_Code, b.Model().GetText().Style)
			assert.Empty(t, b.Model().GetText().Marks.GetMarks())
		})
	}
}

// A multi-line paste splits into several blocks, and the multi-range paste mode
// relies on the first and last of them being different blocks. Sending such a
// paste down the verbatim route would collapse it to one block and the trailing
// text of the selection would be dropped, so the fast path declines anything
// that spans lines. See GO-7330.
func TestPasteText_multiRangePasteKeepsTrailingText(t *testing.T) {
	type want struct {
		blocks []string
	}
	tests := []struct {
		name     string
		textSlot string
		want     want
	}{
		{
			name:     "long formula spanning lines still splits into blocks",
			textSlot: longFormulaHead + "\n" + `\gamma_k$`,
			want:     want{blocks: []string{"PREFIX" + longFormulaHead, `\gamma_k$SUFFIX`}},
		},
		{
			name:     "plain text spanning paragraphs",
			textSlot: "one\n\ntwo",
			want:     want{blocks: []string{"PREFIXone", "twoSUFFIX"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			sb := smarttest.New("test")
			sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"a", "b", "c"}}))
			for id, txt := range map[string]string{"a": "PREFIXmiddle", "b": "center", "c": "firstSUFFIX"} {
				sb.AddBlock(simple.New(&model.Block{Id: id, Content: &model.BlockContentOfText{
					Text: &model.BlockContentText{Text: txt, Marks: &model.BlockContentTextMarks{}},
				}}))
			}
			cb := newFixture(t, sb)

			// when
			_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
				SelectedBlockIds:  []string{"a", "b", "c"},
				SelectedTextRange: &model.Range{From: 6, To: 5},
				TextSlot:          tt.textSlot,
			}, "")

			// then
			require.NoError(t, err)
			checkBlockText(t, sb, tt.want.blocks)
		})
	}
}
