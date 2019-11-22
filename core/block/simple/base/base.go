package base

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
)

func NewBase(block *model.Block) *Base {
	return &Base{Block: block}
}

type Base struct {
	*model.Block
}

func (s *Base) Virtual() bool {
	return false
}

func (s *Base) Model() *model.Block {
	return s.Block
}

func (s *Base) ApplyContentChanges(content model.IsBlockCoreContent) (err error) {
	s.Content = &model.BlockCore{Content: content}
	return
}
