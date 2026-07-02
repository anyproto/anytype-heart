package objectstore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The counter is ordering-only (queue generations, crash-recovery comparison):
// it must stay strictly monotonic across backwards clock steps and must never
// block — sleeping toward a pinned future second used to stall callers (one of
// them holding an open common-DB write tx) for the whole clock-step duration.
func TestGenerateFTQueueCounter(t *testing.T) {
	restore := ftQueueCounter.Load()
	defer ftQueueCounter.Store(restore)

	t.Run("monotonic within a second", func(t *testing.T) {
		a := GenerateFTQueueCounter()
		b := GenerateFTQueueCounter()
		assert.Greater(t, b, a)
	})

	t.Run("backwards clock step stays monotonic", func(t *testing.T) {
		// seed the counter as if it was issued one hour in the future
		// (equivalent to the wall clock stepping back one hour afterwards)
		futureTs := uint64(time.Now().Unix() + 3600)
		ftQueueCounter.Store(futureTs * 10000)

		got := GenerateFTQueueCounter()
		assert.Greater(t, got, futureTs*10000, "counter must not go backwards with the clock")
	})

	t.Run("sequence exhaustion borrows the next second instead of sleeping", func(t *testing.T) {
		futureTs := uint64(time.Now().Unix() + 3600)
		ftQueueCounter.Store(futureTs*10000 + 9999) // exhausted second, pinned in the future

		start := time.Now()
		got := GenerateFTQueueCounter()
		elapsed := time.Since(start)

		assert.Equal(t, (futureTs+1)*10000, got)
		assert.Less(t, elapsed, time.Second, "must not sleep toward the pinned future second")
	})
}
