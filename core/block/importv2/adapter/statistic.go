package adapter

import (
	"fmt"
	"sync"
	"time"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The §15 PUSH producer: one emitter per engine run, fed by the redesigned
// reporter seam (engine.Reporter), by the Notion client's rate-limit hooks
// (notionclient.StatusHook) and by the run's issue funnel. It owns the run's
// live statistic and coalesces it onto the event stream.
//
// Three rules from the spec are structural here rather than incidental:
//
//   - ONE event per window (§15.3): per-item emission is fine at pass 2's
//     ~1.5 items/s and floods at pass 3's 50-200/s.
//   - EXCEPT for a phase change and every state transition, which emit at
//     once. Rate limiting is normal operation, and the whole value of the
//     calm THROTTLED badge is that it appears the moment the crawl pauses;
//     a quarter second of "is it stuck?" is exactly what it exists to
//     prevent.
//   - Push and pull are the SAME builder over the SAME state (§15.5).
//     Snapshot is what the RPCs serve for a live run, so no field can mean
//     one thing pushed and another polled.
//
// Everything here is advisory: a slow or failing emitter must never affect
// a run.

const (
	// statWindow is §15.3's coalescing period.
	statWindow = 250 * time.Millisecond
	// rateWindowSpan bounds the rolling rate sample: long enough to survive
	// one throttle pause, short enough to track a real slowdown.
	rateWindowSpan = 30 * time.Second
	// rateWindowMin is the shortest span that may be called a rate. Below
	// it a single burst reads as a speed the import cannot sustain, and the
	// ETA derived from it is a guess — §15.3 allows only defensible ones.
	rateWindowMin = time.Second
	// rateSampleCap bounds the sample ring.
	rateSampleCap = 64
	// rateSampleInterval is the minimum gap between RING ENTRIES. Without
	// it, pass 3's 50-200 objects/s fills the ring in under a second, the
	// retained span drops below rateWindowMin, and the rate — with the ETA
	// behind it — reads permanently zero in exactly the phase where it is
	// cheapest to compute. Ticks inside the interval update the newest entry
	// instead of adding one, so the ring always covers at least
	// cap*interval of history.
	rateSampleInterval = 250 * time.Millisecond
)

type statConfig struct {
	importId   string
	processId  string
	importType model.ImportType
	// send delivers one built event; nil makes the emitter inert (volatile
	// runs and fixtures without an event sender).
	send   func(*pb.EventImportStatistic)
	window time.Duration
	now    func() time.Time
	// pageRateCeiling is the fastest the SOURCE can yield pages, in
	// pages/s; 0 means no known ceiling. The fetching ETA may never promise
	// faster (§15.3: ~2 requests per page against Notion's documented 3 rps
	// is a known bound, so the estimate is computable rather than guessed).
	pageRateCeiling float64
	// safeToClose{Fetching,Materializing} are the sweep's OWN resume
	// predicates evaluated once for this run — crawlResumable+cap for pass
	// 2, resumable+cap for pass 3. Evaluated once because they change only
	// at lifecycle transitions, and taken from the same functions the sweep
	// consults so the surface and the sweep's behavior cannot drift.
	safeToCloseFetching      bool
	safeToCloseMaterializing bool
}

// statSnapshot is the run's live statistic. Every field is a level.
type statSnapshot struct {
	phase          importv2.Phase
	phaseStartedAt time.Time

	pagesTotal, pagesDone int64
	filesTotal, filesDone int64
	// bytesDone is a RUN level, not a phase one: the bytes are on disk in
	// the spill dir and stay there across the pass boundary — and across
	// incarnations — which is exactly how the dormant surface reads them
	// (importv2.SpillBytes).
	bytesDone int64

	state        pb.EventImportStatisticState
	resumesIn    time.Duration
	attempt      int32
	attemptsMax  int32
	errorMessage string

	objectsCreated           int64
	warningCount, errorCount int64
	currentItem              importv2.DisplayText
}

type rateSample struct {
	at    time.Time
	pages int64
	items int64
}

type statEmitter struct {
	cfg statConfig

	mu      sync.Mutex
	closed  bool
	snap    statSnapshot
	epoch   int
	samples []rateSample
	pending bool
	nextAt  time.Time
	timer   *time.Timer
}

func newStatEmitter(cfg statConfig) *statEmitter {
	if cfg.window <= 0 {
		cfg.window = statWindow
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}
	if cfg.send == nil {
		cfg.send = func(*pb.EventImportStatistic) {}
	}
	e := &statEmitter{cfg: cfg}
	e.snap.phaseStartedAt = cfg.now()
	return e
}

// counterEpoch groups phases into the two counting regimes. SCANNING,
// ANALYZING and FETCHING share one — claims are the denominator, spool rows
// the numerator — and CREATING and FINALIZING share the other, where the
// spool census is the denominator and materialized rows the numerator.
// Grouping rather than resetting per phase is what lets the converter flip
// ANALYZING on and off mid-crawl without erasing the crawl's progress.
func counterEpoch(p importv2.Phase) int {
	if p >= importv2.PhaseCreating {
		return 1
	}
	return 0
}

// --- engine.Reporter -------------------------------------------------------

func (e *statEmitter) Phase(p importv2.Phase) {
	e.mu.Lock()
	defer e.mu.Unlock()
	epoch := counterEpoch(p)
	if epoch != e.epoch {
		// A new counting regime: the old pass's numbers describe work of a
		// different kind at a different speed. Carrying the rate window
		// across would let pass 2's ~1.5 items/s price a pass that runs at
		// persist speed — "about an hour left" for a minute of work.
		e.epoch = epoch
		e.snap.pagesTotal, e.snap.pagesDone = 0, 0
		e.snap.filesTotal, e.snap.filesDone = 0, 0
		e.samples = nil
	}
	if p != e.snap.phase {
		e.snap.phase = p
		e.snap.phaseStartedAt = e.cfg.now()
		// A stale item under the next phase's label ("Creating: Q3
		// Planning") is worse than no subtitle at all.
		e.snap.currentItem = ""
	}
	e.mark(true)
}

func (e *statEmitter) Discovered(kind importv2.Kind, delta int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if kind == importv2.KindFile {
		e.snap.filesTotal += delta
	} else {
		e.snap.pagesTotal += delta
	}
	e.mark(false)
}

func (e *statEmitter) Completed(kind importv2.Kind, delta int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if kind == importv2.KindFile {
		e.snap.filesDone += delta
	} else {
		e.snap.pagesDone += delta
	}
	e.keepDenominatorsHonestLocked()
	e.sampleLocked()
	e.mark(false)
}

// keepDenominatorsHonestLocked restores the one arithmetic invariant a
// progress surface has: a denominator that EXISTS is at least its own
// numerator.
//
// It is not decoration. A crawl-resumed run fills the two counters from
// different sets: pass 1 discovers only what /search re-enumerated, while
// pass 2 seeds done from the whole spool census — which holds rows for
// entities a previous incarnation found through a parent's block tree, and
// /search never returns those (review item 9). The live surface then read
// 2/1 for a dir whose dormant poll reads 2/2, and the ETA answered "unknown"
// for the rest of the crawl, because a negative remainder is not a
// remainder. Raising the total to what is provably done agrees with the
// dormant read exactly, and the crawl's further discoveries move it on from
// there.
//
// A ZERO total is left alone: zero means UNKNOWN in this schema, which is
// what filesTotal deliberately is for the whole crawl (files are found BY
// crawling). Turning it into "exactly what is done" would paint a finished
// file bar over a crawl that has not looked for its files yet.
func (e *statEmitter) keepDenominatorsHonestLocked() {
	if e.snap.pagesTotal > 0 && e.snap.pagesDone > e.snap.pagesTotal {
		e.snap.pagesTotal = e.snap.pagesDone
	}
	if e.snap.filesTotal > 0 && e.snap.filesDone > e.snap.filesTotal {
		e.snap.filesTotal = e.snap.filesDone
	}
}

func (e *statEmitter) Bytes(total int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.snap.bytesDone = total
	e.mark(false)
}

// Created takes a LEVEL, and a level published from racing producers can
// arrive out of order: the engine's persist workers each publish
// created.Add(1) with nothing ordering the increment against the publish, so
// the LOWER level lands last (review item 7 — measured regressing on every
// run of a 600-page import, one settling at 598/600). The engine now
// publishes a high-water mark of its own; this is the surface's own guard,
// because objectsCreated is §15.4's cancel affordance ("stop and remove the
// N objects created") and the dormant poll of the same run serves the exact
// ledger count. A level that only rises is also the only reading that can
// agree with a durable counter.
func (e *statEmitter) Created(count int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if count <= e.snap.objectsCreated {
		return
	}
	e.snap.objectsCreated = count
	e.mark(false)
}

func (e *statEmitter) Item(item importv2.DisplayText) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.snap.currentItem = item
	e.mark(false)
}

// --- notionclient.StatusHook ----------------------------------------------

// Throttled is the CALM state: the shared pacer met a 429/529 pushback and
// the window reopens in resumeIn. It is expected — a large import spends
// much of its life here — so it is a state, never an error.
func (e *statEmitter) Throttled(resumeIn time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.setStateLocked(pb.EventImportStatistic_Throttled, func() {
		e.snap.resumesIn = resumeIn
	})
}

func (e *statEmitter) Retrying(attempt, attemptsMax int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.setStateLocked(pb.EventImportStatistic_Retrying, func() {
		e.snap.attempt, e.snap.attemptsMax = int32(attempt), int32(attemptsMax)
		e.snap.resumesIn = 0
	})
}

func (e *statEmitter) Recovered() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.setStateLocked(pb.EventImportStatistic_Running, func() {
		e.snap.resumesIn, e.snap.attempt, e.snap.attemptsMax = 0, 0, 0
	})
}

// setStateLocked applies a state EDGE. A repeat of the state already shown
// is not an edge and must not jump the coalescing window — every worker's
// request meets the same pushback, so the hooks fire in bursts.
//
// ERROR is TERMINAL here, and that rule lives at this ONE setter rather than
// at its callers (review item 5: the guard sat on Recovered while Throttled
// and Retrying set the state unconditionally — two of three). All three
// speak for the TRANSPORT: a pacer window, a backoff attempt, a request that
// finally worked. None of them knows anything about the fatal that is
// stopping the run, and they fire from prefetch workers that outlive the
// run's own cancel, so a failed run's TERMINAL event could sign off reading
// "waiting for Notion". Only a run can leave the error state, and it does so
// by ending.
func (e *statEmitter) setStateLocked(state pb.EventImportStatisticState, apply func()) {
	if e.snap.state == pb.EventImportStatistic_Error && state != pb.EventImportStatistic_Error {
		return
	}
	changed := e.snap.state != state
	e.snap.state = state
	apply()
	e.mark(changed)
}

// --- issue funnel ----------------------------------------------------------

// Issue folds one reported issue into the live counts — §15.2's reason for
// having them at all: a run pouring out warnings can be abandoned at minute
// 20 instead of minute 110.
func (e *statEmitter) Issue(issue importv2.Issue) {
	e.mu.Lock()
	defer e.mu.Unlock()
	countIssue(issue.Severity, &e.snap.warningCount, &e.snap.errorCount)
	if issue.Severity >= importv2.SeverityFatal && issue.Code != importv2.IssueCancelled {
		// ERROR means something is actually WRONG. A cancel — the user's or
		// a shutdown suspend, both of which arrive as a cancelled fatal — is
		// neither wrong nor alarming, and painting the UI red on the way out
		// of a deliberate stop is the same category error as painting it red
		// for a rate limit.
		e.snap.errorMessage = issueMessage(issue)
		e.setStateLocked(pb.EventImportStatistic_Error, func() {})
		return
	}
	e.mark(false)
}

// statSeed is everything a RESUMED run's live surface already knows before
// its engine has started — §15.4's right-hand column, the same reads the
// dormant poll of this dir performs.
//
// It exists because the emitter is built (and registered live) in
// newLifecycle, while the engine only starts after the ledger load, the
// identity rehydration and the spool open. Everything the surface says in
// that window it says out of its ZERO VALUE unless it is seeded — and the
// zero value is phase SCANNING with cancelEffect NOTHING_TO_UNDO, which
// §15.6 renders as "Cancel (nothing added yet)" for a run whose cancel is
// about to compensate thousands of real objects (review item 4).
type statSeed struct {
	// materializing is the manifest's sticky marker, derived at the ONE
	// lifecycle construction site — the same switch cancelEffectOf and
	// phaseOf read for a dormant run, so push and pull cannot disagree
	// about which side of the pass boundary this run is on.
	materializing bool
	// issues are previous incarnations' retained issues. A resumed run's
	// surface must not report FEWER problems than the same dir reports when
	// polled dormant: the ledger holds every incarnation's, and the engine
	// deliberately re-seeds them without re-reporting (no OnIssue, no abort
	// predicate — they aborted or did not in their own incarnation), so the
	// counts have to arrive here by another door. Fatal records never reach
	// this: resume.rehydrateIssues drops them on load, because a fatal that
	// coexists with a resumable dir IS the abort that made it dormant.
	issues []importv2.Issue
	// created is the ledger's object count — §15.4's "a restart resumes the
	// NUMBERS, not just the work". The engine publishes it again the moment
	// it starts; this is the same number, one rehydration earlier.
	created int64
	// The pass-3 counters, from the spool census and the resume state. Left
	// zero for a fresh run and a crawl resume, whose denominators are still
	// being discovered.
	pagesTotal, pagesDone int64
	filesTotal, filesDone int64
}

// Seed applies a resumed run's starting state. Called once, at construction,
// before the emitter is handed to anything.
func (e *statEmitter) Seed(seed statSeed) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if seed.materializing {
		e.snap.phase = importv2.PhaseCreating
		e.epoch = counterEpoch(importv2.PhaseCreating)
	}
	e.snap.objectsCreated = seed.created
	e.snap.pagesTotal, e.snap.pagesDone = seed.pagesTotal, seed.pagesDone
	e.snap.filesTotal, e.snap.filesDone = seed.filesTotal, seed.filesDone
	e.keepDenominatorsHonestLocked()
	for _, issue := range seed.issues {
		countIssue(issue.Severity, &e.snap.warningCount, &e.snap.errorCount)
	}
	e.mark(false)
}

// issueMessage renders a fatal for the wire's errorMessage. It may carry a
// source key (a Notion id, a markdown path) — displayable, like currentItem,
// and for the same reason never fed to a log from here.
func issueMessage(issue importv2.Issue) string {
	if issue.Message != "" {
		return issue.Message
	}
	if issue.Err != nil {
		return issue.Err.Error()
	}
	return string(issue.Code)
}

// --- emission --------------------------------------------------------------

// mark publishes or schedules. The caller holds mu, and the send happens
// UNDER it: two goroutines building concurrently and sending afterwards
// could deliver an older statistic after a newer one, and a progress
// surface that goes backwards is worse than one that is 250 ms late. That
// is safe because the sender is non-blocking by contract — Broadcast
// enqueues onto each session's bounded queue and closes a client that
// overflows it (core/event/event_grpc.go sendEvent) — so no client can hold
// this lock, and with it a persist worker, for its own reasons.
func (e *statEmitter) mark(immediate bool) {
	if e.closed {
		return
	}
	now := e.cfg.now()
	if immediate || !now.Before(e.nextAt) {
		e.publishLocked(now)
		return
	}
	e.pending = true
	if e.timer == nil {
		// The trailing edge: whatever the window swallowed must still
		// arrive, or a crawl that stalls right after a burst leaves its last
		// number unsent forever.
		e.timer = time.AfterFunc(e.nextAt.Sub(now), e.onTimer)
	}
}

func (e *statEmitter) publishLocked(now time.Time) {
	e.pending = false
	e.nextAt = now.Add(e.cfg.window)
	e.cfg.send(e.buildLocked())
}

func (e *statEmitter) onTimer() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.timer = nil
	if e.closed || !e.pending {
		return
	}
	e.publishLocked(e.cfg.now())
}

// Close flushes the terminal numbers and silences the emitter. Called from
// the run lifecycle's single settlement point, so a run cannot end with its
// last state stuck behind a coalescing window.
func (e *statEmitter) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	if e.timer != nil {
		e.timer.Stop()
		e.timer = nil
	}
	e.cfg.send(e.buildLocked())
	e.closed = true
}

// Snapshot is the PULL half: what ObjectImportRunStatus serves for a live
// run. Same builder, same state, so §15.5's "push and pull must agree" is a
// property of the code rather than a promise in a comment.
func (e *statEmitter) Snapshot() *pb.EventImportStatistic {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.buildLocked()
}

func (e *statEmitter) buildLocked() *pb.EventImportStatistic {
	pageRate, itemRate := e.ratesLocked()
	return &pb.EventImportStatistic{
		ImportId:       e.cfg.importId,
		ProcessId:      e.cfg.processId,
		ImportType:     e.cfg.importType,
		Phase:          wirePhase(e.snap.phase),
		PhaseStartedAt: e.snap.phaseStartedAt.UnixMilli(),
		// Totals are indeterminate exactly while pass 1 is running: the
		// cursor chain does not know its own length. They become known at
		// the pass-1/pass-2 boundary and stay known (§15.3).
		TotalsKnown:          e.snap.phase != importv2.PhaseScanning,
		PagesTotal:           e.snap.pagesTotal,
		PagesDone:            e.snap.pagesDone,
		FilesTotal:           e.snap.filesTotal,
		FilesDone:            e.snap.filesDone,
		BytesDone:            e.snap.bytesDone,
		State:                e.snap.state,
		ResumesInMs:          e.snap.resumesIn.Milliseconds(),
		Attempt:              e.snap.attempt,
		AttemptsMax:          e.snap.attemptsMax,
		ErrorMessage:         e.snap.errorMessage,
		ItemsPerSecond:       itemRate,
		EstimatedRemainingMs: e.etaLocked(pageRate, itemRate),
		CancelEffect:         wireCancelEffect(e.snap.phase),
		ObjectsCreated:       e.snap.objectsCreated,
		SafeToClose:          e.safeToCloseLocked(),
		WarningCount:         e.snap.warningCount,
		ErrorCount:           e.snap.errorCount,
		CurrentItem:          e.snap.currentItem.Display(),
	}
}

func (e *statEmitter) safeToCloseLocked() bool {
	if e.epoch == 1 {
		return e.cfg.safeToCloseMaterializing
	}
	return e.cfg.safeToCloseFetching
}

// sampleLocked records one point of the rolling rate window.
func (e *statEmitter) sampleLocked() {
	now := e.cfg.now()
	sample := rateSample{
		at:    now,
		pages: e.snap.pagesDone,
		items: e.snap.pagesDone + e.snap.filesDone,
	}
	if n := len(e.samples); n > 0 && now.Sub(e.samples[n-1].at) < rateSampleInterval {
		// The tick lands inside the newest entry's interval: drop it. The
		// entry is NOT moved forward — doing that would keep the ring at one
		// element (the newest is also the oldest) or, with two, stretch the
		// "window" over the whole phase and turn a recent-window rate into a
		// lifetime average that no slowdown could move. The cost is that the
		// newest entry's count is up to one interval stale, which biases the
		// rate slightly LOW over a multi-second window: the safe direction
		// for an ETA.
		return
	}
	e.samples = append(e.samples, sample)
	e.pruneSamplesLocked(now)
	if over := len(e.samples) - rateSampleCap; over > 0 {
		e.samples = append(e.samples[:0], e.samples[over:]...)
	}
}

// pruneSamplesLocked drops entries that have fallen out of the rolling span.
// It is called from the READ path too, because time passes whether or not
// anything is completing (review item 6): pruning only on a completion meant
// a run that stopped completing kept whatever samples it had, forever.
// One entry always survives — it is the anchor a later completion measures
// against.
func (e *statEmitter) pruneSamplesLocked(now time.Time) {
	cutoff := now.Add(-rateWindowSpan)
	drop := 0
	for drop < len(e.samples)-1 && e.samples[drop].at.Before(cutoff) {
		drop++
	}
	if drop > 0 {
		e.samples = append(e.samples[:0], e.samples[drop:]...)
	}
}

// ratesLocked reports the observed pages/s and items/s over the window, or
// zero when the window is too short to defend a number.
//
// The window ends at NOW, not at the newest sample. A rate measured between
// two samples answers "how fast was it going while it was going", which is
// not the question: a stalled run kept reporting its last healthy rate and,
// with it, a frozen ETA that never moved — the throttled-vs-stuck
// distinction §15.1 exists to draw, inverted. Measuring to now makes the
// rate decay while nothing completes, and once the whole span has passed
// with no completion the pruning leaves one anchor, no rate, and an ETA of
// unknown, which is the honest answer.
//
// The cost is the documented one: the newest entry's counts are up to
// rateSampleInterval stale (sampleLocked deliberately does not move it
// forward), so the rate reads slightly LOW — the safe direction for an ETA,
// and negligible over any window long enough to be quoted at all.
func (e *statEmitter) ratesLocked() (pageRate, itemRate float64) {
	now := e.cfg.now()
	e.pruneSamplesLocked(now)
	if len(e.samples) < 2 {
		return 0, 0
	}
	first, last := e.samples[0], e.samples[len(e.samples)-1]
	span := now.Sub(first.at)
	if span < rateWindowMin {
		return 0, 0
	}
	seconds := span.Seconds()
	return float64(last.pages-first.pages) / seconds, float64(last.items-first.items) / seconds
}

// etaLocked is §15.3's defensible estimate and nothing else: zero unless
// every input is present. Fetching is additionally capped by the source's
// own ceiling — an unrepresentative burst must not promise the user a
// finish time the API cannot deliver.
func (e *statEmitter) etaLocked(pageRate, itemRate float64) int64 {
	if e.snap.phase == importv2.PhaseScanning {
		return 0 // no total to subtract from
	}
	remaining, rate := e.snap.pagesTotal-e.snap.pagesDone, pageRate
	if e.epoch == 0 {
		// FETCHING: pages only, against a pages/s ceiling. filesTotal is
		// unknown while crawling (files are found BY crawling), so there is
		// no file remainder to add — and the ceiling is a page ceiling.
		if e.cfg.pageRateCeiling > 0 && rate > e.cfg.pageRateCeiling {
			rate = e.cfg.pageRateCeiling
		}
	} else {
		// MATERIALIZING: both totals come from the same spool census, so
		// both remainders are known — and file uploads are real work, not a
		// rounding error, on an import with an image per page. Pricing them
		// at zero would under-report the wait by half on such a run.
		remaining += e.snap.filesTotal - e.snap.filesDone
		rate = itemRate
	}
	if remaining <= 0 || rate <= 0 {
		return 0
	}
	return int64(float64(remaining) / rate * 1000)
}

// stateForLog is the ONLY rendering of an emitter that may reach a log
// line: codes and counts, never content. currentItem is a DisplayText, so
// even this deliberate rendering of it yields the md5 the codebase hashes
// user text with (v1 notion's hashText) rather than the title.
func (e *statEmitter) stateForLog() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return fmt.Sprintf("phase=%s state=%s pages=%d/%d files=%d/%d bytes=%d warnings=%d errors=%d item=%s",
		logPhase(e.snap.phase), logState(e.snap.state),
		e.snap.pagesDone, e.snap.pagesTotal, e.snap.filesDone, e.snap.filesTotal,
		e.snap.bytesDone, e.snap.warningCount, e.snap.errorCount, e.snap.currentItem)
}

func logPhase(p importv2.Phase) string {
	switch p {
	case importv2.PhaseScanning:
		return "scanning"
	case importv2.PhaseAnalyzing:
		return "analyzing"
	case importv2.PhaseFetching:
		return "fetching"
	case importv2.PhaseCreating:
		return "creating"
	case importv2.PhaseFinalizing:
		return "finalizing"
	default:
		return "unknown"
	}
}

func logState(s pb.EventImportStatisticState) string {
	switch s {
	case pb.EventImportStatistic_Throttled:
		return "throttled"
	case pb.EventImportStatistic_Retrying:
		return "retrying"
	case pb.EventImportStatistic_Error:
		return "error"
	default:
		return "running"
	}
}

// wirePhase maps the engine's phase onto the wire enum. The two are kept
// apart on purpose: the engine's vocabulary is shared with the converters
// and must not depend on pb.
func wirePhase(p importv2.Phase) pb.EventImportStatisticPhase {
	switch p {
	case importv2.PhaseAnalyzing:
		return pb.EventImportStatistic_Analyzing
	case importv2.PhaseFetching:
		return pb.EventImportStatistic_Fetching
	case importv2.PhaseCreating:
		return pb.EventImportStatistic_Creating
	case importv2.PhaseFinalizing:
		return pb.EventImportStatistic_Finalizing
	default:
		return pb.EventImportStatistic_Scanning
	}
}

// wireCancelEffect falls straight out of the pass model (§15.2): during
// passes 1-2 nothing has entered the space, so cancel is instant and undoes
// nothing; from pass 3 it removes what was created. The dormant surface
// derives the same answer from the manifest's materialize marker, which is
// set at exactly this boundary.
func wireCancelEffect(p importv2.Phase) pb.EventImportStatisticCancelEffect {
	if p >= importv2.PhaseCreating {
		return pb.EventImportStatistic_RemovesCreated
	}
	return pb.EventImportStatistic_NothingToUndo
}
