package clientserver

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
)

// The iOS listener lifecycle (GO-7577, see
// docs/superpowers/specs/2026-10-09-ios-lan-socket-lifecycle-design.md).
//
// iOS defuncts an app's sockets when it suspends the app: the LAN listener then
// answers RST forever and its accept goroutine parks without an error. So a
// worker rebinds the SAME port on Foreground, when Accept reports a dead socket
// and when the periodic watchdog probe fails. The port never changes at
// runtime: the native mDNS registration, Android NSD, the LocalServer sent in
// exchanges and peers' records all cache it.
//
// Whether Background also closes the port is a policy (SetPauseOnBackground):
//   - on (native discovery, which withdraws its own Bonjour registration on
//     background): Background closes the socket synchronously inside
//     StateChange, so peers get an immediate refusal;
//   - off (default; heart's zeroconf fallback keeps advertising the port with
//     a long TTL): the socket stays up — a refused dial during a short
//     background would make peers drop us with nothing to re-announce — and
//     the first Foreground after a Background rebinds it unconditionally.

const lifecycleWatchdogInterval = 20 * time.Second

var lifecycleRebindBackoff = []time.Duration{
	50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond,
	time.Second, 2 * time.Second, 5 * time.Second,
}

// bindWarnInterval rate-limits the warning for a bind failure that persists.
const bindWarnInterval = time.Minute

type desiredState int

const (
	// desiredForeground is the zero value: before the app reports any state
	// (Background is enum zero in domain.CompState), the listener stays up and
	// the watchdog active
	desiredForeground desiredState = iota
	desiredBackground
)

type listenerLifecycle struct {
	kickOnce sync.Once
	kick     chan struct{}

	// mu orders Background's synchronous pause against the worker binding and
	// installing a socket: a socket is never live but uninstalled while a
	// Background returns. It is held across a local bind syscall, so state
	// handlers may wait for an in-progress bind.
	mu                sync.Mutex
	desired           desiredState
	pauseOnBackground bool
	// sawBackground: a Background arrived while pausing is off; the next
	// Foreground then rebinds without trusting the probe
	sawBackground bool
	forceRebind   bool

	cancel context.CancelFunc
	done   chan struct{}

	// seams for tests; set to the real implementations in startLifecycle
	listen   func(port int) (net.Listener, error)
	probe    func() error
	watchdog time.Duration
	backoff  []time.Duration
}

// kickChan wakes the worker; buffered 1, so signals coalesce.
func (lc *listenerLifecycle) kickChan() chan struct{} {
	lc.kickOnce.Do(func() { lc.kick = make(chan struct{}, 1) })
	return lc.kick
}

func (lc *listenerLifecycle) signal() {
	select {
	case lc.kickChan() <- struct{}{}:
	default:
	}
}

// SetPauseOnBackground switches whether Background closes the LAN port. The
// discovery layer turns it on when it advertises through the platform's own
// discovery, which withdraws the registration on background as well. It takes
// effect from the next Background. Only meaningful where the lifecycle runs
// (iOS); found by callers through a type assertion.
func (s *clientServer) SetPauseOnBackground(enabled bool) {
	s.lc.mu.Lock()
	s.lc.pauseOnBackground = enabled
	s.lc.mu.Unlock()
}

// StateChange is called synchronously under the app lock for every device
// state report, so it never blocks on the network: closing a listener does
// no I/O, binding is left to the worker (a Background may wait for a bind
// already in progress, which is a local syscall).
func (s *clientServer) StateChange(state int) {
	if !s.lifecycle {
		return
	}
	switch domain.CompState(state) {
	case domain.CompStateAppWentBackground:
		s.lc.mu.Lock()
		paused := s.lc.pauseOnBackground
		if paused {
			s.lc.desired = desiredBackground
			if s.lan != nil {
				s.lan.pause()
			}
		} else {
			s.lc.sawBackground = true
		}
		s.lc.mu.Unlock()
		// wake a worker sleeping in a rebind backoff, so it stops retrying
		s.lc.signal()
		if paused {
			log.Info("lan listener paused for background", zap.Int("port", s.port))
		}
	case domain.CompStateAppWentForeground:
		s.lc.mu.Lock()
		s.lc.desired = desiredForeground
		if s.lc.sawBackground {
			s.lc.sawBackground = false
			s.lc.forceRebind = true
		}
		s.lc.mu.Unlock()
		s.lc.signal()
	}
}

func (s *clientServer) startLifecycle() {
	lc := &s.lc
	if lc.listen == nil {
		lc.listen = func(port int) (net.Listener, error) {
			return net.Listen("tcp", ":"+strconv.Itoa(port))
		}
	}
	if lc.probe == nil {
		lc.probe = s.lan.probe
	}
	if lc.watchdog == 0 {
		lc.watchdog = lifecycleWatchdogInterval
	}
	if lc.backoff == nil {
		lc.backoff = lifecycleRebindBackoff
	}
	ctx, cancel := context.WithCancel(context.Background())
	lc.cancel, lc.done = cancel, make(chan struct{})
	go s.lifecycleWorker(ctx, lc.done)
}

func (s *clientServer) stopLifecycle() {
	if s.lc.cancel == nil {
		return
	}
	s.lc.cancel()
	<-s.lc.done
}

func (s *clientServer) lifecycleWorker(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(s.lc.watchdog)
	defer ticker.Stop()
	for {
		// the first pass covers a Background reported before the listener
		// was bound (a cold launch into the background)
		s.reconcile(ctx)
		select {
		case <-ctx.Done():
			return
		case <-s.lc.kickChan():
		case <-ticker.C:
		}
	}
}

// reconcile brings the socket in line with the desired state: paused in the
// background (when pausing is on), bound and healthy otherwise.
func (s *clientServer) reconcile(ctx context.Context) {
	s.lc.mu.Lock()
	if s.lan.isClosed() {
		s.lc.mu.Unlock()
		return
	}
	if s.lc.desired == desiredBackground {
		s.lan.pause() // idempotent; covers a Background seen before start
		s.lc.mu.Unlock()
		return
	}
	force := s.lc.forceRebind
	s.lc.forceRebind = false
	s.lc.mu.Unlock()

	switch probeErr := s.lc.probe(); {
	case force:
		// back from a background that did not pause: the socket may have
		// been defuncted during a suspension, whatever the probe says. The
		// probe result is logged anyway: on a device it shows whether
		// SO_ERROR detects a defunct listener (probe=<nil> after a
		// suspension means it does not)
		log.Info("lan listener rebinding after background", zap.Int("port", s.port), zap.NamedError("probe", probeErr))
	case probeErr == nil:
		return
	case errors.Is(probeErr, errNoListener):
		// paused by us or dropped by Accept: the normal foreground path
		log.Info("lan listener rebinding", zap.Int("port", s.port))
	default:
		// found dead without a pause of ours: on iOS a socket defuncted
		// while no Background arrived; the evidence the probe works
		log.Warn("lan listener socket found dead", zap.Int("port", s.port), zap.Error(probeErr))
	}

	start := time.Now()
	failures := 0
	var lastWarn time.Time
	for step := 0; ; step++ {
		bound, err := s.rebindOnce()
		if err == nil {
			if bound {
				log.Info("lan listener rebound", zap.Int("port", s.port), zap.Int("failures", failures), zap.Duration("took", time.Since(start)))
			}
			return
		}
		failures++
		if failures == 1 || time.Since(lastWarn) >= bindWarnInterval {
			lastWarn = time.Now()
			log.Warn("lan listener rebind failed", zap.Int("port", s.port), zap.Int("failures", failures), zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-s.lc.kickChan():
			// a new Foreground or Background arrived: act on it now and start
			// the backoff over (the warning rate limit keeps counting)
			step = -1
		case <-time.After(s.lc.backoff[min(step, len(s.lc.backoff)-1)]):
		}
	}
}

// rebindOnce binds the saved port and installs the socket, all under lc.mu:
// a Background either runs before (and this sees it) or waits until the new
// socket is installed and then closes it. bound=false with err=nil means
// there is nothing to do (paused for background, or the listener is closed).
func (s *clientServer) rebindOnce() (bound bool, err error) {
	s.lc.mu.Lock()
	defer s.lc.mu.Unlock()
	if s.lc.desired == desiredBackground || s.lan.isClosed() {
		return false, nil
	}
	// close the old socket first: while a defunct fd stays open its PCB
	// keeps the port referenced and the new bind fails
	s.lan.pause()
	next, err := s.lc.listen(s.port)
	if err != nil {
		return false, err
	}
	if err = s.lan.install(next); err != nil {
		_ = next.Close()
		return false, nil
	}
	return true, nil
}
