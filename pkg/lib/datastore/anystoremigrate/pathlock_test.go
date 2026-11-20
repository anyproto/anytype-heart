package anystoremigrate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLockPath(t *testing.T) {
	t.Run("a second holder of a path waits for the first", func(t *testing.T) {
		// given
		unlock := LockPath("/store/a.db")
		got := make(chan struct{})

		// when
		go func() {
			defer close(got)
			LockPath("/store/a.db")()
		}()

		// then
		select {
		case <-got:
			t.Fatal("the lock was taken twice")
		case <-time.After(50 * time.Millisecond):
		}
		unlock()
		<-got
	})

	t.Run("other paths are not held", func(t *testing.T) {
		// given
		unlock := LockPath("/store/a.db")
		defer unlock()

		// when
		done := make(chan struct{})
		go func() {
			defer close(done)
			LockPath("/store/b.db")()
		}()

		// then
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("another path waited")
		}
	})

	t.Run("a released path leaves no entry", func(t *testing.T) {
		// when
		LockPath("/store/c.db")()

		// then
		pathLocks.Lock()
		defer pathLocks.Unlock()
		assert.NotContains(t, pathLocks.held, "/store/c.db")
	})
}
