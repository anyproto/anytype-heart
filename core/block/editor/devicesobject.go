package editor

import (
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
)

type DevicesObject struct {
	smartblock.SmartBlock
	deviceService deviceService
}

func NewDevicesObject(sb smartblock.SmartBlock, deviceService deviceService) *DevicesObject {
	return &DevicesObject{
		SmartBlock:    sb,
		deviceService: deviceService,
	}
}

func (d *DevicesObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = d.SmartBlock.Init(ctx); err != nil {
		return
	}
	d.AddHook(d.deviceService.SaveDeviceInfo, smartblock.HookAfterApply)
	return nil
}
