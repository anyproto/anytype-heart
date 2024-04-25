package peermanager

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/net/peer"
	"github.com/stretchr/testify/require"
	"storj.io/drpc"
)

func TestClientPeerManager_GetResponsiblePeers_Deadline(t *testing.T) {
	t.Run("DeadlineExceeded", func(t *testing.T) {
		cm := &clientPeerManager{
			spaceId:                   "x",
			availableResponsiblePeers: make(chan struct{}),
			Mutex:                     sync.Mutex{},
		}

		ctx := context.WithValue(context.Background(), ContextPeerFindDeadlineKey, time.Now().Add(time.Second))
		go func() {
			<-time.After(time.Second * 2)
			cm.Lock()
			cm.responsiblePeers = []peer.Peer{
				newTestPeer("1"),
			}
			cm.Unlock()
			close(cm.availableResponsiblePeers)
		}()
		peers, err := cm.GetResponsiblePeers(ctx)
		require.Error(t, err, ErrPeerFindDeadlineExceeded)
		require.Nil(t, peers)
	})
	t.Run("DeadlineNotExceeded", func(t *testing.T) {
		cm := &clientPeerManager{
			spaceId:                   "x",
			availableResponsiblePeers: make(chan struct{}),
			Mutex:                     sync.Mutex{},
		}

		ctx := context.WithValue(context.Background(), ContextPeerFindDeadlineKey, time.Now().Add(time.Second))
		go func() {
			<-time.After(time.Millisecond * 100)
			cm.Lock()
			cm.responsiblePeers = []peer.Peer{
				newTestPeer("1"),
			}
			cm.Unlock()
			close(cm.availableResponsiblePeers)
		}()
		peers, err := cm.GetResponsiblePeers(ctx)
		require.NoError(t, err, ErrPeerFindDeadlineExceeded)
		require.Len(t, peers, 1)
	})
}

func newTestPeer(id string) *testPeer {
	return &testPeer{
		id:     id,
		closed: make(chan struct{}),
	}
}

type testPeer struct {
	id     string
	closed chan struct{}
}

func (t *testPeer) SetTTL(ttl time.Duration) {
	return
}

func (t *testPeer) DoDrpc(ctx context.Context, do func(conn drpc.Conn) error) error {
	return fmt.Errorf("not implemented")
}

func (t *testPeer) AcquireDrpcConn(ctx context.Context) (drpc.Conn, error) {
	return nil, fmt.Errorf("not implemented")
}

func (t *testPeer) ReleaseDrpcConn(conn drpc.Conn) {}

func (t *testPeer) Context() context.Context {
	// TODO implement me
	panic("implement me")
}

func (t *testPeer) Accept() (conn net.Conn, err error) {
	// TODO implement me
	panic("implement me")
}

func (t *testPeer) Open(ctx context.Context) (conn net.Conn, err error) {
	// TODO implement me
	panic("implement me")
}

func (t *testPeer) Addr() string {
	return ""
}

func (t *testPeer) Id() string {
	return t.id
}

func (t *testPeer) TryClose(objectTTL time.Duration) (res bool, err error) {
	return true, t.Close()
}

func (t *testPeer) Close() error {
	select {
	case <-t.closed:
		return fmt.Errorf("already closed")
	default:
		close(t.closed)
	}
	return nil
}

func (t *testPeer) IsClosed() bool {
	select {
	case <-t.closed:
		return true
	default:
		return false
	}
}

func (t *testPeer) CloseChan() <-chan struct{} {
	return t.closed
}
