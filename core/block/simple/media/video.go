package media

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
)

type VideoBlock interface {
	Block
}

func init() {
	simple.RegisterCreator(func(m *model.Block) simple.Block {
		return NewVideo(m)
	})
}

func NewVideo(m *model.Block) VideoBlock {
	return &Video{
		Base: base.NewBase(m).(*base.Base),
	}
}

type Video struct {
	*base.Base
	content *model.BlockContentImage
}
