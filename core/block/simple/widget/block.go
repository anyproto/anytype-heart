package widget

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func init() {
	simple.RegisterCreator(NewBlock)
}

func NewBlock(b *model.Block) simple.Block {
	if w := b.GetWidget(); w != nil {
		return &block{
			Base: base.NewBase(b).(*base.Base),
		}
	}
	return nil
}

type Block interface {
	simple.Block
}

type block struct {
	*base.Base
}

func (b *block) Copy() simple.Block {
	return NewBlock(pbtypes.CopyBlock(b.Model()))
}
