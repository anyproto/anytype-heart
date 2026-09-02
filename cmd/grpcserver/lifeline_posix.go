//go:build !nogrpcserver && !_test && !windows
// +build !nogrpcserver,!_test,!windows

package main

import (
	"os/signal"
	"syscall"
)

func ignoreBrokenPipeSignal() {
	// Electron owns stdout and stderr pipes. If it disappears, writes during
	// cleanup must return EPIPE rather than terminating cleanup with SIGPIPE.
	signal.Ignore(syscall.SIGPIPE)
}
