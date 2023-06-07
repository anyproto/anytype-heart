package anymark

import (
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func preprocessBlocks(blocks []*model.Block) (blocksOut []*model.Block) {

	blocksOut = []*model.Block{}
	accum := []*model.Block{}

	for _, b := range blocks {
		if t := b.GetText(); t != nil && t.Style == model.BlockContentText_Code {
			accum = append(accum, b)
		} else {
			if len(accum) > 0 {
				blocksOut = append(blocksOut, combineCodeBlocks(accum)...)
				accum = []*model.Block{}
			}

			blocksOut = append(blocksOut, b)
		}

	}

	if len(accum) > 0 {
		blocksOut = append(blocksOut, combineCodeBlocks(accum)...)
	}

	for _, b := range blocks {
		for i, cId := range b.ChildrenIds {
			if len(cId) == 0 {
				b.ChildrenIds = append(b.ChildrenIds[:i], b.ChildrenIds[i+1:]...)
			}
		}
	}

	return blocksOut
}

func combineCodeBlocks(accum []*model.Block) (res []*model.Block) {
	var (
		textArr         []string
		currLanguage    string
		resultCodeBlock []*model.Block
	)

	if len(accum) > 0 {
		currLanguage = pbtypes.GetString(accum[0].GetFields(), "lang")
	}
	for _, b := range accum {
		blockLanguage := pbtypes.GetString(b.GetFields(), "lang")
		if b.GetText() != nil && blockLanguage == currLanguage {
			textArr = append(textArr, b.GetText().Text)
			continue
		}
		if blockLanguage != currLanguage {
			resultCodeBlock = append(resultCodeBlock, provideCodeBlock(textArr, currLanguage))
			textArr = []string{b.GetText().Text}
			currLanguage = blockLanguage
		}
	}
	if len(textArr) > 0 {
		resultCodeBlock = append(resultCodeBlock, provideCodeBlock(textArr, currLanguage))
	}
	return resultCodeBlock
}

func provideCodeBlock(textArr []string, language string) *model.Block {
	return &model.Block{
		Fields: &types.Struct{Fields: map[string]*types.Value{"lang": pbtypes.String(language)}},
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  strings.Join(textArr, "\n"),
				Style: model.BlockContentText_Code,
				Marks: &model.BlockContentTextMarks{
					Marks: []*model.BlockContentTextMark{},
				},
			},
		},
	}
}
