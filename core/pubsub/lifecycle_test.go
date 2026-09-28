package pubsub

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	anysyncpubsub "github.com/anyproto/any-sync/commonspace/pubsub"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/rpc/rpctest"
	"github.com/anyproto/any-sync/testutil/accounttest"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/require"
)

type gatedPeers struct {
	p               peer.Peer
	once            sync.Once
	entered, resume chan struct{}
	observed        chan error
}

func (g *gatedPeers) SpacePeers(ctx context.Context, _ string) ([]peer.Peer, error) {
	g.once.Do(func() {
		close(g.entered)
		<-g.resume
		g.observed <- ctx.Err()
	})
	return []peer.Peer{g.p}, nil
}

func TestPublishSurvivesRequestCancellation(t *testing.T) {
	ctx := context.Background()
	recvAcc, err := accountdata.NewRandom()
	require.NoError(t, err)
	sendAcc, err := accountdata.NewRandom()
	require.NoError(t, err)
	recv := anysyncpubsub.New(anysyncpubsub.Deps{})
	recvApp := new(app.App)
	recvApp.Register(accounttest.NewWithAcc(recvAcc)).Register(recv)
	require.NoError(t, recvApp.Start(ctx))
	t.Cleanup(func() { require.NoError(t, recvApp.Close(ctx)) })
	recvServer, sendServer := rpctest.NewTestServer(), rpctest.NewTestServer()
	require.NoError(t, anysyncpubsub.RegisterRpc(recvServer.Mux, recv))
	recvID, err := recvAcc.SignKey.GetPublic().Marshall()
	require.NoError(t, err)
	sendID, err := sendAcc.SignKey.GetPublic().Marshall()
	require.NoError(t, err)
	mcRecv, mcSend := rpctest.MultiConnPairWithClientServerIdentity(sendAcc.PeerId, recvAcc.PeerId, sendID, recvID)
	recvPeer, err := peer.NewPeer(mcRecv, recvServer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = recvPeer.Close() })
	sendPeer, err := peer.NewPeer(mcSend, sendServer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sendPeer.Close() })
	peers := &gatedPeers{p: sendPeer, entered: make(chan struct{}), resume: make(chan struct{}), observed: make(chan error, 1)}
	send := anysyncpubsub.New(anysyncpubsub.Deps{Peers: peers})
	sendApp := new(app.App)
	sendApp.Register(accounttest.NewWithAcc(sendAcc)).Register(send)
	require.NoError(t, sendApp.Start(ctx))
	t.Cleanup(func() { require.NoError(t, sendApp.Close(ctx)) })
	s := New().(*service)
	s.engine = send
	t.Cleanup(func() { require.NoError(t, s.Close(ctx)) })
	received := make(chan string, 10)
	_, err = recv.Subscribe("space1", "typing/obj1", func(_, _ string, _ crypto.PubKey, p []byte) { received <- string(p) })
	require.NoError(t, err)
	requestCtx, endRequest := context.WithCancel(ctx)
	require.NoError(t, s.Publish(requestCtx, "space1", "typing/obj1", []byte("accepted-before-request-ended")))
	<-peers.entered
	endRequest() // Same cancellation that the gRPC server performs after a successful unary return.
	close(peers.resume)
	require.NoError(t, <-peers.observed)
	select {
	case got := <-received:
		require.Equal(t, "accepted-before-request-ended", got, "accepted publication must survive request cancellation")
	case <-time.After(2 * time.Second):
		t.Fatal("accepted publication failed to reach loopback receiver")
	}
	// A stream opened by the first request must remain usable by later requests.
	require.NoError(t, s.Publish(ctx, "space1", "typing/obj1", []byte("next request")))
	select {
	case got := <-received:
		require.Equal(t, "next request", got)
	case <-time.After(2 * time.Second):
		t.Fatal("persistent stream was closed with the first request")
	}
	require.NoError(t, s.Close(ctx))
	require.ErrorIs(t, s.ctx.Err(), context.Canceled)
	require.ErrorIs(t, s.Publish(ctx, "space1", "typing/obj1", nil), context.Canceled)
}
