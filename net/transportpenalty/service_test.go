package transportpenalty

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/quicdemotion"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

// fakePenaltyManager stands in for quicdemotion. The service drives it from
// the debounce timer, the startup goroutine and the connectivity hook while
// the test goroutine inspects it, so every field lives under mu - the real
// component is locked the same way, and an unlocked double made the suite
// fail under -race rather than the service.
type fakePenaltyManager struct {
	mu       sync.Mutex
	snapshot quicdemotion.PenaltySnapshot
	observer func()
	seeded   []quicdemotion.PenaltySnapshot
	resets   int
	// snapshots counts Snapshot calls; snapshotGate, when set, holds every
	// Snapshot call until it is closed - a save is then stuck mid-flight.
	snapshots    int
	snapshotGate chan struct{}
}

func (f *fakePenaltyManager) Init(a *app.App) error { return nil }
func (f *fakePenaltyManager) Name() string          { return "fakePeerService" }

func (f *fakePenaltyManager) Snapshot() quicdemotion.PenaltySnapshot {
	f.mu.Lock()
	f.snapshots++
	gate := f.snapshotGate
	f.mu.Unlock()
	if gate != nil {
		// outside the lock: a gated Snapshot must not block mutate
		<-gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	peers := make(map[string]quicdemotion.PeerPenalty, len(f.snapshot.Peers))
	for id, p := range f.snapshot.Peers {
		peers[id] = p
	}
	return quicdemotion.PenaltySnapshot{Peers: peers}
}

func (f *fakePenaltyManager) Seed(snap quicdemotion.PenaltySnapshot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seeded = append(f.seeded, snap)
	f.snapshot = snap
}

// Reset mimics the real semantics: the observer fires only when the reset
// actually mutated state, and outside the lock.
func (f *fakePenaltyManager) Reset() {
	f.mu.Lock()
	f.resets++
	changed := len(f.snapshot.Peers) > 0
	f.snapshot = quicdemotion.PenaltySnapshot{}
	observer := f.observer
	f.mu.Unlock()
	if changed && observer != nil {
		observer()
	}
}

func (f *fakePenaltyManager) SetObserver(observer func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.observer = observer
}

// mutate emulates peerservice recording a penalty: state changes, observer fires.
func (f *fakePenaltyManager) mutate(peerId string, p quicdemotion.PeerPenalty) {
	f.mu.Lock()
	if f.snapshot.Peers == nil {
		f.snapshot.Peers = map[string]quicdemotion.PeerPenalty{}
	}
	f.snapshot.Peers[peerId] = p
	observer := f.observer
	f.mu.Unlock()
	if observer != nil {
		observer()
	}
}

func (f *fakePenaltyManager) gateSnapshots(gate chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshotGate = gate
}

func (f *fakePenaltyManager) resetCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resets
}

func (f *fakePenaltyManager) snapshotCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snapshots
}

func (f *fakePenaltyManager) seededSnapshots() []quicdemotion.PenaltySnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]quicdemotion.PenaltySnapshot(nil), f.seeded...)
}

func (f *fakePenaltyManager) getObserver() func() {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.observer
}

// peers returns a copy of the current penalty map.
func (f *fakePenaltyManager) peers() map[string]quicdemotion.PeerPenalty {
	return f.Snapshot().Peers
}

// fakeNetwork stands in for device.NetworkState. identity is read by the
// startup goroutine and the save path while tests reassign it, hence the lock.
type fakeNetwork struct {
	mu       sync.Mutex
	identity string
	hooks    []func(online bool)
}

func (f *fakeNetwork) Init(a *app.App) error { return nil }
func (f *fakeNetwork) Name() string          { return "fakeNetworkState" }

// NetworkIdentity models the unknown state as an empty identity.
func (f *fakeNetwork) NetworkIdentity() (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.identity, f.identity != ""
}

func (f *fakeNetwork) setIdentity(identity string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.identity = identity
}

func (f *fakeNetwork) RegisterConnectivityHook(hook func(online bool)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hooks = append(f.hooks, hook)
}

func (f *fakeNetwork) fire(online bool) {
	f.mu.Lock()
	hooks := make([]func(online bool), len(f.hooks))
	copy(hooks, f.hooks)
	f.mu.Unlock()
	for _, h := range hooks {
		h(online)
	}
}

func (f *fakeNetwork) fireRecovery() { f.fire(true) }

// fireOffline is a recovery run while the device is known to be offline (the
// real identity then embeds NOT_CONNECTED / an empty interface set, so a test
// sets a different identity string first to mirror that).
func (f *fakeNetwork) fireOffline() { f.fire(false) }

type fakeWallet struct {
	repoPath string
}

func (f *fakeWallet) Init(a *app.App) error { return nil }
func (f *fakeWallet) Name() string          { return walletCName }
func (f *fakeWallet) RepoPath() string      { return f.repoPath }

type fixture struct {
	*service
	a       *app.App
	peers   *fakePenaltyManager
	network *fakeNetwork
	repo    string
}

func newFixture(t *testing.T, identity string) *fixture {
	fx := newStoppedFixture(t, identity)
	fx.start(t)
	return fx
}

// newStoppedFixture wires the app without starting it, so a test can tune the
// service and seed the state file before Init runs.
func newStoppedFixture(t *testing.T, identity string) *fixture {
	fx := &fixture{
		service: New().(*service),
		a:       new(app.App),
		peers:   &fakePenaltyManager{},
		network: &fakeNetwork{identity: identity},
		repo:    t.TempDir(),
	}
	// short intervals so tests don't wait
	fx.service.saveDebounce = 10 * time.Millisecond
	fx.service.startupCheckInterval = 10 * time.Millisecond
	fx.a.Register(&fakeWallet{repoPath: fx.repo}).
		Register(fx.peers).
		Register(fx.network).
		Register(fx.service)
	return fx
}

func (fx *fixture) start(t *testing.T) {
	require.NoError(t, fx.a.Start(ctx))
	t.Cleanup(func() { require.NoError(t, fx.a.Close(ctx)) })
}

func (fx *fixture) statePath() string {
	return filepath.Join(fx.repo, fileName)
}

func (fx *fixture) writeStateFile(t *testing.T, st storedState) {
	data, err := json.Marshal(st)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(fx.repo, fileName), data, 0o600))
}

// readStateFile parses the state file; a missing or torn file yields the zero
// value so Eventually callers can keep polling.
func (fx *fixture) readStateFile(t *testing.T) (st storedState) {
	data, err := os.ReadFile(fx.statePath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &st)
	return
}

func demotedPeers(peerId string) quicdemotion.PenaltySnapshot {
	return quicdemotion.PenaltySnapshot{Peers: map[string]quicdemotion.PeerPenalty{
		peerId: {
			ConsecutiveDegraded: 1,
			DemotedUntil:        time.Now().Add(time.Hour).UTC(),
			BackoffLevel:        1,
		},
	}}
}

func TestService_Seed(t *testing.T) {
	t.Run("seeds stored penalties on start", func(t *testing.T) {
		// given
		repo := t.TempDir()
		st := storedState{NetworkKey: "net-A", UpdatedAt: time.Now(), Penalties: demotedPeers("p1")}
		data, err := json.Marshal(st)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(repo, fileName), data, 0o600))

		fx := &fixture{
			service: New().(*service),
			a:       new(app.App),
			peers:   &fakePenaltyManager{},
			network: &fakeNetwork{identity: "net-A"},
			repo:    repo,
		}
		fx.service.saveDebounce = 10 * time.Millisecond
		fx.service.startupCheckInterval = 10 * time.Millisecond
		fx.a.Register(&fakeWallet{repoPath: repo}).
			Register(fx.peers).
			Register(fx.network).
			Register(fx.service)

		// when
		require.NoError(t, fx.a.Start(ctx))
		defer func() { require.NoError(t, fx.a.Close(ctx)) }()

		// then
		require.Len(t, fx.peers.seededSnapshots(), 1)
		assert.Contains(t, fx.peers.seededSnapshots()[0].Peers, "p1")
	})
	t.Run("no state file means no seed", func(t *testing.T) {
		fx := newFixture(t, "net-A")
		assert.Empty(t, fx.peers.seededSnapshots())
	})
	t.Run("disabled by env: no seed, no observer", func(t *testing.T) {
		t.Setenv(DisableEnv, "0")
		fx := newFixture(t, "net-A")
		fx.writeStateFile(t, storedState{NetworkKey: "net-A", Penalties: demotedPeers("p1")})
		assert.Empty(t, fx.peers.seededSnapshots())
		assert.Nil(t, fx.peers.getObserver())
	})
}

func TestService_Save(t *testing.T) {
	t.Run("penalty mutation writes the state file", func(t *testing.T) {
		// given
		fx := newFixture(t, "net-A")

		// when
		fx.peers.mutate("p1", quicdemotion.PeerPenalty{ConsecutiveDegraded: 1})

		// then
		require.Eventually(t, func() bool {
			_, err := os.Stat(fx.statePath())
			return err == nil
		}, time.Second, 10*time.Millisecond)
		data, err := os.ReadFile(fx.statePath())
		require.NoError(t, err)
		var st storedState
		require.NoError(t, json.Unmarshal(data, &st))
		assert.Equal(t, "net-A", st.NetworkKey)
		assert.Contains(t, st.Penalties.Peers, "p1")
	})
	t.Run("emptied state removes the file", func(t *testing.T) {
		// given
		fx := newFixture(t, "net-A")
		fx.peers.mutate("p1", quicdemotion.PeerPenalty{ConsecutiveDegraded: 1})
		require.Eventually(t, func() bool {
			_, err := os.Stat(fx.statePath())
			return err == nil
		}, time.Second, 10*time.Millisecond)

		// when
		fx.peers.Reset()

		// then
		require.Eventually(t, func() bool {
			_, err := os.Stat(fx.statePath())
			return os.IsNotExist(err)
		}, time.Second, 10*time.Millisecond)
	})
}

func TestService_ConcurrentSave(t *testing.T) {
	t.Run("a mutation during a save waits for it instead of writing alongside", func(t *testing.T) {
		// given: the first save is stuck inside Snapshot
		fx := newFixture(t, "net-A")
		gate := make(chan struct{})
		fx.peers.gateSnapshots(gate)
		t.Cleanup(func() {
			select {
			case <-gate:
			default:
				close(gate)
			}
		})
		fx.peers.mutate("p1", quicdemotion.PeerPenalty{ConsecutiveDegraded: 1})
		require.Eventually(t, func() bool { return fx.peers.snapshotCalls() == 1 }, time.Second, time.Millisecond)

		// when: another mutation arms a second save while the first is in flight
		fx.peers.mutate("p2", quicdemotion.PeerPenalty{ConsecutiveDegraded: 1})
		time.Sleep(10 * fx.service.saveDebounce)

		// then: the second save has not started
		assert.Equal(t, 1, fx.peers.snapshotCalls())

		// and once the first completes, the second runs and the file holds both
		close(gate)
		require.Eventually(t, func() bool { return fx.peers.snapshotCalls() == 2 }, time.Second, time.Millisecond)
		require.Eventually(t, func() bool {
			st := fx.readStateFile(t)
			_, p1 := st.Penalties.Peers["p1"]
			_, p2 := st.Penalties.Peers["p2"]
			return p1 && p2
		}, time.Second, time.Millisecond)
	})
	t.Run("concurrent writers never collide on the temp file", func(t *testing.T) {
		// given
		path := filepath.Join(t.TempDir(), fileName)
		const writers, rounds = 8, 20
		errs := make(chan error, writers*rounds)

		// when
		var wg sync.WaitGroup
		for w := 0; w < writers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < rounds; i++ {
					errs <- writeFileAtomic(path, storedState{NetworkKey: "net-A", Penalties: demotedPeers("p1")})
				}
			}()
		}
		wg.Wait()
		close(errs)

		// then: every write landed and the file is whole
		for err := range errs {
			require.NoError(t, err)
		}
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var st storedState
		require.NoError(t, json.Unmarshal(data, &st))
		assert.Contains(t, st.Penalties.Peers, "p1")
		leftovers, err := filepath.Glob(path + ".*.tmp")
		require.NoError(t, err)
		assert.Empty(t, leftovers)
	})
	t.Run("temp files of a killed write are removed on load", func(t *testing.T) {
		// given
		fx := newStoppedFixture(t, "net-A")
		stale := fx.statePath() + ".123456.tmp"
		require.NoError(t, os.WriteFile(stale, []byte("{"), 0o600))
		fx.writeStateFile(t, storedState{NetworkKey: "net-A", Penalties: demotedPeers("p1")})

		// when
		fx.start(t)

		// then: the leftover is gone and the real file was still seeded
		_, err := os.Stat(stale)
		assert.True(t, os.IsNotExist(err))
		assert.Len(t, fx.peers.seededSnapshots(), 1)
	})
}

func TestService_NetworkChange(t *testing.T) {
	t.Run("mid-session identity change resets penalties", func(t *testing.T) {
		// given
		fx := newFixture(t, "net-A")
		fx.network.fireRecovery() // first observation: net-A
		require.Equal(t, 0, fx.peers.resetCount())

		// when
		fx.network.setIdentity("net-B")
		fx.network.fireRecovery()

		// then
		assert.Equal(t, 1, fx.peers.resetCount())
	})
	t.Run("recovery on the same network does not reset", func(t *testing.T) {
		fx := newFixture(t, "net-A")
		fx.network.fireRecovery()
		fx.network.fireRecovery()
		assert.Equal(t, 0, fx.peers.resetCount())
	})
	t.Run("offline blip on the same network does not reset", func(t *testing.T) {
		// given: verdict observed on net-A
		fx := newFixture(t, "net-A")
		fx.network.fireRecovery()
		fx.peers.mutate("p1", quicdemotion.PeerPenalty{ConsecutiveDegraded: 1})

		// when: the link drops (identity now reflects the offline state) and
		// comes back on the same network
		fx.network.setIdentity("offline")
		fx.network.fireOffline()
		fx.network.setIdentity("net-A")
		fx.network.fireRecovery()

		// then
		assert.Equal(t, 0, fx.peers.resetCount())
		assert.Contains(t, fx.peers.peers(), "p1")
	})
	t.Run("offline before the first observation keeps the seeded verdict", func(t *testing.T) {
		// given: stored net-A verdict, device starts offline, startup check
		// kept out of the way
		fx := newStoppedFixture(t, "offline")
		fx.writeStateFile(t, storedState{NetworkKey: "net-A", UpdatedAt: time.Now(), Penalties: demotedPeers("p1")})
		fx.service.startupCheckInterval = time.Hour
		fx.start(t)
		require.Len(t, fx.peers.seededSnapshots(), 1)

		// when
		fx.network.fireOffline()

		// then: nothing was learned about the network, nothing is dropped
		assert.Equal(t, 0, fx.peers.resetCount())
		_, err := os.Stat(fx.statePath())
		assert.NoError(t, err)

		// and the reconnect on net-A is the first real observation: a match
		fx.network.setIdentity("net-A")
		fx.network.fireRecovery()
		assert.Equal(t, 0, fx.peers.resetCount())
	})
	t.Run("stored key mismatch on first observation resets seeded penalties", func(t *testing.T) {
		// given: verdict learned on net-A, device now on net-B
		repo := t.TempDir()
		st := storedState{NetworkKey: "net-A", UpdatedAt: time.Now(), Penalties: demotedPeers("p1")}
		data, err := json.Marshal(st)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(repo, fileName), data, 0o600))

		fx := &fixture{
			service: New().(*service),
			a:       new(app.App),
			peers:   &fakePenaltyManager{},
			network: &fakeNetwork{identity: "net-B"},
			repo:    repo,
		}
		fx.service.saveDebounce = 10 * time.Millisecond
		fx.service.startupCheckInterval = 10 * time.Millisecond
		fx.a.Register(&fakeWallet{repoPath: repo}).
			Register(fx.peers).
			Register(fx.network).
			Register(fx.service)
		require.NoError(t, fx.a.Start(ctx))
		defer func() { require.NoError(t, fx.a.Close(ctx)) }()

		// then: the startup check notices the mismatch
		require.Eventually(t, func() bool { return fx.peers.resetCount() == 1 }, time.Second, 10*time.Millisecond)
		// and the stale file is gone
		require.Eventually(t, func() bool {
			_, err := os.Stat(filepath.Join(repo, fileName))
			return os.IsNotExist(err)
		}, time.Second, 10*time.Millisecond)
	})
	t.Run("penalty save before the first observation does not launder the stored key", func(t *testing.T) {
		// given: verdict learned on net-A, device now on net-B, and the
		// startup check kept out of the way
		fx := newStoppedFixture(t, "net-B")
		fx.writeStateFile(t, storedState{NetworkKey: "net-A", UpdatedAt: time.Now(), Penalties: demotedPeers("p1")})
		fx.service.startupCheckInterval = time.Hour
		fx.start(t)
		require.Len(t, fx.peers.seededSnapshots(), 1)

		// a strike lands and is persisted before any identity observation
		fx.peers.mutate("p2", quicdemotion.PeerPenalty{ConsecutiveDegraded: 1})
		require.Eventually(t, func() bool {
			_, ok := fx.readStateFile(t).Penalties.Peers["p2"]
			return ok
		}, time.Second, 10*time.Millisecond)

		// when: the first observation arrives
		fx.network.fireRecovery()

		// then: the net-A verdict must still be recognized as foreign
		assert.Equal(t, 1, fx.peers.resetCount())
		require.Eventually(t, func() bool {
			_, err := os.Stat(fx.statePath())
			return os.IsNotExist(err)
		}, time.Second, 10*time.Millisecond)
	})
	t.Run("stored key match keeps seeded penalties", func(t *testing.T) {
		// given
		repo := t.TempDir()
		st := storedState{NetworkKey: "net-A", UpdatedAt: time.Now(), Penalties: demotedPeers("p1")}
		data, err := json.Marshal(st)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(repo, fileName), data, 0o600))

		fx := &fixture{
			service: New().(*service),
			a:       new(app.App),
			peers:   &fakePenaltyManager{},
			network: &fakeNetwork{identity: "net-A"},
			repo:    repo,
		}
		fx.service.saveDebounce = 10 * time.Millisecond
		fx.service.startupCheckInterval = 10 * time.Millisecond
		fx.a.Register(&fakeWallet{repoPath: repo}).
			Register(fx.peers).
			Register(fx.network).
			Register(fx.service)
		require.NoError(t, fx.a.Start(ctx))
		defer func() { require.NoError(t, fx.a.Close(ctx)) }()

		// then
		time.Sleep(50 * time.Millisecond)
		assert.Equal(t, 0, fx.peers.resetCount())
		assert.Len(t, fx.peers.seededSnapshots(), 1)
	})
}

func TestService_UnknownIdentity(t *testing.T) {
	t.Run("startup check waits for the identity to become known", func(t *testing.T) {
		// given: stored net-A verdict, nothing identifies the network yet
		// (mobile cold start before the client's first report)
		fx := newStoppedFixture(t, "")
		fx.writeStateFile(t, storedState{NetworkKey: "net-A", UpdatedAt: time.Now(), Penalties: demotedPeers("p1")})
		fx.start(t)
		require.Len(t, fx.peers.seededSnapshots(), 1)

		// then: the seed survives while the identity is unknown
		time.Sleep(10 * fx.service.startupCheckInterval)
		assert.Equal(t, 0, fx.peers.resetCount())
		_, err := os.Stat(fx.statePath())
		assert.NoError(t, err)

		// when: the identity becomes known and differs
		fx.network.setIdentity("net-B")

		// then: the poll picks it up
		require.Eventually(t, func() bool { return fx.peers.resetCount() == 1 }, time.Second, time.Millisecond)
		require.Eventually(t, func() bool {
			_, err := os.Stat(fx.statePath())
			return os.IsNotExist(err)
		}, time.Second, time.Millisecond)
	})
	t.Run("startup poll is bounded; a later recovery observes instead", func(t *testing.T) {
		// given
		fx := newStoppedFixture(t, "")
		fx.writeStateFile(t, storedState{NetworkKey: "net-A", UpdatedAt: time.Now(), Penalties: demotedPeers("p1")})
		fx.service.startupCheckTimeout = 30 * time.Millisecond
		fx.start(t)
		time.Sleep(3 * fx.service.startupCheckTimeout)

		// when: the identity becomes known after the poll gave up
		fx.network.setIdentity("net-B")
		time.Sleep(10 * fx.service.startupCheckInterval)

		// then: no check ran; the next recovery is the first observation
		assert.Equal(t, 0, fx.peers.resetCount())
		fx.network.fireRecovery()
		assert.Equal(t, 1, fx.peers.resetCount())
	})
	t.Run("unknown identity is never persisted", func(t *testing.T) {
		// given
		fx := newFixture(t, "")

		// when
		fx.peers.mutate("p1", quicdemotion.PeerPenalty{ConsecutiveDegraded: 1})
		time.Sleep(10 * fx.service.saveDebounce)

		// then: no file, rather than a file keyed by nothing
		_, err := os.Stat(fx.statePath())
		assert.True(t, os.IsNotExist(err))

		// and once the network is known the next mutation persists under it
		fx.network.setIdentity("net-A")
		fx.peers.mutate("p2", quicdemotion.PeerPenalty{ConsecutiveDegraded: 1})
		require.Eventually(t, func() bool { return fx.readStateFile(t).NetworkKey == "net-A" }, time.Second, time.Millisecond)
	})
	t.Run("recovery with an unknown identity is not an observation", func(t *testing.T) {
		// given
		fx := newStoppedFixture(t, "")
		fx.writeStateFile(t, storedState{NetworkKey: "net-A", UpdatedAt: time.Now(), Penalties: demotedPeers("p1")})
		fx.service.startupCheckInterval = time.Hour
		fx.start(t)

		// when
		fx.network.fireRecovery()

		// then
		assert.Equal(t, 0, fx.peers.resetCount())
		_, err := os.Stat(fx.statePath())
		assert.NoError(t, err)

		// and the first known observation is still compared to the stored key
		fx.network.setIdentity("net-B")
		fx.network.fireRecovery()
		assert.Equal(t, 1, fx.peers.resetCount())
	})
}

func TestService_CorruptFile(t *testing.T) {
	t.Run("corrupt state file is ignored", func(t *testing.T) {
		repo := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(repo, fileName), []byte("not json"), 0o600))

		fx := &fixture{
			service: New().(*service),
			a:       new(app.App),
			peers:   &fakePenaltyManager{},
			network: &fakeNetwork{identity: "net-A"},
			repo:    repo,
		}
		fx.service.saveDebounce = 10 * time.Millisecond
		fx.service.startupCheckInterval = 10 * time.Millisecond
		fx.a.Register(&fakeWallet{repoPath: repo}).
			Register(fx.peers).
			Register(fx.network).
			Register(fx.service)
		require.NoError(t, fx.a.Start(ctx))
		defer func() { require.NoError(t, fx.a.Close(ctx)) }()

		assert.Empty(t, fx.peers.seededSnapshots())
	})
}
