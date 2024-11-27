package process

import (
	"sync"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("notification-process")

type NotificationService interface {
	CreateAndSend(notification *model.Notification) error
}

type NotificationSender interface {
	SendNotification()
}

type Notificationable interface {
	Progress
	FinishWithNotification(notification *model.Notification, err error)
}

type notificationProcess struct {
	*progress
	notificationService NotificationService

	lock         sync.Mutex
	notification *model.Notification
}

func NewNotificationProcess(processMessage pb.IsModelProcessMessage, notificationService NotificationService) Notificationable {
	return &notificationProcess{progress: &progress{
		id:             bson.NewObjectId().Hex(),
		done:           make(chan struct{}),
		cancel:         make(chan struct{}),
		processMessage: processMessage,
	}, notificationService: notificationService}
}

func (n *notificationProcess) FinishWithNotification(notification *model.Notification, err error) {
	n.setNotification(notification)
	n.Finish(err)
}

func (n *notificationProcess) SendNotification() {
	if notification := n.getNotification(); notification != nil {
		notificationSendErr := n.notificationService.CreateAndSend(notification)
		if notificationSendErr != nil {
			log.Errorf("failed to send notification: %v", notificationSendErr)
		}
	}
}

func (n *notificationProcess) setNotification(notification *model.Notification) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.notification = notification
}

func (n *notificationProcess) getNotification() *model.Notification {
	n.lock.Lock()
	defer n.lock.Unlock()
	return n.notification
}
