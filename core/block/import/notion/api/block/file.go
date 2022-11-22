package block

import "github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"

type FileBlock struct {
	Block
	File    api.FileObject `json:"file"`
	Caption []api.RichText `json:"caption"`
}

type ImageBlock struct {
	Block
	File api.FileObject `json:"image"`
}

type PdfBlock struct {
	Block
	File api.FileObject `json:"pdf"`
}

type VideoBlock struct {
	Block
	File api.FileObject `json:"video"`
}
