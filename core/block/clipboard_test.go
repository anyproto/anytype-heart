package block

/*
import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/stretchr/testify/require"
	"strconv"
	"testing"
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

func createPage(t *testing.T, textArr []string) *pageFixture {
	blocks := createBlocks(textArr)

	fx := newPageFixture(t, blocks...)
	defer fx.ctrl.Finish()
	defer fx.tearDown()

	return fx
}

func createPageWithMarks(t *testing.T, textArr []string, marksArr [][]*model.BlockContentTextMark) *pageFixture {
	blocks := createBlocksWithMarks(textArr, marksArr)

	fx := newPageFixture(t, blocks...)
	defer fx.ctrl.Finish()
	defer fx.tearDown()

	return fx
}

func checkBlockText(t *testing.T, fx *pageFixture, textArr []string)  {
	require.Len(t, fx.versions[fx.GetId()].Model().ChildrenIds, len(textArr))

	for i := 0; i < len(textArr); i++  {
		id := fx.versions[fx.GetId()].Model().ChildrenIds[i]
		require.Equal(t, textArr[i], fx.versions[id].Model().GetText().Text)
	}
	//fmt.Print("\n")
}

func checkBlockMarks(t *testing.T, fx *pageFixture, marksArr [][]*model.BlockContentTextMark)  {
	require.Len(t, fx.versions[fx.GetId()].Model().ChildrenIds, len(marksArr))

	for i := 0; i < len(marksArr); i++  {
		id := fx.versions[fx.GetId()].Model().ChildrenIds[i]

		/*if marksArr[i] != nil {
			fmt.Println( i, ">>",  marksArr[i], fx.versions[id].Model().GetText().Marks.Marks)
			require.True(t, fx.versions[id].Model().GetText().Marks != nil)
			require.True(t, len(fx.versions[id].Model().GetText().Marks.Marks) > 0)
		}*/
		/*
		if fx.versions[id].Model().GetText().Marks != nil &&
			len(fx.versions[id].Model().GetText().Marks.Marks) > 0 &&
			marksArr[i] != nil {

			require.Equal(t, len(marksArr[i]), len(fx.versions[id].Model().GetText().Marks.Marks))
			//fmt.Println("Marks count:", len(marksArr[i]), len(fx.versions[id].Model().GetText().Marks.Marks))
			for j := 0; j < len(marksArr[i]); j++ {
				require.Equal(t, marksArr[i][j], fx.versions[id].Model().GetText().Marks.Marks[j])
				//fmt.Println("Should be:", marksArr[i][j], "Real:", fx.versions[id].Model().GetText().Marks.Marks[j])
			}
		}
	}
}

func checkBlockTextAndStyle(t *testing.T, fx *pageFixture, textArr []string)  {
	require.Len(t, fx.versions[fx.GetId()].Model().ChildrenIds, len(textArr))

/*	cIds := fx.versions[fx.GetId()].Model().ChildrenIds
	for i := 0; i < len(cIds); i++  {
		fmt.Println( i, ": ", fx.versions[cIds[i]].Model() )
	}*/
/*
	for i := 0; i < len(textArr); i++  {
		id := fx.versions[fx.GetId()].Model().ChildrenIds[i]
		require.Equal(t, textArr[i], fx.versions[id].Model().GetText().Text)
		//fmt.Println( i, ": ",fx.versions[id].Model().String() )
	}
}

func pasteAny(t *testing.T, fx *pageFixture, id string, textRange model.Range, selectedBlockIds []string, blocks []*model.Block) {
	req := pb.RpcBlockPasteRequest{}
	if id != "" { req.FocusedBlockId = id }
	if len(selectedBlockIds) > 0 { req.SelectedBlockIds = selectedBlockIds }
	req.SelectedTextRange = &textRange
	req.AnySlot = blocks
	_, err := fx.pasteAny(req)
	require.NoError(t, err)
}

func pasteText(t *testing.T, fx *pageFixture, id string, textRange model.Range, selectedBlockIds []string, textSlot string) {
	req := pb.RpcBlockPasteRequest{}
	if id != "" { req.FocusedBlockId = id }
	if len(selectedBlockIds) > 0 { req.SelectedBlockIds = selectedBlockIds }
	req.TextSlot = textSlot
	req.SelectedTextRange = &textRange
	_, err := fx.pasteText(req)
	require.NoError(t, err)
}

func pasteHTML(t *testing.T, fx *pageFixture, id string, textRange model.Range, selectedBlockIds []string, htmlSlot string) {
	req := pb.RpcBlockPasteRequest{}
	if id != "" { req.FocusedBlockId = id }
	if len(selectedBlockIds) > 0 { req.SelectedBlockIds = selectedBlockIds }
	req.HtmlSlot = htmlSlot
	req.SelectedTextRange = &textRange
	_, err := fx.pasteHtml(req)
	require.NoError(t, err)
}

func checkEvents(t *testing.T, fx *pageFixture, eventsLen int, messagesLen int) {
	//require.Len(t, fx.serviceFx.events, eventsLen)
	//require.Len(t, fx.serviceFx.events[1].Messages, messagesLen)
}

func TestCommonSmart_splitMarks(t *testing.T) {
	t.Run("<b>lorem</b> lorem (**********)  :--->   <b>lorem</b> lorem __PASTE__  \n(m.Range.From < r.From) && (m.Range.To <= r.From)", func(t *testing.T) {
		initialText := []string{"abcdef"}
		initialMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:1, To:3},
				Type: model.BlockContentTextMark_Bold,
			}},
		}

		pasteText := []string{"123456"}
		pasteMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:0, To:4},
				Type: model.BlockContentTextMark_Bold,
			}},
		}

		fx := createPageWithMarks(t, initialText, initialMarks)

		pasteAny(t, fx, "1", model.Range{From: 5, To: 5}, []string{}, createBlocksWithMarks(pasteText, pasteMarks));
		checkBlockMarks(t, fx, [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:0+5, To:4+5},
				Type: model.BlockContentTextMark_Bold,
			},{
				Range: &model.Range{From:1, To:3},
				Type: model.BlockContentTextMark_Bold,
			}},
		});
	})

	t.Run("<b>lorem lorem(******</b>******)  :--->   <b>lorem lorem</b> __PASTE__  \n(m.Range.From < r.From) && (m.Range.To > r.From) && (m.Range.To < r.To)", func(t *testing.T) {
		initialText := []string{"abcdef"}
		initialMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:1, To:3},
				Type: model.BlockContentTextMark_Bold,
			}},
		}

		pasteText := []string{"123456"}
		pasteMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:0, To:4},
				Type: model.BlockContentTextMark_Bold,
			}},
		}

		fx := createPageWithMarks(t, initialText, initialMarks)

		pasteAny(t, fx, "1", model.Range{From: 2, To: 5}, []string{}, createBlocksWithMarks(pasteText, pasteMarks));
		checkBlockMarks(t, fx, [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:0+5-3, To:4+5-3},
				Type: model.BlockContentTextMark_Bold,
			},{
				Range: &model.Range{From:1, To:2},
				Type: model.BlockContentTextMark_Bold,
			}},
		});
	})

	t.Run("(**<b>******</b>******)  :--->     __PASTE__  (m.Range.From >= r.From) && (m.Range.To <= r.To)", func(t *testing.T) {
		initialText := []string{"abcdef"}
		initialMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:1, To:3},
				Type: model.BlockContentTextMark_Bold,
			}},
		}

		pasteText := []string{"123456"}
		pasteMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:0, To:4},
				Type: model.BlockContentTextMark_Bold,
			}},
		}

		fx := createPageWithMarks(t, initialText, initialMarks)

		pasteAny(t, fx, "1", model.Range{From: 1, To: 3}, []string{}, createBlocksWithMarks(pasteText, pasteMarks));
		checkBlockMarks(t, fx, [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:1, To:5},
				Type: model.BlockContentTextMark_Bold,
			}},
		});
	})

	t.Run("<b>lorem (*********) lorem</b>  :--->   <b>lorem</b> __PASTE__ <b>lorem</b>  (m.Range.From < r.From) && (m.Range.To > r.To)", func(t *testing.T) {
		initialText := []string{"abcdef"}
		initialMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:1, To:4},
				Type: model.BlockContentTextMark_Bold,
			}},
		}

		pasteText := []string{"123456"}
		pasteMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:1, To:4},
				Type: model.BlockContentTextMark_Italic,
			}},
		}

		fx := createPageWithMarks(t, initialText, initialMarks)

		pasteAny(t, fx, "1", model.Range{From: 2, To: 3}, []string{}, createBlocksWithMarks(pasteText, pasteMarks));
		checkBlockMarks(t, fx, [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:3, To:6},
				Type: model.BlockContentTextMark_Italic,
			},{
				Range: &model.Range{From:1, To:2},
				Type: model.BlockContentTextMark_Bold,
			},
			{
				Range: &model.Range{From:8, To:9},
				Type: model.BlockContentTextMark_Bold,
			}},
		});
	})

	t.Run("(*********) <b>lorem lorem</b>  :--->   __PASTE__ <b>lorem lorem</b>  (m.Range.From > r.To)", func(t *testing.T) {
		initialText := []string{"abcdef"}
		initialMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:3, To:4},
				Type: model.BlockContentTextMark_Bold,
			}},
		}

		pasteText := []string{"123456"}
		pasteMarks := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:3, To:4},
				Type: model.BlockContentTextMark_Italic,
			}},
		}

		fx := createPageWithMarks(t, initialText, initialMarks)

		pasteAny(t, fx, "1", model.Range{From: 1, To: 2}, []string{}, createBlocksWithMarks(pasteText, pasteMarks));

		checkBlockMarks(t, fx, [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:4, To:5},
				Type: model.BlockContentTextMark_Italic,
			},{
				Range: &model.Range{From:8, To:9},
				Type: model.BlockContentTextMark_Bold,
			}},
		});
	})
}


func TestCommonSmart_pasteTitle(t *testing.T) {

	t.Run("Simple: 2 p blocks", func(t *testing.T) {
		blocks := []*model.Block{}

		blocks = append(blocks, &model.Block{Id: "1",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "Title",
					Style: model.BlockContentText_Title,
				},
			},
		})

		blocks = append(blocks, &model.Block{Id: "2",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "Text1",
					Style: model.BlockContentText_Paragraph,
				},
			},
		})

		blocks = append(blocks, &model.Block{Id: "3",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "Text2",
					Style: model.BlockContentText_Paragraph,
				},
			},
		})

		fx := newPageFixture(t, blocks...)
		defer fx.ctrl.Finish()
		defer fx.tearDown()

		pasteHTML(t, fx, "1", model.Range{From: 5, To: 5}, []string{}, "<p>abcdef</p><p>hello</p><p>ololo</p>");
		checkBlockText(t, fx, []string{"Titleabcdef", "hello", "ololo", "Text1", "Text2"});
	})
}

func TestCommonSmart_pasteHTML(t *testing.T) {

	t.Run("Simple: 2 p blocks", func(t *testing.T) {
		fx := createPage(t, []string{"11111", "22222", "33333", "abcde", "55555"})
		pasteHTML(t, fx, "4", model.Range{From: 2, To: 4}, []string{}, "<p>abcdef</p><p>hello</p>");
		checkBlockText(t, fx, []string{"11111", "22222", "33333", "ab", "abcdef", "hello", "e", "55555"});
		checkEvents(t, fx, 2, 5)
	})

	t.Run("Simple: 1 p 1 h2", func(t *testing.T) {
		fx := createPage(t, []string{})
		pasteHTML(t, fx, "", model.Range{From: 0, To: 0}, []string{}, "<h2>lorem</h2><p>ipsum</p>");
		checkBlockTextAndStyle(t, fx, []string{"lorem", "ipsum"});
		checkEvents(t, fx, 2, 5)
	})

	t.Run("Simple: 1 p with markup", func(t *testing.T) {
		fx := createPage(t, []string{})
		pasteHTML(t, fx, "", model.Range{From: 0, To: 0}, []string{}, "<p>i<b>p</b>s <i>um</i> ololo</p>");
		checkBlockTextAndStyle(t, fx, []string{"ips um ololo"});
		checkEvents(t, fx, 2, 5)
	})

	t.Run("Markup in header", func(t *testing.T) {
		fx := createPage(t, []string{})
		pasteHTML(t, fx, "", model.Range{From: 0, To: 0}, []string{}, "<h1>foo <em>bar</em> baz</h1>\n");
		checkBlockTextAndStyle(t, fx, []string{"foo bar baz"});
		checkEvents(t, fx, 2, 5)
	})

	t.Run("Different headers", func(t *testing.T) {
		fx := createPage(t, []string{})
		pasteHTML(t, fx, "", model.Range{From: 0, To: 0}, []string{}, "<h3>foo</h3>\n<h2>foo</h2>\n<h1>foo</h1>\n");
		checkBlockTextAndStyle(t, fx, []string{"foo", "foo", "foo"});
		checkEvents(t, fx, 2, 5)
	})

	t.Run("Code block -> header", func(t *testing.T) {
		fx := createPage(t, []string{})
		pasteHTML(t, fx, "", model.Range{From: 0, To: 0}, []string{}, "<pre><code># foo\n</code></pre>\n",);
		checkBlockTextAndStyle(t, fx, []string{"# foo\n\n"});
		checkEvents(t, fx, 2, 5)
	})

	t.Run("Link markup, auto paragraph", func(t *testing.T) {
		fx := createPage(t, []string{})
		pasteHTML(t, fx, "", model.Range{From: 0, To: 0}, []string{}, "<div><a href=\"bar\">foo</a></div>\n");
		checkBlockTextAndStyle(t, fx, []string{"foo"});
		checkEvents(t, fx, 2, 5)
	})

	t.Run("", func(t *testing.T) {
		fx := createPage(t, []string{})
		pasteHTML(t, fx, "", model.Range{From: 0, To: 0}, []string{}, "<table><tr><td>\nfoo\n</td></tr></table>\n");
		checkBlockTextAndStyle(t, fx, []string{"foo"});
		checkEvents(t, fx, 2, 5)
	})

	t.Run("Link in paragraph", func(t *testing.T) {
		fx := createPage(t, []string{})
		pasteHTML(t, fx, "", model.Range{From: 0, To: 0}, []string{}, "<p><a href=\"url\">foo</a></p>\n");
		checkBlockTextAndStyle(t, fx, []string{"foo"});
		checkEvents(t, fx, 2, 5)
	})

	t.Run("Nested tags: p inside quote && header with markup", func(t *testing.T) {
		fx := createPage(t, []string{})
		pasteHTML(t, fx, "", model.Range{From: 0, To: 0}, []string{}, "<h1><a href=\"/url\">Foo</a></h1>\n<blockquote>\n<p>bar</p>\n</blockquote>\n");
		checkBlockTextAndStyle(t, fx, []string{"Foo", "bar"});
		checkEvents(t, fx, 2, 5)
	})

	t.Run("Nested tags: h1 && p inside quote", func(t *testing.T) {
		fx := createPage(t, []string{})
		pasteHTML(t, fx, "", model.Range{From: 0, To: 0}, []string{}, "<blockquote>\n<h1>Foo</h1>\n<p>bar\nbaz</p>\n</blockquote>\n");
		checkBlockTextAndStyle(t, fx, []string{"Foo", "bar\nbaz"});
		checkEvents(t, fx, 2, 5)
	})
}

func TestCommonSmart_pasteAny_marks(t *testing.T) {

	t.Run("should paste single mark; paste to the end, no focus", func(t *testing.T) {
		textArr := []string{"11111"}
		marksArr := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:1, To:2},
				Type: model.BlockContentTextMark_Bold,
			}},
		}

		fx := createPage(t, textArr)

		pasteAny(t, fx, "", model.Range{From: 0, To: 0}, []string{}, createBlocksWithMarks([]string{"99999"}, marksArr));
		checkBlockMarks(t, fx, [][]*model.BlockContentTextMark{
			{{}},
			{{
				Range: &model.Range{From:1, To:2},
				Type: model.BlockContentTextMark_Bold,
			}},
		});
		checkEvents(t, fx, 2, 5)
	})

	t.Run("should paste multiple marks; paste to the end, no focus", func(t *testing.T) {
		textArr := []string{"11111"}
		marksArr := [][]*model.BlockContentTextMark{
			{{
				Range: &model.Range{From:1, To:2},
				Type: model.BlockContentTextMark_Bold,
			}, {
				Range: &model.Range{From:4, To:5},
				Type: model.BlockContentTextMark_Strikethrough,
			}},
			{{
				Range: &model.Range{From:0, To:4},
				Type: model.BlockContentTextMark_Italic,
			}},
		}

		fx := createPage(t, textArr)

		pasteAny(t, fx, "", model.Range{From: 0, To: 0}, []string{}, createBlocksWithMarks([]string{"99999", "00000"}, marksArr));
		checkBlockMarks(t, fx, [][]*model.BlockContentTextMark{
			{{}},
			{{
				Range: &model.Range{From:1, To:2},
				Type: model.BlockContentTextMark_Bold,
			}, {
				Range: &model.Range{From:4, To:5},
				Type: model.BlockContentTextMark_Strikethrough,
			}},
			{{
				Range: &model.Range{From:0, To:4},
				Type: model.BlockContentTextMark_Italic,
			}},
		});
		checkEvents(t, fx, 2, 5)
	})
}

func TestCommonSmart_pasteAny(t *testing.T) {

	t.Run("should split block on paste", func(t *testing.T) {
		fx := createPage(t, []string{"11111", "22222", "33333", "abcde", "55555"})
		pasteAny(t, fx, "4", model.Range{From: 2, To: 4}, []string{}, createBlocks([]string{"22222", "33333"}));

		checkBlockText(t, fx, []string{"11111", "22222", "33333", "ab", "22222", "33333", "e", "55555"});
		checkEvents(t, fx, 2, 5)
	})

	t.Run("should paste to the end when no focus", func(t *testing.T) {
		fx := createPage(t, []string{"11111", "22222", "33333", "44444", "55555"})
		pasteAny(t, fx, "", model.Range{From: 0, To: 0}, []string{}, createBlocks([]string{"22222", "33333"}));

		checkBlockText(t, fx, []string{"11111", "22222", "33333", "44444", "55555", "22222", "33333"});
		checkEvents(t, fx, 2, 3)
	})

	t.Run("should paste to the end when no focus", func(t *testing.T) {
		fx := createPage(t, []string{"11111", "22222", "33333", "44444", "55555"})
		pasteAny(t, fx, "", model.Range{From: 0, To: 0}, []string{"2", "3", "4"}, createBlocks([]string{"22222", "33333"}));

		checkBlockText(t, fx, []string{"11111", "22222", "33333", "55555"});
		checkEvents(t, fx, 2, 6)
	})

	t.Run("should paste to the empty page", func(t *testing.T) {
		fx := createPage(t, []string{})
		pasteAny(t, fx, "", model.Range{From: 0, To: 0}, []string{}, createBlocks([]string{"22222", "33333"}));

		checkBlockText(t, fx, []string{"22222", "33333"});
		checkEvents(t, fx, 2, 6)
	})

	t.Run("should paste when all blocks selected", func(t *testing.T) {
		fx := createPage(t, []string{"11111", "22222", "33333", "44444", "55555"})
		pasteAny(t, fx, "", model.Range{From: 0, To: 0}, []string{"1", "2", "3", "4", "5"}, createBlocks([]string{"aaaaa", "bbbbb"}));

		checkBlockText(t, fx, []string{"aaaaa", "bbbbb"});
		checkEvents(t, fx, 2, 6)
	})
}

func TestCommonSmart_RangeSplit(t *testing.T) {
	t.Run("1. Cursor at the beginning, range == 0. Expected behavior: inserting blocks on top", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteAny(t, fx, "4", model.Range{From:0, To:0}, []string{}, createBlocks([]string{ "aaaaa",  "bbbbb" }));

		checkBlockText(t, fx, []string{ "11111", "22222", "33333", "qwerty", "aaaaa", "bbbbb",  "55555" });
		checkEvents(t, fx, 2, 6)
	})

	t.Run("2. Cursor in a middle, range == 0. Expected behaviour: split block top + bottom, insert in a middle", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteAny(t, fx, "4", model.Range{From:2, To:2}, []string{}, createBlocks([]string{ "aaaaa",  "bbbbb" }));

		checkBlockText(t, fx, []string{ "11111",  "22222",  "33333", "qw",  "aaaaa",  "bbbbb",  "erty", "55555" });
		checkEvents(t, fx, 2, 6)
	})

	t.Run("3. Cursor: end, range == 0. Expected behaviour: insert after block", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteAny(t, fx, "4", model.Range{From:6, To:6}, []string{}, createBlocks([]string{ "aaaaa",  "bbbbb" }));

		checkBlockText(t, fx, []string{ "11111", "22222", "33333", "qwerty", "aaaaa", "bbbbb", "55555" });
		checkEvents(t, fx, 2, 6)
	})

	t.Run("4. Cursor: from 1/4 to 3/4, range == 1/2. Expected behaviour: split block top + bottom, remove Range, insert in a middle", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteAny(t, fx, "4", model.Range{From:2, To:4}, []string{}, createBlocks([]string{ "aaaaa",  "bbbbb" }));

		checkBlockText(t, fx, []string{ "11111", "22222", "33333", "qw", "aaaaa", "bbbbb", "ty", "55555" });
		checkEvents(t, fx, 2, 6)
	})

	t.Run("5. Cursor: from start to middle, range == 1/2. Expected Behavior: top insert, range removal", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteAny(t, fx, "4", model.Range{From:0, To:3}, []string{}, createBlocks([]string{ "aaaaa",  "bbbbb" }));

		checkBlockText(t, fx, []string{ "11111",  "22222",  "33333", "aaaaa", "bbbbb", "rty", "55555" });
		checkEvents(t, fx, 2, 6)
	})

	t.Run("6. Cursor: middle to end, range == 1/2. Expected Behavior: bottom insert, range removal", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteAny(t, fx, "4", model.Range{From:3, To:6}, []string{}, createBlocks([]string{ "aaaaa",  "bbbbb" }));

		checkBlockText(t, fx, []string{ "11111", "22222", "33333", "qwe", "aaaaa", "bbbbb",  "55555" });
		checkEvents(t, fx, 2, 6)
	})

	t.Run("7. Cursor from start to end, range == 1. Expected behavior: bottom / top insert, block deletion", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteAny(t, fx, "4", model.Range{From:0, To:6}, []string{}, createBlocks([]string{ "aaaaa",  "bbbbb" }));

		checkBlockText(t, fx, []string{ "11111",  "22222",  "33333", "aaaaa",  "bbbbb",  "55555" });
		checkEvents(t, fx, 2, 6)
	})
}

func TestCommonSmart_TextSlot_RangeSplitCases(t *testing.T) {
	t.Run("1. Cursor at the beginning, range == 0. Expected behavior: inserting blocks on top", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteText(t, fx, "4", model.Range{From:0, To:0}, []string{}, "aaaaa\nbbbbb");

		checkBlockText(t, fx, []string{ "11111", "22222", "33333",  "qwerty", "aaaaa", "bbbbb", "55555" });
	})

	t.Run("2. Cursor in a middle, range == 0. Expected behaviour: split block top + bottom, insert in a middle", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteText(t, fx, "4", model.Range{From:2, To:2}, []string{}, "aaaaa\nbbbbb");

		checkBlockText(t, fx, []string{ "11111",  "22222",  "33333", "qw",  "aaaaa",  "bbbbb",  "erty", "55555" });
	})

	t.Run("3. Cursor: end, range == 0. Expected behaviour: insert after block", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteText(t, fx, "4", model.Range{From:6, To:6}, []string{}, "aaaaa\nbbbbb");

		checkBlockText(t, fx, []string{ "11111", "22222", "33333", "qwerty", "aaaaa", "bbbbb", "55555" });
	})

	t.Run("4. Cursor from 1/4 to 3/4, range == 1/2. Expected behaviour: split block: top + bottom, remove Range, insert in a middle", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteText(t, fx, "4", model.Range{From:2, To:4}, []string{}, "aaaaa\nbbbbb");

		checkBlockText(t, fx, []string{ "11111", "22222", "33333", "qw", "aaaaa", "bbbbb", "ty", "55555" });
	})

	t.Run("5. Cursor from stast to middle, range == 1/2. Expected behaviour: insert top, remove Range", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteText(t, fx, "4", model.Range{From:0, To:3}, []string{}, "aaaaa\nbbbbb");

		checkBlockText(t, fx, []string{ "11111",  "22222",  "33333", "aaaaa", "bbbbb", "rty", "55555" });
	})

	t.Run("6. Cursor: middle to end, range == 1/2. Expected Behavior: bottom insert, range removal", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteText(t, fx, "4", model.Range{From:3, To:6}, []string{}, "aaaaa\nbbbbb");

		checkBlockText(t, fx, []string{ "11111", "22222", "33333", "qwe", "aaaaa", "bbbbb",  "55555" });
	})

	t.Run("7. Cursor from start to end, range == 1. Expected behavior: bottom / top insert, block deletion", func(t *testing.T) {
		fx := createPage(t, []string{ "11111",  "22222",  "33333",  "qwerty",  "55555" })
		pasteText(t, fx, "4", model.Range{From:0, To:6}, []string{}, "aaaaa\nbbbbb");

		checkBlockText(t, fx, []string{ "11111",  "22222",  "33333", "aaaaa",  "bbbbb",  "55555" });
	})
}

func TestCommonSmart_TextSlot_CommonCases(t *testing.T) {

	t.Run("should split block on paste", func(t *testing.T) {
		fx := createPage(t, []string{"11111", "22222", "33333", "abcde", "55555"})
		pasteText(t, fx, "4", model.Range{From: 2, To: 4}, []string{}, "22222\n33333");

		checkBlockText(t, fx, []string{"11111", "22222", "33333", "ab", "22222", "33333", "e", "55555"});
	})

	t.Run("should paste to the end when no focus", func(t *testing.T) {
		fx := createPage(t, []string{"11111", "22222", "33333", "44444", "55555"})
		pasteText(t, fx, "", model.Range{From: 0, To: 0}, []string{}, "22222\n33333");

		checkBlockText(t, fx, []string{"11111", "22222", "33333", "44444", "55555", "22222", "33333"});
	})

	t.Run("should paste to the end when no focus", func(t *testing.T) {
		fx := createPage(t, []string{"11111", "22222", "33333", "44444", "55555"})
		pasteText(t, fx, "", model.Range{From: 0, To: 0}, []string{"2", "3", "4"}, "22222\n33333");

		checkBlockText(t, fx, []string{"11111", "22222", "33333", "55555"});
	})

	t.Run("should paste to the empty page", func(t *testing.T) {
		fx := createPage(t, []string{})
		pasteText(t, fx, "", model.Range{From: 0, To: 0}, []string{}, "22222\n33333");

		checkBlockText(t, fx, []string{"22222", "33333"});
	})

	t.Run("should paste when all blocks selected", func(t *testing.T) {
		fx := createPage(t, []string{"11111", "22222", "33333", "44444", "55555"})
		pasteText(t, fx, "", model.Range{From: 0, To: 0}, []string{"1", "2", "3", "4", "5"}, "aaaaa\nbbbbb");

		checkBlockText(t, fx, []string{"aaaaa", "bbbbb"});
	})
}
*/