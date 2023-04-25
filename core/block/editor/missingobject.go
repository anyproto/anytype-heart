package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
)

func NewMissingObject() *MissingObject {
	return &MissingObject{
		SmartBlock: smartblock.New(),
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
