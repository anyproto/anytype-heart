package anymark

import (
	"strings"

	"github.com/globalsign/mgo/bson"
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
				result, possibleChildIDs := combineCodeBlocks(accum)
				blocksOut = append(blocksOut, result...)
				addNewChildrenIDs(blocks, possibleChildIDs)
				accum = []*model.Block{}
			}

			blocksOut = append(blocksOut, b)
		}

	}

	if len(accum) > 0 {
		result, possibleChildIDs := combineCodeBlocks(accum)
		blocksOut = append(blocksOut, result...)
		addNewChildrenIDs(blocks, possibleChildIDs)
	}

	for _, b := range blocks {
		for i, cId := range b.ChildrenIds {
			if cId == "" {
				b.ChildrenIds = append(b.ChildrenIds[:i], b.ChildrenIds[i+1:]...)
			}
		}
	}

	return blocksOut
}

func addNewChildrenIDs(blocks []*model.Block, possibleChildIDs map[string]string) {
	for _, block := range blocks {
		for _, id := range block.ChildrenIds {
			if newID, ok := possibleChildIDs[id]; ok {
				block.ChildrenIds = append(block.ChildrenIds, newID)
			}
		}
	}
}

func combineCodeBlocks(accum []*model.Block) ([]*model.Block, map[string]string) {
	var (
		textArr          []string
		currLanguage     string
		resultCodeBlock  []*model.Block
		currBlockOldID   string
		possibleChildIDs = make(map[string]string, 0)
	)

	if len(accum) > 0 {
		currLanguage = pbtypes.GetString(accum[0].GetFields(), "lang")
		currBlockOldID = accum[0].Id
	}
	for _, b := range accum {
		blockLanguage := pbtypes.GetString(b.GetFields(), "lang")
		if b.GetText() != nil && blockLanguage == currLanguage {
			textArr = append(textArr, b.GetText().Text)
			continue
		}
		if blockLanguage != currLanguage {
			codeBlock := provideCodeBlock(textArr, currLanguage)
			resultCodeBlock = append(resultCodeBlock, codeBlock)
			possibleChildIDs[currBlockOldID] = codeBlock.Id
			textArr = []string{b.GetText().Text}
			currLanguage = blockLanguage
			currBlockOldID = b.Id
		}
	}
	if len(textArr) > 0 {
		codeBlock := provideCodeBlock(textArr, currLanguage)
		resultCodeBlock = append(resultCodeBlock, codeBlock)
		possibleChildIDs[currBlockOldID] = codeBlock.Id
	}
	return resultCodeBlock, possibleChildIDs
}

func provideCodeBlock(textArr []string, language string) *model.Block {
	var field *types.Struct
	if language != "" {
		field = &types.Struct{Fields: map[string]*types.Value{"lang": pbtypes.String(language)}}
	}
	return &model.Block{
		Id:     bson.NewObjectId().Hex(),
		Fields: field,
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
