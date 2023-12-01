package notifications

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/peer"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("notifications")

const CName = "notificationService"

type Notifications interface {
	app.ComponentRunnable
	CreateAndSend(notification *model.Notification) error
	UpdateAndSend(notification *model.Notification) error
	Reply(notificationID []string, notificationAction model.NotificationActionType) error
	List(limit int64, includeRead bool) ([]*model.Notification, error)
}

type notificationService struct {
	notificationID     string
	notificationCh     chan struct{}
	notificationErr    error
	notificationCancel context.CancelFunc
	eventSender        event.Sender
	notificationStore  NotificationStore
	spaceService       space.Service
	picker             block.ObjectGetter
}

func New() Notifications {
	return &notificationService{
		notificationCh: make(chan struct{}),
	}
}

func (n *notificationService) Init(a *app.App) (err error) {
	n.notificationStore = app.MustComponent[NotificationStore](a)
	n.eventSender = app.MustComponent[event.Sender](a)
	n.spaceService = app.MustComponent[space.Service](a)
	n.picker = app.MustComponent[block.ObjectGetter](a)
	return nil
}

func (n *notificationService) Name() (name string) {
	return CName
}

func (n *notificationService) Run(_ context.Context) (err error) {
	notificationContext, notificationCancel := context.WithCancel(context.Background())
	n.notificationCancel = notificationCancel
	go n.loadNotificationObject(notificationContext)
	go n.indexNotifications(notificationContext)
	return nil
}

func (n *notificationService) indexNotifications(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			log.Errorf("failed to index notifications: %v", ctx.Err())
			return
		case <-n.notificationCh:
			if n.notificationErr != nil {
				log.Errorf("failed to get notification object: %v", n.notificationErr)
				return
			}
			n.updateNotificationsInLocalStore()
			return
		}
	}
}

func (n *notificationService) updateNotificationsInLocalStore() {
	var notifications map[string]*model.Notification
	err := block.DoState(n.picker, n.notificationID, func(s *state.State, sb smartblock.SmartBlock) error {
		notifications = s.ListNotifications()
		return nil
	})
	if err != nil {
		log.Errorf("failed to get notifications from object: %s", err)
	}
	for _, notification := range notifications {
		err := n.notificationStore.SaveNotification(notification)
		if err != nil {
			log.Errorf("failed to save notification %s: %s", notification.Id, err)
		}
	}
}

func (n *notificationService) Close(_ context.Context) (err error) {
	if n.notificationCancel != nil {
		n.notificationCancel()
	}
	return nil
}

func (n *notificationService) CreateAndSend(notification *model.Notification) error {
	if !notification.IsLocal {
		var exist bool
		err := block.DoState(n.picker, n.notificationID, func(s *state.State, sb smartblock.SmartBlock) error {
			stateNotification := s.GetNotificationByID(notification.Id)
			if stateNotification != nil {
				exist = true
				return nil
			}
			s.AddNotification(notification)
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to update notification object: %w", err)
		}
		if exist {
			return nil
		}
	}
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
		return fmt.Errorf("failed to update notification %s: %w", notification.Id, err)
	}
	return nil
}

func (n *notificationService) Reply(notificationIDs []string, notificationAction model.NotificationActionType) error {
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

		if !notification.IsLocal {
			err = block.DoState(n.picker, n.notificationID, func(s *state.State, sb smartblock.SmartBlock) error {
				s.AddNotification(notification)
				return nil
			})
			if err != nil {
				return fmt.Errorf("failed to update notification object: %w", err)
			}
		}
	}
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
		if n.isNotificationRead(notification) && !includeRead {
			continue
		}
		result = append(result, notification)
		addCount++
	}
	return result, nil
}

func (n *notificationService) isNotificationRead(notification *model.Notification) bool {
	return notification.GetStatus() == model.Notification_Read || notification.GetStatus() == model.Notification_Replied
}

func (n *notificationService) loadNotificationObject(ctx context.Context) {
	defer close(n.notificationCh)
	uk, err := domain.NewUniqueKey(sb.SmartBlockTypeNotificationObject, "")
	if err != nil {
		n.notificationErr = err
		return
	}
	spc, err := n.spaceService.GetPersonalSpace(ctx)
	if err != nil {
		n.notificationErr = err
		return
	}
	n.notificationID, err = spc.DeriveObjectID(ctx, uk)
	if err != nil {
		n.notificationErr = err
		return
	}
	ctxWithPeer := peer.CtxWithPeerId(ctx, peer.CtxResponsiblePeers)
	_, err = spc.GetObject(ctxWithPeer, n.notificationID)
	if err != nil {
		_, dErr := spc.DeriveTreeObject(ctx, objectcache.TreeDerivationParams{
			Key: uk,
			InitFunc: func(id string) *smartblock.InitContext {
				return &smartblock.InitContext{
					Ctx:     ctx,
					SpaceID: spc.Id(),
					State:   state.NewDoc(id, nil).(*state.State),
				}
			},
		})
		if dErr != nil {
			n.notificationErr = dErr
			return
		}
	}
}
