package editor

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/notifications"
)

type NotificationObject struct {
	notificationService notifications.Notifications
	smartblock.SmartBlock
}

func NewNotificationObject(sb smartblock.SmartBlock, notificationService notifications.Notifications) *NotificationObject {
	return &NotificationObject{
		notificationService: notificationService,
		SmartBlock:          sb,
	}
}

func (n *NotificationObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = n.SmartBlock.Init(ctx); err != nil {
		return
	}
	n.AddHook(n.onNotificationChange, smartblock.HookAfterApply)
	return nil
}

func (n *NotificationObject) onNotificationChange(info smartblock.ApplyInfo) (err error) {
	state := n.NewState()
	for _, change := range info.Changes {
		if notificationChange := change.GetNotificationUpdate(); notificationChange != nil {
			notification := state.GetNotificationByID(notificationChange.Id)
			if notification == nil {
				continue
			}
			err := n.notificationService.UpdateAndSend(notification)
			if err != nil {
				return fmt.Errorf("failed to send notification after state apply: %s", err)
			}
		}
	}
	return nil
}
