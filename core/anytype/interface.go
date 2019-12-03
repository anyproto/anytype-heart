package anytype

import (
	"github.com/anytypeio/go-anytype-library/core"
)

type Anytype interface {
	GetBlock(id string) (Block, error)
	PredefinedBlockIds() core.PredefinedBlockIds
}

type Block interface {
	core.Block
	Close() error
}

type BlockVersion interface {
	core.BlockVersion
}
