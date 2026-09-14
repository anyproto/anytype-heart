//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package main

import (
	"bufio"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/anytype-heart/core"
)

const (
	parentLifelineEnv       = "ANYTYPE_PARENT_LIFELINE"
	parentLifelineStdin     = "stdin"
	gracefulShutdownTimeout = 10 * time.Second

	// parentLifelineSecretPrefix marks the stdin line carrying the per-launch
	// local API secret. The parent holds the only write end of this pipe — it
	// is not in the process table like argv, and not readable cross-user like
	// an env var — which is what makes the value a proof of parenthood.
	parentLifelineSecretPrefix = "secret "

	// parentLifelineSecretTimeout bounds how long startup waits for that line
	// before running permissive. Kept short: it delays every parented launch
	// whose parent does not send one.
	parentLifelineSecretTimeout = 5 * time.Second
)

type parentLifelineEvent string

const (
	parentLifelineShutdown parentLifelineEvent = "received shutdown message"
	parentLifelineClosed   parentLifelineEvent = "parent lifeline closed"
)

type parentLifelineMonitor struct {
	events   chan parentLifelineEvent
	secret   parentLifelineSecret
	done     chan struct{}
	stopped  chan struct{}
	doneOnce sync.Once
}

// parentLifelineSecret carries the secret from the stdin reader to startup.
// The channel is buffered so the reader never blocks on a startup that already
// gave up waiting, and closed when the stream ends without one so the wait
// fails fast instead of burning its whole window.
type parentLifelineSecret struct {
	values chan string
	once   sync.Once
}

func (s *parentLifelineSecret) deliver(secret string) {
	s.once.Do(func() {
		s.values <- secret
		close(s.values)
	})
}

// finish releases a waiter once the stdin stream is done, whether or not a
// secret arrived.
func (s *parentLifelineSecret) finish() {
	s.once.Do(func() {
		close(s.values)
	})
}

// waitForSecret blocks until the parent sends the secret, the stdin stream
// ends, timeout elapses, or abort fires. It reports false in every case but the
// first, leaving the caller to run permissive.
//
// abort carries OS signals so a quit arriving during the window is acted on at
// once: by the time this runs, signal.Notify has already disabled Go's default
// terminate behavior, so without it the process would look unresponsive for the
// rest of the window. A nil abort channel simply never fires.
func (m *parentLifelineMonitor) waitForSecret(timeout time.Duration, abort <-chan os.Signal) (string, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case secret, ok := <-m.secret.values:
		return secret, ok
	case <-abort:
		return "", false
	case <-timer.C:
		return "", false
	}
}

// awaitSecret blocks until the secret arrives or the stdin stream ends, with no
// deadline. It is how a secret that missed the startup window is still picked
// up.
func (m *parentLifelineMonitor) awaitSecret() (string, bool) {
	secret, ok := <-m.secret.values
	return secret, ok
}

type parentLifelineDeadline struct {
	elapsed <-chan time.Time
	stop    func()
}

type parentLifelineDeadlineFactory func(time.Duration) parentLifelineDeadline

func parentLifelineEnabled() bool {
	return os.Getenv(parentLifelineEnv) == parentLifelineStdin
}

func shouldMonitorParentStdin() bool {
	// Older Windows clients already use the stdin shutdown command but do not
	// set the lifeline environment variable. Preserve that protocol there.
	return parentLifelineEnabled() || runtime.GOOS == "windows"
}

func startParentLifelineMonitor(
	reader io.Reader,
	closeOnEOF bool,
	timeout time.Duration,
	forceExit func(int),
) *parentLifelineMonitor {
	return startParentLifelineMonitorWithDeadline(reader, closeOnEOF, timeout, newParentLifelineDeadline, forceExit)
}

func startParentLifelineMonitorWithDeadline(
	reader io.Reader,
	closeOnEOF bool,
	timeout time.Duration,
	deadlineFactory parentLifelineDeadlineFactory,
	forceExit func(int),
) *parentLifelineMonitor {
	monitor := &parentLifelineMonitor{
		events:  make(chan parentLifelineEvent, 1),
		secret:  parentLifelineSecret{values: make(chan string, 1)},
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	go monitor.run(reader, closeOnEOF, timeout, deadlineFactory, forceExit)
	return monitor
}

func newParentLifelineDeadline(timeout time.Duration) parentLifelineDeadline {
	timer := time.NewTimer(timeout)
	return parentLifelineDeadline{
		elapsed: timer.C,
		stop: func() {
			timer.Stop()
		},
	}
}

func (m *parentLifelineMonitor) run(
	reader io.Reader,
	closeOnEOF bool,
	timeout time.Duration,
	deadlineFactory parentLifelineDeadlineFactory,
	forceExit func(int),
) {
	defer close(m.stopped)

	event, shouldShutdown := readParentLifeline(reader, closeOnEOF, m.secret.deliver)
	// The stream is done either way: release a startup still waiting on the
	// secret rather than letting it sit out its full window.
	m.secret.finish()
	if !shouldShutdown {
		return
	}
	if !closeOnEOF {
		// Legacy Windows clients send the shutdown command without opting into
		// parent ownership. Preserve their historical unbounded graceful stop.
		notifyParentLifeline(m.events, event)
		return
	}

	// Start the hard deadline here rather than in main so a dead owner cannot
	// leave the helper alive when startup has not reached its event loop yet.
	deadline := deadlineFactory(timeout)
	defer deadline.stop()

	notifyParentLifeline(m.events, event)
	select {
	case <-m.done:
		return
	case <-deadline.elapsed:
		// This callback must not perform logging or any other blocking I/O.
		// Parent-owned output pipes may be full or already closed.
		forceExit(1)
	}
}

func (m *parentLifelineMonitor) markShutdownComplete() {
	m.doneOnce.Do(func() {
		close(m.done)
	})
}

// readParentLifeline consumes the parent's stdin protocol: a leading
// "secret <value>" line, handed to onSecret, then the "shutdown" command or
// EOF. onSecret is called at most once — the secret is a per-launch value, so
// a later line repeating it is a parent bug, not a rotation.
func readParentLifeline(reader io.Reader, closeOnEOF bool, onSecret func(string)) (parentLifelineEvent, bool) {
	scanner := bufio.NewScanner(reader)
	secretSeen := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "shutdown" {
			return parentLifelineShutdown, true
		}
		if !secretSeen && strings.HasPrefix(line, parentLifelineSecretPrefix) {
			secretSeen = true
			// Never log the value, here or anywhere downstream.
			onSecret(strings.TrimSpace(strings.TrimPrefix(line, parentLifelineSecretPrefix)))
		}
	}

	// Read errors fail closed for an opted-in owner, just like EOF. Do not log
	// here: the parent-owned output pipes may be the source of the failure.
	return parentLifelineClosed, closeOnEOF
}

func notifyParentLifeline(events chan<- parentLifelineEvent, event parentLifelineEvent) {
	select {
	case events <- event:
	default:
	}
}

// registerParentLocalAPISecret hands the parent-delivered secret to the
// middleware, which then requires it on the account-bootstrap RPCs. It must
// run before the gRPC server serves: a request arriving before registration
// would be judged under permissive mode.
//
// A parented launch whose parent sends no secret within the window keeps
// running permissive — older desktop clients do not send one yet, and refusing
// to start would break them. Once the client ships it this becomes fail-closed
// (see §3.4/§6.1 of the design spec).
func registerParentLocalAPISecret(mw *core.Middleware, monitor *parentLifelineMonitor, lifelineEnabled bool, abort <-chan os.Signal) {
	watchParentLocalAPISecret(monitor, lifelineEnabled, parentLifelineSecretTimeout, abort, mw.SetLocalAPISecret)
}

// watchParentLocalAPISecret registers the parent-delivered secret, and keeps
// watching for one that did not arrive in time.
//
// In parented mode it waits, bounded, before returning, so the gate is on
// before the server serves. When that window expires the watch continues in the
// background: a parent whose main thread stalled past it still delivers, and
// dropping that secret would leave the bootstrap API open for the whole session
// while its client believes the header it sends is doing something. Registering
// late narrows the hole instead of leaving it open — the store is write-once,
// so a late secret cannot displace one already in place.
//
// Outside parented mode nothing is waited on: standalone and Docker have no
// parent channel at all, and the legacy Windows stdin protocol reads the same
// pipe without opting in, so stalling their startup would buy nothing. The
// background watch still runs for them, which is what keeps a Windows client
// that sends a secret without setting ANYTYPE_PARENT_LIFELINE from being
// silently unprotected.
func watchParentLocalAPISecret(
	monitor *parentLifelineMonitor,
	lifelineEnabled bool,
	timeout time.Duration,
	abort <-chan os.Signal,
	register func(string),
) {
	if monitor == nil {
		return
	}

	if lifelineEnabled {
		if secret, ok := monitor.waitForSecret(timeout, abort); ok && secret != "" {
			register(secret)
			log.Info("local api secret registered: the account bootstrap API now requires it")
			return
		}
		// Only parented launches are expected to send one, so only they are
		// worth warning about — standalone and Docker would log this on every
		// start with nothing the operator could do about it.
		log.Warn("parent sent no local api secret in time: the account bootstrap API stays open to any local caller")
	}

	go func() {
		secret, ok := monitor.awaitSecret()
		if !ok || secret == "" {
			return
		}
		register(secret)
		log.Warn("local api secret registered late: the account bootstrap API was open to any local caller until now")
	}()
}
