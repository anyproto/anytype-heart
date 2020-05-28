package anyblocks

import (
	"strings"

	"github.com/anytypeio/go-anytype-library/pb/model"
)

func AllBlocksToCode(blocks []*model.Block) (blocksOut []*model.Block) {
	for i, b := range blocks {
		if t := b.GetText(); t != nil {
			blocks[i].GetText().Style = model.BlockContentText_Code
		}
	}

	return PreprocessBlocks(blocks)
}

func PreprocessBlocks(blocks []*model.Block) (blocksOut []*model.Block) {

	blocksOut = []*model.Block{}
	accum := []*model.Block{}

	for _, b := range blocks {
		if t := b.GetText(); t != nil && t.Style == model.BlockContentText_Code {
			accum = append(accum, b)
		} else {
			if len(accum) > 0 {
				blocksOut = append(blocksOut, CombineCodeBlocks(accum))
				accum = []*model.Block{}
			}

			blocksOut = append(blocksOut, b)
		}

	}

	if len(accum) > 0 {
		blocksOut = append(blocksOut, CombineCodeBlocks(accum))
	}

	return blocksOut
}

func CombineCodeBlocks(accum []*model.Block) (res *model.Block) {
	var textArr []string

	for _, b := range accum {
		if b.GetText() != nil {
			textArr = append(textArr, b.GetText().Text)
		}
	}

	res = &model.Block{
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  strings.Join(textArr, "\n"),
				Style: model.BlockContentText_Code,
			},
		},
	}

	return res
}
