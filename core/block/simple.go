package block

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
)

type simple interface {
	Virtual() bool
	Model() *model.Block
}

type simpleBlock struct {
	*model.Block
}

func (s simpleBlock) Virtual() bool {
	return false
}

func (s simpleBlock) Model() *model.Block {
	return s.Block
}

type virtualBlock struct {
	*model.Block
}

func (v virtualBlock) Virtual() bool {
	return true
}

func (v virtualBlock) Model() *model.Block {
	return v.Block
}
