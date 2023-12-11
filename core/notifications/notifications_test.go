package notifications

import (
	"path/filepath"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TODO: Right now we use only CLOSE action type in protocol, so tests should be improved when we have more types
const notCloseActionType = 1

func TestNotificationService_List(t *testing.T) {
	t.Run("no notification in store - empty result", func(t *testing.T) {
		//given
		sender := mock_event.NewMockSender(t)
		notifications := notificationService{
			eventSender:       sender,
			notificationStore: NewTestStore(t),
		}

		// when
		list, err := notifications.List(100, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, list, 0)
	})
	t.Run("limit = 0 - empty result", func(t *testing.T) {
		//given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Created})
		assert.Nil(t, err)
		err = storeFixture.SaveNotification(&model.Notification{Id: "id1", Status: model.Notification_Created})
		assert.Nil(t, err)
		sender := mock_event.NewMockSender(t)
		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
		}

		// when
		list, err := notifications.List(0, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, list, 0)
	})
	t.Run("all notification in store are read and option includeRead=false - empty result", func(t *testing.T) {
		//given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Read})
		assert.Nil(t, err)
		err = storeFixture.SaveNotification(&model.Notification{Id: "id1", Status: model.Notification_Read})
		assert.Nil(t, err)
		sender := mock_event.NewMockSender(t)
		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
		}

		// when
		list, err := notifications.List(10, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, list, 0)
	})
	t.Run("all notification in store are read and option includeRead=true - return all notification", func(t *testing.T) {
		//given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Read})
		assert.Nil(t, err)
		err = storeFixture.SaveNotification(&model.Notification{Id: "id1", Status: model.Notification_Read})
		assert.Nil(t, err)
		sender := mock_event.NewMockSender(t)
		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
		}

		// when
		list, err := notifications.List(10, true)

		// then
		assert.Nil(t, err)
		assert.Len(t, list, 2)
	})
	t.Run("1 notification in store read and 1 not read, includeRead=false - 1 notification in result", func(t *testing.T) {
		//given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Replied})
		assert.Nil(t, err)
		err = storeFixture.SaveNotification(&model.Notification{Id: "id1", Status: model.Notification_Created})
		assert.Nil(t, err)
		sender := mock_event.NewMockSender(t)
		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
		}

		// when
		list, err := notifications.List(10, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, list, 1)
	})
}

func TestNotificationService_Reply(t *testing.T) {
	t.Run("action = close - status == read", func(t *testing.T) {
		//given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Created})
		assert.Nil(t, err)

		sender := mock_event.NewMockSender(t)
		sender.EXPECT().Broadcast(mock.Anything).Return().Times(1)

		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
		}

		// when
		err = notifications.Reply([]string{"id"}, model.Notification_CLOSE)
		assert.Nil(t, err)
		notification, err := storeFixture.GetNotificationById("id")
		assert.Nil(t, err)

		// then
		assert.Equal(t, model.Notification_Read, notification.Status)
	})
	t.Run("action != close - status == replied", func(t *testing.T) {
		//given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Created})
		assert.Nil(t, err)

		sender := mock_event.NewMockSender(t)
		sender.EXPECT().Broadcast(mock.Anything).Return().Times(1)

		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
		}

		// when
		err = notifications.Reply([]string{"id"}, notCloseActionType)
		assert.Nil(t, err)
		notification, err := storeFixture.GetNotificationById("id")
		assert.Nil(t, err)

		// then
		assert.Equal(t, model.Notification_Replied, notification.Status)
	})

	t.Run("close multiple notifications", func(t *testing.T) {
		//given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Created})
		assert.Nil(t, err)
		err = storeFixture.SaveNotification(&model.Notification{Id: "id1", Status: model.Notification_Created})
		assert.Nil(t, err)

		sender := mock_event.NewMockSender(t)
		sender.EXPECT().Broadcast(mock.Anything).Return().Times(2)

		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
		}

		// when
		err = notifications.Reply([]string{"id", "id1"}, notCloseActionType)
		assert.Nil(t, err)

		// then
		notification, err := storeFixture.GetNotificationById("id")
		assert.Nil(t, err)
		assert.Equal(t, model.Notification_Replied, notification.Status)

		notification, err = storeFixture.GetNotificationById("id1")
		assert.Nil(t, err)
		assert.Equal(t, model.Notification_Replied, notification.Status)
	})
}

func NewTestStore(t *testing.T) NotificationStore {
	db, err := badger.Open(badger.DefaultOptions(filepath.Join(t.TempDir(), "badger")))
	require.NoError(t, err)
	return &notificationStore{
		db: db,
	}
}
