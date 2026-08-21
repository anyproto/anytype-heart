package acl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestParticipantSub_CloseWithoutRun ensures Close returns promptly when Run
// was never started — e.g. when Init of an enclosing component fails and the
// app framework tears down partially-initialized children. Before this was
// fixed Close blocked forever waiting on the Run-side waiter channel, hanging
// app shutdown for the full test timeout.
func TestParticipantSub_CloseWithoutRun(t *testing.T) {
	sub := newParticipantGetter("id", "ownIdentity", nil, nil, nil)

	done := make(chan error, 1)
	go func() { done <- sub.Close() }()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("participantSub.Close blocked when Run was never called")
	}
}
