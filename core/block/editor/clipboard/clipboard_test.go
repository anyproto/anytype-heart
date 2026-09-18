package clipboard

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
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
	_ "github.com/anyproto/anytype-heart/core/block/simple/file"
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
	t.Run("1. Cursor at the beginning, range == 0. Expected behavior: paste text is merged into block at cursor position", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteText(t, sb, "4", model.Range{From: 0, To: 0}, []string{}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa\nbbbbbqwerty", "55555"})
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
	t.Run("paste - when pasted blocks contain featuredRelations system block", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithTitle))
		cb := newFixture(t, sb)

		// when
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			AnySlot: []*model.Block{
				{
					Id: "paste1",
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{Text: "some text"},
					},
				},
				{
					Id:      template.FeaturedRelationsId,
					Content: &model.BlockContentOfFeaturedRelations{FeaturedRelations: &model.BlockContentFeaturedRelations{}},
				},
			},
		}, "")

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "system block")
		assert.Contains(t, err.Error(), template.FeaturedRelationsId)
	})
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

func TestClipboard_PasteToCodeBlock_TrailingNewline(t *testing.T) {
	t.Run("strip trailing newline when selection extends to end", func(t *testing.T) {
		// given
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

		// when — select " code" (positions 4..9, end of text) and paste with trailing \n
		cb := newFixture(t, sb)
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			FocusedBlockId:    codeBlock.Model().Id,
			SelectedTextRange: rng(4, 9),
			TextSlot:          " replacement\n",
		}, "")

		// then — trailing newline should be stripped
		require.NoError(t, err)
		assert.Equal(t, "some replacement", sb.Doc.Pick(codeBlock.Model().Id).Model().GetText().Text)
	})

	t.Run("full text replacement strips trailing newline", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithTitle))
		s := sb.NewState()
		codeBlock := simple.New(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Style: model.BlockContentText_Code,
					Text:  "hello world",
				},
			},
		})
		s.Add(codeBlock)
		s.InsertTo("", model.Block_Inner, codeBlock.Model().Id)
		require.NoError(t, sb.Apply(s))

		// when — select all text and paste with trailing \n
		cb := newFixture(t, sb)
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			FocusedBlockId:    codeBlock.Model().Id,
			SelectedTextRange: rng(0, 11),
			TextSlot:          "replacement\n",
		}, "")

		// then — no blank line at the end
		require.NoError(t, err)
		assert.Equal(t, "replacement", sb.Doc.Pick(codeBlock.Model().Id).Model().GetText().Text)
	})

	t.Run("preserve trailing newline when selection is in the middle", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithTitle))
		s := sb.NewState()
		codeBlock := simple.New(&model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Style: model.BlockContentText_Code,
					Text:  "some code here",
				},
			},
		})
		s.Add(codeBlock)
		s.InsertTo("", model.Block_Inner, codeBlock.Model().Id)
		require.NoError(t, sb.Apply(s))

		// when — select "code" (positions 5..9, NOT end of text) and paste with trailing \n
		cb := newFixture(t, sb)
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			FocusedBlockId:    codeBlock.Model().Id,
			SelectedTextRange: rng(5, 9),
			TextSlot:          "block\n",
		}, "")

		// then — trailing newline should be preserved as a separator
		require.NoError(t, err)
		assert.Equal(t, "some block\n here", sb.Doc.Pick(codeBlock.Model().Id).Model().GetText().Text)
	})
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

func TestPasteIntoEmptyStyledBlock(t *testing.T) {
	for _, tc := range []struct {
		name        string
		style       model.BlockContentTextStyle
		pasteBlocks []*model.Block
	}{
		{
			name:  "multi-block header paste into empty bullet",
			style: model.BlockContentText_Marked,
			pasteBlocks: []*model.Block{
				withBold(blockbuilder.Text("Header Text", blockbuilder.ID("p1"), blockbuilder.TextStyle(model.BlockContentText_Header1)).Block(), 0, 6),
				blockbuilder.Text("Paragraph Text", blockbuilder.ID("p2"), blockbuilder.TextStyle(model.BlockContentText_Paragraph)).Block(),
			},
		},
		{
			name:  "multi-block header paste into empty toggle",
			style: model.BlockContentText_Toggle,
			pasteBlocks: []*model.Block{
				withBold(blockbuilder.Text("Header Text", blockbuilder.ID("p1"), blockbuilder.TextStyle(model.BlockContentText_Header1)).Block(), 0, 6),
				blockbuilder.Text("Paragraph Text", blockbuilder.ID("p2"), blockbuilder.TextStyle(model.BlockContentText_Paragraph)).Block(),
			},
		},
		{
			name:  "multi-block header paste into empty callout",
			style: model.BlockContentText_Callout,
			pasteBlocks: []*model.Block{
				withBold(blockbuilder.Text("Header Text", blockbuilder.ID("p1"), blockbuilder.TextStyle(model.BlockContentText_Header1)).Block(), 0, 6),
				blockbuilder.Text("Paragraph Text", blockbuilder.ID("p2"), blockbuilder.TextStyle(model.BlockContentText_Paragraph)).Block(),
			},
		},
		{
			name:  "single header paste into empty bullet",
			style: model.BlockContentText_Marked,
			pasteBlocks: []*model.Block{
				withBold(blockbuilder.Text("Header Text", blockbuilder.ID("p1"), blockbuilder.TextStyle(model.BlockContentText_Header1)).Block(), 0, 6),
			},
		},
		{
			name:  "multi-block paragraph paste into empty checkbox",
			style: model.BlockContentText_Checkbox,
			pasteBlocks: []*model.Block{
				withBold(blockbuilder.Text("First Line", blockbuilder.ID("p1"), blockbuilder.TextStyle(model.BlockContentText_Paragraph)).Block(), 0, 5),
				blockbuilder.Text("Second Line", blockbuilder.ID("p2"), blockbuilder.TextStyle(model.BlockContentText_Paragraph)).Block(),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			sb := smarttest.New("test")
			sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.Text(
						"",
						blockbuilder.ID("1"),
						blockbuilder.TextStyle(tc.style),
					),
				)))

			// when
			cb := newFixture(t, sb)
			blockIds, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
				FocusedBlockId:    "1",
				SelectedTextRange: &model.Range{From: 0, To: 0},
				AnySlot:           tc.pasteBlocks,
			}, "")

			// then
			require.NoError(t, err)
			// Resolve the replacement through the live tree. forkFilledEmptyBlock
			// unlinks the focused block and re-creates it under a fresh id, so
			// Pick("1") still returns the unlinked original — asserting on that
			// passes even when the block actually in the document lost its style.
			st := sb.NewState()
			childIds := st.Pick("root").Model().ChildrenIds
			assert.NotContains(t, childIds, "1", "the original block id must be unlinked after the fork")
			// one live block per pasted block, single- and multi-block alike, so a paste
			// that duplicates the first line instead of reusing the target is caught
			require.Len(t, childIds, len(tc.pasteBlocks), "one live block per pasted block")
			// the caller must be handed the live replacement, never the unlinked target
			assert.Equal(t, childIds, blockIds, "returned ids must be the live blocks, in document order")

			targetBlock := st.Pick(childIds[0])
			require.NotNil(t, targetBlock, "replacement block should be in the document")
			assert.Equal(t, tc.style, targetBlock.Model().GetText().Style, "target block style should be preserved")

			// The empty focused block is reused for the first paste line (both for
			// single- and multi-block paste) instead of being left as a stray empty
			// paragraph above the pasted content; remaining paste blocks are inserted
			// below. The block keeps its own style.
			assert.Equal(t, tc.pasteBlocks[0].GetText().Text, targetBlock.Model().GetText().Text, "first paste text should be merged into target")
			assert.Equal(t, tc.pasteBlocks[0].GetText().Marks.GetMarks(), targetBlock.Model().GetText().Marks.GetMarks(),
				"the first pasted line's marks must be carried into the reused block")
			if len(tc.pasteBlocks) > 1 {
				lastPasteBlock := st.Pick(childIds[len(childIds)-1])
				require.NotNil(t, lastPasteBlock, "last paste block should be in the document")
				assert.Equal(t, tc.pasteBlocks[len(tc.pasteBlocks)-1].GetText().Text, lastPasteBlock.Model().GetText().Text, "remaining paste block text should be preserved")
			}
		})
	}
}

// A plain empty paragraph carries no style intent of its own, so a multi-block paste
// must drop it and let the first pasted block land untouched. Contrast with
// TestPasteIntoEmptyStyledBlock, where the target's own style is what has to survive.
func TestPasteMultiBlockIntoEmptyParagraph(t *testing.T) {
	type want struct {
		style   model.BlockContentTextStyle
		text    string
		checked bool
		color   string
		bgColor string
		icon    string
		lang    string
		// text of each child, so a corrupted child is caught rather than just a missing one
		children []string
		// text of each grandchild: one level of nesting cannot show that deeper levels survive
		grandchildren []string
		// the marks carried onto the first pasted block, compared in full: a count alone
		// passes when the type or range is corrupted
		marks []*model.BlockContentTextMark
	}
	for _, tc := range []struct {
		name        string
		pasteBlocks []*model.Block
		want        want
	}{
		{
			name: "header style is adopted",
			pasteBlocks: []*model.Block{
				withBold(textBlock("p1", "Title", model.BlockContentText_Header2), 0, 5),
				textBlock("p2", "body", model.BlockContentText_Paragraph),
			},
			want: want{style: model.BlockContentText_Header2, text: "Title", marks: []*model.BlockContentTextMark{
				{Range: &model.Range{From: 0, To: 5}, Type: model.BlockContentTextMark_Bold},
				{Range: &model.Range{From: 0, To: 5}, Type: model.BlockContentTextMark_Link,
					Param: "https://example.com/kept"},
			}},
		},
		{
			name: "checked checkbox stays checked",
			pasteBlocks: []*model.Block{
				func() *model.Block {
					b := textBlock("p1", "done", model.BlockContentText_Checkbox)
					b.GetText().Checked = true
					return b
				}(),
				textBlock("p2", "todo", model.BlockContentText_Checkbox),
			},
			want: want{style: model.BlockContentText_Checkbox, text: "done", checked: true},
		},
		{
			name: "title style is demoted, never adopted",
			pasteBlocks: []*model.Block{
				textBlock("p1", "I am a title", model.BlockContentText_Title),
				textBlock("p2", "body", model.BlockContentText_Paragraph),
			},
			want: want{style: model.BlockContentText_Header1, text: "I am a title"},
		},
		{
			name: "description style is demoted, never adopted",
			pasteBlocks: []*model.Block{
				textBlock("p1", "I am a description", model.BlockContentText_Description),
				textBlock("p2", "body", model.BlockContentText_Paragraph),
			},
			want: want{style: model.BlockContentText_Paragraph, text: "I am a description"},
		},
		{
			name: "code block keeps its language",
			pasteBlocks: []*model.Block{
				func() *model.Block {
					b := textBlock("p1", "fmt.Println(1)", model.BlockContentText_Code)
					b.Fields = &types.Struct{Fields: map[string]*types.Value{
						"lang": pbtypes.String("go"),
					}}
					return b
				}(),
				textBlock("p2", "after", model.BlockContentText_Paragraph),
			},
			want: want{style: model.BlockContentText_Code, text: "fmt.Println(1)", lang: "go"},
		},
		{
			name: "callout keeps its icon and colors",
			pasteBlocks: []*model.Block{
				func() *model.Block {
					b := textBlock("p1", "note", model.BlockContentText_Callout)
					b.GetText().IconEmoji = "💡"
					b.GetText().Color = "red"
					b.BackgroundColor = "blue"
					return b
				}(),
				textBlock("p2", "body", model.BlockContentText_Paragraph),
			},
			want: want{
				style: model.BlockContentText_Callout, text: "note",
				color: "red", bgColor: "blue", icon: "💡",
			},
		},
		{
			name: "toggle keeps its children and grandchildren",
			pasteBlocks: []*model.Block{
				func() *model.Block {
					b := textBlock("p1", "toggle head", model.BlockContentText_Toggle)
					b.ChildrenIds = []string{"c1"}
					return b
				}(),
				func() *model.Block {
					b := textBlock("c1", "nested child", model.BlockContentText_Paragraph)
					b.ChildrenIds = []string{"g1"}
					return b
				}(),
				textBlock("g1", "nested grandchild", model.BlockContentText_Paragraph),
				textBlock("p2", "body", model.BlockContentText_Paragraph),
			},
			want: want{
				style: model.BlockContentText_Toggle, text: "toggle head",
				children:      []string{"nested child"},
				grandchildren: []string{"nested grandchild"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			sb := smarttest.New("test")
			sb.Doc = testutil.BuildStateFromAST(blockbuilder.Root(
				blockbuilder.ID("root"),
				blockbuilder.Children(
					blockbuilder.Text("", blockbuilder.ID("1"),
						blockbuilder.TextStyle(model.BlockContentText_Paragraph)),
				)))
			cb := newFixture(t, sb)

			// when
			blockIds, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
				FocusedBlockId:    "1",
				SelectedTextRange: &model.Range{From: 0, To: 0},
				AnySlot:           tc.pasteBlocks,
			}, "")

			// then
			require.NoError(t, err)
			childIds := sb.NewState().Pick("root").Model().ChildrenIds
			require.Len(t, childIds, 2, "the empty paragraph must be dropped, not left above the paste")
			// the caller is told about every block that landed, nested ones included, in
			// document order — a count alone passes when an id is duplicated or misordered
			st0 := sb.NewState()
			var walk func(ids []string) []string
			walk = func(ids []string) []string {
				var out []string
				for _, id := range ids {
					out = append(out, id)
					if b := st0.Pick(id); b != nil {
						out = append(out, walk(b.Model().ChildrenIds)...)
					}
				}
				return out
			}
			assert.Equal(t, walk(childIds), blockIds,
				"returned ids must be the live pasted tree in traversal order")

			st := sb.NewState()
			first := st.Pick(childIds[0])
			require.NotNil(t, first)
			var childTexts, grandchildTexts []string
			for _, id := range first.Model().ChildrenIds {
				child := st.Pick(id)
				require.NotNil(t, child, "pasted child %s must be in the document", id)
				childTexts = append(childTexts, child.Model().GetText().Text)
				for _, gid := range child.Model().ChildrenIds {
					g := st.Pick(gid)
					require.NotNil(t, g, "pasted grandchild %s must be in the document", gid)
					grandchildTexts = append(grandchildTexts, g.Model().GetText().Text)
				}
			}
			got := want{
				style:         first.Model().GetText().Style,
				text:          first.Model().GetText().Text,
				checked:       first.Model().GetText().Checked,
				color:         first.Model().GetText().Color,
				bgColor:       first.Model().BackgroundColor,
				icon:          first.Model().GetText().IconEmoji,
				lang:          pbtypes.GetString(first.Model().Fields, "lang"),
				children:      childTexts,
				grandchildren: grandchildTexts,
				marks:         first.Model().GetText().Marks.GetMarks(),
			}
			assert.Equal(t, tc.want, got)

			// the trailing pasted block must survive too
			last := st.Pick(childIds[len(childIds)-1])
			require.NotNil(t, last)
			assert.Equal(t, tc.pasteBlocks[len(tc.pasteBlocks)-1].GetText().Text, last.Model().GetText().Text,
				"last pasted block text must be preserved")
		})
	}
}

// A paragraph is only disposable when it holds nothing the user put there. Every case here
// has empty text but carries one setting that develop preserves by reusing the block, so
// dropping the block would silently destroy it. Style residue (a code block's language, a
// callout icon, a checkbox's checked state) survives a style change back to Paragraph and
// reappears when the style changes back, so it is not dead data.
func TestPasteIntoConfiguredEmptyParagraph(t *testing.T) {
	type want struct {
		align   model.BlockAlign
		vAlign  model.BlockVerticalAlign
		color   string
		bgColor string
		icon    string
		iconImg string
		lang    string
		checked bool
	}
	for _, tc := range []struct {
		name  string
		apply func(b *model.Block)
		want  want
	}{
		{"center aligned", func(b *model.Block) { b.Align = model.Block_AlignCenter },
			want{align: model.Block_AlignCenter}},
		{"vertical align", func(b *model.Block) { b.VerticalAlign = model.Block_VerticalAlignBottom },
			want{vAlign: model.Block_VerticalAlignBottom}},
		{"text color", func(b *model.Block) { b.GetText().Color = "red" },
			want{color: "red"}},
		{"background color", func(b *model.Block) { b.BackgroundColor = "blue" },
			want{bgColor: "blue"}},
		{"callout icon residue", func(b *model.Block) { b.GetText().IconEmoji = "\U0001f525" },
			want{icon: "\U0001f525"}},
		{"icon image residue", func(b *model.Block) { b.GetText().IconImage = "imagehash" },
			want{iconImg: "imagehash"}},
		{"code language residue", func(b *model.Block) {
			b.Fields = &types.Struct{Fields: map[string]*types.Value{"lang": pbtypes.String("go")}}
		}, want{lang: "go"}},
		{"checked residue", func(b *model.Block) { b.GetText().Checked = true },
			want{checked: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			target := textBlock("target", "", model.BlockContentText_Paragraph)
			tc.apply(target)
			sb := smarttest.New("test")
			sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"target"}}))
			sb.AddBlock(simple.New(target))
			cb := newFixture(t, sb)

			// when
			_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
				FocusedBlockId:    "target",
				SelectedTextRange: &model.Range{From: 0, To: 0},
				AnySlot: []*model.Block{
					textBlock("p1", "first", model.BlockContentText_Paragraph),
					textBlock("p2", "second", model.BlockContentText_Paragraph),
				},
			}, "")

			// then
			require.NoError(t, err)
			st := sb.NewState()
			childIds := st.Pick("test").Model().ChildrenIds
			require.Len(t, childIds, 2)
			first := st.Pick(childIds[0])
			require.NotNil(t, first)
			got := want{
				align:   first.Model().Align,
				vAlign:  first.Model().VerticalAlign,
				color:   first.Model().GetText().Color,
				bgColor: first.Model().BackgroundColor,
				icon:    first.Model().GetText().IconEmoji,
				iconImg: first.Model().GetText().IconImage,
				lang:    pbtypes.GetString(first.Model().Fields, "lang"),
				checked: first.Model().GetText().Checked,
			}
			assert.Equal(t, tc.want, got, "the setting must survive the paste")
			assert.Equal(t, "first", first.Model().GetText().Text, "first pasted line still lands here")
			second := st.Pick(childIds[1])
			require.NotNil(t, second)
			assert.Equal(t, "second", second.Model().GetText().Text, "trailing pasted line must be intact")
		})
	}
}

// Dropping the focused block instead of writing to it must not slip past a restriction that a
// write would have failed, and must not silently discard the restriction itself. Each flag is
// exercised on its own: a fixture that sets several at once passes even if the predicate only
// looks at one of them.
func TestPasteIntoRestrictedEmptyParagraph(t *testing.T) {
	for _, tc := range []struct {
		name       string
		r          *model.BlockRestrictions
		wantReject bool // Edit forbids the write the reuse path performs
	}{
		{"read", &model.BlockRestrictions{Read: true}, false},
		{"edit", &model.BlockRestrictions{Edit: true}, true},
		{"remove", &model.BlockRestrictions{Remove: true}, false},
		{"drag", &model.BlockRestrictions{Drag: true}, false},
		{"dropOn", &model.BlockRestrictions{DropOn: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			target := textBlock("target", "", model.BlockContentText_Paragraph)
			target.Restrictions = tc.r
			sb := smarttest.New("test")
			sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"target"}}))
			sb.AddBlock(simple.New(target))
			cb := newFixture(t, sb)

			// when
			_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
				FocusedBlockId:    "target",
				SelectedTextRange: &model.Range{From: 0, To: 0},
				AnySlot: []*model.Block{
					textBlock("p1", "first", model.BlockContentText_Paragraph),
					textBlock("p2", "second", model.BlockContentText_Paragraph),
				},
			}, "")

			// then: Edit forbids writing to the block, so the paste must be rejected and the
			// document left alone. Every other flag permits the paste, and the block must be
			// reused rather than dropped, so the restriction survives with it. Accepting
			// "any error" here would hide a regression that rejects permitted pastes.
			st := sb.NewState()
			if tc.wantReject {
				require.Error(t, err, "a write-forbidding restriction must reject the paste")
				assert.Equal(t, []string{"target"}, st.Pick("test").Model().ChildrenIds,
					"a rejected paste must leave the document untouched")
				assert.Equal(t, "", st.Pick("target").Model().GetText().Text,
					"a rejected paste must not write into the block")
				return
			}
			require.NoError(t, err, "this restriction does not forbid the paste")
			childIds := st.Pick("test").Model().ChildrenIds
			require.Len(t, childIds, 2, "both pasted lines must land")
			first := st.Pick(childIds[0])
			require.NotNil(t, first)
			assert.Equal(t, "first", first.Model().GetText().Text)
			assert.Equal(t, tc.r, first.Model().Restrictions,
				"the restriction must survive the paste")
		})
	}
}

// An empty paragraph is only dropped when it holds nothing worth keeping. Empty text does
// not imply an empty subtree: unlinking a block orphans its children, and the state apply
// then deletes them, so a paragraph with children is reused the way it always was.
func TestPasteIntoEmptyParagraphWithChildren(t *testing.T) {
	// given
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"target"}}))
	sb.AddBlock(simple.New(&model.Block{
		Id:          "target",
		ChildrenIds: []string{"child"},
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text: "", Style: model.BlockContentText_Paragraph}},
	}))
	sb.AddBlock(simple.New(&model.Block{
		Id:          "child",
		ChildrenIds: []string{"grandchild"},
		Content:     &model.BlockContentOfText{Text: &model.BlockContentText{Text: "existing child"}},
	}))
	sb.AddBlock(simple.New(&model.Block{
		Id:      "grandchild",
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "existing grandchild"}},
	}))
	cb := newFixture(t, sb)

	// when
	_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
		FocusedBlockId:    "target",
		SelectedTextRange: &model.Range{From: 0, To: 0},
		AnySlot: []*model.Block{
			textBlock("p1", "one", model.BlockContentText_Paragraph),
			textBlock("p2", "two", model.BlockContentText_Paragraph),
		},
	}, "")

	// then: the paste itself must be correct, with no stray empty block left behind
	require.NoError(t, err)
	st := sb.NewState()
	childIds := st.Pick("test").Model().ChildrenIds
	require.Len(t, childIds, 2, "exactly the two pasted lines, no stray empty paragraph")

	first, second := st.Pick(childIds[0]), st.Pick(childIds[1])
	require.NotNil(t, first)
	require.NotNil(t, second)
	assert.Equal(t, "one", first.Model().GetText().Text)
	assert.Equal(t, "two", second.Model().GetText().Text)

	// and the existing subtree must still hang off the reused block, with its content intact
	require.Equal(t, []string{"child"}, first.Model().ChildrenIds, "existing child stays under the reused block")
	child := st.Pick("child")
	require.NotNil(t, child, "existing child must survive the paste")
	assert.Equal(t, "existing child", child.Model().GetText().Text)
	require.Equal(t, []string{"grandchild"}, child.Model().ChildrenIds)
	grandchild := st.Pick("grandchild")
	require.NotNil(t, grandchild, "existing grandchild must survive the paste")
	assert.Equal(t, "existing grandchild", grandchild.Model().GetText().Text)
}

// A single-block markdown body goes through intoBlock/RangeTextPaste rather than singleRange,
// so it needs its own guard: a checked task pasted there must stay checked. The API creates an
// empty paragraph and pastes into it, so `{"markdown": "- [x] Done"}` hits exactly this path.
func TestPasteSingleBlockIntoEmptyParagraphKeepsChecked(t *testing.T) {
	for _, tc := range []struct {
		name      string
		markdown  string
		want      []bool   // checked state per resulting block
		wantTexts []string // and its label, so a truncated or blanked task is caught
	}{
		{"single checked task", "- [x] Done\n", []bool{true}, []string{"Done"}},
		{"single unchecked task", "- [ ] Todo\n", []bool{false}, []string{"Todo"}},
		{"checked then unchecked", "- [x] Done\n- [ ] Todo\n", []bool{true, false}, []string{"Done", "Todo"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			sb := smarttest.New("test")
			sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"b"}}))
			sb.AddBlock(simple.New(textBlock("b", "", model.BlockContentText_Paragraph)))
			cb := newFixture(t, sb)

			// when
			_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
				FocusedBlockId: "b",
				TextSlot:       tc.markdown,
			}, "")

			// then
			require.NoError(t, err)
			st := sb.NewState()
			childIds := st.Pick("test").Model().ChildrenIds
			require.Len(t, childIds, len(tc.want))
			got := make([]bool, 0, len(childIds))
			for _, id := range childIds {
				b := st.Pick(id)
				require.NotNil(t, b)
				assert.Equal(t, model.BlockContentText_Checkbox, b.Model().GetText().Style)
				assert.Equal(t, tc.wantTexts[len(got)], b.Model().GetText().Text,
					"the task label must survive the paste")
				got = append(got, b.Model().GetText().Checked)
			}
			assert.Equal(t, tc.want, got, "checked state must survive the paste")
		})
	}
}

// Replacing the label of a completed task must not reopen it. GO-250 makes a plain pasted
// paragraph inherit the focused block's style, so an HTML paste into a checkbox arrives as a
// checkbox — and RangeTextPaste adopts the pasted block's checked state along with its style.
// The synthesized block therefore has to borrow the target's checked state too, or the adoption
// silently clears it. A real checkbox carrying its own state is a different case and is adopted.
func TestPasteOverCheckedCheckbox(t *testing.T) {
	for _, tc := range []struct {
		name        string
		req         func(r *pb.RpcBlockPasteRequest)
		wantStyle   model.BlockContentTextStyle
		wantChecked bool
	}{
		{
			name:        "plain html keeps the task completed",
			req:         func(r *pb.RpcBlockPasteRequest) { r.HtmlSlot = "<p>New</p>" },
			wantStyle:   model.BlockContentText_Checkbox,
			wantChecked: true,
		},
		{
			// replacing the label never moves the state, whatever was pasted over it
			name: "an unchecked checkbox pasted over it leaves it completed",
			req: func(r *pb.RpcBlockPasteRequest) {
				r.AnySlot = []*model.Block{textBlock("p1", "New", model.BlockContentText_Checkbox)}
			},
			wantStyle:   model.BlockContentText_Checkbox,
			wantChecked: true,
		},
		{
			// checked is meaningful only for a checkbox, so adopting some other style
			// leaves it untouched: the block keeps its state while it is a paragraph and
			// gets it back if it is turned into a checkbox again.
			name:        "plain text turns it into a paragraph without touching the state",
			req:         func(r *pb.RpcBlockPasteRequest) { r.TextSlot = "New" },
			wantStyle:   model.BlockContentText_Paragraph,
			wantChecked: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			done := textBlock("t", "Done", model.BlockContentText_Checkbox)
			done.GetText().Checked = true
			sb := smarttest.New("test")
			sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"t"}}))
			sb.AddBlock(simple.New(done))
			cb := newFixture(t, sb)

			req := &pb.RpcBlockPasteRequest{
				FocusedBlockId:    "t",
				SelectedTextRange: &model.Range{From: 0, To: 4}, // replaces the whole label
			}
			tc.req(req)

			// when
			_, _, _, _, err := cb.Paste(nil, req, "")

			// then
			require.NoError(t, err)
			st := sb.NewState()
			childIds := st.Pick("test").Model().ChildrenIds
			require.Len(t, childIds, 1)
			got := st.Pick(childIds[0])
			require.NotNil(t, got)
			assert.Equal(t, "New", got.Model().GetText().Text)
			assert.Equal(t, tc.wantStyle, got.Model().GetText().Style)
			assert.Equal(t, tc.wantChecked, got.Model().GetText().Checked, "checked state")
		})
	}
}

// A paragraph carrying checked residue — a checked checkbox someone turned into a paragraph —
// must survive a paste with its state intact, and single-block and multi-block paste must agree.
// They are fixed by different mechanisms in different packages, so nothing else pins that.
func TestPasteKeepsCheckedResidue(t *testing.T) {
	for _, tc := range []struct{ name, markdown string }{
		{"single block", "New text"},
		{"multi block", "New text\n\nAfter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			residue := textBlock("t", "", model.BlockContentText_Paragraph)
			residue.GetText().Checked = true
			sb := smarttest.New("test")
			sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"t"}}))
			sb.AddBlock(simple.New(residue))
			cb := newFixture(t, sb)

			// when
			_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
				FocusedBlockId: "t", TextSlot: tc.markdown,
			}, "")

			// then
			require.NoError(t, err)
			st := sb.NewState()
			first := st.Pick(st.Pick("test").Model().ChildrenIds[0])
			require.NotNil(t, first)
			assert.Equal(t, "New text", first.Model().GetText().Text)
			assert.True(t, first.Model().GetText().Checked, "checked residue must survive the paste")
		})
	}
}

// Lines pasted under a completed task become new tasks, and new tasks are not born complete.
// GO-250 gives them the focused block's style; the state that goes with it is not theirs.
func TestPasteHtmlUnderCheckedCheckbox(t *testing.T) {
	// given
	done := textBlock("t", "Done", model.BlockContentText_Checkbox)
	done.GetText().Checked = true
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"t"}}))
	sb.AddBlock(simple.New(done))
	cb := newFixture(t, sb)

	// when: caret at the end of the label, nothing replaced
	_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
		FocusedBlockId:    "t",
		SelectedTextRange: &model.Range{From: 4, To: 4},
		HtmlSlot:          "<p>new task one</p><p>new task two</p>",
	}, "")

	// then
	require.NoError(t, err)
	st := sb.NewState()
	childIds := st.Pick("test").Model().ChildrenIds
	require.Len(t, childIds, 3)

	first := st.Pick(childIds[0])
	require.NotNil(t, first)
	assert.True(t, first.Model().GetText().Checked, "the original task stays completed")

	for i, id := range childIds[1:] {
		b := st.Pick(id)
		require.NotNil(t, b)
		assert.Equal(t, model.BlockContentText_Checkbox, b.Model().GetText().Style,
			"GO-250: pasted lines inherit the focused block's style")
		assert.Equal(t, []string{"new task one", "new task two"}[i], b.Model().GetText().Text,
			"the pasted label must survive, in order")
		assert.False(t, b.Model().GetText().Checked,
			"a new task must not be born completed: %q", b.Model().GetText().Text)
	}
}

// withBold attaches a bold mark, so a test that would otherwise ignore marks notices when
// they are dropped on the way into the reused block.
func withBold(b *model.Block, from, to int32) *model.Block {
	b.GetText().Marks = &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
		{Range: &model.Range{From: from, To: to}, Type: model.BlockContentTextMark_Bold},
		// a link keeps its destination in Param: a bold-only fixture cannot show that a
		// mark's payload survives, only that a mark of some kind does
		{Range: &model.Range{From: from, To: to}, Type: model.BlockContentTextMark_Link,
			Param: "https://example.com/kept"},
	}}
	return b
}

// textBlock builds a styled text block for the paste slot.
func textBlock(id, txt string, style model.BlockContentTextStyle) *model.Block {
	return &model.Block{
		Id: id,
		Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:  txt,
			Style: style,
			Marks: &model.BlockContentTextMarks{},
		}},
	}
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

func Test_CopyAndCutMultiBlockRange(t *testing.T) {
	givenSbWithThreeTextBlocks := func(t *testing.T) (*smarttest.SmartTest, []*model.Block) {
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithEmpty))
		s := sb.NewState()
		blocks := []*model.Block{
			givenTextBlockWithMarks("1", "first block", nil),
			givenTextBlockWithMarks("2", "middle block", nil),
			givenTextBlockWithMarks("3", "last block", nil),
		}
		for _, b := range blocks {
			insertBlock(s, b, "")
		}
		require.NoError(t, sb.Apply(s))
		return sb, blocks
	}

	t.Run("copy - partial first and last block", func(t *testing.T) {
		// given
		sb, blocks := givenSbWithThreeTextBlocks(t)
		cb := newFixture(t, sb)

		// when
		textSlot, htmlSlot, anySlot, err := cb.Copy(nil, pb.RpcBlockCopyRequest{
			Blocks:                     blocks,
			SelectedTextRange:          &model.Range{From: 6, To: 11},
			SelectedTextRangeLastBlock: &model.Range{From: 0, To: 4},
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, "block\nmiddle block\nlast", textSlot)
		require.Len(t, anySlot, 3)
		assert.Equal(t, "block", anySlot[0].GetText().Text)
		assert.Equal(t, "middle block", anySlot[1].GetText().Text)
		assert.Equal(t, "last", anySlot[2].GetText().Text)
		assert.NotEmpty(t, htmlSlot)

		// document is not modified
		assert.Equal(t, "first block", sb.Pick("1").Model().GetText().Text)
		assert.Equal(t, "middle block", sb.Pick("2").Model().GetText().Text)
		assert.Equal(t, "last block", sb.Pick("3").Model().GetText().Text)
	})

	t.Run("copy - marks are shifted to the range start", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithEmpty))
		s := sb.NewState()
		blocks := []*model.Block{
			givenTextBlockWithMarks("1", "first block", []*model.BlockContentTextMark{
				{Range: &model.Range{From: 6, To: 11}, Type: model.BlockContentTextMark_Bold},
			}),
			givenTextBlockWithMarks("2", "last block", []*model.BlockContentTextMark{
				{Range: &model.Range{From: 0, To: 4}, Type: model.BlockContentTextMark_Italic},
			}),
		}
		for _, b := range blocks {
			insertBlock(s, b, "")
		}
		require.NoError(t, sb.Apply(s))
		cb := newFixture(t, sb)
		want := []*model.BlockContentTextMark{
			{Range: &model.Range{From: 0, To: 5}, Type: model.BlockContentTextMark_Bold},
		}

		// when
		_, _, anySlot, err := cb.Copy(nil, pb.RpcBlockCopyRequest{
			Blocks:                     blocks,
			SelectedTextRange:          &model.Range{From: 6, To: 11},
			SelectedTextRangeLastBlock: &model.Range{From: 0, To: 4},
		})

		// then
		require.NoError(t, err)
		require.Len(t, anySlot, 2)
		assert.Equal(t, want, anySlot[0].GetText().Marks.Marks)
		assert.Equal(t, []*model.BlockContentTextMark{
			{Range: &model.Range{From: 0, To: 4}, Type: model.BlockContentTextMark_Italic},
		}, anySlot[1].GetText().Marks.Marks)
	})

	t.Run("copy - zero ranges mean whole blocks", func(t *testing.T) {
		// given
		sb, blocks := givenSbWithThreeTextBlocks(t)
		cb := newFixture(t, sb)

		// when
		textSlot, _, anySlot, err := cb.Copy(nil, pb.RpcBlockCopyRequest{
			Blocks:                     blocks,
			SelectedTextRange:          &model.Range{},
			SelectedTextRangeLastBlock: &model.Range{},
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, "first block\nmiddle block\nlast block", textSlot)
		require.Len(t, anySlot, 3)
		assert.Equal(t, "first block", anySlot[0].GetText().Text)
		assert.Equal(t, "last block", anySlot[2].GetText().Text)
	})

	t.Run("copy - last block range only", func(t *testing.T) {
		// given
		sb, blocks := givenSbWithThreeTextBlocks(t)
		cb := newFixture(t, sb)

		// when
		textSlot, _, _, err := cb.Copy(nil, pb.RpcBlockCopyRequest{
			Blocks:                     blocks,
			SelectedTextRangeLastBlock: &model.Range{From: 0, To: 4},
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, "first block\nmiddle block\nlast", textSlot)
	})

	t.Run("cut - partial first and last block", func(t *testing.T) {
		// given
		sb, blocks := givenSbWithThreeTextBlocks(t)
		cb := newFixture(t, sb)

		// when
		textSlot, htmlSlot, anySlot, err := cb.Cut(nil, pb.RpcBlockCutRequest{
			Blocks:                     blocks,
			SelectedTextRange:          &model.Range{From: 6, To: 11},
			SelectedTextRangeLastBlock: &model.Range{From: 0, To: 5},
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, "block\nmiddle block\nlast ", textSlot)
		require.Len(t, anySlot, 3)
		assert.Equal(t, "block", anySlot[0].GetText().Text)
		assert.Equal(t, "middle block", anySlot[1].GetText().Text)
		assert.Equal(t, "last ", anySlot[2].GetText().Text)
		assert.NotEmpty(t, htmlSlot)

		// partially selected blocks keep the rest of their text, the middle block is removed
		assert.Equal(t, "first ", sb.Pick("1").Model().GetText().Text)
		assert.Nil(t, sb.Pick("2"))
		assert.Equal(t, "block", sb.Pick("3").Model().GetText().Text)
	})

	t.Run("cut - zero ranges cut whole blocks", func(t *testing.T) {
		// given
		sb, blocks := givenSbWithThreeTextBlocks(t)
		cb := newFixture(t, sb)

		// when
		textSlot, _, anySlot, err := cb.Cut(nil, pb.RpcBlockCutRequest{
			Blocks:                     blocks,
			SelectedTextRange:          &model.Range{},
			SelectedTextRangeLastBlock: &model.Range{},
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, "first block\nmiddle block\nlast block", textSlot)
		require.Len(t, anySlot, 3)
		assert.Nil(t, sb.Pick("1"))
		assert.Nil(t, sb.Pick("2"))
		assert.Nil(t, sb.Pick("3"))
	})

	t.Run("cut - backward compatibility: no last block range cuts whole blocks", func(t *testing.T) {
		// given
		sb, blocks := givenSbWithThreeTextBlocks(t)
		cb := newFixture(t, sb)

		// when
		textSlot, _, _, err := cb.Cut(nil, pb.RpcBlockCutRequest{
			Blocks:            blocks,
			SelectedTextRange: &model.Range{From: 6, To: 11},
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, "first block\nmiddle block\nlast block", textSlot)
		assert.Nil(t, sb.Pick("1"))
		assert.Nil(t, sb.Pick("2"))
		assert.Nil(t, sb.Pick("3"))
	})

	t.Run("cut - single block ignores last block range", func(t *testing.T) {
		// given
		sb, blocks := givenSbWithThreeTextBlocks(t)
		cb := newFixture(t, sb)

		// when
		textSlot, _, _, err := cb.Cut(nil, pb.RpcBlockCutRequest{
			Blocks:                     blocks[:1],
			SelectedTextRange:          &model.Range{From: 0, To: 5},
			SelectedTextRangeLastBlock: &model.Range{From: 0, To: 4},
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, "first", textSlot)
		assert.Equal(t, " block", sb.Pick("1").Model().GetText().Text)
	})

	t.Run("cut - non-text first block is cut whole", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		require.NoError(t, smartblock.ObjectApplyTemplate(sb, nil, template.WithEmpty))
		s := sb.NewState()
		fileBlock := &model.Block{
			Id:      "1",
			Content: &model.BlockContentOfFile{File: &model.BlockContentFile{Name: "image"}},
		}
		insertBlock(s, fileBlock, "")
		textBlock := givenTextBlockWithMarks("2", "last block", nil)
		insertBlock(s, textBlock, "")
		require.NoError(t, sb.Apply(s))
		cb := newFixture(t, sb)

		// when
		textSlot, _, anySlot, err := cb.Cut(nil, pb.RpcBlockCutRequest{
			Blocks:                     []*model.Block{fileBlock, textBlock},
			SelectedTextRange:          &model.Range{},
			SelectedTextRangeLastBlock: &model.Range{From: 0, To: 4},
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, "last", textSlot)
		require.Len(t, anySlot, 2)
		assert.Nil(t, sb.Pick("1"))
		assert.Equal(t, " block", sb.Pick("2").Model().GetText().Text)
	})
}

func givenTextBlockWithMarks(id string, text string, marks []*model.BlockContentTextMark) *model.Block {
	return &model.Block{
		Id: id,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  text,
				Marks: &model.BlockContentTextMarks{Marks: marks},
			},
		},
	}
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
		{"toggle_header1", model.BlockContentText_ToggleHeader1, "some text 1", ""},
		{"toggle_header2", model.BlockContentText_ToggleHeader2, "some text 1", ""},
		{"toggle_header3", model.BlockContentText_ToggleHeader3, "some text 1", ""},
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
		{"toggle_header1", model.BlockContentText_ToggleHeader1, "\t## some text 1", ""},
		{"toggle_header2", model.BlockContentText_ToggleHeader2, "\t### some text 1", ""},
		{"toggle_header3", model.BlockContentText_ToggleHeader3, "\t#### some text 1", ""},
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
		file.EXPECT().CreateFromImport(domain.FullFileId{FileId: fileId, SpaceId: space1}, mock.Anything, mock.Anything).Return(fileObject2, nil)

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
		file.EXPECT().CreateFromImport(domain.FullFileId{FileId: fileId, SpaceId: space1}, mock.Anything, mock.Anything).Return("", fmt.Errorf("some error"))

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

	t.Run("empty target object id is skipped without calling GetFileIdFromObject", func(t *testing.T) {
		// given
		file := mock_fileobject.NewMockService(t)
		// No expectations set — any call to GetFileIdFromObject would fail the test

		c := &clipboard{
			SmartBlock:        sb,
			fileObjectService: file,
		}

		fb := &model.BlockContentOfFile{File: &model.BlockContentFile{TargetObjectId: ""}}

		// when
		c.processFileBlock(fb)

		// then
		assert.Equal(t, "", fb.File.TargetObjectId)
	})

	t.Run("nil file content is skipped", func(t *testing.T) {
		// given
		file := mock_fileobject.NewMockService(t)

		c := &clipboard{
			SmartBlock:        sb,
			fileObjectService: file,
		}

		fb := &model.BlockContentOfFile{File: nil}

		// when
		c.processFileBlock(fb)

		// then
		assert.Nil(t, fb.File)
	})
}

func TestPasteEmptyFileBlock(t *testing.T) {
	t.Run("empty file placeholder pastes without error", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{"text1"}, emptyMarks))
		cb := newFixture(t, sb)

		req := &pb.RpcBlockPasteRequest{
			FocusedBlockId:    "1",
			SelectedTextRange: &model.Range{From: 5, To: 5},
			AnySlot: []*model.Block{
				{
					Id: "fileBlock1",
					Content: &model.BlockContentOfFile{
						File: &model.BlockContentFile{
							State: model.BlockContentFile_Empty,
							Type:  model.BlockContentFile_File,
						},
					},
				},
			},
		}

		// when
		blockIds, uploadArr, _, _, err := cb.Paste(nil, req, "")

		// then
		require.NoError(t, err)
		assert.NotEmpty(t, blockIds)
		assert.Empty(t, uploadArr)
	})

	t.Run("empty file placeholder generates no upload requests", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
		cb := newFixture(t, sb)

		req := &pb.RpcBlockPasteRequest{
			FocusedBlockId:    "",
			SelectedTextRange: &model.Range{},
			AnySlot: []*model.Block{
				{
					Id: "fileBlock1",
					Content: &model.BlockContentOfFile{
						File: &model.BlockContentFile{
							State: model.BlockContentFile_Empty,
							Type:  model.BlockContentFile_Image,
						},
					},
				},
			},
		}

		// when
		_, uploadArr, _, _, err := cb.Paste(nil, req, "")

		// then
		require.NoError(t, err)
		assert.Empty(t, uploadArr)
	})

	t.Run("file block with URL in Name still generates upload request", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
		cb := newFixture(t, sb)

		req := &pb.RpcBlockPasteRequest{
			FocusedBlockId:    "",
			SelectedTextRange: &model.Range{},
			AnySlot: []*model.Block{
				{
					Id: "fileBlock1",
					Content: &model.BlockContentOfFile{
						File: &model.BlockContentFile{
							State: model.BlockContentFile_Empty,
							Type:  model.BlockContentFile_Image,
							Name:  "https://example.com/image.png",
						},
					},
				},
			},
		}

		// when
		_, uploadArr, _, _, err := cb.Paste(nil, req, "")

		// then
		require.NoError(t, err)
		require.Len(t, uploadArr, 1)
		assert.Equal(t, "https://example.com/image.png", uploadArr[0].Url)
	})

	t.Run("mixed text and empty file blocks all paste correctly", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
		cb := newFixture(t, sb)

		req := &pb.RpcBlockPasteRequest{
			FocusedBlockId:    "",
			SelectedTextRange: &model.Range{},
			AnySlot: []*model.Block{
				{
					Id: "textBlock1",
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{Text: "hello"},
					},
				},
				{
					Id: "fileBlock1",
					Content: &model.BlockContentOfFile{
						File: &model.BlockContentFile{
							State: model.BlockContentFile_Empty,
							Type:  model.BlockContentFile_File,
						},
					},
				},
				{
					Id: "textBlock2",
					Content: &model.BlockContentOfText{
						Text: &model.BlockContentText{Text: "world"},
					},
				},
			},
		}

		// when
		blockIds, uploadArr, _, _, err := cb.Paste(nil, req, "")

		// then
		require.NoError(t, err)
		assert.Len(t, blockIds, 3)
		assert.Empty(t, uploadArr)
	})
}

func TestClipboard_PasteIntoEmptyBlockForksId(t *testing.T) {
	newTextModel := func(text string) *model.Block {
		return &model.Block{Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: text}}}
	}
	newPage := func(t *testing.T) *smarttest.SmartTest {
		return createPage(t, []*model.Block{
			{Id: "a", Content: &model.BlockContentOfText{Text: &model.BlockContentText{Text: "existing"}}},
			{Id: "empty", Content: &model.BlockContentOfText{Text: &model.BlockContentText{}}},
		})
	}

	t.Run("single text block into empty block gets a fresh id", func(t *testing.T) {
		// given
		sb := newPage(t)
		cb := newFixture(t, sb)

		// when
		blockIds, _, caret, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			FocusedBlockId:    "empty",
			SelectedTextRange: &model.Range{},
			AnySlot:           []*model.Block{newTextModel("pasted")},
		}, "")

		// then
		require.NoError(t, err)
		assert.Nil(t, sb.Pick("empty"), "the peer-known empty block id must be replaced")
		require.Len(t, blockIds, 1)
		forked := sb.Pick(blockIds[0])
		require.NotNil(t, forked)
		assert.Equal(t, "pasted", forked.Model().GetText().Text)
		assert.EqualValues(t, -1, caret, "caret must be reported via blockIds, not a position in the replaced block")
		checkBlockText(t, sb, []string{"existing", "pasted"})
	})

	t.Run("multi block paste into empty block forks the first line", func(t *testing.T) {
		// given
		sb := newPage(t)
		cb := newFixture(t, sb)

		// when
		blockIds, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			FocusedBlockId:    "empty",
			SelectedTextRange: &model.Range{},
			AnySlot:           []*model.Block{newTextModel("first"), newTextModel("second")},
		}, "")

		// then
		require.NoError(t, err)
		assert.Nil(t, sb.Pick("empty"))
		require.Len(t, blockIds, 2)
		assert.Equal(t, "first", sb.Pick(blockIds[0]).Model().GetText().Text)
		checkBlockText(t, sb, []string{"existing", "first", "second"})
	})

	t.Run("paste into non-empty block keeps its id", func(t *testing.T) {
		// given
		sb := newPage(t)
		cb := newFixture(t, sb)

		// when: caret at the end of "existing"
		_, _, caret, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			FocusedBlockId:    "a",
			SelectedTextRange: &model.Range{From: 8, To: 8},
			AnySlot:           []*model.Block{newTextModel("-tail")},
		}, "")

		// then
		require.NoError(t, err)
		require.NotNil(t, sb.Pick("a"))
		assert.Equal(t, "existing-tail", sb.Pick("a").Model().GetText().Text)
		assert.EqualValues(t, 13, caret)
	})

	t.Run("empty title keeps its id", func(t *testing.T) {
		// given
		sb := smarttest.New("text")
		s := sb.NewState()
		template.InitTemplate(s, template.WithTitle)
		_, _, err := state.ApplyState("", s, false)
		require.NoError(t, err)
		cb := newFixture(t, sb)

		// when
		_, _, _, _, err = cb.Paste(nil, &pb.RpcBlockPasteRequest{
			FocusedBlockId:    template.TitleBlockId,
			SelectedTextRange: &model.Range{},
			AnySlot:           []*model.Block{newTextModel("t")},
		}, "")

		// then
		require.NoError(t, err)
		require.NotNil(t, sb.Pick(template.TitleBlockId))
		assert.Equal(t, "t", sb.Pick(template.TitleBlockId).Model().GetText().Text)
	})
}

// renderTree draws the live document one line per block, indented by nesting depth and
// prefixed with the block's style. A child that vanished, lost its parent or changed places
// is then as visible in the diff as one whose text was corrupted.
func renderTree(t *testing.T, sb *smarttest.SmartTest) []string {
	t.Helper()
	st := sb.NewState()
	var out []string
	var walk func(ids []string, depth int)
	walk = func(ids []string, depth int) {
		for _, id := range ids {
			b := st.Pick(id)
			require.NotNil(t, b, "block %s is referenced but not in the document", id)
			out = append(out, fmt.Sprintf("%s%s %s", strings.Repeat("  ", depth),
				b.Model().GetText().GetStyle(), b.Model().GetText().GetText()))
			walk(b.Model().ChildrenIds, depth+1)
		}
	}
	walk(st.Pick(st.RootId()).Model().ChildrenIds, 0)
	return out
}

// liveIds lists the document's blocks in the order a reader meets them, which is the order
// paste reports them in.
func liveIds(t *testing.T, sb *smarttest.SmartTest) []string {
	t.Helper()
	st := sb.NewState()
	var out []string
	var walk func(ids []string)
	walk = func(ids []string) {
		for _, id := range ids {
			out = append(out, id)
			if b := st.Pick(id); b != nil {
				walk(b.Model().ChildrenIds)
			}
		}
	}
	walk(st.Pick(st.RootId()).Model().ChildrenIds)
	return out
}

// A multi-block paste into an empty block the user styled reuses that block for the first
// pasted line and drops the pasted block it stood in for. State.Unlink only detaches an id
// from its parent, so the dropped block's subtree stayed in the paste state unreachable from
// its root, and insertUnderSelection — which walks the tree — never moved it into the
// document: pasting a toggle with anything under it lost everything under it.
//
// The subtree is re-parented onto the reused block when its style can own children, and
// follows it as siblings when it cannot. GO-7513 keeps the target's own children alive, so
// both sides can be populated at once: the pasted subtree goes first, where the caret was.
func TestPasteNestedContentIntoEmptyStyledBlock(t *testing.T) {
	nestedPaste := func() []*model.Block {
		head := textBlock("p1", "head", model.BlockContentText_Toggle)
		head.ChildrenIds = []string{"c1"}
		child := textBlock("c1", "nested child", model.BlockContentText_Paragraph)
		child.ChildrenIds = []string{"g1"}
		return []*model.Block{
			head, child,
			textBlock("g1", "nested grandchild", model.BlockContentText_Paragraph),
			textBlock("p2", "body", model.BlockContentText_Paragraph),
		}
	}
	type want struct {
		tree []string
		// the reused block's own Fields must come through the re-parenting and the fork
		// untouched: it is the block the user configured, not the one that was pasted
		firstLang string
	}
	for _, tc := range []struct {
		name        string
		targetStyle model.BlockContentTextStyle
		targetKids  []string // text of the children the target already had
		targetLang  string   // a code language the target was left carrying
		pasteBlocks []*model.Block
		want        want
	}{
		{
			name:        "a style that can own children adopts the pasted subtree",
			targetStyle: model.BlockContentText_Toggle,
			pasteBlocks: nestedPaste(),
			want: want{tree: []string{
				"Toggle head",
				"  Paragraph nested child",
				"    Paragraph nested grandchild",
				"Paragraph body",
			}},
		},
		{
			name:        "a callout adopts it too",
			targetStyle: model.BlockContentText_Callout,
			pasteBlocks: nestedPaste(),
			want: want{tree: []string{
				"Callout head",
				"  Paragraph nested child",
				"    Paragraph nested grandchild",
				"Paragraph body",
			}},
		},
		{
			// a header cannot own children, so the subtree keeps the place
			// insertUnderSelection gave it rather than being forced under a block that
			// would not render it
			name:        "a style that cannot own children is followed by the subtree",
			targetStyle: model.BlockContentText_Header1,
			pasteBlocks: nestedPaste(),
			want: want{tree: []string{
				"Header1 head",
				"Paragraph nested child",
				"  Paragraph nested grandchild",
				"Paragraph body",
			}},
		},
		{
			// the caret sat in the block's text, above everything nested under it, so
			// what was pasted at the caret belongs in front of what was already there
			name:        "the pasted subtree goes in front of the block's own children",
			targetStyle: model.BlockContentText_Toggle,
			targetKids:  []string{"own child"},
			pasteBlocks: nestedPaste(),
			want: want{tree: []string{
				"Toggle head",
				"  Paragraph nested child",
				"    Paragraph nested grandchild",
				"  Paragraph own child",
				"Paragraph body",
			}},
		},
		{
			name:        "a block that cannot own children keeps its own and is followed by the subtree",
			targetStyle: model.BlockContentText_Header1,
			targetKids:  []string{"own child"},
			pasteBlocks: nestedPaste(),
			want: want{tree: []string{
				"Header1 head",
				"  Paragraph own child",
				"Paragraph nested child",
				"  Paragraph nested grandchild",
				"Paragraph body",
			}},
		},
		{
			name:        "several children keep their order",
			targetStyle: model.BlockContentText_Toggle,
			pasteBlocks: func() []*model.Block {
				head := textBlock("p1", "head", model.BlockContentText_Toggle)
				head.ChildrenIds = []string{"c1", "c2", "c3"}
				return []*model.Block{
					head,
					textBlock("c1", "one", model.BlockContentText_Paragraph),
					textBlock("c2", "two", model.BlockContentText_Paragraph),
					textBlock("c3", "three", model.BlockContentText_Paragraph),
					textBlock("p2", "body", model.BlockContentText_Paragraph),
				}
			}(),
			want: want{tree: []string{
				"Toggle head",
				"  Paragraph one",
				"  Paragraph two",
				"  Paragraph three",
				"Paragraph body",
			}},
		},
		{
			// the target keeps the fields the user left on it. The pasted head is dropped
			// in favour of this block, so nothing of the pasted block's own may be
			// written onto it on the way past.
			name:        "the reused block keeps its own fields through the adoption",
			targetStyle: model.BlockContentText_Toggle,
			targetLang:  "go",
			pasteBlocks: nestedPaste(),
			want: want{tree: []string{
				"Toggle head",
				"  Paragraph nested child",
				"    Paragraph nested grandchild",
				"Paragraph body",
			}, firstLang: "go"},
		},
		{
			// the same, with the pasted head carrying a conflicting language of its own
			name:        "a conflicting language on the pasted head does not reach the reused block",
			targetStyle: model.BlockContentText_Toggle,
			targetLang:  "go",
			pasteBlocks: func() []*model.Block {
				blocks := nestedPaste()
				blocks[0].Fields = &types.Struct{Fields: map[string]*types.Value{
					"lang": pbtypes.String("rust"),
				}}
				return blocks
			}(),
			want: want{tree: []string{
				"Toggle head",
				"  Paragraph nested child",
				"    Paragraph nested grandchild",
				"Paragraph body",
			}, firstLang: "go"},
		},
		{
			// and where the subtree is not adopted, the sibling path must leave the
			// block's fields alone too
			name:        "the fields survive on the sibling path as well",
			targetStyle: model.BlockContentText_Header1,
			targetLang:  "go",
			pasteBlocks: nestedPaste(),
			want: want{tree: []string{
				"Header1 head",
				"Paragraph nested child",
				"  Paragraph nested grandchild",
				"Paragraph body",
			}, firstLang: "go"},
		},
		{
			// the block the reuse drops carries no children of its own here, so nothing
			// should move: the plain case must keep behaving exactly as GO-7513 left it
			name:        "a paste with no nesting is unaffected",
			targetStyle: model.BlockContentText_Toggle,
			pasteBlocks: []*model.Block{
				textBlock("p1", "head", model.BlockContentText_Header1),
				textBlock("p2", "body", model.BlockContentText_Paragraph),
			},
			want: want{tree: []string{
				"Toggle head",
				"Paragraph body",
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			target := textBlock("target", "", tc.targetStyle)
			if tc.targetLang != "" {
				target.Fields = &types.Struct{Fields: map[string]*types.Value{
					"lang": pbtypes.String(tc.targetLang),
				}}
			}
			sb := smarttest.New("test")
			for i, txt := range tc.targetKids {
				id := fmt.Sprintf("own%d", i)
				target.ChildrenIds = append(target.ChildrenIds, id)
				sb.AddBlock(simple.New(textBlock(id, txt, model.BlockContentText_Paragraph)))
			}
			sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"target"}}))
			sb.AddBlock(simple.New(target))
			cb := newFixture(t, sb)

			// when
			blockIds, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
				FocusedBlockId:    "target",
				SelectedTextRange: &model.Range{From: 0, To: 0},
				AnySlot:           tc.pasteBlocks,
			}, "")

			// then
			require.NoError(t, err)
			st := sb.NewState()
			first := st.Pick(st.Pick(st.RootId()).Model().ChildrenIds[0])
			require.NotNil(t, first)
			got := want{
				tree:      renderTree(t, sb),
				firstLang: pbtypes.GetString(first.Model().Fields, "lang"),
			}
			assert.Equal(t, tc.want, got)
			// Every pasted block must be reported, nested ones included, each once and in
			// the order a reader meets it: a tree alone passes when the caller is told
			// nothing. The count pins that none went missing — one live block per pasted
			// block, the reused target standing in for the first — and comparing against
			// the live document filtered to the reported ids pins their order and that
			// every one of them is really there.
			live := liveIds(t, sb)
			require.Len(t, blockIds, len(tc.pasteBlocks), "one reported id per pasted block")
			reported := lo.Filter(live, func(id string, _ int) bool {
				return lo.Contains(blockIds, id)
			})
			assert.Equal(t, reported, blockIds,
				"returned ids must be live, unique and in traversal order")
			assert.NotContains(t, live, "target",
				"the original block id must be unlinked after the fork")
		})
	}
}

// A body of exactly one block goes through intoBlock and RangeTextPaste, a longer one through
// singleRange, and the two disagreed about what a pasted block is: the short route carried
// the style but not the callout's icon nor the code block's language, so the same clipboard
// arrived stripped or intact depending on whether a trailing line happened to follow it.
// Every case here is pasted both ways and must come out the same.
func TestPasteSingleBlockKeepsStyleProperties(t *testing.T) {
	type want struct {
		style model.BlockContentTextStyle
		text  string
		icon  string
		lang  string
	}
	for _, tc := range []struct {
		name  string
		block *model.Block
		want  want
	}{
		{
			name: "a fenced code block keeps its language",
			block: func() *model.Block {
				b := textBlock("p1", "fmt.Println(1)", model.BlockContentText_Code)
				b.Fields = &types.Struct{Fields: map[string]*types.Value{
					"lang": pbtypes.String("go"),
				}}
				return b
			}(),
			want: want{style: model.BlockContentText_Code, text: "fmt.Println(1)", lang: "go"},
		},
		{
			name: "a callout keeps its icon",
			block: func() *model.Block {
				b := textBlock("p1", "note", model.BlockContentText_Callout)
				b.GetText().IconEmoji = "\U0001f4a1"
				return b
			}(),
			want: want{style: model.BlockContentText_Callout, text: "note", icon: "\U0001f4a1"},
		},
	} {
		for _, route := range []struct {
			name    string
			trailer []*model.Block
		}{
			{name: "pasted alone", trailer: nil},
			{name: "pasted with a trailing paragraph", trailer: []*model.Block{
				textBlock("p2", "after", model.BlockContentText_Paragraph),
			}},
		} {
			t.Run(tc.name+", "+route.name, func(t *testing.T) {
				// given
				sb := smarttest.New("test")
				sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"target"}}))
				sb.AddBlock(simple.New(textBlock("target", "", model.BlockContentText_Paragraph)))
				cb := newFixture(t, sb)
				blocks := append([]*model.Block{pbtypes.CopyBlock(tc.block)}, route.trailer...)

				// when
				_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
					FocusedBlockId:    "target",
					SelectedTextRange: &model.Range{From: 0, To: 0},
					AnySlot:           blocks,
				}, "")

				// then
				require.NoError(t, err)
				st := sb.NewState()
				childIds := st.Pick("test").Model().ChildrenIds
				require.Len(t, childIds, len(blocks), "one live block per pasted block")
				first := st.Pick(childIds[0])
				require.NotNil(t, first)
				got := want{
					style: first.Model().GetText().Style,
					text:  first.Model().GetText().Text,
					icon:  first.Model().GetText().IconEmoji,
					lang:  pbtypes.GetString(first.Model().Fields, "lang"),
				}
				assert.Equal(t, tc.want, got)
			})
		}
	}
}

// caretPosition addresses a position inside the block the request focused. singleRange never
// set it, so it kept its zero value and Paste reported 0 for a block it had just unlinked —
// either dropped outright or forked to a fresh id. Android prefers any caretPosition >= 0 over
// blockIds, so it pinned the caret to an id that no longer exists; -1 sends it to blockIds,
// which is how intoBlock already reports the same situation.
func TestPasteCaretPositionWhenFocusedBlockIsReplaced(t *testing.T) {
	type want struct {
		caret int32
		// whether the focused block is still in the document; -1 is only correct because
		// it is not, so asserting the number alone would not say why
		focusedSurvives bool
	}
	for _, tc := range []struct {
		name        string
		targetStyle model.BlockContentTextStyle
		targetText  string
		rng         model.Range
		pasteBlocks []*model.Block
		want        want
	}{
		{
			name:        "several blocks into an empty styled block, which is reused and forked",
			targetStyle: model.BlockContentText_Toggle,
			rng:         model.Range{From: 0, To: 0},
			pasteBlocks: []*model.Block{
				textBlock("p1", "first", model.BlockContentText_Paragraph),
				textBlock("p2", "second", model.BlockContentText_Paragraph),
			},
			want: want{caret: -1},
		},
		{
			name:        "several blocks into an empty paragraph, which is dropped",
			targetStyle: model.BlockContentText_Paragraph,
			rng:         model.Range{From: 0, To: 0},
			pasteBlocks: []*model.Block{
				textBlock("p1", "first", model.BlockContentText_Paragraph),
				textBlock("p2", "second", model.BlockContentText_Paragraph),
			},
			want: want{caret: -1},
		},
		{
			name:        "several blocks at the start of a block with text, which is emptied and dropped",
			targetStyle: model.BlockContentText_Paragraph,
			targetText:  "existing",
			rng:         model.Range{From: 0, To: 8},
			pasteBlocks: []*model.Block{
				textBlock("p1", "first", model.BlockContentText_Paragraph),
				textBlock("p2", "second", model.BlockContentText_Paragraph),
			},
			want: want{caret: -1},
		},
		{
			// intoBlock, which has always reported -1 here for the same reason
			name:        "one block into an empty paragraph, which is filled and forked",
			targetStyle: model.BlockContentText_Paragraph,
			rng:         model.Range{From: 0, To: 0},
			pasteBlocks: []*model.Block{
				textBlock("p1", "first", model.BlockContentText_Paragraph),
			},
			want: want{caret: -1},
		},
		{
			// the block survives, so a position inside it is meaningful and is reported
			name:        "one block into a block with text, which survives",
			targetStyle: model.BlockContentText_Paragraph,
			targetText:  "existing",
			rng:         model.Range{From: 8, To: 8},
			pasteBlocks: []*model.Block{
				textBlock("p1", "first", model.BlockContentText_Paragraph),
			},
			want: want{caret: 13, focusedSurvives: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			sb := smarttest.New("test")
			sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"target"}}))
			sb.AddBlock(simple.New(textBlock("target", tc.targetText, tc.targetStyle)))
			cb := newFixture(t, sb)
			rng := tc.rng

			// when
			_, _, caret, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
				FocusedBlockId:    "target",
				SelectedTextRange: &rng,
				AnySlot:           tc.pasteBlocks,
			}, "")

			// then
			require.NoError(t, err)
			got := want{
				caret:           caret,
				focusedSurvives: lo.Contains(liveIds(t, sb), "target"),
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

// Retitling a callout through the HTML slot must not cost it its icon. pasteHtml hands an
// incoming plain paragraph the focused block's style and nothing else (GO-250), so the block
// that reaches RangeTextPaste is a Callout with an empty icon — and an icon adopted
// unconditionally would take that emptiness for an instruction and wipe the real one.
//
// Driven through the HtmlSlot rather than AnySlot on purpose: the synthesized block is what
// makes this case, and a hand-built paste block cannot reproduce it.
func TestPasteHtmlIntoStyledBlockKeepsItsIcon(t *testing.T) {
	type want struct {
		style model.BlockContentTextStyle
		text  string
		icon  string
		image string
	}
	for _, tc := range []struct {
		name       string
		targetText string
		rng        model.Range
		html       string
		want       want
	}{
		{
			name:       "replacing all of the text",
			targetText: "old",
			rng:        model.Range{From: 0, To: 3},
			html:       "<p>New note</p>",
			want: want{
				style: model.BlockContentText_Callout, text: "New note",
				icon: "\U0001f4a1", image: "imagehash",
			},
		},
		{
			name:       "appending at the end",
			targetText: "old",
			rng:        model.Range{From: 3, To: 3},
			html:       "<p>more</p>",
			want: want{
				style: model.BlockContentText_Callout, text: "oldmore",
				icon: "\U0001f4a1", image: "imagehash",
			},
		},
		{
			// an empty callout has a style of its own, so it is not adopting one
			name:       "filling it while it is empty",
			targetText: "",
			rng:        model.Range{From: 0, To: 0},
			html:       "<p>New note</p>",
			want: want{
				style: model.BlockContentText_Callout, text: "New note",
				icon: "\U0001f4a1", image: "imagehash",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			target := textBlock("target", tc.targetText, model.BlockContentText_Callout)
			target.GetText().IconEmoji = "\U0001f4a1"
			target.GetText().IconImage = "imagehash"
			sb := smarttest.New("test")
			sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"target"}}))
			sb.AddBlock(simple.New(target))
			cb := newFixture(t, sb)
			rng := tc.rng

			// when
			_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
				FocusedBlockId:    "target",
				SelectedTextRange: &rng,
				HtmlSlot:          tc.html,
			}, "")

			// then
			require.NoError(t, err)
			st := sb.NewState()
			childIds := st.Pick("test").Model().ChildrenIds
			require.Len(t, childIds, 1, "the paste must stay in the one block")
			b := st.Pick(childIds[0])
			require.NotNil(t, b)
			got := want{
				style: b.Model().GetText().Style,
				text:  b.Model().GetText().Text,
				icon:  b.Model().GetText().IconEmoji,
				image: b.Model().GetText().IconImage,
			}
			assert.Equal(t, tc.want, got)
		})
	}
}
