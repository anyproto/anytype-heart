package peerstatus

import (
	"context"
	"fmt"
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
	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	*p2pStatus
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

		// then
		f.Close(nil)
	})
}

func TestP2pStatus_SendNewStatus(t *testing.T) {
	t.Run("send NotPossible status", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// when
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_NotPossible,
							DevicesCounter: 0,
						},
					},
				},
			},
		})

		f.setNotPossibleStatus(localdiscovery.DiscoveryNoInterfaces)

		// then

		err := waitForStatus("spaceId", f.p2pStatus, NotPossible)
		assert.Nil(t, err)

		// when
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_NotConnected,
							DevicesCounter: 0,
						},
					},
				},
			},
		})
		f.setNotPossibleStatus(localdiscovery.DiscoveryPossible)

		err = f.refreshSpaces([]string{"spaceId"})
		assert.Nil(t, err)

		checkStatus(t, "spaceId", f.p2pStatus, NotConnected)

		assert.Nil(t, err)
		f.Close(nil)
	})
	t.Run("send NotConnected status", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// then
		status := f.p2pStatus
		assert.NotNil(t, status)
		err := waitForStatus("spaceId", status, NotConnected)
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

		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_Connected,
							DevicesCounter: 1,
						},
					},
				},
			},
		})
		// then
		f.Close(nil)

		checkStatus(t, "spaceId", f.p2pStatus, Connected)
		// should not create a problem, cause we already closed
		f.store.RemoveLocalPeer("peerId")

	})
	t.Run("send NotConnected status, because we have peer and then were disconnected", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_Connected, 1)
		ctrl := gomock.NewController(t)
		peer := mock_peer.NewMockPeer(ctrl)
		peer.EXPECT().Id().Return("peerId")
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_Connected,
							DevicesCounter: 1,
						},
					},
				},
			},
		})
		err := f.pool.AddPeer(context.Background(), peer)
		assert.Nil(t, err)
		f.store.UpdateLocalPeer("peerId", []string{"spaceId"})
		checkStatus(t, "spaceId", f.p2pStatus, Connected)

		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_NotConnected,
							DevicesCounter: 0,
						},
					},
				},
			},
		})
		f.store.RemoveLocalPeer("peerId")

		checkStatus(t, "spaceId", f.p2pStatus, NotConnected)

		// then
		f.Close(nil)
		assert.Nil(t, err)
	})
	t.Run("connection was not possible, but after a while starts working", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// when
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_NotPossible,
							DevicesCounter: 0,
						},
					},
				},
			},
		})
		f.setNotPossibleStatus(localdiscovery.DiscoveryNoInterfaces)
		checkStatus(t, "spaceId", f.p2pStatus, NotPossible)

		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_Connected,
							DevicesCounter: 1,
						},
					},
				},
			},
		})
		f.store.UpdateLocalPeer("peerId", []string{"spaceId"})
		ctrl := gomock.NewController(t)
		peer := mock_peer.NewMockPeer(ctrl)
		peer.EXPECT().Id().Return("peerId")
		err := f.pool.AddPeer(context.Background(), peer)
		assert.Nil(t, err)

		checkStatus(t, "spaceId", f.p2pStatus, Connected)
		// then
		f.Close(nil)
	})
	t.Run("no peers were connected, but after a while one is connected", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// when
		checkStatus(t, "spaceId", f.p2pStatus, NotConnected)

		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_Connected,
							DevicesCounter: 1,
						},
					},
				},
			},
		})
		f.store.UpdateLocalPeer("peerId", []string{"spaceId"})
		ctrl := gomock.NewController(t)
		peer := mock_peer.NewMockPeer(ctrl)
		peer.EXPECT().Id().Return("peerId")
		err := f.pool.AddPeer(context.Background(), peer)
		assert.Nil(t, err)

		checkStatus(t, "spaceId", f.p2pStatus, Connected)

		// then
		f.Close(nil)
	})
	t.Run("reset not possible status", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// when
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_NotPossible,
							DevicesCounter: 0,
						},
					},
				},
			},
		})
		f.setNotPossibleStatus(localdiscovery.DiscoveryNoInterfaces)
		checkStatus(t, "spaceId", f.p2pStatus, NotPossible)

		// double set should not generate new event
		f.setNotPossibleStatus(localdiscovery.DiscoveryNoInterfaces)
		checkStatus(t, "spaceId", f.p2pStatus, NotPossible)

		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_NotConnected,
							DevicesCounter: 0,
						},
					},
				},
			},
		})

		f.setNotPossibleStatus(localdiscovery.DiscoveryPossible)
		checkStatus(t, "spaceId", f.p2pStatus, NotConnected)
		// then
		f.Close(nil)
	})
	t.Run("don't reset not possible status, because status != NotPossible", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// when
		checkStatus(t, "spaceId", f.p2pStatus, NotConnected)

		f.setNotPossibleStatus(localdiscovery.DiscoveryPossible)
		checkStatus(t, "spaceId", f.p2pStatus, NotConnected)
		// then
		f.Close(nil)
		checkStatus(t, "spaceId", f.p2pStatus, NotConnected)
	})
}

func TestP2pStatus_SendToNewSession(t *testing.T) {
	t.Run("send event only to new session", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_Connected, 1)
		ctrl := gomock.NewController(t)
		peer := mock_peer.NewMockPeer(ctrl)
		peer.EXPECT().Id().Return("peerId")
		f.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_Connected,
							DevicesCounter: 1,
						},
					},
				},
			},
		})
		err := f.pool.AddPeer(context.Background(), peer)
		assert.Nil(t, err)
		f.store.UpdateLocalPeer("peerId", []string{"spaceId"})
		checkStatus(t, "spaceId", f.p2pStatus, Connected)

		f.sender.EXPECT().SendToSession("token1", &pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId:        "spaceId",
							Status:         pb.EventP2PStatus_Connected,
							DevicesCounter: 1,
						},
					},
				},
			},
		})
		err = f.sendStatusForNewSession(session.NewContext(session.WithSession("token1")))
		assert.Nil(t, err)

		// then
		f.Close(nil)
	})
}
func TestP2pStatus_UnregisterSpace(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// when
		f.UnregisterSpace("spaceId")

		// then
		f.p2pStatus.Lock()
		defer f.p2pStatus.Unlock()
		status := f.p2pStatus
		assert.Len(t, status.spaceIds, 0)
	})
	t.Run("delete non existing space", func(t *testing.T) {
		// given
		f := newFixture(t, "spaceId", pb.EventP2PStatus_NotConnected, 1)

		// when
		f.UnregisterSpace("spaceId1")

		// then
		f.p2pStatus.Lock()
		defer f.p2pStatus.Unlock()
		status := f.p2pStatus
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
	hookRegister.EXPECT().RegisterDiscoveryPossibilityHook(mock.Anything).Return()

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
						DevicesCounter: 0,
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

	err = status.Run(context.Background())
	assert.Nil(t, err)

	status.RegisterSpace(spaceId)

	f := &fixture{
		p2pStatus:    status.(*p2pStatus),
		sender:       sender,
		service:      service,
		store:        store,
		pool:         pool,
		hookRegister: hookRegister,
	}

	for range 10 {
		f.p2pStatus.Lock()
		if len(f.p2pStatus.spaceIds) != 0 {
			f.p2pStatus.Unlock()
			return f
		}
		f.p2pStatus.Unlock()
		time.Sleep(time.Millisecond * 10)
	}
	t.Fatalf("failed to register space")
	return f
}

func waitForStatus(spaceId string, statusSender *p2pStatus, expectedStatus Status) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond * 10):
			statusSender.Lock()
			if status, ok := statusSender.spaceIds[spaceId]; !ok {
				statusSender.Unlock()
				return fmt.Errorf("spaceId %s not found", spaceId)
			} else {
				if status.status == expectedStatus {
					statusSender.Unlock()
					return nil
				}
			}
			statusSender.Unlock()
		}
	}

}

func checkStatus(t *testing.T, spaceId string, statusSender *p2pStatus, expectedStatus Status) {
	time.Sleep(time.Millisecond * 300)
	statusSender.Lock()
	defer statusSender.Unlock()
	if status, ok := statusSender.spaceIds[spaceId]; !ok {
		assert.Fail(t, "spaceId %s not found", spaceId)
	} else {
		assert.Equal(t, expectedStatus, status.status)
	}
}
