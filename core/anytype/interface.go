package anytype

import (
	"io"

	"github.com/anytypeio/go-anytype-library/core"
)

type Anytype interface {
	GetBlock(id string) (Block, error)
	PredefinedBlockIds() core.PredefinedBlockIds
	FileAddWithReader(content io.Reader, name string) (core.File, error)
	ImageAddWithReader(content io.Reader, name string) (core.Image, error)
	FileByHash(hash string) (core.File, error)
}

type Block interface {
	core.Block
	Close() error
}

type BlockVersion interface {
	core.BlockVersion
}

type File interface {
	core.File
}

type Image interface {
	core.Image
}
