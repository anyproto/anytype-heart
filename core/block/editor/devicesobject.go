package editor

import (
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
)

// required relations for device beside the bundle.RequiredInternalRelations
var deviceRequiredRelations = []domain.RelationKey{}

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
	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, deviceRequiredRelations...)
	if err = d.SmartBlock.Init(ctx); err != nil {
		return
	}
	d.AddHook(d.deviceService.SaveDeviceInfo, smartblock.HookAfterApply)
	return nil
}
