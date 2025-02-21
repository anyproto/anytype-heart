package anymark

import (
	"path/filepath"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func preprocessBlocks(blocks []*model.Block) (blocksOut []*model.Block, rootBlockIDs []string) {
	accum := make([]*model.Block, 0)
	for _, b := range blocks {
		if t := b.GetText(); t != nil && t.Style == model.BlockContentText_Code {
			accum = append(accum, b)
		} else {
			if len(accum) > 0 {
				result := combineCodeBlocks(accum)
				blocksOut = append(blocksOut, result...)
				accum = []*model.Block{}
			}
			blocksOut = append(blocksOut, b)
		}

	}
	if len(accum) > 0 {
		result := combineCodeBlocks(accum)
		blocksOut = append(blocksOut, result...)
	}
	var blockHasParent = make(map[string]struct{})
	for _, b := range blocks {
		for i, cId := range b.ChildrenIds {
			blockHasParent[cId] = struct{}{}
			if cId == "" {
				b.ChildrenIds = append(b.ChildrenIds[:i], b.ChildrenIds[i+1:]...)
			}
		}
	}
	for _, b := range blocks {
		if _, ok := blockHasParent[b.Id]; !ok {
			rootBlockIDs = append(rootBlockIDs, b.Id)
		}
	}

	return blocksOut, rootBlockIDs
}

func combineCodeBlocks(accum []*model.Block) []*model.Block {
	var (
		textArr          []string
		currLanguage     string
		resultCodeBlocks []*model.Block
		currBlockID      string
	)

	if len(accum) > 0 {
		currLanguage = pbtypes.GetString(accum[0].GetFields(), "lang")
		currBlockID = accum[0].Id
	}
	for _, b := range accum {
		blockLanguage := pbtypes.GetString(b.GetFields(), "lang")
		if b.GetText() != nil && blockLanguage == currLanguage {
			textArr = append(textArr, b.GetText().Text)
			continue
		}
		if blockLanguage != currLanguage {
			codeBlock := provideCodeBlock(textArr, currLanguage, currBlockID)
			resultCodeBlocks = append(resultCodeBlocks, codeBlock)
			textArr = []string{b.GetText().Text}
			currLanguage = blockLanguage
			currBlockID = b.Id
		}
	}
	if len(textArr) > 0 {
		codeBlock := provideCodeBlock(textArr, currLanguage, currBlockID)
		resultCodeBlocks = append(resultCodeBlocks, codeBlock)
	}
	return resultCodeBlocks
}

func provideCodeBlock(textArr []string, language string, id string) *model.Block {
	var field *types.Struct
	if language != "" {
		field = &types.Struct{Fields: map[string]*types.Value{"lang": pbtypes.String(language)}}
	}
	return &model.Block{
		Id:     id,
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

func ConvertTextToFile(filePath string) *model.BlockContentOfFile {
	// "svg" excluded
	if filePath == "" {
		return nil
	}

	imageFormats := []string{"jpg", "jpeg", "png", "gif", "webp"}
	videoFormats := []string{"mp4", "m4v", "mov"}
	audioFormats := []string{"mp3", "ogg", "wav", "m4a", "flac"}
	pdfFormat := "pdf"

	fileType := model.BlockContentFile_File
	fileExt := filepath.Ext(filePath)
	if fileExt != "" {
		fileExt = fileExt[1:]
		for _, ext := range imageFormats {
			if strings.EqualFold(fileExt, ext) {
				fileType = model.BlockContentFile_Image
				break
			}
		}

		for _, ext := range videoFormats {
			if strings.EqualFold(fileExt, ext) {
				fileType = model.BlockContentFile_Video
				break
			}
		}

		for _, ext := range audioFormats {
			if strings.EqualFold(fileExt, ext) {
				fileType = model.BlockContentFile_Audio
				break
			}
		}

		if strings.EqualFold(fileExt, pdfFormat) {
			fileType = model.BlockContentFile_PDF
		}
	}
	return &model.BlockContentOfFile{
		File: &model.BlockContentFile{
			Name:  filePath,
			State: model.BlockContentFile_Empty,
			Type:  fileType,
		},
	}
}

func AddRootBlock(blocks []*model.Block, rootBlockID string) []*model.Block {
	for i, b := range blocks {
		if _, ok := b.Content.(*model.BlockContentOfSmartblock); ok {
			blocks[i].Id = rootBlockID
			return blocks
		}
	}
	notRootBlockChild := make(map[string]bool, 0)
	for _, b := range blocks {
		for _, id := range b.ChildrenIds {
			notRootBlockChild[id] = true
		}
	}
	childrenIds := make([]string, 0)
	for _, b := range blocks {
		if _, ok := notRootBlockChild[b.Id]; !ok {
			childrenIds = append(childrenIds, b.Id)
		}
	}
	blocks = append(blocks, &model.Block{
		Id: rootBlockID,
		Content: &model.BlockContentOfSmartblock{
			Smartblock: &model.BlockContentSmartblock{},
		},
		ChildrenIds: childrenIds,
	})
	return blocks
}
