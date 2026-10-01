package device

/*
AI generated

Name: Wake (freeze) tracker
Scope: part of the networkState component

## Responsibility
- Estimates, from consecutive (wall, monotonic) clock observations, whether
  the process was frozen (sleep/suspend) and opens a new wake generation when
  the freeze exceeds the platform threshold (wakeTracker.observe is pure)
- Tracks each generation through observed -> queued -> running -> completed
  (completed is a watermark) so a generation covered by a queued, running or
  completed recovery never triggers another one
- Hosts the networkState glue that every observer shares: observeGapLocked,
  the heartbeat callback, the desktop-only monitor fallback and the
  transition-suppression check
*/

import (
	"fmt"
	"time"

	"go.uber.org/zap"
)

const (
	// wakeGapThreshold is the estimated freeze (sleep/suspend) duration above
	// which a wake generation opens on desktop: recoverAfter plus one
	// heartbeat period.
	wakeGapThreshold = recoverAfter + heartbeatInterval
	// mobileWakeGapThreshold keeps mobile at the pre-GO-7556 sleep threshold
	// (the old 30s clock-jump detector), so mobile never flushes more often
	// than before for short device sleeps.
	mobileWakeGapThreshold = time.Second * 30
	// wakeCoverWindow: a long-background Foreground report skips its own
	// recovery only if a wake generation observed since the last Foreground
	// transition is covered by a recovery that is still queued/running, or
	// the latest recovery (of any kind) completed less than this long ago.
	// Older recoveries don't count: mobile's long-background recovery
	// (GO-7302) must not be swallowed by a network-change recovery that ran
	// while backgrounded.
	wakeCoverWindow = time.Second * 30
)

// Sources of a gap observation.
const (
	observeSourceStart         = "start"
	observeSourceSampler       = "sampler"
	observeSourceStateChange   = "stateChange"
	observeSourceAdmission     = "admission"
	observeSourceTrailing      = "trailingTimer"
	observeSourceRecoveryStart = "recoveryStart"
)

// monoEpoch anchors the default monotonic clock: time.Since uses the
// monotonic reading, which pauses during sleep on macOS, iOS, Linux and
// Android, and keeps counting on Windows.
var monoEpoch = time.Now()

// wakeTracker is the freeze detector state. Guarded by networkState.recoveryMu.
type wakeTracker struct {
	init     bool
	baseWall time.Time
	baseMono time.Duration

	// observed is the latest wake generation (0 = none yet).
	observed int64
	// queued is the highest generation covered by a queued recovery or by the
	// pending coalesced trailing run; running is the highest one bound by a
	// recovery at execution start; completed is the coverage watermark.
	queued    int64
	running   int64
	completed int64
	// completion time of the latest recovery (it covers everything up to
	// completed, since recoveries are serialized and bind generations in
	// order); any recovery refreshes it, wake-triggered or not
	completedWall time.Time
	completedMono time.Duration

	// last generation, for telemetry (the rest is in the WARN log)
	lastAtWall    time.Time
	lastFrozenFor time.Duration
	lastSource    string
}

// gapObservation is what one observe call saw.
type gapObservation struct {
	wallGap, monoGap time.Duration
	drift, frozen    time.Duration
	wallStepBack     bool
	newGen           bool
}

// observe compares (wall, mono) with the previous observation from any source
// and opens a new wake generation when the estimated freeze exceeds
// threshold. Pure: no clocks, no locks, no logging.
//
// Freeze estimate:
//   - monotonic pauses during sleep (macOS, iOS, Linux, Android): the drift
//     wallDelta-monoDelta. Starvation (App Nap, VM steal, swap) advances both
//     clocks and is not a wake; a forward wall step is (one false flush).
//   - Windows (monoCountsSleep): monoDelta minus one heartbeat period, since
//     the monotonic clock includes sleep; wall steps can't fake a wake, but
//     starving the sampler for longer than the threshold can.
func (w *wakeTracker) observe(wall time.Time, mono time.Duration, monoCountsSleep bool, period, threshold time.Duration) (o gapObservation) {
	if !w.init {
		w.init, w.baseWall, w.baseMono = true, wall, mono
		return
	}
	o.monoGap = mono - w.baseMono
	if o.monoGap < 0 {
		// can't happen with a real monotonic clock; never move the baseline
		// backwards
		o.monoGap = 0
	} else {
		w.baseMono = mono
	}
	o.wallGap = wall.Sub(w.baseWall)
	if o.wallGap < 0 {
		// Backward wall step (NTP, manual change): no gap. Re-anchor the wall
		// baseline to the stepped clock, otherwise drift detection (the only
		// sleep signal on macOS/Linux) would stay blind until the clock
		// catches up with the old baseline. A sleep inside the same sample
		// interval as the step is cancelled out by it (known limit).
		o.wallStepBack = true
		o.wallGap = 0
	}
	w.baseWall = wall
	o.drift = o.wallGap - o.monoGap
	o.frozen = o.drift
	if monoCountsSleep {
		o.frozen = o.monoGap - period
	}
	if o.frozen <= threshold {
		return
	}
	w.observed++
	w.lastAtWall, w.lastFrozenFor = wall, o.frozen
	o.newGen = true
	return
}

// coveredMax is the highest generation any queued, running or completed
// recovery covers.
func (w *wakeTracker) coveredMax() int64 {
	return max(w.queued, w.running, w.completed)
}

// uncovered reports a wake generation that no recovery covers yet.
func (w *wakeTracker) uncovered() bool {
	return w.observed > w.coveredMax()
}

// outcome describes the latest generation, derived from the watermarks.
func (w *wakeTracker) outcome(mobile bool) string {
	switch {
	case w.observed == 0:
		return ""
	case w.observed <= w.completed:
		return "completed"
	case w.observed <= w.running:
		return "running"
	case w.observed <= w.queued:
		return "queued"
	case mobile:
		return "pendingForeground"
	default:
		return "observed"
	}
}

func (n *networkState) wakeThreshold() time.Duration {
	if n.mobile {
		return mobileWakeGapThreshold
	}
	return wakeGapThreshold
}

// observeGapLocked is the single freeze detector shared by the heartbeat
// sampler, StateChange, recovery admission, the trailing timer and recovery
// execution start. It reads both clocks itself (under recoveryMu, so
// observations are ordered). Every caller observes before deciding, so
// whichever source sees a freeze first creates the generation and everyone
// else reuses it.
func (n *networkState) observeGapLocked(source string) (newGen bool) {
	w := &n.wake
	o := w.observe(n.wallNow(), n.monoNow(), n.monoCountsSleep, n.heartbeatPeriod(), n.wakeThreshold())
	if o.wallStepBack {
		log.Info("wall clock stepped back; drift baseline re-anchored")
	}
	if !o.newGen {
		return false
	}
	w.lastSource = source
	rule := "drift"
	if n.monoCountsSleep {
		rule = "monoGap"
	}
	// WARN on purpose: once per wake, and user builds log most loggers at
	// WARN, so this is the field evidence for GO-7556
	log.Warn("wake observed",
		zap.Int64("wakeGen", w.observed),
		zap.String("source", source),
		zap.String("rule", rule),
		zap.Duration("drift", o.drift),
		zap.Duration("frozenFor", o.frozen),
		zap.Duration("wallGap", o.wallGap),
		zap.Duration("monoGap", o.monoGap),
		zap.String("deviceState", deviceStateName(n.lastDeviceState, n.deviceStateReported)),
		zap.String("prevDeviceState", n.prevDeviceState))
	return true
}

// desktopFallbackLocked: on desktop any uncovered wake generation recovers on
// its own, whatever the stored device state, so a lost suspend and a lost
// resume report are both covered. Mobile waits for the Foreground report
// (avoids flushing on background fetch / push wakes).
func (n *networkState) desktopFallbackLocked(newGen bool) {
	if !n.wake.uncovered() {
		return
	}
	if n.mobile {
		if newGen {
			log.Warn("wake: mobile, recovery waits for the Foreground report", zap.Int64("wakeGen", n.wake.observed))
		}
		return
	}
	log.Warn("wake: desktop fallback recovery", zap.Int64("wakeGen", n.wake.observed),
		zap.String("deviceState", deviceStateName(n.lastDeviceState, n.deviceStateReported)))
	n.admitLocked(fmt.Sprintf("wake from sleep (desktop fallback, wakeGen %d, frozen %s)",
		n.wake.observed, n.wake.lastFrozenFor.Round(time.Second)), triggerDesktopFallback, false)
}

// onHeartbeat is the sampler callback: observe the gap and, on desktop,
// recover any uncovered wake generation. Never blocks (admission only
// enqueues).
func (n *networkState) onHeartbeat() {
	n.recoveryMu.Lock()
	defer n.recoveryMu.Unlock()
	if n.closed {
		return
	}
	n.desktopFallbackLocked(n.observeGapLocked(observeSourceSampler))
}

// coveredByRecentWakeRecoveryLocked reports whether the long-background
// transition recovery is redundant: a wake generation observed since the last
// Foreground transition is queued or running, or the latest recovery, which
// covers it, completed less than wakeCoverWindow ago.
func (n *networkState) coveredByRecentWakeRecoveryLocked() bool {
	w := &n.wake
	g := w.observed
	if g == 0 || g <= n.backgroundGenBase {
		return false
	}
	if g > w.completed {
		return g <= w.coveredMax() // queued or running
	}
	// g <= completed with g > 0: a recovery has completed, so completedWall/
	// completedMono are set
	return n.sleepAwareSince(w.completedWall, w.completedMono) < wakeCoverWindow
}
