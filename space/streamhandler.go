package space

import (
	"errors"
	"github.com/anytypeio/any-sync/commonspace"
	"github.com/anytypeio/any-sync/commonspace/spacesyncproto"
	"github.com/anytypeio/any-sync/net/peer"
	"golang.org/x/net/context"
	"storj.io/drpc"
	"sync/atomic"
	"time"
)

var (
	errUnexpectedMessage = errors.New("unexpected message")
)

var lastMsgId atomic.Uint64

type streamHandler struct {
	s *service
}

func (s *streamHandler) OpenStream(ctx context.Context, p peer.Peer) (stream drpc.Stream, tags []string, err error) {
	objectStream, err := spacesyncproto.NewDRPCSpaceSyncClient(p).ObjectSyncStream(ctx)
	if err != nil {
		return
	}
	openedSpaceIds := s.s.getOpenedSpaceIds()
	if len(openedSpaceIds) > 0 {
		var msg = &spacesyncproto.SpaceSubscription{
			SpaceIds: openedSpaceIds,
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

	space, err := s.s.GetSpace(ctx, syncMsg.SpaceId)
	if err != nil {
		return
	}
	err = space.HandleMessage(ctx, commonspace.HandleMessage{
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
