package base

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
)

func NewVirtual(block *model.Block) *Virtual {
	return &Virtual{Block: block}
}

type Virtual struct {
	*model.Block
}

func (v *Virtual) Virtual() bool {
	return true
}

func (v *Virtual) Model() *model.Block {
	return v.Block
}
