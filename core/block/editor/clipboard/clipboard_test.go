package clipboard

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
)

func TestCommonSmart_importFromMarkdown(t *testing.T) {
	t.Run("No marks, paste middleCut to the middle", func(t *testing.T) {
		sb := page(block("1", "123456789"))
		cb := NewClipboard(sb)
		cb.ConvertMarkdown("/Users/enkogu/Downloads/Export-78020b1c-9a70-46a8-89c6-16f3136a10a8")
	})
}

func TestCommonSmart_copyRangeNoMarks(t *testing.T) {
	t.Run("No marks, paste middleCut to the middle", func(t *testing.T) {
		sb := page(block("1", "123456789"))
		rangePaste(sb, t, "1", rng(2, 4), rng(3, 6), block("n1", "abcdefghi"))
		shouldBe(sb, t, block("1", "12def56789"))
	})

	t.Run("No marks, paste fullCut to the middle", func(t *testing.T) {
		sb := page(block("1", "123456789"))
		rangePaste(sb, t, "1", rng(2, 4), rng(0, 9), block("n1", "abcdefghi"))
		shouldBe(sb, t, block("1", "12abcdefghi56789"))
	})

	t.Run("No marks, paste zeroCut to the middle", func(t *testing.T) {
		sb := page(block("1", "123456789"))
		rangePaste(sb, t, "1", rng(2, 4), rng(5, 5), block("n1", "abcdefghi"))
		shouldBe(sb, t, block("1", "1256789"))
	})

	t.Run("No marks, paste middleToEndCut to the middle", func(t *testing.T) {
		sb := page(block("1", "123456789"))
		rangePaste(sb, t, "1", rng(2, 5), rng(0, 3), block("n1", "abcdefghi"))
		shouldBe(sb, t, block("1", "12abc6789"))
	})

	t.Run("No marks, paste fullCut to the full", func(t *testing.T) {
		sb := page(block("1", "123456789"))
		rangePaste(sb, t, "1", rng(0, 9), rng(0, 9), block("n1", "abcdefghi"))
		shouldBe(sb, t, block("1", "abcdefghi"))
	})

	t.Run("No marks, paste zeroCut to the full", func(t *testing.T) {
		sb := page(block("1", "123456789"))
		rangePaste(sb, t, "1", rng(0, 9), rng(3, 3), block("n1", "abcdefghi"))
		shouldBe(sb, t, block("1", ""))
	})
}

func TestCommonSmart_copyRangeOnePageMarkPartlyInRange(t *testing.T) {
	t.Run("One mark in page: one middleRange mark in page, fullCut to middle, cut mark at start", func(t *testing.T) {
		sb := page(
			block("1", "123456789", mark(bold, 3, 8)),
		)
		rangePaste(sb, t, "1", &model.Range{From: 2, To: 4}, &model.Range{From: 0, To: 9},
			block("n1", "abcdefghi"),
		)
		shouldBe(sb, t,
			block("1", "12abcdefghi56789", mark(bold, 11, 15)),
		)
	})

	t.Run("One mark in page: one middleRange mark in page, fullCut to middle, cut mark at end", func(t *testing.T) {
		sb := page(
			block("1", "123456789", mark(bold, 3, 7)),
		)
		rangePaste(sb, t, "1", &model.Range{From: 6, To: 8}, &model.Range{From: 0, To: 9},
			block("n1", "abcdefghi"),
		)
		shouldBe(sb, t,
			block("1", "123456abcdefghi9", mark(bold, 3, 6)),
		)
	})
}

func TestCommonSmart_copyRangeOnePageMark(t *testing.T) {
	t.Run("One mark in page: one fullRange mark in page, middleCut to middle", func(t *testing.T) {
		sb := page(
			block("1", "123456789", mark(bold, 0, 9)),
		)
		rangePaste(sb, t, "1", &model.Range{From: 2, To: 4}, &model.Range{From: 3, To: 6},
			block("n1", "abcdefghi"),
		)
		shouldBe(sb, t,
			block("1", "12def56789", mark(bold, 0, 2), mark(bold, 5, 10)),
		)
	})

	t.Run("One mark in page: one fullRange mark in page, middleCut to to the end", func(t *testing.T) {
		sb := page(
			block("1", "123456789", mark(bold, 0, 9)),
		)
		rangePaste(sb, t, "1", &model.Range{From: 9, To: 9}, &model.Range{From: 3, To: 6},
			block("n1", "abcdefghi"),
		)
		shouldBe(sb, t,
			block("1", "123456789def", mark(bold, 0, 9)),
		)
	})

	t.Run("One mark in page: one fullRange mark in page, middleCut to to the start", func(t *testing.T) {
		sb := page(
			block("1", "123456789", mark(bold, 0, 9)),
		)
		rangePaste(sb, t, "1", &model.Range{From: 0, To: 0}, &model.Range{From: 3, To: 6},
			block("n1", "abcdefghi"),
		)
		shouldBe(sb, t,
			block("1", "def123456789", mark(bold, 3, 12)),
		)
	})
}

func TestCommonSmart_copyRangeOnePasteMark(t *testing.T) {
	t.Run("One mark in page: one fullRange mark in paste, middleCut to end", func(t *testing.T) {
		sb := page(
			block("1", "123456789"),
		)
		rangePaste(sb, t, "1", &model.Range{From: 2, To: 4}, &model.Range{From: 3, To: 8},
			block("n1", "abcdefghi", mark(bold, 0, 9)),
		)
		shouldBe(sb, t,
			block("1", "12defgh56789", mark(bold, 2, 7)),
		)
	})

	t.Run("One mark in page: one fullRange mark in paste, middleCut to the end", func(t *testing.T) {
		sb := page(
			block("1", "123456789"),
		)
		rangePaste(sb, t, "1", &model.Range{From: 9, To: 9}, &model.Range{From: 3, To: 8},
			block("n1", "abcdefghi", mark(bold, 0, 9)),
		)
		shouldBe(sb, t,
			block("1", "123456789defgh", mark(bold, 9, 14)),
		)
	})

	t.Run("One mark in page: one fullRange mark in paste, middleCut to the start", func(t *testing.T) {
		sb := page(
			block("1", "123456789"),
		)
		rangePaste(sb, t, "1", &model.Range{From: 0, To: 0}, &model.Range{From: 3, To: 8},
			block("n1", "abcdefghi", mark(bold, 0, 9)),
		)
		shouldBe(sb, t,
			block("1", "defgh123456789", mark(bold, 0, 5)),
		)
	})

	t.Run("One mark in page: one middleRange mark in paste, middleCut to the middle", func(t *testing.T) {
		sb := page(
			block("1", "123456789"),
		)
		rangePaste(sb, t, "1", &model.Range{From: 2, To: 4}, &model.Range{From: 3, To: 8},
			block("n1", "abcdefghi", mark(bold, 6, 7)),
		)
		shouldBe(sb, t,
			block("1", "12defgh56789", mark(bold, 5, 6)),
		)
	})

	t.Run("One mark in page: one middleRange mark in paste, middleCut to the start", func(t *testing.T) {
		sb := page(
			block("1", "123456789"),
		)
		rangePaste(sb, t, "1", &model.Range{From: 0, To: 0}, &model.Range{From: 3, To: 8},
			block("n1", "abcdefghi", mark(bold, 6, 7)),
		)
		shouldBe(sb, t,
			block("1", "defgh123456789", mark(bold, 3, 4)),
		)
	})

	t.Run("One mark in page: one middleRange mark in paste, middleCut to the end", func(t *testing.T) {
		sb := page(
			block("1", "123456789"),
		)
		rangePaste(sb, t, "1", &model.Range{From: 9, To: 9}, &model.Range{From: 3, To: 8},
			block("n1", "abcdefghi", mark(bold, 6, 7)),
		)
		shouldBe(sb, t,
			block("1", "123456789defgh", mark(bold, 12, 13)),
		)
	})

	t.Run("One mark in page: one middleRange mark in paste, middleCut to the middle, mark is out of range", func(t *testing.T) {
		sb := page(
			block("1", "123456789"),
		)
		rangePaste(sb, t, "1", &model.Range{From: 2, To: 4}, &model.Range{From: 3, To: 8},
			block("n1", "abcdefghi", mark(bold, 1, 2)),
		)
		shouldBe(sb, t,
			block("1", "12defgh56789"),
		)
	})
}

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

	t.Run("", func(t *testing.T) {
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
		checkBlockMarks(t, sb, [][]*model.BlockContentTextMark{
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
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa", "bbbbb", "qwerty", "55555"})
	})

	t.Run("2. Cursor in a middle, range == 0. Expected behaviour: split block top + bottom, insert in a middle", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteText(t, sb, "4", model.Range{From: 2, To: 2}, []string{}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qw", "aaaaa", "bbbbb", "erty", "55555"})
	})

	t.Run("3. Cursor: end, range == 0. Expected behaviour: insert after block", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteText(t, sb, "4", model.Range{From: 6, To: 6}, []string{}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qwerty", "aaaaa", "bbbbb", "55555"})
	})

	t.Run("4. Cursor from 1/4 to 3/4, range == 1/2. Expected behaviour: split block: top + bottom, remove Range, insert in a middle", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteText(t, sb, "4", model.Range{From: 2, To: 4}, []string{}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qw", "aaaaa", "bbbbb", "ty", "55555"})
	})

	t.Run("5. Cursor from stast to middle, range == 1/2. Expected behaviour: insert top, remove Range", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteText(t, sb, "4", model.Range{From: 0, To: 3}, []string{}, "eeeee\naaaaa\nbbbbb\nccccc")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "eeeee", "aaaaa", "bbbbb", "ccccc", "rty", "55555"})
	})

	t.Run("6. Cursor: middle to end, range == 1/2. Expected Behavior: bottom insert, range removal", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteText(t, sb, "4", model.Range{From: 3, To: 6}, []string{}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qwe", "aaaaa", "bbbbb", "55555"})
	})

	t.Run("7. Cursor from start to end, range == 1. Expected behavior: bottom / top insert, block deletion", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "qwerty", "55555"}, emptyMarks))
		pasteText(t, sb, "4", model.Range{From: 0, To: 6}, []string{}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa", "bbbbb", "55555"})
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
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "ab", "22222", "33333", "e", "55555"})
	})

	t.Run("should paste to the end when no focus", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "44444", "55555"}, emptyMarks))
		pasteText(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "44444", "55555", "aaaaa", "bbbbb"})
	})

	t.Run("should paste to the end when no focus", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "44444", "55555"}, emptyMarks))
		pasteText(t, sb, "", model.Range{From: 0, To: 0}, []string{"2", "3", "4"}, "22222\n33333")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "55555"})
	})

	t.Run("should paste to the empty page", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{}, emptyMarks))
		pasteText(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "22222\n33333")
		checkBlockText(t, sb, []string{"22222", "33333"})
	})

	t.Run("should paste when all blocks selected", func(t *testing.T) {
		sb := createPage(t, createBlocks([]string{}, []string{"11111", "22222", "33333", "44444", "55555"}, emptyMarks))
		pasteText(t, sb, "", model.Range{From: 0, To: 0}, []string{"1", "2", "3", "4", "5"}, "aaaaa\nbbbbb")
		checkBlockText(t, sb, []string{"aaaaa", "bbbbb"})
	})
}
