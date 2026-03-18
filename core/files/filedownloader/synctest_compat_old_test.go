//go:build !go1.26

package filedownloader

import (
	"testing"
	"testing/synctest"
)

func runSynctest(_ *testing.T, f func()) {
	synctest.Run(f)
}
