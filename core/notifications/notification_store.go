package notifications

import (
	"context"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

type NotificationStore interface {
	SaveNotification(notification *model.Notification) error
	ListNotifications() ([]*model.Notification, error)
	GetNotificationById(notificationID string) (*model.Notification, error)
}

type notificationStore struct {
	db keyvaluestore.Store[*model.Notification]
}

func NewNotificationStore(db anystore.DB) (NotificationStore, error) {
	kv, err := keyvaluestore.New(db, "notifications", func(notification *model.Notification) ([]byte, error) {
		return proto.Marshal(notification)
	}, func(raw []byte) (*model.Notification, error) {
		n := &model.Notification{}
		err := proto.Unmarshal(raw, n)
		return n, err
	})
	if err != nil {
		return nil, fmt.Errorf("init store: %w", err)
	}
	return &notificationStore{db: kv}, nil
}

func (n *notificationStore) SaveNotification(notification *model.Notification) error {
	return n.db.Set(context.Background(), notification.Id, notification)
}

func (n *notificationStore) ListNotifications() ([]*model.Notification, error) {
	return n.db.ListAllValues(context.Background())
}

func (n *notificationStore) GetNotificationById(notificationId string) (*model.Notification, error) {
	return n.db.Get(context.Background(), notificationId)
}
