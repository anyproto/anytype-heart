package objectstore

import (
	"github.com/dgraph-io/badger/v4"
	"github.com/gogo/protobuf/proto"
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
	GetNotificationByID(notificationID string) (*model.Notification, error)
}

func (d *dsObjectStore) SaveNotification(notification *model.Notification) error {
	return badgerhelper.SetValue(d.db, notificationsInfo.ChildString(notification.Id).Bytes(), notification)
}

func (d *dsObjectStore) ListNotifications() ([]*model.Notification, error) {
	return badgerhelper.ViewTxnWithResult(d.db, func(txn *badger.Txn) ([]*model.Notification, error) {
		keys := localstore.GetKeys(txn, notificationsInfo.String(), 0)

		notificationsIDs, err := localstore.GetLeavesFromResults(keys)
		if err != nil {
			return nil, err
		}

		notifications := make([]*model.Notification, 0, len(notificationsIDs))
		for _, id := range notificationsIDs {
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

func (d *dsObjectStore) GetNotificationByID(notificationID string) (*model.Notification, error) {
	return badgerhelper.GetValue(d.db, notificationsInfo.ChildString(notificationID).Bytes(), unmarshalNotification)
}

func unmarshalNotification(raw []byte) (*model.Notification, error) {
	v := &model.Notification{}
	return v, proto.Unmarshal(raw, v)
}
