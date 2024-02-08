package notifications

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/spacecore"
)

var log = logging.Logger("notifications")

const CName = "notificationService"

type Notifications interface {
	app.ComponentRunnable
	CreateAndSend(notification *model.Notification) error
	UpdateAndSend(notification *model.Notification) error
	Reply(notificationIds []string, notificationAction model.NotificationActionType) error
	List(limit int64, includeRead bool) ([]*model.Notification, error)
}

type notificationService struct {
	notificationId     string
	notificationCancel context.CancelFunc
	eventSender        event.Sender
	notificationStore  NotificationStore
	spaceService       space.Service
	picker             block.ObjectGetter
	spaceCore          spacecore.SpaceCoreService

	sync.RWMutex
	lastNotificationIdToAcl map[string]string
}

func New() Notifications {
	return &notificationService{
		lastNotificationIdToAcl: make(map[string]string, 0),
	}
}

func (n *notificationService) Init(a *app.App) (err error) {
	datastoreService := app.MustComponent[datastore.Datastore](a)
	db, err := datastoreService.LocalStorage()
	if err != nil {
		return fmt.Errorf("failed to initialize notification store %w", err)
	}
	n.notificationStore = NewNotificationStore(db)
	n.eventSender = app.MustComponent[event.Sender](a)
	n.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
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
	return nil
}

func (n *notificationService) indexNotifications(ctx context.Context) {
	select {
	case <-ctx.Done():
		log.Errorf("failed to index notifications: %v", ctx.Err())
		return
	default:
		n.updateNotificationsInLocalStore()
	}
}

func (n *notificationService) updateNotificationsInLocalStore() {
	var notifications map[string]*model.Notification
	err := block.Do(n.picker, n.notificationId, func(sb smartblock.SmartBlock) error {
		s := sb.NewState()
		notifications = s.ListNotifications()
		return nil
	})
	if err != nil {
		log.Errorf("failed to get notifications from object: %s", err)
	}
	lastNotificationTimestamp := make(map[string]int64, 0)
	for _, notification := range notifications {
		err := n.notificationStore.SaveNotification(notification)
		if err != nil {
			log.Errorf("failed to save notification %s: %s", notification.Id, err)
		}
		if notification.Acl != "" && notification.GetCreateTime() > lastNotificationTimestamp[notification.Acl] {
			n.Lock()
			n.lastNotificationIdToAcl[notification.Acl] = notification.Id
			n.Unlock()
			lastNotificationTimestamp[notification.Acl] = notification.GetCreateTime()

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
	if notification.Id == "" {
		notification.Id = uuid.New().String()
	}
	notification.CreateTime = time.Now().Unix()
	if !notification.IsLocal {
		var exist bool
		err := block.DoState(n.picker, n.notificationId, func(s *state.State, sb smartblock.SmartBlock) error {
			stateNotification := s.GetNotificationById(notification.Id)
			if stateNotification != nil {
				exist = true
				return nil
			}
			s.AddNotification(notification)
			n.Lock()
			n.lastNotificationIdToAcl[notification.Acl] = notification.Id
			n.Unlock()
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

func (n *notificationService) Reply(notificationIds []string, notificationAction model.NotificationActionType) error {
	for _, id := range notificationIds {
		status := model.Notification_Replied
		if notificationAction == model.Notification_CLOSE {
			status = model.Notification_Read
		}

		notification, err := n.notificationStore.GetNotificationById(id)
		if err != nil {
			return err
		}
		notification.Status = status
		err = n.UpdateAndSend(notification)
		if err != nil {
			return fmt.Errorf("failed to update notification: %w", err)
		}

		if !notification.IsLocal {
			err = block.DoState(n.picker, n.notificationId, func(s *state.State, sb smartblock.SmartBlock) error {
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
		if !includeRead && n.isNotificationRead(notification) {
			continue
		}
		result = append(result, notification)
		addCount++
	}
	return result, nil
}

func (n *notificationService) GetLastNotificationId(acl string) string {
	n.RLock()
	defer n.RUnlock()
	return n.lastNotificationIdToAcl[acl]
}

func (n *notificationService) isNotificationRead(notification *model.Notification) bool {
	return notification.GetStatus() == model.Notification_Read || notification.GetStatus() == model.Notification_Replied
}

func (n *notificationService) loadNotificationObject(ctx context.Context) {
	uk, err := domain.NewUniqueKey(sb.SmartBlockTypeNotificationObject, "")
	if err != nil {
		log.Errorf("failed to get notification object unique key: %v", err)
		return
	}
	techSpaceID, err := n.spaceCore.DeriveID(ctx, spacecore.TechSpaceType)
	if err != nil {
		log.Errorf("failed to get personal space for notifications: %v", err)
		return
	}
	techSpace, err := n.spaceService.Get(ctx, techSpaceID)
	if err != nil {
		return
	}
	notificationObject, err := techSpace.DeriveTreeObject(ctx, objectcache.TreeDerivationParams{
		Key: uk,
		InitFunc: func(id string) *smartblock.InitContext {
			return &smartblock.InitContext{
				Ctx:     ctx,
				SpaceID: techSpace.Id(),
				State:   state.NewDoc(id, nil).(*state.State),
			}
		},
	})
	if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
		log.Errorf("failed to derive notification object: %v", err)
		return
	}
	if err == nil {
		n.notificationId = notificationObject.Id()
	}
	if errors.Is(err, treestorage.ErrTreeExists) {
		notificationID, err := techSpace.DeriveObjectID(ctx, uk)
		if err != nil {
			log.Errorf("failed to derive notification object id: %v", err)
			return
		}
		n.notificationId = notificationID
	}
	n.indexNotifications(ctx)
}
