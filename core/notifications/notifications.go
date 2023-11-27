package notifications

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"

	//"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	//"github.com/anyproto/anytype-heart/core/block/editor/state"
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
	Reply(spaceID, notificationID string, notificationAction model.NotificationActionType) error
	List(limit int, includeRead bool) ([]*model.Notification, error)
	IsNotificationRead(notification *model.Notification) bool
}

type notificationService struct {
	eventSender       event.Sender
	notificationStore objectstore.NotificationStore
	//spaceService      space.Service
	//picker            block.ObjectGetter
}

func New() Notifications {
	return &notificationService{}
}

func (n *notificationService) Init(a *app.App) (err error) {
	n.notificationStore = app.MustComponent[objectstore.ObjectStore](a)
	n.eventSender = app.MustComponent[event.Sender](a)
	//n.spaceService = app.MustComponent[space.Service](a)
	//n.picker = app.MustComponent[block.ObjectGetter](a)
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
	// TODO check if notification exist in notification object, if so - check status
	//spc, err := n.spaceService.Get(ctx, spaceID)
	//if err != nil {
	//	return fmt.Errorf("failed to get space for notification: %w", err)
	//}
	//err = block.DoState(n.picker, spc.DerivedIDs().Notifications, func(s *state.State, sb smartblock.SmartBlock) error {
	//	s.AddNotification(notification)
	//	return nil
	//})
	//if err != nil {
	//	return fmt.Errorf("failed to update notification object: %w", err)
	//}
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

func (n *notificationService) Reply(contextID, notificationID string, notificationAction model.NotificationActionType) error {
	status := model.Notification_Replied
	if notificationAction == model.Notification_CLOSE {
		status = model.Notification_Read
	}

	notification, err := n.notificationStore.GetNotificationByID(notificationID)
	if err != nil {
		return err
	}
	notification.Status = status
	err = n.UpdateAndSend(notification)
	if err != nil {
		return fmt.Errorf("failed to update notification: %w", err)
	}
	//if !notification.IsLocal {
	//	err := block.DoState(n.picker, contextID, func(s *state.State, sb smartblock.SmartBlock) error {
	//		s.AddNotification(notification)
	//		return nil
	//	})
	//	if err != nil {
	//		return fmt.Errorf("failed to update notification object: %w", err)
	//	}
	//}
	return nil
}

func (n *notificationService) List(limit int, includeRead bool) ([]*model.Notification, error) {
	notifications, err := n.notificationStore.ListNotifications()
	if err != nil {
		return nil, fmt.Errorf("failed to list notifications: %w", err)
	}

	var (
		result   []*model.Notification
		addCount int
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
