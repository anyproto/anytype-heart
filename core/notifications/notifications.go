package notifications

import (
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "notificationService"

type Notification interface {
	app.Component
	Send(notification *model.Notification) error
	Update(notification *model.Notification) error
	List(limit int, includeRead bool) ([]*model.Notification, error)
}

type NotificationService struct {
	eventSender       event.Sender
	notificationStore objectstore.NotificationStore
}

func New() Notification {
	return &NotificationService{}
}

func (n NotificationService) Init(a *app.App) (err error) {
	n.notificationStore = app.MustComponent[objectstore.ObjectStore](a)
	n.eventSender = app.MustComponent[event.Sender](a)
	return nil
}

func (n NotificationService) Name() (name string) {
	return CName
}

func (n NotificationService) Send(notification *model.Notification) error {
	n.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfNotificationSend{
					NotificationSend: &pb.EventNotificationSend{
						Notification: notification,
					},
				},
			},
		},
	})
	err := n.notificationStore.SaveNotification(notification)
	if err != nil {
		return fmt.Errorf("failed to add notification %s to cache: %s", notification.Id, err)
	}
	return nil
}

func (n NotificationService) Update(notification *model.Notification) error {
	panic("implement me")
}

func (n NotificationService) List(limit int, includeRead bool) ([]*model.Notification, error) {
	notifications, err := n.notificationStore.ListNotifications()
	if err != nil {
		return nil, fmt.Errorf("failed to list notifications: %s", err)
	}

	var (
		result   []*model.Notification
		addCount int
	)
	for _, notification := range notifications {
		if addCount == limit {
			break
		}
		if n.isNotificationRead(notification) && !includeRead {
			continue
		}
		result = append(result, notification)
		addCount++
	}
	return result, nil
}

func (n NotificationService) isNotificationRead(notification *model.Notification) bool {
	return notification.GetStatus() == model.Notification_Read || notification.GetStatus() == model.Notification_Replied
}
