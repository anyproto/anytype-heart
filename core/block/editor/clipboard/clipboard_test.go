package clipboard

import (
	"testing"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
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
	for _, b := range blocks {
		sb.AddBlock(simple.New(b))
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
