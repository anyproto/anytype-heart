package payments

import (
	"context"
	"sync"
	"time"

	"go.uber.org/atomic"
	"go.uber.org/zap"
)

// Manual refresh (the Refresh button, MembershipV2GetStatus.forceRefreshSec)
// limits. They are enforced here, across renderers.
var (
	// manualForceMaxWindow caps a manual window, measured from its original
	// admission: extensions never slide the cap
	manualForceMaxWindow = 3 * time.Minute
	// manualForceMinGap is the minimum time between two manual admissions;
	// requests within it only fetch
	manualForceMinGap = 30 * time.Second
)

// forceOrigin is who asked for a forced poll. Each origin keeps its own
// deadline, so a manual request never shortens a purchase window and vice
// versa.
type forceOrigin int

const (
	forceOriginPurchase forceOrigin = iota
	forceOriginManual
	forceOriginCount
)

func (o forceOrigin) String() string {
	switch o {
	case forceOriginPurchase:
		return "purchase"
	case forceOriginManual:
		return "manual"
	default:
		return "unknown"
	}
}

// clockReading is a sleep-aware point in time: the wall clock (without the
// monotonic reading) together with the process monotonic clock. Monotonic
// time pauses during sleep on macOS, iOS, Linux and Android, and the wall
// clock can be stepped backwards, so a deadline counts as reached as soon as
// either clock says so.
type clockReading struct {
	wall time.Time
	mono time.Duration
}

var processStart = time.Now()

func systemClock() clockReading {
	now := time.Now()
	return clockReading{wall: now.Round(0), mono: now.Sub(processStart)}
}

func (c clockReading) add(d time.Duration) clockReading {
	return clockReading{wall: c.wall.Add(d), mono: c.mono + d}
}

// reached reports whether deadline d has passed by either clock
func (c clockReading) reached(d clockReading) bool {
	return !c.wall.Before(d.wall) || c.mono >= d.mono
}

// elapsedSince is the larger of the wall and monotonic time since earlier
func (c clockReading) elapsedSince(earlier clockReading) time.Duration {
	return max(c.wall.Sub(earlier.wall), c.mono-earlier.mono)
}

// later returns the element-wise maximum, so merging deadlines never shortens
// either clock's view of a window
func (c clockReading) later(o clockReading) clockReading {
	if o.wall.After(c.wall) {
		c.wall = o.wall
	}
	if o.mono > c.mono {
		c.mono = o.mono
	}
	return c
}

// earlier returns the element-wise minimum
func (c clockReading) earlier(o clockReading) clockReading {
	if o.wall.Before(c.wall) {
		c.wall = o.wall
	}
	if o.mono < c.mono {
		c.mono = o.mono
	}
	return c
}

// forceIntent is one origin's forced-poll window. The deadline is absolute
// and fixed at admission.
type forceIntent struct {
	active   bool
	deadline clockReading
	// manual only: the original admission + manualForceMaxWindow
	limit clockReading
	// admission sequence of the latest admission or extension: a fetch that
	// started before it can't clear this intent
	seq uint64
}

type forceStats struct {
	admissions [forceOriginCount]atomic.Int64
	// admissions that extended an already active window of the same origin
	extended [forceOriginCount]atomic.Int64
	// manual requests within manualForceMinGap of the previous admission
	manualRejected  atomic.Int64
	forcedFetches   atomic.Int64
	expired         [forceOriginCount]atomic.Int64
	stoppedOnChange atomic.Int64
}

// refreshController runs the periodic membership poll and the forced polls.
// Admission (Force, AdmitManual) never blocks on the loop: it stores the
// intent under a mutex and wakes the loop through a capacity-1 channel.
type refreshController struct {
	ctx           context.Context
	cancel        context.CancelFunc
	fetch         func(ctx context.Context, forceFetch bool) (bool, error)
	interval      time.Duration
	forceInterval time.Duration
	closeCh       chan struct{}
	wakeCh        chan struct{}
	now           func() clockReading

	// spaceForcedPolls (V2): forced polls are at least forceInterval apart,
	// including the first one after an admission, and an extension doesn't
	// reset the next poll. V1 keeps polling immediately on every Force.
	spaceForcedPolls bool

	mu                  sync.Mutex
	intents             [forceOriginCount]forceIntent
	admitSeq            uint64
	pokeNow             bool
	lastFetch           clockReading
	hasLastFetch        bool
	lastForcedFetch     clockReading
	hasLastForcedFetch  bool
	lastManualAdmission clockReading
	hasManualAdmission  bool
	// failing: the previous fetch failed (log transitions at WARN only)
	failing bool

	stats forceStats
}

func newRefreshController(parent context.Context, fetch func(ctx context.Context, forceFetch bool) (bool, error), interval, forceInterval time.Duration) *refreshController {
	ctx, cancel := context.WithCancel(parent)
	return &refreshController{
		ctx:           ctx,
		cancel:        cancel,
		fetch:         fetch,
		interval:      interval,
		forceInterval: forceInterval,
		closeCh:       make(chan struct{}),
		wakeCh:        make(chan struct{}, 1),
		now:           systemClock,
	}
}

func (rc *refreshController) Start() {
	if rc == nil {
		return
	}
	go rc.loop()
}

func (rc *refreshController) Stop() {
	if rc == nil {
		return
	}
	rc.cancel()
	<-rc.closeCh
}

// Force opens or extends a purchase forced-poll window. It never blocks.
func (rc *refreshController) Force(duration time.Duration) {
	if rc.fetch == nil {
		return
	}
	if duration <= 0 {
		duration = rc.interval
	}
	rc.mu.Lock()
	now := rc.now()
	deadline := now.add(duration)
	in := &rc.intents[forceOriginPurchase]
	if in.active && !now.reached(in.deadline) {
		// pending intents merge by keeping the longest purchase deadline
		in.deadline = in.deadline.later(deadline)
		rc.stats.extended[forceOriginPurchase].Inc()
	} else {
		in.active = true
		in.deadline = deadline
	}
	rc.admitSeq++
	in.seq = rc.admitSeq
	if !rc.spaceForcedPolls {
		rc.pokeNow = true
	}
	rc.stats.admissions[forceOriginPurchase].Inc()
	rc.mu.Unlock()
	rc.wake()
}

// AdmitManual opens or extends the manual forced-poll window after an
// explicit fetch that just completed; that fetch counts as the first forced
// attempt. It returns false when the request is rate limited (it then only
// fetched). It never blocks.
func (rc *refreshController) AdmitManual(window time.Duration) bool {
	if rc.fetch == nil || window <= 0 {
		return false
	}
	window = min(window, manualForceMaxWindow)
	rc.mu.Lock()
	now := rc.now()
	if rc.hasManualAdmission && now.elapsedSince(rc.lastManualAdmission) < manualForceMinGap {
		rc.mu.Unlock()
		rc.stats.manualRejected.Inc()
		return false
	}
	rc.lastManualAdmission = now
	rc.hasManualAdmission = true

	in := &rc.intents[forceOriginManual]
	if in.active && !now.reached(in.deadline) {
		in.deadline = in.deadline.later(now.add(window)).earlier(in.limit)
		rc.stats.extended[forceOriginManual].Inc()
	} else {
		in.active = true
		in.limit = now.add(manualForceMaxWindow)
		in.deadline = now.add(window)
	}
	rc.admitSeq++
	in.seq = rc.admitSeq
	// the explicit fetch is the first forced attempt: no immediate duplicate
	rc.lastForcedFetch = now
	rc.hasLastForcedFetch = true
	rc.stats.admissions[forceOriginManual].Inc()
	rc.mu.Unlock()
	rc.wake()
	return true
}

func (rc *refreshController) wake() {
	select {
	case rc.wakeCh <- struct{}{}:
	default:
	}
}

func (rc *refreshController) loop() {
	defer close(rc.closeCh)
	if rc.fetch == nil {
		return
	}

	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()

	for {
		// a 0-wait timer may be ready together with ctx.Done: never run a
		// fetch after Stop
		if rc.ctx.Err() != nil {
			return
		}
		if wait, ok := rc.nextWait(); ok {
			resetTimer(timer, wait)
		} else {
			timer.Stop()
		}
		select {
		case <-rc.ctx.Done():
			return
		case <-rc.wakeCh:
		case <-timer.C:
			rc.runFetch()
		}
	}
}

// dropExpiredLocked retires intents whose deadline passed by either clock.
// Unlike the old V1 loop (deadline checked after the fetch), an intent can
// expire before its first forced fetch; only windows shorter than the poll
// gap are affected, and every Force caller uses 30m.
func (rc *refreshController) dropExpiredLocked(now clockReading) {
	for o := range rc.intents {
		in := &rc.intents[o]
		if in.active && now.reached(in.deadline) {
			// how far the wall clock ran ahead of the monotonic one (sleep on
			// darwin/linux/mobile, or a clock step) since admission
			origin := zap.Stringer("origin", forceOrigin(o))
			wallLeft := zap.Duration("wallLeft", in.deadline.wall.Sub(now.wall))
			monoLeft := zap.Duration("monoLeft", in.deadline.mono-now.mono)
			*in = forceIntent{}
			rc.stats.expired[o].Inc()
			if forceOrigin(o) == forceOriginPurchase {
				log.Warn("membership refresh: forced refresh timed out before change", origin, wallLeft, monoLeft)
			} else {
				log.Info("membership refresh: forced refresh window ended", origin, wallLeft, monoLeft)
			}
		}
	}
}

func (rc *refreshController) forceActiveLocked() bool {
	for _, in := range rc.intents {
		if in.active {
			return true
		}
	}
	return false
}

// nextWait returns how long until the next fetch; ok=false means no fetch is
// scheduled (no periodic interval and no forced window)
func (rc *refreshController) nextWait() (wait time.Duration, ok bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	now := rc.now()
	rc.dropExpiredLocked(now)
	if rc.pokeNow {
		return 0, true
	}
	if rc.forceActiveLocked() {
		if !rc.hasLastForcedFetch {
			return 0, true
		}
		return max(rc.forceInterval-now.elapsedSince(rc.lastForcedFetch), 0), true
	}
	if rc.interval <= 0 {
		return 0, false
	}
	if !rc.hasLastFetch {
		return 0, true
	}
	return max(rc.interval-now.elapsedSince(rc.lastFetch), 0), true
}

func (rc *refreshController) runFetch() {
	rc.mu.Lock()
	// expiry is checked before each forced fetch, including after sleep
	rc.dropExpiredLocked(rc.now())
	force := rc.forceActiveLocked()
	startSeq := rc.admitSeq
	rc.pokeNow = false
	rc.mu.Unlock()
	if force {
		rc.stats.forcedFetches.Inc()
	}

	changed, err := rc.fetch(rc.ctx, force)

	rc.mu.Lock()
	defer rc.mu.Unlock()
	switch {
	case err != nil && rc.ctx.Err() != nil:
		log.Debug("membership refresh: fetch stopped", zap.Error(err))
	case err != nil && (!rc.failing || !IsTransientError(err)):
		log.Warn("membership refresh: fetch failed", zap.Error(err), zap.Bool("force", force))
	case err != nil:
		log.Debug("membership refresh: fetch still failing", zap.Error(err), zap.Bool("force", force))
	case rc.failing:
		log.Info("membership refresh: fetch recovered", zap.Bool("force", force))
	}
	rc.failing = err != nil
	now := rc.now()
	rc.lastFetch = now
	rc.hasLastFetch = true
	if force {
		rc.lastForcedFetch = now
		rc.hasLastForcedFetch = true
		if changed {
			// P0 stop rule: the controller's own changed=true. An intent
			// admitted or extended while this fetch ran is newer than it and
			// stays.
			for o := range rc.intents {
				in := &rc.intents[o]
				if in.active && in.seq <= startSeq {
					*in = forceIntent{}
					rc.stats.stoppedOnChange.Inc()
				}
			}
		}
	}
}

func resetTimer(timer *time.Timer, interval time.Duration) {
	if interval < 0 {
		interval = 0
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(interval)
}
