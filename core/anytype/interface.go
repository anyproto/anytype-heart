package anytype

import (
	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
)

type Anytype interface {
	CreateBlock(content model.IsBlockContent) (core.Block, error)
	GetBlock(id string) (core.Block, error)
}

type Block interface {
	core.Block
}

type BlockVersion interface {
	core.BlockVersion
}
