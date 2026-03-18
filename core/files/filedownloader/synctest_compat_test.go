//go:build go1.26

package filedownloader

import (
	"testing"
	"testing/synctest"
)

func runSynctest(t *testing.T, f func()) {
	t.Helper()
	synctest.Test(t, func(*testing.T) {
		f()
	})
}
