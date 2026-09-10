//go:build !nogrpcserver && !_test && windows
// +build !nogrpcserver,!_test,windows

package main

import "testing"

func TestIgnoreBrokenPipeSignal(t *testing.T) {
	t.Skip("SIGPIPE is a POSIX signal")
}
