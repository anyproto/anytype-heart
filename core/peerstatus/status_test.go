package peerstatus

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/peer/mock_peer"
	"github.com/anyproto/any-sync/net/rpc/rpctest"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/peerstatus/mock_peerstatus"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	PeerToPeerStatus
	sender       *mock_event.MockSender
	service      *mock_nodeconf.MockService
	store        peerstore.PeerStore
	pool         *rpctest.TestPool
	hookRegister *mock_peerstatus.MockLocalDiscoveryHook
}

func TestP2PStatus_Init(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// when
		f.Run(nil)

		// then
		f.Close(nil)
	})
}

func TestP2pStatus_SendNewStatus(t *testing.T) {
	t.Run("send NotPossible status", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)
		f.Run(nil)

		// when
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_NotPossible,
							DevicesCounter: 1,
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

		f.CheckPeerStatus()
		err = waitForStatus(status, NotPossible)

		assert.Nil(t, err)
		f.Close(nil)
	})
	t.Run("send NotConnected status", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// when
		f.Run(nil)

		// then
		status := f.PeerToPeerStatus.(*p2pStatus)
		assert.NotNil(t, status)
		err := waitForStatus(status, NotConnected)
		assert.Nil(t, err)
		f.Close(nil)
	})
}

func TestP2pStatus_SendPeerUpdate(t *testing.T) {
	t.Run("send Connected status, because we have peers", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_Connected, 1)
		f.store.UpdateLocalPeer("peerId", []string{"spaceId"})
		ctrl := gomock.NewController(t)
		peer := mock_peer.NewMockPeer(ctrl)
		peer.EXPECT().Id().Return("peerId")
		err := f.pool.AddPeer(context.Background(), peer)
		assert.Nil(t, err)

		// when
		f.Run(nil)
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_Connected,
							DevicesCounter: 2,
						},
					},
				},
			},
		})
		f.CheckPeerStatus()

		// then
		f.Close(nil)

		status := f.PeerToPeerStatus.(*p2pStatus)
		assert.NotNil(t, status)
		err = waitForStatus(status, Connected)
		assert.Nil(t, err)
	})
	t.Run("send NotConnected status, because we have peer were disconnected", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_Connected, 1)
		f.store.UpdateLocalPeer("peerId", []string{"spaceId"})
		ctrl := gomock.NewController(t)
		peer := mock_peer.NewMockPeer(ctrl)
		peer.EXPECT().Id().Return("peerId")
		err := f.pool.AddPeer(context.Background(), peer)
		assert.Nil(t, err)

		// when
		f.Run(nil)
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_Connected,
							DevicesCounter: 2,
						},
					},
				},
			},
		})
		err = waitForStatus(f.PeerToPeerStatus.(*p2pStatus), Connected)
		assert.Nil(t, err)

		f.store.RemoveLocalPeer("peerId")
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_NotConnected,
							DevicesCounter: 1,
						},
					},
				},
			},
		})
		f.CheckPeerStatus()
		err = waitForStatus(f.PeerToPeerStatus.(*p2pStatus), NotConnected)
		assert.Nil(t, err)

		// then
		f.Close(nil)
		assert.Nil(t, err)

		status := f.PeerToPeerStatus.(*p2pStatus)
		assert.NotNil(t, status)
		err = waitForStatus(status, NotConnected)
	})
	t.Run("connection was not possible, but after a while starts working", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// when
		f.Run(nil)

		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_NotPossible,
							DevicesCounter: 1,
						},
					},
				},
			},
		})
		f.SendNotPossibleStatus()
		err := waitForStatus(f.PeerToPeerStatus.(*p2pStatus), NotPossible)
		assert.Nil(t, err)

		f.store.UpdateLocalPeer("peerId", []string{"spaceId"})
		ctrl := gomock.NewController(t)
		peer := mock_peer.NewMockPeer(ctrl)
		peer.EXPECT().Id().Return("peerId")
		err = f.pool.AddPeer(context.Background(), peer)
		assert.Nil(t, err)

		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_Connected,
							DevicesCounter: 2,
						},
					},
				},
			},
		})
		f.CheckPeerStatus()
		err = waitForStatus(f.PeerToPeerStatus.(*p2pStatus), Connected)
		assert.Nil(t, err)

		// then
		f.Close(nil)
		assert.Nil(t, err)

		status := f.PeerToPeerStatus.(*p2pStatus)
		assert.NotNil(t, status)
		checkStatus(t, status, Connected)
	})
	t.Run("no peers were connected, but after a while one is connected", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// when
		f.Run(nil)

		err := waitForStatus(f.PeerToPeerStatus.(*p2pStatus), NotConnected)

		f.store.UpdateLocalPeer("peerId", []string{"spaceId"})
		ctrl := gomock.NewController(t)
		peer := mock_peer.NewMockPeer(ctrl)
		peer.EXPECT().Id().Return("peerId")
		err = f.pool.AddPeer(context.Background(), peer)
		assert.Nil(t, err)

		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_Connected,
							DevicesCounter: 2,
						},
					},
				},
			},
		})
		f.CheckPeerStatus()
		err = waitForStatus(f.PeerToPeerStatus.(*p2pStatus), Connected)
		assert.Nil(t, err)

		// then
		f.Close(nil)
		assert.Nil(t, err)

		status := f.PeerToPeerStatus.(*p2pStatus)
		assert.NotNil(t, status)
		checkStatus(t, status, Connected)
	})
	t.Run("reset not possible status", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// when
		f.Run(nil)

		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_NotPossible,
							DevicesCounter: 1,
						},
					},
				},
			},
		})
		f.SendNotPossibleStatus()
		status := f.PeerToPeerStatus.(*p2pStatus)
		assert.NotNil(t, status)
		err := waitForStatus(status, NotPossible)
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_NotConnected,
							DevicesCounter: 1,
						},
					},
				},
			},
		})
		f.ResetNotPossibleStatus()
		err = waitForStatus(status, NotConnected)
		assert.Nil(t, err)

		// then
		f.Close(nil)
		assert.Nil(t, err)
		checkStatus(t, status, NotConnected)
	})
	t.Run("don't reset not possible status, because status != NotPossible", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// when
		f.Run(nil)

		status := f.PeerToPeerStatus.(*p2pStatus)

		err := waitForStatus(status, NotConnected)
		f.ResetNotPossibleStatus()
		err = waitForStatus(status, NotConnected)

		// then
		f.Close(nil)
		assert.Nil(t, err)
		checkStatus(t, status, NotConnected)
	})
}

func TestP2pStatus_UnregisterSpace(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// when
		f.UnregisterSpace("spaceId")

		// then

		status := f.PeerToPeerStatus.(*p2pStatus)
		assert.Len(t, status.spaceIds, 0)
	})
	t.Run("delete non existing space", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// when
		f.UnregisterSpace("spaceId1")

		// then
		status := f.PeerToPeerStatus.(*p2pStatus)
		assert.Len(t, status.spaceIds, 1)
	})
}

func newFixture(t *testing.T, spaceId string, initialStatus pb.EventP2PStatusStatus, deviceCount int) *fixture {
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
	hookRegister := mock_peerstatus.NewMockLocalDiscoveryHook(t)
	hookRegister.EXPECT().RegisterP2PNotPossible(mock.Anything).Return()
	hookRegister.EXPECT().RegisterResetNotPossible(mock.Anything).Return()

	a := &app.App{}
	ctx := context.Background()
	a.Register(testutil.PrepareMock(ctx, a, sender)).
		Register(testutil.PrepareMock(ctx, a, service)).
		Register(store).
		Register(pool).
		Register(testutil.PrepareMock(ctx, a, hookRegister)).
		Register(session.NewHookRunner())
	status := New()

	err := status.Init(a)
	sender.EXPECT().Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfP2PStatusUpdate{
					P2PStatusUpdate: &pb.EventP2PStatusUpdate{
						SpaceId:        spaceId,
						DevicesCounter: 1,
					},
				},
			},
		},
	}).Maybe()
	sender.EXPECT().Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfP2PStatusUpdate{
					P2PStatusUpdate: &pb.EventP2PStatusUpdate{
						SpaceId:        spaceId,
						Status:         initialStatus,
						DevicesCounter: int64(deviceCount),
					},
				},
			},
		},
	}).Maybe()
	status.RegisterSpace(spaceId)
	assert.Nil(t, err)

	f := &fixture{
		PeerToPeerStatus: status,
		sender:           sender,
		service:          service,
		store:            store,
		pool:             pool,
		hookRegister:     hookRegister,
	}
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
