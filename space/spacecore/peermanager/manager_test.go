package peermanager

import (
	"context"
	"fmt"
	"net"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/net/pool/mock_pool"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/mock/gomock"
	"storj.io/drpc"

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
		f := newFixtureManager(t, spaceId)
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("failed"))
		f.updater.EXPECT().Refresh(spaceId)
		f.fetchAndWaitNode(t)
		require.Equal(t, f.cm.nodeStatus.GetNodeStatus("spaceId"), nodestatus.ConnectionError)
	})
	t.Run("no local peers", func(t *testing.T) {
		f := newFixtureManager(t, spaceId)
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		f.updater.EXPECT().Refresh(spaceId)
		f.fetchAndWaitNode(t)
		require.Equal(t, f.cm.nodeStatus.GetNodeStatus("spaceId"), nodestatus.Online)
	})
	t.Run("local peers connected", func(t *testing.T) {
		// given
		f := newFixtureManager(t, spaceId)
		f.store.UpdateLocalPeer("peerId", []string{spaceId})

		// when
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		f.pool.EXPECT().Get(gomock.Any(), "peerId").Return(newTestPeer("id1"), nil)
		f.updater.EXPECT().Refresh(spaceId)
		f.fetchAndWaitNode(t)

	})
	t.Run("local peer not connected", func(t *testing.T) {
		// given
		f := newFixtureManager(t, spaceId)
		f.store.UpdateLocalPeer("peerId", []string{spaceId})

		// when
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		f.pool.EXPECT().Get(gomock.Any(), "peerId").Return(nil, fmt.Errorf("error"))
		f.pool.EXPECT().Pick(gomock.Any(), "peerId").Return(nil, ocache.ErrNotExists)
		f.updater.EXPECT().Refresh(spaceId)
		f.fetchAndWaitNode(t)
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
	})
	t.Run("local peer not connected", func(t *testing.T) {
		// given
		f := newFixtureManager(t, spaceId)
		f.store.UpdateLocalPeer("peerId", []string{spaceId})

		// when
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		f.pool.EXPECT().Get(f.cm.ctx, "peerId").Return(nil, fmt.Errorf("error"))
		f.pool.EXPECT().Get(f.cm.ctx, "id").Return(newTestPeer("id"), nil)
		peers, err := f.cm.getStreamResponsiblePeers(context.Background())

		// then
		assert.Nil(t, err)
		assert.Len(t, peers, 1)
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
	ctx    context.Context
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

func (t *testPeer) ReleaseDrpcConn(ctx context.Context, conn drpc.Conn) {}

func (t *testPeer) Context() context.Context {
	return t.ctx
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
	responsibleNodeIdsUpdated := atomic.NewTime(time.Now().Add(time.Minute))
	cm := &clientPeerManager{
		responsibleNodeIds:        []string{"nodeId"},
		responsibleNodeIdsUpdated: *responsibleNodeIdsUpdated,
		p:                         provider,
		spaceId:                   spaceId,
		peerStore:                 store,
		watchingPeers:             map[string]struct{}{},
		rebuildResponsiblePeers:   make(chan struct{}, 1),
		nodeLookupDone:            make(chan struct{}, 1),
		ctx:                       context.Background(),
		nodeStatus:                ns,
		spaceSyncService:          updater,
		peerToPeerStatus:          peerToPeerStatus,
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

// fetchAndWaitNode runs one fetch and waits for its node verdict, which lands
// on a goroutine of its own.
func (f *fixture) fetchAndWaitNode(t *testing.T) {
	t.Helper()
	f.cm.fetchResponsiblePeers()
	f.waitNodeVerdict(t)
}

func (f *fixture) waitNodeVerdict(t *testing.T) {
	t.Helper()
	select {
	case <-f.cm.nodeLookupDone:
	case <-time.After(time.Second * 5):
		t.Fatal("node lookup must complete")
	}
}

func (f *fixture) waitFetchSignal(t *testing.T) {
	t.Helper()
	select {
	case <-f.cm.rebuildResponsiblePeers:
	case <-time.After(time.Second * 5):
		t.Fatal("a fetch must be signalled")
	}
}

type fakeConnectivity struct {
	offline atomic.Bool
	hook    func(online bool)
}

func (f *fakeConnectivity) RegisterConnectivityHook(hook func(online bool)) { f.hook = hook }
func (f *fakeConnectivity) IsOffline() bool                                 { return f.offline.Load() }

func Test_provider_connectivity(t *testing.T) {
	t.Run("no device component: never offline", func(t *testing.T) {
		p := &provider{}
		assert.False(t, p.isOffline())
	})
	t.Run("offline state mirrors connectivity", func(t *testing.T) {
		conn := &fakeConnectivity{}
		p := &provider{connectivity: conn, managers: map[*clientPeerManager]struct{}{}}
		assert.False(t, p.isOffline())
		conn.offline.Store(true)
		assert.True(t, p.isOffline())
	})
	t.Run("connectivity change signals rebuild on every registered manager", func(t *testing.T) {
		p := &provider{managers: map[*clientPeerManager]struct{}{}}
		cm1 := &clientPeerManager{rebuildResponsiblePeers: make(chan struct{}, 1)}
		cm2 := &clientPeerManager{rebuildResponsiblePeers: make(chan struct{}, 1)}
		p.registerManager(cm1)
		p.registerManager(cm2)

		p.onConnectivityChange(true)
		select {
		case <-cm1.rebuildResponsiblePeers:
		default:
			t.Fatal("manager 1 not signalled")
		}
		select {
		case <-cm2.rebuildResponsiblePeers:
		default:
			t.Fatal("manager 2 not signalled")
		}

		// unregistered managers are not signalled anymore
		p.unregisterManager(cm2)
		p.onConnectivityChange(false)
		select {
		case <-cm1.rebuildResponsiblePeers:
		default:
			t.Fatal("manager 1 not signalled after second change")
		}
		select {
		case <-cm2.rebuildResponsiblePeers:
			t.Fatal("unregistered manager must not be signalled")
		default:
		}
	})
}

func Test_signalRebuild_coalesces(t *testing.T) {
	cm := &clientPeerManager{rebuildResponsiblePeers: make(chan struct{}, 1)}
	cm.signalRebuild()
	cm.signalRebuild() // must not block
	<-cm.rebuildResponsiblePeers
	select {
	case <-cm.rebuildResponsiblePeers:
		t.Fatal("expected exactly one pending rebuild signal")
	default:
	}
}

// headSyncStub records DiffSync kicks; the rest of headsync.HeadSync is inert.
type headSyncStub struct {
	kicked chan struct{}
}

func (h *headSyncStub) Init(a *app.App) error           { return nil }
func (h *headSyncStub) Name() string                    { return "headsync" }
func (h *headSyncStub) Run(ctx context.Context) error   { return nil }
func (h *headSyncStub) Close(ctx context.Context) error { return nil }
func (h *headSyncStub) ExternalIds() []string           { return nil }
func (h *headSyncStub) AllIds() []string                { return nil }
func (h *headSyncStub) DiffSync(ctx context.Context) error {
	select {
	case h.kicked <- struct{}{}:
	default:
	}
	return nil
}
func (h *headSyncStub) HandleRangeRequest(ctx context.Context, req *spacesyncproto.HeadSyncRequest) (*spacesyncproto.HeadSyncResponse, error) {
	return nil, nil
}

func Test_fetchResponsiblePeers_reconnectKick(t *testing.T) {
	spaceId := "spaceId"
	waitKick := func(t *testing.T, h *headSyncStub) bool {
		select {
		case <-h.kicked:
			return true
		case <-time.After(time.Second):
			return false
		}
	}
	t.Run("ConnectionError to Online kicks one diff round", func(t *testing.T) {
		f := newFixtureManager(t, spaceId)
		hs := &headSyncStub{kicked: make(chan struct{}, 1)}
		f.cm.headSync = hs
		f.cm.nodeStatus.SetNodesStatus(spaceId, nodestatus.ConnectionError)
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		f.updater.EXPECT().Refresh(spaceId)
		f.fetchAndWaitNode(t)
		assert.True(t, waitKick(t, hs), "reconnect must kick an immediate diff round")
	})
	t.Run("already online: no kick", func(t *testing.T) {
		f := newFixtureManager(t, spaceId)
		hs := &headSyncStub{kicked: make(chan struct{}, 1)}
		f.cm.headSync = hs
		f.cm.nodeStatus.SetNodesStatus(spaceId, nodestatus.Online)
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		f.updater.EXPECT().Refresh(spaceId)
		f.fetchAndWaitNode(t)
		select {
		case <-hs.kicked:
			t.Fatal("steady online state must not kick diff rounds")
		case <-time.After(time.Millisecond * 100):
		}
	})
	t.Run("still offline: no kick", func(t *testing.T) {
		f := newFixtureManager(t, spaceId)
		hs := &headSyncStub{kicked: make(chan struct{}, 1)}
		f.cm.headSync = hs
		f.cm.nodeStatus.SetNodesStatus(spaceId, nodestatus.ConnectionError)
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("offline"))
		f.updater.EXPECT().Refresh(spaceId)
		f.fetchAndWaitNode(t)
		select {
		case <-hs.kicked:
			t.Fatal("no kick while the node is still unreachable")
		case <-time.After(time.Millisecond * 100):
		}
	})
}

func TestClientPeerManager_GetResponsiblePeers_ClosedPeersNotServed(t *testing.T) {
	t.Run("waits for rebuild instead of serving closed peers", func(t *testing.T) {
		// after a pool flush the cached list still holds closed peers until
		// fetchResponsiblePeers swaps it; they must not be handed out
		cm := &clientPeerManager{spaceId: "x", Mutex: sync.Mutex{}}
		dead := newTestPeer("dead")
		require.NoError(t, dead.Close())
		cm.responsiblePeers = []peer.Peer{dead}

		live := newTestPeer("live")
		go func() {
			time.Sleep(time.Millisecond * 100)
			// mimic fetchResponsiblePeers completing after a re-dial
			cm.Lock()
			cm.responsiblePeers = []peer.Peer{live}
			if cm.availableResponsiblePeers != nil {
				close(cm.availableResponsiblePeers)
				cm.availableResponsiblePeers = nil
			}
			cm.Unlock()
		}()

		ctx := context.WithValue(context.Background(), ContextPeerFindDeadlineKey, time.Now().Add(time.Second*2))
		peers, err := cm.GetResponsiblePeers(ctx)
		require.NoError(t, err)
		require.Len(t, peers, 1)
		assert.Equal(t, "live", peers[0].Id())
	})
	t.Run("only closed peers and no rebuild: deadline exceeded", func(t *testing.T) {
		cm := &clientPeerManager{spaceId: "x", Mutex: sync.Mutex{}}
		dead := newTestPeer("dead")
		require.NoError(t, dead.Close())
		cm.responsiblePeers = []peer.Peer{dead}

		ctx := context.WithValue(context.Background(), ContextPeerFindDeadlineKey, time.Now().Add(time.Millisecond*200))
		peers, err := cm.GetResponsiblePeers(ctx)
		require.ErrorIs(t, err, ErrPeerFindDeadlineExceeded)
		require.Nil(t, peers)
	})
	t.Run("live subset is served, closed peers dropped", func(t *testing.T) {
		cm := &clientPeerManager{spaceId: "x", Mutex: sync.Mutex{}}
		dead := newTestPeer("dead")
		require.NoError(t, dead.Close())
		live := newTestPeer("live")
		cm.responsiblePeers = []peer.Peer{dead, live}

		peers, err := cm.GetResponsiblePeers(context.Background())
		require.NoError(t, err)
		require.Len(t, peers, 1)
		assert.Equal(t, "live", peers[0].Id())
	})
}

func Test_nextCheckInterval(t *testing.T) {
	spaceId := "spaceId"
	f := newFixtureManager(t, spaceId)
	conn := &fakeConnectivity{}
	f.cm.p.connectivity = conn

	assert.Equal(t, responsiblePeersCheckInterval, f.cm.nextCheckInterval(), "online: full cadence")

	conn.offline.Store(true)
	assert.Equal(t, responsiblePeersCheckIntervalOffline, f.cm.nextCheckInterval(), "offline, no LAN peers: back off")

	// LAN peers present: "no internet" must not slow local-only P2P sync
	f.store.UpdateLocalPeer("peerId", []string{spaceId})
	assert.Equal(t, responsiblePeersCheckInterval, f.cm.nextCheckInterval(), "offline with LAN peers: full cadence")
}

func Test_provider_ProvideStat(t *testing.T) {
	conn := &fakeConnectivity{}
	p := &provider{connectivity: conn, managers: map[*clientPeerManager]struct{}{}}
	cm1 := &clientPeerManager{rebuildResponsiblePeers: make(chan struct{}, 1)}
	cm2 := &clientPeerManager{rebuildResponsiblePeers: make(chan struct{}, 1)}
	p.registerManager(cm1)
	p.registerManager(cm2)

	p.onConnectivityChange(true)
	p.onConnectivityChange(false)
	p.stats.reconnectDiffKicks.Inc()
	p.stats.closedPeersFiltered.Add(3)
	p.stats.localPeerSignals.Add(2)

	st := p.ProvideStat().(providerStat)
	assert.Equal(t, 2, st.Managers)
	assert.False(t, st.Offline)
	assert.Equal(t, int64(2), st.ConnectivityEvents)
	assert.Equal(t, int64(4), st.RebuildSignals, "one signal per manager per event")
	assert.Equal(t, int64(1), st.ReconnectDiffKicks)
	assert.Equal(t, int64(3), st.ClosedPeersFiltered)
	assert.Equal(t, int64(2), st.LocalPeerSignals)
}

func Test_nextCheckInterval_fastRetryAfterFailure(t *testing.T) {
	spaceId := "spaceId"
	f := newFixtureManager(t, spaceId)

	f.cm.nodeStatus.SetNodesStatus(spaceId, nodestatus.Online)
	assert.Equal(t, responsiblePeersCheckInterval, f.cm.nextCheckInterval(), "online + healthy: full cadence")

	// a failed node lookup while online was likely a bounded wait behind a
	// slow dial — retry quickly to pick up a conn landed by another space
	f.cm.nodeStatus.SetNodesStatus(spaceId, nodestatus.ConnectionError)
	f.cm.consecutiveFetchFailures.Store(1)
	assert.Equal(t, responsiblePeersRetryInterval, f.cm.nextCheckInterval(), "online + failed lookup: fast retry")

	// steady-state failure (LAN without internet, captive portal): the fast
	// window decays so nodes aren't hammered every 5s forever while local-only
	// P2P keeps syncing
	f.cm.consecutiveFetchFailures.Store(fastRetryMaxAttempts + 1)
	assert.Equal(t, responsiblePeersCheckInterval, f.cm.nextCheckInterval(), "steady failure: back to normal cadence")

	// a fresh connectivity event (rebuild signal) reopens the fast window
	// even after steady failure — e.g. a LAN-only device walking out to cellular
	f.cm.rebuildResponsiblePeers = make(chan struct{}, 1)
	f.cm.signalRebuild()
	assert.Equal(t, responsiblePeersRetryInterval, f.cm.nextCheckInterval(), "rebuild signal: fast window reopened")
}

// Regression guard for the GO-7379 dial-budget starvation: the node lookup must
// NOT carry a deadline. GetOneOf walks every responsible node x scheme x
// address, and any bound smaller than that whole product truncates the walk --
// one address that accepts UDP and never replies then consumes the budget and
// the yamux fallback is never attempted. Per-attempt transport caps already
// guarantee the walk terminates. A bound may only return once GO-7410 lands.
func Test_fetchResponsiblePeers_nodeLookupIsNotTruncated(t *testing.T) {
	spaceId := "spaceId"
	f := newFixtureManager(t, spaceId)
	f.updater.EXPECT().Refresh(spaceId)
	f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, ids []string) (peer.Peer, error) {
			_, ok := ctx.Deadline()
			assert.False(t, ok, "node lookup must not carry a deadline: it truncates the address walk before the yamux fallback")
			return newTestPeer("id"), nil
		})
	f.fetchAndWaitNode(t)
}

func Test_fetchResponsiblePeers_nodePublishedBeforeLocalDials(t *testing.T) {
	// the field failure: a stale mDNS peer from the previous network parks the
	// fetch in an unroutable dial while a healthy node connection sits idle —
	// the node peer must be served to waiters before the local dials settle
	spaceId := "spaceId"
	f := newFixtureManager(t, spaceId)
	f.store.UpdateLocalPeer("staleWifiPeer", []string{spaceId})
	f.updater.EXPECT().Refresh(spaceId)

	localDialStarted := make(chan struct{})
	releaseLocalDial := make(chan struct{})
	f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("node"), nil)
	f.pool.EXPECT().Get(gomock.Any(), "staleWifiPeer").DoAndReturn(
		func(ctx context.Context, id string) (peer.Peer, error) {
			close(localDialStarted)
			select {
			case <-releaseLocalDial:
			case <-ctx.Done():
			}
			return nil, fmt.Errorf("unroutable from cellular")
		})
	f.pool.EXPECT().Pick(gomock.Any(), "staleWifiPeer").Return(nil, ocache.ErrNotExists)

	fetchDone := make(chan struct{})
	go func() {
		defer close(fetchDone)
		f.cm.fetchResponsiblePeers()
	}()

	<-localDialStarted
	// local dial is parked; the node peer must already be served
	ctx := context.WithValue(context.Background(), ContextPeerFindDeadlineKey, time.Now().Add(time.Second))
	peers, err := f.cm.GetResponsiblePeers(ctx)
	require.NoError(t, err, "node peer must be available while local dials are in flight")
	require.Len(t, peers, 1)
	assert.Equal(t, "node", peers[0].Id())

	close(releaseLocalDial)
	select {
	case <-fetchDone:
	case <-time.After(time.Second * 2):
		t.Fatal("fetch must finish")
	}
	// the failed local dial must evict the stale peer
	assert.Empty(t, f.store.LocalPeerIds(spaceId), "stale local peer must be removed after a failed bounded dial")
	f.waitNodeVerdict(t)
}

// Local (mDNS) dials are bounded PER DIAL, not by one budget shared across the
// loop. A shared budget is the GO-7409 defect in miniature: the first stale
// entry consumes it and every peer behind it is evicted without being tried --
// and in LocalOnly mode, where there are no nodes, that costs sync outright.
func Test_fetchResponsiblePeers_boundsEachLocalDialSeparately(t *testing.T) {
	spaceId := "spaceId"
	f := newFixtureManager(t, spaceId)
	f.store.UpdateLocalPeer("stalePeer", []string{spaceId})
	f.store.UpdateLocalPeer("healthyPeer", []string{spaceId})
	f.updater.EXPECT().Refresh(spaceId)
	f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("node"), nil)

	// the stale peer burns its whole budget, as an unroutable Wi-Fi-era entry does
	f.pool.EXPECT().Get(gomock.Any(), "stalePeer").DoAndReturn(
		func(ctx context.Context, id string) (peer.Peer, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})
	f.pool.EXPECT().Pick(gomock.Any(), "stalePeer").Return(nil, ocache.ErrNotExists)
	// the healthy peer behind it must still get a full budget of its own
	f.pool.EXPECT().Get(gomock.Any(), "healthyPeer").DoAndReturn(
		func(ctx context.Context, id string) (peer.Peer, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok, "local dials must carry a deadline")
			assert.Greater(t, time.Until(deadline), localPeerDialTimeout/2,
				"each local dial needs its own budget: a stale peer must not starve the ones behind it")
			return newTestPeer("healthyPeer"), nil
		})

	f.fetchAndWaitNode(t)

	peers, err := f.cm.GetResponsiblePeers(context.Background())
	require.NoError(t, err)
	require.Len(t, peers, 2, "node and healthy local peer must both be served")
	assert.Equal(t, []string{"healthyPeer"}, f.store.LocalPeerIds(spaceId),
		"only the stale peer may be evicted")
}

// The field failure (2026-10-12, GO-7581 device test): after an interface loss
// cut the internet the node walk blocked for 20-70s while the cable peer's
// live connection sat in the pool. The local pass must serve it without
// waiting for the node verdict.
func Test_fetchResponsiblePeers_localPeersServedWhileNodeLookupBlocks(t *testing.T) {
	// given
	spaceId := "spaceId"
	f := newFixtureManager(t, spaceId)
	f.store.UpdateLocalPeer("cablePeer", []string{spaceId})
	f.updater.EXPECT().Refresh(spaceId)
	releaseNode := make(chan struct{})
	f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, ids []string) (peer.Peer, error) {
			<-releaseNode
			return nil, fmt.Errorf("unable to connect")
		})
	// an already-connected peer: the pool serves it without a dial
	f.pool.EXPECT().Get(gomock.Any(), "cablePeer").Return(newTestPeer("cablePeer"), nil)
	want := []string{"cablePeer"}

	// when
	fetchDone := make(chan struct{})
	go func() {
		defer close(fetchDone)
		f.cm.fetchResponsiblePeers()
	}()

	// then: while the node walk is parked, the cable peer is served
	ctx := context.WithValue(context.Background(), ContextPeerFindDeadlineKey, time.Now().Add(time.Second))
	peers, err := f.cm.GetResponsiblePeers(ctx)
	close(releaseNode)
	require.NoError(t, err, "local peer must be served while the node walk is in flight")
	assert.Equal(t, want, peerIds(peers))

	// and the failed node verdict does not take it away
	f.waitNodeVerdict(t)
	<-fetchDone
	peers, err = f.cm.GetResponsiblePeers(context.Background())
	require.NoError(t, err)
	assert.Equal(t, want, peerIds(peers))
	assert.Equal(t, nodestatus.ConnectionError, f.cm.nodeStatus.GetNodeStatus(spaceId))
}

// A fetch while a node lookup is in flight (cadence tick, LAN signal,
// connectivity signal) runs only the local pass: lookups never stack.
func Test_fetchResponsiblePeers_nodeLookupSingleFlight(t *testing.T) {
	// given
	spaceId := "spaceId"
	f := newFixtureManager(t, spaceId)
	f.store.UpdateLocalPeer("localPeer", []string{spaceId})
	f.updater.EXPECT().Refresh(spaceId).Times(2)
	releaseNode := make(chan struct{})
	f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, ids []string) (peer.Peer, error) {
			<-releaseNode
			return newTestPeer("node"), nil
		})
	// one local pass per fetch
	f.pool.EXPECT().Get(gomock.Any(), "localPeer").Times(4).Return(newTestPeer("localPeer"), nil)

	// when
	f.cm.fetchResponsiblePeers()
	f.cm.fetchResponsiblePeers()
	f.cm.fetchResponsiblePeers()
	close(releaseNode)
	f.waitNodeVerdict(t)

	// then
	peers, err := f.cm.GetResponsiblePeers(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"node", "localPeer"}, peerIds(peers))

	// the lookup is over: the next fetch starts a new one
	f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("node"), nil)
	f.fetchAndWaitNode(t)
}

// A failed lookup drops the node from the list but keeps the local peers, and
// a later success adds it back: the two sources are independent.
func Test_fetchResponsiblePeers_nodeVerdictAndLocalPassAreIndependent(t *testing.T) {
	spaceId := "spaceId"
	f := newFixtureManager(t, spaceId)
	f.store.UpdateLocalPeer("localPeer", []string{spaceId})
	f.updater.EXPECT().Refresh(spaceId).Times(3)
	f.pool.EXPECT().Get(gomock.Any(), "localPeer").Times(3).Return(newTestPeer("localPeer"), nil)

	for _, tc := range []struct {
		name         string
		nodeErr      error
		wantPeers    []string
		wantFailures int32
	}{
		{name: "node found", wantPeers: []string{"node", "localPeer"}},
		{name: "node lost", nodeErr: fmt.Errorf("unable to connect"), wantPeers: []string{"localPeer"}, wantFailures: 1},
		{name: "node back", wantPeers: []string{"node", "localPeer"}},
	} {
		// given
		if tc.nodeErr != nil {
			f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(nil, tc.nodeErr)
		} else {
			f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("node"), nil)
		}

		// when
		f.fetchAndWaitNode(t)

		// then
		peers, err := f.cm.GetResponsiblePeers(context.Background())
		require.NoError(t, err, tc.name)
		assert.Equal(t, tc.wantPeers, peerIds(peers), tc.name)
		assert.Equal(t, tc.wantFailures, f.cm.consecutiveFetchFailures.Load(), tc.name)
	}
}

// A pass serves each local peer the moment it lands and keeps serving the
// ones it has not reached yet: a stale entry burning its dial budget must
// neither hold back the peers found before it nor hide the ones listed after.
func Test_fetchLocalPeers_servesPeersAsTheyLand(t *testing.T) {
	// given: peerA was just discovered; the previous pass served peerB and a
	// peer whose connection died with the old network
	spaceId := "spaceId"
	f := newFixtureManager(t, spaceId)
	peerA, stale, peerB := newTestPeer("peerA"), newTestPeer("stalePeer"), newTestPeer("peerB")
	for _, p := range []*testPeer{peerA, stale, peerB} {
		f.store.UpdateLocalPeer(p.Id(), []string{spaceId})
	}
	f.cm.setLocalPeers([]peer.Peer{stale, peerB})
	require.NoError(t, stale.Close())

	dialStarted := make(chan struct{})
	releaseDial := make(chan struct{})
	f.pool.EXPECT().Get(gomock.Any(), "peerA").Return(peerA, nil)
	f.pool.EXPECT().Get(gomock.Any(), "stalePeer").DoAndReturn(
		func(ctx context.Context, id string) (peer.Peer, error) {
			close(dialStarted)
			<-releaseDial
			return nil, context.DeadlineExceeded
		})
	f.pool.EXPECT().Pick(gomock.Any(), "stalePeer").Return(nil, ocache.ErrNotExists)
	f.pool.EXPECT().Get(gomock.Any(), "peerB").Return(peerB, nil)
	want := []string{"peerA", "peerB"}

	// when
	passDone := make(chan struct{})
	go func() {
		defer close(passDone)
		f.cm.fetchLocalPeers()
	}()
	<-dialStarted

	// then: while the pass is parked on the stale peer
	peers, err := f.cm.GetResponsiblePeers(context.Background())
	require.NoError(t, err)
	assert.Contains(t, peerIds(peers), "peerA", "a peer the pass has found must be served at once")
	assert.Contains(t, peerIds(peers), "peerB", "a peer the pass has not reached yet must stay served")

	// and after the pass the list is exactly what it found
	close(releaseDial)
	select {
	case <-passDone:
	case <-time.After(time.Second * 5):
		t.Fatal("the local pass must finish")
	}
	peers, err = f.cm.GetResponsiblePeers(context.Background())
	require.NoError(t, err)
	assert.Equal(t, want, peerIds(peers))
	assert.Equal(t, want, f.store.LocalPeerIds(spaceId), "only the stale peer may be evicted")
}

// After a flush the local pass dials a LAN peer while the peer reconnects to
// us; the pool does not hand the inbound connection to the dial under way. A
// failed dial must not evict a peer the pool holds a live connection to: the
// pass serves that connection instead.
func Test_fetchLocalPeers_servesPeerThatConnectedDuringFailedDial(t *testing.T) {
	// given
	spaceId := "spaceId"
	f := newFixtureManager(t, spaceId)
	f.store.UpdateLocalPeer("cablePeer", []string{spaceId})
	inbound := newTestPeer("cablePeer")
	f.pool.EXPECT().Get(gomock.Any(), "cablePeer").Return(nil, fmt.Errorf("dial tcp: i/o timeout"))
	f.pool.EXPECT().Pick(gomock.Any(), "cablePeer").Return(inbound, nil)
	want := []string{"cablePeer"}

	// when
	f.cm.fetchLocalPeers()

	// then
	ctx := context.WithValue(context.Background(), ContextPeerFindDeadlineKey, time.Now().Add(time.Second))
	peers, err := f.cm.GetResponsiblePeers(ctx)
	require.NoError(t, err, "the inbound connection must be served")
	assert.Equal(t, want, peerIds(peers))
	assert.Equal(t, want, f.store.LocalPeerIds(spaceId), "a peer with a live connection must not be evicted")
}

// Closing the manager mid-fetch fails every pool call with the manager's own
// cancellation; that must not evict the LAN peers from the store, which is
// shared by every space.
func Test_fetchLocalPeers_noEvictionWhenClosing(t *testing.T) {
	// given
	spaceId := "spaceId"
	f := newFixtureManager(t, spaceId)
	f.store.UpdateLocalPeer("localPeer", []string{spaceId})
	ctx, cancel := context.WithCancel(context.Background())
	f.cm.ctx = ctx
	cancel()
	f.pool.EXPECT().Get(gomock.Any(), "localPeer").DoAndReturn(
		func(ctx context.Context, id string) (peer.Peer, error) {
			return nil, ctx.Err()
		})

	// when
	f.cm.fetchLocalPeers()

	// then
	assert.Equal(t, []string{"localPeer"}, f.store.LocalPeerIds(spaceId), "a closing manager must not evict local peers")
}

// The node lookup outlives Close until GetOneOf returns (a cgo DNS resolve does
// not honour the ctx). A result that lands after Close is void: no status
// change, no served node, no sync-status refresh, no reconnect kick.
func Test_lookupNode_resultAfterCloseIsVoid(t *testing.T) {
	// given: a lookup in flight while the node is unreachable
	spaceId := "spaceId"
	f := newFixtureManager(t, spaceId)
	f.cm.ctx, f.cm.ctxCancel = context.WithCancel(context.Background())
	f.cm.nodeStatus.SetNodesStatus(spaceId, nodestatus.ConnectionError)
	hs := &headSyncStub{kicked: make(chan struct{}, 1)}
	f.cm.headSync = hs
	lookupStarted := make(chan struct{})
	releaseNode := make(chan struct{})
	f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, ids []string) (peer.Peer, error) {
			close(lookupStarted)
			<-releaseNode
			return newTestPeer("node"), nil
		})
	f.peerToPeerStatus.EXPECT().UnregisterSpace(spaceId)
	// no Refresh expected: the mock fails the test on an unexpected call
	f.cm.startNodeLookup()
	<-lookupStarted

	// when: the manager closes, then the walk returns a node
	require.NoError(t, f.cm.Close(context.Background()))
	close(releaseNode)
	f.waitNodeVerdict(t)

	// then
	assert.Equal(t, nodestatus.ConnectionError, f.cm.nodeStatus.GetNodeStatus(spaceId))
	f.cm.Lock()
	assert.Nil(t, f.cm.nodePeer)
	assert.Empty(t, f.cm.responsiblePeers)
	f.cm.Unlock()
	select {
	case <-hs.kicked:
		t.Fatal("a closed manager must not kick a diff round")
	default:
	}
	assert.Equal(t, int64(0), f.cm.p.stats.reconnectDiffKicks.Load())
}

// The node verdict decides the cadence. With the lookup off the loop, a failed
// verdict landing while the loop waits re-arms the timer, so the retry comes
// 5s after the failure, as when the loop waited the lookup out, and not at the
// 20s tick armed before it.
func Test_manageResponsiblePeers_retryCountsFromNodeVerdict(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// given
		spaceId := "spaceId"
		f := newFixtureManager(t, spaceId)
		f.cm.ctx, f.cm.ctxCancel = context.WithCancel(context.Background())
		f.updater.EXPECT().Refresh(spaceId).Times(2)
		start := time.Now()
		lookupStarts := make(chan time.Duration, 2)
		gomock.InOrder(
			f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, ids []string) (peer.Peer, error) {
					lookupStarts <- time.Since(start)
					// an offline walk outlasting the tick armed while online
					time.Sleep(30 * time.Second)
					return nil, fmt.Errorf("unable to connect")
				}),
			f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, ids []string) (peer.Peer, error) {
					lookupStarts <- time.Since(start)
					return newTestPeer("node"), nil
				}),
		)
		want := []time.Duration{0, 35 * time.Second}

		// when
		go f.cm.manageResponsiblePeers()
		got := []time.Duration{<-lookupStarts, <-lookupStarts}
		synctest.Wait()
		f.cm.ctxCancel()

		// then
		assert.Equal(t, want, got, "the 20s tick must not start a second lookup; the retry must come 5s after the 30s verdict")
		assert.Equal(t, nodestatus.Online, f.cm.nodeStatus.GetNodeStatus(spaceId))
	})
}

// After a flush closes the served node, its watcher signals one fetch. While
// the replacement lookup still walks (20-70s offline), that fetch must not
// serve the closed node again: a fresh watcher would find it closed at once
// and signal another fetch, spinning the loop until the verdict lands.
func Test_manageResponsiblePeers_closedNodeCostsOnePass(t *testing.T) {
	for _, tc := range []struct {
		name       string
		localPeers []string
		wantServed []string
	}{
		{name: "no LAN peers", wantServed: []string{}},
		{name: "live cable peer", localPeers: []string{"cablePeer"}, wantServed: []string{"cablePeer"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// given: the node served before the flush, and a replacement
				// lookup that walks on for the rest of the test
				spaceId := "spaceId"
				f := newFixtureManager(t, spaceId)
				f.cm.ctx, f.cm.ctxCancel = context.WithCancel(context.Background())
				store := &passCountingStore{PeerStore: f.store, limit: 10, stop: f.cm.ctxCancel}
				f.cm.peerStore = store
				for _, id := range tc.localPeers {
					f.store.UpdateLocalPeer(id, []string{spaceId})
					f.pool.EXPECT().Get(gomock.Any(), id).Return(newTestPeer(id), nil).AnyTimes()
				}
				f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, ids []string) (peer.Peer, error) {
						<-ctx.Done()
						return nil, ctx.Err()
					})
				node := newTestPeer("node")
				f.cm.setNodePeer(node)
				go f.cm.manageResponsiblePeers()
				synctest.Wait()
				require.Equal(t, int32(1), store.passes.Load())

				// when: the flush closes the node
				require.NoError(t, node.Close())
				synctest.Wait()

				// then: one pass for the closure, then idle until the cadence tick
				assert.Equal(t, int32(2), store.passes.Load(), "a closed node must cost one pass, not a loop")
				f.cm.Lock()
				served := peerIds(f.cm.responsiblePeers)
				f.cm.Unlock()
				assert.Equal(t, tc.wantServed, served, "a closed peer must not be served")
				time.Sleep(responsiblePeersCheckInterval)
				synctest.Wait()
				assert.Equal(t, int32(3), store.passes.Load(), "the next pass comes on the cadence tick")
				f.cm.ctxCancel()
			})
		})
	}
}

// A node that is closed when its lookup returns (a flush raced it), or that
// closes before the lookup has finished, must be looked up again at once. The
// fetch its closure asks for can land while the lookup still holds the
// single-flight slot (here: held in Refresh) and then runs only the local
// pass, so the space would stay without a node until the 20s tick.
func Test_manageResponsiblePeers_closedNodeIsLookedUpAgainAtOnce(t *testing.T) {
	for _, tc := range []struct {
		name            string
		closedOnArrival bool
	}{
		{name: "closed on arrival", closedOnArrival: true},
		{name: "closes before the lookup finishes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// given
				spaceId := "spaceId"
				f := newFixtureManager(t, spaceId)
				f.cm.ctx, f.cm.ctxCancel = context.WithCancel(context.Background())
				first, replacement := newTestPeer("node"), newTestPeer("node")
				if tc.closedOnArrival {
					require.NoError(t, first.Close())
				}
				start := time.Now()
				lookupStarts := make(chan time.Duration, 3)
				gomock.InOrder(
					f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).DoAndReturn(
						func(ctx context.Context, ids []string) (peer.Peer, error) {
							lookupStarts <- time.Since(start)
							return first, nil
						}),
					f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).DoAndReturn(
						func(ctx context.Context, ids []string) (peer.Peer, error) {
							lookupStarts <- time.Since(start)
							return replacement, nil
						}),
				)
				releaseRefresh := make(chan struct{})
				f.updater.EXPECT().Refresh(spaceId).Run(func(string) { <-releaseRefresh }).Once()
				f.updater.EXPECT().Refresh(spaceId).Once()
				go f.cm.manageResponsiblePeers()
				synctest.Wait()
				if !tc.closedOnArrival {
					require.NoError(t, first.Close())
					synctest.Wait()
				}

				// when: the first lookup finishes
				close(releaseRefresh)
				synctest.Wait()

				// then
				var got []time.Duration
				for len(lookupStarts) > 0 {
					got = append(got, <-lookupStarts)
				}
				assert.Equal(t, []time.Duration{0, 0}, got, "the replacement lookup must start at once, not on the 20s tick")
				f.cm.Lock()
				served := slices.Clone(f.cm.responsiblePeers)
				f.cm.Unlock()
				require.Len(t, served, 1)
				assert.Same(t, replacement, served[0])
				f.cm.ctxCancel()
			})
		})
	}
}

// The LAN twin: a connection that is closed by the time the pass serves it
// (the peer reconnecting) is left out and asks for another pass at once, which
// serves the new connection, while the node lookup walks on.
func Test_manageResponsiblePeers_closedLocalPeerIsFetchedAgainAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// given
		spaceId := "spaceId"
		f := newFixtureManager(t, spaceId)
		f.cm.ctx, f.cm.ctxCancel = context.WithCancel(context.Background())
		f.store.UpdateLocalPeer("cablePeer", []string{spaceId})
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, ids []string) (peer.Peer, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			})
		closedConn, replacement := newTestPeer("cablePeer"), newTestPeer("cablePeer")
		require.NoError(t, closedConn.Close())
		gomock.InOrder(
			f.pool.EXPECT().Get(gomock.Any(), "cablePeer").Return(closedConn, nil),
			f.pool.EXPECT().Get(gomock.Any(), "cablePeer").Return(replacement, nil),
		)

		// when
		go f.cm.manageResponsiblePeers()
		synctest.Wait()

		// then: served without waiting for the 20s tick
		f.cm.Lock()
		served := slices.Clone(f.cm.responsiblePeers)
		f.cm.Unlock()
		require.Len(t, served, 1)
		assert.Same(t, replacement, served[0])
		f.cm.ctxCancel()
	})
}

// passCountingStore counts local passes and stops the manager past limit, so a
// fetch storm fails the test instead of spinning the synctest bubble forever.
type passCountingStore struct {
	peerstore.PeerStore
	passes atomic.Int32
	limit  int32
	stop   context.CancelFunc
}

func (s *passCountingStore) LocalPeerIds(spaceId string) []string {
	if s.passes.Inc() > s.limit {
		s.stop()
	}
	return s.PeerStore.LocalPeerIds(spaceId)
}

// A peer that closes releases its watch slot before it signals the fetch: the
// fetch may serve a new connection to the same peer at once (an inbound
// reconnect is already in the pool), and that connection must be watched too,
// or its own close goes unnoticed until the next cadence tick.
func Test_watchPeer_reconnectedPeerIsWatched(t *testing.T) {
	// given
	spaceId := "spaceId"
	f := newFixtureManager(t, spaceId)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.cm.ctx = ctx
	old := newTestPeer("lanPeer")
	f.cm.serveLocalPeer(old)

	// when: the connection closes while a publish holds the manager
	f.cm.Lock()
	require.NoError(t, old.Close())
	select {
	case <-f.cm.rebuildResponsiblePeers:
		t.Fatal("the fetch must not be signalled before the watch slot is released")
	case <-time.After(time.Millisecond * 100):
	}
	f.cm.Unlock()
	f.waitFetchSignal(t)
	reconnected := newTestPeer("lanPeer")
	f.cm.serveLocalPeer(reconnected)
	require.NoError(t, reconnected.Close())

	// then
	f.waitFetchSignal(t)
}

// A LAN peer joining a space signals that space's manager, so the peer is
// served now rather than at the next cadence tick. Only a real change does:
// the store drops an unchanged list, and removals are the managers' own doing.
func Test_provider_onLocalPeerUpdated(t *testing.T) {
	// given
	ctrl := gomock.NewController(t)
	poolService := mock_pool.NewMockService(ctrl)
	poolService.EXPECT().Name().Return(pool.CName).AnyTimes()
	store := peerstore.New()
	a := &app.App{}
	a.Register(store)
	a.Register(poolService)
	p := New().(*provider)
	require.NoError(t, p.Init(a))
	cmA := &clientPeerManager{spaceId: "A", rebuildResponsiblePeers: make(chan struct{}, 1)}
	cmB := &clientPeerManager{spaceId: "B", rebuildResponsiblePeers: make(chan struct{}, 1)}
	p.registerManager(cmA)
	p.registerManager(cmB)
	cmA.consecutiveFetchFailures.Store(fastRetryMaxAttempts + 1)
	signalled := func() (spaceIds []string) {
		for _, cm := range []*clientPeerManager{cmA, cmB} {
			select {
			case <-cm.rebuildResponsiblePeers:
				spaceIds = append(spaceIds, cm.spaceId)
			default:
			}
		}
		return spaceIds
	}

	for _, tc := range []struct {
		name   string
		update func()
		want   []string
	}{
		{name: "joins A", update: func() { store.UpdateLocalPeer("peer", []string{"A"}) }, want: []string{"A"}},
		{name: "unchanged list", update: func() { store.UpdateLocalPeer("peer", []string{"A"}) }},
		{name: "joins B too", update: func() { store.UpdateLocalPeer("peer", []string{"A", "B"}) }, want: []string{"B"}},
		{name: "leaves A", update: func() { store.UpdateLocalPeer("peer", []string{"B"}) }},
		{name: "evicted", update: func() { store.RemoveLocalPeer("peer") }},
		{name: "back after eviction", update: func() { store.UpdateLocalPeer("peer", []string{"A", "B"}) }, want: []string{"A", "B"}},
	} {
		// when
		tc.update()

		// then
		assert.Equal(t, tc.want, signalled(), tc.name)
	}
	assert.Equal(t, fastRetryMaxAttempts+1, int(cmA.consecutiveFetchFailures.Load()), "a LAN event opens no fast-retry window")
	assert.Equal(t, int64(4), p.stats.localPeerSignals.Load())
}

func peerIds(peers []peer.Peer) []string {
	ids := make([]string, 0, len(peers))
	for _, p := range peers {
		ids = append(ids, p.Id())
	}
	return ids
}
