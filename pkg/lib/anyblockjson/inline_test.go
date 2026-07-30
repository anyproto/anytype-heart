package anyblockjson

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func mark(t model.BlockContentTextMarkType, from, to int32, param string) *model.BlockContentTextMark {
	return &model.BlockContentTextMark{Range: &model.Range{From: from, To: to}, Type: t, Param: param}
}

const (
	mBold    = model.BlockContentTextMark_Bold
	mItalic  = model.BlockContentTextMark_Italic
	mStrike  = model.BlockContentTextMark_Strikethrough
	mCode    = model.BlockContentTextMark_Keyboard
	mLink    = model.BlockContentTextMark_Link
	mObject  = model.BlockContentTextMark_Object
	mMention = model.BlockContentTextMark_Mention
	mUnder   = model.BlockContentTextMark_Underscored
	mColor   = model.BlockContentTextMark_TextColor
	mBg      = model.BlockContentTextMark_BackgroundColor
	mEmoji   = model.BlockContentTextMark_Emoji
)

// TestRenderInline_Golden checks canonical rendering and that every canonical
// form is byte-stable through parse ∘ render (§11.2).
func TestRenderInline_Golden(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		marks []*model.BlockContentTextMark
		want  string
	}{
		{"plain", "hello world", nil, "hello world"},
		{"bold", "hello world", []*model.BlockContentTextMark{mark(mBold, 0, 5, "")}, "**hello** world"},
		{"italic", "hello world", []*model.BlockContentTextMark{mark(mItalic, 6, 11, "")}, "hello *world*"},
		{"strike", "done", []*model.BlockContentTextMark{mark(mStrike, 0, 4, "")}, "~~done~~"},
		{"code", "code", []*model.BlockContentTextMark{mark(mCode, 0, 4, "")}, "`code`"},
		{"bold italic same range", "x", []*model.BlockContentTextMark{mark(mBold, 0, 1, ""), mark(mItalic, 0, 1, "")}, "***x***"},
		{"bold italic overlap", "abc", []*model.BlockContentTextMark{mark(mBold, 0, 2, ""), mark(mItalic, 1, 3, "")}, "**a*b****c*"},
		{"link", "docs here", []*model.BlockContentTextMark{mark(mLink, 0, 4, "https://x.io")}, "[docs](https://x.io) here"},
		{"object link", "docs here", []*model.BlockContentTextMark{mark(mObject, 0, 4, "bafy1")}, "[docs](anytype://object?objectId=bafy1) here"},
		{"mention", "ping Roman", []*model.BlockContentTextMark{mark(mMention, 5, 10, "bafyid")}, `ping <mention objectId="bafyid">Roman</mention>`},
		{"underline", "docs", []*model.BlockContentTextMark{mark(mUnder, 0, 4, "")}, "<u>docs</u>"},
		{"text color", "x", []*model.BlockContentTextMark{mark(mColor, 0, 1, "red")}, `<font color="red">x</font>`},
		{"background", "x", []*model.BlockContentTextMark{mark(mBg, 0, 1, "yellow")}, `<font background="yellow">x</font>`},
		{"coincident color and background", "x",
			[]*model.BlockContentTextMark{mark(mColor, 0, 1, "red"), mark(mBg, 0, 1, "yellow")},
			`<font color="red" background="yellow">x</font>`},
		{"nested color and background", "abc",
			[]*model.BlockContentTextMark{mark(mColor, 0, 3, "red"), mark(mBg, 1, 2, "yellow")},
			`<font color="red">a<font background="yellow">b</font>c</font>`},
		{"whitespace boundary shrink", " hi ", []*model.BlockContentTextMark{mark(mBold, 0, 4, "")}, " **hi** "},
		{"all whitespace mark dropped", "a b", []*model.BlockContentTextMark{mark(mBold, 1, 2, "")}, "a b"},
		{"same-type overlap truncated", "abcd",
			[]*model.BlockContentTextMark{mark(mLink, 0, 2, "u1"), mark(mLink, 1, 4, "u2")},
			"[ab](u1)[cd](u2)"},
		{"emoji materialized", "abc", []*model.BlockContentTextMark{mark(mEmoji, 1, 2, "😀")}, "a😀c"},
		{"emoji under bold", "abc",
			[]*model.BlockContentTextMark{mark(mBold, 0, 3, ""), mark(mEmoji, 1, 2, "😀")},
			"**a😀c**"},
		{"escape star", "2*3 = 6", nil, `2\*3 = 6`},
		{"star with spaces literal", "a * b", nil, "a * b"},
		{"escape backtick", "a`b", nil, "a\\`b"},
		{"escape tilde run", "~~x~~", nil, `\~\~x\~\~`},
		{"single tilde literal", "~x", nil, "~x"},
		{"escape bracket", "[note]", nil, `\[note]`},
		{"escape whitelisted tag", "<u>", nil, `\<u>`},
		{"unknown tag literal", "<div>x", nil, "<div>x"},
		{"escape entity", "&lt;", nil, `\&lt;`},
		{"bare ampersand literal", "R&D", nil, "R&D"},
		{"escape underscore", "_x_", nil, `\_x\_`},
		{"intraword underscore literal", "snake_case", nil, "snake_case"},
		{"backslash before punct", `a\*b`, nil, `a\\\*b`},
		{"backslash before letter", `a\b`, nil, `a\b`},
		{"code span with backtick", "a`b", []*model.BlockContentTextMark{mark(mCode, 0, 3, "")}, "``a`b``"},
		{"code span starts with backtick", "`x", []*model.BlockContentTextMark{mark(mCode, 0, 2, "")}, "`` `x ``"},
		{"bold inside link label", "click here",
			[]*model.BlockContentTextMark{mark(mLink, 0, 10, "https://x.io"), mark(mBold, 6, 10, "")},
			"[click **here**](https://x.io)"},
		{"parens in url", "x", []*model.BlockContentTextMark{mark(mLink, 0, 1, "http://x/a(1)")}, `[x](http://x/a\(1\))`},
		{"space in url", "x", []*model.BlockContentTextMark{mark(mLink, 0, 1, "a b")}, "[x](<a b>)"},
		{"soft line break in bold", "a\nb", []*model.BlockContentTextMark{mark(mBold, 0, 3, "")}, "**a\nb**"},
		{"astral bold", "𝒜b", []*model.BlockContentTextMark{mark(mBold, 0, 2, "")}, "**𝒜**b"},
		{"escaped chars inside mention", "see *x*",
			[]*model.BlockContentTextMark{mark(mMention, 4, 7, "id1")},
			`see <mention objectId="id1">\*x\*</mention>`},
		{"bracket inside link label", "a[b]c",
			[]*model.BlockContentTextMark{mark(mLink, 0, 5, "u")},
			`[a\[b\]c](u)`},
		{"code under bold overlap", "abc",
			[]*model.BlockContentTextMark{mark(mCode, 0, 3, ""), mark(mBold, 1, 2, "")},
			"`a`**`b`**`c`"},
		{"adjacent same type merges", "ab",
			[]*model.BlockContentTextMark{mark(mBold, 0, 1, ""), mark(mBold, 1, 2, "")},
			"**ab**"},
		{"invalid ranges dropped", "ab",
			[]*model.BlockContentTextMark{mark(mBold, 3, 5, ""), mark(mBold, 1, 1, ""), mark(mLink, 0, 1, "")},
			"ab"},
		{"zero length text", "", []*model.BlockContentTextMark{mark(mBold, 0, 0, "")}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := renderInline(tc.text, tc.marks)
			assert.Equal(t, tc.want, got)

			// canonical form must be byte-stable: parse it and render again
			txt, marks, err := parseInline(got)
			require.NoError(t, err)
			again := renderInline(txt, marks)
			assert.Equal(t, got, again, "Export ∘ Import must be byte-stable")
		})
	}
}

// TestParseInline_Golden checks the parser on canonical and liberal input.
func TestParseInline_Golden(t *testing.T) {
	tests := []struct {
		name      string
		md        string
		wantText  string
		wantMarks []*model.BlockContentTextMark
	}{
		{"plain", "hello", "hello", nil},
		{"bold", "**hi**", "hi", []*model.BlockContentTextMark{mark(mBold, 0, 2, "")}},
		{"italic star", "*hi*", "hi", []*model.BlockContentTextMark{mark(mItalic, 0, 2, "")}},
		{"italic underscore alias", "_hi_", "hi", []*model.BlockContentTextMark{mark(mItalic, 0, 2, "")}},
		{"bold underscore alias", "__hi__", "hi", []*model.BlockContentTextMark{mark(mBold, 0, 2, "")}},
		{"strike", "~~hi~~", "hi", []*model.BlockContentTextMark{mark(mStrike, 0, 2, "")}},
		{"code span", "`x`", "x", []*model.BlockContentTextMark{mark(mCode, 0, 1, "")}},
		{"code span padded", "`` `x ``", "`x", []*model.BlockContentTextMark{mark(mCode, 0, 2, "")}},
		{"link", "[a](u)", "a", []*model.BlockContentTextMark{mark(mLink, 0, 1, "u")}},
		{"object link", "[a](anytype://object?objectId=id1)", "a",
			[]*model.BlockContentTextMark{mark(mObject, 0, 1, "id1")}},
		{"mention", `<mention objectId="id1">Roman</mention>`, "Roman",
			[]*model.BlockContentTextMark{mark(mMention, 0, 5, "id1")}},
		{"mention single quotes attr", `<mention objectId='id1'>R</mention>`, "R",
			[]*model.BlockContentTextMark{mark(mMention, 0, 1, "id1")}},
		{"font attr order and spaces", `<font  background = "y"  color = "r" >x</font>`, "x",
			[]*model.BlockContentTextMark{mark(mColor, 0, 1, "r"), mark(mBg, 0, 1, "y")}},
		{"underline", "<u>x</u>", "x", []*model.BlockContentTextMark{mark(mUnder, 0, 1, "")}},
		{"zero-length tag dropped", `a<mention objectId="x"></mention>b`, "ab", nil},
		{"self-closing tag dropped", `a<u/>b`, "ab", nil},
		{"entities", "&lt;u&gt; &amp; &#65;", "<u> & A", nil},
		{"escapes", `\*x\* \[y] \~\~`, "*x* [y] ~~", nil},
		{"unmatched bold literal", "**unclosed", "**unclosed", nil},
		{"unmatched code literal", "`unclosed", "`unclosed", nil},
		{"link with space in dest literal", "[a](b c)", "[a](b c)", nil},
		{"angle dest", "[a](<b c>)", "a", []*model.BlockContentTextMark{mark(mLink, 0, 1, "b c")}},
		{"adjacent italic merges", "*a**b*", "ab", []*model.BlockContentTextMark{mark(mItalic, 0, 2, "")}},
		{"nested em in strong", "**a *b* c**", "a b c",
			[]*model.BlockContentTextMark{mark(mBold, 0, 5, ""), mark(mItalic, 2, 3, "")}},
		{"stars with spaces literal", "a * b * c", "a * b * c", nil},
		{"empty link param dropped", "[a]()", "a", nil},
		{"tilde run of three literal", "~~~x~~~", "~~~x~~~", nil},
		{"utf16 offsets astral", "𝒜 **b**", "𝒜 b", []*model.BlockContentTextMark{mark(mBold, 3, 4, "")}},
		{"soft break", "a\nb", "a\nb", nil},
		{"emphasis across soft break", "**a\nb**", "a\nb", []*model.BlockContentTextMark{mark(mBold, 0, 3, "")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text, marks, err := parseInline(tc.md)
			require.NoError(t, err)
			assert.Equal(t, tc.wantText, text)
			assert.Equal(t, tc.wantMarks, marks)

			// render(parse(J)) is the canonical form; parsing it again must
			// reproduce the same state (idempotence, §11.2)
			canonical := renderInline(text, marks)
			text2, marks2, err := parseInline(canonical)
			require.NoError(t, err)
			assert.Equal(t, text, text2)
			assert.Equal(t, marks, marks2)
			assert.Equal(t, canonical, renderInline(text2, marks2))
		})
	}
}

func TestParseInline_Errors(t *testing.T) {
	tests := []struct {
		name string
		md   string
	}{
		{"unclosed u tag", "<u>x"},
		{"unmatched closing tag", "x</u>"},
		{"mention without objectId", "<mention>x</mention>"},
		{"font without attrs", "<font>x</font>"},
		{"unknown font attr", `<font size="2">x</font>`},
		{"unquoted attr value", `<font color=red>x</font>`},
		{"misnested tags", `<u>a<font color="r">b</u></font>`},
		{"duplicate attr", `<font color="r" color="b">x</font>`},
		{"unterminated tag", "<u attr"},
		{"closing tag with attrs", `</font color="r">`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseInline(tc.md)
			require.Error(t, err)
		})
	}
}

// TestInline_UnmatchedDelimiterLiteralization: unmatched openers demote to
// literal text with correct mark offset shifting.
func TestInline_UnmatchedDelimiterLiteralization(t *testing.T) {
	// the '*' opener never closes; the code span mark must shift right
	text, marks, err := parseInline("*`c`")
	require.NoError(t, err)
	assert.Equal(t, "*c", text)
	assert.Equal(t, []*model.BlockContentTextMark{mark(mCode, 1, 2, "")}, marks)
}

// TestInline_PropertyRoundTrip generates random states and checks the §11
// guarantees: canonical output always parses, and Export ∘ Import is
// byte-stable from the first canonical form on.
func TestInline_PropertyRoundTrip(t *testing.T) {
	alphabet := []string{
		"a", "b", "c", " ", "\n", "*", "_", "~", "`", "[", "]", "(", ")", "<", ">",
		"&", "\\", "\"", "'", "😀", "𝒜", "é", "u", "font", "mention", "lt;", "#x",
	}
	types := []model.BlockContentTextMarkType{
		mBold, mItalic, mStrike, mCode, mLink, mObject, mMention, mUnder, mColor, mBg, mEmoji,
	}
	params := []string{"", "red", "https://x.io/a(b)", "id1", "a b", "😀", "x\"y"}
	rnd := rand.New(rand.NewSource(42))
	for i := 0; i < 20000; i++ {
		var sb strings.Builder
		for n := rnd.Intn(12); n > 0; n-- {
			sb.WriteString(alphabet[rnd.Intn(len(alphabet))])
		}
		txt := sb.String()
		u16len := int32(len(utf16.Encode([]rune(txt))))
		var marks []*model.BlockContentTextMark
		for n := rnd.Intn(6); n > 0; n-- {
			from := rnd.Int31n(u16len + 1)
			to := rnd.Int31n(u16len + 1)
			marks = append(marks, mark(types[rnd.Intn(len(types))], from, to, params[rnd.Intn(len(params))]))
		}
		md1 := renderInline(txt, marks)
		text1, marks1, err := parseInline(md1)
		require.NoErrorf(t, err, "case %d: canonical output must parse: text=%q marks=%v md=%q", i, txt, marks, md1)
		md2 := renderInline(text1, marks1)
		require.Equalf(t, md1, md2, "case %d: not byte-stable: text=%q marks=%v", i, txt, marks)
		text2, marks2, err := parseInline(md2)
		require.NoError(t, err)
		require.Equalf(t, text1, text2, "case %d", i)
		require.Equalf(t, marks1, marks2, "case %d: marks not stable: md=%q", i, md1)
	}
}
