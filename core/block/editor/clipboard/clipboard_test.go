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
		ChildrenIds: []string{"1", "2", "3", "4", "5"},
	})).AddBlock(simple.New(blocks[0])).
		AddBlock(simple.New(blocks[1])).
		AddBlock(simple.New(blocks[2])).
		AddBlock(simple.New(blocks[3])).
		AddBlock(simple.New(blocks[4]))

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
	blocks := sb.Blocks()
	require.Len(t, len(blocks), len(textArr))

	for i, b := range blocks {
		require.Equal(t, textArr[i], b.GetText().Text)
	}
}

func checkBlockTextDebug(t *testing.T,  sb *smarttest.SmartTest, textArr []string)  {
	for i, _ := range textArr {
		fmt.Println( textArr[i])
	}

	fmt.Println("--------")
	cIds := sb.Pick("test").Model().ChildrenIds
	for _, c := range cIds {
		fmt.Println( sb.Pick(c))
	}
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

/*func pasteAny(t *testing.T, sb *smarttest.SmartTest, id string, textRange model.Range, selectedBlockIds []string, blocks []*model.Block) {
	req := pb.RpcBlockPasteRequest{}
	if id != "" { req.FocusedBlockId = id }
	if len(selectedBlockIds) > 0 { req.SelectedBlockIds = selectedBlockIds }
	req.SelectedTextRange = &textRange
	req.AnySlot = blocks
	_, err := sb.pasteAny(req)

	require.NoError(t, err)
}

func pasteText(t *testing.T, sb *smarttest.SmartTest, id string, textRange model.Range, selectedBlockIds []string, textSlot string) {
	req := pb.RpcBlockPasteRequest{}
	if id != "" { req.FocusedBlockId = id }
	if len(selectedBlockIds) > 0 { req.SelectedBlockIds = selectedBlockIds }
	req.TextSlot = textSlot
	req.SelectedTextRange = &textRange
	_, err := fx.pasteText(req)
	require.NoError(t, err)
}
*/
func pasteHtmlReq(t *testing.T, id string, textRange model.Range, selectedBlockIds []string, htmlSlot string) (req pb.RpcBlockPasteRequest) {
	req = pb.RpcBlockPasteRequest{}
	if id != "" { req.FocusedBlockId = id }
	if len(selectedBlockIds) > 0 { req.SelectedBlockIds = selectedBlockIds }
	req.HtmlSlot = htmlSlot
	req.SelectedTextRange = &textRange
	return req
}

func TestCommonSmart_splitMarks(t *testing.T) {
	t.Run("Simple: 2 p blocks", func(t *testing.T) {
		sb := smarttest.New("test")

		blocks := createBlocks([]string{"11111", "22222", "33333", "abcde", "55555"})

		sb.AddBlock(simple.New(&model.Block{
			Id: "test",
			ChildrenIds: []string{"1","2","3","4","5"},
		})).
			AddBlock(simple.New(blocks[0])).
			AddBlock(simple.New(blocks[1])).
			AddBlock(simple.New(blocks[2])).
			AddBlock(simple.New(blocks[3])).
			AddBlock(simple.New(blocks[4]))

		req := pasteHtmlReq(t, "4", model.Range{From: 2, To: 4}, []string{}, "<p>abcdef</p><p>hello</p>");

		cb := NewClipboard(sb)

		fmt.Println("req:", req)
		blockIds, err  := cb.Paste(req)

		fmt.Println("blockIds:", blockIds, "err:", err)

		textCheck := []string{"11111", "22222", "33333", "ab", "abcdef", "hello", "e", "55555"}
		for i, _ := range textCheck {
			fmt.Println( textCheck[i])
		}

		//s := sb.NewState()
		fmt.Println(":::")
		cIds := sb.Pick("test").Model().ChildrenIds
		fmt.Println(">>>:::")
		///intln("blocks", blocks)

		fmt.Println("cIds", cIds)
		for _, c := range cIds {
			fmt.Println( sb.Pick(c).Model().GetText().Text)
		}
	})
}
