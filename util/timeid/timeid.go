package timeid

import (
	"sync/atomic"
	"time"
)

var lastUsed int64

// NewNano generates a new ID based on the current time in nanoseconds + atomic counter in the lower bits in case of a collision.
// Within the app lifetime it's guaranteed to be unique and strictly increasing, even across multiple goroutines.
func NewNano() int64 {
	for {
		// Snapshot the current global 64-bit value
		old := atomic.LoadInt64(&lastUsed)

		// Construct the new value: timestamp (in ms) << SHIFT
		now := time.Now().UnixNano()

		// If old already >= now, we are in the same or “earlier” millisecond.
		// Just bump old by 1 to ensure strictly increasing.
		newVal := now
		if old >= now {
			newVal = old + 1
		}

		// Try to swap. If successful, return the new value.
		if atomic.CompareAndSwapInt64(&lastUsed, old, newVal) {
			return newVal
		}
		// Otherwise, loop and try again.
	}
}
