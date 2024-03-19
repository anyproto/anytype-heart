package editor

import (
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
)

type DeviceObject struct {
	smartblock.SmartBlock
}

func NewDeviceObject(sb smartblock.SmartBlock) *NotificationObject {
	return &NotificationObject{
		SmartBlock: sb,
	}
}

func (d *DeviceObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = d.SmartBlock.Init(ctx); err != nil {
		return
	}
	return nil
}
