package clipboard

import (
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/restriction"
	"github.com/anyproto/anytype-heart/core/block/simple"
	_ "github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/fileobject/mock_fileobject"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/blockbuilder"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	textutil "github.com/anyproto/anytype-heart/util/text"
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

	t.Run("9. Return ids of new blocks", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "44444", "55555"}, emptyMarks))
		ids, isSameFocusedBlock := pasteAny(t, sb, "4", model.Range{}, []string{}, createBlocks([]string{"new1", "new2"}, []string{"aaaaa", "bbbbb"}, emptyMarks))
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa", "bbbbb", "44444", "55555"})
		assert.Len(t, ids, 2)
		assert.False(t, isSameFocusedBlock)
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
		_, _, err := state.ApplyState("", s, false)
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
		_, _, err := state.ApplyState("", s, false)
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

	requiredBlockReq := func(blockId string) *pb.RpcBlockPasteRequest {
		return &pb.RpcBlockPasteRequest{
			SelectedBlockIds:  []string{blockId},
			SelectedTextRange: &model.Range{},
			AnySlot: []*model.Block{
				newTextBlock("whatever").Model(),
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

	t.Run("paste - when base64 file", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithTitle))
		sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
		))

		// when
		cb := newFixture(t, sb)
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			HtmlSlot: `<img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABgAAAAYCAYAAADgdz34AAAABHNCSVQICAgIfAhkiAAAAAlwSFlzAAAApgAAAKYB3X3/OAAAABl0RVh0U29mdHdhcmUAd3d3Lmlua3NjYXBlLm9yZ5vuPBoAAANCSURBVEiJtZZPbBtFFMZ/M7ubXdtdb1xSFyeilBapySVU8h8OoFaooFSqiihIVIpQBKci6KEg9Q6H9kovIHoCIVQJJCKE1ENFjnAgcaSGC6rEnxBwA04Tx43t2FnvDAfjkNibxgHxnWb2e/u992bee7tCa00YFsffekFY+nUzFtjW0LrvjRXrCDIAaPLlW0nHL0SsZtVoaF98mLrx3pdhOqLtYPHChahZcYYO7KvPFxvRl5XPp1sN3adWiD1ZAqD6XYK1b/dvE5IWryTt2udLFedwc1+9kLp+vbbpoDh+6TklxBeAi9TL0taeWpdmZzQDry0AcO+jQ12RyohqqoYoo8RDwJrU+qXkjWtfi8Xxt58BdQuwQs9qC/afLwCw8tnQbqYAPsgxE1S6F3EAIXux2oQFKm0ihMsOF71dHYx+f3NND68ghCu1YIoePPQN1pGRABkJ6Bus96CutRZMydTl+TvuiRW1m3n0eDl0vRPcEysqdXn+jsQPsrHMquGeXEaY4Yk4wxWcY5V/9scqOMOVUFthatyTy8QyqwZ+kDURKoMWxNKr2EeqVKcTNOajqKoBgOE28U4tdQl5p5bwCw7BWquaZSzAPlwjlithJtp3pTImSqQRrb2Z8PHGigD4RZuNX6JYj6wj7O4TFLbCO/Mn/m8R+h6rYSUb3ekokRY6f/YukArN979jcW+V/S8g0eT/N3VN3kTqWbQ428m9/8k0P/1aIhF36PccEl6EhOcAUCrXKZXXWS3XKd2vc/TRBG9O5ELC17MmWubD2nKhUKZa26Ba2+D3P+4/MNCFwg59oWVeYhkzgN/JDR8deKBoD7Y+ljEjGZ0sosXVTvbc6RHirr2reNy1OXd6pJsQ+gqjk8VWFYmHrwBzW/n+uMPFiRwHB2I7ih8ciHFxIkd/3Omk5tCDV1t+2nNu5sxxpDFNx+huNhVT3/zMDz8usXC3ddaHBj1GHj/As08fwTS7Kt1HBTmyN29vdwAw+/wbwLVOJ3uAD1wi/dUH7Qei66PfyuRj4Ik9is+hglfbkbfR3cnZm7chlUWLdwmprtCohX4HUtlOcQjLYCu+fzGJH2QRKvP3UNz8bWk1qMxjGTOMThZ3kvgLI5AzFfo379UAAAAASUVORK5CYII=">`,
		}, "")

		// then
		assert.Equal(t, "image", sb.Doc.Blocks()[len(sb.Doc.Blocks())-1].GetFile().Name)
		require.NoError(t, err)
	})

	t.Run("single to empty title", func(t *testing.T) {
		st := withTitle(t, "")
		cb := newFixture(t, st)
		_, _, _, _, err := cb.Paste(nil, singleBlockReq, "")
		require.NoError(t, err)
		assert.Equal(t, "single", st.Doc.Pick(template.TitleBlockId).Model().GetText().Text)
	})

	for _, text := range []string{"", "full"} {
		t.Run("paste - when text is ("+text+")", func(t *testing.T) {
			// given
			sb := smarttest.New("text")
			require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithTitle))
			sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.Text(
						text,
						blockbuilder.ID("1"),
						blockbuilder.TextStyle(model.BlockContentText_Paragraph),
					),
					blockbuilder.Text(
						"toggle",
						blockbuilder.ID("2"),
						blockbuilder.TextStyle(model.BlockContentText_Toggle),
					),
				)))

			// when
			cb := newFixture(t, sb)
			_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
				FocusedBlockId:    "1",
				SelectedTextRange: &model.Range{From: 0, To: int32(textutil.UTF16RuneCountString(sb.Pick("1").Model().GetText().Text))},
				AnySlot:           []*model.Block{sb.Pick("2").Model()},
				IsPartOfBlock:     true,
			}, "")

			// then
			require.NoError(t, err)
			assert.Equal(t, model.BlockContentText_Toggle, sb.Doc.Pick("1").Model().GetText().Style)
		})
	}
	t.Run("paste - when text is empty, and style is not Paragraph", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithTitle))
		sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"",
					blockbuilder.ID("1"),
					blockbuilder.TextStyle(model.BlockContentText_Numbered),
				),
				blockbuilder.Text(
					"toggle",
					blockbuilder.ID("2"),
					blockbuilder.TextStyle(model.BlockContentText_Toggle),
				),
			)))

		// when
		cb := newFixture(t, sb)
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			FocusedBlockId:    "1",
			SelectedTextRange: &model.Range{From: 0, To: int32(textutil.UTF16RuneCountString(sb.Pick("1").Model().GetText().Text))},
			AnySlot:           []*model.Block{sb.Pick("2").Model()},
			IsPartOfBlock:     true,
		}, "")

		// then
		require.NoError(t, err)
		assert.Equal(t, model.BlockContentText_Numbered, sb.Doc.Pick("1").Model().GetText().Style)
	})
	for _, text := range []string{template.TitleBlockId, template.DescriptionBlockId} {
		t.Run("paste - when to block with id ("+text+")", func(t *testing.T) {
			// given
			sb := smarttest.New("text")
			require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithTitle))
			sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.Text(
						"whatever",
						blockbuilder.ID(text),
						blockbuilder.TextStyle(model.BlockContentText_Paragraph),
					),
					blockbuilder.Text(
						"toggle",
						blockbuilder.ID("2"),
						blockbuilder.TextStyle(model.BlockContentText_Toggle),
					),
				)))

			// when
			cb := newFixture(t, sb)
			_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
				FocusedBlockId:    "1",
				SelectedTextRange: &model.Range{From: 0, To: int32(textutil.UTF16RuneCountString(sb.Pick(text).Model().GetText().Text))},
				AnySlot:           []*model.Block{sb.Pick("2").Model()},
				IsPartOfBlock:     true,
			}, "")

			// then
			require.NoError(t, err)
			assert.Equal(t, model.BlockContentText_Paragraph, sb.Doc.Pick(text).Model().GetText().Style)
		})
	}
	for _, style := range []model.BlockContentTextStyle{
		model.BlockContentText_Description,
		model.BlockContentText_Title,
	} {
		t.Run("paste - when from block with style ("+style.String()+")", func(t *testing.T) {
			// given
			sb := smarttest.New("text")
			require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithTitle))
			sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.Text(
						"whatever",
						blockbuilder.ID("1"),
						blockbuilder.TextStyle(model.BlockContentText_Paragraph),
					),
					blockbuilder.Text(
						"toggle",
						blockbuilder.ID("2"),
						blockbuilder.TextStyle(style),
					),
				)))

			// when
			cb := newFixture(t, sb)
			_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
				FocusedBlockId:    "1",
				SelectedTextRange: &model.Range{From: 0, To: int32(textutil.UTF16RuneCountString(sb.Pick("1").Model().GetText().Text))},
				AnySlot:           []*model.Block{sb.Pick("2").Model()},
				IsPartOfBlock:     true,
			}, "")

			// then
			require.NoError(t, err)
			assert.Equal(t, model.BlockContentText_Paragraph, sb.Doc.Pick("1").Model().GetText().Style)
		})
	}

	for _, blockId := range []string{
		template.TitleBlockId,
		template.DescriptionBlockId,
	} {
		t.Run("paste - when to system blockId ("+blockId+")", func(t *testing.T) {
			// given
			sb := smarttest.New("text")
			require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithTitle))
			sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.Text(
						"",
						blockbuilder.ID(blockId),
					),
				)))

			// when
			cb := newFixture(t, sb)
			_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
				FocusedBlockId: blockId,
				FileSlot: []*pb.RpcBlockPasteRequestFile{
					{
						Name: "image.jpg",
					},
				},
			}, "")

			// then
			require.NoError(t, err)
			changes := sb.Doc.(*state.State).GetChanges()
			_, blockRemoved := lo.Find(changes, func(cc *pb.ChangeContent) bool {
				if blockRemove, ok := cc.Value.(*pb.ChangeContentValueOfBlockRemove); ok {
					_, blockIdFound := lo.Find(blockRemove.BlockRemove.Ids, func(s string) bool {
						return s == blockId
					})
					return blockIdFound
				}
				return false
			})
			require.False(t, blockRemoved)

			_, hasBlockId := lo.Find(sb.Doc.Pick("root").Model().ChildrenIds, func(s string) bool {
				return s == blockId
			})

			require.True(t, hasBlockId)
		})
	}
	t.Run("paste - when insert partially", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"123",
					blockbuilder.ID("1"),
				),
				blockbuilder.Text(
					"toggle",
					blockbuilder.ID("2"),
					blockbuilder.TextStyle(model.BlockContentText_Toggle),
				),
			)))

		// when
		cb := newFixture(t, sb)
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			FocusedBlockId:    "1",
			SelectedTextRange: &model.Range{From: 1, To: 1},
			AnySlot:           []*model.Block{sb.Pick("2").Model()},
			IsPartOfBlock:     true,
		}, "")

		// then
		require.NoError(t, err)
		assert.Equal(t, model.BlockContentText_Paragraph, sb.Pick("1").Model().GetText().Style)
	})
	t.Run("single description to empty title", func(t *testing.T) {
		// given
		sb := withTitle(t, "")
		addDescription(sb, "current description")
		cb := newFixture(t, sb)

		// when
		_, _, _, _, err := cb.Paste(nil, descriptionBlockReq(), "")

		// then
		require.NoError(t, err)
		assert.Equal(t, "current description", sb.Doc.Pick(template.DescriptionBlockId).Model().GetText().Text)
		find, _ := lo.Find(
			sb.Doc.Blocks(),
			func(block *model.Block) bool {
				return block.GetText() != nil && block.GetText().Text == "paste description"
			},
		)
		assert.True(t, true, find)
	})

	for _, blockIdToPasteTo := range []string{
		template.TitleBlockId,
		template.HeaderLayoutId,
		template.FeaturedRelationsId,
		template.DescriptionBlockId,
	} {
		t.Run("single text to "+blockIdToPasteTo, func(t *testing.T) {
			// given
			sb := withTitle(t, "")
			addRelations(sb)
			cb := newFixture(t, sb)

			// when
			_, _, _, _, err := cb.Paste(nil, requiredBlockReq(blockIdToPasteTo), "")

			// then
			require.NoError(t, err)
			assert.NotNil(t, sb.Doc.Pick(blockIdToPasteTo))
		})
	}
	t.Run("single to not empty title", func(t *testing.T) {
		st := withTitle(t, "title")
		cb := newFixture(t, st)
		req := singleBlockReq
		req.SelectedTextRange = &model.Range{From: 1, To: 4}
		_, _, _, _, err := cb.Paste(nil, req, "")
		require.NoError(t, err)
		assert.Equal(t, "tsinglee", st.Doc.Pick(template.TitleBlockId).Model().GetText().Text)
		assert.Equal(t, model.BlockContentText_Title, st.Doc.Pick(template.TitleBlockId).Model().GetText().Style)
	})
	t.Run("single to not empty title - select all", func(t *testing.T) {
		st := withTitle(t, "title")
		cb := newFixture(t, st)
		req := singleBlockReq
		req.SelectedTextRange = &model.Range{From: 0, To: 5}
		_, _, _, _, err := cb.Paste(nil, req, "")
		require.NoError(t, err)
		assert.Equal(t, "single", st.Doc.Pick(template.TitleBlockId).Model().GetText().Text)
		assert.Equal(t, model.BlockContentText_Title, st.Doc.Pick(template.TitleBlockId).Model().GetText().Style)
	})
	t.Run("multi to empty title", func(t *testing.T) {
		st := withTitle(t, "")
		cb := newFixture(t, st)
		_, _, _, _, err := cb.Paste(nil, multiBlockReq, "")
		require.NoError(t, err)
		rootChild := st.Doc.Pick(st.RootId()).Model().ChildrenIds
		assert.Equal(t, "first", st.Doc.Pick(template.TitleBlockId).Model().GetText().Text)
		assert.Equal(t, "second", st.Doc.Pick(rootChild[1]).Model().GetText().Text)
		assert.Equal(t, "third", st.Doc.Pick(rootChild[2]).Model().GetText().Text)
	})
	t.Run("multi to not empty title", func(t *testing.T) {
		st := withTitle(t, "title")
		cb := newFixture(t, st)
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
		cb := newFixture(t, st)
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
		cb := newFixture(t, st)
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
		// given
		ctx := session.NewContext()
		st := withTitle(t, "real title", "second")
		cb := newFixture(t, st)

		secondTextBlock := newTextBlock("second").Model()
		secondTextBlock.Id = "id0"

		req := pb.RpcBlockCutRequest{
			Blocks: []*model.Block{
				st.Doc.NewState().Get("title").Model(),
				secondTextBlock,
			},
			SelectedTextRange: &model.Range{},
		}

		// when
		_, _, anySlot, err := cb.Cut(ctx, req)

		// then
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
			result           = text
		)
		st := withBookmark(t, text, "", url)
		cb := newFixture(t, st)
		textBlock := newTextBlock(text).Model()
		textBlock.Id = firstTextBlockId
		bookmark := newBookmark(url).Model()
		bookmark.Id = bookmarkId
		blockCutReq := pb.RpcBlockCutRequest{
			ContextId:         "context",
			SelectedTextRange: &model.Range{From: 0, To: 11},
			Blocks:            []*model.Block{textBlock, bookmark},
		}
		textSlot, htmlSlot, anySlot, err := cb.Cut(session.NewContext(), blockCutReq)
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
			result           = firstText + "\n" + secondText
		)
		st := withBookmark(t, firstText, secondText, url)
		cb := newFixture(t, st)
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
		textSlot, htmlSlot, anySlot, err := cb.Cut(session.NewContext(), blockCutReq)
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
		cb := newFixture(t, st)
		req := pb.RpcBlockCutRequest{
			Blocks: []*model.Block{
				st.Doc.NewState().Get("title").Model(),
			},
			SelectedTextRange: &model.Range{From: 1, To: 3},
		}
		textSlot, htmlSlot, anySlot, err := cb.Cut(session.NewContext(), req)
		require.NoError(t, err)
		assert.Equal(t, "tle", st.Doc.Pick(template.TitleBlockId).Model().GetText().Text)
		assert.Equal(t, "it", textSlot)
		assert.NotContains(t, htmlSlot, ">title<")
		assert.Contains(t, htmlSlot, ">it<")
		require.Len(t, anySlot, 1)
		assert.Equal(t, "it", anySlot[0].GetText().Text)
	})

	t.Run("do not paste if Blocks restriction is set to smartblock", func(t *testing.T) {
		// given
		sb := smarttest.New("test")
		sb.TestRestrictions = restriction.Restrictions{Object: restriction.ObjectRestrictions{model.Restrictions_Blocks: {}}}
		cb := newFixture(t, sb)

		// when
		_, _, _, _, err := cb.Paste(nil, nil, "")

		// then
		assert.Error(t, err)
		assert.True(t, errors.Is(err, restriction.ErrRestricted))
	})
}

func addDescription(st *smarttest.SmartTest, description string) {
	newState := st.Doc.NewState()
	template.InitTemplate(newState, template.WithForcedDescription)
	newState.Get(template.DescriptionBlockId).(text.Block).SetText(description, nil)
	state.ApplyState("", newState, false)
}

func addRelations(st *smarttest.SmartTest) {
	newState := st.Doc.NewState()
	template.InitTemplate(newState, template.RequireHeader)
	template.InitTemplate(newState, template.WithFeaturedRelationsBlock)
	template.InitTemplate(newState, template.WithForcedDescription)
	state.ApplyState("", newState, false)
}

func TestClipboard_PasteToCodeBlock(t *testing.T) {
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

	cb := newFixture(t, sb)
	_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
		FocusedBlockId:    codeBlock.Model().Id,
		SelectedTextRange: &model.Range{4, 5},
		TextSlot:          "\nsome text\nhere\n",
	}, "")
	require.NoError(t, err)
	assert.Equal(t, "some\nsome text\nhere\ncode", sb.Doc.Pick(codeBlock.Model().Id).Model().GetText().Text)
	assert.Equal(t, model.BlockContentText_Code, sb.Doc.Pick(codeBlock.Model().Id).Model().GetText().Style)
}

func TestClipboard_PasteToTableCellBlock(t *testing.T) {
	// given
	sb := smarttest.New("text")
	sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
		blockbuilder.ID("root"),
		blockbuilder.Children(
			blockbuilder.Text(
				"table",
				blockbuilder.ID("2-2"),
				blockbuilder.TextStyle(model.BlockContentText_Paragraph),
			),
		)))

	// when
	cb := newFixture(t, sb)
	_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
		FocusedBlockId:    "2-2",
		SelectedTextRange: rng(0, 0),
		AnySlot: []*model.Block{
			blockbuilder.Text(
				"text1",
				blockbuilder.ID("id1"),
				blockbuilder.TextStyle(model.BlockContentText_Code),
			).Block(),
			blockbuilder.Text(
				"text2",
				blockbuilder.ID("id2"),
				blockbuilder.TextStyle(model.BlockContentText_Toggle),
			).Block(),
			blockbuilder.Text(
				"table",
				blockbuilder.ID("2-2"),
				blockbuilder.TextStyle(model.BlockContentText_Paragraph),
			).Block(),
		},
	}, "")

	// then
	require.NoError(t, err)
	assert.Equal(t, "text1\ntext2\ntabletable", sb.Doc.Pick("2-2").Model().GetText().Text)
}

func Test_PasteText(t *testing.T) {

	t.Run("paste", func(t *testing.T) {
		// given
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

		// when
		cb := newFixture(t, sb)
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			SelectedBlockIds: []string{"1", "2"},
			TextSlot:         "One string",
		}, "")

		// then
		require.NoError(t, err)
		assert.Equal(t, "One string", sb.NewState().Snippet())
	})

	t.Run("paste - when asterisks", func(t *testing.T) {
		// given
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

		// when
		cb := newFixture(t, sb)
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			SelectedBlockIds: []string{"1"},
			TextSlot:         "a * b * c",
			HtmlSlot:         "<meta charset='utf-8'><p data-pm-slice=\"1 1 []\">a *<em> b</em> * c</p>",
		}, "")

		// then
		require.NoError(t, err)
		assert.Equal(t, "a * b * c", sb.NewState().Snippet())
	})
}

func Test_CopyAndCutText(t *testing.T) {

	t.Run("preserve style - when empty text copied", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"toggle",
					blockbuilder.ID("2"),
					blockbuilder.TextStyle(model.BlockContentText_Toggle),
					blockbuilder.BackgroundColor("grey"),
				),
			)))

		// when
		cb := newFixture(t, sb)
		_, _, anySlotCopy, err := cb.Copy(nil, pb.RpcBlockCopyRequest{
			Blocks:            []*model.Block{sb.Pick("2").Model()},
			SelectedTextRange: &model.Range{From: 1, To: 1},
		})
		_, _, anySlotCut, err := cb.Cut(nil, pb.RpcBlockCutRequest{
			SelectedTextRange: &model.Range{From: 1, To: 1},
			Blocks:            []*model.Block{sb.Pick("2").Model()},
		})

		// then
		require.NoError(t, err)

		assert.Equal(t, model.BlockContentText_Toggle, anySlotCopy[0].GetText().Style)
		assert.Equal(t, model.BlockContentText_Toggle, anySlotCut[0].GetText().Style)

		assert.Equal(t, "", anySlotCopy[0].GetText().Text)
		assert.Equal(t, "", anySlotCut[0].GetText().Text)

		assert.Equal(t, "grey", anySlotCopy[0].BackgroundColor)
		assert.Equal(t, "grey", anySlotCut[0].BackgroundColor)
	})

	t.Run("do not preserve style - when not empty and not full text copied", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"toggle",
					blockbuilder.ID("2"),
					blockbuilder.TextStyle(model.BlockContentText_Toggle),
				),
			)))

		// when
		cb := newFixture(t, sb)
		_, _, anySlotCopy, err := cb.Copy(nil, pb.RpcBlockCopyRequest{
			Blocks:            []*model.Block{sb.Pick("2").Model()},
			SelectedTextRange: &model.Range{From: 1, To: 2},
		})
		_, _, anySlotCut, err := cb.Cut(nil, pb.RpcBlockCutRequest{
			SelectedTextRange: &model.Range{From: 1, To: 2},
			Blocks:            []*model.Block{sb.Pick("2").Model()},
		})

		// then
		require.NoError(t, err)

		assert.Equal(t, model.BlockContentText_Paragraph, anySlotCopy[0].GetText().Style)
		assert.Equal(t, model.BlockContentText_Paragraph, anySlotCut[0].GetText().Style)

		assert.Equal(t, "", anySlotCopy[0].BackgroundColor)
		assert.Equal(t, "", anySlotCut[0].BackgroundColor)
	})

	t.Run("copy/cut preserve style - when full text copied", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
			blockbuilder.ID("root"),
			blockbuilder.Children(
				blockbuilder.Text(
					"toggle",
					blockbuilder.ID("2"),
					blockbuilder.TextStyle(model.BlockContentText_Toggle),
					blockbuilder.BackgroundColor("grey"),
				),
			)))

		// when
		cb := newFixture(t, sb)
		_, _, anySlotCopy, err := cb.Copy(nil, pb.RpcBlockCopyRequest{
			Blocks:            []*model.Block{sb.Pick("2").Model()},
			SelectedTextRange: &model.Range{From: 0, To: int32(textutil.UTF16RuneCountString(sb.Pick("2").Model().GetText().Text))},
		})
		_, _, anySlotCut, err := cb.Cut(nil, pb.RpcBlockCutRequest{
			SelectedTextRange: &model.Range{From: 0, To: int32(textutil.UTF16RuneCountString(sb.Pick("2").Model().GetText().Text))},
			Blocks:            []*model.Block{sb.Pick("2").Model()},
		})

		// then
		require.NoError(t, err)

		assert.Equal(t, model.BlockContentText_Toggle, anySlotCopy[0].GetText().Style)
		assert.Equal(t, model.BlockContentText_Toggle, anySlotCut[0].GetText().Style)

		assert.Equal(t, "grey", anySlotCopy[0].BackgroundColor)
		assert.Equal(t, "grey", anySlotCut[0].BackgroundColor)
	})

	t.Run("copy/cut - when with children", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithEmpty))
		s := sb.NewState()
		block1 := &model.Block{
			Id: "1",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "some text 1",
				},
			},
		}
		simpleBlock1 := simple.New(block1)
		s.Add(simpleBlock1)
		s.InsertTo("", model.Block_Inner, simpleBlock1.Model().Id)
		block2 := &model.Block{
			Id: "2",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "some text 2",
				},
			},
		}
		simpleBlock2 := simple.New(block2)
		s.Add(simpleBlock2)
		s.InsertTo("1", model.Block_Inner, simpleBlock2.Model().Id)
		require.NoError(t, sb.Apply(s))

		// when
		cb := newFixture(t, sb)
		textSlotCopy, _, _, err := cb.Copy(nil, pb.RpcBlockCopyRequest{
			Blocks: []*model.Block{block1, block2},
		})
		textSlotCut, _, _, err := cb.Cut(nil, pb.RpcBlockCutRequest{
			SelectedTextRange: &model.Range{},
			Blocks:            []*model.Block{block1, block2},
		})

		// then
		require.NoError(t, err)
		const expected = "some text 1\n\tsome text 2"
		assert.Equal(t, expected, textSlotCopy)
		assert.Equal(t, expected, textSlotCut)
	})

	t.Run("copy/cut - when numbered with children", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithEmpty))
		s := sb.NewState()

		block1 := givenRow1Level1NumberedBlock(s)
		block2 := givenRow2Level2NumberedBlockNestedInFirst(s)
		block3 := givenRow3Level1NumberedBlock(s)
		block4 := givenRow4Level1TextBlock(s)
		block5 := givenRow5Level1NumberedBlock(s)
		block6 := givenRow6Level1NumberedBlock(s)
		require.NoError(t, sb.Apply(s))

		// when
		cb := newFixture(t, sb)
		textSlotCopy, _, _, err := cb.Copy(nil, pb.RpcBlockCopyRequest{
			Blocks: []*model.Block{block1, block2, block3, block4, block5, block6},
		})
		textSlotCut, _, _, err := cb.Cut(nil, pb.RpcBlockCutRequest{
			SelectedTextRange: &model.Range{},
			Blocks:            []*model.Block{block1, block2, block3, block4, block5, block6},
		})

		// then
		require.NoError(t, err)
		const expected = "1. A-1\n\t1. B-1\n2. C-1\nD-1\n1. E-1\n2. F-1"
		assert.Equal(t, expected, textSlotCopy)
		assert.Equal(t, expected, textSlotCut)
	})

	t.Run("cut/copy - text range from 0", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithEmpty))
		s := sb.NewState()

		bl := givenBlockWithStyle(0, "")
		insertBlock(s, bl, "")
		require.NoError(t, sb.Apply(s))

		// when
		cb := newFixture(t, sb)
		textSlotCopy, _, _, err := cb.Copy(nil, pb.RpcBlockCopyRequest{
			SelectedTextRange: &model.Range{From: 0, To: 7},
			Blocks:            []*model.Block{bl},
		})
		textSlotCut, _, _, err := cb.Cut(nil, pb.RpcBlockCutRequest{
			SelectedTextRange: &model.Range{From: 0, To: 7},
			Blocks:            []*model.Block{bl},
		})

		// then
		require.NoError(t, err)
		const expected = "some te"
		assert.Equal(t, expected, textSlotCopy)
		assert.Equal(t, expected, textSlotCut)
		assert.Len(t, sb.Blocks(), 2)
	})

	t.Run("cut/copy - text range from 0 to the end of block", func(t *testing.T) {
		// given
		const expected = "some text 1"
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithEmpty))
		s := sb.NewState()

		bl := givenBlockWithStyle(0, "")
		insertBlock(s, bl, "")
		require.NoError(t, sb.Apply(s))

		// when
		cb := newFixture(t, sb)
		textSlotCopy, _, _, err := cb.Copy(nil, pb.RpcBlockCopyRequest{
			SelectedTextRange: &model.Range{From: 0, To: int32(len(expected))},
			Blocks:            []*model.Block{bl},
		})
		textSlotCut, _, _, err := cb.Cut(nil, pb.RpcBlockCutRequest{
			SelectedTextRange: &model.Range{From: 0, To: int32(len(expected))},
			Blocks:            []*model.Block{bl},
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, expected, textSlotCopy)
		assert.Equal(t, expected, textSlotCut)
		assert.Len(t, sb.Blocks(), 1)
	})

	t.Run("cut/copy - inner text range", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithEmpty))
		s := sb.NewState()

		bl := givenBlockWithStyle(0, "")
		insertBlock(s, bl, "")
		require.NoError(t, sb.Apply(s))

		// when
		cb := newFixture(t, sb)
		textSlotCopy, _, _, err := cb.Copy(nil, pb.RpcBlockCopyRequest{
			SelectedTextRange: &model.Range{From: 2, To: 8},
			Blocks:            []*model.Block{bl},
		})
		textSlotCut, _, _, err := cb.Cut(nil, pb.RpcBlockCutRequest{
			SelectedTextRange: &model.Range{From: 2, To: 8},
			Blocks:            []*model.Block{bl},
		})

		// then
		require.NoError(t, err)
		const expected = "me tex"
		assert.Equal(t, expected, textSlotCopy)
		assert.Equal(t, expected, textSlotCut)
		assert.Len(t, sb.Blocks(), 2)
	})

	t.Run("cut/copy - text range from 0 to 0", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithEmpty))
		s := sb.NewState()

		bl := givenBlockWithStyle(0, "")
		insertBlock(s, bl, "")
		require.NoError(t, sb.Apply(s))

		// when
		cb := newFixture(t, sb)
		textSlotCopy, _, anySlotCopy, err := cb.Copy(nil, pb.RpcBlockCopyRequest{
			SelectedTextRange: &model.Range{From: 0, To: 0},
			Blocks:            []*model.Block{bl},
		})
		textSlotCut, _, anySlotCut, err := cb.Cut(nil, pb.RpcBlockCutRequest{
			SelectedTextRange: &model.Range{From: 0, To: 0},
			Blocks:            []*model.Block{bl},
		})

		// then
		require.NoError(t, err)
		const expected = "some text 1"
		assert.Equal(t, expected, textSlotCopy)
		assert.Equal(t, expected, textSlotCut)
		assert.Len(t, sb.Blocks(), 1)
		assert.Len(t, anySlotCopy, 1)
		assert.Len(t, anySlotCut, 1)
	})
}

func givenRow3Level1NumberedBlock(s *state.State) *model.Block {
	numberedBlock := givenNumberedBlock("3", "C-1")
	insertBlock(s, numberedBlock, "")
	return numberedBlock
}

func givenRow4Level1TextBlock(s *state.State) *model.Block {
	block := &model.Block{
		Id: "4",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "D-1",
			},
		},
	}
	insertBlock(s, block, "")
	return block
}

func givenRow5Level1NumberedBlock(s *state.State) *model.Block {
	numberedBlock := givenNumberedBlock("5", "E-1")
	insertBlock(s, numberedBlock, "")
	return numberedBlock
}

func givenRow6Level1NumberedBlock(s *state.State) *model.Block {
	numberedBlock := givenNumberedBlock("6", "F-1")
	insertBlock(s, numberedBlock, "")
	return numberedBlock
}

func givenRow2Level2NumberedBlockNestedInFirst(s *state.State) *model.Block {
	numberedBlock := givenNumberedBlock("2", "B-1")
	insertBlock(s, numberedBlock, "1")
	return numberedBlock
}

func givenRow1Level1NumberedBlock(s *state.State) *model.Block {
	numberedBlock := givenNumberedBlock("1", "A-1")
	insertBlock(s, numberedBlock, "")
	return numberedBlock
}

func insertBlock(s *state.State, block1 *model.Block, targetID string) {
	simpleBlock1 := simple.New(block1)
	s.Add(simpleBlock1)
	s.InsertTo(targetID, model.Block_Inner, simpleBlock1.Model().Id)
}

func givenNumberedBlock(id string, text string) *model.Block {
	return &model.Block{
		Id: id,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  text,
				Style: model.BlockContentText_Numbered,
			},
		},
	}
}

func Test_StyleAndTabExtractionIgnoreStyle(t *testing.T) {
	type fixture struct {
		styleName string
		style     model.BlockContentTextStyle
		expected  string
		emoji     string
	}
	testData := []*fixture{
		{"title", model.BlockContentText_Title, "some text 1", ""},
		{"header1", model.BlockContentText_Header1, "some text 1", ""},
		{"header2", model.BlockContentText_Header2, "some text 1", ""},
		{"header3", model.BlockContentText_Header3, "some text 1", ""},
		{"header4", model.BlockContentText_Header4, "some text 1", ""},
		{"quote", model.BlockContentText_Quote, "some text 1", ""},
		{"code", model.BlockContentText_Code, "some text 1", ""},
		{"checkbox", model.BlockContentText_Checkbox, "some text 1", ""},
		{"bulleted", model.BlockContentText_Marked, "some text 1", ""},
		{"numbered", model.BlockContentText_Numbered, "some text 1", ""},
		{"callout", model.BlockContentText_Callout, "some text 1", "👍"},
	}

	for _, testCase := range testData {
		t.Run("extract - when style is "+testCase.styleName, func(t *testing.T) {
			// given
			givenBlock := givenBlockWithStyle(testCase.style, testCase.emoji)

			// when
			result, _ := extractTextWithStyleAndTabs(givenBlock, []string{}, 1, 0, true)

			// then
			assert.Equal(t, []string{testCase.expected}, result)
		})
	}
}

func Test_StyleAndTabExtraction(t *testing.T) {
	type fixture struct {
		styleName string
		style     model.BlockContentTextStyle
		expected  string
		emoji     string
	}
	testDataWithStyle := []*fixture{
		{"title", model.BlockContentText_Title, "\t# some text 1", ""},
		{"header1", model.BlockContentText_Header1, "\t## some text 1", ""},
		{"header2", model.BlockContentText_Header2, "\t### some text 1", ""},
		{"header3", model.BlockContentText_Header3, "\t#### some text 1", ""},
		{"header4", model.BlockContentText_Header4, "\t##### some text 1", ""},
		{"quote", model.BlockContentText_Quote, "\t> some text 1", ""},
		{"code", model.BlockContentText_Code, "\t```some text 1```", ""},
		{"checkbox", model.BlockContentText_Checkbox, "\t- [ ] some text 1", ""},
		{"bulleted", model.BlockContentText_Marked, "\t- some text 1", ""},
		{"numbered", model.BlockContentText_Numbered, "\t1. some text 1", ""},
		{"callout", model.BlockContentText_Callout, "\t👍 some text 1", "👍"},
	}

	for _, testCase := range testDataWithStyle {
		t.Run("extract - when style is "+testCase.styleName, func(t *testing.T) {
			// given
			givenBlock := givenBlockWithStyle(testCase.style, testCase.emoji)

			// when
			result, _ := extractTextWithStyleAndTabs(givenBlock, []string{}, 1, 0, false)

			// then
			assert.Equal(t, []string{testCase.expected}, result)
		})
	}
}

func givenBlockWithStyle(style model.BlockContentTextStyle, emoji string) *model.Block {
	return &model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:      "some text 1",
				Style:     style,
				IconEmoji: emoji,
			},
		},
	}
}

func Test_splitStringIntoParagraphs(t *testing.T) {
	type args struct {
		s                  string
		lineBreakSoftLimit int
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			"double",
			args{
				s: `aaa

bbb

ccc`,
				lineBreakSoftLimit: 1024,
			},
			[]string{"aaa", "bbb", "ccc"},
		},
		{
			"double with whitespaces",
			args{
				s: `aaa
   
bbb
 
ccc`,
				lineBreakSoftLimit: 1024,
			},
			[]string{"aaa", "bbb", "ccc"},
		},
		{
			"more than 2 line breaks with whitespaces",
			args{
				s: `aaa
   

 
  


bbb


ccc`,
				lineBreakSoftLimit: 1024,
			},
			[]string{"aaa", "bbb", "ccc"},
		},
		{
			"single",
			args{
				s: `aaa
bbb`,
				lineBreakSoftLimit: 1024,
			},
			[]string{`aaa
bbb`},
		},
		{
			"mixed",
			args{
				s: `aaa
bbb

ccc`,
				lineBreakSoftLimit: 1024,
			},
			[]string{`aaa
bbb`, "ccc"},
		},
		{
			"soft limit",
			args{
				s: `very long string that is longer than the soft limit
bbb`,
				lineBreakSoftLimit: 15,
			},
			[]string{`very long string that is longer than the soft limit`, `bbb`},
		},
		{
			"soft limit disabled",
			args{
				s: `very long string
bbb`,
				lineBreakSoftLimit: 0,
			},
			[]string{`very long string
bbb`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, splitStringIntoParagraphs(tt.args.s, tt.args.lineBreakSoftLimit), "splitStringIntoParagraphs(%v, %v)", tt.args.s, tt.args.lineBreakSoftLimit)
		})
	}
}

func TestProcessFileBlock(t *testing.T) {
	const (
		fileObject1 = "fileObject1"
		fileObject2 = "fileObject2"
		space1      = "space1"
		space2      = "space2"
		fileId      = domain.FileId("fileId")
	)

	sb := smarttest.New("test")
	sb.SetSpaceId(space1)

	t.Run("old target object id remains if space is the same", func(t *testing.T) {
		// given
		file := mock_fileobject.NewMockService(t)
		file.EXPECT().GetFileIdFromObject(fileObject1).Return(domain.FullFileId{SpaceId: space1, FileId: fileId}, nil)

		c := &clipboard{
			SmartBlock:        sb,
			fileObjectService: file,
		}

		fb := &model.BlockContentOfFile{File: &model.BlockContentFile{TargetObjectId: fileObject1}}

		// when
		c.processFileBlock(fb)

		// then
		assert.Equal(t, fileObject1, fb.File.TargetObjectId)
	})

	t.Run("new target object id is set if space is different", func(t *testing.T) {
		// given
		file := mock_fileobject.NewMockService(t)
		file.EXPECT().GetFileIdFromObject(fileObject1).Return(domain.FullFileId{SpaceId: space2, FileId: fileId}, nil)
		file.EXPECT().CreateFromImport(domain.FullFileId{FileId: fileId, SpaceId: space1}, mock.Anything).Return(fileObject2, nil)

		c := &clipboard{
			SmartBlock:        sb,
			fileObjectService: file,
		}

		fb := &model.BlockContentOfFile{File: &model.BlockContentFile{TargetObjectId: fileObject1}}

		// when
		c.processFileBlock(fb)

		// then
		assert.Equal(t, fileObject2, fb.File.TargetObjectId)
	})

	t.Run("old target object id remains if failed to create new object", func(t *testing.T) {
		// given
		file := mock_fileobject.NewMockService(t)
		file.EXPECT().GetFileIdFromObject(fileObject1).Return(domain.FullFileId{SpaceId: space2, FileId: fileId}, nil)
		file.EXPECT().CreateFromImport(domain.FullFileId{FileId: fileId, SpaceId: space1}, mock.Anything).Return("", fmt.Errorf("some error"))

		c := &clipboard{
			SmartBlock:        sb,
			fileObjectService: file,
		}

		fb := &model.BlockContentOfFile{File: &model.BlockContentFile{TargetObjectId: fileObject1}}

		// when
		c.processFileBlock(fb)

		// then
		assert.Equal(t, fileObject1, fb.File.TargetObjectId)
	})

	t.Run("old target object id remains if failed to get file id", func(t *testing.T) {
		// given
		file := mock_fileobject.NewMockService(t)
		file.EXPECT().GetFileIdFromObject(fileObject1).Return(domain.FullFileId{}, fmt.Errorf("not found"))

		c := &clipboard{
			SmartBlock:        sb,
			fileObjectService: file,
		}

		fb := &model.BlockContentOfFile{File: &model.BlockContentFile{TargetObjectId: fileObject1}}

		// when
		c.processFileBlock(fb)

		// then
		assert.Equal(t, fileObject1, fb.File.TargetObjectId)
	})
}
