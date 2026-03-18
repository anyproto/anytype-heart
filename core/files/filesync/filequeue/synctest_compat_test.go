//go:build go1.26

package filequeue

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
