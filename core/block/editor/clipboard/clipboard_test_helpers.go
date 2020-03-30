package clipboard

import (
	"fmt"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/stretchr/testify/require"
	"strconv"

	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"testing"
)

var emptyMarks [][]*model.BlockContentTextMark

func createBlocks(idsArr []string, textArr []string, marksArr [][]*model.BlockContentTextMark) ([]*model.Block) {
	blocks := []*model.Block{}
	for i := 0; i < len(textArr); i++  {
		marks := []*model.BlockContentTextMark{}
		if len(marksArr) > 0 && len(marksArr) > i {
			marks = marksArr[i]
		}

		id := strconv.Itoa(i + 1)
		if  len(idsArr) > 0 && len(idsArr) >= i {
			id = idsArr[i]
		}

		blocks = append(blocks, &model.Block{Id: id,
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: textArr[i],
					Marks: &model.BlockContentTextMarks{
						Marks: marks,
					},
				},

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

func createPage(t *testing.T, blocks []*model.Block) (sb *smarttest.SmartTest)  {
	sb = smarttest.New("test")

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
/*
func createPageWithBlocks(t *testing.T, blocks []*model.Block) (sb *smarttest.SmartTest)  {
	sb = smarttest.New("test")

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
}*/
/*
func createPageWithMarks(t *testing.T, textArr []string, marksArr [][]*model.BlockContentTextMark) (sb *smarttest.SmartTest) {
	sb = smarttest.New("test")
	blocks := createBlocksWithMarks(textArr, marksArr)

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
*/

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
		fmt.Println( "ID:", sb.Pick(c).Model().Id, "cId:", c, "Text:", sb.Pick(c).Model().GetText())
	}
}

func checkBlockMarks(t *testing.T, sb *smarttest.SmartTest, marksArr [][]*model.BlockContentTextMark)  {
	cIds := sb.Pick("test").Model().ChildrenIds
	require.Equal(t, len(cIds), len(marksArr))

	for i, c := range cIds {
		b := sb.Pick(c).Model()
		if marksArr[i] != nil {
			//require.True(t, b.GetText().Marks.Marks != nil)
			//require.True(t, len(b.GetText().Marks.Marks) > 0)
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

func checkBlockMarksDebug(t *testing.T, sb *smarttest.SmartTest, marksArr [][]*model.BlockContentTextMark)  {
	for i, _ := range marksArr {
		fmt.Println( marksArr[i])
	}

	cIds := sb.Pick("test").Model().ChildrenIds
	fmt.Println("--------", len(cIds), len(marksArr))

	for _, c := range cIds {
		fmt.Println( sb.Pick(c).Model().Id, sb.Pick(c).Model().GetText())
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

func pasteText(t *testing.T, sb *smarttest.SmartTest, id string, textRange model.Range, selectedBlockIds []string, textSlot string) {
	cb := NewClipboard(sb)
	req := pb.RpcBlockPasteRequest{}
	if id != "" { req.FocusedBlockId = id }
	if len(selectedBlockIds) > 0 { req.SelectedBlockIds = selectedBlockIds }
	req.TextSlot = textSlot
	req.SelectedTextRange = &textRange

	_, err  := cb.Paste(req)
	require.NoError(t, err)
}

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

