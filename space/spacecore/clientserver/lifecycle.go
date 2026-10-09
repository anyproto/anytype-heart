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
// answers RST forever and its accept goroutine parks without an error. So:
//   - Background closes the socket synchronously inside StateChange, so the
//     port is closed by the time AppSetDeviceState returns to the app;
//   - Foreground, a dead socket reported by Accept and a periodic watchdog
//     probe make a worker rebind the SAME port, with backoff. The port never
//     changes at runtime: the native mDNS registration, Android NSD, the
//     LocalServer sent in exchanges and peers' records all cache it.

const lifecycleWatchdogInterval = 20 * time.Second

var lifecycleRebindBackoff = []time.Duration{
	50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond,
	time.Second, 2 * time.Second, 5 * time.Second,
}

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

	// mu orders Background's synchronous pause against the worker installing
	// a rebound socket, so a socket is never installed after a Background
	mu      sync.Mutex
	desired desiredState

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

// StateChange is called synchronously under the app lock for every device
// state report, so it never blocks on the network: closing a listener does
// no I/O, rebinding is left to the worker.
func (s *clientServer) StateChange(state int) {
	if !s.lifecycle {
		return
	}
	switch domain.CompState(state) {
	case domain.CompStateAppWentBackground:
		s.lc.mu.Lock()
		s.lc.desired = desiredBackground
		if s.lan != nil {
			s.lan.pause()
		}
		s.lc.mu.Unlock()
		log.Info("lan listener paused for background", zap.Int("port", s.port))
	case domain.CompStateAppWentForeground:
		s.lc.mu.Lock()
		s.lc.desired = desiredForeground
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
// background, bound and healthy in the foreground.
func (s *clientServer) reconcile(ctx context.Context) {
	s.lc.mu.Lock()
	if s.lc.desired == desiredBackground {
		s.lan.pause() // idempotent; covers a Background seen before start
		s.lc.mu.Unlock()
		return
	}
	s.lc.mu.Unlock()

	probeErr := s.lc.probe()
	if probeErr == nil {
		return
	}
	if errors.Is(probeErr, errNoListener) {
		// paused by us or dropped by Accept: the normal foreground path
		log.Info("lan listener rebinding", zap.Int("port", s.port))
	} else {
		// found dead without a pause of ours: on iOS a socket defuncted
		// while no Background arrived; the evidence the probe works
		log.Warn("lan listener socket found dead", zap.Int("port", s.port), zap.Error(probeErr))
	}
	start := time.Now()
	for attempt := 0; ; attempt++ {
		bound, err := s.rebindOnce()
		if bound || err == nil {
			if bound {
				log.Info("lan listener rebound", zap.Int("port", s.port), zap.Int("attempts", attempt+1), zap.Duration("took", time.Since(start)))
			}
			return
		}
		// once at the start, then about once a minute while it persists
		if attempt == 0 || attempt%12 == 0 {
			log.Warn("lan listener rebind failed", zap.Int("port", s.port), zap.Int("attempt", attempt+1), zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-s.lc.kickChan():
			// a new Foreground (or Background) arrived: act on it now and
			// start the backoff over
			attempt = -1
		case <-time.After(s.lc.backoff[min(attempt, len(s.lc.backoff)-1)]):
		}
	}
}

// rebindOnce binds the saved port and installs the socket, all under lc.mu:
// a Background either runs before (and this sees it) or waits until the new
// socket is installed and then closes it. The bind is a local syscall, so
// StateChange waits for microseconds at most. bound=false with err=nil means
// there is nothing to do (background, or the listener was closed).
func (s *clientServer) rebindOnce() (bound bool, err error) {
	s.lc.mu.Lock()
	defer s.lc.mu.Unlock()
	if s.lc.desired == desiredBackground {
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
