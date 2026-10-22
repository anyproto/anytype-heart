package md

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// exportStyled renders a single block of the given style with the given marks.
func exportStyled(text string, style model.BlockContentTextStyle, marks ...*model.BlockContentTextMark) string {
	s := stateWithMarks(text, style, marks...)
	return string(NewMDConverter(s, &testFileNamer{}, false).Convert(model.SmartBlockType_Page))
}

// keyboardMark marks the whole of text as a code span.
func keyboardMark(text string) *model.BlockContentTextMark {
	return mark(0, int32(len([]rune(text))), model.BlockContentTextMark_Keyboard, "")
}

// GO-7514: markdown code spans are literal — a backslash written inside one is
// content, not an escape. The renderer escaped code span contents like any other
// text, so a Keyboard mark on `a_b` exported as `a\_b` and re-imported with the
// backslash baked into the text. This affected Paragraph exactly as much as
// Quote; Quote only started showing it once it stopped bypassing the renderer.
func TestMD_CodeSpanContentsAreNotEscaped(t *testing.T) {
	for _, tc := range []struct {
		name      string
		text      string
		wantPara  string
		wantQuote string
	}{
		{
			name:      "underscore is not escaped inside a code span",
			text:      "a_b",
			wantPara:  "`a_b`   \n",
			wantQuote: "> `a_b`   \n\n",
		},
		{
			name:      "asterisk is not escaped inside a code span",
			text:      "a*b",
			wantPara:  "`a*b`   \n",
			wantQuote: "> `a*b`   \n\n",
		},
		{
			name:      "snake_case identifier",
			text:      "snake_case_name",
			wantPara:  "`snake_case_name`   \n",
			wantQuote: "> `snake_case_name`   \n\n",
		},
		{
			name:      "backtick inside the content widens the delimiter",
			text:      "a`b",
			wantPara:  "``a`b``   \n",
			wantQuote: "> ``a`b``   \n\n",
		},
		{
			name:      "double backtick run inside the content widens further",
			text:      "a``b",
			wantPara:  "```a``b```   \n",
			wantQuote: "> ```a``b```   \n\n",
		},
		{
			name:      "leading backtick is separated by padding",
			text:      "`lead",
			wantPara:  "`` `lead ``   \n",
			wantQuote: "> `` `lead ``   \n\n",
		},
		{
			name:      "trailing backtick is separated by padding",
			text:      "trail`",
			wantPara:  "`` trail` ``   \n",
			wantQuote: "> `` trail` ``   \n\n",
		},
		{
			name:      "content surrounded by spaces is padded so the parser's strip is undone",
			text:      " sp ",
			wantPara:  "`  sp  `   \n",
			wantQuote: "> `  sp  `   \n\n",
		},
		{
			name:      "plain content keeps the single backtick delimiter",
			text:      "x",
			wantPara:  "`x`   \n",
			wantQuote: "> `x`   \n\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given / when
			gotPara := exportStyled(tc.text, model.BlockContentText_Paragraph, keyboardMark(tc.text))
			gotQuote := exportStyled(tc.text, model.BlockContentText_Quote, keyboardMark(tc.text))

			// then
			assert.Equal(t, tc.wantPara, gotPara, "paragraph")
			assert.Equal(t, tc.wantQuote, gotQuote, "quote")
		})
	}
}

// The whole point of the delimiter and padding rules: the content must come back
// byte for byte, still marked as a code span.
func TestMD_CodeSpanRoundTripsExactly(t *testing.T) {
	for _, text := range []string{
		"a_b",
		"a*b",
		"snake_case_name",
		"a`b",
		"a``b",
		"`lead",
		"trail`",
		" sp ",
		"x",
		"a\\b",
		"**not bold**",
	} {
		for _, style := range []struct {
			name  string
			style model.BlockContentTextStyle
		}{
			{"paragraph", model.BlockContentText_Paragraph},
			{"quote", model.BlockContentText_Quote},
		} {
			t.Run(style.name+" "+text, func(t *testing.T) {
				// given
				md := exportStyled(text, style.style, keyboardMark(text))

				// when
				parsed, _, err := anymark.MarkdownToBlocks([]byte(md), "", nil)

				// then
				require.NoError(t, err)
				require.NotEmpty(t, parsed, "exported markdown was %q", md)
				got := parsed[0].GetText()
				require.NotNil(t, got, "exported markdown was %q", md)
				assert.Equal(t, text, got.Text, "exported markdown was %q", md)

				var hasKeyboard bool
				for _, m := range got.GetMarks().GetMarks() {
					if m.Type == model.BlockContentTextMark_Keyboard {
						hasKeyboard = true
					}
				}
				assert.True(t, hasKeyboard,
					"the code span mark must survive, exported markdown was %q, got marks %v", md, got.GetMarks().GetMarks())
			})
		}
	}
}

// Text OUTSIDE a code span must still be escaped. Without escaping, literal
// punctuation typed by the user is re-read as markdown syntax on import: "a*b*c"
// comes back as "abc" with a spurious Italic mark. Note the committed tests used
// punctuation-free text, so removing escaping entirely used to pass them all.
func TestMD_LiteralPunctuationOutsideCodeSpansIsEscaped(t *testing.T) {
	for _, tc := range []struct {
		name      string
		text      string
		wantPara  string
		wantQuote string
	}{
		{
			name:      "asterisks are escaped",
			text:      "a*b*c",
			wantPara:  "a\\*b\\*c   \n",
			wantQuote: "> a\\*b\\*c   \n\n",
		},
		{
			name:      "underscores are escaped",
			text:      "snake_case",
			wantPara:  "snake\\_case   \n",
			wantQuote: "> snake\\_case   \n\n",
		},
		{
			name:      "backticks are escaped",
			text:      "a`b",
			wantPara:  "a\\`b   \n",
			wantQuote: "> a\\`b   \n\n",
		},
		{
			name:      "double asterisks are escaped",
			text:      "x **bold** y",
			wantPara:  "x \\*\\*bold\\*\\* y   \n",
			wantQuote: "> x \\*\\*bold\\*\\* y   \n\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given / when
			gotPara := exportStyled(tc.text, model.BlockContentText_Paragraph)
			gotQuote := exportStyled(tc.text, model.BlockContentText_Quote)

			// then
			assert.Equal(t, tc.wantPara, gotPara, "paragraph")
			assert.Equal(t, tc.wantQuote, gotQuote, "quote")
		})
	}
}

// The behavioural half of the escaping guard: literal punctuation must not be
// re-interpreted as emphasis when the export is read back.
//
// This asserts only that no emphasis MARK is invented. It deliberately does not
// assert the text comes back byte-identical: the importer does not strip the
// backslash from an escaped character, so "snake_case" currently round-trips as
// "snake\_case". That is a separate, pre-existing importer defect affecting every
// style, tracked outside this change.
func TestMD_LiteralPunctuationDoesNotBecomeEmphasisOnReimport(t *testing.T) {
	for _, style := range []struct {
		name  string
		style model.BlockContentTextStyle
	}{
		{"paragraph", model.BlockContentText_Paragraph},
		{"quote", model.BlockContentText_Quote},
	} {
		t.Run(style.name, func(t *testing.T) {
			// given
			md := exportStyled("a*b*c and x **bold** y", style.style)

			// when
			parsed, _, err := anymark.MarkdownToBlocks([]byte(md), "", nil)

			// then
			require.NoError(t, err)
			require.NotEmpty(t, parsed, "exported markdown was %q", md)
			got := parsed[0].GetText()
			require.NotNil(t, got)
			for _, m := range got.GetMarks().GetMarks() {
				assert.NotEqual(t, model.BlockContentTextMark_Italic, m.Type,
					"literal asterisks must not become italic, exported markdown was %q", md)
				assert.NotEqual(t, model.BlockContentTextMark_Bold, m.Type,
					"literal asterisks must not become bold, exported markdown was %q", md)
			}
			assert.Contains(t, got.Text, "bold",
				"the literal text must survive, exported markdown was %q", md)
		})
	}
}

func TestMD_MaxBacktickRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want int
	}{
		{name: "no backticks", in: "abc", want: 0},
		{name: "single", in: "a`b", want: 1},
		{name: "double", in: "a``b", want: 2},
		{name: "longest run wins", in: "`a```b``", want: 3},
		{name: "only backticks", in: "````", want: 4},
		{name: "empty", in: "", want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, maxBacktickRun(tc.in))
		})
	}
}

func TestMD_NewCodeSpan(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want codeSpan
	}{
		{name: "plain content", in: "abc", want: codeSpan{delim: "`"}},
		{name: "one backtick inside", in: "a`b", want: codeSpan{delim: "``"}},
		{name: "leading backtick needs padding", in: "`a", want: codeSpan{delim: "``", pad: true}},
		{name: "trailing backtick needs padding", in: "a`", want: codeSpan{delim: "``", pad: true}},
		{name: "spaces both ends need padding", in: " a ", want: codeSpan{delim: "`", pad: true}},
		{name: "leading space only is safe", in: " a", want: codeSpan{delim: "`"}},
		{name: "trailing space only is safe", in: "a ", want: codeSpan{delim: "`"}},
		{name: "all spaces must not be padded", in: "   ", want: codeSpan{delim: "`"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, newCodeSpan(tc.in))
		})
	}
}
