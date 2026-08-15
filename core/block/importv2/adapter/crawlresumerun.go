package adapter

import (
	"context"
	"encoding/json"
	"fmt"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/engine"
	"github.com/anyproto/anytype-heart/core/block/importv2/identity"
	"github.com/anyproto/anytype-heart/core/block/importv2/markdown"
	notionclient "github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/core/block/importv2/persist"
	"github.com/anyproto/anytype-heart/core/block/importv2/resume"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/block/importv2/source"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// resumeCrawlRun is the sweep's pass-2 crawl-resume branch (DM spec §8.3):
// a run interrupted mid-crawl re-runs both passes against the live source,
// with the spool as the skip set and the converter rebuilt from the
// manifest's stored request — the one resume class that needs the source
// and its credentials (OQ2's scope). Store ownership arrives from the
// sweep; every path out settles or keeps it via the run lifecycle rules.
func (s *service) resumeCrawlRun(ctx context.Context, store *runstore.Store, manifest runstore.Manifest) (outcome sweepOutcome) {
	outcome.Dir = store.Dir()
	// Attempts move durably BEFORE any work (the resumeRun rule): a
	// resume-and-crash loop is bounded by the cap however early the crash
	// lands. Detached for the same reason as its sibling.
	manifest, err := store.BeginCrawlResume(context.Background())
	if err != nil {
		_ = store.Close()
		outcome.Action, outcome.Err = sweepSkippedError, fmt.Errorf("begin crawl resume: %w", err)
		return outcome
	}
	st, err := resume.LoadCrawl(ctx, store)
	if err != nil {
		return s.resumePrologueExit(ctx, store, outcome, store.RefundCrawlResumeAttempt, fmt.Errorf("load crawl state: %w", err))
	}
	wireReq := &pb.RpcObjectImportRequest{}
	if err = wireReq.Unmarshal(st.Manifest.Request); err != nil {
		return s.resumePrologueExit(ctx, store, outcome, store.RefundCrawlResumeAttempt, fmt.Errorf("decode stored request: %w", err))
	}
	var preset *schemaplan.Plan
	if len(st.PlanJSON) > 0 {
		preset = &schemaplan.Plan{}
		if err = json.Unmarshal(st.PlanJSON, preset); err != nil {
			return s.resumePrologueExit(ctx, store, outcome, store.RefundCrawlResumeAttempt, fmt.Errorf("decode recorded plan: %w", err))
		}
	}
	// Markdown inputs parse in the prologue: a broken request never reaches
	// the engine; a moved source keeps the dir for retry (attempts-capped —
	// a USB drive may return; exhaustion routes to compensation, the §6.1
	// source-unavailable disposition).
	importType := model.ImportType(manifest.ImportType)
	var paths []string
	var params mdParams
	var src source.Source
	switch importType {
	case model.Import_Notion:
		if wireReq.GetNotionParams().GetApiKey() == "" {
			return s.resumePrologueExit(ctx, store, outcome, store.RefundCrawlResumeAttempt, fmt.Errorf("stored request carries no api key"))
		}
	case model.Import_Markdown, model.Import_Obsidian:
		if paths, params, err = markdownParams(wireReq); err != nil {
			return s.resumePrologueExit(ctx, store, outcome, store.RefundCrawlResumeAttempt, fmt.Errorf("stored request: %w", err))
		}
		params.Planner = plannerFromRequest(wireReq)
		if manifest.PathIndex >= len(paths) {
			return s.resumePrologueExit(ctx, store, outcome, store.RefundCrawlResumeAttempt, fmt.Errorf("stored request has %d paths, run is path %d", len(paths), manifest.PathIndex))
		}
		if src, err = source.Open(paths[manifest.PathIndex]); err != nil {
			return s.resumePrologueExit(ctx, store, outcome, store.RefundCrawlResumeAttempt, fmt.Errorf("open source: %w", err))
		}
		defer src.Close()
	default:
		return s.resumePrologueExit(ctx, store, outcome, store.RefundCrawlResumeAttempt, fmt.Errorf("import type %s has no crawl resume", importType))
	}
	spc, err := s.spaceService.Get(ctx, manifest.SpaceId)
	if err != nil {
		return s.resumePrologueExit(ctx, store, outcome, store.RefundCrawlResumeAttempt, fmt.Errorf("get space: %w", err))
	}
	spool, err := store.Spool(ctx)
	if err != nil {
		return s.resumePrologueExit(ctx, store, outcome, store.RefundCrawlResumeAttempt, fmt.Errorf("open spool: %w", err))
	}

	request := importv2.Request{
		SpaceID:        manifest.SpaceId,
		Origin:         objectorigin.Import(importType),
		Mode:           importv2.Mode(manifest.Mode),
		UpdateExisting: manifest.UpdateExisting,
		NoCollection:   manifest.NoCollection,
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

	lc := s.newLifecycle(store, manifest, progress, pageRateCeilingFor(importType), st.Engine.Issues)
	defer lc.release()
	// The converter is rebuilt from the stored request through the SAME
	// builders the fresh run uses; the plan reuse carries the recording (a
	// run that never completed its plan phase records afresh — its spool is
	// empty by construction, so replanning is safe).
	reuse := schemaplan.Reuse{Preset: preset, Record: s.planRecorder(lc)}
	var converter importv2.Converter
	if importType == model.Import_Notion {
		converter = s.notionConverter(wireReq, lc, reuse)
	} else {
		converter = markdown.New(src, s.markdownParamsFor(params, lc, reuse),
			&collectionFactory{service: s.collectionService})
	}

	deps, _ := s.engineDeps(request, spc, lc, progress,
		[]identity.Option{resume.ClaimLedgerOption(lc.store), st.IdentityOption()})
	deps.Spool = spool
	result := engine.ResumeCrawl(runCtx, request, converter, deps, &st.Engine)

	if result.Suspended {
		// An orderly suspend refunds its attempt (review Class F) — the cap
		// bounds CRASH loops. Before finishRun, which closes the store.
		if err := store.RefundCrawlResumeAttempt(context.Background()); err != nil {
			log.Errorf("refund crawl resume attempt: %s", err)
		}
		s.finishRun(lc, result)
		s.settleRun(wireReq, progress, result)
		progressSettled = true
		outcome.Action = sweepResumedSuspended
		return outcome
	}
	if s.transientCrawlFailure(store, result) {
		// Offline laptop, Notion outage, exhausted rate budget: the crawl
		// artifact must survive the condition that interrupted it — the
		// attempt is REFUNDED (review P1: with it spent, four offline app
		// starts destroyed a two-hour crawl; the cap bounds crash loops and
		// genuine failures, which never reach this settlement), the dir
		// stays exactly as the engine left it (state running, request
		// intact — the empty-journal compensation skip guarantees no
		// compensating transition fired), and the next start retries.
		if err := store.RefundCrawlResumeAttempt(context.Background()); err != nil {
			log.Errorf("refund crawl resume attempt: %s", err)
		}
		lc.settled = true
		lc.settleTracking()
		if err := store.Close(); err != nil {
			log.Errorf("close kept crawl run: %s", err)
		}
		log.With("dir", outcome.Dir).Warnf("crawl resume hit a transient failure; dir kept for the next start: %s", result.Err)
		progress.Finish(result.Err)
		s.fileSync.ClearImportEvents()
		progressSettled = true
		outcome.Action, outcome.Err = sweepSkippedError, result.Err
		return outcome
	}

	// A multi-path markdown request resumed at path k finishes the
	// remaining paths as fresh runs (08-13 §6.2 item 1), sharing
	// executeMarkdown's combine rules. The resumed run's own dir settles
	// first — each continuation path owns its own dir.
	combined := &importv2.Result{}
	stop := combinePathResult(combined, result)
	s.finishRun(lc, result)
	if !stop && len(paths) > manifest.PathIndex+1 {
		s.runMarkdownPaths(runCtx, request, wireReq, spc, progress, paths, params, manifest.PathIndex+1, combined)
	}

	outcome.Result = persist.CompensationResult{Compensated: combined.Compensated, Leaked: combined.Leaked}
	s.settleRun(wireReq, progress, combined)
	progressSettled = true
	switch {
	case combined.Suspended:
		outcome.Action = sweepResumedSuspended // a continuation path met Close
	case combined.Err != nil:
		outcome.Action = sweepResumedFailed
		outcome.Err = combined.Err
	default:
		outcome.Action = sweepResumedCompleted
		if combined.RootCollectionId != "" {
			s.createRootWidget(spc.DerivedIDs().Widgets, combined)
		}
	}
	return outcome
}

// userCancelled reports a failed run the USER stopped — the one stop that
// means "discard this import" rather than "finish it later". It reads the
// engine's STOP SOURCE (Result.Cancelled), never the fatal's code: a code is
// a shape, and it lied in both directions (review item 1). A cancelled
// Notion call is retryable-SHAPED — "retries exhausted" wrapping a transport
// context.Canceled — so a retryability test alone kept a cancelled import's
// dir, token intact, for the next start to silently re-run (review P0-C);
// and a transport DEADLINE — the client's own http.Client{Timeout:
// time.Minute} — wore the cancel's code, so reading the code let a
// 60-second server hang delete a two-hour crawl. A suspend is not a cancel
// (its cause is ErrSuspended), and the engine says so on both fields;
// Suspended is consulted first anyway, belt to the engine's braces.
func userCancelled(result *importv2.Result) bool {
	return result.Err != nil && !result.Suspended && result.Cancelled
}

// transientCrawlFailure reports a crawl-resume failure worth the QUIET keep
// (no failure notification, dir untouched): the stop is
// consulted first (userCancelled — a cancel is never transient however
// retryable its wrap looks), then transient-shaped by the Notion client's
// OWN retryability rule (one classification, not a parallel list), on a run
// still in the crawl-resumable state — the belt: a post-materialize or
// compensated failure must settle normally, never dodge its disposal.
// Non-transient failures are not the destructive complement anymore: they
// settle loudly through finishRun, whose empty-journal rule keeps the crawl
// artifact for the sweep regardless of shape (review P0-B).
func (s *service) transientCrawlFailure(store *runstore.Store, result *importv2.Result) bool {
	if result.Err == nil || userCancelled(result) || !notionclient.IsRetryable(result.Err) {
		return false
	}
	m, merr := store.Manifest(context.Background())
	return merr == nil && !m.MaterializeStarted &&
		m.State == runstore.StateRunning && len(m.Request) > 0
}
