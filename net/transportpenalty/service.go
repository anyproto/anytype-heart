package transportpenalty

/*
AI generated

Name: Transport Penalty Store
Scope: global

## Responsibility
- Persists the QUIC penalty state (peers demoted to yamux-first after
  DPI-degraded connection deaths, see any-sync net/quicdemotion) across
  restarts, keyed by the current network identity — a freshly opened app on a
  QUIC-hostile network must not re-learn the demotion (mobile apps are
  evicted constantly)
- Seeds the stored penalties into quicdemotion at startup, then drops them if
  the network identity turns out to differ from the one the verdict was
  learned on (checked once shortly after start and on every connectivity
  recovery)
- Resets the penalties on a mid-session network change: a new network
  deserves a clean verdict
- Disabled entirely via ANYTYPE_QUIC_AUTO_DEMOTION=0
*/

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/net/quicdemotion"
	"go.uber.org/zap"
)

const (
	CName    = "net.transportpenalty"
	fileName = "transport_penalties.json"
	// DisableEnv set to "0" turns QUIC auto-demotion off entirely (debugging
	// escape hatch): bootstrap skips registering quicdemotion and this
	// component skips seeding and persistence.
	DisableEnv = "ANYTYPE_QUIC_AUTO_DEMOTION"

	walletCName = accountservice.CName

	defaultSaveDebounce      = time.Second
	defaultStartupCheckDelay = 3 * time.Second
)

var log = logger.NewNamed(CName)

func New() Service {
	return &service{
		saveDebounce:      defaultSaveDebounce,
		startupCheckDelay: defaultStartupCheckDelay,
	}
}

type Service interface {
	app.ComponentRunnable
}

// penaltyManager is the quicdemotion surface this component drives.
type penaltyManager interface {
	Snapshot() quicdemotion.PenaltySnapshot
	Seed(snap quicdemotion.PenaltySnapshot)
	Reset()
	SetObserver(observer func())
}

// networkIdentity is the device.NetworkState surface this component needs.
type networkIdentity interface {
	NetworkIdentity() string
	RegisterConnectivityHook(hook func(online bool))
}

type repoPathProvider interface {
	RepoPath() string
}

// storedState is the on-disk format of <repo>/transport_penalties.json.
type storedState struct {
	// NetworkKey is the network identity the penalties were learned on;
	// empty when it was never observed (fail open: the verdict is kept).
	NetworkKey string                       `json:"networkKey"`
	UpdatedAt  time.Time                    `json:"updatedAt"`
	Penalties  quicdemotion.PenaltySnapshot `json:"penalties"`
}

type service struct {
	peers    penaltyManager
	network  networkIdentity
	filePath string
	disabled bool

	saveDebounce      time.Duration
	startupCheckDelay time.Duration

	mu           sync.Mutex
	lastIdentity string // last observed network identity; "" until first observation
	storedKey    string // network key the seeded penalties were learned on
	saveTimer    *time.Timer
	savePending  bool
	closed       bool
	closeCh      chan struct{}
}

func (s *service) Name() string { return CName }

func (s *service) Init(a *app.App) error {
	s.closeCh = make(chan struct{})
	repoPath := a.MustComponent(walletCName).(repoPathProvider).RepoPath()
	s.filePath = filepath.Join(repoPath, fileName)
	if os.Getenv(DisableEnv) == "0" {
		s.disabled = true
		log.Info("quic auto-demotion persistence disabled by env")
		return nil
	}
	// quicdemotion is only registered as an app component when the feature is
	// enabled (see bootstrap), so its lookup must not happen above the
	// disabled check.
	s.peers = app.MustComponent[penaltyManager](a)
	s.network = app.MustComponent[networkIdentity](a)
	s.load()
	s.peers.SetObserver(s.scheduleSave)
	s.network.RegisterConnectivityHook(s.onConnectivityRecovery)
	return nil
}

func (s *service) Run(ctx context.Context) error {
	if s.disabled {
		return nil
	}
	// The stored-key check needs the current network identity, which isn't
	// known at Init: the net monitor's first interface snapshot and the
	// client's first network report both arrive shortly after start. On a
	// stable network no connectivity recovery ever fires, so check once here.
	go func() {
		select {
		case <-time.After(s.startupCheckDelay):
		case <-s.closeCh:
			return
		}
		s.checkIdentity(s.network.NetworkIdentity())
	}()
	return nil
}

func (s *service) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.closeCh)
	if s.saveTimer != nil {
		s.saveTimer.Stop()
	}
	pending := s.savePending
	s.savePending = false
	s.mu.Unlock()
	if pending {
		s.save()
	}
	return nil
}

// load reads the stored penalties and seeds them into quicdemotion. A snapshot
// from an unknown network is seeded anyway (fail open): worst case is one
// yamux-first session on a good network, which still works — and the startup
// identity check drops it as soon as the mismatch is visible.
func (s *service) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Warn("read transport penalties", zap.Error(err))
		}
		return
	}
	var st storedState
	if err = json.Unmarshal(data, &st); err != nil {
		log.Warn("parse transport penalties, dropping the file", zap.Error(err))
		_ = os.Remove(s.filePath)
		return
	}
	if len(st.Penalties.Peers) == 0 {
		return
	}
	s.mu.Lock()
	s.storedKey = st.NetworkKey
	s.mu.Unlock()
	s.peers.Seed(st.Penalties)
	log.Info("seeded stored transport penalties",
		zap.Int("peers", len(st.Penalties.Peers)),
		zap.String("networkKey", st.NetworkKey),
		zap.Time("updatedAt", st.UpdatedAt))
}

// onConnectivityRecovery runs on every connectivity recovery (network switch,
// interface change, wake, foreground resume), after the connection pool has
// been flushed.
func (s *service) onConnectivityRecovery(online bool) {
	s.checkIdentity(s.network.NetworkIdentity())
}

// checkIdentity compares the current network identity against the last known
// one and resets the penalties when the device moved to another network. The
// first observation is compared against the persisted key instead: penalties
// learned on a different network must not apply here.
func (s *service) checkIdentity(identity string) {
	s.mu.Lock()
	prev := s.lastIdentity
	s.lastIdentity = identity
	storedKey := s.storedKey
	s.mu.Unlock()
	if prev == identity {
		return
	}
	if prev == "" {
		if storedKey != "" && storedKey != identity {
			log.Info("stored transport penalties are from another network, resetting",
				zap.String("storedKey", storedKey), zap.String("identity", identity))
			s.peers.Reset()
			s.removeFile()
		}
		return
	}
	log.Info("network changed, resetting transport penalties",
		zap.String("from", prev), zap.String("to", identity))
	s.peers.Reset()
}

// scheduleSave debounces persistence: penalty mutations arrive in bursts
// (every degraded conn death mutates the state).
func (s *service) scheduleSave() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.savePending {
		return
	}
	s.savePending = true
	s.saveTimer = time.AfterFunc(s.saveDebounce, func() {
		s.mu.Lock()
		s.savePending = false
		closed := s.closed
		s.mu.Unlock()
		if !closed {
			s.save()
		}
	})
}

func (s *service) save() {
	snap := s.peers.Snapshot()
	if len(snap.Peers) == 0 {
		s.removeFile()
		return
	}
	identity := s.network.NetworkIdentity()
	s.mu.Lock()
	s.storedKey = identity
	s.mu.Unlock()
	st := storedState{
		NetworkKey: identity,
		UpdatedAt:  time.Now().UTC(),
		Penalties:  snap,
	}
	if err := writeFileAtomic(s.filePath, st); err != nil {
		log.Warn("save transport penalties", zap.Error(err))
	}
}

func (s *service) removeFile() {
	if err := os.Remove(s.filePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Warn("remove transport penalties file", zap.Error(err))
	}
}

func writeFileAtomic(path string, st storedState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write temp state file: %w", err)
	}
	if err = os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename state file: %w", err)
	}
	return nil
}
