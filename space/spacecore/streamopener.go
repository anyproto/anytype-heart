package spacecore

import (
	"errors"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/commonspace/sync/objectsync/objectmessages"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/streampool"
	"github.com/anyproto/any-sync/net/streampool/streamhandler"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"storj.io/drpc"
)

var (
	errUnexpectedMessage = errors.New("unexpected message")
)

func NewStreamOpener() streamhandler.StreamHandler {
	return &streamOpener{}
}

type streamOpener struct {
	spaceCore  *service
	streamPool streampool.StreamPool
}

func (s *streamOpener) Init(a *app.App) (err error) {
	s.spaceCore = app.MustComponent[SpaceCoreService](a).(*service)
	s.streamPool = app.MustComponent[streampool.StreamPool](a)
	return nil
}

func (s *streamOpener) Name() (name string) {
	return streamhandler.CName
}

func (s *streamOpener) OpenStream(ctx context.Context, p peer.Peer) (stream drpc.Stream, tags []string, queueSize int, err error) {
	spaceIds := s.spaceCore.getOpenedSpaceIds()
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
		payload, merr := msg.MarshalVT()
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
	return objectStream, nil, 300, nil
}

func (s *streamOpener) HandleMessage(peerCtx context.Context, peerId string, msg drpc.Message) (err error) {
	syncMsg, ok := msg.(*objectmessages.HeadUpdate)
	if !ok {
		err = errUnexpectedMessage
		return
	}
	if syncMsg.SpaceId() == "" {
		var msg = &spacesyncproto.SpaceSubscription{}
		if err = msg.UnmarshalVT(syncMsg.Bytes); err != nil {
			return
		}
		log.InfoCtx(peerCtx, "got subscription message", zap.Strings("spaceIds", msg.SpaceIds))
		if msg.Action == spacesyncproto.SpaceSubscriptionAction_Subscribe {
			return s.streamPool.AddTagsCtx(peerCtx, msg.SpaceIds...)
		} else {
			return s.streamPool.RemoveTagsCtx(peerCtx, msg.SpaceIds...)
		}
	}
	sp, err := s.spaceCore.Get(peerCtx, syncMsg.SpaceId())
	if err != nil {
		return
	}
	return sp.HandleMessage(peerCtx, syncMsg)
}

func (s *streamOpener) NewReadMessage() drpc.Message {
	return &objectmessages.HeadUpdate{}
}
