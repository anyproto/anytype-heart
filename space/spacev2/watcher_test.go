package spacev2

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/dedupqueue"
)

type recordingApplier struct {
	mu       sync.Mutex
	statuses []spaceViewStatus
}

func (r *recordingApplier) onSpaceStatusUpdated(status spaceViewStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses = append(r.statuses, status)
}

func (r *recordingApplier) spaceIds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.statuses))
	for _, s := range r.statuses {
		ids = append(ids, s.spaceId)
	}
	return ids
}

// The watcher pipes coalesced statuses to the applier. We drive the dedup
// queue directly (the subscription integration is covered by the Task 8
// service-level enumeration test with the real subscription fixture).
func TestWatcher_CoalescesPerSpace(t *testing.T) {
	// given
	applier := &recordingApplier{}
	queue := dedupqueue.New(0)
	w := &spaceWatcher{queue: queue, applier: applier}
	queue.Run()
	defer queue.Close()

	// when: many updates for one space and one for another
	for i := 0; i < 5; i++ {
		w.enqueue(spaceViewStatus{spaceId: "spaceA", spaceViewId: "viewA"})
	}
	w.enqueue(spaceViewStatus{spaceId: "spaceB", spaceViewId: "viewB"})

	// then: spaceB is applied exactly once; spaceA at least once and at most 5x
	require.Eventually(t, func() bool {
		ids := applier.spaceIds()
		return countIds(ids, "spaceA") > 0 && countIds(ids, "spaceB") > 0
	}, time.Second, 5*time.Millisecond)
	assert.LessOrEqual(t, countIds(applier.spaceIds(), "spaceA"), 5)
	assert.Equal(t, 1, countIds(applier.spaceIds(), "spaceB"))
}

func countIds(ids []string, id string) int {
	n := 0
	for _, i := range ids {
		if i == id {
			n++
		}
	}
	return n
}
