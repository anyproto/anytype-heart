package editor

import (
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
)

func NewMissingObject(sb smartblock.SmartBlock) *MissingObject {
	return &MissingObject{
		SmartBlock: sb,
	}
}

type MissingObject struct {
	smartblock.SmartBlock
}

func (m *MissingObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = m.SmartBlock.Init(ctx); err != nil {
		return
	}
	return nil
}
