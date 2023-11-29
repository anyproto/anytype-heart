package notifications

import (
	"github.com/anyproto/any-sync/app"
	"github.com/dgraph-io/badger/v4"
	"github.com/gogo/protobuf/proto"
	ds "github.com/ipfs/go-datastore"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

const (
	notificationsPrefix   = "notifications"
	notificationStoreName = "notification_store"
)

var notificationsInfo = ds.NewKey("/" + notificationsPrefix + "/info")

type NotificationStore interface {
	app.Component
	SaveNotification(notification *model.Notification) error
	ListNotifications() ([]*model.Notification, error)
	GetNotificationByID(notificationID string) (*model.Notification, error)
}

type notificationStore struct {
	db *badger.DB
}

func NewNotificationStore() NotificationStore {
	return &notificationStore{}
}

func (n *notificationStore) Init(a *app.App) (err error) {
	datastoreService := app.MustComponent[datastore.Datastore](a)
	n.db, err = datastoreService.LocalStorage()
	return err
}

func (n *notificationStore) Name() (name string) {
	return notificationStoreName
}

func (n *notificationStore) SaveNotification(notification *model.Notification) error {
	return badgerhelper.SetValue(n.db, notificationsInfo.ChildString(notification.Id).Bytes(), notification)
}

func (n *notificationStore) ListNotifications() ([]*model.Notification, error) {
	return badgerhelper.ViewTxnWithResult(n.db, func(txn *badger.Txn) ([]*model.Notification, error) {
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

func (n *notificationStore) GetNotificationByID(notificationID string) (*model.Notification, error) {
	return badgerhelper.GetValue(n.db, notificationsInfo.ChildString(notificationID).Bytes(), unmarshalNotification)
}

func unmarshalNotification(raw []byte) (*model.Notification, error) {
	v := &model.Notification{}
	return v, proto.Unmarshal(raw, v)
}
