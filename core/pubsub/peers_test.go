package pubsub

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

type testPeerStore struct {
	peerstore.PeerStore
	nodes, locals []string
}

func (s testPeerStore) ResponsibleNodeIds(string) []string { return s.nodes }
func (s testPeerStore) LocalPeerIds(string) []string       { return s.locals }

type testDialer struct {
	dial func(context.Context, string) (peer.Peer, error)
}

func (s testDialer) Name() string        { return "net.peerservice" }
func (s testDialer) Init(*app.App) error { return nil }
func (s testDialer) Dial(ctx context.Context, id string) (peer.Peer, error) {
	return s.dial(ctx, id)
}

type testPoolPeer struct {
	peer.Peer
	id     string
	closed chan struct{}
	once   sync.Once
}

func (p *testPoolPeer) Id() string                           { return p.id }
func (p *testPoolPeer) CloseChan() <-chan struct{}           { return p.closed }
func (p *testPoolPeer) Close() error                         { p.once.Do(func() { close(p.closed) }); return nil }
func (p *testPoolPeer) TryClose(time.Duration) (bool, error) { return false, nil }
func (p *testPoolPeer) IsClosed() bool {
	select {
	case <-p.closed:
		return true
	default:
		return false
	}
}

func newPeerService(t *testing.T, dialer testDialer) (*service, pool.Pool) {
	t.Helper()
	p := pool.New()
	a := new(app.App)
	a.Register(dialer).Register(p)
	require.NoError(t, a.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, a.Close(context.Background())) })
	s := New().(*service)
	s.pool = p
	s.peerStore = testPeerStore{nodes: []string{"node"}, locals: []string{"lan"}}
	t.Cleanup(func() { require.NoError(t, s.Close(context.Background())) })
	return s, p
}

func TestSpacePeersReturnsConnectedRoutesWithoutWaitingForDials(t *testing.T) {
	for _, liveID := range []string{"node", "lan"} {
		t.Run(liveID, func(t *testing.T) {
			started := make(chan string, 1)
			dialer := testDialer{dial: func(ctx context.Context, id string) (peer.Peer, error) {
				started <- id
				<-ctx.Done()
				return nil, ctx.Err()
			}}
			s, p := newPeerService(t, dialer)
			live := &testPoolPeer{id: liveID, closed: make(chan struct{})}
			require.NoError(t, p.AddPeer(context.Background(), live))
			deadID := "node"
			if liveID == "node" {
				deadID = "lan"
			}
			// Exercise Pick while another user of the shared pool is dialing.
			dialCtx, cancelDial := context.WithCancel(context.Background())
			defer cancelDial()
			dialDone := make(chan struct{})
			go func() { _, _ = p.Get(dialCtx, deadID); close(dialDone) }()
			require.Equal(t, deadID, <-started)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			peers, err := s.SpacePeers(ctx, testSpaceId)
			// If lookup waited for the stuck dial, its caller deadline has elapsed.
			require.NoError(t, ctx.Err())
			require.NoError(t, err)
			require.Equal(t, []peer.Peer{live}, peers)
			cancelDial()
			<-dialDone
		})
	}
}

func TestSpacePeersRacesColdRoutes(t *testing.T) {
	for _, liveID := range []string{"node", "lan"} {
		t.Run(liveID, func(t *testing.T) {
			live := &testPoolPeer{id: liveID, closed: make(chan struct{})}
			dialer := testDialer{dial: func(ctx context.Context, id string) (peer.Peer, error) {
				if id == liveID {
					return live, nil
				}
				<-ctx.Done()
				return nil, ctx.Err()
			}}
			s, _ := newPeerService(t, dialer)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			peers, err := s.SpacePeers(ctx, testSpaceId)
			require.NoError(t, err)
			require.Equal(t, []peer.Peer{live}, peers)
			require.NoError(t, ctx.Err(), "lookup must not consume the caller's deadline")
			require.False(t, live.IsClosed(), "ending lookup must not close the selected peer")
		})
	}
}

func TestSpacePeersCancellationAndEmptyCandidates(t *testing.T) {
	started := make(chan struct{}, 2)
	s, _ := newPeerService(t, testDialer{dial: func(ctx context.Context, _ string) (peer.Peer, error) {
		started <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	done := make(chan error, 1)
	go func() { _, err := s.SpacePeers(context.Background(), testSpaceId); done <- err }()
	<-started
	<-started
	require.NoError(t, s.Close(context.Background()))
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel peer lookup")
	}

	empty, _ := newPeerService(t, testDialer{dial: func(context.Context, string) (peer.Peer, error) {
		return nil, errors.New("unexpected dial")
	}})
	empty.peerStore = testPeerStore{}
	_, err := empty.SpacePeers(context.Background(), testSpaceId)
	require.ErrorContains(t, err, "no pubsub peers")
}

func TestSpacePeersReturnsAllConnectedCandidatesOnce(t *testing.T) {
	s, p := newPeerService(t, testDialer{dial: func(context.Context, string) (peer.Peer, error) {
		return nil, errors.New("unexpected dial")
	}})
	s.peerStore = testPeerStore{nodes: []string{"node"}, locals: []string{"node", "lan", "lan"}}
	for _, id := range []string{"node", "lan"} {
		require.NoError(t, p.AddPeer(context.Background(), &testPoolPeer{id: id, closed: make(chan struct{})}))
	}
	peers, err := s.SpacePeers(context.Background(), testSpaceId)
	require.NoError(t, err)
	require.Len(t, peers, 2)
	require.Equal(t, "node", peers[0].Id())
	require.Equal(t, "lan", peers[1].Id())
}
