package recovery

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/anyproto/any-sync/commonspace"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacedomain"
)

// viewGateStallBound is how many consecutive responsible tech-space diffs may
// resolve nothing before the SpaceView-completeness gate opens anyway, with
// viewsConfirmed=false. Self-limiting: with the network down no diffs arrive.
const viewGateStallBound = 2

// OnSpaceView is the discovery seam: every SpaceView status the tech-space
// subscription delivers (space.onSpaceStatusUpdated). It is the ONLY source of
// SpaceDiscovered — a space pushed by a LAN peer produces no pull events at
// all — and the only source of "deleted while recovering". Every view id seen
// here also resolves the tech-space completeness gate: SpaceView object id ==
// tree id.
func (t *Tracker) OnSpaceView(spaceId, spaceViewId string, deleted bool) {
	defer containTelemetry("space view")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun || t.terminalLocked() || spaceId == "" || spaceId == t.account.techSpaceId {
		return
	}
	t.resolveViewLocked(spaceViewId)
	if deleted {
		if s, tracked := t.spaces[spaceId]; tracked {
			t.transitionLocked(spaceId, s, pb.EventAccountRecovery_Removed, &errInfo{
				class: pb.EventAccountRecovery_SpaceDeleted,
				debug: "space deleted while recovering",
			})
			delete(t.spaces, spaceId)
		}
		t.forgetSpaceLocked(spaceId)
	} else {
		// Discovery is authoritative: a space it names is present again.
		delete(t.spacesRemoved, spaceId)
		t.spaceLocked(spaceId, spaceViewId, pb.EventAccountRecovery_Regular)
	}
	t.checkFinishedLocked()
	t.armSettleLocked()
	t.refreshPhaseLocked(false)
}

// OnSpaceViewInactive is a SpaceView whose space the space controller will
// never run a spaceloader for. Today that is a pending join:
// AccountStatusJoining dispatches to mode.ModeJoining, which builds a joiner,
// so the space can never publish a load result. Tracking it as one awaiting a
// result wedges Finished for the entire run — and does so on every app open
// for as long as the invite sits unaccepted.
//
// It still resolves the completeness gate: the view exists locally and the
// watcher listed it, so the gate must not wait for it either. A space that
// later becomes active arrives again through OnSpaceView and is tracked then.
func (t *Tracker) OnSpaceViewInactive(spaceId, spaceViewId string) {
	defer containTelemetry("space view inactive")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun || t.terminalLocked() || spaceId == "" || spaceId == t.account.techSpaceId {
		return
	}
	t.resolveViewLocked(spaceViewId)
	// Tracked from an earlier active status: it has left the recovery set, so
	// stop awaiting a result that is no longer coming. Removed says exactly
	// that, and the class stays None because nothing failed and nothing was
	// deleted.
	if s, tracked := t.spaces[spaceId]; tracked {
		t.transitionLocked(spaceId, s, pb.EventAccountRecovery_Removed, &errInfo{
			debug: "no loader will run for this space (pending join)",
		})
		delete(t.spaces, spaceId)
		t.forgetSpaceLocked(spaceId)
	}
	t.checkFinishedLocked()
	t.armSettleLocked()
	t.refreshPhaseLocked(false)
}

// OnSpaceViewsInitial is the SpaceView watcher's first pass over the local
// store (space.spacewatcher, inside watcher.Run). The completeness gate cannot
// open until it has run AND every view it listed has been delivered through
// OnSpaceView: the batch is enqueued asynchronously, and a tech-space diff
// landing in between must not let Finished claim "no more spaces" while a
// local one is still on its way. This protects every mode; on NewAccount it
// is the only thing that does.
func (t *Tracker) OnSpaceViewsInitial(spaceViewIds []string) {
	defer containTelemetry("initial space views")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun || t.terminalLocked() {
		return
	}
	t.views.initialDelivered = true
	for _, id := range spaceViewIds {
		if _, seen := t.views.seen[id]; id != "" && !seen {
			t.views.expected[id] = struct{}{}
		}
	}
	t.checkFinishedLocked()
	t.armSettleLocked()
	t.refreshPhaseLocked(false)
}

// OnSpaceLoadStarted is the spaceloader's startLoad seam. optimistic is the
// on-disk fast path: the loader never publishes Loading for it, so "no
// transition seen" is already loaded here too.
func (t *Tracker) OnSpaceLoadStarted(spaceId string, optimistic bool) {
	defer containTelemetry("space load started")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun || t.terminalLocked() || spaceId == "" {
		return
	}
	s := t.spaceLocked(spaceId, "", pb.EventAccountRecovery_Regular)
	if s == nil {
		return
	}
	next := pb.EventAccountRecovery_Loading
	if optimistic {
		next = pb.EventAccountRecovery_Loaded
	}
	t.transitionLocked(spaceId, s, next, nil)
	t.checkFinishedLocked()
	t.armSettleLocked()
	t.refreshPhaseLocked(false)
}

// OnSpaceLoaded is the spaceloader's onLoad seam: the space controller's own
// load result, which is what finalizes a space for this surface. deleted is
// the loader's deletion verdict (a bool so this package never imports
// spacecore, whose tests import the tracker). A cancelled build is a shutdown,
// not a result — the loader leaves the persisted status untouched and so does
// the fold.
func (t *Tracker) OnSpaceLoaded(spaceId string, err error, deleted bool) {
	defer containTelemetry("space loaded")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun || t.terminalLocked() || spaceId == "" {
		return
	}
	s := t.spaceLocked(spaceId, "", pb.EventAccountRecovery_Regular)
	if s == nil {
		return
	}
	switch {
	case err == nil:
		t.transitionLocked(spaceId, s, pb.EventAccountRecovery_Loaded, nil)
		s.settled = true
	case deleted:
		t.transitionLocked(spaceId, s, pb.EventAccountRecovery_Error, &errInfo{
			class: pb.EventAccountRecovery_SpaceDeleted,
			debug: truncate(err.Error(), debugMessageMax),
		})
		s.settled = true
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return
	default:
		info := classify(err)
		if info == nil {
			return
		}
		if errors.Is(err, spacedomain.ErrUnexpectedSpaceType) {
			info.class, info.retryable = pb.EventAccountRecovery_Unexpected, false
		}
		t.transitionLocked(spaceId, s, pb.EventAccountRecovery_Error, info)
		s.settled = true
	}
	t.checkFinishedLocked()
	t.armSettleLocked()
	t.refreshPhaseLocked(false)
}

// OnHeadSync is the treesyncer's SyncAll seam. It is called for EVERY space
// but consumed ONLY for the tech space: the tech space's diff against a
// responsible node is what proves the SpaceView set is complete, and that is
// the one thing head-sync feeds on this surface. Regular spaces finalize on
// their load result, never on sync — do not generalise this.
func (t *Tracker) OnHeadSync(spaceId, peerId string, missing []string, responsible bool) {
	defer containTelemetry("head sync")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun || t.terminalLocked() || spaceId == "" || spaceId != t.account.techSpaceId {
		return
	}
	if !responsible && !t.run.localOnly {
		// LAN peers are not the source of truth — unless they are the only
		// source there is
		return
	}
	next := make(map[string]struct{}, len(missing))
	for _, id := range missing {
		if _, seen := t.views.seen[id]; !seen {
			next[id] = struct{}{}
		}
	}
	if t.views.diffSeen {
		shrank := false
		for id := range t.views.unresolved {
			if _, still := next[id]; !still {
				shrank = true
				break
			}
		}
		if len(t.views.unresolved) > 0 && !t.views.resolvedSinceDiff && !shrank {
			t.views.stalledDiffs++
		} else {
			t.views.stalledDiffs = 0
		}
	}
	t.views.diffSeen = true
	t.views.unresolved = next
	t.views.resolvedSinceDiff = false
	t.checkFinishedLocked()
	t.armSettleLocked()
	t.refreshPhaseLocked(false)
}

// spacePullLocked folds a regular space's SpacePull progress (the tech space's
// is the account fetch, see account.go).
func (t *Tracker) spacePullLocked(ev commonspace.PullEvent) {
	s := t.spaceLocked(ev.SpaceId, "", pb.EventAccountRecovery_Regular)
	if s == nil {
		return
	}
	switch ev.Kind {
	case commonspace.PullEventWaiting, commonspace.PullEventAttempt:
		if s.state == pb.EventAccountRecovery_Queued || s.state == pb.EventAccountRecovery_Loading {
			t.transitionLocked(ev.SpaceId, s, pb.EventAccountRecovery_Pulling, nil)
		}
	case commonspace.PullEventResult:
		if ev.Err == nil {
			t.transitionLocked(ev.SpaceId, s, pb.EventAccountRecovery_Loading, nil)
			return
		}
		info := classify(ev.Err)
		if info == nil {
			return
		}
		s.attempt++
		t.transitionLocked(ev.SpaceId, s, pb.EventAccountRecovery_Pulling, info)
	}
}

// spaceLocked returns the tracked entry, creating and announcing it when a
// producer sees a space before the SpaceView subscription did (on-demand
// promotion races the watcher). A later view id — or kind — re-announces;
// SpaceDiscovered is idempotent for clients.
func (t *Tracker) spaceLocked(spaceId, spaceViewId string, kind pb.EventAccountRecoverySpaceKind) *spaceState {
	if _, gone := t.spacesRemoved[spaceId]; gone {
		// The space left this run and the client was told to drop it. A load
		// or pull already in flight still reports afterwards, and recreating
		// the entry on that would resurrect a space the client no longer has,
		// inflate spacesTotal, and — for anything short of a verdict — hold
		// Finished open again. Only the SpaceView seam brings a space back.
		return nil
	}
	s, tracked := t.spaces[spaceId]
	if !tracked {
		s = &spaceState{kind: kind, state: pb.EventAccountRecovery_Queued, from: pb.EventAccountRecovery_Queued}
		t.spaces[spaceId] = s
	}
	announce := !tracked
	if spaceViewId != "" && s.spaceViewId != spaceViewId {
		s.spaceViewId = spaceViewId
		announce = true
	}
	if s.kind != kind {
		s.kind = kind
		announce = true
	}
	if announce {
		t.markLocked(&pb.EventAccountRecoveryUpdatePayloadOfSpaceDiscovered{SpaceDiscovered: &pb.EventAccountRecoverySpaceDiscovered{
			SpaceId:     spaceId,
			SpaceViewId: s.spaceViewId,
			Kind:        s.kind,
		}}, nil)
	}
	return s
}

// transitionLocked applies a space state level. A state or error-class edge
// publishes at once; a repeat with only the attempt moved coalesces.
func (t *Tracker) transitionLocked(spaceId string, s *spaceState, next pb.EventAccountRecoverySpaceState, info *errInfo) {
	stateChanged := next != s.state
	errChanged := (s.lastError == nil) != (info == nil) || (info != nil && s.lastError.class != info.class)
	if !stateChanged && !errChanged && info == nil {
		return
	}
	s.from, s.state, s.lastError = s.state, next, info
	var key *coalesceKey
	if !stateChanged && !errChanged {
		key = &coalesceKey{kind: "space", id: spaceId}
	}
	t.markLocked(&pb.EventAccountRecoveryUpdatePayloadOfSpaceStateChanged{SpaceStateChanged: &pb.EventAccountRecoverySpaceStateChanged{
		SpaceId:   spaceId,
		State:     next,
		FromState: s.from,
		Error:     info.toPb(),
		Attempt:   int32(s.attempt),
	}}, key)
}

// resolveViewLocked marks a SpaceView as present for the completeness gate.
func (t *Tracker) resolveViewLocked(spaceViewId string) {
	if spaceViewId == "" {
		return
	}
	t.views.seen[spaceViewId] = struct{}{}
	delete(t.views.expected, spaceViewId)
	if _, pending := t.views.unresolved[spaceViewId]; pending {
		delete(t.views.unresolved, spaceViewId)
		t.views.resolvedSinceDiff = true
		t.views.stalledDiffs = 0
	}
}

// gateLocked answers whether the SpaceView set may be considered complete,
// and whether that was earned (confirmed against the network) or granted by
// the stall bound. Both halves are required: the local views (the watcher's
// first pass, fully delivered) and the network's (a responsible diff).
func (t *Tracker) gateLocked() (open, confirmed bool) {
	if !t.views.diffSeen || !t.views.initialDelivered || len(t.views.expected) > 0 {
		return false, false
	}
	confirmed = len(t.views.unresolved) == 0
	return confirmed || t.views.stalledDiffs >= viewGateStallBound, confirmed
}

// checkFinishedLocked is the terminal rule: the account is ready, the
// SpaceView set is complete (or the stall bound gave up on it), and every
// tracked space has published its load result. Finished carries the verdict;
// the phase then moves to Done and the run goes silent.
func (t *Tracker) checkFinishedLocked() {
	if t.terminalLocked() || !t.account.ready || len(t.spaces) == 0 {
		return
	}
	open, confirmed := t.gateLocked()
	if !open {
		return
	}
	var loaded, failed int32
	for _, s := range t.spaces {
		if !s.settled {
			return
		}
		switch s.state {
		case pb.EventAccountRecovery_Loaded:
			loaded++
		case pb.EventAccountRecovery_Error:
			failed++
		default:
			return
		}
	}
	t.finishLocked(loaded, failed, confirmed)
}

// finishLocked emits the terminal Finished and takes the run silent.
func (t *Tracker) finishLocked(loaded, failed int32, confirmed bool) {
	t.stopSettleTimerLocked()
	t.markLocked(&pb.EventAccountRecoveryUpdatePayloadOfFinished{Finished: &pb.EventAccountRecoveryFinished{
		SpacesTotal:     int32(len(t.spaces)),
		SpacesLoaded:    loaded,
		SpacesFailed:    failed,
		TotalDurationMs: t.clock.Now().Sub(t.run.startedAt).Milliseconds(),
		ViewsConfirmed:  confirmed,
	}}, nil)
	t.phase.finished = true
	t.phase.viewsConfirmed = confirmed
	t.stopOutageTimerLocked()
	t.refreshPhaseLocked(true)
}

// settleBound is how long the run may sit with the local SpaceView set complete
// and nothing at all changing before it stops waiting. The two outcomes are
// deliberately different:
//
//   - Every space reached a terminal state and only the network's confirmation
//     is missing — a warm start with no connectivity loads everything from disk
//     but never sees a tech-space diff. Finished fires with viewsConfirmed
//     false: "ready, but we could not check whether there are spaces you do not
//     have yet."
//   - Some space never published a load result. It becomes Stalled and the run
//     stays OPEN. "79 of 80, one stalled" is a determinate answer a client can
//     offer a retry for, and an open run still reports the space if its load
//     completes later. Claiming Finished here would claim the account is
//     recovered when it demonstrably is not.
//
// Any fold that changes something resets it, so a slow but progressing sync
// never trips it.
const settleBound = 20 * time.Second

// otherwiseSettledLocked: the run has everything it needs to finish except the
// spaces themselves and the network's completeness confirmation.
func (t *Tracker) otherwiseSettledLocked() bool {
	return t.begun && !t.terminalLocked() && t.account.ready &&
		t.views.initialDelivered && len(t.views.expected) == 0 && len(t.spaces) > 0
}

// armSettleLocked restarts the settle bound. Called from every fold that
// changed something: progress is exactly what must postpone the verdict.
func (t *Tracker) armSettleLocked() {
	t.stopSettleTimerLocked()
	if !t.otherwiseSettledLocked() {
		return
	}
	t.settleTimer = t.clock.AfterFunc(settleBound, t.onSettleTimer)
}

func (t *Tracker) stopSettleTimerLocked() {
	if t.settleTimer != nil {
		t.settleTimer.Stop()
		t.settleTimer = nil
	}
}

func (t *Tracker) onSettleTimer() {
	defer containTelemetry("settle timer")
	t.mu.Lock()
	defer t.mu.Unlock()
	t.settleTimer = nil
	if !t.otherwiseSettledLocked() {
		return
	}
	var loaded, failed int32
	stragglers := make([]string, 0, len(t.spaces))
	// usable: shown Loaded on the optimistic fast path, but the background
	// build never reported. The space is on disk and was Ok last session, so it
	// works — there is nothing to stall about, and waiting longer for a build
	// that has gone quiet would hold the run open over a space the user can
	// already open.
	usable := make([]string, 0, len(t.spaces))
	for id, s := range t.spaces {
		switch {
		case s.state == pb.EventAccountRecovery_Loaded && s.settled:
			loaded++
		case s.state == pb.EventAccountRecovery_Loaded:
			usable = append(usable, id)
		case s.state == pb.EventAccountRecovery_Error && s.settled:
			failed++
		default:
			stragglers = append(stragglers, id)
		}
	}
	if len(stragglers) == 0 {
		for _, id := range usable {
			t.spaces[id].settled = true
			loaded++
		}
		// The gate may well be open here: the run was only ever held by a
		// verdict that never came, not by the view set.
		open, confirmed := t.gateLocked()
		t.finishLocked(loaded, failed, open && confirmed)
		return
	}
	// Deterministic order so the event stream is reproducible under replay.
	slices.Sort(stragglers)
	for _, id := range stragglers {
		t.transitionLocked(id, t.spaces[id], pb.EventAccountRecovery_Stalled, nil)
	}
	t.refreshPhaseLocked(false)
}

// forgetSpaceLocked tombstones a space that left the run, so a producer event
// still in flight for it cannot bring it back.
func (t *Tracker) forgetSpaceLocked(spaceId string) {
	if t.spacesRemoved == nil {
		t.spacesRemoved = map[string]struct{}{}
	}
	t.spacesRemoved[spaceId] = struct{}{}
}
