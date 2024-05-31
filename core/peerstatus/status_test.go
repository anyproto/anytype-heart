package peerstatus

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/net/peer/mock_peer"
	"github.com/anyproto/any-sync/net/rpc/rpctest"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/peerstatus/mock_peerstatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

type fixture struct {
	PeerToPeerStatus
	sender         *mock_event.MockSender
	service        *mock_nodeconf.MockService
	store          peerstore.PeerStore
	pool           *rpctest.TestPool
	hookRegister   *mock_peerstatus.MockHookRegister
	peerUpdateHook *mock_peerstatus.MockPeerUpdateHook
}

func TestP2PStatus_Init(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected)

		// when
		f.Run()

		// then
		f.Close()
	})
}

func TestP2pStatus_SendNewStatus(t *testing.T) {
	t.Run("send NotPossible status", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected)
		f.Run()

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
		status := f.PeerToPeerStatus.(*p2pStatus)
		assert.NotNil(t, status)
		err := waitForStatus(status, NotPossible)
		assert.Nil(t, err)
		f.Close()
	})
	t.Run("send NotConnected status", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected)

		// when
		f.Run()

		// then
		status := f.PeerToPeerStatus.(*p2pStatus)
		assert.NotNil(t, status)
		err := waitForStatus(status, NotConnected)
		assert.Nil(t, err)
		f.Close()
	})
}

func TestP2pStatus_SendPeerUpdate(t *testing.T) {
	t.Run("send Connected status, because we have peers", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_Connected)
		f.store.UpdateLocalPeer("peerId", []string{"spaceId"})

		// when
		f.Run()
		f.CheckPeerStatus()

		// then
		f.Close()

		status := f.PeerToPeerStatus.(*p2pStatus)
		assert.NotNil(t, status)
		err := waitForStatus(status, Connected)
		assert.Nil(t, err)
	})
	t.Run("send NotConnected status, because we have peer were disconnected", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_Connected)
		f.store.UpdateLocalPeer("peerId", []string{"spaceId"})

		// when
		f.Run()

		err := waitForStatus(f.PeerToPeerStatus.(*p2pStatus), Connected)
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
		err = waitForStatus(f.PeerToPeerStatus.(*p2pStatus), NotConnected)
		assert.Nil(t, err)

		// then
		f.Close()
		assert.Nil(t, err)

		status := f.PeerToPeerStatus.(*p2pStatus)
		assert.NotNil(t, status)
		err = waitForStatus(status, NotConnected)
	})
	t.Run("connection was not possible, but after a while starts working", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected)

		// when
		f.Run()

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
		err := waitForStatus(f.PeerToPeerStatus.(*p2pStatus), NotPossible)
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
		err = waitForStatus(f.PeerToPeerStatus.(*p2pStatus), Connected)
		assert.Nil(t, err)

		// then
		f.Close()
		assert.Nil(t, err)

		status := f.PeerToPeerStatus.(*p2pStatus)
		assert.NotNil(t, status)
		checkStatus(t, status, Connected)
	})
	t.Run("no peers were connected, but after a while one is connected", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected)

		// when
		f.Run()

		err := waitForStatus(f.PeerToPeerStatus.(*p2pStatus), NotConnected)

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
		err = waitForStatus(f.PeerToPeerStatus.(*p2pStatus), Connected)
		assert.Nil(t, err)

		// then
		f.Close()
		assert.Nil(t, err)

		status := f.PeerToPeerStatus.(*p2pStatus)
		assert.NotNil(t, status)
		checkStatus(t, status, Connected)
	})
}

func newFixture(t *testing.T, spaceId string, initialStatus pb.EventP2PStatusStatus) *fixture {
	ctrl := gomock.NewController(t)
	sender := mock_event.NewMockSender(t)
	service := mock_nodeconf.NewMockService(ctrl)
	service.EXPECT().Name().Return("common.nodeconf").AnyTimes()
	pool := rpctest.NewTestPool()
	pool.WithServer(rpctest.NewTestServer())
	peer := mock_peer.NewMockPeer(ctrl)
	peer.EXPECT().Id().Return("peerId")
	pool.AddPeer(context.Background(), peer)
	store := peerstore.New()
	hookRegister := mock_peerstatus.NewMockHookRegister(t)
	hookRegister.EXPECT().RegisterP2PNotPossible(mock.Anything).Return()
	peerUpdateHook := mock_peerstatus.NewMockPeerUpdateHook(t)
	peerUpdateHook.EXPECT().Register(mock.Anything).Return()

	status := NewP2PStatus(spaceId, sender, pool, hookRegister, peerUpdateHook, store)
	f := &fixture{
		PeerToPeerStatus: status,
		sender:           sender,
		service:          service,
		store:            store,
		pool:             pool,
		hookRegister:     hookRegister,
		peerUpdateHook:   peerUpdateHook,
	}
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
