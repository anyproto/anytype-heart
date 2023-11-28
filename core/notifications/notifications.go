package notifications

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "notificationService"

type Notifications interface {
	app.Component
	CreateAndSendLocal(notification *model.Notification) error
	CreateAndSendCrossDevice(ctx context.Context, spaceID string, notification *model.Notification) error
	UpdateAndSend(notification *model.Notification) error
	Reply(contextID string, notificationID []string, notificationAction model.NotificationActionType) error
	List(limit int64, includeRead bool) ([]*model.Notification, error)
	IsNotificationRead(notification *model.Notification) bool
}

type notificationService struct {
	eventSender       event.Sender
	notificationStore objectstore.NotificationStore
}

func New() Notifications {
	return &notificationService{}
}

func (n *notificationService) Init(a *app.App) (err error) {
	n.notificationStore = app.MustComponent[objectstore.ObjectStore](a)
	n.eventSender = app.MustComponent[event.Sender](a)
	return nil
}

func (n *notificationService) Name() (name string) {
	return CName
}

func (n *notificationService) CreateAndSendLocal(notification *model.Notification) error {
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
		return fmt.Errorf("failed to add notification %s to cache: %w", notification.Id, err)
	}
	return nil
}

func (n *notificationService) CreateAndSendCrossDevice(ctx context.Context, spaceID string, notification *model.Notification) error {
	// TODO check if notification exist in notification object
	err := n.CreateAndSendLocal(notification)
	if err != nil {
		return err
	}
	return nil
}

func (n *notificationService) UpdateAndSend(notification *model.Notification) error {
	n.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfNotificationUpdate{
					NotificationUpdate: &pb.EventNotificationUpdate{
						Notification: notification,
					},
				},
			},
		},
	})
	err := n.notificationStore.SaveNotification(notification)
	if err != nil {
		return fmt.Errorf("failed to update notification %s: %s", notification.Id, err)
	}
	return nil
}

func (n *notificationService) Reply(contextID string, notificationIDs []string, notificationAction model.NotificationActionType) error {
	for _, id := range notificationIDs {
		status := model.Notification_Replied
		if notificationAction == model.Notification_CLOSE {
			status = model.Notification_Read
		}

		notification, err := n.notificationStore.GetNotificationByID(id)
		if err != nil {
			return err
		}
		notification.Status = status
		err = n.UpdateAndSend(notification)
		if err != nil {
			return fmt.Errorf("failed to update notification: %w", err)
		}
	}
	// TODO check notification in notification object and update it
	return nil
}

func (n *notificationService) List(limit int64, includeRead bool) ([]*model.Notification, error) {
	notifications, err := n.notificationStore.ListNotifications()
	if err != nil {
		return nil, fmt.Errorf("failed to list notifications: %w", err)
	}

	var (
		result   []*model.Notification
		addCount int64
	)
	for _, notification := range notifications {
		if addCount == limit {
			break
		}
		if n.IsNotificationRead(notification) && !includeRead {
			continue
		}
		result = append(result, notification)
		addCount++
	}
	return result, nil
}

func (n *notificationService) IsNotificationRead(notification *model.Notification) bool {
	return notification.GetStatus() == model.Notification_Read || notification.GetStatus() == model.Notification_Replied
}
