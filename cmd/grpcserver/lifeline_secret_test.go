//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package main

import (
	"io"
	"strings"
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

	secret, ok := monitor.waitForSecret(lifelineTestTimeout)
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
	secret, ok := monitor.waitForSecret(time.Hour)
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
	blocking, _ := newBlockingReader()
	monitor := startParentLifelineMonitor(blocking, true, time.Second, func(int) {})
	defer monitor.markShutdownComplete()

	secret, ok := monitor.waitForSecret(50 * time.Millisecond)
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

func TestParentLocalAPISecret(t *testing.T) {
	t.Run("returns the secret in parented mode", func(t *testing.T) {
		monitor := startParentLifelineMonitor(strings.NewReader("secret s3cret\n"), true, time.Second, func(int) {})
		defer monitor.markShutdownComplete()

		secret, ok := parentLocalAPISecret(monitor, true)
		if !ok || secret != "s3cret" {
			t.Fatalf("expected the parent secret, got %q, ok %t", secret, ok)
		}
	})

	t.Run("reports none when the stream ends without one", func(t *testing.T) {
		monitor := startParentLifelineMonitor(strings.NewReader(""), true, time.Second, func(int) {})
		defer monitor.markShutdownComplete()

		secret, ok := parentLocalAPISecret(monitor, true)
		if ok || secret != "" {
			t.Fatalf("expected no secret, got %q, ok %t", secret, ok)
		}
	})

	t.Run("reports none for an empty secret line", func(t *testing.T) {
		monitor := startParentLifelineMonitor(strings.NewReader("secret \n"), true, time.Second, func(int) {})
		defer monitor.markShutdownComplete()

		secret, ok := parentLocalAPISecret(monitor, true)
		if ok || secret != "" {
			t.Fatalf("expected an empty secret line to be rejected, got %q, ok %t", secret, ok)
		}
	})

	// Standalone, Docker and the legacy Windows stdin protocol have no parent
	// channel. Waiting on one there would stall every launch for the full
	// window before the server can serve.
	t.Run("does not wait when the lifeline is not parented", func(t *testing.T) {
		blocking, release := newBlockingReader()
		defer release()
		monitor := startParentLifelineMonitor(blocking, false, time.Second, func(int) {})
		defer monitor.markShutdownComplete()

		start := time.Now()
		if _, ok := parentLocalAPISecret(monitor, false); ok {
			t.Fatal("expected no secret outside parented mode")
		}
		if elapsed := time.Since(start); elapsed > lifelineTestTimeout {
			t.Fatalf("expected an immediate return outside parented mode, took %s", elapsed)
		}
	})

	t.Run("reports none without a monitor", func(t *testing.T) {
		if _, ok := parentLocalAPISecret(nil, true); ok {
			t.Fatal("expected no secret without a lifeline monitor")
		}
	})
}
