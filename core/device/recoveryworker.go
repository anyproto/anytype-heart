package device

/*
AI generated

Name: Connectivity recovery worker
Scope: part of the networkState component

## Responsibility
- Admits recoveries leading-edge with a suppression window (bursts coalesce
  into one trailing run) and hands them to one serialized worker without
  blocking the caller
- Runs the pipeline: pool Flush (bounded by flushTimeout; never overlapping an
  abandoned one: waits for it at most another flushTimeout, else skips this
  Flush), completes the bound wake generation (with any-sync pool generations
  Flush invalidates every pre-flush peer synchronously), then connectivity
  hooks, head-sync and the opened-objects refresh (bounded) when a Foreground
  transition asked for it
- Logs wake/lifecycle-related runs at WARN (visible in user builds), plain
  network-change runs at Info
- After CompStateAppClosingInitiated or Close: completes queued generations
  without flushing and skips hooks, head-sync and refresh
*/

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

const (
	// recoverySuppressWindow bounds how often the recovery pipeline can run.
	// Signals arrive in bursts (wake fires the freeze detector, the interface
	// diff and the client's foreground RPC within seconds); the first one runs
	// immediately, the rest coalesce into at most one trailing run so fresh
	// connections aren't torn down repeatedly. Deliberately NOT equal to
	// netMonitorTickInterval: with equal values, an event observed exactly one
	// tick after a recovery lands on the window boundary and
	// leading-vs-coalesced is decided by sub-millisecond races. Measured on the
	// monotonic clock (as time.Now().Sub always was).
	recoverySuppressWindow = time.Second * 6
	// flushTimeout bounds one pool Flush on the worker: a Flush stuck behind a
	// stalled dial must not wedge every later recovery.
	flushTimeout = time.Second * 10
	// refreshTimeout bounds the opened-objects refresh run by the worker.
	refreshTimeout = time.Second * 10
	// closeTimeout bounds how long Close waits for a running recovery; any-sync
	// panics if app.Close takes longer than its stop deadline.
	closeTimeout = time.Second * 5
)

// Recovery triggers, passed explicitly by every admission path; used for logs
// and retained counters.
const (
	triggerTransition          = "transition"
	triggerForegroundWake      = "foregroundWake"
	triggerDuplicateForeground = "duplicateForeground"
	triggerDesktopFallback     = "desktopFallback"
	triggerInterfaceLost       = "interfaceLost"
	triggerInterfaceRegained   = "interfaceRegained"
	triggerNetworkType         = "networkType"
	triggerNetworkPath         = "networkPath"
	triggerOther               = "other"
)

var allTriggers = [...]string{
	triggerTransition, triggerForegroundWake, triggerDuplicateForeground,
	triggerDesktopFallback, triggerInterfaceLost, triggerInterfaceRegained,
	triggerNetworkType, triggerNetworkPath, triggerOther,
}

func triggerIndex(trigger string) int {
	for i, t := range allTriggers {
		if t == trigger {
			return i
		}
	}
	return len(allTriggers) - 1 // other
}

// isWakeTrigger: lifecycle/wake triggers (as opposed to network changes).
func isWakeTrigger(trigger string) bool {
	switch trigger {
	case triggerTransition, triggerForegroundWake, triggerDuplicateForeground, triggerDesktopFallback:
		return true
	}
	return false
}

// recoveryJob is one queued run of the recovery pipeline.
type recoveryJob struct {
	reason  string
	trigger string
	// trailing runs are the coalesced tail of a burst; they execute only if
	// the network changed since the last run or a wake generation is not yet
	// completed (queued generations, including the run's own, count as not
	// handled).
	trailing bool
	// refresh: refresh opened objects after the flush (a Foreground
	// transition enqueued or merged into this leading job)
	refresh bool
}

// triggerRecovery is the common recovery admission path for signals that
// carry no wake semantics of their own (network reports, monitor interface
// events). It observes the gap first, so an interface event that is the
// first thing to run after a wake opens (and covers) the wake generation.
func (n *networkState) triggerRecovery(reason, trigger string) {
	n.recoveryMu.Lock()
	defer n.recoveryMu.Unlock()
	n.observeGapLocked(observeSourceAdmission)
	n.admitLocked(reason, trigger, false)
}

// admitLocked admits a recovery, leading-edge with a suppression window: the
// first signal is queued immediately; signals inside the window coalesce into
// a single trailing run (a second real change right after the first must not
// be lost, but fresh connections must not be flushed over and over during an
// event burst). Never blocks: the pipeline itself runs on the worker. Returns
// true only when a leading job was enqueued (or merged into the queued one),
// i.e. a flush will run after this admission.
func (n *networkState) admitLocked(reason, trigger string, refresh bool) (enqueued bool) {
	if n.closed {
		return false
	}
	w := &n.wake
	mono := n.monoNow()
	since := mono - n.lastRecoveryMono
	if n.hasLastRecovery && since < recoverySuppressWindow {
		n.stats.signalsCoalesced.Inc()
		// remember the latest reason so the trailing run reports what actually
		// coalesced last, not the first suppressed signal
		n.pendingReason, n.pendingTrigger = reason, trigger
		// the pending trailing run covers the current wake generation; it
		// stays uncompleted until that run executes
		w.queued = max(w.queued, w.observed)
		if !n.recoveryPending {
			n.recoveryPending = true
			n.pendingTimer = n.schedule(recoverySuppressWindow-since, n.runPendingRecovery)
		}
		lvl := log.Info
		if w.observed > w.completed {
			lvl = log.Warn // coalescing an uncompleted wake: keep it visible
		}
		lvl("connectivity recovery coalesced", zap.String("reason", reason), zap.String("trigger", trigger),
			zap.Int64("wakeGen", w.observed))
		return false
	}
	n.hasLastRecovery, n.lastRecoveryMono = true, mono
	n.enqueueLocked(recoveryJob{reason: reason, trigger: trigger, refresh: refresh})
	return true
}

func (n *networkState) runPendingRecovery() {
	n.recoveryMu.Lock()
	defer n.recoveryMu.Unlock()
	if n.closed {
		return
	}
	// a timer armed before a sleep fires first after the wake: observe here
	// so it opens (and covers) the wake generation
	n.observeGapLocked(observeSourceTrailing)
	n.recoveryPending = false
	n.hasLastRecovery, n.lastRecoveryMono = true, n.monoNow()
	n.enqueueLocked(recoveryJob{reason: "coalesced: " + n.pendingReason, trigger: n.pendingTrigger, trailing: true})
}

// enqueueLocked hands a job to the serialized worker without blocking. At most
// one job waits: a later admission merges into it (the merged run binds its
// wake generation when it starts, so nothing is lost). A leading job always
// wins over a trailing one in the slot (a trailing run may be skipped, a
// leading one may not). A job admitted while another runs waits and runs
// afterwards.
func (n *networkState) enqueueLocked(job recoveryJob) {
	if n.closed {
		return
	}
	// a recovery records the wake generation it covers when it is queued
	n.wake.queued = max(n.wake.queued, n.wake.observed)
	if q := n.queuedJob; q != nil {
		refresh := q.refresh || job.refresh
		if !job.trailing || q.trailing {
			*q = job
		}
		q.refresh = refresh
	} else {
		n.queuedJob = &job
	}
	n.kickWorker()
}

func (n *networkState) kickWorker() {
	select {
	case n.workerKick <- struct{}{}:
	default:
	}
}

func (n *networkState) runRecoveryWorker(ctx context.Context) {
	defer close(n.workerDone)
	for {
		select {
		case <-ctx.Done():
			return
		case <-n.workerKick:
			for ctx.Err() == nil && n.processQueuedRecovery() {
			}
		}
	}
}

// drainRecoveries runs queued recoveries on the caller's goroutine (tests in
// manualDrive mode).
func (n *networkState) drainRecoveries() {
	for n.processQueuedRecovery() {
	}
}

// processQueuedRecovery executes the queued job, if any. It reports whether a
// job was taken.
func (n *networkState) processQueuedRecovery() bool {
	n.recoveryMu.Lock()
	if n.closed || n.queuedJob == nil {
		n.recoveryMu.Unlock()
		return false
	}
	job := *n.queuedJob
	n.queuedJob = nil
	// bind the flush to the generation seen when execution starts; a freeze
	// that happens later, during the run, stays pending
	n.observeGapLocked(observeSourceRecoveryStart)
	w := &n.wake
	gen := w.observed
	if job.trailing {
		// A trailing run only makes sense when the network actually changed
		// since the last run (e.g. Wi-Fi->cellular right after a wake), or a
		// wake generation is not completed yet — on macOS/Linux a wake within
		// the monotonic suppression window after a pre-sleep recovery is
		// coalesced into exactly this run. Queued work (including this run's
		// own capture) doesn't count as handled.
		if gen <= w.completed && n.fingerprint() == n.lastRecoveredFingerprint {
			// (a trailing job never carries a refresh: a leading job, which may,
			// always wins the queue slot)
			n.stats.trailingSkipped.Inc()
			n.recoveryMu.Unlock()
			log.Info("connectivity recovery skipped: no network change since the last run",
				zap.String("reason", job.reason), zap.Int64("wakeGen", gen))
			return true
		}
		n.stats.trailingRuns.Inc()
	}
	// wake-related runs log at WARN (user builds), others at Info
	wakeRelated := gen > w.completed || isWakeTrigger(job.trigger)
	w.running = max(w.running, gen)
	n.recoveryBusy = true
	n.lastRecoveredFingerprint = n.fingerprint()
	n.recoveryMu.Unlock()

	n.recover(job, gen, wakeRelated)

	// A Foreground transition that arrived while this job ran (after the
	// job's own refresh started, see recover) asked for a refresh; checked
	// under the same lock that clears recoveryBusy so it can't be lost.
	n.recoveryMu.Lock()
	n.recoveryBusy = false
	refresh := n.refreshAfterRunning && !n.closing
	n.refreshAfterRunning = false
	n.recoveryMu.Unlock()
	if refresh {
		n.refreshOpenedObjects()
	}
	return true
}

// refreshOpenedObjects runs the opened-objects refresh bounded by
// refreshTimeout (and Close), on its own goroutine so a refresh stuck in a
// space load can't wedge the worker.
func (n *networkState) refreshOpenedObjects() {
	if n.objectsRefresher == nil {
		return
	}
	ctx, cancel := context.WithTimeout(n.workCtx(), n.refreshTimeout())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		n.objectsRefresher.RefreshOpenedObjects(ctx)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		select {
		case <-done: // finished right at the deadline
		default:
			n.stats.refreshTimeouts.Inc()
			log.Warn("opened objects refresh after recovery abandoned", zap.Error(ctx.Err()))
		}
	}
}

type flushResult int

const (
	flushRan flushResult = iota
	// skipped: an earlier abandoned Flush is still running after another
	// flushTimeout; flushes must not overlap, the run goes on without one
	flushSkippedAbandoned
	// skipped: Close cancelled the run
	flushSkippedClosed
)

// flush runs pool.Flush bounded by flushTimeout and by Close (workCtx). Flush
// runs on its own goroutine so even a Flush that ignores its context can't
// wedge the worker. An abandoned Flush is remembered: the next flush first
// waits for it, at most another flushTimeout, so two flushes never overlap; if
// it is still running then, this run skips its Flush rather than stall every
// later recovery.
func (n *networkState) flush(gen int64) flushResult {
	base := n.workCtx()
	n.recoveryMu.Lock()
	abandoned := n.abandonedFlush
	n.recoveryMu.Unlock()
	if abandoned != nil {
		log.Warn("flush: waiting for a previously abandoned flush", zap.Int64("wakeGen", gen))
		timer := time.NewTimer(n.flushTimeout())
		select {
		case <-abandoned:
			n.recoveryMu.Lock()
			n.abandonedFlush = nil
			n.recoveryMu.Unlock()
		case <-base.Done():
		case <-timer.C:
			timer.Stop()
			n.stats.flushSkippedAbandoned.Inc()
			log.Warn("flush skipped: a previously abandoned flush is still running", zap.Int64("wakeGen", gen))
			return flushSkippedAbandoned
		}
		timer.Stop()
	}
	if base.Err() != nil {
		n.stats.flushSkippedClosed.Inc()
		return flushSkippedClosed
	}
	start := n.timeNow()
	ctx, cancel := context.WithTimeout(base, n.flushTimeout())
	defer cancel()
	res := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		res <- n.pool.Flush(ctx)
	}()
	var err error
	select {
	case err = <-res:
	case <-ctx.Done():
		n.recoveryMu.Lock()
		n.abandonedFlush = finished
		n.recoveryMu.Unlock()
		if base.Err() == nil {
			n.stats.flushTimeouts.Inc()
		}
		err = fmt.Errorf("flush abandoned: %w", ctx.Err())
	}
	n.stats.lastFlushDurationMs.Store(n.timeNow().Sub(start).Milliseconds())
	if err != nil && !errors.Is(err, context.Canceled) {
		// a failed or timed-out Flush still completes the generation: with
		// pool generations (any-sync C.0) Flush invalidates every pre-flush
		// peer synchronously, and retrying would only churn
		n.stats.flushErrors.Inc()
		n.stats.lastFlushError.Store(err.Error())
		log.Warn("flush pool on connectivity recovery failed or timed out", zap.Error(err), zap.Int64("wakeGen", gen))
	}
	return flushRan
}

// recover runs the recovery pipeline for one job bound to wake generation gen.
func (n *networkState) recover(job recoveryJob, gen int64, wakeRelated bool) {
	logf := log.Info
	if wakeRelated {
		// WARN on purpose: once per wake, the field evidence for GO-7556 in
		// user builds (which log most loggers at WARN)
		logf = log.Warn
	}
	n.recoveryMu.Lock()
	closing := n.closing
	n.recoveryMu.Unlock()
	online := !n.IsOffline()
	var flushDur time.Duration
	result := flushRan
	errsBefore, timeoutsBefore := n.stats.flushErrors.Load(), n.stats.flushTimeouts.Load()
	if !closing {
		n.stats.recoveries.Inc()
		n.stats.executedByTrigger[triggerIndex(job.trigger)].Inc()
		if !online {
			n.stats.recoveriesOffline.Inc()
		}
		n.stats.lastRecoveryUnix.Store(n.timeNow().Unix())
		n.stats.lastRecoveryReason.Store(job.reason)
		n.stats.lastRecoveryTrigger.Store(job.trigger)
		n.stats.lastRecoveryWakeGen.Store(gen)
		logf("connectivity recovery start", zap.String("reason", job.reason), zap.String("trigger", job.trigger),
			zap.Int64("wakeGen", gen), zap.Bool("online", online))
		// Flush drops every pooled connection (closing the underlying
		// sockets), so the next Get re-dials instead of serving a connection
		// that died with the old network path. Flushing while offline is
		// still right: it kills dead sockets that would otherwise block
		// writers for the transport-timeout window.
		if n.pool != nil {
			start := n.timeNow()
			result = n.flush(gen)
			flushDur = n.timeNow().Sub(start)
			if result == flushSkippedClosed {
				closing = true
			}
		}
	}
	// the generation counts as completed once Flush has returned, timed out
	// or was skipped (or, while closing, without a flush: nothing will run
	// any more)
	n.recoveryMu.Lock()
	w := &n.wake
	w.completed = max(w.completed, gen)
	w.completedWall, w.completedMono = n.wallNow(), n.monoNow()
	closing = closing || n.closing
	n.recoveryMu.Unlock()

	if closing {
		// app shutdown in progress: the pool, hook owners, the space syncer
		// and the objects refresher may already be closing
		log.Info("connectivity recovery: closing, skipping flush/hooks/refresh", zap.Int64("wakeGen", gen))
		return
	}
	n.runConnectivityHooks(online)
	if online && n.spaceSyncer != nil {
		n.spaceSyncer.SyncAllSpaceHeads()
	}
	if job.refresh {
		// this refresh also serves any Foreground that arrived during the run
		// so far; one arriving after this point refreshes again afterwards
		n.recoveryMu.Lock()
		n.refreshAfterRunning = false
		n.recoveryMu.Unlock()
		n.refreshOpenedObjects()
	}
	logf("connectivity recovery finished", zap.String("trigger", job.trigger), zap.Int64("wakeGen", gen),
		zap.Duration("flushDuration", flushDur),
		zap.Bool("flushSkipped", result != flushRan),
		zap.Bool("flushFailed", n.stats.flushErrors.Load() > errsBefore),
		zap.Bool("flushTimedOut", n.stats.flushTimeouts.Load() > timeoutsBefore),
		zap.Bool("refreshed", job.refresh))
}
