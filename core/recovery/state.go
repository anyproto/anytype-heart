package recovery

import (
	"time"

	"github.com/anyproto/anytype-heart/pb"
)

type runState struct {
	runId     string
	mode      pb.EventAccountRecoveryMode
	networkId string
	startedAt time.Time
	started   bool // Started has been published
	closed    bool
}

type phaseState struct {
	current        pb.EventAccountRecoveryPhase
	startedAt      time.Time
	finished       bool
	failed         *errInfo
	viewsConfirmed bool
}

type peerState struct {
	kind              pb.EventAccountRecoveryPeerKind
	nodeTypes         []string
	open              int // open connections, clamped at zero; never a latched bool
	dialAttempts      int // DialFailed since the last Connected
	lastError         *errInfo
	lastTransport     string
	lastProto         uint32
	discoveredLocally bool
}

type netState struct {
	peers               map[string]*peerState
	openTotal           int
	openNodes           int
	dialsStarted        bool
	outageSince         time.Time // zero when no outage
	incompatibleLatched bool
	waitingForNetwork   bool
	waitingError        *errInfo
	discovery           pb.EventAccountRecoveryDiscoveryState
	deviceOffline       bool
}

type accountState struct {
	techSpaceId  string
	fetchStarted bool
	fetchPeer    string
	attempt      int
	lastError    *errInfo
	ready        bool
	readyAt      time.Time
}

type spaceState struct {
	spaceViewId string
	kind        pb.EventAccountRecoverySpaceKind
	state       pb.EventAccountRecoverySpaceState
	from        pb.EventAccountRecoverySpaceState
	attempt     int
	lastError   *errInfo
}

// viewGate is the tech-space SpaceView-completeness gate on Finished.
type viewGate struct {
	diffSeen     bool
	unresolved   map[string]struct{}
	stalledDiffs int
}

// wirePhaseLocked derives the coarse phase: a monotone max over milestones,
// with WaitingForNetwork as an overlay and the two terminals on top.
func (t *Tracker) wirePhaseLocked() pb.EventAccountRecoveryPhase {
	switch {
	case t.phase.failed != nil:
		return pb.EventAccountRecovery_Failed
	case t.phase.finished:
		return pb.EventAccountRecovery_Done
	case t.net.waitingForNetwork:
		return pb.EventAccountRecovery_WaitingForNetwork
	case t.account.ready:
		return pb.EventAccountRecovery_LoadingSpaces
	case t.account.fetchStarted && t.net.openNodes > 0:
		return pb.EventAccountRecovery_FetchingAccount
	case t.net.dialsStarted:
		return pb.EventAccountRecovery_Connecting
	default:
		return pb.EventAccountRecovery_LookingForPeers
	}
}

// refreshPhaseLocked publishes a PhaseChanged when the derived phase moved.
// force lets a terminal through a closed run (Fail after Close).
func (t *Tracker) refreshPhaseLocked(force bool) {
	next := t.wirePhaseLocked()
	if next == t.phase.current {
		return
	}
	now := t.clock.Now()
	changed := &pb.EventAccountRecoveryPhaseChanged{
		Phase:                   next,
		FromPhase:               t.phase.current,
		PreviousPhaseDurationMs: now.Sub(t.phase.startedAt).Milliseconds(),
	}
	switch next {
	case pb.EventAccountRecovery_Failed:
		changed.Error = t.phase.failed.toPb()
	case pb.EventAccountRecovery_WaitingForNetwork:
		changed.Error = t.net.waitingError.toPb()
	}
	t.phase.current = next
	t.phase.startedAt = now
	t.emitLocked(&pb.EventAccountRecoveryUpdatePayloadOfPhaseChanged{PhaseChanged: changed}, nil, force)
}
