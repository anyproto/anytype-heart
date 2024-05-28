package peerstatus

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	StatusUpdateSender
	sender  *mock_event.MockSender
	service *mock_nodeconf.MockService
	store   peerstore.PeerStore
}

func TestP2PStatus_Init(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected)

		// when
		err := f.Run(context.Background())
		assert.Nil(t, err)

		// then
		err = f.Close(context.Background())
		assert.Nil(t, err)
	})
}

func TestP2pStatus_SendNewStatus(t *testing.T) {
	t.Run("send NotPossible status", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected)
		err := f.Run(context.Background())
		assert.Nil(t, err)

		// when
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId: "spaceId",
							Status:  pb.EventP2PStatus_NotPossible,
						},
					},
				},
			},
		})
		f.SendNotPossibleStatus()

		// then
		status := f.StatusUpdateSender.(*p2pStatus)
		assert.NotNil(t, status)
		err = waitForStatus(status, NotPossible)
		assert.Nil(t, err)
		err = f.Close(context.Background())
		assert.Nil(t, err)
	})

	t.Run("send NotConnected status", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected)
		err := f.Run(context.Background())
		assert.Nil(t, err)

		// when
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId: "spaceId",
							Status:  pb.EventP2PStatus_NotConnected,
						},
					},
				},
			},
		})
		f.SendNotPossibleStatus()

		// then
		status := f.StatusUpdateSender.(*p2pStatus)
		assert.NotNil(t, status)
		checkStatus(t, status, NotConnected)

		err = f.Close(context.Background())
		assert.Nil(t, err)
	})
	t.Run("send NotConnected status", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected)
		err := f.Run(context.Background())
		assert.Nil(t, err)

		// when
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId: "spaceId",
							Status:  pb.EventP2PStatus_NotConnected,
						},
					},
				},
			},
		})
		f.SendNotPossibleStatus()

		// then
		status := f.StatusUpdateSender.(*p2pStatus)
		assert.NotNil(t, status)
		checkStatus(t, status, NotConnected)
		err = f.Close(context.Background())
		assert.Nil(t, err)
	})
	t.Run("send Connected status", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected)
		err := f.Run(context.Background())
		assert.Nil(t, err)

		// when
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId: "spaceId",
							Status:  pb.EventP2PStatus_Connected,
						},
					},
				},
			},
		})
		f.SendNotPossibleStatus()

		// then
		status := f.StatusUpdateSender.(*p2pStatus)
		assert.NotNil(t, status)
		checkStatus(t, status, Connected)
		err = f.Close(context.Background())
		assert.Nil(t, err)
	})
}

func TestP2pStatus_SendPeerUpdate(t *testing.T) {
	t.Run("send Connected status, because we have peers", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_Connected)
		f.store.UpdateLocalPeer("peerId", []string{"spaceId"})

		// when
		err := f.Run(context.Background())
		assert.Nil(t, err)
		f.CheckPeerStatus()

		// then
		err = f.Close(context.Background())
		assert.Nil(t, err)

		status := f.StatusUpdateSender.(*p2pStatus)
		assert.NotNil(t, status)
		checkStatus(t, status, Connected)
	})
	t.Run("send NotConnected status, because we have peer were disconnected", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_Connected)
		f.store.UpdateLocalPeer("peerId", []string{"spaceId"})

		// when
		err := f.Run(context.Background())
		assert.Nil(t, err)

		err = waitForStatus(f.StatusUpdateSender.(*p2pStatus), Connected)
		assert.Nil(t, err)

		f.store.RemoveLocalPeer("peerId")
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId: "spaceId",
							Status:  pb.EventP2PStatus_NotConnected,
						},
					},
				},
			},
		})
		f.CheckPeerStatus()
		err = waitForStatus(f.StatusUpdateSender.(*p2pStatus), NotConnected)
		assert.Nil(t, err)

		// then
		err = f.Close(context.Background())
		assert.Nil(t, err)

		status := f.StatusUpdateSender.(*p2pStatus)
		assert.NotNil(t, status)
		checkStatus(t, status, NotConnected)
	})
	t.Run("connection was not possible, but after a while starts working", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected)

		// when
		err := f.Run(context.Background())
		assert.Nil(t, err)

		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId: "spaceId",
							Status:  pb.EventP2PStatus_NotPossible,
						},
					},
				},
			},
		})
		f.SendNotPossibleStatus()
		err = waitForStatus(f.StatusUpdateSender.(*p2pStatus), NotPossible)
		assert.Nil(t, err)

		f.store.UpdateLocalPeer("peerId", []string{"spaceId"})
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId: "spaceId",
							Status:  pb.EventP2PStatus_Connected,
						},
					},
				},
			},
		})
		f.CheckPeerStatus()
		err = waitForStatus(f.StatusUpdateSender.(*p2pStatus), Connected)
		assert.Nil(t, err)

		// then
		err = f.Close(context.Background())
		assert.Nil(t, err)

		status := f.StatusUpdateSender.(*p2pStatus)
		assert.NotNil(t, status)
		checkStatus(t, status, Connected)
	})
	t.Run("no peers were connected, but after a while one is connected", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected)

		// when
		err := f.Run(context.Background())
		assert.Nil(t, err)

		err = waitForStatus(f.StatusUpdateSender.(*p2pStatus), NotConnected)
		assert.Nil(t, err)

		f.store.UpdateLocalPeer("peerId", []string{"spaceId"})
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId: "spaceId",
							Status:  pb.EventP2PStatus_Connected,
						},
					},
				},
			},
		})
		f.CheckPeerStatus()
		err = waitForStatus(f.StatusUpdateSender.(*p2pStatus), Connected)
		assert.Nil(t, err)

		// then
		err = f.Close(context.Background())
		assert.Nil(t, err)

		status := f.StatusUpdateSender.(*p2pStatus)
		assert.NotNil(t, status)
		checkStatus(t, status, Connected)
	})
}

func newFixture(t *testing.T, spaceId string, initialStatus pb.EventP2PStatusStatus) *fixture {
	ctrl := gomock.NewController(t)
	a := &app.App{}
	ctx := context.Background()
	sender := mock_event.NewMockSender(t)
	service := mock_nodeconf.NewMockService(ctrl)
	service.EXPECT().Name().Return("common.nodeconf").AnyTimes()
	store := peerstore.New()
	a.Register(testutil.PrepareMock(ctx, a, sender)).
		Register(service).
		Register(store)
	err := store.Init(a)
	assert.Nil(t, err)
	status := NewP2PStatus(spaceId)
	f := &fixture{
		StatusUpdateSender: status,
		sender:             sender,
		service:            service,
		store:              store,
	}
	err = f.Init(a)
	assert.Nil(t, err)
	f.sender.EXPECT().Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfP2PStatusUpdate{
					P2PStatusUpdate: &pb.EventP2PStatusUpdate{
						SpaceId: spaceId,
						Status:  initialStatus,
					},
				},
			},
		},
	}).Maybe()
	return f
}

func waitForStatus(statusSender *p2pStatus, expectedStatus Status) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			statusSender.Lock()
			if statusSender.status == expectedStatus {
				statusSender.Unlock()
				return nil
			}
			statusSender.Unlock()
		}
	}
}

func checkStatus(t *testing.T, statusSender *p2pStatus, expectedStatus Status) {
	statusSender.Lock()
	defer statusSender.Unlock()
	assert.Equal(t, expectedStatus, statusSender.status)
}
