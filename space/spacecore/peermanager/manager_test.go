package peermanager

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/pool/mock_pool"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"storj.io/drpc"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus"
	"github.com/anyproto/anytype-heart/space/spacecore/peermanager/mock_peermanager"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
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

	t.Run("NoDeadline", func(t *testing.T) {
		cm := &clientPeerManager{
			spaceId:                   "x",
			availableResponsiblePeers: make(chan struct{}),
			Mutex:                     sync.Mutex{},
		}

		go func() {
			<-time.After(time.Millisecond * 100)
			cm.Lock()
			cm.responsiblePeers = []peer.Peer{
				newTestPeer("1"),
			}
			cm.Unlock()
			close(cm.availableResponsiblePeers)
		}()
		peers, err := cm.GetResponsiblePeers(context.Background())
		require.NoError(t, err, ErrPeerFindDeadlineExceeded)
		require.Len(t, peers, 1)
	})
}

func Test_fetchResponsiblePeers(t *testing.T) {
	spaceId := "spaceId"
	t.Run("node offline", func(t *testing.T) {
		// given
		f := newFixtureManager(t, spaceId)

		// when
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("failed"))
		status := domain.MakeSyncStatus(f.cm.spaceId, domain.Offline, domain.Null, domain.Objects)
		f.updater.EXPECT().SendUpdate(status)
		f.cm.fetchResponsiblePeers()

		// then
		f.updater.AssertCalled(t, "SendUpdate", status)
	})t.Run("no local peers", func(t *testing.T) {
		// given
		f := newFixtureManager(t, spaceId)

		// when
		f.conf.EXPECT().NodeIds(f.cm.spaceId).Return([]string{"id"})
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		f.cm.fetchResponsiblePeers()

		// then
		f.peerToPeerStatus.AssertNotCalled(t, "CheckPeerStatus")
	})
	t.Run("local peers connected", func(t *testing.T) {
		// given
		f := newFixtureManager(t, spaceId)
		f.store.UpdateLocalPeer("peerId", []string{spaceId})

		// when
		f.conf.EXPECT().NodeIds(f.cm.spaceId).Return([]string{"id"})
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		f.pool.EXPECT().Get(f.cm.ctx, "peerId").Return(newTestPeer("id1"), nil)
		f.cm.fetchResponsiblePeers()

		// then
		f.peerToPeerStatus.AssertNotCalled(t, "CheckPeerStatus")
	})
	t.Run("local peer not connected", func(t *testing.T) {
		// given
		f := newFixtureManager(t, spaceId)
		f.store.UpdateLocalPeer("peerId", []string{spaceId})
		f.peerToPeerStatus.EXPECT().CheckPeerStatus().Return()

		// when
		f.conf.EXPECT().NodeIds(f.cm.spaceId).Return([]string{"id"})
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		f.pool.EXPECT().Get(f.cm.ctx, "peerId").Return(nil, fmt.Errorf("error"))
		f.cm.fetchResponsiblePeers()

		// then
		f.peerToPeerStatus.AssertCalled(t, "CheckPeerStatus")
	})
}

func Test_getStreamResponsiblePeers(t *testing.T) {
	spaceId := "spaceId"
	t.Run("no local peers", func(t *testing.T) {
		// given
		f := newFixtureManager(t, spaceId)

		// when
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		f.pool.EXPECT().Get(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		peers, err := f.cm.getStreamResponsiblePeers(context.Background())

		// then
		assert.Nil(t, err)
		assert.Len(t, peers, 1)
		f.peerToPeerStatus.AssertNotCalled(t, "CheckPeerStatus")
	})
	t.Run("local peers connected", func(t *testing.T) {
		// given
		f := newFixtureManager(t, spaceId)
		f.store.UpdateLocalPeer("peerId", []string{spaceId})

		// when
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		f.pool.EXPECT().Get(f.cm.ctx, "peerId").Return(newTestPeer("id1"), nil)
		f.pool.EXPECT().Get(f.cm.ctx, "id").Return(newTestPeer("id"), nil)
		peers, err := f.cm.getStreamResponsiblePeers(context.Background())

		// then
		assert.Nil(t, err)
		assert.Len(t, peers, 2)
		f.peerToPeerStatus.AssertNotCalled(t, "CheckPeerStatus")
	})
	t.Run("local peer not connected", func(t *testing.T) {
		// given
		f := newFixtureManager(t, spaceId)
		f.store.UpdateLocalPeer("peerId", []string{spaceId})
		f.peerToPeerStatus.EXPECT().CheckPeerStatus().Return()

		// when
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		f.pool.EXPECT().Get(f.cm.ctx, "peerId").Return(nil, fmt.Errorf("error"))
		f.pool.EXPECT().Get(f.cm.ctx, "id").Return(newTestPeer("id"), nil)
		peers, err := f.cm.getStreamResponsiblePeers(context.Background())

		// then
		assert.Nil(t, err)
		assert.Len(t, peers, 1)
		f.peerToPeerStatus.AssertCalled(t, "CheckPeerStatus")
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

type fixture struct {
	cm               *clientPeerManager
	pool             *mock_pool.MockPool
	store            peerstore.PeerStore
	conf             *mock_nodeconf.MockService
	updater          *mock_peermanager.MockUpdater
	peerToPeerStatus *mock_peermanager.MockPeerToPeerStatus
}

func newFixtureManager(t *testing.T, spaceId string) *fixture {
	ctrl := gomock.NewController(t)
	pool := mock_pool.NewMockPool(ctrl)
	provider := &provider{pool: pool}
	conf := mock_nodeconf.NewMockService(ctrl)
	a := &app.App{}
	a.Register(conf)
	ns := nodestatus.NewNodeStatus()
	err := ns.Init(a)
	assert.Nil(t, err)
	store := peerstore.New()
	updater := mock_peermanager.NewMockUpdater(t)
	peerToPeerStatus := mock_peermanager.NewMockPeerToPeerStatus(t)
	cm := &clientPeerManager{
		p:                provider,
		spaceId:          spaceId,
		peerStore:        store,
		watchingPeers:    map[string]struct{}{},
		ctx:              context.Background(),
		nodeStatus:       ns,
		spaceSyncService: updater,
		peerToPeerStatus: peerToPeerStatus,
	}
	return &fixture{
		cm:               cm,
		pool:             pool,
		store:            store,
		conf:             conf,
		updater:          updater,
		peerToPeerStatus: peerToPeerStatus,
	}
}
