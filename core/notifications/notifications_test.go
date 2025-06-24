package notifications

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TODO: Right now we use only CLOSE action type in protocol, so tests should be improved when we have more types
const notCloseActionType = 1

func TestNotificationService_List(t *testing.T) {
	t.Run("no notification in store - empty result", func(t *testing.T) {
		// given
		sender := mock_event.NewMockSender(t)
		notifications := notificationService{
			eventSender:       sender,
			notificationStore: NewTestStore(t),
			loadTimeout:       10 * time.Millisecond,
		}

		// when
		list, err := notifications.List(100, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, list, 0)
	})
	t.Run("limit = 0 - empty result", func(t *testing.T) {
		// given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Created})
		assert.Nil(t, err)
		err = storeFixture.SaveNotification(&model.Notification{Id: "id1", Status: model.Notification_Created})
		assert.Nil(t, err)
		sender := mock_event.NewMockSender(t)
		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
			loadTimeout:       10 * time.Millisecond,
		}

		// when
		list, err := notifications.List(0, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, list, 0)
	})
	t.Run("all notification in store are read and option includeRead=false - empty result", func(t *testing.T) {
		// given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Read})
		assert.Nil(t, err)
		err = storeFixture.SaveNotification(&model.Notification{Id: "id1", Status: model.Notification_Read})
		assert.Nil(t, err)
		sender := mock_event.NewMockSender(t)
		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
			loadTimeout:       10 * time.Millisecond,
		}

		// when
		list, err := notifications.List(10, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, list, 0)
	})
	t.Run("all notification in store are read and option includeRead=true - return all notification", func(t *testing.T) {
		// given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Read})
		assert.Nil(t, err)
		err = storeFixture.SaveNotification(&model.Notification{Id: "id1", Status: model.Notification_Read})
		assert.Nil(t, err)
		sender := mock_event.NewMockSender(t)
		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
			loadTimeout:       10 * time.Millisecond,
		}

		// when
		list, err := notifications.List(10, true)

		// then
		assert.Nil(t, err)
		assert.Len(t, list, 2)
	})
	t.Run("1 notification in store read and 1 not read, includeRead=false - 1 notification in result", func(t *testing.T) {
		// given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Replied})
		assert.Nil(t, err)
		err = storeFixture.SaveNotification(&model.Notification{Id: "id1", Status: model.Notification_Created})
		assert.Nil(t, err)
		sender := mock_event.NewMockSender(t)
		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
			loadTimeout:       10 * time.Millisecond,
		}

		// when
		list, err := notifications.List(10, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, list, 1)
	})
	t.Run("notifications with GetRequestToLeave payload are filtered out", func(t *testing.T) {
		// given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{
			Id:     "regular1",
			Status: model.Notification_Created,
			Payload: &model.NotificationPayloadOfTest{
				Test: &model.NotificationTest{},
			},
		})
		assert.Nil(t, err)
		err = storeFixture.SaveNotification(&model.Notification{
			Id:     "request1",
			Status: model.Notification_Created,
			Payload: &model.NotificationPayloadOfRequestToLeave{
				RequestToLeave: &model.NotificationRequestToLeave{
					SpaceId: "space123",
				},
			},
		})
		assert.Nil(t, err)
		err = storeFixture.SaveNotification(&model.Notification{
			Id:     "regular2",
			Status: model.Notification_Created,
			Payload: &model.NotificationPayloadOfTest{
				Test: &model.NotificationTest{},
			},
		})
		assert.Nil(t, err)
		sender := mock_event.NewMockSender(t)
		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
			loadTimeout:       10 * time.Millisecond,
		}

		// when
		list, err := notifications.List(10, false)

		// then
		assert.Nil(t, err)
		assert.Len(t, list, 2)
		assert.Equal(t, "regular1", list[0].Id)
		assert.Equal(t, "regular2", list[1].Id)
	})
}

func TestNotificationService_Reply(t *testing.T) {
	t.Run("action = close - status == read", func(t *testing.T) {
		// given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Created, IsLocal: true})
		assert.Nil(t, err)

		sender := mock_event.NewMockSender(t)
		sender.EXPECT().Broadcast(mock.Anything).Return().Times(1)

		notifications := notificationService{
			eventSender:             sender,
			notificationStore:       storeFixture,
			lastNotificationIdToAcl: map[string]string{},
			loadTimeout:             10 * time.Millisecond,
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
		// given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Created, IsLocal: true})
		assert.Nil(t, err)

		sender := mock_event.NewMockSender(t)
		sender.EXPECT().Broadcast(mock.Anything).Return().Times(1)

		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
			loadTimeout:       10 * time.Millisecond,
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
		// given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Created, IsLocal: true})
		assert.Nil(t, err)
		err = storeFixture.SaveNotification(&model.Notification{Id: "id1", Status: model.Notification_Created, IsLocal: true})
		assert.Nil(t, err)

		sender := mock_event.NewMockSender(t)
		sender.EXPECT().Broadcast(mock.Anything).Return().Times(2)

		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
			loadTimeout:       10 * time.Millisecond,
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

func TestNotificationService_CreateAndSend(t *testing.T) {
	t.Run("notification exist in store - don't send it again", func(t *testing.T) {
		// given
		storeFixture := NewTestStore(t)
		err := storeFixture.SaveNotification(&model.Notification{Id: "id", Status: model.Notification_Created, IsLocal: true})
		assert.Nil(t, err)

		testNotification := &model.Notification{
			Id:         "id",
			CreateTime: time.Now().Unix(),
			Status:     model.Notification_Created,
			IsLocal:    false,
			Payload:    &model.NotificationPayloadOfTest{Test: &model.NotificationTest{}},
		}

		sender := mock_event.NewMockSender(t)
		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
			loadTimeout:       10 * time.Millisecond,
		}

		// when
		err = notifications.CreateAndSend(testNotification)
		assert.Nil(t, err)

		// then
		sender.AssertNotCalled(t, "Broadcast", testNotification)
	})
	t.Run("notification not exist in store, but exit in NotificationObject - don't send it again", func(t *testing.T) {
		// given
		storeFixture := NewTestStore(t)

		notificationObjectId := "notificationId"
		notificationObject := editor.NewNotificationObject(smarttest.New(notificationObjectId))
		state := notificationObject.NewState()
		testNotification := &model.Notification{
			Id:         "id",
			CreateTime: time.Now().Unix(),
			Status:     model.Notification_Created,
			IsLocal:    false,
			Payload:    &model.NotificationPayloadOfTest{Test: &model.NotificationTest{}},
		}
		state.AddNotification(testNotification)
		err := notificationObject.Apply(state)
		assert.Nil(t, err)

		objectGetter := mock_cache.NewMockObjectGetter(t)
		objectGetter.EXPECT().GetObject(context.Background(), notificationObjectId).Return(notificationObject, nil)

		sender := mock_event.NewMockSender(t)
		notifications := notificationService{
			eventSender:       sender,
			notificationStore: storeFixture,
			picker:            objectGetter,
			notificationId:    notificationObjectId,
			loadTimeout:       10 * time.Millisecond,
		}

		// when
		err = notifications.CreateAndSend(testNotification)
		assert.Nil(t, err)

		// then
		sender.AssertNotCalled(t, "Broadcast", testNotification)
	})
	t.Run("notification not exist in store and not exit in NotificationObject - send it", func(t *testing.T) {
		// given
		storeFixture := NewTestStore(t)

		notificationObjectId := "notificationId"
		notificationObject := editor.NewNotificationObject(smarttest.New(notificationObjectId))
		testNotification := &model.Notification{
			Id:         "id",
			CreateTime: time.Now().Unix(),
			Status:     model.Notification_Created,
			IsLocal:    false,
			Payload:    &model.NotificationPayloadOfTest{Test: &model.NotificationTest{}},
		}

		objectGetter := mock_cache.NewMockObjectGetter(t)
		objectGetter.EXPECT().GetObject(context.Background(), notificationObjectId).Return(notificationObject, nil)

		sender := mock_event.NewMockSender(t)
		event := &pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfNotificationSend{
						NotificationSend: &pb.EventNotificationSend{
							Notification: testNotification,
						},
					},
				},
			},
		}
		sender.EXPECT().Broadcast(event).Return().Times(1)

		notifications := notificationService{
			eventSender:             sender,
			notificationStore:       storeFixture,
			picker:                  objectGetter,
			notificationId:          notificationObjectId,
			lastNotificationIdToAcl: map[string]string{},
			loadTimeout:             10 * time.Millisecond,
		}

		// when
		err := notifications.CreateAndSend(testNotification)
		assert.Nil(t, err)

		// then
		sender.AssertCalled(t, "Broadcast", event)
	})
}
func NewTestStore(t *testing.T) NotificationStore {
	db, err := anystore.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	kv, err := NewNotificationStore(db)
	require.NoError(t, err)

	return kv
}
