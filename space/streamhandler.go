package space

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/anyproto/any-sync/commonspace/objectsync"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net/peer"
	"golang.org/x/net/context"
	"storj.io/drpc"
)

var (
	errUnexpectedMessage = errors.New("unexpected message")
)

var lastMsgId atomic.Uint64

type streamHandler struct {
	s *service
}

func (s *streamHandler) OpenStream(ctx context.Context, p peer.Peer) (stream drpc.Stream, tags []string, err error) {
	return s.OpenSpaceStream(ctx, p, s.s.getOpenedSpaceIds())
}

func (s *streamHandler) OpenSpaceStream(ctx context.Context, p peer.Peer, spaceIds []string) (stream drpc.Stream, tags []string, err error) {
	conn, err := p.AcquireDrpcConn(ctx)
	if err != nil {
		return
	}
	objectStream, err := spacesyncproto.NewDRPCSpaceSyncClient(conn).ObjectSyncStream(ctx)
	if err != nil {
		return
	}
	if len(spaceIds) > 0 {
		var msg = &spacesyncproto.SpaceSubscription{
			SpaceIds: spaceIds,
			Action:   spacesyncproto.SpaceSubscriptionAction_Subscribe,
		}
		payload, merr := msg.Marshal()
		if merr != nil {
			err = merr
			return
		}
		if err = objectStream.Send(&spacesyncproto.ObjectSyncMessage{
			Payload: payload,
		}); err != nil {
			return
		}
	}
	return objectStream, nil, nil
}

func (s *streamHandler) HandleMessage(ctx context.Context, peerId string, msg drpc.Message) (err error) {
	syncMsg, ok := msg.(*spacesyncproto.ObjectSyncMessage)
	if !ok {
		err = errUnexpectedMessage
		return
	}
	ctx = peer.CtxWithPeerId(ctx, peerId)

	if syncMsg.SpaceId == "" {
		return s.s.HandleMessage(ctx, peerId, syncMsg)
	}

	space, err := s.s.GetSpace(ctx, syncMsg.SpaceId)
	if err != nil {
		return
	}
	err = space.HandleMessage(ctx, objectsync.HandleMessage{
		Id:       lastMsgId.Add(1),
		Deadline: time.Now().Add(time.Minute),
		SenderId: peerId,
		Message:  syncMsg,
		PeerCtx:  ctx,
	})
	return
}

func (s *streamHandler) NewReadMessage() drpc.Message {
	// TODO: we can use sync.Pool here
	return new(spacesyncproto.ObjectSyncMessage)
}
