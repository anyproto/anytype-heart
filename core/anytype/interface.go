package anytype

import (
	"github.com/anytypeio/go-anytype-library/core"
)

type Anytype interface {
	GetBlock(id string) (core.Block, error)
	PredefinedBlockIds() core.PredefinedBlockIds
}

type Block interface {
	core.Block
}

type BlockVersion interface {
	core.BlockVersion
}
