package base

import (
	"fmt"

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

func (v *Virtual) ApplyContentChanges(content model.IsBlockCoreContent) (err error) {
	return fmt.Errorf("can't change virtual block")
}
