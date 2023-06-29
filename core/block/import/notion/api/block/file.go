package block

import (
	"github.com/anyproto/anytype-heart/core/block/import/notion/api"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type FileBlock struct {
	Block
	File    api.FileObject `json:"file"`
	Caption []api.RichText `json:"caption"`
}

func (f *FileBlock) GetBlocks(*NotionImportContext, string) *MapResponse {
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

func (i *ImageBlock) GetBlocks(*NotionImportContext, string) *MapResponse {
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

func (p *PdfBlock) GetBlocks(*NotionImportContext, string) *MapResponse {
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

func (p *VideoBlock) GetBlocks(*NotionImportContext, string) *MapResponse {
	block, id := p.File.GetFileBlock(model.BlockContentFile_Video)
	return &MapResponse{
		Blocks:   []*model.Block{block},
		BlockIDs: []string{id},
	}
}

type AudioBlock struct {
	Block
	File api.FileObject `json:"audio"`
}

func (p *AudioBlock) GetBlocks(*NotionImportContext, string) *MapResponse {
	block, id := p.File.GetFileBlock(model.BlockContentFile_Audio)
	return &MapResponse{
		Blocks:   []*model.Block{block},
		BlockIDs: []string{id},
	}
}
