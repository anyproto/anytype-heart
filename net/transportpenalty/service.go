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
  learned on (checked as soon as the identity is known after start, and on
  every online connectivity recovery)
- Persists only under a known network identity: while nothing identifies the
  network the state stays in memory rather than on disk under a key that
  would match anywhere
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
	// tempSuffixGlob is appended to the state file name for temp files: the
	// "*" is the random part os.CreateTemp fills in, and the same pattern
	// finds leftovers on load.
	tempSuffixGlob = ".*.tmp"
	// DisableEnv set to "0" turns QUIC auto-demotion off entirely (debugging
	// escape hatch): bootstrap skips registering quicdemotion and this
	// component skips seeding and persistence.
	DisableEnv = "ANYTYPE_QUIC_AUTO_DEMOTION"

	walletCName = accountservice.CName

	defaultSaveDebounce = time.Second
	// The startup identity check polls until the identity is known rather
	// than sampling once: the monitor's first snapshot and the client's
	// first report both land some time after Run, and the client's needs
	// the account app up first. A single sample at a fixed delay compared
	// the stored key against an unknown identity on mobile cold starts and
	// deleted the verdict the file exists to keep. The poll is bounded: an
	// identity that never becomes known leaves the seed in place, and the
	// next online recovery observes instead.
	defaultStartupCheckInterval = time.Second
	defaultStartupCheckTimeout  = 30 * time.Second
)

var log = logger.NewNamed(CName)

func New() Service {
	return &service{
		saveDebounce:         defaultSaveDebounce,
		startupCheckInterval: defaultStartupCheckInterval,
		startupCheckTimeout:  defaultStartupCheckTimeout,
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
	// NetworkIdentity reports ok=false while nothing identifies the network;
	// such a key is never compared, persisted or remembered here.
	NetworkIdentity() (identity string, ok bool)
	RegisterConnectivityHook(hook func(online bool))
}

type repoPathProvider interface {
	RepoPath() string
}

// storedState is the on-disk format of <repo>/transport_penalties.json.
type storedState struct {
	// NetworkKey is the network identity the penalties were learned on. A
	// file this version writes always carries one (saves wait for a known
	// identity); an empty key keeps the verdict (fail open).
	NetworkKey string                       `json:"networkKey"`
	UpdatedAt  time.Time                    `json:"updatedAt"`
	Penalties  quicdemotion.PenaltySnapshot `json:"penalties"`
}

type service struct {
	peers    penaltyManager
	network  networkIdentity
	filePath string
	disabled bool

	saveDebounce         time.Duration
	startupCheckInterval time.Duration
	startupCheckTimeout  time.Duration

	// saveMu serializes the writers of the state file. The debounce timer
	// clears savePending before it calls save, so a mutation arriving
	// mid-write arms a new timer and a second save would otherwise run
	// alongside the first: two writers on one temp file truncate each
	// other's output and one rename fails, dropping an update or leaving a
	// torn file.
	saveMu sync.Mutex

	mu           sync.Mutex
	lastIdentity string // last known network identity; "" until the first known observation
	// loadedKey is the network key of the state file as it was loaded, i.e.
	// the network the seeded penalties were learned on. Only load sets it:
	// if a save could rewrite it with the current identity, a penalty
	// mutation landing before the first observation would relabel a verdict
	// from network A as learned on B and the stale-verdict check would
	// never fire.
	loadedKey   string
	saveTimer   *time.Timer
	savePending bool
	closed      bool
	closeCh     chan struct{}
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
	go s.startupCheck()
	return nil
}

// startupCheck runs the stored-key check as soon as the network identity is
// known. On a stable network no connectivity recovery ever fires, so without
// it a stale verdict would never be dropped.
func (s *service) startupCheck() {
	ticker := time.NewTicker(s.startupCheckInterval)
	defer ticker.Stop()
	deadline := time.NewTimer(s.startupCheckTimeout)
	defer deadline.Stop()
	for {
		select {
		case <-ticker.C:
			if identity, ok := s.network.NetworkIdentity(); ok {
				s.checkIdentity(identity)
				return
			}
		case <-deadline.C:
			log.Info("network identity still unknown, keeping stored transport penalties as seeded")
			return
		case <-s.closeCh:
			return
		}
	}
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
	s.removeStaleTemps()
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
	s.loadedKey = st.NetworkKey
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
	// Offline says nothing about which network the device is on, and the
	// identity embeds the connection state: comparing it here would read a
	// tunnel, a lid close, an AP roam or Android's onLost (which fires on
	// every transition) as a move to another network, drop a verdict for a
	// network the device never left, and fire again on the way back. The
	// recovery that follows the reconnect carries the observation.
	if !online {
		return
	}
	identity, ok := s.network.NetworkIdentity()
	if !ok {
		return
	}
	s.checkIdentity(identity)
}

// checkIdentity compares the current network identity against the last known
// one and resets the penalties when the device moved to another network. The
// first observation is compared against the persisted key instead: penalties
// learned on a different network must not apply here.
func (s *service) checkIdentity(identity string) {
	s.mu.Lock()
	prev := s.lastIdentity
	s.lastIdentity = identity
	loadedKey := s.loadedKey
	s.mu.Unlock()
	if prev == identity {
		return
	}
	if prev == "" {
		if loadedKey != "" && loadedKey != identity {
			log.Info("stored transport penalties are from another network, resetting",
				zap.String("storedKey", loadedKey), zap.String("identity", identity))
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
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	snap := s.peers.Snapshot()
	if len(snap.Peers) == 0 {
		s.removeFileLocked()
		return
	}
	identity, ok := s.network.NetworkIdentity()
	if !ok {
		// Nothing to attribute the verdict to. Persisted under an unknown
		// key it would match on the next start wherever the device is, so
		// it stays in memory; the next mutation on a known network persists.
		log.Debug("network identity unknown, not persisting transport penalties")
		return
	}
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
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	s.removeFileLocked()
}

func (s *service) removeFileLocked() {
	if err := os.Remove(s.filePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Warn("remove transport penalties file", zap.Error(err))
	}
}

// removeStaleTemps drops temp files of writes the process died in the middle
// of (between CreateTemp and the rename). They are never read - only the
// renamed file is - so this is housekeeping, not recovery.
func (s *service) removeStaleTemps() {
	stale, err := filepath.Glob(s.filePath + tempSuffixGlob)
	if err != nil {
		return
	}
	for _, path := range stale {
		if err = os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Warn("remove stale temp state file", zap.String("path", path), zap.Error(err))
		}
	}
}

func writeFileAtomic(path string, st storedState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	// A unique temp name per writer (0600, like the file itself): with a
	// shared fixed name a second writer truncates the first's output and one
	// of the renames fails with ENOENT.
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+tempSuffixGlob)
	if err != nil {
		return fmt.Errorf("create temp state file: %w", err)
	}
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write temp state file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("close temp state file: %w", err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("rename state file: %w", err)
	}
	return nil
}
