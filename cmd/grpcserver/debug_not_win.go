//go:build !nogrpcserver && !_test && !windows

package main

import (
	"os"
	"syscall"
)

var signals = append(commonOSSignals, syscall.SIGUSR1)

func shouldSaveStack(signal os.Signal) bool {
	return signal == syscall.SIGUSR1
}
