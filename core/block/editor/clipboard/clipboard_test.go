package clipboard

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/stretchr/testify/require"
	"strconv"
)


func createBlocks(textArr []string) ([]*model.Block) {
	blocks := []*model.Block{}
	for i := 0; i < len(textArr); i++  {
		blocks = append(blocks, &model.Block{Id: strconv.Itoa(i + 1),
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{ Text: textArr[i] },
			},
		})
	}
	return blocks
}

func createBlocksWithId(textArr []string, idsArr []string) ([]*model.Block) {
	blocks := []*model.Block{}
	for i := 0; i < len(textArr); i++  {
		blocks = append(blocks, &model.Block{Id: idsArr[i],
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{ Text: textArr[i] },
			},
		})
	}
	return blocks
}

func createBlocksWithMarks(textArr []string, marksArr [][]*model.BlockContentTextMark) ([]*model.Block) {
	blocks := []*model.Block{}
	for i := 0; i < len(textArr); i++  {
		blocks = append(blocks, &model.Block{Id: strconv.Itoa(i + 1),
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: textArr[i],
					Marks: &model.BlockContentTextMarks{
						Marks: marksArr[i],
					},
				},

			},
		})
	}
	return blocks
}

func createPage(t *testing.T, textArr []string) (sb *smarttest.SmartTest)  {
	sb = smarttest.New("test")
	blocks := createBlocks(textArr)
	fmt.Println("BLOCKS:", blocks)

	cIds := []string{}
	for _, b := range blocks {
		cIds = append(cIds, b.Id)
	}

	sb.AddBlock(simple.New(&model.Block{
		Id: "test",
		ChildrenIds: cIds,
	}))

	for i, _ := range blocks {
		sb.AddBlock(simple.New(blocks[i]))
	}

	return sb
}

func createPageWithMarks(t *testing.T, textArr []string, marksArr [][]*model.BlockContentTextMark) (sb *smarttest.SmartTest) {
	sb = smarttest.New("test")
	blocks := createBlocksWithMarks(textArr, marksArr)
	for _, b := range blocks {
		sb.AddBlock(simple.New(b))
	}

	return sb
}

func checkBlockText(t *testing.T, sb *smarttest.SmartTest, textArr []string)  {
	cIds := sb.Pick("test").Model().ChildrenIds

	require.Equal(t, len(cIds), len(textArr))

	for i, c := range cIds {
		require.Equal(t, textArr[i], sb.Pick(c).Model().GetText().Text)
	}
}

func checkBlockTextDebug(t *testing.T,  sb *smarttest.SmartTest, textArr []string)  {
	for i, _ := range textArr {
		fmt.Println( textArr[i])
	}

	fmt.Println("--------")
	cIds := sb.Pick("test").Model().ChildrenIds
	for _, c := range cIds {
		fmt.Println( sb.Pick(c).Model().GetText())
	}
/*	blocks := sb.Blocks()
	fmt.Println("blocks", blocks)
	for i, b := range blocks {
		fmt.Println("i:", i,  b.GetText())
	}*/
}

func checkBlockMarks(t *testing.T, sb *smarttest.SmartTest, marksArr [][]*model.BlockContentTextMark)  {
	blocks := sb.Blocks()
	require.Len(t, len(blocks), len(marksArr))

	for i, b := range blocks {
		if marksArr[i] != nil {
			require.True(t, b.GetText().Marks.Marks != nil)
			require.True(t, len(b.GetText().Marks.Marks) > 0)
		}

		if b.GetText().Marks != nil &&
			len(b.GetText().Marks.Marks) > 0 &&
			marksArr[i] != nil {

			require.Equal(t, len(marksArr[i]), len(b.GetText().Marks.Marks))
			for j := 0; j < len(marksArr[i]); j++ {
				require.Equal(t, marksArr[i][j], b.GetText().Marks.Marks[j])
			}
		}
	}
}

func pasteAny(t *testing.T, sb *smarttest.SmartTest, id string, textRange model.Range, selectedBlockIds []string, blocks []*model.Block) {
	cb := NewClipboard(sb)
	req := pb.RpcBlockPasteRequest{}
	if id != "" { req.FocusedBlockId = id }
	if len(selectedBlockIds) > 0 { req.SelectedBlockIds = selectedBlockIds }
	req.AnySlot = blocks
	req.SelectedTextRange = &textRange

	_, err  := cb.Paste(req)
	require.NoError(t, err)
}

/*func pasteText(t *testing.T, sb *smarttest.SmartTest, id string, textRange model.Range, selectedBlockIds []string, textSlot string) {
	req := pb.RpcBlockPasteRequest{}
	if id != "" { req.FocusedBlockId = id }
	if len(selectedBlockIds) > 0 { req.SelectedBlockIds = selectedBlockIds }
	req.TextSlot = textSlot
	req.SelectedTextRange = &textRange
	_, err := fx.pasteText(req)
	require.NoError(t, err)
}*/


func pasteHtml(t *testing.T, sb *smarttest.SmartTest, id string, textRange model.Range, selectedBlockIds []string, htmlSlot string) {
	cb := NewClipboard(sb)
	req := pb.RpcBlockPasteRequest{}
	if id != "" { req.FocusedBlockId = id }
	if len(selectedBlockIds) > 0 { req.SelectedBlockIds = selectedBlockIds }
	req.HtmlSlot = htmlSlot
	req.SelectedTextRange = &textRange

	_, err  := cb.Paste(req)
	require.NoError(t, err)
}

func TestCommonSmart_pasteHtml(t *testing.T) {
	t.Run("Simple: 2 p blocks", func(t *testing.T) {
		sb := createPage(t, []string{"11111","22222", "33333", "abcde", "55555"})
		pasteHtml(t, sb,"4", model.Range{From: 2, To: 4}, []string{}, "<p>lkjhg</p><p>hello</p>")
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "ab", "lkjhg", "hello", "e", "55555"})
	})

	t.Run("Simple: 1 p 1 h2", func(t *testing.T) {
		sb := createPage(t, []string{})
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<h2>lorem</h2><p>ipsum</p>");
		checkBlockText(t, sb, []string{"lorem", "ipsum"});
	})

	t.Run("Simple: 1 p with markup", func(t *testing.T) {
		sb := createPage(t, []string{})
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<p>i<b>p</b>s <i>um</i> ololo</p>");
		checkBlockText(t, sb, []string{"ips um ololo"});
	})

	t.Run("Markup in header", func(t *testing.T) {
		sb := createPage(t, []string{})
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<h1>foo <em>bar</em> baz</h1>\n");
		checkBlockText(t, sb, []string{"foo bar baz"});
	})

	t.Run("Different headers", func(t *testing.T) {
		sb := createPage(t, []string{})
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<h3>foo</h3>\n<h2>foo</h2>\n<h1>foo</h1>\n");
		checkBlockText(t, sb, []string{"foo", "foo", "foo"});
	})

	t.Run("Code block -> header", func(t *testing.T) {
		sb := createPage(t, []string{})
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<pre><code># foo\n</code></pre>\n",);
		checkBlockText(t, sb, []string{"# foo\n\n"});
	})

	t.Run("Link markup, auto paragraph", func(t *testing.T) {
		sb := createPage(t, []string{})
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<div><a href=\"bar\">foo</a></div>\n");
		checkBlockText(t, sb, []string{"foo"});
	})

	t.Run("", func(t *testing.T) {
		sb := createPage(t, []string{})
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<table><tr><td>\nfoo\n</td></tr></table>\n");
		checkBlockText(t, sb, []string{"foo"});
	})

	t.Run("Link in paragraph", func(t *testing.T) {
		sb := createPage(t, []string{})
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<p><a href=\"url\">foo</a></p>\n");
		checkBlockText(t, sb, []string{"foo"});
	})

	t.Run("Nested tags: p inside quote && header with markup", func(t *testing.T) {
		sb := createPage(t, []string{})
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<h1><a href=\"/url\">Foo</a></h1>\n<blockquote>\n<p>bar</p>\n</blockquote>\n");
		checkBlockText(t, sb, []string{"Foo", "bar"});
	})

	t.Run("Nested tags: h1 && p inside quote", func(t *testing.T) {
		sb := createPage(t, []string{})
		pasteHtml(t, sb, "", model.Range{From: 0, To: 0}, []string{}, "<blockquote>\n<h1>Foo</h1>\n<p>bar\nbaz</p>\n</blockquote>\n");
		checkBlockText(t, sb, []string{"Foo", "bar\nbaz"});
	})
}

func TestCommonSmart_pasteAny(t *testing.T) {
	t.Run("1. Cursor at the beginning, range == 0. Expected behavior: inserting blocks on top", func(t *testing.T) {
		sb := createPage(t, []string{"11111", "22222", "33333", "qwerty", "55555"})
		pasteAny(t, sb, "4", model.Range{From: 0, To: 0}, []string{}, createBlocksWithId([]string{"aaaaa", "bbbbb"}, []string{"new1", "new2"}));
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa", "bbbbb", "qwerty", "55555"});
	})

	t.Run("2. Cursor in a middle, range == 0. Expected behaviour: split block top + bottom, insert in a middle", func(t *testing.T) {
		sb := createPage(t, []string{"11111", "22222", "33333", "qwerty", "55555"})
		pasteAny(t, sb, "4", model.Range{From: 2, To: 2}, []string{}, createBlocksWithId([]string{"aaaaa", "bbbbb"}, []string{"new1", "new2"}));
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qw", "aaaaa", "bbbbb", "erty", "55555"});
	})

	t.Run("3. Cursor: end, range == 0. Expected behaviour: insert after block", func(t *testing.T) {
		sb := createPage(t, []string{"11111", "22222", "33333", "qwerty", "55555"})
		pasteAny(t, sb, "4", model.Range{From: 6, To: 6}, []string{}, createBlocksWithId([]string{"aaaaa", "bbbbb"}, []string{"new1", "new2"}));
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qwerty", "aaaaa", "bbbbb", "55555"});
	})

	t.Run("4. Cursor: from 1/4 to 3/4, range == 1/2. Expected behaviour: split block top + bottom, remove Range, insert in a middle", func(t *testing.T) {
		sb := createPage(t, []string{"11111", "22222", "33333", "qwerty", "55555"})
		pasteAny(t, sb, "4", model.Range{From: 2, To: 4}, []string{}, createBlocksWithId([]string{"aaaaa", "bbbbb"}, []string{"new1", "new2"}));
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qw", "aaaaa", "bbbbb", "ty", "55555"});
	})

	t.Run("5. Cursor: from start to middle, range == 1/2. Expected Behavior: top insert, range removal", func(t *testing.T) {
		sb := createPage(t, []string{"11111", "22222", "33333", "qwerty", "55555"})
		pasteAny(t, sb, "4", model.Range{From: 0, To: 3}, []string{}, createBlocksWithId([]string{"aaaaa", "bbbbb"}, []string{"new1", "new2"}));
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa", "bbbbb", "rty", "55555"});
	})

	t.Run("6. Cursor: middle to end, range == 1/2. Expected Behavior: bottom insert, range removal", func(t *testing.T) {
		sb := createPage(t, []string{"11111", "22222", "33333", "qwerty", "55555"})
		pasteAny(t, sb, "4", model.Range{From: 3, To: 6}, []string{}, createBlocksWithId([]string{"aaaaa", "bbbbb"}, []string{"new1", "new2"}));
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "qwe", "aaaaa", "bbbbb", "55555"});
	})

	t.Run("7. Cursor from start to end, range == 1. Expected behavior: bottom / top insert, block deletion", func(t *testing.T) {
		sb := createPage(t, []string{"11111", "22222", "33333", "qwerty", "55555"})
		pasteAny(t, sb, "4", model.Range{From: 0, To: 6}, []string{}, createBlocksWithId([]string{"aaaaa", "bbbbb"}, []string{"new1", "new2"}));
		checkBlockText(t, sb, []string{"11111", "22222", "33333", "aaaaa", "bbbbb", "55555"});
	})
}