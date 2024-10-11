package block

import (
	"regexp"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	youtubeRegexp    = regexp.MustCompile(`https?:\/\/(?:www\.)?(?:youtube\.com\/(?:live\/)?|(?:youtu\.be\/))([^\?\n\s]+)`)
	soundCloudRegexp = regexp.MustCompile(`https?:\/\/(?:www\.)?(?:soundcloud\.com\/(?:[\w\d-]+\/(?:tracks|sets)\/[\w\d-]+|[\w\d-]+)|on\.soundcloud\.com\/[\w\d-]+)`)
	vimeoRegexp      = regexp.MustCompile(`https?:\/\/(?:www\.)?vimeo\.com\/(\d+)`)
)

type FileBlock struct {
	Block
	File    api.FileObject `json:"file"`
	Caption []api.RichText `json:"caption"`
}

func (f *FileBlock) GetBlocks(*api.NotionImportContext, string) *MapResponse {
	block, id := f.File.GetFileBlock(model.BlockContentFile_File)
	return &MapResponse{
		Blocks:   []*model.Block{block},
		BlockIDs: []string{id},
	}
}

type ImageBlock struct {
	Block
	File api.FileObject `json:"image"`
}

func (i *ImageBlock) GetBlocks(*api.NotionImportContext, string) *MapResponse {
	block, id := i.File.GetFileBlock(model.BlockContentFile_Image)
	return &MapResponse{
		Blocks:   []*model.Block{block},
		BlockIDs: []string{id},
	}
}

type PdfBlock struct {
	Block
	File api.FileObject `json:"pdf"`
}

func (p *PdfBlock) GetBlocks(*api.NotionImportContext, string) *MapResponse {
	block, id := p.File.GetFileBlock(model.BlockContentFile_PDF)
	return &MapResponse{
		Blocks:   []*model.Block{block},
		BlockIDs: []string{id},
	}
}

type VideoBlock struct {
	Block
	File api.FileObject `json:"video"`
}

func (v *VideoBlock) GetBlocks(*api.NotionImportContext, string) *MapResponse {
	if v.isEmbedBlock() {
		return v.provideEmbedBlock()
	}
	block, id := v.File.GetFileBlock(model.BlockContentFile_Video)
	return &MapResponse{
		Blocks:   []*model.Block{block},
		BlockIDs: []string{id},
	}
}
func (v *VideoBlock) provideEmbedBlock() *MapResponse {
	var processor model.BlockContentLatexProcessor
	if youtubeRegexp.MatchString(v.File.External.URL) {
		processor = model.BlockContentLatex_Youtube
	}
	if vimeoRegexp.MatchString(v.File.External.URL) {
		processor = model.BlockContentLatex_Vimeo
	}
	id := bson.NewObjectId().Hex()
	bl := &model.Block{
		Id:          id,
		ChildrenIds: []string{},
		Content: &model.BlockContentOfLatex{
			Latex: &model.BlockContentLatex{
				Text:      v.File.External.URL,
				Processor: processor,
			},
		},
	}
	return &MapResponse{
		Blocks:   []*model.Block{bl},
		BlockIDs: []string{id},
	}
}

func (v *VideoBlock) isEmbedBlock() bool {
	return youtubeRegexp.MatchString(v.File.External.URL) || vimeoRegexp.MatchString(v.File.External.URL)
}

type AudioBlock struct {
	Block
	File api.FileObject `json:"audio"`
}

func (a *AudioBlock) GetBlocks(*api.NotionImportContext, string) *MapResponse {
	if soundCloudRegexp.MatchString(a.File.External.URL) {
		id := bson.NewObjectId().Hex()
		bl := &model.Block{
			Id:          id,
			ChildrenIds: []string{},
			Content: &model.BlockContentOfLatex{
				Latex: &model.BlockContentLatex{
					Text:      a.File.External.URL,
					Processor: model.BlockContentLatex_Soundcloud,
				},
			},
		}
		return &MapResponse{
			Blocks:   []*model.Block{bl},
			BlockIDs: []string{id},
		}
	}
	block, id := a.File.GetFileBlock(model.BlockContentFile_Audio)
	return &MapResponse{
		Blocks:   []*model.Block{block},
		BlockIDs: []string{id},
	}
}
