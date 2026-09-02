package recovery

import (
	"errors"

	"github.com/anyproto/any-sync/commonspace"

	"github.com/anyproto/anytype-heart/pb"
)

// ErrAccountNotFound is a sentinel for upper layers to join into a start
// error when one of their own sentinels (space.ErrSpaceNotExists) means the
// account is not on this network. Classification stays in one place without
// this package importing the space tree.
var ErrAccountNotFound = errors.New("account not found on this network")

// OnTechSpaceId tells the tracker which space is the account's. It is called
// from space.Init, before Run starts the pull, so the tech-space pull is
// labelled as the account fetch while it happens.
func (t *Tracker) OnTechSpaceId(id string) {
	defer containTelemetry("tech space id")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun {
		return
	}
	t.account.techSpaceId = id
}

// ObservePullEvent implements commonspace.PullObserver. Only the tech space
// is folded here: it is the account fetch. Regular spaces get their Pulling
// state from the space state machine.
func (t *Tracker) ObservePullEvent(ev commonspace.PullEvent) {
	defer containTelemetry("pull event")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun || ev.SpaceId == "" || ev.SpaceId != t.account.techSpaceId {
		return
	}
	switch ev.Kind {
	case commonspace.PullEventWaiting:
		// once per pull round, before the responsible-peer lookup, which
		// blocks until a peer appears: this is the only sign the wait began
		t.account.fetchStarted = true
		t.account.attempt++
		t.account.fetchPeer = ""
		t.markLocked(t.accountFetchStartedLocked(ev.SpaceId, ""), nil)
	case commonspace.PullEventAttempt:
		t.account.fetchPeer = ev.PeerId
		t.markLocked(t.accountFetchStartedLocked(ev.SpaceId, ev.PeerId), nil)
	case commonspace.PullEventResult:
		if ev.Err == nil {
			// AccountReady follows once the space is open
			return
		}
		info := classifyAccount(ev.Err)
		if info == nil {
			return // cancelled: the caller gave up, not a failure
		}
		t.account.lastError = info
		t.markLocked(&pb.EventAccountRecoveryUpdatePayloadOfAccountFetchError{AccountFetchError: &pb.EventAccountRecoveryAccountFetchError{
			PeerId:  ev.PeerId,
			Error:   info.toPb(),
			Attempt: int32(t.account.attempt),
		}}, nil)
	default:
		return
	}
	t.refreshPhaseLocked(false)
}

func (t *Tracker) accountFetchStartedLocked(spaceId, peerId string) pb.IsEventAccountRecoveryUpdatePayload {
	return &pb.EventAccountRecoveryUpdatePayloadOfAccountFetchStarted{AccountFetchStarted: &pb.EventAccountRecoveryAccountFetchStarted{
		SpaceId: spaceId,
		PeerId:  peerId,
		Attempt: int32(t.account.attempt),
	}}
}

// OnAccountReady is the "UI may start" milestone: the tech space and its
// account object are present. Fired where space closes techSpaceReady, on
// every path that does so; idempotent. The tech space enters the space log
// here as Loaded — it is never Error, a tech-space failure is the run's
// Failed.
func (t *Tracker) OnAccountReady() {
	defer containTelemetry("account ready")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun || t.account.ready {
		return
	}
	now := t.clock.Now()
	t.account.ready = true
	t.account.readyAt = now
	t.account.lastError = nil
	t.markLocked(&pb.EventAccountRecoveryUpdatePayloadOfAccountReady{AccountReady: &pb.EventAccountRecoveryAccountReady{
		DurationMs: now.Sub(t.run.startedAt).Milliseconds(),
	}}, nil)
	if id := t.account.techSpaceId; id != "" {
		t.spaces[id] = &spaceState{
			kind:  pb.EventAccountRecovery_Tech,
			state: pb.EventAccountRecovery_Loaded,
			from:  pb.EventAccountRecovery_Queued,
		}
		t.markLocked(&pb.EventAccountRecoveryUpdatePayloadOfSpaceDiscovered{SpaceDiscovered: &pb.EventAccountRecoverySpaceDiscovered{
			SpaceId: id,
			Kind:    pb.EventAccountRecovery_Tech,
		}}, nil)
		t.markLocked(&pb.EventAccountRecoveryUpdatePayloadOfSpaceStateChanged{SpaceStateChanged: &pb.EventAccountRecoverySpaceStateChanged{
			SpaceId:   id,
			State:     pb.EventAccountRecovery_Loaded,
			FromState: pb.EventAccountRecovery_Queued,
		}}, nil)
	}
	t.refreshPhaseLocked(false)
}
