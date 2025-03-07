package notifications

import (
	"github.com/dgraph-io/badger/v4"
	ds "github.com/ipfs/go-datastore"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

const notificationsPrefix = "notifications"

var notificationsInfo = ds.NewKey("/" + notificationsPrefix + "/info")

type NotificationStore interface {
	SaveNotification(notification *model.Notification) error
	ListNotifications() ([]*model.Notification, error)
	GetNotificationById(notificationID string) (*model.Notification, error)
}

type notificationStore struct {
	db *badger.DB
}

func NewNotificationStore(db *badger.DB) NotificationStore {
	return &notificationStore{db: db}
}

func (n *notificationStore) SaveNotification(notification *model.Notification) error {
	return badgerhelper.SetValue(n.db, notificationsInfo.ChildString(notification.Id).Bytes(), notification)
}

func (n *notificationStore) ListNotifications() ([]*model.Notification, error) {
	return badgerhelper.ViewTxnWithResult(n.db, func(txn *badger.Txn) ([]*model.Notification, error) {
		keys := localstore.GetKeys(txn, notificationsInfo.String(), 0)

		notificationsIds, err := localstore.GetLeavesFromResults(keys)
		if err != nil {
			return nil, err
		}

		notifications := make([]*model.Notification, 0, len(notificationsIds))
		for _, id := range notificationsIds {
			notificationInfo := notificationsInfo.ChildString(id)
			notification, err := badgerhelper.GetValueTxn(txn, notificationInfo.Bytes(), unmarshalNotification)
			if badgerhelper.IsNotFound(err) {
				continue
			}
			notifications = append(notifications, notification)
		}

		return notifications, nil
	})
}

func (n *notificationStore) GetNotificationById(notificationId string) (*model.Notification, error) {
	return badgerhelper.GetValue(n.db, notificationsInfo.ChildString(notificationId).Bytes(), unmarshalNotification)
}

func unmarshalNotification(raw []byte) (*model.Notification, error) {
	v := &model.Notification{}
	return v, v.UnmarshalVT(raw)
}
