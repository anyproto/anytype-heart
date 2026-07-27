package peermanager

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net/peer"
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
		f.cm.fetchResponsiblePeers()
		require.Equal(t, f.cm.nodeStatus.GetNodeStatus("spaceId"), nodestatus.ConnectionError)
	})
	t.Run("no local peers", func(t *testing.T) {
		f := newFixtureManager(t, spaceId)
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		f.updater.EXPECT().Refresh(spaceId)
		f.cm.fetchResponsiblePeers()
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
		f.cm.fetchResponsiblePeers()

	})
	t.Run("local peer not connected", func(t *testing.T) {
		// given
		f := newFixtureManager(t, spaceId)
		f.store.UpdateLocalPeer("peerId", []string{spaceId})

		// when
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		f.pool.EXPECT().Get(gomock.Any(), "peerId").Return(nil, fmt.Errorf("error"))
		f.updater.EXPECT().Refresh(spaceId)
		f.cm.fetchResponsiblePeers()
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
		f.cm.fetchResponsiblePeers()
		assert.True(t, waitKick(t, hs), "reconnect must kick an immediate diff round")
	})
	t.Run("already online: no kick", func(t *testing.T) {
		f := newFixtureManager(t, spaceId)
		hs := &headSyncStub{kicked: make(chan struct{}, 1)}
		f.cm.headSync = hs
		f.cm.nodeStatus.SetNodesStatus(spaceId, nodestatus.Online)
		f.pool.EXPECT().GetOneOf(gomock.Any(), gomock.Any()).Return(newTestPeer("id"), nil)
		f.updater.EXPECT().Refresh(spaceId)
		f.cm.fetchResponsiblePeers()
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
		f.cm.fetchResponsiblePeers()
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

	st := p.ProvideStat().(providerStat)
	assert.Equal(t, 2, st.Managers)
	assert.False(t, st.Offline)
	assert.Equal(t, int64(2), st.ConnectivityEvents)
	assert.Equal(t, int64(4), st.RebuildSignals, "one signal per manager per event")
	assert.Equal(t, int64(1), st.ReconnectDiffKicks)
	assert.Equal(t, int64(3), st.ClosedPeersFiltered)
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
	f.cm.fetchResponsiblePeers()
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
	// the healthy peer behind it must still get a full budget of its own
	f.pool.EXPECT().Get(gomock.Any(), "healthyPeer").DoAndReturn(
		func(ctx context.Context, id string) (peer.Peer, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok, "local dials must carry a deadline")
			assert.Greater(t, time.Until(deadline), localPeerDialTimeout/2,
				"each local dial needs its own budget: a stale peer must not starve the ones behind it")
			return newTestPeer("healthyPeer"), nil
		})

	f.cm.fetchResponsiblePeers()

	peers, err := f.cm.GetResponsiblePeers(context.Background())
	require.NoError(t, err)
	require.Len(t, peers, 2, "node and healthy local peer must both be served")
	assert.Equal(t, []string{"healthyPeer"}, f.store.LocalPeerIds(spaceId),
		"only the stale peer may be evicted")
}
