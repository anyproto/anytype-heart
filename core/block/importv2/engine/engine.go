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

// Reporter is the rich internal progress seam; the adapter down-projects it
// onto process.Progress. Implementations must be safe for concurrent use.
type Reporter interface {
	Phase(name string)
	AddTotal(delta int64)
	Step(delta int64)
}

type noopReporter struct{}

func (noopReporter) Phase(string)   {}
func (noopReporter) AddTotal(int64) {}
func (noopReporter) Step(int64)     {}

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
func Run(ctx context.Context, req importv2.Request, converter importv2.Converter, deps Deps) (result *importv2.Result) {
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
		failedKeys: map[string]struct{}{},
		issuedKeys: map[string]struct{}{},
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
	r.streamPass(runCtx, &spoolReplayConverter{spool: spool})
	if r.fatalIssue() != nil || runCtx.Err() != nil {
		return r.finish(runCtx, importv2.RootSpec{})
	}
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
		r.report(importv2.Fatal(importv2.IssueInvariant, fmt.Errorf("resume requires the run's durable spool")))
		return r.finish(runCtx, importv2.RootSpec{})
	}
	r.created.Store(state.Created)
	r.updated.Store(state.Updated)
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
	allStagesDone    bool
	// suspendedRun records the engine's own verdict — the run stopped for a
	// shutdown suspend and was NOT compensated — carried out via
	// Result.Suspended so the adapter never re-derives it from a context.
	suspendedRun bool
	// resume is non-nil on a resumed incarnation (engine.Resume): the sink
	// consults its skip set, finalize and the report consult its recorded
	// outcomes.
	resume *ResumeState
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
	r.deps.Reporter.Phase("Scanning source")
	count := int64(0)
	err := converter.EnumerateIdentities(ctx, func(claim importv2.IdentityClaim) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.deps.Identity.Claim(ctx, claim); err != nil {
			return err
		}
		count++
		return nil
	})
	if err != nil {
		issue := classifyFatal(err, importv2.IssueSourceInvalid)
		return &issue
	}
	if count == 0 {
		issue := importv2.Fatal(importv2.IssueNoObjects, fmt.Errorf("source contains no importable objects"))
		return &issue
	}
	if err := r.deps.Identity.FlushClaims(ctx); err != nil {
		issue := classifyFatal(err, importv2.IssueStoreError)
		return &issue
	}
	r.deps.Reporter.AddTotal(count)
	return nil
}

func (r *run) streamPass(ctx context.Context, converter importv2.Converter) importv2.RootSpec {
	r.deps.Reporter.Phase("Creating objects")
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
		r.report(classifyFatal(convertErr, importv2.IssueSourceInvalid))
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
		r.created.Add(1)
	case persist.ActionUpdated:
		r.updated.Add(1)
	default:
		r.skipped.Add(1)
	}
	r.deps.Reporter.Step(1)
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
	r.deps.Reporter.Phase("Finalizing")
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
	}
	r.issueMu.Lock()
	issues := append([]importv2.Issue(nil), r.issues...)
	dropped := r.issuesDropped
	r.issueMu.Unlock()
	object := report.Build(title, issues, dropped, r.deps.Identity.Resolve)
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
		Suspended:        r.suspendedRun,
	}
	if fatal.Code != "" {
		result.Err = fatal
	}
	return result
}

func classifyFatal(err error, fallback importv2.IssueCode) importv2.Issue {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return importv2.Fatal(importv2.IssueCancelled, err)
	}
	return importv2.AsIssue(err, importv2.SeverityFatal, fallback)
}

// isDerivedClass reports whether the object's identity derives from a unique
// key on demand (never claimed in pass 1).
func isDerivedClass(sbType coresb.SmartBlockType) bool {
	switch sbType {
	case coresb.SmartBlockTypeRelation,
		coresb.SmartBlockTypeRelationOption,
		coresb.SmartBlockTypeObjectType,
		coresb.SmartBlockTypeProfilePage:
		return true
	default:
		return false
	}
}

func isFileClass(sbType coresb.SmartBlockType) bool {
	return sbType == coresb.SmartBlockTypeFileObject || sbType == coresb.SmartBlockTypeFile
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
