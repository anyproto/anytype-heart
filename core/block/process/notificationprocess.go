package process

import (
	"github.com/anyproto/anytype-heart/core/notifications"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("notification-process")

type Notificationable interface {
	SendNotification()
	SetNotification(notification *model.Notification)
}

type NotificationProcess struct {
	Progress
	notification        *model.Notification
	notificationService notifications.Notifications
}

func NewNotificationProcess(pbType pb.ModelProcessType, notificationService notifications.Notifications) *NotificationProcess {
	return &NotificationProcess{Progress: NewProgress(pbType), notificationService: notificationService}
}

func (n *NotificationProcess) SetNotification(notification *model.Notification) {
	n.notification = notification
}

func (n *NotificationProcess) SendNotification() {
	if n.notification != nil {
		notificationSendErr := n.notificationService.CreateAndSendLocal(n.notification)
		if notificationSendErr != nil {
			log.Errorf("failed to send notification: %v", notificationSendErr)
		}
	}
}
