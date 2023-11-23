package editor

import "github.com/anyproto/anytype-heart/core/block/editor/smartblock"

type NotificationObject struct {
	smartblock.SmartBlock
}

func NewNotificationObject(sb smartblock.SmartBlock) *NotificationObject {
	return &NotificationObject{
		SmartBlock: sb,
	}
}

func (m *NotificationObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = m.SmartBlock.Init(ctx); err != nil {
		return
	}
	// TODO hook after apply - send events to clients
	return nil
}
