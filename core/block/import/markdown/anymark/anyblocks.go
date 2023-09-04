package anymark

import (
	"path/filepath"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func preprocessBlocks(blocks []*model.Block) (blocksOut []*model.Block) {
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
	for _, b := range blocks {
		for i, cId := range b.ChildrenIds {
			if cId == "" {
				b.ChildrenIds = append(b.ChildrenIds[:i], b.ChildrenIds[i+1:]...)
			}
		}
	}
	return blocksOut
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

func ConvertTextToFile(block *model.Block) {
	// "svg" excluded
	if block.GetText().GetMarks().Marks[0].Param == "" {
		return
	}

	imageFormats := []string{"jpg", "jpeg", "png", "gif", "webp"}
	videoFormats := []string{"mp4", "m4v"}
	audioFormats := []string{"mp3", "ogg", "wav", "m4a", "flac"}
	pdfFormat := "pdf"

	fileType := model.BlockContentFile_File
	fileExt := filepath.Ext(block.GetText().GetMarks().Marks[0].Param)
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

		block.Content = &model.BlockContentOfFile{
			File: &model.BlockContentFile{
				Name:  block.GetText().GetMarks().Marks[0].Param,
				State: model.BlockContentFile_Empty,
				Type:  fileType,
			},
		}
	}
}
