//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package main

import (
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReadParentLifelineCapturesSecret(t *testing.T) {
	var captured []string
	event, shouldShutdown := readParentLifeline(
		strings.NewReader("secret s3cret\nshutdown\n"),
		true,
		func(secret string) { captured = append(captured, secret) },
	)

	if len(captured) != 1 || captured[0] != "s3cret" {
		t.Fatalf("expected the secret to be captured once, got %q", captured)
	}
	if !shouldShutdown || event != parentLifelineShutdown {
		t.Fatalf("expected the shutdown line after the secret, got event %q, shutdown %t", event, shouldShutdown)
	}
}

func TestReadParentLifelineSecretThenEOF(t *testing.T) {
	var captured []string
	event, shouldShutdown := readParentLifeline(
		strings.NewReader("secret s3cret\n"),
		true,
		func(secret string) { captured = append(captured, secret) },
	)

	if len(captured) != 1 || captured[0] != "s3cret" {
		t.Fatalf("expected the secret to be captured once, got %q", captured)
	}
	if !shouldShutdown || event != parentLifelineClosed {
		t.Fatalf("expected EOF to fail closed after the secret, got event %q, shutdown %t", event, shouldShutdown)
	}
}

func TestReadParentLifelineIgnoresRepeatedSecretLines(t *testing.T) {
	var captured []string
	readParentLifeline(
		strings.NewReader("secret first\nsecret second\nshutdown\n"),
		true,
		func(secret string) { captured = append(captured, secret) },
	)

	if len(captured) != 1 || captured[0] != "first" {
		t.Fatalf("expected only the first secret line to be captured, got %q", captured)
	}
}

func TestReadParentLifelineWithoutSecret(t *testing.T) {
	var captured []string
	readParentLifeline(
		strings.NewReader("secretive\nshutdown\n"),
		true,
		func(secret string) { captured = append(captured, secret) },
	)

	if len(captured) != 0 {
		t.Fatalf("expected no secret to be captured, got %q", captured)
	}
}

func TestMonitorParentLifelineDeliversSecret(t *testing.T) {
	monitor := startParentLifelineMonitor(strings.NewReader("secret s3cret\nshutdown\n"), true, time.Second, func(int) {})
	defer monitor.markShutdownComplete()

	secret, ok := monitor.waitForSecret(lifelineTestTimeout, nil)
	if !ok || secret != "s3cret" {
		t.Fatalf("expected the parent secret, got %q, ok %t", secret, ok)
	}
	assertLifelineEvent(t, monitor.events, parentLifelineShutdown)
}

// The wait must end as soon as the stream does, not burn the whole window: a
// parent that sends no secret would otherwise stall every startup.
func TestMonitorParentLifelineSecretWaitEndsWithTheStream(t *testing.T) {
	monitor := startParentLifelineMonitor(strings.NewReader(""), true, time.Second, func(int) {})
	defer monitor.markShutdownComplete()

	start := time.Now()
	secret, ok := monitor.waitForSecret(time.Hour, nil)
	if ok || secret != "" {
		t.Fatalf("expected no secret, got %q, ok %t", secret, ok)
	}
	if elapsed := time.Since(start); elapsed > lifelineTestTimeout {
		t.Fatalf("expected the wait to end with the stream, took %s", elapsed)
	}
}

func TestMonitorParentLifelineSecretWaitTimesOut(t *testing.T) {
	// A reader that never yields a line and never ends, like a live parent that
	// has not sent anything yet.
	blocking, release := newBlockingReader()
	defer release()
	monitor := startParentLifelineMonitor(blocking, true, time.Second, func(int) {})
	defer monitor.markShutdownComplete()

	secret, ok := monitor.waitForSecret(50*time.Millisecond, nil)
	if ok || secret != "" {
		t.Fatalf("expected the wait to time out, got %q, ok %t", secret, ok)
	}
}

// newBlockingReader returns a reader that blocks until its stop func is called,
// then reports EOF.
func newBlockingReader() (*blockingReader, func()) {
	r := &blockingReader{release: make(chan struct{})}
	return r, func() { close(r.release) }
}

type blockingReader struct {
	release chan struct{}
}

func (r *blockingReader) Read([]byte) (int, error) {
	<-r.release
	return 0, io.EOF
}

// An OS signal must end the wait: signal.Notify has already disabled Go's
// default terminate behavior by the time this runs, so a quit arriving during
// the window would otherwise sit unhandled until it expires.
func TestWaitForSecretAbortsOnSignal(t *testing.T) {
	blocking, release := newBlockingReader()
	defer release()
	monitor := startParentLifelineMonitor(blocking, true, time.Second, func(int) {})
	defer monitor.markShutdownComplete()

	abort := make(chan os.Signal, 1)
	abort <- syscall.SIGTERM

	start := time.Now()
	secret, ok := monitor.waitForSecret(time.Hour, abort)

	if ok || secret != "" {
		t.Fatalf("expected no secret on abort, got %q, ok %t", secret, ok)
	}
	if elapsed := time.Since(start); elapsed > lifelineTestTimeout {
		t.Fatalf("expected the wait to end at once on a signal, took %s", elapsed)
	}
}

func TestWatchParentLocalAPISecret(t *testing.T) {
	// A parent whose main thread stalls past the window still delivers. Dropping
	// that secret would leave the bootstrap API open for the whole session.
	t.Run("a secret arriving after the window is registered late", func(t *testing.T) {
		registered := make(chan string, 1)
		reader := &delayedReader{after: 150 * time.Millisecond, line: "secret late\n"}
		monitor := startParentLifelineMonitor(reader, true, time.Second, func(int) {})
		defer monitor.markShutdownComplete()

		watchParentLocalAPISecret(monitor, true, 20*time.Millisecond, nil,
			func(secret string) { registered <- secret })

		select {
		case got := <-registered:
			if got != "late" {
				t.Fatalf("expected the late secret, got %q", got)
			}
		case <-time.After(lifelineTestTimeout):
			t.Fatal("expected a late secret to be registered")
		}
	})

	// The legacy Windows path monitors stdin without opting into the lifeline.
	// It must not stall waiting for a secret, but a secret it does receive has
	// to count — otherwise a client that sends one is silently unprotected.
	t.Run("a secret on a non-parented stream is registered without waiting", func(t *testing.T) {
		registered := make(chan string, 1)
		reader := &delayedReader{after: 100 * time.Millisecond, line: "secret windows\n"}
		monitor := startParentLifelineMonitor(reader, false, time.Second, func(int) {})
		defer monitor.markShutdownComplete()

		start := time.Now()
		watchParentLocalAPISecret(monitor, false, time.Hour, nil,
			func(secret string) { registered <- secret })

		if elapsed := time.Since(start); elapsed > lifelineTestTimeout {
			t.Fatalf("a non-parented stream must not be waited on, took %s", elapsed)
		}
		select {
		case got := <-registered:
			if got != "windows" {
				t.Fatalf("expected the secret, got %q", got)
			}
		case <-time.After(lifelineTestTimeout):
			t.Fatal("expected a secret on a non-parented stream to be registered")
		}
	})

	t.Run("a secret within the window is registered before returning", func(t *testing.T) {
		var registered []string
		monitor := startParentLifelineMonitor(strings.NewReader("secret prompt\n"), true, time.Second, func(int) {})
		defer monitor.markShutdownComplete()

		watchParentLocalAPISecret(monitor, true, lifelineTestTimeout, nil,
			func(secret string) { registered = append(registered, secret) })

		if len(registered) != 1 || registered[0] != "prompt" {
			t.Fatalf("expected the secret registered before serving, got %v", registered)
		}
	})

	t.Run("nothing is registered when the stream ends without a secret", func(t *testing.T) {
		monitor := startParentLifelineMonitor(strings.NewReader(""), true, time.Second, func(int) {})
		defer monitor.markShutdownComplete()

		watchParentLocalAPISecret(monitor, true, lifelineTestTimeout, nil,
			func(string) { t.Fatal("registered a secret that was never sent") })
		// The background watch ends with the stream; give it a moment to prove
		// it does not register anything.
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("an empty secret line registers nothing", func(t *testing.T) {
		monitor := startParentLifelineMonitor(strings.NewReader("secret \n"), true, time.Second, func(int) {})
		defer monitor.markShutdownComplete()

		watchParentLocalAPISecret(monitor, true, 50*time.Millisecond, nil,
			func(secret string) { t.Fatalf("registered an empty secret line as %q", secret) })
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("nothing is registered without a monitor", func(t *testing.T) {
		watchParentLocalAPISecret(nil, true, time.Millisecond, nil,
			func(string) { t.Fatal("registered a secret with no lifeline monitor") })
	})
}

// delayedReader yields one line after a pause, then blocks until it is read
// from again — a parent that is slow to write.
type delayedReader struct {
	after time.Duration
	line  string
	done  bool
}

func (r *delayedReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	time.Sleep(r.after)
	r.done = true
	return copy(p, r.line), nil
}
