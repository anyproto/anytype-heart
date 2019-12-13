package media

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
)

type ImageBlock interface {
	Block
}

func init() {
	simple.RegisterCreator(func(m *model.Block) simple.Block {
		return NewImage(m)
	})
}

func NewImage(m *model.Block) ImageBlock {
	return &Image{
		Base: base.NewBase(m).(*base.Base),
	}
}

type Image struct {
	*base.Base
	image *model.BlockContentImage
}
