package spacecore

import (
	"errors"
	"sync/atomic"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/streampool/streamopener"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"storj.io/drpc"
)

var (
	errUnexpectedMessage = errors.New("unexpected message")
)

var lastMsgId atomic.Uint64

type streamOpener struct {
	spaceId string
}

func (s *streamOpener) Init(a *app.App) (err error) {
	return nil
}

func (s *streamOpener) Name() (name string) {
	return streamopener.CName
}

func (s *streamOpener) OpenStream(ctx context.Context, p peer.Peer) (stream drpc.Stream, tags []string, err error) {
	log.DebugCtx(ctx, "open outgoing stream", zap.String("peerId", p.Id()))
	ctx = peer.CtxWithPeerId(ctx, p.Id())
	conn, err := p.AcquireDrpcConn(ctx)
	if err != nil {
		return
	}
	objectStream, err := spacesyncproto.NewDRPCSpaceSyncClient(conn).ObjectSyncStream(ctx)
	if err != nil {
		log.WarnCtx(ctx, "open outgoing stream error", zap.String("peerId", p.Id()), zap.Error(err))
		return
	}
	log.DebugCtx(ctx, "outgoing stream opened", zap.String("peerId", p.Id()))
	if err = objectStream.Send(&spacesyncproto.ObjectSyncMessage{
		SpaceId: s.spaceId,
	}); err != nil {
		return
	}
	stream = objectStream
	return
}
