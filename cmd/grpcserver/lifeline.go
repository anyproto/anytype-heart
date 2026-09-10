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
)

const (
	parentLifelineEnv       = "ANYTYPE_PARENT_LIFELINE"
	parentLifelineStdin     = "stdin"
	gracefulShutdownTimeout = 10 * time.Second
)

type parentLifelineEvent string

const (
	parentLifelineShutdown parentLifelineEvent = "received shutdown message"
	parentLifelineClosed   parentLifelineEvent = "parent lifeline closed"
)

type parentLifelineMonitor struct {
	events   chan parentLifelineEvent
	done     chan struct{}
	stopped  chan struct{}
	doneOnce sync.Once
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

	event, shouldShutdown := readParentLifeline(reader, closeOnEOF)
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

func readParentLifeline(reader io.Reader, closeOnEOF bool) (parentLifelineEvent, bool) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "shutdown" {
			return parentLifelineShutdown, true
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
