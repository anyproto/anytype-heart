package block

import "github.com/anytypeio/go-anytype-middleware/core/block/import/notion/api"

type Image struct {
	Caption  []api.RichText  `json:"caption,omitempty"`
	Type     api.FileType    `json:"type"`
	File     *api.FileObject `json:"file,omitempty"`
	External *api.FileObject `json:"external,omitempty"`
}