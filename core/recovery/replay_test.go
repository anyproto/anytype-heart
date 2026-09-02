package recovery

import (
	"context"
	"errors"
	"testing"

	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

// clientModel is what a client folds the event log into. The replay property
// — the model after every event equals the tracker's own Snapshot — is what
// makes "push and pull cannot drift" a tested fact rather than a promise.
// Each phase of the implementation extends apply with the payloads it adds;
// an unhandled payload fails the test on purpose.
type clientModel struct {
	runId       string
	lastEventId int64
	mode        pb.EventAccountRecoveryMode
	networkId   string
	phase       pb.EventAccountRecoveryPhase
	done        bool
	err         *pb.EventAccountRecoveryErrorInfo
}

func (m *clientModel) apply(t *testing.T, u *pb.EventAccountRecoveryUpdate) {
	t.Helper()
	if u.RunId != m.runId {
		*m = clientModel{runId: u.RunId}
	}
	require.Equal(t, m.lastEventId+1, u.Id, "ids are contiguous")
	m.lastEventId = u.Id
	switch p := u.Payload.(type) {
	case *pb.EventAccountRecoveryUpdatePayloadOfStarted:
		m.mode = p.Started.Mode
		m.networkId = p.Started.NetworkId
		m.phase = pb.EventAccountRecovery_LookingForPeers
	case *pb.EventAccountRecoveryUpdatePayloadOfPhaseChanged:
		require.Equal(t, m.phase, p.PhaseChanged.FromPhase)
		m.phase = p.PhaseChanged.Phase
		switch m.phase {
		case pb.EventAccountRecovery_Failed:
			m.done = true
			m.err = p.PhaseChanged.Error
		case pb.EventAccountRecovery_Done:
			m.done = true
		}
	default:
		t.Fatalf("replay model does not handle %T", u.Payload)
	}
}

func (m *clientModel) assertMatches(t *testing.T, snap *pb.EventAccountRecoverySnapshot) {
	t.Helper()
	want := &clientModel{
		runId:       snap.RunId,
		lastEventId: snap.LastEventId,
		mode:        snap.Mode,
		networkId:   snap.NetworkId,
		phase:       snap.Phase,
		done:        snap.Done,
		err:         snap.Error,
	}
	assert.Equal(t, want, m)
}

func TestReplayProperty(t *testing.T) {
	t.Run("folding the log reproduces the snapshot at every step", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		model := &clientModel{}
		applied := 0
		check := func() {
			t.Helper()
			for _, u := range fx.sender.updates()[applied:] {
				model.apply(t, u)
				applied++
			}
			model.assertMatches(t, fx.Snapshot())
		}

		// when / then
		fx.init(t)
		check()
		fx.Fail(errors.Join(errors.New("init tech space"), spacesyncproto.ErrSpaceIsDeleted))
		check()
		require.NoError(t, fx.Close(context.Background()))
		check()

		// a new run resets the model through the runId change
		fx.Begin(Run{Mode: pb.EventAccountRecovery_WarmStart, Sender: fx.sender})
		fx.init(t)
		check()
	})
}
