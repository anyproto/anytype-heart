package syncstatus

import (
	"testing"

	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/objectsyncstatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

func TestUpdateReceiver_UpdateTree(t *testing.T) {
	t.Run("update to sync status", func(t *testing.T) {
		// given
		receiver := newFixture(t)
		receiver.nodeConnected = true
		receiver.nodeConf.EXPECT().NetworkCompatibilityStatus().Return(nodeconf.NetworkCompatibilityStatusOk)
		receiver.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{Value: &pb.EventMessageValueOfThreadStatus{ThreadStatus: &pb.EventStatusThread{
				Summary: &pb.EventStatusThreadSummary{Status: pb.EventStatusThread_Synced},
				Cafe: &pb.EventStatusThreadCafe{
					Status: pb.EventStatusThread_Synced,
					Files:  &pb.EventStatusThreadCafePinStatus{},
				},
			}}}},
			ContextId: "id",
		}).Return()

		// when
		err := receiver.UpdateTree(nil, "id", objectsyncstatus.StatusSynced)

		// then
		assert.Nil(t, err)
	})
	t.Run("network incompatible", func(t *testing.T) {
		// given
		receiver := newFixture(t)
		receiver.nodeConnected = true
		receiver.nodeConf.EXPECT().NetworkCompatibilityStatus().Return(nodeconf.NetworkCompatibilityStatusIncompatible)
		receiver.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{Value: &pb.EventMessageValueOfThreadStatus{ThreadStatus: &pb.EventStatusThread{
				Summary: &pb.EventStatusThreadSummary{Status: pb.EventStatusThread_IncompatibleVersion},
				Cafe: &pb.EventStatusThreadCafe{
					Status: pb.EventStatusThread_IncompatibleVersion,
					Files:  &pb.EventStatusThreadCafePinStatus{},
				},
			}}}},
			ContextId: "id",
		}).Return()

		// when
		err := receiver.UpdateTree(nil, "id", objectsyncstatus.StatusNotSynced)

		// then
		assert.Nil(t, err)
	})
	t.Run("file storage limited", func(t *testing.T) {
		// given
		receiver := newFixture(t)
		receiver.nodeConnected = true
		receiver.nodeConf.EXPECT().NetworkCompatibilityStatus().Return(nodeconf.NetworkCompatibilityStatusOk)
		receiver.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{Value: &pb.EventMessageValueOfThreadStatus{ThreadStatus: &pb.EventStatusThread{
				Summary: &pb.EventStatusThreadSummary{Status: pb.EventStatusThread_Syncing},
				Cafe: &pb.EventStatusThreadCafe{
					Status: pb.EventStatusThread_Syncing,
					Files:  &pb.EventStatusThreadCafePinStatus{},
				},
			}}}},
			ContextId: "id",
		}).Return()

		receiver.store.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               domain.String("id"),
				bundle.RelationKeyFileBackupStatus: domain.Int64(int64(filesyncstatus.Limited)),
			},
		})

		// when
		err := receiver.UpdateTree(nil, "id", objectsyncstatus.StatusNotSynced)

		// then
		assert.Nil(t, err)
	})
	t.Run("object sync status - syncing", func(t *testing.T) {
		// given
		receiver := newFixture(t)
		receiver.nodeConnected = true
		receiver.nodeConf.EXPECT().NetworkCompatibilityStatus().Return(nodeconf.NetworkCompatibilityStatusOk)
		receiver.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{Value: &pb.EventMessageValueOfThreadStatus{ThreadStatus: &pb.EventStatusThread{
				Summary: &pb.EventStatusThreadSummary{Status: pb.EventStatusThread_Syncing},
				Cafe: &pb.EventStatusThreadCafe{
					Status: pb.EventStatusThread_Syncing,
					Files:  &pb.EventStatusThreadCafePinStatus{},
				},
			}}}},
			ContextId: "id",
		}).Return()

		// when
		err := receiver.UpdateTree(nil, "id", objectsyncstatus.StatusNotSynced)

		// then
		assert.Nil(t, err)
	})
	t.Run("object sync status - unknown", func(t *testing.T) {
		// given
		receiver := newFixture(t)
		receiver.nodeConnected = true
		receiver.nodeConf.EXPECT().NetworkCompatibilityStatus().Return(nodeconf.NetworkCompatibilityStatusOk)
		receiver.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{Value: &pb.EventMessageValueOfThreadStatus{ThreadStatus: &pb.EventStatusThread{
				Summary: &pb.EventStatusThreadSummary{Status: pb.EventStatusThread_Unknown},
				Cafe: &pb.EventStatusThreadCafe{
					Status: pb.EventStatusThread_Unknown,
					Files:  &pb.EventStatusThreadCafePinStatus{},
				},
			}}}},
			ContextId: "id",
		}).Return()

		// when
		err := receiver.UpdateTree(nil, "id", objectsyncstatus.StatusUnknown)

		// then
		assert.Nil(t, err)
	})
	t.Run("object sync status - connection error", func(t *testing.T) {
		// given
		receiver := newFixture(t)
		receiver.nodeConnected = false
		receiver.nodeConf.EXPECT().NetworkCompatibilityStatus().Return(nodeconf.NetworkCompatibilityStatusOk)
		receiver.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{Value: &pb.EventMessageValueOfThreadStatus{ThreadStatus: &pb.EventStatusThread{
				Summary: &pb.EventStatusThreadSummary{Status: pb.EventStatusThread_Offline},
				Cafe: &pb.EventStatusThreadCafe{
					Status: pb.EventStatusThread_Offline,
					Files:  &pb.EventStatusThreadCafePinStatus{},
				},
			}}}},
			ContextId: "id",
		}).Return()

		// when
		err := receiver.UpdateTree(nil, "id", objectsyncstatus.StatusSynced)

		// then
		assert.Nil(t, err)
	})
	t.Run("file sync status - synced", func(t *testing.T) {
		// given
		receiver := newFixture(t)
		receiver.nodeConnected = true
		receiver.nodeConf.EXPECT().NetworkCompatibilityStatus().Return(nodeconf.NetworkCompatibilityStatusOk)
		receiver.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{Value: &pb.EventMessageValueOfThreadStatus{ThreadStatus: &pb.EventStatusThread{
				Summary: &pb.EventStatusThreadSummary{Status: pb.EventStatusThread_Synced},
				Cafe: &pb.EventStatusThreadCafe{
					Status: pb.EventStatusThread_Synced,
					Files:  &pb.EventStatusThreadCafePinStatus{},
				},
			}}}},
			ContextId: "id",
		}).Return()

		receiver.store.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               domain.String("id"),
				bundle.RelationKeyFileBackupStatus: domain.Int64(int64(filesyncstatus.Synced)),
			},
		})

		// when
		err := receiver.UpdateTree(nil, "id", objectsyncstatus.StatusSynced)

		// then
		assert.Nil(t, err)
	})
	t.Run("file sync status - syncing", func(t *testing.T) {
		// given
		receiver := newFixture(t)
		receiver.nodeConnected = true
		receiver.nodeConf.EXPECT().NetworkCompatibilityStatus().Return(nodeconf.NetworkCompatibilityStatusOk)
		receiver.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{Value: &pb.EventMessageValueOfThreadStatus{ThreadStatus: &pb.EventStatusThread{
				Summary: &pb.EventStatusThreadSummary{Status: pb.EventStatusThread_Syncing},
				Cafe: &pb.EventStatusThreadCafe{
					Status: pb.EventStatusThread_Syncing,
					Files:  &pb.EventStatusThreadCafePinStatus{},
				},
			}}}},
			ContextId: "id",
		}).Return()

		receiver.store.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               domain.String("id"),
				bundle.RelationKeyFileBackupStatus: domain.Int64(int64(filesyncstatus.Syncing)),
			},
		})

		// when
		err := receiver.UpdateTree(nil, "id", objectsyncstatus.StatusUnknown)

		// then
		assert.Nil(t, err)
	})
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	nodeConf := mock_nodeconf.NewMockService(ctrl)
	conf := &config.Config{}
	sender := mock_event.NewMockSender(t)
	storeFixture := objectstore.NewStoreFixture(t)
	status := nodestatus.NewNodeStatus()

	receiver := newUpdateReceiver(nodeConf, conf, sender, storeFixture, status)
	return &fixture{
		updateReceiver: receiver,
		sender:         sender,
		nodeConf:       nodeConf,
		store:          storeFixture,
	}
}

type fixture struct {
	*updateReceiver
	sender   *mock_event.MockSender
	nodeConf *mock_nodeconf.MockService
	store    *objectstore.StoreFixture
}
