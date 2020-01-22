package block

import (
	"fmt"
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

func createPage(t *testing.T, textArr []string) *pageFixture {
	blocks := createBlocks(textArr)

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
	fmt.Print("\n")
}

func pasteAny(t *testing.T, fx *pageFixture, id string, textRange model.Range, selectedBlockIds []string, blocks []*model.Block) {
	req := pb.RpcBlockPasteRequest{}
	if id != "" { req.FocusedBlockId = id }
	if len(selectedBlockIds) > 0 { req.SelectedBlockIds = selectedBlockIds }
	req.SelectedTextRange = &textRange
	req.AnySlot = blocks
	err := fx.pasteAny(req)
	require.NoError(t, err)
}

func pasteText(t *testing.T, fx *pageFixture, id string, textRange model.Range, selectedBlockIds []string, textSlot string) {
	req := pb.RpcBlockPasteRequest{}
	if id != "" { req.FocusedBlockId = id }
	if len(selectedBlockIds) > 0 { req.SelectedBlockIds = selectedBlockIds }
	req.TextSlot = textSlot
	req.SelectedTextRange = &textRange
	err := fx.pasteText(req)
	require.NoError(t, err)
}

func pasteHTML(t *testing.T, fx *pageFixture, id string, textRange model.Range, selectedBlockIds []string, htmlSlot string) {
	req := pb.RpcBlockPasteRequest{}
	if id != "" { req.FocusedBlockId = id }
	if len(selectedBlockIds) > 0 { req.SelectedBlockIds = selectedBlockIds }
	req.HtmlSlot = htmlSlot
	req.SelectedTextRange = &textRange
	err := fx.pasteText(req)
	require.NoError(t, err)
}

func checkEvents(t *testing.T, fx *pageFixture, eventsLen int, messagesLen int) {
	//require.Len(t, fx.serviceFx.events, eventsLen)
	//require.Len(t, fx.serviceFx.events[1].Messages, messagesLen)
}

func TestCommonSmart_pasteHTML(t *testing.T) {

	t.Run("Simple: 2 <p> blocks", func(t *testing.T) {
		fx := createPage(t, []string{"11111", "22222", "33333", "abcde", "55555"})
		pasteHTML(t, fx, "4", model.Range{From: 2, To: 4}, []string{}, "<p>abcdef</p><p>hello</p>");
		checkBlockText(t, fx, []string{"11111", "22222", "33333", "ab", "abcdef", "hello", "e", "55555"});
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

		checkBlockText(t, fx, []string{ "11111", "22222", "33333", "aaaaa", "bbbbb", "qwerty", "55555" });
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

		checkBlockText(t, fx, []string{ "11111", "22222", "33333", "aaaaa", "bbbbb", "qwerty", "55555" });
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
