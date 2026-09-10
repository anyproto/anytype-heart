//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	lifelineTestRoleEnv = "ANYTYPE_LIFELINE_TEST_ROLE"
	lifelineTestPIDLine = "LIFELINE_CHILD_PID="
	lifelineTestTimeout = 5 * time.Second
)

func TestParentLifelineEnabled(t *testing.T) {
	t.Setenv(parentLifelineEnv, parentLifelineStdin)
	if !parentLifelineEnabled() {
		t.Fatal("expected stdin parent lifeline to be enabled")
	}

	t.Setenv(parentLifelineEnv, "unknown")
	if parentLifelineEnabled() {
		t.Fatal("expected unknown parent lifeline mode to be disabled")
	}
}

func TestMonitorParentLifelineShutdownMessage(t *testing.T) {
	monitor := startParentLifelineMonitor(strings.NewReader("ignored\nshutdown\n"), true, time.Second, func(int) {})
	defer monitor.markShutdownComplete()

	assertLifelineEvent(t, monitor.events, parentLifelineShutdown)
}

func TestMonitorParentLifelineEOF(t *testing.T) {
	monitor := startParentLifelineMonitor(strings.NewReader(""), true, time.Second, func(int) {})
	defer monitor.markShutdownComplete()

	assertLifelineEvent(t, monitor.events, parentLifelineClosed)
}

func TestMonitorParentLifelineReadErrorFailsClosed(t *testing.T) {
	monitor := startParentLifelineMonitor(errorReader{}, true, time.Second, func(int) {})
	defer monitor.markShutdownComplete()

	assertLifelineEvent(t, monitor.events, parentLifelineClosed)
}

func TestMonitorParentLifelineForcesExitWithoutMainLoop(t *testing.T) {
	deadline := make(chan time.Time, 1)
	exitCode := make(chan int, 1)
	monitor := startParentLifelineMonitorWithDeadline(
		strings.NewReader(""),
		true,
		time.Hour,
		deadlineFactory(deadline),
		func(code int) { exitCode <- code },
	)
	defer monitor.markShutdownComplete()

	assertLifelineEvent(t, monitor.events, parentLifelineClosed)
	deadline <- time.Now()
	assertExitCode(t, exitCode, 1)
}

func TestMonitorParentLifelineCompletionCancelsDeadline(t *testing.T) {
	deadline := make(chan time.Time, 1)
	exitCode := make(chan int, 1)
	monitor := startParentLifelineMonitorWithDeadline(
		strings.NewReader(""),
		true,
		time.Hour,
		deadlineFactory(deadline),
		func(code int) { exitCode <- code },
	)

	assertLifelineEvent(t, monitor.events, parentLifelineClosed)
	monitor.markShutdownComplete()
	assertMonitorStopped(t, monitor)

	select {
	case code := <-exitCode:
		t.Fatalf("unexpected forced exit after completed shutdown: %d", code)
	default:
	}
}

func TestLegacyStdinShutdownMessageStillWorks(t *testing.T) {
	event, shouldShutdown := readParentLifeline(strings.NewReader("shutdown\n"), false)

	if !shouldShutdown || event != parentLifelineShutdown {
		t.Fatalf("expected legacy shutdown message, got event %q, shutdown %t", event, shouldShutdown)
	}
}

func TestLegacyStdinShutdownDoesNotArmDeadline(t *testing.T) {
	deadlineCreated := make(chan struct{}, 1)
	monitor := startParentLifelineMonitorWithDeadline(
		strings.NewReader("shutdown\n"),
		false,
		time.Hour,
		func(time.Duration) parentLifelineDeadline {
			deadlineCreated <- struct{}{}
			return deadlineFactory(make(chan time.Time))(time.Hour)
		},
		func(int) {},
	)

	assertLifelineEvent(t, monitor.events, parentLifelineShutdown)
	assertMonitorStopped(t, monitor)
	select {
	case <-deadlineCreated:
		t.Fatal("legacy stdin shutdown unexpectedly armed the parent deadline")
	default:
	}
}

func TestLegacyStdinEOFDoesNotShutdown(t *testing.T) {
	event, shouldShutdown := readParentLifeline(strings.NewReader(""), false)

	if shouldShutdown {
		t.Fatalf("expected legacy stdin EOF to be ignored, got %q", event)
	}
}

func TestParentDeathClosesLifelinePipe(t *testing.T) {
	switch os.Getenv(lifelineTestRoleEnv) {
	case "parent":
		runLifelineTestParent()
		return
	case "child":
		runLifelineTestChild()
		return
	}

	parent := exec.Command(os.Args[0], "-test.run=^TestParentDeathClosesLifelinePipe$")
	parent.Env = lifelineTestEnv("parent")
	parent.Stderr = os.Stderr
	stdout, err := parent.StdoutPipe()
	if err != nil {
		t.Fatalf("create parent stdout pipe: %v", err)
	}
	if err = parent.Start(); err != nil {
		t.Fatalf("start parent fixture: %v", err)
	}

	var child *os.Process
	defer func() {
		if child != nil {
			_ = child.Kill()
		}
		_ = parent.Process.Kill()
		_ = parent.Wait()
	}()

	pidResult := make(chan int, 1)
	outputClosed := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if !scanner.Scan() {
			scanErr := scanner.Err()
			if scanErr == nil {
				scanErr = io.ErrUnexpectedEOF
			}
			outputClosed <- fmt.Errorf("read child pid: %w", scanErr)
			return
		}

		line := strings.TrimSpace(scanner.Text())
		pid, parseErr := strconv.Atoi(strings.TrimPrefix(line, lifelineTestPIDLine))
		if parseErr != nil || !strings.HasPrefix(line, lifelineTestPIDLine) {
			outputClosed <- fmt.Errorf("invalid child pid line %q", line)
			return
		}
		pidResult <- pid

		for scanner.Scan() {
		}
		outputClosed <- scanner.Err()
	}()

	select {
	case pid := <-pidResult:
		child, err = os.FindProcess(pid)
		if err != nil {
			t.Fatalf("find child fixture: %v", err)
		}
	case outputErr := <-outputClosed:
		t.Fatalf("parent fixture exited before reporting child: %v", outputErr)
	case <-time.After(lifelineTestTimeout):
		t.Fatal("parent fixture did not report its child")
	}

	if err = parent.Process.Kill(); err != nil {
		t.Fatalf("kill parent fixture: %v", err)
	}

	select {
	case outputErr := <-outputClosed:
		if outputErr != nil {
			t.Fatalf("read fixture output: %v", outputErr)
		}
	case <-time.After(lifelineTestTimeout):
		t.Fatal("lifeline child remained alive after its parent was killed")
	}
}

func runLifelineTestParent() {
	child := exec.Command(os.Args[0], "-test.run=^TestParentDeathClosesLifelinePipe$")
	child.Env = lifelineTestEnv("child")
	stdin, err := child.StdinPipe()
	if err != nil {
		os.Exit(2)
	}
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err = child.Start(); err != nil {
		os.Exit(2)
	}

	_, _ = fmt.Fprintf(os.Stdout, "%s%d\n", lifelineTestPIDLine, child.Process.Pid)
	_ = child.Wait()
	runtime.KeepAlive(stdin)
	os.Exit(3)
}

func runLifelineTestChild() {
	startParentLifelineMonitor(os.Stdin, true, 100*time.Millisecond, os.Exit)
	select {}
}

func lifelineTestEnv(role string) []string {
	prefix := lifelineTestRoleEnv + "="
	env := make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, prefix) {
			env = append(env, value)
		}
	}
	return append(env, prefix+role)
}

func assertLifelineEvent(t *testing.T, events <-chan parentLifelineEvent, expected parentLifelineEvent) {
	t.Helper()
	select {
	case event := <-events:
		if event != expected {
			t.Fatalf("expected %q, got %q", expected, event)
		}
	case <-time.After(lifelineTestTimeout):
		t.Fatalf("timed out waiting for %q", expected)
	}
}

func assertExitCode(t *testing.T, exitCode <-chan int, expected int) {
	t.Helper()
	select {
	case code := <-exitCode:
		if code != expected {
			t.Fatalf("expected forced exit code %d, got %d", expected, code)
		}
	case <-time.After(lifelineTestTimeout):
		t.Fatal("timed out waiting for forced exit")
	}
}

func assertMonitorStopped(t *testing.T, monitor *parentLifelineMonitor) {
	t.Helper()
	select {
	case <-monitor.stopped:
	case <-time.After(lifelineTestTimeout):
		t.Fatal("timed out waiting for parent lifeline monitor to stop")
	}
}

func deadlineFactory(elapsed <-chan time.Time) parentLifelineDeadlineFactory {
	return func(time.Duration) parentLifelineDeadline {
		return parentLifelineDeadline{
			elapsed: elapsed,
			stop:    func() {},
		}
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}
