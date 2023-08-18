package clipboard

import (
	"strconv"
	"testing"

	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"

	_ "github.com/anyproto/anytype-heart/core/block/simple/base"
)

func TestCommonSmart_pasteHtml(t *testing.T) {
	t.Run("Simple: single p block", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "abcde", "55555"}, emptyMarks))
		pasteHtml(t, sb, "4", model.Range{From: 2, To: 4}, []string{}, "<p>000</p>")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "ab000e", "55555"})
	})

	t.Run("Simple: 2 p blocks", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "abcde", "55555"}, emptyMarks))
		pasteHtml(t, sb, "4", model.Range{From: 2, To: 4}, []string{}, "<p>lkjhg</p><p>hello</p>")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "ab", "lkjhg", "hello", "e", "55555"})
	})

	t.Run("Simple: 1 p 1 h2", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<h2>lorem</h2><p>ipsum</p>")
		checkBlockText(t, sb, []string{"lorem", "ipsum"})
	})

	t.Run("Simple: 1 p with markup", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<p>i<b>p</b>s <i>um</i> ololo</p>")
		checkBlockText(t, sb, []string{"ips um ololo"})
	})

	t.Run("Markup in header", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<h1>foo <em>bar</em> baz</h1>\n")
		checkBlockText(t, sb, []string{"foo bar baz"})
	})

	t.Run("Different headers", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<h3>foo</h3>\n<h2>foo</h2>\n<h1>foo</h1>\n")
		checkBlockText(t, sb, []string{"foo", "foo", "foo"})
	})

	t.Run("Code block -> header", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<pre><code># foo\n</code></pre>\n")
		checkBlockText(t, sb, []string{"# foo\n\n"})
	})

	t.Run("Link markup, auto paragraph", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<div><a href=\"bar\">foo</a></div>\n")
		checkBlockText(t, sb, []string{"foo"})
	})

	t.Run("Table block", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<table><tr><td>\nfoo\n</td></tr></table>\n")
		checkBlockText(t, sb, []string{"foo"})
	})

	t.Run("Link in paragraph", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<p><a href=\"url\">foo</a></p>\n")
		checkBlockText(t, sb, []string{"foo"})
	})

	t.Run("Nested tags: p inside quote && header with markup", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<h1><a href=\"/url\">Foo</a></h1>\n<blockquote>\n<p>bar</p>\n</blockquote>\n")
		checkBlockText(t, sb, []string{"Foo", "bar"})
	})

	t.Run("Nested tags: h1 && p inside quote", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<blockquote>\n<h1>Foo</h1>\n<p>bar\nbaz</p>\n</blockquote>\n")
		checkBlockText(t, sb, []string{"Foo", "bar\nbaz"})
	})
}

func TestCommonSmart_pasteAny(t *testing.T) {
	t.Run("1. Cursor at the beginning, range == 0. Expected behavior: inserting blocks on top", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteAny(t, sb, "4", model.Range{From: 0, To: 0}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa", "bbbbb", "qwerty", "55555"})
	})

	t.Run("2. Cursor in a middle, range == 0. Expected behaviour: split block top + bottom, insert in a middle", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteAny(t, sb, "4", model.Range{From: 2, To: 2}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qw", "aaaaa", "bbbbb", "erty", "55555"})
	})

	t.Run("3. Cursor: end, range == 0. Expected behaviour: insert after block", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteAny(t, sb, "4", model.Range{From: 6, To: 6}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qwerty", "aaaaa", "bbbbb", "55555"})
	})

	t.Run("4. Cursor: from 1/4 to 3/4, range == 1/2. Expected behaviour: split block top + bottom, remove Range, insert in a middle", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteAny(t, sb, "4", model.Range{From: 2, To: 4}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qw", "aaaaa", "bbbbb", "ty", "55555"})
	})

	t.Run("5. Cursor: from start to middle, range == 1/2. Expected Behavior: top insert, range removal", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteAny(t, sb, "4", model.Range{From: 0, To: 3}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa", "bbbbb", "rty", "55555"})
	})

	t.Run("6. Cursor: middle to end, range == 1/2. Expected Behavior: bottom insert, range removal", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteAny(t, sb, "4", model.Range{From: 3, To: 6}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qwe", "aaaaa", "bbbbb", "55555"})
	})

	t.Run("7. Cursor from start to end, range == 1. Expected behavior: bottom / top insert, block deletion", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteAny(t, sb, "4", model.Range{From: 0, To: 6}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa", "bbbbb", "55555"})
	})

	t.Run("8. Replace selection", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "44444", "55555"}, emptyMarks))
		pasteAny(t, sb, "", model.Range{}, []string{"3", "4"}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "aaaaa", "bbbbb", "55555"})
	})

	t.Run("9. Save id of focused block", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "44444", "55555"}, emptyMarks))
		pasteAny(t, sb, "4", model.Range{}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa", "bbbbb", "44444", "55555"})
		assert.Equal(t, sb.Blocks()[5].Id, "4")
	})

}

func TestCommonSmart_splitMarks(t *testing.T) {
	t.Run("<b>lorem</b> lorem (**********)  :--->   <b>lorem</b> lorem __PASTE__  \n(m.Range.From < r.From) && (m.Range.To <= r.From)", func(t *testing.T) {
		initialText := []string{"abcdef"}
		initialMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From: 1, To: 3},
				Type:  model.BlockContentTextMark_Bold,
			}},
		}

		pasteText := []string{"123456"}
		pasteMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From: 0, To: 4},
				Type:  model.BlockContentTextMark_Bold,
			}},
		}

		sb := createPage(t, createBlocks([]string{}, initialText, initialMarks))
		pasteAny(t, sb, "1", model.Range{From: 5, To: 5}, []string{}, createBlocks([]string{"new1"}, pasteText, pasteMarks)) // @marks
		checkBlockMarksDebug(t, sb, [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From: 1, To: 3},
				Type:  model.BlockContentTextMark_Bold,
			}, {
				Range: &model.Range{From: 0 + 5, To: 4 + 5},
				Type:  model.BlockContentTextMark_Bold,
			}},
		})
	})

	t.Run("<b>lorem lorem(******</b>******)  :--->   <b>lorem lorem</b> __PASTE__  \n(m.Range.From < r.From) && (m.Range.To > r.From) && (m.Range.To < r.To)", func(t *testing.T) {
		initialText := []string{"abcdef"}
		initialMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From: 1, To: 3},
				Type:  model.BlockContentTextMark_Bold,
			}},
		}

		pasteText := []string{"123456"}
		pasteMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From: 0, To: 4},
				Type:  model.BlockContentTextMark_Bold,
			}},
		}

		sb := createPage(t, createBlocks([]string{}, initialText, initialMarks))
		pasteAny(t, sb, "1", model.Range{From: 2, To: 5}, []string{}, createBlocks([]string{"new1"}, pasteText, pasteMarks)) // @marks
		checkBlockMarks(t, sb, [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From: 1, To: 6},
				Type:  model.BlockContentTextMark_Bold,
			}},
		})
	})

	t.Run("(**<b>******</b>******)  :--->     __PASTE__  (m.Range.From >= r.From) && (m.Range.To <= r.To)", func(t *testing.T) {
		initialText := []string{"abcdef"}
		initialMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From: 1, To: 3},
				Type:  model.BlockContentTextMark_Bold,
			}},
		}

		pasteText := []string{"123456"}
		pasteMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From: 0, To: 4},
				Type:  model.BlockContentTextMark_Bold,
			}},
		}

		sb := createPage(t, createBlocks([]string{}, initialText, initialMarks))

		pasteAny(t, sb, "1", model.Range{From: 1, To: 3}, []string{}, createBlocks([]string{"new1"}, pasteText, pasteMarks)) // @marks
		checkBlockMarks(t, sb, [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From: 1, To: 5},
				Type:  model.BlockContentTextMark_Bold,
			}},
		})
	})

	t.Run("<b>lorem (*********) lorem</b>  :--->   <b>lorem</b> __PASTE__ <b>lorem</b>  (m.Range.From < r.From) && (m.Range.To > r.To)", func(t *testing.T) {
		initialText := []string{"abcdef"}
		initialMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From: 1, To: 4},
				Type:  model.BlockContentTextMark_Bold,
			}},
		}

		pasteText := []string{"123456"}
		pasteMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From: 1, To: 4},
				Type:  model.BlockContentTextMark_Italic,
			}},
		}

		sb := createPage(t, createBlocks([]string{}, initialText, initialMarks))

		pasteAny(t, sb, "1", model.Range{From: 2, To: 3}, []string{}, createBlocks([]string{"new1"}, pasteText, pasteMarks)) // @marks
		checkBlockMarks(t, sb, [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From: 3, To: 6},
				Type:  model.BlockContentTextMark_Italic,
			}, {
				Range: &model.Range{From: 1, To: 2},
				Type:  model.BlockContentTextMark_Bold,
			},
				{
					Range: &model.Range{From: 8, To: 9},
					Type:  model.BlockContentTextMark_Bold,
				}},
		})
	})

	t.Run("(*********) <b>lorem lorem</b>  :--->   __PASTE__ <b>lorem lorem</b>  (m.Range.From > r.To)", func(t *testing.T) {
		sb := page(
			block("1", "abcdef", mark(bold, 3, 5)),
		)
		rangePaste(sb, t, "1", rng(1, 2), rng(0, 6),
			block("n1", "123456", mark(bold, 1, 4)),
		)
		shouldBe(sb, t,
			block("1", "a123456cdef", mark(bold, 2, 5), mark(bold, 8, 10)),
		)

	})
}

func TestCommonSmart_pasteAny_marks(t *testing.T) {
	t.Run("should paste single mark paste to the end, no focus", func(t *testing.T) {
		textArr := []string{"11111"}
		marksArr := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From: 1, To: 2},
				Type:  model.BlockContentTextMark_Bold,
			}},
		}

		sb := createPage(t, createBlocks([]string{}, textArr, emptyMarks))
		pasteAny(t, sb, "", model.Range{From: 0, To: 0}, []string{}, createBlocks([]string{"new1"}, []string{"99999"}, marksArr)) // @marks
		checkBlockMarks(t, sb, [][]*model.BlockContentTextMark{
			{{}},
			{{
				Range: &model.Range{From: 1, To: 2},
				Type:  model.BlockContentTextMark_Bold,
			}},
		})
	})

	t.Run("should paste multiple marks paste to the end, no focus", func(t *testing.T) {
		pasteMarksArr := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From: 1, To: 2},
				Type:  model.BlockContentTextMark_Bold,
			}, {
				Range: &model.Range{From: 4, To: 5},
				Type:  model.BlockContentTextMark_Strikethrough,
			}},
			{{
				Range: &model.Range{From: 0, To: 4},
				Type:  model.BlockContentTextMark_Italic,
			}},
		}

		sb := createPage(t, createBlocks([]string{}, []string{"11111"}, emptyMarks))
		pasteAny(t, sb, "", model.Range{From: 0, To: 0}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"99999", "00000"}, pasteMarksArr))
		checkBlockMarks(t, sb, [][]*model.BlockContentTextMark{
			{{}},
			{{
				Range: &model.Range{From: 1, To: 2},
				Type:  model.BlockContentTextMark_Bold,
			}, {
				Range: &model.Range{From: 4, To: 5},
				Type:  model.BlockContentTextMark_Strikethrough,
			}},
			{{
				Range: &model.Range{From: 0, To: 4},
				Type:  model.BlockContentTextMark_Italic,
			}},
		})
	})
}

func TestCommonSmart_RangeSplit(t *testing.T) {
	t.Run("1. Cursor at the beginning, range == 0. Expected behavior: inserting blocks on top", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteAny(t, sb, "4", model.Range{From: 0, To: 0}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa", "bbbbb", "qwerty", "55555"})
	})

	t.Run("2. Cursor in a middle, range == 0. Expected behaviour: split block top + bottom, insert in a middle", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteAny(t, sb, "4", model.Range{From: 2, To: 2}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qw", "aaaaa", "bbbbb", "erty", "55555"})
	})

	t.Run("3. Cursor: end, range == 0. Expected behaviour: insert after block", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteAny(t, sb, "4", model.Range{From: 6, To: 6}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qwerty", "aaaaa", "bbbbb", "55555"})
	})

	t.Run("4. Cursor: from 1/4 to 3/4, range == 1/2. Expected behaviour: split block top + bottom, remove Range, insert in a middle", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteAny(t, sb, "4", model.Range{From: 2, To: 4}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qw", "aaaaa", "bbbbb", "ty", "55555"})
	})

	t.Run("5. Cursor: from start to middle, range == 1/2. Expected Behavior: top insert, range removal", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteAny(t, sb, "4", model.Range{From: 0, To: 3}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa", "bbbbb", "rty", "55555"})
	})

	t.Run("6. Cursor: middle to end, range == 1/2. Expected Behavior: bottom insert, range removal", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteAny(t, sb, "4", model.Range{From: 3, To: 6}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qwe", "aaaaa", "bbbbb", "55555"})
	})

	t.Run("7. Cursor from start to end, range == 1. Expected behavior: bottom / top insert, block deletion", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteAny(t, sb, "4", model.Range{From: 0, To: 6}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa", "bbbbb", "55555"})
	})
}

func TestCommonSmart_TextSlot_RangeSplitCases(t *testing.T) {
	t.Run("1. Cursor at the beginning, range == 0. Expected behavior: inserting blocks on top", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteText(t, sb, "4", model.Range{From: 0, To: 0}, []string{}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa\nbbbbb", "qwerty", "55555"})
	})

	t.Run("2. Cursor in a middle, range == 0. Expected behaviour: split block top + bottom, insert in a middle", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteText(t, sb, "4", model.Range{From: 2, To: 2}, []string{}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qwaaaaa\nbbbbberty", "55555"})
	})

	t.Run("3. Cursor: end, range == 0. Expected behaviour: insert after block", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteText(t, sb, "4", model.Range{From: 6, To: 6}, []string{}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qwertyaaaaa\nbbbbb", "55555"})
	})

	t.Run("4. Cursor from 1/4 to 3/4, range == 1/2. Expected behaviour: split block: top + bottom, remove Range, insert in a middle", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteText(t, sb, "4", model.Range{From: 2, To: 4}, []string{}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qwaaaaa\nbbbbbty", "55555"})
	})

	t.Run("5. Cursor from stast to middle, range == 1/2. Expected behaviour: insert top, remove Range", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteText(t, sb, "4", model.Range{From: 0, To: 3}, []string{}, "eeeee\naaaaa\nbbbbb\nccccc")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "eeeee\naaaaa\nbbbbb\ncccccrty", "55555"})
	})

	t.Run("6. Cursor: middle to end, range == 1/2. Expected Behavior: bottom insert, range removal", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteText(t, sb, "4", model.Range{From: 3, To: 6}, []string{}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qweaaaaa\nbbbbb", "55555"})
	})

	t.Run("7. Cursor from start to end, range == 1. Expected behavior: bottom / top insert, block deletion", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteText(t, sb, "4", model.Range{From: 0, To: 6}, []string{}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa\nbbbbb", "55555"})
	})

	t.Run("8.0 Cursor in the middle. Paste two blocks", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"123456789"}, emptyMarks))
		pasteAny(t, sb, "1", model.Range{From: 0, To: 0}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"abc", "def"}, emptyMarks))
		checkBlockText(t, sb, []string{"abc", "def", "123456789"})
	})

	t.Run("8.1 Cursor in the middle. Paste two blocks", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"123456789"}, emptyMarks))
		pasteAny(t, sb, "1", model.Range{From: 1, To: 1}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"abc", "def"}, emptyMarks))
		checkBlockText(t, sb, []string{"1", "abc", "def", "23456789"})
	})

	t.Run("8.2 Cursor in the middle. Paste two blocks", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"123456789"}, emptyMarks))
		pasteAny(t, sb, "1", model.Range{From: 2, To: 2}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"abc", "def"}, emptyMarks))
		checkBlockText(t, sb, []string{"12", "abc", "def", "3456789"})
	})

	t.Run("9. Cursor at the pre-end. Paste two blocks", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"123456789"}, emptyMarks))
		pasteAny(t, sb, "1", model.Range{From: 4, To: 4}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"abc", "def"}, emptyMarks))
		checkBlockText(t, sb, []string{"1234", "abc", "def", "56789"})
	})

	t.Run("10. Cursor at the end. Paste two blocks", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"123456789"}, emptyMarks))
		pasteAny(t, sb, "1", model.Range{From: 9, To: 9}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"abc", "def"}, emptyMarks))
		checkBlockText(t, sb, []string{"123456789", "abc", "def"})
	})
}

func TestCommonSmart_TextSlot_CommonCases(t *testing.T) {
	t.Run("should split block on paste", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "abcde", "55555"}, emptyMarks))
		pasteText(t, sb, "4", model.Range{From: 2, To: 4}, []string{}, "22222\n33333")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "ab22222\n33333e", "55555"})
	})

	t.Run("should paste to the end when no focus", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "44444", "55555"}, emptyMarks))
		pasteText(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "44444", "55555", "aaaaa\nbbbbb"})
	})

	t.Run("should paste to the end when no focus", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "44444", "55555"}, emptyMarks))
		pasteText(t, sb, "", model.Range{From: 0, To: 0}, []string{"2", "3", "4"}, "22222\n33333")
		checkBlockText(t, sb, []string{"11111", "22222\n33333", "55555"})
	})

	t.Run("should paste to the empty page", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
		pasteText(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "22222\n33333")
		checkBlockText(t, sb, []string{"22222\n33333"})
	})

	t.Run("should paste when all blocks selected", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "44444", "55555"}, emptyMarks))
		pasteText(t, sb, "", model.Range{From: 0, To: 0}, []string{"1", "2", "3", "4", "5"}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"aaaaa\nbbbbb"})
	})

	t.Run("paste single to empty block", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "", "33333"}, emptyMarks))
		pasteText(t, sb, "2", model.Range{From: 0, To: 0}, []string{}, "text")
		checkBlockText(t, sb, []string{"11111", "text", "33333"})
	})

	t.Run("paste multi to empty block", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "", "33333"}, emptyMarks))
		pasteText(t, sb, "2", model.Range{From: 0, To: 0}, []string{}, "text\ntext2")
		checkBlockText(t, sb, []string{"11111", "text\ntext2", "33333"})
	})
}

func TestClipboard_TitleOps(t *testing.T) {
	newTextBlock := func(text string) simple.Block {
		return simple.New(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: text,
				},
			},
		})
	}

	newBookmark := func(url string) simple.Block {
		return simple.New(&model.Block{
			Content: &model.BlockContentOfBookmark{
				Bookmark: &model.BlockContentBookmark{
					Url: url,
				},
			},
		})
	}

	withTitle := func(t *testing.T, title string, textBlocks ...string) *smarttest.SmartTest {
		sb := smarttest.New("text")
		s := sb.NewState()
		template.InitTemplate(s, template.WithTitle)
		s.Get(template.TitleBlockId).(text.Block).SetText(title, nil)
		for i, tt := range textBlocks {
			tb := newTextBlock(tt)
			tb.Model().Id = "id" + strconv.Itoa(i)
			s.Add(tb)
			s.InsertTo("", 0, tb.Model().Id)
		}
		_, _, err := state.ApplyState(s, false)
		require.NoError(t, err)
		return sb
	}

	withBookmark := func(t *testing.T, firstTextBlock, lastTextBlock, bookmarkUrl string) *smarttest.SmartTest {
		sb := smarttest.New("text")
		s := sb.NewState()
		template.InitTemplate(s, template.WithTitle)
		if firstTextBlock != "" {
			tb := newTextBlock(firstTextBlock)
			tb.Model().Id = "firstTextBlockId"
			s.Add(tb)
			s.InsertTo("", 0, tb.Model().Id)
		}
		if lastTextBlock != "" {
			tb := newTextBlock(lastTextBlock)
			tb.Model().Id = "lastTextBlockId"
			s.Add(tb)
			s.InsertTo("", 0, tb.Model().Id)
		}
		bm := newBookmark(bookmarkUrl)
		bm.Model().Id = "bookmarkId"
		s.Add(bm)
		s.InsertTo("", 0, bm.Model().Id)
		_, _, err := state.ApplyState(s, false)
		require.NoError(t, err)
		return sb
	}

	singleBlockReq := &pb.RpcBlockPasteRequest{
		FocusedBlockId:    template.TitleBlockId,
		SelectedTextRange: &model.Range{},
		AnySlot: []*model.Block{
			newTextBlock("single").Model(),
		},
	}

	descriptionBlockReq := func() *pb.RpcBlockPasteRequest {
		textBlock := newTextBlock("paste description")
		textBlock.Model().Id = template.DescriptionBlockId
		textBlock.Model().Fields = &types.Struct{
			Fields: map[string]*types.Value{
				text.DetailsKeyFieldName: pbtypes.String("default description"),
			},
		}
		return &pb.RpcBlockPasteRequest{
			FocusedBlockId:    template.TitleBlockId,
			SelectedTextRange: &model.Range{},
			AnySlot: []*model.Block{
				newTextBlock("whatever").Model(),
				textBlock.Model(),
			},
		}
	}

	multiBlockReq := &pb.RpcBlockPasteRequest{
		FocusedBlockId:    template.TitleBlockId,
		SelectedTextRange: &model.Range{},
		AnySlot: []*model.Block{
			newTextBlock("first").Model(),
			newTextBlock("second").Model(),
			newTextBlock("third").Model(),
		},
	}

	t.Run("single to empty title", func(t *testing.T) {
		st := withTitle(t, "")
		cb := NewClipboard(st, nil, nil, nil, nil)
		_, _, _, _, err := cb.Paste(nil, singleBlockReq, "")
		require.NoError(t, err)
		assert.Equal(t, "single", st.Doc.Pick(template.TitleBlockId).Model().GetText().Text)
	})
	t.Run("single description to empty description", func(t *testing.T) {
		//given
		state := withTitle(t, "")
		addDescription(state, "current description")
		cb := NewClipboard(state, nil, nil, nil, nil)

		//when
		_, _, _, _, err := cb.Paste(nil, descriptionBlockReq(), "")

		//then
		require.NoError(t, err)
		assert.Equal(t, "current description", state.Doc.Pick(template.DescriptionBlockId).Model().GetText().Text)
		find, _ := lo.Find(
			state.Doc.Blocks(),
			func(block *model.Block) bool {
				return block.GetText() != nil && block.GetText().Text == "paste description"
			},
		)
		assert.True(t, true, find)
	})
	t.Run("single to not empty title", func(t *testing.T) {
		st := withTitle(t, "title")
		cb := NewClipboard(st, nil, nil, nil, nil)
		req := singleBlockReq
		req.SelectedTextRange = &model.Range{From: 1, To: 4}
		_, _, _, _, err := cb.Paste(nil, req, "")
		require.NoError(t, err)
		assert.Equal(t, "tsinglee", st.Doc.Pick(template.TitleBlockId).Model().GetText().Text)
		assert.Equal(t, model.BlockContentText_Title, st.Doc.Pick(template.TitleBlockId).Model().GetText().Style)
	})
	t.Run("single to not empty title - select all", func(t *testing.T) {
		st := withTitle(t, "title")
		cb := NewClipboard(st, nil, nil, nil, nil)
		req := singleBlockReq
		req.SelectedTextRange = &model.Range{From: 0, To: 5}
		_, _, _, _, err := cb.Paste(nil, req, "")
		require.NoError(t, err)
		assert.Equal(t, "single", st.Doc.Pick(template.TitleBlockId).Model().GetText().Text)
		assert.Equal(t, model.BlockContentText_Title, st.Doc.Pick(template.TitleBlockId).Model().GetText().Style)
	})
	t.Run("multi to empty title", func(t *testing.T) {
		st := withTitle(t, "")
		cb := NewClipboard(st, nil, nil, nil, nil)
		_, _, _, _, err := cb.Paste(nil, multiBlockReq, "")
		require.NoError(t, err)
		rootChild := st.Doc.Pick(st.RootId()).Model().ChildrenIds
		assert.Equal(t, "first", st.Doc.Pick(template.TitleBlockId).Model().GetText().Text)
		assert.Equal(t, "second", st.Doc.Pick(rootChild[1]).Model().GetText().Text)
		assert.Equal(t, "third", st.Doc.Pick(rootChild[2]).Model().GetText().Text)
	})
	t.Run("multi to not empty title", func(t *testing.T) {
		st := withTitle(t, "title")
		cb := NewClipboard(st, nil, nil, nil, nil)
		_, _, _, _, err := cb.Paste(nil, multiBlockReq, "")
		require.NoError(t, err)
		rootChild := st.Doc.Pick(st.RootId()).Model().ChildrenIds
		assert.Equal(t, "first", st.Doc.Pick(template.TitleBlockId).Model().GetText().Text)
		assert.Equal(t, "second", st.Doc.Pick(rootChild[1]).Model().GetText().Text)
		assert.Equal(t, "third", st.Doc.Pick(rootChild[2]).Model().GetText().Text)
		assert.Equal(t, "title", st.Doc.Pick(rootChild[3]).Model().GetText().Text)
	})
	t.Run("multi to not empty title with range", func(t *testing.T) {
		st := withTitle(t, "title")
		cb := NewClipboard(st, nil, nil, nil, nil)
		req := multiBlockReq
		req.SelectedTextRange = &model.Range{From: 1, To: 4}
		_, _, _, _, err := cb.Paste(nil, req, "")
		require.NoError(t, err)
		rootChild := st.Doc.Pick(st.RootId()).Model().ChildrenIds
		assert.Equal(t, "t", st.Doc.Pick(template.TitleBlockId).Model().GetText().Text)
		assert.Equal(t, "first", st.Doc.Pick(rootChild[1]).Model().GetText().Text)
		assert.Equal(t, "second", st.Doc.Pick(rootChild[2]).Model().GetText().Text)
		assert.Equal(t, "third", st.Doc.Pick(rootChild[3]).Model().GetText().Text)
		assert.Equal(t, "e", st.Doc.Pick(rootChild[4]).Model().GetText().Text)
	})
	t.Run("multi to end of title", func(t *testing.T) {
		st := withTitle(t, "title")
		cb := NewClipboard(st, nil, nil, nil, nil)
		req := multiBlockReq
		req.SelectedTextRange = &model.Range{From: 5, To: 5}
		_, _, _, _, err := cb.Paste(nil, req, "")
		require.NoError(t, err)
		rootChild := st.Doc.Pick(st.RootId()).Model().ChildrenIds
		assert.Equal(t, "title", st.Doc.Pick(template.TitleBlockId).Model().GetText().Text)
		assert.Equal(t, "first", st.Doc.Pick(rootChild[1]).Model().GetText().Text)
		assert.Equal(t, "second", st.Doc.Pick(rootChild[2]).Model().GetText().Text)
		assert.Equal(t, "third", st.Doc.Pick(rootChild[3]).Model().GetText().Text)
	})

	t.Run("cut title and another block", func(t *testing.T) {
		//given
		st := withTitle(t, "real title", "second")
		cb := NewClipboard(st, nil, nil, nil, nil)

		secondTextBlock := newTextBlock("second").Model()
		secondTextBlock.Id = "id0"

		req := pb.RpcBlockCutRequest{
			Blocks: []*model.Block{
				st.Doc.NewState().Get("title").Model(),
				secondTextBlock,
			},
			SelectedTextRange: &model.Range{},
		}

		//when
		_, _, anySlot, err := cb.Cut(nil, req)

		//then
		require.NoError(t, err)
		assert.Equal(t, "", st.Doc.Pick(template.TitleBlockId).Model().GetText().Text)
		assert.Equal(t, "real title", anySlot[0].GetText().Text)
	})

	t.Run("cut text and object block", func(t *testing.T) {
		var (
			url              = "http://example.com"
			text             = "simple text"
			firstTextBlockId = "firstTextBlockId"
			bookmarkId       = "bookmarkId"
			result           = text + "\n"
		)
		st := withBookmark(t, text, "", url)
		cb := NewClipboard(st, nil, nil, nil, nil)
		textBlock := newTextBlock(text).Model()
		textBlock.Id = firstTextBlockId
		bookmark := newBookmark(url).Model()
		bookmark.Id = bookmarkId
		blockCutReq := pb.RpcBlockCutRequest{
			ContextId:         "context",
			SelectedTextRange: &model.Range{From: 0, To: 11},
			Blocks:            []*model.Block{textBlock, bookmark},
		}
		textSlot, htmlSlot, anySlot, err := cb.Cut(nil, blockCutReq)
		require.NoError(t, err)
		assert.Equal(t, result, textSlot)
		assert.Len(t, anySlot, 2)
		assert.Equal(t, firstTextBlockId, anySlot[0].Id)
		assert.Equal(t, bookmarkId, anySlot[1].Id)
		assert.Contains(t, htmlSlot, text)
		assert.Contains(t, htmlSlot, url)
	})
	t.Run("cut simple text, link object and simple text", func(t *testing.T) {
		var (
			url              = "http://example.com"
			firstText        = "first text"
			firstTextBlockId = "firstTextBlockId"
			lastTextBlockId  = "lastTextBlockId"
			bookmarkId       = "bookmarkId"
			secondText       = "second text"
			result           = firstText + "\n" + secondText + "\n"
		)
		st := withBookmark(t, firstText, secondText, url)
		cb := NewClipboard(st, nil, nil, nil, nil)
		textBlock := newTextBlock(firstText).Model()
		textBlock.Id = firstTextBlockId
		bookmark := newBookmark(url).Model()
		bookmark.Id = bookmarkId
		lastTextBlock := newTextBlock(secondText).Model()
		lastTextBlock.Id = lastTextBlockId
		blockCutReq := pb.RpcBlockCutRequest{
			ContextId:         "context",
			SelectedTextRange: &model.Range{From: 0, To: 11},
			Blocks:            []*model.Block{textBlock, bookmark, lastTextBlock},
		}
		textSlot, htmlSlot, anySlot, err := cb.Cut(nil, blockCutReq)
		require.NoError(t, err)
		assert.Equal(t, result, textSlot)
		assert.Len(t, anySlot, 3)
		assert.Equal(t, firstTextBlockId, anySlot[0].Id)
		assert.Equal(t, bookmarkId, anySlot[1].Id)
		assert.Equal(t, lastTextBlockId, anySlot[2].Id)
		assert.Contains(t, htmlSlot, firstText)
		assert.Contains(t, htmlSlot, url)
		assert.Contains(t, htmlSlot, secondText)
	})
	t.Run("cut from title", func(t *testing.T) {
		st := withTitle(t, "title")
		cb := NewClipboard(st, nil, nil, nil, nil)
		req := pb.RpcBlockCutRequest{
			Blocks: []*model.Block{
				st.Doc.NewState().Get("title").Model(),
			},
			SelectedTextRange: &model.Range{From: 1, To: 3},
		}
		textSlot, htmlSlot, anySlot, err := cb.Cut(nil, req)
		require.NoError(t, err)
		assert.Equal(t, "tle", st.Doc.Pick(template.TitleBlockId).Model().GetText().Text)
		assert.Equal(t, "it", textSlot)
		assert.NotContains(t, htmlSlot, ">title<")
		assert.Contains(t, htmlSlot, ">it<")
		require.Len(t, anySlot, 1)
		assert.Equal(t, "it", anySlot[0].GetText().Text)
	})
}

func addDescription(st *smarttest.SmartTest, description string) {
	newState := st.Doc.NewState()
	template.InitTemplate(newState, template.WithForcedDescription)
	newState.Get(template.DescriptionBlockId).(text.Block).SetText(description, nil)
	state.ApplyState(newState, false)
}

func TestClipboard_PasteToCodeBock(t *testing.T) {
	sb := smarttest.New("text")
	require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithTitle))
	s := sb.NewState()
	codeBlock := simple.New(&model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Style: model.BlockContentText_Code,
				Text:  "some code",
			},
		},
	})
	s.Add(codeBlock)
	s.InsertTo("", model.Block_Inner, codeBlock.Model().Id)
	require.NoError(t, sb.Apply(s))

	cb := NewClipboard(sb, nil, nil, nil, nil)
	_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
		FocusedBlockId:    codeBlock.Model().Id,
		SelectedTextRange: &model.Range{4, 5},
		TextSlot:          "\nsome text\nhere\n",
	}, "")
	require.NoError(t, err)
	assert.Equal(t, "some\nsome text\nhere\ncode", sb.Doc.Pick(codeBlock.Model().Id).Model().GetText().Text)
	assert.Equal(t, model.BlockContentText_Code, sb.Doc.Pick(codeBlock.Model().Id).Model().GetText().Style)
}

func Test_PasteText(t *testing.T) {

	t.Run("paste", func(t *testing.T) {
		//given
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithEmpty))
		s := sb.NewState()
		b1 := simple.New(&model.Block{
			Id: "1",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "some text 1",
				},
			},
		})
		s.Add(b1)
		s.InsertTo("", model.Block_Inner, b1.Model().Id)
		b2 := simple.New(&model.Block{
			Id: "2",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "some text 2",
				},
			},
		})
		s.Add(b2)
		s.InsertTo("", model.Block_Inner, b2.Model().Id)
		require.NoError(t, sb.Apply(s))

		//when
		cb := NewClipboard(sb, nil, nil, nil, nil)
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			SelectedBlockIds: []string{"1", "2"},
			TextSlot:         "One string",
		}, "")

		//then
		require.NoError(t, err)
		assert.Equal(t, "One string", sb.NewState().Snippet())
	})

	t.Run("paste - when asterisks", func(t *testing.T) {
		//given
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithEmpty))
		s := sb.NewState()
		b1 := simple.New(&model.Block{
			Id: "1",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "some text 1",
				},
			},
		})
		s.Add(b1)
		s.InsertTo("", model.Block_Inner, b1.Model().Id)
		require.NoError(t, sb.Apply(s))

		//when
		cb := NewClipboard(sb, nil, nil, nil, nil)
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			SelectedBlockIds: []string{"1"},
			TextSlot:         "a * b * c",
			HtmlSlot:         "<meta charset='utf-8'><p data-pm-slice=\"1 1 []\">a *<em> b</em> * c</p>",
		}, "")

		//then
		require.NoError(t, err)
		assert.Equal(t, "a * b * c", sb.NewState().Snippet())
	})
}
