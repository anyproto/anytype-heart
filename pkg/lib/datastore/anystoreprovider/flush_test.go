package anystoreprovider

import (
	"testing"
	"time"

	anystore "github.com/anyproto/any-store/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A flush covers every open crdt database, also one whose getter lock another caller holds, and
// skips without waiting only those still opening.
func TestFlushCoversOpenCrdtDbsWhoeverHoldsTheirLock(t *testing.T) {
	// given
	p := newTestProvider(t)
	openDb, err := p.GetCrdtDb("open").Wait()
	require.NoError(t, err)
	// a caller paused inside Wait of the open database, and an open in progress of another
	open := p.GetCrdtDb("open")
	opening := p.GetCrdtDb("opening")
	open.lock.Lock()
	opening.lock.Lock()
	defer open.lock.Unlock()
	defer opening.lock.Unlock()
	want := []anystore.DB{openDb, p.GetCommonDb()}

	// when
	got := p.flushTargets()
	flushed := make(chan struct{})
	go func() {
		p.Flush(5*time.Second, false, anystore.FlushModeCheckpointPassive)
		close(flushed)
	}()

	// then
	assert.ElementsMatch(t, want, got)
	select {
	case <-flushed:
	case <-time.After(10 * time.Second):
		t.Fatal("the flush waited for a getter lock")
	}
	// and Wait of the open database does not need the lock either
	again, err := open.Wait()
	require.NoError(t, err)
	assert.Same(t, openDb, again)
}
