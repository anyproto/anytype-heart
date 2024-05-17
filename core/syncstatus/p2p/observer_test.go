package p2p

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/commonspace/peerstatus"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pb"
)

func TestObservers_BroadcastStatus(t *testing.T) {
	t.Run("TestObservers_BroadcastStatus on registered observers", func(t *testing.T) {
		// given
		observers := NewObservers()
		status := newFixture(t, "spaceId1", pb.EventP2PStatus_NotConnected)
		err := status.Run(context.Background())
		assert.Nil(t, err)
		observers.AddObserver("spaceId1", status)

		status2 := newFixture(t, "spaceId2", pb.EventP2PStatus_NotConnected)
		err = status2.Run(context.Background())
		assert.Nil(t, err)
		observers.AddObserver("spaceId2", status2)

		// when
		status.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId: "spaceId1",
							Status:  pb.EventP2PStatus_Connected,
						},
					},
				},
			},
		})
		status2.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId: "spaceId2",
							Status:  pb.EventP2PStatus_Connected,
						},
					},
				},
			},
		})
		observers.BroadcastStatus(peerstatus.Connected)
		for status.StatusUpdateSender.(*p2pStatus).status != peerstatus.Connected {
		}

		for status2.StatusUpdateSender.(*p2pStatus).status != peerstatus.Connected {
		}

		// then
		assert.Equal(t, peerstatus.Connected, status.StatusUpdateSender.(*p2pStatus).status)
		assert.Equal(t, peerstatus.Connected, status2.StatusUpdateSender.(*p2pStatus).status)

		err = status.Close(context.Background())
		assert.Nil(t, err)
		err = status2.Close(context.Background())
		assert.Nil(t, err)
	})
}

func TestObservers_BroadcastPeerUpdate(t *testing.T) {
	t.Run("BroadcastPeerUpdate on registered observers", func(t *testing.T) {
		// given
		observers := NewObservers()
		status := newFixture(t, "spaceId1", pb.EventP2PStatus_NotConnected)
		err := status.Run(context.Background())
		assert.Nil(t, err)
		observers.AddObserver("spaceId1", status)

		status2 := newFixture(t, "spaceId2", pb.EventP2PStatus_NotConnected)
		err = status2.Run(context.Background())
		assert.Nil(t, err)
		observers.AddObserver("spaceId2", status2)

		// when
		status.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId: "spaceId1",
							Status:  pb.EventP2PStatus_Connected,
						},
					},
				},
			},
		})
		status2.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId: "spaceId2",
							Status:  pb.EventP2PStatus_Connected,
						},
					},
				},
			},
		})
		status.store.UpdateLocalPeer("peerId", []string{"spaceId1"})
		status2.store.UpdateLocalPeer("peerId", []string{"spaceId2"})
		observers.BroadcastPeerUpdate()
		for status.StatusUpdateSender.(*p2pStatus).status != peerstatus.Connected {
		}

		for status2.StatusUpdateSender.(*p2pStatus).status != peerstatus.Connected {
		}

		// then
		assert.Equal(t, peerstatus.Connected, status.StatusUpdateSender.(*p2pStatus).status)
		assert.Equal(t, peerstatus.Connected, status2.StatusUpdateSender.(*p2pStatus).status)

		err = status.Close(context.Background())
		assert.Nil(t, err)
		err = status2.Close(context.Background())
		assert.Nil(t, err)
	})
}

func TestObservers_SendPeerUpdate(t *testing.T) {
	t.Run("BroadcastPeerUpdate on registered observers", func(t *testing.T) {
		// given
		observers := NewObservers()
		status := newFixture(t, "spaceId1", pb.EventP2PStatus_NotConnected)
		err := status.Run(context.Background())
		assert.Nil(t, err)
		observers.AddObserver("spaceId1", status)

		status2 := newFixture(t, "spaceId2", pb.EventP2PStatus_NotConnected)
		err = status2.Run(context.Background())
		assert.Nil(t, err)
		observers.AddObserver("spaceId2", status2)

		// when
		status.sender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{
				{
					Value: &pb.EventMessageValueOfP2PStatusUpdate{
						P2PStatusUpdate: &pb.EventP2PStatusUpdate{
							SpaceId: "spaceId1",
							Status:  pb.EventP2PStatus_Connected,
						},
					},
				},
			},
		})
		status.store.UpdateLocalPeer("peerId", []string{"spaceId1"})
		observers.SendPeerUpdate([]string{"spaceId1"})
		for status.StatusUpdateSender.(*p2pStatus).status != peerstatus.Connected {
		}

		// then
		assert.Equal(t, peerstatus.Connected, status.StatusUpdateSender.(*p2pStatus).status)
		assert.Equal(t, peerstatus.NotConnected, status2.StatusUpdateSender.(*p2pStatus).status)

		err = status.Close(context.Background())
		assert.Nil(t, err)
		err = status2.Close(context.Background())
		assert.Nil(t, err)
	})
}
