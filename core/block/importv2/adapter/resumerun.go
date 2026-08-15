package adapter

import (
	"context"
	"fmt"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/engine"
	"github.com/anyproto/anytype-heart/core/block/importv2/identity"
	"github.com/anyproto/anytype-heart/core/block/importv2/persist"
	"github.com/anyproto/anytype-heart/core/block/importv2/resume"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

// resumeRun is the sweep's resume branch (DM spec §8.1): restart pass 3 of
// a fetched/materializing/suspended-mid-materialize run from its dir alone.
// Headless by construction — the manifest carries no request blob and no
// credentials (OQ2 stays avoided): everything pass 3 consumes is the
// ledger, the spool and the space. The store's ownership arrives from the
// sweep; every path out settles or keeps it via the run lifecycle rules.
func (s *service) resumeRun(ctx context.Context, store *runstore.Store, manifest runstore.Manifest) (outcome sweepOutcome) {
	outcome.Dir = store.Dir()
	// Attempts move durably BEFORE any work: a resume-and-crash loop is
	// bounded by the cap however early the crash lands. DETACHED (review
	// P2): this is the one prologue step that WRITES, two-phase — a Close
	// landing mid-write would tear it; on Background the small write
	// completes and the very next step (Load, on the live ctx) classifies
	// the stop calmly and refunds.
	manifest, err := store.BeginResume(context.Background())
	if err != nil {
		_ = store.Close()
		outcome.Action, outcome.Err = sweepSkippedError, fmt.Errorf("begin resume: %w", err)
		return outcome
	}
	state, err := resume.Load(ctx, store)
	if err != nil {
		return s.resumePrologueExit(ctx, store, outcome, store.RefundResumeAttempt, fmt.Errorf("load resume state: %w", err))
	}
	spc, err := s.spaceService.Get(ctx, manifest.SpaceId)
	if err != nil {
		return s.resumePrologueExit(ctx, store, outcome, store.RefundResumeAttempt, fmt.Errorf("get space: %w", err))
	}
	// The spool opens in the PROLOGUE: a transient failure here must keep
	// the dir via the skipped-error path (retry next start, attempts-capped)
	// — never flow into the delivery path as a fatal result for a run that
	// never started (review Class A, the spool-open sibling site).
	spool, err := store.Spool(ctx)
	if err != nil {
		return s.resumePrologueExit(ctx, store, outcome, store.RefundResumeAttempt, fmt.Errorf("open spool: %w", err))
	}

	request := importv2.Request{
		SpaceID:        manifest.SpaceId,
		Origin:         objectorigin.Import(model.ImportType(manifest.ImportType)),
		Mode:           importv2.Mode(manifest.Mode),
		UpdateExisting: manifest.UpdateExisting,
		NoCollection:   manifest.NoCollection,
	}
	// The wire-facing request is reconstructed minimally: notification and
	// event delivery need the type and space, nothing else survives the
	// restart (and nothing else was stored).
	wireReq := &pb.RpcObjectImportRequest{
		SpaceId:               manifest.SpaceId,
		Type:                  model.ImportType(manifest.ImportType),
		UpdateExistingObjects: manifest.UpdateExisting,
	}

	progress := s.setupProgress(wireReq)
	progressSettled := false
	defer func() {
		if !progressSettled {
			progress.Finish(fmt.Errorf("import resume aborted"))
			s.fileSync.ClearImportEvents()
		}
	}()
	runCtx, cancel := context.WithCancelCause(s.componentCtx)
	defer cancel(nil)
	handle := s.registerRun(cancel) // Close suspends a mid-flight resume like any run
	defer s.unregisterRun(handle)
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-progress.Canceled():
			cancel(nil) // user cancel: plain cause — the engine compensates
		case <-watchDone:
		}
	}()

	// The resumed run's surface starts where its predecessor stopped, from
	// the SAME reads the dormant poll of this dir performs (§15.4's
	// right-hand column): the spool census as the pass-3 denominators, the
	// ledger's terminal rows as its numerators, the ledger's object count as
	// the cancel affordance. Without it the whole rehydration window — the
	// load above, the identity rehydration and the engine's own start —
	// reported the emitter's zero value, which reads as "Scanning, nothing
	// added yet" for a run about to compensate everything it made. Advisory,
	// like every census read: a failure costs telemetry only.
	seed := statSeed{issues: state.Engine.Issues, created: state.Engine.Created}
	if pages, files, _, censusErr := spool.Census(ctx); censusErr == nil {
		seed.pagesTotal, seed.filesTotal = int64(pages), int64(files)
		seed.pagesDone, seed.filesDone = state.PagesDone, state.FilesDone
	}
	lc := s.newLifecycle(store, manifest, progress, pageRateCeilingFor(model.ImportType(manifest.ImportType)), seed)
	defer lc.release()
	result := s.resumeEngine(runCtx, request, spc, lc, progress, state, spool)
	if result.Suspended {
		// An orderly suspend refunds its attempt (review Class F): the cap
		// bounds CRASH loops, and a crash never reaches this settlement
		// path. Before finishRun — it closes the store.
		if err := store.RefundResumeAttempt(context.Background()); err != nil {
			log.Errorf("refund resume attempt: %s", err)
		}
	}
	s.finishRun(lc, result)

	outcome.Result = persist.CompensationResult{Compensated: result.Compensated, Leaked: result.Leaked}
	s.settleRun(wireReq, progress, result)
	progressSettled = true
	switch {
	case result.Suspended:
		outcome.Action = sweepResumedSuspended
	case result.Err != nil:
		outcome.Action = sweepResumedFailed
		outcome.Err = result.Err
	default:
		outcome.Action = sweepResumedCompleted
		if result.RootCollectionId != "" {
			// checkDuplicatedTarget makes this idempotent across a crash
			// that landed between the collection and its widget.
			s.createRootWidget(spc.DerivedIDs().Widgets, result)
		}
	}
	return outcome
}

// resumePrologueExit settles a resume that failed before the engine
// started. The stop classification reaches this exit like every other
// (review Class G, the fourth-strike shape: a Load iterator dying on a
// closing component surfaces sqlite 'interrupted', not a ctx error): a
// shutdown-shaped exit is CALM — attempt refunded (zero work was done,
// review Class F), dir kept, resumed-suspended — while a genuine failure
// keeps the spent attempt so the cap still routes a never-loading dir to
// compensation. refund is the caller's OWN counter (review P1: the two
// resume classes budget separately) — RefundResumeAttempt for the pass-3
// branch, RefundCrawlResumeAttempt for the crawl branch.
func (s *service) resumePrologueExit(ctx context.Context, store *runstore.Store, outcome sweepOutcome, refund func(context.Context) error, err error) sweepOutcome {
	if ctx.Err() != nil {
		if refundErr := refund(context.Background()); refundErr != nil {
			log.Errorf("refund resume attempt: %s", refundErr)
		}
		if flushErr := store.Flush(context.Background()); flushErr != nil {
			log.Errorf("flush after prologue suspend: %s", flushErr)
		}
		_ = store.Close()
		log.With("dir", outcome.Dir).Warnf("resume interrupted by shutdown before it started: %s", err)
		outcome.Action = sweepResumedSuspended
		return outcome
	}
	_ = store.Close()
	outcome.Action, outcome.Err = sweepSkippedError, err
	return outcome
}

// resumeEngine wires the resumed engine run: the same per-run components as
// runEngine, plus the rehydrated identity, the heal policy and the resumed
// progress total.
func (s *service) resumeEngine(ctx context.Context, request importv2.Request, spc clientspace.Space, lc *runLifecycle, progress process.Progress, st *resume.State, spool engine.Spool) *importv2.Result {
	deps, persister := s.engineDeps(request, spc, lc, progress,
		[]identity.Option{resume.ClaimLedgerOption(lc.store), st.IdentityOption()})
	deps.Spool = spool
	persister.SetResumeHeal(st.Heal())
	st.SeedJournal(deps.Journal)
	// The resumed run's progress total is no longer set here: the engine
	// re-bases it from the spool census at the start of pass 3, on this path
	// and the fresh one alike (§15.4's one derivation).
	return engine.Resume(ctx, request, deps, &st.Engine)
}
