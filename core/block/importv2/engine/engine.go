// Package engine orchestrates one import run: the identity pass, the
// streaming convert→resolve→persist pipeline with bounded channels and a
// worker pool, the single cancellation context, the uniform abort predicate,
// journal-based compensation on failure, and root-collection finalization.
package engine

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/identity"
	"github.com/anyproto/anytype-heart/core/block/importv2/persist"
	"github.com/anyproto/anytype-heart/core/block/importv2/report"
	"github.com/anyproto/anytype-heart/core/block/importv2/resolve"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	// channelCapacity bounds queued heavy objects per lane; workerCount
	// bounds in-flight persists. Peak heavy-object residency is
	// 2*channelCapacity + workerCount — the memory invariant.
	channelCapacity = 16
	workerCount     = 8

	compensationTimeout = 5 * time.Minute
)

// Reporter is the rich internal progress seam (deferred-materialization
// spec §15.4): the adapter down-projects it onto process.Progress AND feeds
// the coalescing importStatistic emitter from it. Implementations must be
// safe for concurrent use, and must never affect control flow — every call
// here is advisory telemetry.
//
// The counters are PER KIND and PER PHASE, which is the whole point of the
// redesign. The legacy seam blended pages, files and definitions into one
// `Step(1)`, so no honest `pagesDone` could be published under a field name
// the wire contract had already fixed (§15.7). And blending the two passes
// into one denominator is what §15.3 forbids outright: pass 2 runs at the
// pacer's ~1.5 items/s and pass 3 at persist speed, so one bar crawls for an
// hour and then leaps.
type Reporter interface {
	// Phase announces a stage. It RE-BASES the counters: fetching counts
	// spooled rows against pass 1's claim count, materializing counts
	// persisted rows against the spool census.
	Phase(p importv2.Phase)
	// Discovered adds to the current phase's denominator for one kind.
	Discovered(kind importv2.Kind, delta int64)
	// Completed adds to the current phase's numerator for one kind.
	Completed(kind importv2.Kind, delta int64)
	// Bytes publishes the run's downloaded bytes as a LEVEL — bytes ON
	// DISK in the spill dir, which is the only definition the dormant
	// surface can also hold (importv2.SpillBytes). A level and not a delta
	// because a resumed run inherits its predecessor's downloads. No total
	// accompanies it: Notion's file blocks carry no size, so bytesTotal
	// stays the schema's documented 0-is-unknown.
	Bytes(total int64)
	// Created publishes the run's created-object count as a LEVEL — the
	// cancel affordance's "stop and remove the N objects created". A level
	// and not a delta because a resumed run starts at the ledger's count.
	Created(count int64)
	// Item sets the displayable current item (user content; see
	// importv2.DisplayText).
	Item(item importv2.DisplayText)
}

type noopReporter struct{}

func (noopReporter) Phase(importv2.Phase)            {}
func (noopReporter) Discovered(importv2.Kind, int64) {}
func (noopReporter) Completed(importv2.Kind, int64)  {}
func (noopReporter) Bytes(int64)                     {}
func (noopReporter) Created(int64)                   {}
func (noopReporter) Item(importv2.DisplayText)       {}

// IdentityService is the identity seam (implemented by identity.Service).
type IdentityService interface {
	Claim(ctx context.Context, c importv2.IdentityClaim) error
	// FlushClaims drains the buffered durable claim batch (no-op without a
	// ledger); the engine calls it at the end of pass 1.
	FlushClaims(ctx context.Context) error
	Assign(sourceKey string) (identity.Assignment, error)
	AssignDerived(ctx context.Context, o *importv2.Object) (identity.Assignment, error)
	RegisterFile(sourceKey string)
	CompleteFile(sourceKey, id string, err error)
	Resolve(sourceKey string) (id string, ok bool)
	// UnassignedClaims lists pass-1 claims that never arrived in pass 2.
	UnassignedClaims() []string
}

// ObjectPersister is the persistence seam (implemented by persist.Persister).
type ObjectPersister interface {
	Persist(ctx context.Context, o *importv2.Object, target persist.Target, report func(importv2.Issue)) (persist.Outcome, error)
}

type Deps struct {
	Identity  IdentityService
	Persister ObjectPersister
	Journal   *persist.Journal
	// Objects serves compensation deletes.
	Objects persist.ObjectAccess
	Formats *resolve.Formats
	Keys    *KeyTable
	// Collection may be nil when root collections are not supported.
	Collection importv2.CollectionFactory
	Reporter   Reporter
	// Gauge, when set, tracks in-flight heavy objects (test hook for the
	// bounded-memory invariant).
	Gauge func(delta int)
	// OnCompensating, when set, is invoked once before the first
	// compensation delete — the adapter persists the run's "compensating"
	// state there, so a crash mid-cleanup is finished by the startup sweep
	// (spec §6.5). A non-nil error GATES the cleanup: without the durable
	// marker a crash mid-compensation would make the next start RESUME a
	// partly-compensated run, silently missing its deleted objects — so no
	// marker, no deletes (the dir is kept via the CompensationRan rule).
	OnCompensating func() error
	// OnIssue, when set, receives every retained issue as it is reported —
	// the adapter's durable issue ledger (pass-2 issues must survive to the
	// pass-3 report page, DM spec §6.2). Called outside the issue lock; must
	// be safe for concurrent use and must not block.
	OnIssue func(issue importv2.Issue)
	// Spool is the pass-2 → pass-3 queue. nil falls back to an in-memory
	// spool (unit tests; makes no memory-invariant claim). Real runs get the
	// disk-backed runstore spool from the adapter.
	Spool Spool
	// SpillDir, when set, is where pass 2 drains file Open closures so the
	// spooled object carries a plain path. Empty keeps closures in place
	// (memory-spool mode only).
	SpillDir string
	// OnFetched, when set, fires between pass 2 and pass 3 with pass 2's
	// RootSpec — the adapter persists it and marks the manifest
	// fetched/materializing there (DM spec §4.1 + §6.4: a restart has no
	// converter to re-produce the RootSpec). Its failure is FATAL (S6): the
	// transition is journaling — the A1 compensation gate depends on it —
	// and §7.2 forbids creating objects past a failed journal write.
	OnFetched func(rootSpec importv2.RootSpec) error
	// ShutdownCtx, when set, bounds compensation (S1): it must survive the
	// RUN's cancellation (compensation runs exactly when the run ctx is
	// dead) but die with the COMPONENT, so Close actually reaches the
	// between-deletes check instead of burning its grace while deletes
	// continue into closing services. nil falls back to Background.
	ShutdownCtx context.Context
}

// Run executes one import. The passed ctx is the run's single cancellation
// source (the adapter joins process cancel into it).
//
// Panic firewall (§16 item 2): a panic anywhere in the run becomes a typed
// invariant issue, never a process crash — per object in persistGuarded, per
// goroutine in the converter/worker spawns, and here as the last resort for
// the main-goroutine stages (identity pass, finalize, compensation).
func Run(ctx context.Context, req importv2.Request, converter importv2.Converter, deps Deps) *importv2.Result {
	return startRun(ctx, req, converter, deps, nil)
}

// CrawlResumeState seeds a pass-2 crawl restart (DM spec §8.3): a run
// interrupted mid-crawl re-runs both passes against the live source, with
// the spool's recorded keys as the skip set — no separate status
// bookkeeping, the recording IS the progress.
type CrawlResumeState struct {
	// SpooledKeys are the rows a previous incarnation recorded: the
	// converter's Skip set (ResumableConverter) and the sink backstop's
	// drop set — a re-emission of a recorded key is absorbed before any
	// download or append, and the replay materializes the recorded row.
	SpooledKeys map[string]struct{}
	// PriorClaims are previous incarnations' claim keys. A prior claim that
	// neither re-enumerates this incarnation nor sits in the spool is
	// SOURCE DRIFT (a page deleted between sessions, 08-13 §5.4) — dropped
	// with a data-loss warning at reconciliation, where a fresh claim in
	// the same gap stays the invariant violation it always was.
	PriorClaims map[string]struct{}
	// Issues are previous incarnations' retained issues, seeded without
	// re-reporting (as engine.Resume).
	Issues []importv2.Issue
}

// ResumeCrawl is pass 2 restarted against the live source (DM spec §8.3):
// pass 1 re-runs with a rehydrated identity (claims are reuses — the
// adapter wires identity.WithRehydrated with reclaimable entries), pass 2
// re-crawls skipping what the spool already holds, and pass 3 then
// materializes the whole recording exactly as a fresh run would. Unlike
// engine.Resume this DOES need the source and its credentials — which is
// why the manifest carries the request for exactly this class of run.
func ResumeCrawl(ctx context.Context, req importv2.Request, converter importv2.Converter, deps Deps, state *CrawlResumeState) *importv2.Result {
	return startRun(ctx, req, converter, deps, state)
}

func startRun(ctx context.Context, req importv2.Request, converter importv2.Converter, deps Deps, crawl *CrawlResumeState) (result *importv2.Result) {
	if deps.Reporter == nil {
		deps.Reporter = noopReporter{}
	}
	if deps.Gauge == nil {
		deps.Gauge = func(int) {}
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	r := &run{
		req:        req,
		deps:       deps,
		cancel:     cancel,
		stopCtx:    ctx,
		failedKeys: map[string]struct{}{},
		issuedKeys: map[string]struct{}{},
	}
	if crawl != nil {
		if deps.Spool == nil {
			// INERT failure, the Resume nil-spool sibling (review P1-D): the
			// memory fallback cannot hold incarnation 1's rows, so running on
			// would silently re-import half the source. Nothing happened here,
			// so nothing may be undone: CompensationRan stays false and the
			// disposal invariant keeps the dir for the sweep.
			issue := importv2.Fatal(importv2.IssueInvariant, fmt.Errorf("crawl resume requires the run's durable spool"))
			return &importv2.Result{Err: issue, Issues: []importv2.Issue{issue}}
		}
		r.crawlResume = crawl
		r.claimedNow = map[string]struct{}{}
		r.seedIssues(crawl.Issues)
		if rc, ok := converter.(importv2.ResumableConverter); ok {
			rc.SetSkip(func(sourceKey string) bool {
				_, spooled := crawl.SpooledKeys[sourceKey]
				return spooled
			})
			// The seam's obligation half (review P0-A): every prior claim
			// without a spool row is offered for recovery — the skip set
			// suppresses re-walking recorded parents, so a claim made during
			// pass-2 discovery would otherwise never be re-found and would
			// misreport as source drift. Sorted for determinism; the
			// converter filters keys it re-encounters on its own.
			unrecorded := make([]string, 0, len(crawl.PriorClaims))
			for key := range crawl.PriorClaims {
				if _, spooled := crawl.SpooledKeys[key]; !spooled {
					unrecorded = append(unrecorded, key)
				}
			}
			sort.Strings(unrecorded)
			rc.SetRecover(unrecorded)
		}
	}
	defer func() {
		if rec := recover(); rec != nil {
			r.report(importv2.Fatal(importv2.IssueInvariant, panicError("import run", rec)))
			result = r.finish(runCtx, importv2.RootSpec{})
		}
	}()

	if fatal := r.identityPass(runCtx, converter); fatal != nil {
		r.report(*fatal)
		return r.finish(runCtx, importv2.RootSpec{})
	}

	// Pass 2 — fetch, convert, spool: nothing enters the space, so an abort
	// anywhere in here compensates nothing and costs nothing (DM spec §7).
	spool := deps.Spool
	if spool == nil {
		spool = &memorySpool{}
	}
	rootSpec := r.spoolPass(runCtx, converter, spool)
	if r.fatalIssue() != nil || runCtx.Err() != nil {
		return r.finish(runCtx, importv2.RootSpec{})
	}
	if r.deps.OnFetched != nil {
		if err := r.deps.OnFetched(rootSpec); err != nil {
			r.report(importv2.Fatal(importv2.IssueStoreError, fmt.Errorf("mark pass boundary: %w", err)))
			return r.finish(runCtx, importv2.RootSpec{})
		}
	}

	// Pass 3 — materialize: the existing streaming pipeline fed by the
	// recording. rootSpec comes from pass 2; the replay's own is empty.
	return r.materializeTail(runCtx, spool, rootSpec, reportTitle(rootSpec, converter))
}

// materializeTail is pass 3 through the exit: replay, finalize, reconcile,
// report, finish. Shared verbatim between a first run and a resumed one so
// the two paths cannot drift.
func (r *run) materializeTail(runCtx context.Context, spool Spool, rootSpec importv2.RootSpec, title string) *importv2.Result {
	r.beginMaterialize(runCtx, spool)
	r.streamPass(runCtx, &spoolReplayConverter{spool: spool})
	if r.fatalIssue() != nil || runCtx.Err() != nil {
		return r.finish(runCtx, importv2.RootSpec{})
	}
	r.deps.Reporter.Phase(importv2.PhaseFinalizing)
	reportClaimed := r.maybeClaimReport(runCtx)
	r.finalize(runCtx, rootSpec)
	r.reconcileClaims()
	if r.fatalIssue() != nil || runCtx.Err() != nil {
		return r.finish(runCtx, importv2.RootSpec{})
	}
	r.emitReport(runCtx, reportClaimed, title)
	r.allStagesDone = true
	return r.finish(runCtx, rootSpec)
}

// beginMaterialize announces pass 3 and fixes its denominators from the
// spool census — the SAME rows the pull surface counts for a dormant run
// (§15.4), so a run that is polled and a run that is pushed cannot report
// different totals. A census failure costs telemetry only: the seam is
// advisory and must never fail a run that is otherwise fine.
func (r *run) beginMaterialize(ctx context.Context, spool Spool) {
	r.deps.Reporter.Phase(importv2.PhaseCreating)
	// Bytes are a RUN level measured from the spill, so a pass-3 restart —
	// which never runs spoolPass — still reports its predecessor's
	// downloads instead of zero.
	r.deps.Reporter.Bytes(importv2.SpillBytes(r.deps.SpillDir))
	pages, files, _, err := spool.Census(ctx)
	if err != nil {
		// Swallowed deliberately, and not turned into an issue: the replay
		// reads the same rows a moment later and fails LOUDLY there
		// (storeError, §7.2) — reporting it twice would put a telemetry
		// failure on the user's report page under its own code.
		return
	}
	r.deps.Reporter.Discovered(importv2.KindPage, int64(pages))
	r.deps.Reporter.Discovered(importv2.KindFile, int64(files))
}

// publishCreated announces the created LEVEL, and never a lower one than it
// has already announced.
//
// Reporter.Created takes a level rather than a delta because a resumed run
// starts at its ledger's count — but workerCount persist workers publish
// created.Add(1) concurrently, and the atomic orders the INCREMENTS, not the
// calls: worker B can take 6, worker A take 5, and A announce after B. The
// surface then counts DOWN (measured regressing on every run of a 600-page
// import, one settling at 598/600), which is §15.4's cancel affordance —
// "stop and remove the N objects created" — and disagrees with the exact
// ledger count the same run answers when polled dormant (§15.5).
//
// The lock is held ACROSS the call on purpose: dropping it first would order
// the high-water mark and leave the announcements to race again. The seam is
// advisory and non-blocking by contract (the emitter sends under its own
// lock for the same reason), and this costs one uncontended mutex per
// created object.
func (r *run) publishCreated(level int64) {
	r.createdMu.Lock()
	defer r.createdMu.Unlock()
	if level <= r.createdPublished {
		return
	}
	r.createdPublished = level
	r.deps.Reporter.Created(level)
}

// countObject is the ONE classification behind every per-kind counter, used
// by pass 2 (rows spooled) and pass 3 (rows materialized) alike so the two
// halves of the run cannot disagree about what a "page" is. Derived-class
// definitions — relations, types, options — are counted by neither kind:
// they are engine bookkeeping with no pass-1 claim, and counting them would
// push done past a total that is the claim count.
// nameTableCap bounds the report's name table. It is far above any workspace
// that fits the issue cap, and exists so a pathological import cannot grow
// the table without limit.
const nameTableCap = 100_000

// rememberName records what an object is called, for the report.
func (r *run) rememberName(o *importv2.Object) {
	if o == nil || o.Payload == nil || o.SourceKey == "" {
		return
	}
	// Recorded even when the name is empty: membership means "this run
	// actually emitted the object", which is what makes a mention link safe.
	// A key can resolve through the identity table and still have no object
	// behind it — a claim from an interrupted session, a page whose fetch
	// failed — and a mention pointing at one of those renders as a dead
	// _missing_object rather than a name.
	name := o.Payload.Details.GetString(bundle.RelationKeyName)
	r.nameMu.Lock()
	defer r.nameMu.Unlock()
	if r.names == nil {
		r.names = make(map[string]string)
	}
	if len(r.names) >= nameTableCap {
		return
	}
	r.names[o.SourceKey] = name
}

// reportLookup is the report's view of a source key: what it is called and
// whether it can be linked.
func (r *run) reportLookup(sourceKey string) report.Source {
	id, ok := r.deps.Identity.Resolve(sourceKey)
	r.nameMu.Lock()
	name, emitted := r.names[sourceKey]
	r.nameMu.Unlock()
	return report.Source{Name: name, Resolved: ok && id != "" && emitted}
}

func (r *run) countObject(o *importv2.Object) {
	r.rememberName(o)
	switch {
	case isFileClass(o.SbType):
		r.deps.Reporter.Completed(importv2.KindFile, 1)
	case !isDerivedClass(o.SbType):
		r.deps.Reporter.Completed(importv2.KindPage, 1)
	}
}

// ResumeState seeds a pass-3 restart (DM spec §8.1): everything the run
// carries across incarnations beyond what the identity service rehydrates.
type ResumeState struct {
	// RootSpec is pass 2's persisted output — a restart has no converter to
	// re-produce it.
	RootSpec importv2.RootSpec
	// ConverterName is the report-title fallback (manifest.Converter).
	ConverterName string
	// SkipKeys are rows a previous incarnation finished (terminal entries,
	// done files): the sink acknowledges and drops them — membership and
	// progress, no persist. Derived-class rows are never skipped whatever
	// this set says (their re-derivation reseeds the format/key registries
	// and converges via dedup or deterministic derivation).
	SkipKeys map[string]struct{}
	// RootCollectionId, when non-empty, marks finalize complete in a
	// previous incarnation: it is reused, never rebuilt (a rebuilt
	// collection would mint a second one — the name is date-suffixed, so
	// every build claims a fresh key).
	RootCollectionId string
	// ReportObjectId, when non-empty, marks the report page persisted in a
	// previous incarnation. Issues the RESUMED incarnation adds do not
	// reach that page (they still reach the result and the wire counts) —
	// accepted: re-persisting the report would re-open the very
	// crash-window class this phase closes.
	ReportObjectId string
	// Created/Updated continue the ledger's counts (§15.4: a restart
	// resumes the numbers, not just the work). Skipped/Failed are not
	// durable and restart at zero — recorded, not hidden.
	Created int64
	Updated int64
	// Issues are the previous incarnations' retained issues: seeded
	// directly (no re-report — the abort predicate and the durable ledger
	// already saw them in their own incarnation).
	Issues []importv2.Issue
}

// Resume is pass 3 restarted from the recorded spool: no pass 1, no
// pass 2, no converter, no network — the run is a function of (run dir,
// space). deps.Identity must be rehydrated (identity.WithRehydrated) and
// deps.Spool must be the run's durable spool.
func Resume(ctx context.Context, req importv2.Request, deps Deps, state *ResumeState) (result *importv2.Result) {
	if deps.Reporter == nil {
		deps.Reporter = noopReporter{}
	}
	if deps.Gauge == nil {
		deps.Gauge = func(int) {}
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	r := &run{
		req:        req,
		deps:       deps,
		cancel:     cancel,
		stopCtx:    ctx,
		failedKeys: map[string]struct{}{},
		issuedKeys: map[string]struct{}{},
		resume:     state,
	}
	defer func() {
		if rec := recover(); rec != nil {
			r.report(importv2.Fatal(importv2.IssueInvariant, panicError("import resume", rec)))
			result = r.finish(runCtx, importv2.RootSpec{})
		}
	}()
	if deps.Spool == nil {
		// INERT failure, deliberately bypassing finish() (review P1-D): the
		// run never started, and finish's compensation would run over the
		// journal — which resume construction SEEDS with every previous
		// incarnation's effects — turning a wiring bug into the destruction
		// of the whole import and then (Leaked 0) of its ledger. Nothing
		// happened here, so nothing may be undone: CompensationRan stays
		// false and the disposal invariant keeps the dir for the sweep.
		issue := importv2.Fatal(importv2.IssueInvariant, fmt.Errorf("resume requires the run's durable spool"))
		return &importv2.Result{Err: issue, Issues: []importv2.Issue{issue}}
	}
	r.created.Store(state.Created)
	r.updated.Store(state.Updated)
	// A restart resumes the NUMBERS, not just the work (§15.4): the cancel
	// affordance must say "remove the N objects created" counting every
	// incarnation, not only this one's. Through publishCreated like every
	// other publication, so this incarnation's workers start ABOVE the seed
	// rather than beside it.
	r.publishCreated(state.Created)
	r.seedIssues(state.Issues)
	title := "Import report — " + state.ConverterName
	if state.RootSpec.CollectionName != "" {
		title = "Import report — " + state.RootSpec.CollectionName
	}
	return r.materializeTail(runCtx, deps.Spool, state.RootSpec, title)
}

// seedIssues restores previous incarnations' issues without re-reporting
// them: no OnIssue (the durable ledger already holds them), no abort
// predicate (they aborted or not in their own incarnation — in particular
// the interrupted incarnation's own cancelled fatal must not abort the
// resumed one).
func (r *run) seedIssues(issues []importv2.Issue) {
	r.issueMu.Lock()
	defer r.issueMu.Unlock()
	for _, issue := range issues {
		if len(r.issues) < importv2.IssueCap {
			r.issues = append(r.issues, issue)
		} else {
			r.issuesDropped++
		}
		if issue.SourceKey != "" {
			r.issuedKeys[issue.SourceKey] = struct{}{}
		}
		if issue.Severity >= importv2.SeverityWarning {
			r.loudIssues++
		}
	}
}

// finish is the SINGLE exit classification (Invariant 1): every path out of
// a run — pass-1 failure, stream abort, finalize-stage fatal, panic, and the
// success path itself — consults the stop state here. A dead run context
// with no recorded fatal becomes a cancelled fatal (with lanes holding
// 2*C+K objects, a small import is fully buffered when cancel fires: the
// converter has already returned cleanly, interrupted persists are
// accounted skipped, and nobody else reports the abort — without this, a
// user cancel yielded a silent "success" and the review confirmed the same
// hole again during finalize). On any fatal, the suspend verdict (cause
// importv2.ErrSuspended, spec §6.4) decides compensation: a suspend keeps
// the durable state for the startup sweep; everything else compensates.
func (r *run) finish(runCtx context.Context, rootSpec importv2.RootSpec) *importv2.Result {
	// The STOP SOURCE, decided once and carried out on the Result (review
	// item 1): every downstream decision about this run's dir used to read
	// the fatal's CODE, and a code is a shape both a transport deadline and
	// a cancelled Notion call wear wrongly. The caller's context is
	// unambiguous — progress.Canceled() fires cancel(nil), shutdown fires
	// cancel(ErrSuspended) — so ask it, not the error.
	r.cancelledRun = r.stopCtx != nil && r.stopCtx.Err() != nil &&
		!errors.Is(context.Cause(r.stopCtx), importv2.ErrSuspended)
	// E4: claims made after pass 2 (root collection, report page) must
	// reach the ledger — every exit passes through here, so this is the
	// one flush that cannot be skipped. Detached ctx: the run context is
	// dead on the abort paths, and intent must still land (P0-1 rule).
	if err := r.deps.Identity.FlushClaims(context.Background()); err != nil {
		r.report(importv2.Warning(importv2.IssueStoreError, "",
			fmt.Sprintf("flush late claims: %s", err)))
	}
	if runCtx.Err() != nil && r.fatalIssue() == nil {
		// B3: a shutdown suspend landing after EVERY mutating stage has
		// completed has nothing left to stop — classifying it as suspended
		// made the next sweep compensate a complete import. The run is
		// terminal-success. A user cancel in the same window still undoes:
		// that is the cancel contract.
		if r.allStagesDone && errors.Is(context.Cause(runCtx), importv2.ErrSuspended) {
			return r.buildResult(importv2.Issue{}, rootSpec)
		}
		r.report(importv2.Fatal(importv2.IssueCancelled, context.Cause(runCtx)))
	}
	fatal := r.fatalIssue()
	if fatal == nil {
		return r.buildResult(importv2.Issue{}, rootSpec)
	}
	r.suspendedRun = errors.Is(context.Cause(runCtx), importv2.ErrSuspended)
	if !r.suspendedRun {
		r.compensate()
	}
	return r.buildResult(*fatal, importv2.RootSpec{})
}

func reportTitle(rootSpec importv2.RootSpec, converter importv2.Converter) string {
	name := rootSpec.CollectionName
	if name == "" {
		name = converter.Name()
	}
	return "Import report — " + name
}

// panicError renders a recovered panic (with the panic-site stack — the
// frames are still live inside the recovering defer) as a wrappable error.
func panicError(where string, rec any) error {
	return fmt.Errorf("%s panic: %v\n%s", where, rec, debug.Stack())
}

type run struct {
	req    importv2.Request
	deps   Deps
	cancel context.CancelFunc

	created atomic.Int64
	updated atomic.Int64
	skipped atomic.Int64
	failed  atomic.Int64
	// createdMu/createdPublished serialize the created LEVEL's publication.
	// See publishCreated: the counter is atomic, but a level is an ORDERED
	// quantity and nothing else here orders the increment against the call
	// that announces it.
	createdMu        sync.Mutex
	createdPublished int64
	// spilledBytes is the pass-2 download level, seeded from the spill dir
	// so a resumed crawl continues its predecessor's count.
	spilledBytes atomic.Int64

	// nameMu guards names: source key → the display name of the object it
	// became. The report shows these instead of source keys, because a
	// Notion id ("450313a1-1e14-82b1-…") tells a reader nothing. A resumed
	// incarnation rebuilds only the names of objects it re-emits; the rest
	// fall back to their key, which is the same information the ledger had
	// before names existed.
	nameMu sync.Mutex
	names  map[string]string

	issueMu sync.Mutex
	issues  []importv2.Issue
	// issuedKeys records every SourceKey that carries an issue, uncapped —
	// the exclusion set for claims reconciliation (a claim that failed
	// loudly is not a silent drop).
	issuedKeys    map[string]struct{}
	issuesDropped int64
	// loudIssues counts warning-or-worse issues: info diagnostics ride along
	// in a report page but never cause one.
	loudIssues int64
	fatal      *importv2.Issue

	// rootCandidates is recorded in stream order (sink side) so membership
	// stays deterministic; failed objects are filtered out at finalize.
	rootMu         sync.Mutex
	rootCandidates []string
	failedKeys     map[string]struct{}

	rootCollectionId string
	reportObjectId   string
	compensated      int
	leaked           int
	compensateState  int // 0 not run, 1 running, 2 done
	compensationRan  bool
	nothingToUndo    bool
	allStagesDone    bool
	// suspendedRun records the engine's own verdict — the run stopped for a
	// shutdown suspend and was NOT compensated — carried out via
	// Result.Suspended so the adapter never re-derives it from a context.
	suspendedRun bool
	// cancelledRun is the sibling verdict: the USER stopped this run,
	// carried out via Result.Cancelled. Read from stopCtx below.
	cancelledRun bool
	// stopCtx is the context the CALLER owns — the only unambiguous stop
	// source. The engine derives runCtx from it and cancels THAT itself on
	// every abort, so context.Cause(runCtx) reads context.Canceled for a
	// self-abort exactly as it does for a user cancel; only the caller's
	// context tells the two apart.
	stopCtx context.Context
	// resume is non-nil on a resumed incarnation (engine.Resume): the sink
	// consults its skip set, finalize and the report consult its recorded
	// outcomes.
	resume *ResumeState
	// crawlResume is non-nil on a crawl-resumed incarnation (ResumeCrawl):
	// the spool sink consults its recorded-key set, reconciliation its
	// prior-claim set.
	crawlResume *CrawlResumeState
	// claimedNow records claims arriving in THIS incarnation (crawl resume
	// only) — the input separating source drift from converter bugs at
	// reconciliation. Guarded by claimMu: claims arrive on the main
	// goroutine (pass 1), the converter goroutine (late claims) and the
	// replay goroutine (pass 3), sequential phases on different goroutines.
	claimMu    sync.Mutex
	claimedNow map[string]struct{}
}

// noteClaimed records one claim of the current incarnation (crawl resume
// only — a fresh run has no prior claims to separate from).
func (r *run) noteClaimed(sourceKey string) {
	if r.crawlResume == nil {
		return
	}
	r.claimMu.Lock()
	r.claimedNow[sourceKey] = struct{}{}
	r.claimMu.Unlock()
}

// recordedInSpool reports whether a previous incarnation already recorded
// the key (crawl resume): the sink backstop drops such a re-emission before
// any download or append — the replay materializes the recorded row.
func (r *run) recordedInSpool(sourceKey string) bool {
	if r.crawlResume == nil {
		return false
	}
	_, ok := r.crawlResume.SpooledKeys[sourceKey]
	return ok
}

// staleAcrossIncarnations reports whether an unassigned claim is source
// drift on a crawl-resumed run: claimed by a previous incarnation, never
// spooled, and never re-enumerated by this incarnation's pass 1 — the
// entity disappeared from the source between sessions (08-13 §5.4).
func (r *run) staleAcrossIncarnations(key string) bool {
	if r.crawlResume == nil {
		return false
	}
	if _, prior := r.crawlResume.PriorClaims[key]; !prior {
		return false
	}
	r.claimMu.Lock()
	defer r.claimMu.Unlock()
	_, reclaimed := r.claimedNow[key]
	return !reclaimed
}

// report is the single issue funnel: collects (capped) and applies the one
// abort predicate.
func (r *run) report(issue importv2.Issue) {
	r.issueMu.Lock()
	kept := len(r.issues) < importv2.IssueCap
	if kept {
		r.issues = append(r.issues, issue)
	} else {
		r.issuesDropped++
	}
	if issue.SourceKey != "" {
		r.issuedKeys[issue.SourceKey] = struct{}{}
	}
	if issue.Severity >= importv2.SeverityWarning {
		r.loudIssues++
	}
	abort := importv2.ShouldAbort(issue.Severity, r.req.Mode) && r.fatal == nil
	if abort {
		fatal := issue
		r.fatal = &fatal
	}
	r.issueMu.Unlock()
	if kept && r.deps.OnIssue != nil {
		r.deps.OnIssue(issue)
	}
	if abort {
		r.cancel()
	}
}

func (r *run) fatalIssue() *importv2.Issue {
	r.issueMu.Lock()
	defer r.issueMu.Unlock()
	return r.fatal
}

func (r *run) identityPass(ctx context.Context, converter importv2.Converter) *importv2.Issue {
	r.deps.Reporter.Phase(importv2.PhaseScanning)
	count := int64(0)
	err := converter.EnumerateIdentities(ctx, func(claim importv2.IdentityClaim) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.deps.Identity.Claim(ctx, claim); err != nil {
			return err
		}
		r.noteClaimed(claim.SourceKey)
		count++
		// Per claim, not once at the end: a cursor-chained /search does not
		// know its own count until the chain ends, so SCANNING publishes a
		// count-up ("3,412 found") and totalsKnown stays false until the
		// pass-1/pass-2 boundary (§15.3).
		r.deps.Reporter.Discovered(importv2.KindPage, 1)
		return nil
	})
	if err != nil {
		issue := classifyFatal(ctx, err, importv2.IssueSourceInvalid)
		return &issue
	}
	if count == 0 {
		issue := importv2.Fatal(importv2.IssueNoObjects, fmt.Errorf("source contains no importable objects"))
		return &issue
	}
	if err := r.deps.Identity.FlushClaims(ctx); err != nil {
		issue := classifyFatal(ctx, err, importv2.IssueStoreError)
		return &issue
	}
	return nil
}

func (r *run) streamPass(ctx context.Context, converter importv2.Converter) importv2.RootSpec {
	objectCh := make(chan work, channelCapacity)
	fileCh := make(chan work, channelCapacity)
	sink := &engineSink{run: r, objectCh: objectCh, fileCh: fileCh}

	var rootSpec importv2.RootSpec
	var convertErr error
	converterDone := make(chan struct{})
	go func() {
		defer close(converterDone)
		defer close(objectCh)
		defer close(fileCh)
		// Registered last so it runs first: recover before the channels
		// close, turning a converter panic into a fatal invariant issue.
		defer func() {
			if rec := recover(); rec != nil {
				convertErr = importv2.Fatal(importv2.IssueInvariant, panicError("converter", rec))
			}
		}()
		rootSpec, convertErr = converter.Convert(ctx, sink)
	}()

	var wg sync.WaitGroup
	// One worker is dedicated to the file lane so a queued file object can
	// always progress — this is what keeps future waits deadlock-free even
	// under converter contract violations.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer r.recoverWorker()
		for w := range fileCh {
			r.process(ctx, w)
		}
	}()
	for i := 0; i < workerCount-1; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer r.recoverWorker()
			objects, files := objectCh, fileCh
			for objects != nil || files != nil {
				select {
				case w, ok := <-objects:
					if !ok {
						objects = nil
						continue
					}
					r.process(ctx, w)
				case w, ok := <-files:
					if !ok {
						files = nil
						continue
					}
					r.process(ctx, w)
				}
			}
		}()
	}
	<-converterDone
	wg.Wait()

	if convertErr != nil && r.fatalIssue() == nil {
		r.report(classifyFatal(ctx, convertErr, importv2.IssueSourceInvalid))
	}
	return rootSpec
}

type work struct {
	object *importv2.Object
	target persist.Target
	isFile bool
}

// recoverWorker is the per-worker insurance firewall: persistGuarded already
// contains per-object panics, so this only fires on an engine bug in the loop
// itself — the run aborts with a typed issue and cancel unblocks the lanes.
func (r *run) recoverWorker() {
	if rec := recover(); rec != nil {
		r.report(importv2.Fatal(importv2.IssueInvariant, panicError("persist worker", rec)))
	}
}

func (r *run) process(ctx context.Context, w work) {
	defer r.deps.Gauge(-1)
	if ctx.Err() != nil {
		if w.isFile {
			r.deps.Identity.CompleteFile(w.object.SourceKey, "", ctx.Err())
		}
		r.skipped.Add(1)
		return
	}
	outcome, err := r.persistGuarded(ctx, w)
	if w.isFile {
		r.deps.Identity.CompleteFile(w.object.SourceKey, outcome.Id, err)
	}
	if err != nil {
		if ctx.Err() != nil &&
			(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) &&
			importv2.AsIssue(err, importv2.SeverityObjectError, importv2.IssueObjectFailed).Severity < importv2.SeverityFatal {
			// The run is being cancelled or suspended: an interrupted
			// persist is not an object failure — the abort is accounted
			// once, by the fatal cancellation (Run's post-stream guard), and
			// the issue code must stay "cancelled" (not "objectFailed") for
			// the wire mapping. Fatal issues that merely WRAP a context
			// error (a durable-journal write timing out during shutdown)
			// must not be absorbed here: they abort loudly.
			r.skipped.Add(1)
			return
		}
		r.failed.Add(1)
		r.rootMu.Lock()
		r.failedKeys[w.object.SourceKey] = struct{}{}
		r.rootMu.Unlock()
		r.report(importv2.AsIssue(err, importv2.SeverityObjectError, importv2.IssueObjectFailed))
		return
	}
	switch outcome.Action {
	case persist.ActionCreated:
		r.publishCreated(r.created.Add(1))
	case persist.ActionUpdated:
		r.updated.Add(1)
	default:
		r.skipped.Add(1)
	}
	// Counted for every outcome the pass FINISHED, skips included — as the
	// blended Step(1) did. A dormant run's poll cannot see the skips (they
	// write no ledger row, exactly as Result.Skipped is not durable and
	// restarts at zero), so a restarted run re-earns them: the numbers
	// converge, and neither surface ever invents one.
	r.countObject(w.object)
}

// persistGuarded is the per-object firewall: a panic in persist/resolve/store
// code fails only this object. The caller's file-future completion and failed
// accounting run on the returned error as for any ordinary persist failure.
func (r *run) persistGuarded(ctx context.Context, w work) (outcome persist.Outcome, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			outcome = persist.Outcome{}
			err = importv2.ObjectError(importv2.IssueInvariant, w.object.SourceKey, panicError("persist", rec))
		}
	}()
	return r.deps.Persister.Persist(ctx, w.object, w.target, r.report)
}

// stageInterrupted mirrors process()'s ctx-absorb for the finalize-stage
// calls that go to the persister directly: a ctx-shaped failure while the
// run is being stopped is the stop itself (finish classifies), never an
// objectFailed fatal — with the same fatal-severity carve-out.
func (r *run) stageInterrupted(ctx context.Context, err error) bool {
	return ctx.Err() != nil &&
		(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) &&
		importv2.AsIssue(err, importv2.SeverityObjectError, importv2.IssueObjectFailed).Severity < importv2.SeverityFatal
}

func (r *run) finalize(ctx context.Context, rootSpec importv2.RootSpec) {
	if r.resume != nil && r.resume.RootCollectionId != "" {
		// A previous incarnation completed finalize: reuse its collection.
		// Rebuilding would mint a SECOND one. NOTE the key is NOT reliably
		// fresh across incarnations — the adapter's date suffix has MINUTE
		// granularity, so a fast crash-restart re-claims the same source
		// key; the ledger's displacement machinery (synthetic rows) is what
		// keeps that safe, not key freshness (review Class B).
		r.rootCollectionId = r.resume.RootCollectionId
		return
	}
	if r.req.NoCollection {
		return
	}
	if rootSpec.RootObjectKey != "" {
		if id, ok := r.deps.Identity.Resolve(rootSpec.RootObjectKey); ok {
			r.rootCollectionId = id
		}
		return
	}
	r.rootMu.Lock()
	candidates := make([]string, 0, len(r.rootCandidates))
	for _, key := range r.rootCandidates {
		if _, failed := r.failedKeys[key]; !failed {
			candidates = append(candidates, key)
		}
	}
	r.rootMu.Unlock()
	if rootSpec.CollectionName == "" || len(candidates) == 0 || r.deps.Collection == nil {
		return
	}
	object, err := r.deps.Collection.MakeCollection(rootSpec.CollectionName, candidates)
	if err != nil {
		r.report(importv2.ObjectError(importv2.IssueObjectFailed, "", fmt.Errorf("make root collection: %w", err)))
		return
	}
	if err := r.deps.Identity.Claim(ctx, importv2.IdentityClaim{SourceKey: object.SourceKey, SbType: object.SbType}); err != nil {
		if r.stageInterrupted(ctx, err) {
			return
		}
		r.report(importv2.ObjectError(importv2.IssueObjectFailed, object.SourceKey, fmt.Errorf("claim root collection: %w", err)))
		return
	}
	// Write-ahead order, the finalize site of the P0-D rule: the claim must
	// be durable BEFORE the create it authorizes — a crash inside the
	// persist below would otherwise leave a tree with no ledger row at all
	// (the buffered claim was intent that never wrote), invisible to
	// compensation. finish()'s E4 flush is completeness insurance, not
	// ordering.
	if err := r.deps.Identity.FlushClaims(ctx); err != nil {
		if r.stageInterrupted(ctx, err) {
			return
		}
		r.report(importv2.ObjectError(importv2.IssueObjectFailed, object.SourceKey, fmt.Errorf("flush root collection claim: %w", err)))
		return
	}
	assignment, err := r.deps.Identity.Assign(object.SourceKey)
	if err != nil {
		r.report(importv2.ObjectError(importv2.IssueObjectFailed, object.SourceKey, fmt.Errorf("assign root collection: %w", err)))
		return
	}
	outcome, err := r.deps.Persister.Persist(ctx, object, persist.Target{
		Id:         assignment.Id,
		IsExisting: assignment.IsExisting,
		Payload:    assignment.Payload,
	}, r.report)
	if err != nil {
		if r.stageInterrupted(ctx, err) {
			return
		}
		r.report(importv2.AsIssue(err, importv2.SeverityObjectError, importv2.IssueObjectFailed))
		return
	}
	r.rootCollectionId = outcome.Id
}

// reconcileClaims is the completeness invariant (§16 item 4): on a run that
// finished without a fatal, every pass-1 claim must have arrived in pass 2 or
// carry a reported issue — a bare gap means a converter dropped an object
// silently, which is exactly the failure class the engine exists to forbid.
func (r *run) reconcileClaims() {
	if r.fatalIssue() != nil {
		// Aborted/cancelled runs legitimately strand claims.
		return
	}
	unassigned := r.deps.Identity.UnassignedClaims()
	sort.Strings(unassigned)
	for _, key := range unassigned {
		if key == report.SourceKey {
			// Claimed before finalize, emitted after this reconciliation.
			continue
		}
		r.issueMu.Lock()
		_, loud := r.issuedKeys[key]
		r.issueMu.Unlock()
		if loud {
			continue
		}
		if r.staleAcrossIncarnations(key) {
			// Expected drift on a crawl-resumed run, not a converter bug
			// (08-13 §5.4): the entity was claimed in a previous session,
			// never recorded, and this session did not find it again. Loud
			// but non-failing — the invariant's teeth stay for claims made
			// (or re-made) within the current incarnation. The wording
			// states only what the engine KNOWS (review P0-A): whether
			// non-re-enumeration proves deletion depends on the converter's
			// enumeration completeness — a converter that can re-fetch by
			// key does so via the recovery seam and reports its own precise
			// issue, which excludes the key from this fallback.
			r.report(importv2.Warning(importv2.IssueDataLoss, key,
				"object claimed by an interrupted import session was not found when the import resumed; it was not imported"))
			continue
		}
		r.failed.Add(1)
		r.report(importv2.ObjectError(importv2.IssueInvariant, key,
			fmt.Errorf("claimed object was never emitted by the converter")))
	}
}

// hasIssues reports whether the ledger holds anything worth a report page:
// at least one warning-or-worse issue (info-only runs are clean).
func (r *run) hasIssues() bool {
	r.issueMu.Lock()
	defer r.issueMu.Unlock()
	return r.loudIssues > 0 || r.issuesDropped > 0
}

// maybeClaimReport claims the report page before finalize so the root
// collection lists it next to the imported content. Claimed only when issues
// already exist — issues never shrink, so the report is then guaranteed to be
// emitted (a claim without a later object would be a reconciliation gap).
func (r *run) maybeClaimReport(ctx context.Context) bool {
	if r.resume != nil && r.resume.ReportObjectId != "" {
		// The report page persisted in a previous incarnation: reuse it.
		r.reportObjectId = r.resume.ReportObjectId
		return false
	}
	if !r.hasIssues() {
		return false
	}
	if err := r.deps.Identity.Claim(ctx, importv2.IdentityClaim{SourceKey: report.SourceKey, SbType: coresb.SmartBlockTypePage}); err != nil {
		r.report(importv2.Warning(importv2.IssueObjectFailed, report.SourceKey,
			fmt.Sprintf("claim import report: %s", err)))
		return false
	}
	// Write-ahead order (the P0-D rule, finalize site): durable before the
	// create it authorizes — see finalize.
	if err := r.deps.Identity.FlushClaims(ctx); err != nil {
		r.report(importv2.Warning(importv2.IssueObjectFailed, report.SourceKey,
			fmt.Sprintf("flush import report claim: %s", err)))
		return false
	}
	r.rootMu.Lock()
	r.rootCandidates = append(r.rootCandidates, report.SourceKey)
	r.rootMu.Unlock()
	return true
}

// emitReport builds and persists the report page from the final issue list
// (§16 item 1). Its own failures degrade to warnings: a report problem must
// never abort — or compensate away — an otherwise finished import.
func (r *run) emitReport(ctx context.Context, claimed bool, title string) {
	if r.resume != nil && r.resume.ReportObjectId != "" {
		return // reused from a previous incarnation (see maybeClaimReport)
	}
	if !r.hasIssues() {
		return
	}
	if !claimed {
		// Issues appeared only during finalize/reconciliation: claim late —
		// reconciliation already ran, and the object follows immediately.
		if err := r.deps.Identity.Claim(ctx, importv2.IdentityClaim{SourceKey: report.SourceKey, SbType: coresb.SmartBlockTypePage}); err != nil {
			r.report(importv2.Warning(importv2.IssueObjectFailed, report.SourceKey,
				fmt.Sprintf("claim import report: %s", err)))
			return
		}
		// Write-ahead order (the P0-D rule, finalize site).
		if err := r.deps.Identity.FlushClaims(ctx); err != nil {
			r.report(importv2.Warning(importv2.IssueObjectFailed, report.SourceKey,
				fmt.Sprintf("flush import report claim: %s", err)))
			return
		}
	}
	r.issueMu.Lock()
	issues := append([]importv2.Issue(nil), r.issues...)
	dropped := r.issuesDropped
	r.issueMu.Unlock()
	object := report.Build(title, issues, dropped, r.reportLookup)
	assignment, err := r.deps.Identity.Assign(object.SourceKey)
	if err != nil {
		r.report(importv2.Warning(importv2.IssueObjectFailed, object.SourceKey,
			fmt.Sprintf("assign import report: %s", err)))
		return
	}
	outcome, err := r.persistGuarded(ctx, work{object: object, target: persist.Target{
		Id:         assignment.Id,
		IsExisting: assignment.IsExisting,
		Payload:    assignment.Payload,
	}})
	if err != nil {
		if !r.stageInterrupted(ctx, err) {
			r.report(importv2.Warning(importv2.IssueObjectFailed, object.SourceKey,
				fmt.Sprintf("persist import report: %s", err)))
		}
		return
	}
	r.reportObjectId = outcome.Id
}

func (r *run) compensate() {
	switch r.compensateState {
	case 2:
		return
	case 1:
		// Re-entered through the panic guard: the pass did NOT complete.
		// A2: an incomplete compensation must report as leaked — a clean
		// zero here made finishRun drop the run dir while the run's objects
		// remained in the space (the evidence deleted itself).
		r.compensateState = 2
		r.leaked++
		r.issueMu.Lock()
		if len(r.issues) < importv2.IssueCap {
			r.issues = append(r.issues, importv2.Issue{
				Severity: importv2.SeverityWarning,
				Code:     importv2.IssueStoreError,
				Message:  "compensation aborted by a panic; the run dir is kept for the sweep to retry",
			})
		}
		r.issueMu.Unlock()
		return
	}
	r.compensateState = 1
	if r.deps.Journal.IsEmpty() {
		// Nothing to undo — an abort during passes 1–2, where compensation
		// is definitionally Drop() (DM spec §7). The zero-delete cleanup is
		// skipped TOGETHER WITH the OnCompensating marker, which exists so a
		// crash mid-cleanup is finished by the sweep — there is no cleanup
		// to finish, and the marker's durable state transition would scrub
		// the manifest's crawl request and burn the dir's crawl-resumable
		// class (DM-3 §8.3) over nothing. CompensationRan stays FALSE
		// (review P0-B): nothing ran, and the flag is consumed downstream as
		// disposal authority — setting it here destroyed the crawl artifact
		// on every non-transient mid-crawl failure. The vacuousness travels
		// as its own field; the adapter disposes only a user-cancelled
		// nothing-to-undo run.
		r.compensateState = 2
		r.nothingToUndo = true
		return
	}
	if r.deps.OnCompensating != nil {
		if err := r.deps.OnCompensating(); err != nil {
			// No durable marker, no deletes (see Deps.OnCompensating): keep
			// every effect and say so — CompensationRan stays false, so the
			// dir survives for the sweep to retry.
			r.compensateState = 2
			r.issueMu.Lock()
			if len(r.issues) < importv2.IssueCap {
				r.issues = append(r.issues, importv2.Issue{
					Severity: importv2.SeverityWarning,
					Code:     importv2.IssueStoreError,
					Message:  "compensation intent could not be recorded; cleanup deferred to the sweep",
					Err:      err,
				})
			}
			r.issueMu.Unlock()
			return
		}
	}
	r.compensationRan = true
	base := r.deps.ShutdownCtx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, compensationTimeout)
	defer cancel()
	result := r.deps.Journal.Compensate(ctx, r.deps.Objects)
	r.compensateState = 2
	r.compensated = result.Compensated
	r.leaked = result.Leaked
	r.issueMu.Lock()
	defer r.issueMu.Unlock()
	for _, issue := range result.Issues {
		if len(r.issues) < importv2.IssueCap {
			r.issues = append(r.issues, issue)
		} else {
			r.issuesDropped++
		}
	}
	for _, id := range result.Uncovered {
		if len(r.issues) < importv2.IssueCap {
			r.issues = append(r.issues, importv2.Issue{
				Severity: importv2.SeverityWarning,
				Code:     importv2.IssueDataLoss,
				ObjectId: id,
				Message:  "updated existing object was not rolled back (compensation covers created objects only)",
			})
		}
	}
}

func (r *run) buildResult(fatal importv2.Issue, rootSpec importv2.RootSpec) *importv2.Result {
	r.issueMu.Lock()
	issues := append([]importv2.Issue(nil), r.issues...)
	dropped := r.issuesDropped
	r.issueMu.Unlock()
	result := &importv2.Result{
		RootCollectionId: r.rootCollectionId,
		ReportObjectId:   r.reportObjectId,
		WidgetLayout:     rootSpec.WidgetLayout,
		Created:          r.created.Load(),
		Updated:          r.updated.Load(),
		Skipped:          r.skipped.Load(),
		Failed:           r.failed.Load(),
		Issues:           issues,
		IssuesDropped:    dropped,
		Compensated:      r.compensated,
		Leaked:           r.leaked,
		CompensationRan:  r.compensationRan,
		NothingToUndo:    r.nothingToUndo,
		Suspended:        r.suspendedRun,
		Cancelled:        r.cancelledRun,
	}
	if fatal.Code != "" {
		result.Err = fatal
	}
	return result
}

// classifyFatal decides whether a stage's failure IS the run's stop, by
// asking the run CONTEXT — the same discipline process(), stageInterrupted,
// stopFatal and runstore.IsCorrupted already follow. It used to read the
// error's shape instead, and matched anything wrapping
// context.DeadlineExceeded: including the Notion client's own
// http.Client{Timeout: time.Minute}, so a 60-second server hang reported
// itself as the user's cancel (review item 1). A ctx-shaped error on a LIVE
// context is a source failure like any other, and keeps its retryable shape
// for the transient-keep classification.
func classifyFatal(ctx context.Context, err error, fallback importv2.IssueCode) importv2.Issue {
	if ctx.Err() != nil {
		return importv2.Fatal(importv2.IssueCancelled, err)
	}
	return importv2.AsIssue(err, importv2.SeverityFatal, fallback)
}

// isDerivedClass delegates to the root predicate (shared with persist's
// write-ahead intent — the two must never disagree about the class).
func isDerivedClass(sbType coresb.SmartBlockType) bool {
	return importv2.IsDerivedClass(sbType)
}

// isFileClass delegates to the root predicate (shared with the crawl
// loader's claim/spool cross-check — the two must never disagree).
func isFileClass(sbType coresb.SmartBlockType) bool {
	return importv2.IsFileClass(sbType)
}

// registerRelationMeta feeds the format registry and key-adoption table from
// a relation definition, under both the emitted and the adopted key.
func (r *run) registerRelationMeta(object *importv2.Object, assignment identity.Assignment) {
	if object.SbType == coresb.SmartBlockTypeRelation {
		format := model.RelationFormat(object.Payload.Details.GetInt64(bundle.RelationKeyRelationFormat))
		if key := object.Payload.Key; key != "" {
			r.deps.Formats.Register(domain.RelationKey(key), format)
		}
		if assignment.InternalKey != "" {
			r.deps.Formats.Register(domain.RelationKey(assignment.InternalKey), format)
		}
	}
	sourceKey := object.Payload.Key
	if sourceKey != "" && assignment.InternalKey != "" && assignment.InternalKey != sourceKey {
		r.deps.Keys.Set(sourceKey, assignment.InternalKey)
	}
}
