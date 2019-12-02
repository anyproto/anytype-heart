package base

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
)

func NewVirtual(m *model.Block) simple.Block {
	return &Virtual{Block: NewBase(m)}
}

type Virtual struct {
	simple.Block
}

func (v *Virtual) Virtual() bool {
	return true
}
