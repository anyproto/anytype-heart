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
	"sync"
	"sync/atomic"
	"time"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/identity"
	"github.com/anyproto/anytype-heart/core/block/importv2/persist"
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
	Assign(sourceKey string) (identity.Assignment, error)
	AssignDerived(ctx context.Context, o *importv2.Object) (identity.Assignment, error)
	RegisterFile(sourceKey string)
	CompleteFile(sourceKey, id string, err error)
	Resolve(sourceKey string) (id string, ok bool)
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
	}
	defer func() {
		if rec := recover(); rec != nil {
			r.report(importv2.Fatal(importv2.IssueInvariant, panicError("import run", rec)))
			r.compensate()
			result = r.buildResult(*r.fatalIssue(), importv2.RootSpec{})
		}
	}()

	if fatal := r.identityPass(runCtx, converter); fatal != nil {
		return r.buildResult(*fatal, importv2.RootSpec{})
	}
	rootSpec := r.streamPass(runCtx, converter)

	if fatal := r.fatalIssue(); fatal != nil {
		r.compensate()
		return r.buildResult(*fatal, importv2.RootSpec{})
	}
	r.finalize(runCtx, rootSpec)
	if fatal := r.fatalIssue(); fatal != nil {
		r.compensate()
		return r.buildResult(*fatal, importv2.RootSpec{})
	}
	result = r.buildResult(importv2.Issue{}, rootSpec)
	result.Err = nil
	return result
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

	issueMu       sync.Mutex
	issues        []importv2.Issue
	issuesDropped int64
	fatal         *importv2.Issue

	// rootCandidates is recorded in stream order (sink side) so membership
	// stays deterministic; failed objects are filtered out at finalize.
	rootMu         sync.Mutex
	rootCandidates []string
	failedKeys     map[string]struct{}

	rootCollectionId string
	compensated      int
	leaked           int
}

// report is the single issue funnel: collects (capped) and applies the one
// abort predicate.
func (r *run) report(issue importv2.Issue) {
	r.issueMu.Lock()
	if len(r.issues) < importv2.IssueCap {
		r.issues = append(r.issues, issue)
	} else {
		r.issuesDropped++
	}
	abort := importv2.ShouldAbort(issue.Severity, r.req.Mode) && r.fatal == nil
	if abort {
		fatal := issue
		r.fatal = &fatal
	}
	r.issueMu.Unlock()
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
	r.deps.Reporter.AddTotal(count)
	return nil
}

func (r *run) streamPass(ctx context.Context, converter importv2.Converter) importv2.RootSpec {
	r.deps.Reporter.Phase("Importing objects")
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

func (r *run) finalize(ctx context.Context, rootSpec importv2.RootSpec) {
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
		r.report(importv2.AsIssue(err, importv2.SeverityObjectError, importv2.IssueObjectFailed))
		return
	}
	r.rootCollectionId = outcome.Id
}

func (r *run) compensate() {
	ctx, cancel := context.WithTimeout(context.Background(), compensationTimeout)
	defer cancel()
	result := r.deps.Journal.Compensate(ctx, r.deps.Objects)
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
		WidgetLayout:     rootSpec.WidgetLayout,
		Created:          r.created.Load(),
		Updated:          r.updated.Load(),
		Skipped:          r.skipped.Load(),
		Failed:           r.failed.Load(),
		Issues:           issues,
		IssuesDropped:    dropped,
		Compensated:      r.compensated,
		Leaked:           r.leaked,
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
