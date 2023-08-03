//go:build !nogrpcserver && !_test && windows

package main

import (
	"os"
)

var signals = commonOSSignals

func shouldSaveStack(signal os.Signal) bool {
	return false
}
