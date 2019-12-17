package anytype

import (
	"io"

	"github.com/anytypeio/go-anytype-library/core"
)

type Anytype interface {
	GetBlock(id string) (Block, error)
	PredefinedBlockIds() core.PredefinedBlockIds
	FileAddWithReader(content io.Reader, media string, name string) (*core.File, error)
	FileByHash(hash string) (*core.File, error)
}

type Block interface {
	core.Block
	Close() error
}

type BlockVersion interface {
	core.BlockVersion
}
