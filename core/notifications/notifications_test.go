package notifications

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestNotificationService_List(t *testing.T) {
	t.Run("no notification in store - empty result", func(t *testing.T) {
		//given
		storeFixture := objectstore.NewStoreFixture(t)
		sender := mock_event.NewMockSender(t)
		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
		}

		// when
		list, err := notifications.List(100, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, list, 0)
	})
	t.Run("limit = 0 - empty result", func(t *testing.T) {
		//given
		storeFixture := objectstore.NewStoreFixture(t)
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
		storeFixture := objectstore.NewStoreFixture(t)
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
		storeFixture := objectstore.NewStoreFixture(t)
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
		storeFixture := objectstore.NewStoreFixture(t)
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
		storeFixture := objectstore.NewStoreFixture(t)
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
		notification, err := storeFixture.GetNotificationByID("id")
		assert.Nil(t, err)

		// then
		assert.Equal(t, model.Notification_Read, notification.Status)
	})
	t.Run("action != close - status == replied", func(t *testing.T) {
		//given
		storeFixture := objectstore.NewStoreFixture(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Created})
		assert.Nil(t, err)

		sender := mock_event.NewMockSender(t)
		sender.EXPECT().Broadcast(mock.Anything).Return().Times(1)

		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
		}

		// when
		err = notifications.Reply([]string{"id"}, model.Notification_REPORT)
		assert.Nil(t, err)
		notification, err := storeFixture.GetNotificationByID("id")
		assert.Nil(t, err)

		// then
		assert.Equal(t, model.Notification_Replied, notification.Status)
	})

	t.Run("close multiple notifications", func(t *testing.T) {
		//given
		storeFixture := objectstore.NewStoreFixture(t)
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
		err = notifications.Reply([]string{"id", "id1"}, model.Notification_REPORT)
		assert.Nil(t, err)

		// then
		notification, err := storeFixture.GetNotificationByID("id")
		assert.Nil(t, err)
		assert.Equal(t, model.Notification_Replied, notification.Status)

		notification, err = storeFixture.GetNotificationByID("id1")
		assert.Nil(t, err)
		assert.Equal(t, model.Notification_Replied, notification.Status)
	})
}
