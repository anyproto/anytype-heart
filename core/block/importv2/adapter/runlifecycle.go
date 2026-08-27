package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/identity"
	notionclient "github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/core/block/importv2/resume"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/core/block/importv2/schemaplan"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/vcs"
)

// runLifecycle owns one engine run's durable state: the per-run store (with
// the effect ledger the journal writes through) and the spill dir, which
// lives inside the run dir so spilled file bytes share the run's lifetime.
// In volatile mode (no repo path — test environments) there is
// no store and the spill dir is an OS temp dir removed on finish, as before.
type runLifecycle struct {
	store    *runstore.Store
	spillDir string
	cleanup  func()
	settled  bool
	// stats is the run's statistic emitter — never nil, so every call
	// site can report unconditionally. It is the push producer AND what the
	// pull RPCs serve for this run while it is live.
	stats *statEmitter
	// untrack removes the run from the live-status registry (runstatus.go);
	// called on every settlement path exactly once via settleTracking.
	untrack func()
	// seedTotal is the resumed run's pass-3 denominator, kept here so the
	// LEGACY scalar can be seeded from the same census the emitter is
	// (engineDeps). Zero for a fresh run and a crawl resume, whose
	// denominators are still being discovered.
	seedTotal int64
	// kept records the DISPOSAL verdict finishRun reached: the dir survived
	// this settlement, so something will look at it again. Recorded rather
	// than re-derived, because the predicate that decides it is finishRun's
	// own ladder and a second copy of it would drift (review item 14 needs
	// the answer; discardable and the invariant branches produce it).
	kept bool
}

// newLifecycle is the ONE construction of a run's lifecycle handle, shared
// by the fresh run and both resume branches. It exists because the live
// registry hold used to be wired at three sites and the statistic emitter
// would have made that four — the recurring shape where a rule holds in one
// sibling and not in the next.
func (s *service) newLifecycle(store *runstore.Store, manifest runstore.Manifest, progress process.Progress, ceiling float64, seed statSeed) *runLifecycle {
	fetching, materializing := safeToCloseFor(manifest)
	send := func(*pb.EventImportStatistic) {}
	if s.eventSender != nil {
		send = func(status *pb.EventImportStatistic) {
			s.eventSender.Broadcast(event.NewEventSingleMessage("",
				&pb.EventMessageValueOfImportStatistic{ImportStatistic: status}))
		}
	}
	lc := &runLifecycle{
		store: store,
		stats: newStatEmitter(statConfig{
			importId:                 manifest.RunId,
			processId:                progressId(progress),
			importType:               model.ImportType(manifest.ImportType),
			send:                     send,
			now:                      time.Now,
			pageRateCeiling:          ceiling,
			safeToCloseFetching:      fetching,
			safeToCloseMaterializing: materializing,
		}),
	}
	// A resumed run inherits its predecessors' numbers here, at the one
	// construction site, rather than at each resume branch (see statSeed).
	// The pass-boundary half is derived from the manifest rather than
	// passed: it is the SAME sticky marker the dormant surface reads, so a
	// run cannot be mid-materialize when polled and mid-scan when pushed.
	seed.materializing = manifest.MaterializeStarted
	lc.stats.Seed(seed)
	lc.seedTotal = seed.pagesTotal + seed.filesTotal
	if store != nil {
		lc.spillDir = store.SpillDir()
		lc.untrack = s.trackLive(manifest.RunId, store, lc)
	}
	return lc
}

func progressId(progress process.Progress) string {
	if progress == nil {
		return ""
	}
	return progress.Id()
}

// safeToCloseFor evaluates the SWEEP's own resume predicates for the state
// this run will be dormant in if the app closes during each phase: mid-crawl
// the dir is `running` with its request held, mid-materialize it is
// `materializing`. Same functions, same attempt caps — so the surface and
// the sweep's actual behavior cannot drift apart, which is the exact defect
// the DM-3 fix round found in the pull side of this field.
//
// Evaluated once per run because the inputs move only at lifecycle
// transitions: a mid-run poll must not cost a manifest read per event.
func safeToCloseFor(m runstore.Manifest) (fetching, materializing bool) {
	crawl := m
	crawl.State = runstore.StateRunning
	crawl.MaterializeStarted = false
	fetching = crawlResumable(crawl) && crawl.CrawlResumeAttempts < maxResumeAttempts

	mat := m
	mat.State = runstore.StateMaterializing
	mat.MaterializeStarted = true
	materializing = resumable(mat) && mat.ResumeAttempts < maxResumeAttempts
	return fetching, materializing
}

// settleTracking releases the run's live-registry hold and flushes its
// terminal event, on EVERY settlement path (release, finishRun and the
// crawl-resume's transient keep all come through here). The VERDICT travels
// with it (review item 11): the emitter's own hooks only ever speak for the
// transport, so without it the last word of a failed import was a pacer
// window or a retry attempt.
func (lc *runLifecycle) settleTracking(verdict statVerdict) {
	if lc.untrack != nil {
		lc.untrack()
		lc.untrack = nil
	}
	if lc.stats != nil {
		lc.stats.Close(verdict)
	}
}

// release is DEFERRED by the run owner immediately after beginRun
// (Invariant 3): if finishRun never ran — a panic between beginRun and
// finishRun — the store is still closed, so the active-dir registry entry
// cannot leak and block the dir from ever being swept. The dir itself is
// kept: an unsettled run is exactly what the sweep exists to settle.
func (lc *runLifecycle) release() {
	if lc.settled {
		return
	}
	lc.settled = true
	// A run that reached here never settled — the owner panicked between
	// beginRun and finishRun. Its outcome is unknown, but it certainly did
	// not finish, and the surface saying so beats a terminal Running that
	// never moves again.
	lc.settleTracking(statVerdict{failed: true, message: "import run ended without settling"})
	if lc.store != nil {
		if err := lc.store.Close(); err != nil {
			log.Errorf("release unsettled run store: %s", err)
		}
		// stats.Close already flushed the terminal event; the loggable
		// rendering (codes and counts, never content) says how far the
		// abandoned run got, which is otherwise unknowable until the sweep
		// opens the dir.
		log.With("dir", lc.store.Dir(), "stats", lc.stats.stateForLog()).
			Errorf("import run was abandoned without settling; dir left for the startup sweep")
		return
	}
	if lc.cleanup != nil {
		lc.cleanup()
	}
}

// beginRun creates the run dir + store before the engine starts. A store
// creation failure fails the run (a run that cannot journal must
// not create objects). The serialized wire request rides the manifest so a
// crawl interrupted mid-pass-2 can rebuild its converter on the next start
// (stored as-is, scrubbed by the store on
// every transition out of the crawl-resumable states).
func (s *service) beginRun(ctx context.Context, request importv2.Request, wireReq *pb.RpcObjectImportRequest, converterName string, pathIndex int, progress process.Progress) (*runLifecycle, error) {
	ceiling := pageRateCeilingFor(request.Origin.ImportType)
	if s.config.RepoPath == "" {
		spillDir, err := os.MkdirTemp("", "anytype-import-v2-*")
		if err != nil {
			return nil, fmt.Errorf("create spill dir: %w", err)
		}
		// Volatile mode still emits: the statistic is the run's progress
		// surface, and a run without a durable dir has no importId to poll
		// by — which the empty id says honestly, rather than by silence.
		lc := s.newLifecycle(nil, runstore.Manifest{ImportType: int64(request.Origin.ImportType)}, progress, ceiling, statSeed{})
		lc.spillDir = spillDir
		lc.cleanup = func() { _ = os.RemoveAll(spillDir) }
		return lc, nil
	}
	requestBlob, err := wireReq.Marshal()
	if err != nil {
		// The run can proceed — only the crawl-resume class is lost. Loud:
		// this should never happen for a request that already crossed the
		// wire.
		log.Errorf("serialize import request for the run manifest: %s", err)
		requestBlob = nil
	}
	runId := bson.NewObjectId().Hex()
	manifest := runstore.Manifest{
		RunId:          runId,
		SpaceId:        request.SpaceID,
		ImportType:     int64(request.Origin.ImportType),
		Mode:           int64(request.Mode),
		UpdateExisting: request.UpdateExisting,
		NoCollection:   request.NoCollection,
		PathIndex:      pathIndex,
		Converter:      converterName,
		AppVersion:     vcs.GetVCSInfo().Version(),
		Request:        requestBlob,
	}
	store, err := runstore.Create(ctx, filepath.Join(runstore.RunsRoot(s.config.RepoPath), runId), manifest)
	if err != nil {
		return nil, fmt.Errorf("create run store: %w", err)
	}
	// Create stamps the schema version; safeToCloseFor's predicates gate on
	// it, so read back what was written rather than the caller's struct.
	if written, readErr := store.Manifest(ctx); readErr == nil {
		manifest = written
	}
	return s.newLifecycle(store, manifest, progress, ceiling, statSeed{}), nil
}

// pageRateCeilingFor is the fastest the SOURCE can yield pages, per import
// type — the input the fetching ETA is capped by. Only Notion has a
// documented one; a local markdown tree is bounded by disk, which is not a
// number this may promise anything about.
func pageRateCeilingFor(importType model.ImportType) float64 {
	if importType == model.Import_Notion {
		return notionclient.PageRateCeiling()
	}
	return 0
}

// planRecorder persists a fresh run's sanitized structure plan to the run
// kv (a resumed crawl reuses the recording, never replans). nil
// in volatile mode — schemaplan.Resolve treats a nil recorder as no-op.
func (s *service) planRecorder(lc *runLifecycle) func(schemaplan.Plan) error {
	if lc.store == nil {
		return nil
	}
	return func(plan schemaplan.Plan) error {
		data, err := json.Marshal(plan)
		if err != nil {
			return fmt.Errorf("marshal structure plan: %w", err)
		}
		// Detached: the plan write is journaling (its loss would make a
		// resumed crawl replan divergently), same discipline as every
		// ledger write.
		return lc.store.SetPlanJSON(context.Background(), data)
	}
}

// finishRun settles a run's durable state. A run the ENGINE says was
// suspended (Result.Suspended — the single source of that verdict; it means
// compensation was skipped) keeps its dir, marked suspended and flushed for
// the startup sweep. Every other outcome is disposed whole: terminal
// state, then delete the dir. The state write is insurance — if Drop fails,
// the sweep sees a terminal manifest and just deletes the dir. State writes
// run on a background context: the run ctx is typically already cancelled
// on the failure path.
func (s *service) finishRun(lc *runLifecycle, result *importv2.Result) {
	lc.settled = true
	lc.settleTracking(verdictOf(result))
	if lc.store == nil {
		if lc.cleanup != nil {
			lc.cleanup()
		}
		return
	}
	if result.Suspended {
		ctx := context.Background()
		if err := lc.store.SetState(ctx, runstore.StateSuspended); err != nil {
			log.Errorf("mark run suspended: %s", err)
		}
		if err := lc.store.Flush(ctx); err != nil {
			log.Errorf("flush suspended run: %s", err)
		}
		if err := lc.store.Close(); err != nil {
			log.Errorf("close suspended run: %s", err)
		}
		lc.kept = true
		log.With("dir", lc.store.Dir()).Warnf("import run suspended for shutdown; state kept for the startup sweep")
		return
	}
	if result.Err != nil && !result.CompensationRan && !discardable(lc, result) {
		// The disposal invariant (review Class A): a failure whose effects no
		// compensation covered must not destroy the dir — it is the only
		// record of what was created. Keep it EXACTLY as it is (no state
		// change): the sweep decides — resume (attempts-capped) or
		// compensate. Covers prologue failures (spool open, load), the
		// engine's nil-spool guard, a gated-out compensation, and (review
		// P0-B) every mid-crawl abort with an empty journal — the crawl
		// artifact survives whatever interrupted it, structurally, not by a
		// retryability allowlist. The ONE exception is the user's cancel
		// with nothing to undo whose pass 3 never began (discardable): the
		// user discarded the import and the space is clean, so keeping the
		// dir would silently resurrect a cancelled import on the next start
		// (review P0-C's disposal half). A cancel whose compensation was
		// gated or leaked stays kept — uncompensated effects outrank the
		// cancel.
		if err := lc.store.Close(); err != nil {
			log.Errorf("close unsettled failed run: %s", err)
		}
		lc.kept = true
		log.With("dir", lc.store.Dir()).Warnf("run failed before compensation could run; dir kept for the sweep")
		return
	}
	if result.Err != nil && result.Leaked > 0 {
		// Invariant 2, the in-process half (the sweep already obeys it): a
		// compensation that leaked keeps the dir so the next start retries
		// instead of making the leak permanent. The state is ensured here
		// rather than assumed from the engine's OnCompensating hook — the
		// rule must hold whatever path produced the leak.
		if err := lc.store.SetState(context.Background(), runstore.StateCompensating); err != nil {
			log.Errorf("mark leaked run compensating: %s", err)
		}
		if err := lc.store.Flush(context.Background()); err != nil {
			log.Errorf("flush leaked run: %s", err)
		}
		if err := lc.store.Close(); err != nil {
			log.Errorf("close leaked run: %s", err)
		}
		lc.kept = true
		log.With("dir", lc.store.Dir()).Warnf("compensation leaked %d objects; dir kept for the startup sweep to retry", result.Leaked)
		return
	}
	if result.Err != nil && lc.store.MaterializeStarted() {
		// The disposal invariant's other half, and the same class as review
		// item 3 one branch over. In-process compensation walks the IN-MEMORY
		// journal; the DURABLE scope is runstore.CompensationInputs. A pass-3
		// create torn between the tree write and its effect row leaves a row
		// still in the claimed status — which the journal never held and the
		// delete set does hold, past the sticky marker. "Compensation ran and
		// leaked nothing" is therefore a statement about the journal alone,
		// and dropping the dir on it destroyed the only attribution those
		// hollow trees will ever have: the evidence deleting itself, exactly
		// as in A2.
		//
		// The dir is left as the engine left it — the compensating marker is
		// already on disk (OnCompensating gates the first delete), which is
		// the state the sweep's compensate branch reads. Deleting an
		// already-deleted id counts compensated (CompensateIds tolerates
		// not-found), so the retry costs one pass and then drops the dir.
		// Before the marker the two scopes provably agree — nothing has
		// entered the space by construction — so the common
		// mid-crawl failure is untouched.
		if err := lc.store.Close(); err != nil {
			log.Errorf("close compensated materializing run: %s", err)
		}
		lc.kept = true
		log.With("dir", lc.store.Dir()).
			Warnf("pass 3 failed; dir kept so the sweep can compensate the durable scope the journal cannot see")
		return
	}
	state := runstore.StateCompleted
	if result.Err != nil {
		state = runstore.StateFailed
	}
	if err := lc.store.SetState(context.Background(), state); err != nil {
		log.Errorf("mark run %s: %s", state, err)
	}
	if err := lc.store.Drop(); err != nil {
		log.Errorf("drop run dir: %s", err)
	}
}

// discardable is the ONE exception to the disposal invariant: the user
// cancelled, and this dir provably owes the space nothing.
//
// Both halves are needed because they answer different questions.
// Result.NothingToUndo is an IN-MEMORY oracle — the engine's journal held no
// entry — while the durable compensation scope is the manifest's sticky
// MaterializeStarted marker, which is what runstore.CompensationInputs
// consults: past it, a still-claimed row is the crash window of a possible
// create and enters the delete set. A cancel early in pass 3 tears up to
// workerCount in-flight creates and STILL finds an empty journal (review
// item 3), and those claim rows are the only attribution the hollow trees
// left behind will ever have. Before pass 3 the two agree — nothing has
// entered the space by construction — so the crawl-cancel
// carve-out this exists for is untouched.
func discardable(lc *runLifecycle, result *importv2.Result) bool {
	if !userCancelled(result) || !result.NothingToUndo {
		return false
	}
	return lc.store == nil || !lc.store.MaterializeStarted()
}

// identityOptions attaches the durable claim ledger in durable mode (one
// implementation, resume.ClaimLedgerOption — shared with the harnesses).
func (lc *runLifecycle) identityOptions() []identity.Option {
	if lc.store == nil {
		return nil
	}
	return []identity.Option{resume.ClaimLedgerOption(lc.store)}
}

// onIssue is the run's issue fan-out: the live counts (so a run pouring
// out warnings can be abandoned at minute 20, not minute 110) and the
// durable ledger, whose rows are what the dormant surface counts later. Both
// halves in one hook — the counts must not exist on one path only.
func (s *service) onIssue(lc *runLifecycle) func(importv2.Issue) {
	record := func(importv2.Issue) {}
	if lc.store != nil {
		record = resume.IssueRecorder(lc.store)
	}
	return func(issue importv2.Issue) {
		lc.stats.Issue(issue)
		record(issue)
	}
}

// onFetched marks the pass-2/pass-3 boundary durably: RootSpec, then fetched flushed to disk, then materializing — the
// one-place transition (runstore.MarkFetched) shared with every harness. A
// crash after fetched resumes from the spool. nil in volatile mode.
func (s *service) onFetched(lc *runLifecycle) func(importv2.RootSpec) error {
	if lc.store == nil {
		return nil
	}
	return func(rootSpec importv2.RootSpec) error {
		return lc.store.MarkFetched(context.Background(), rootSpec)
	}
}

// onCompensating persists the compensating state before the engine's first
// compensation delete so a crash mid-cleanup is finished by the
// sweep. nil in volatile mode.
func (s *service) onCompensating(lc *runLifecycle) func() error {
	if lc.store == nil {
		return nil
	}
	return func() error {
		// The engine treats this as a GATE: no durable marker, no deletes
		// (a crash mid-cleanup without it would make the next start resume
		// a partly-compensated run). FLUSHED like every other durability
		// point (review P2): a committed-but-unflushed marker can be lost
		// to power loss while its authorised deletes are already in the
		// space.
		if err := lc.store.SetState(context.Background(), runstore.StateCompensating); err != nil {
			log.Errorf("mark run compensating: %s", err)
			return fmt.Errorf("mark run compensating: %w", err)
		}
		if err := lc.store.Flush(context.Background()); err != nil {
			log.Errorf("flush compensating marker: %s", err)
			return fmt.Errorf("flush compensating marker: %w", err)
		}
		return nil
	}
}

// registerRun tracks an active run's cancel-cause func so Close can suspend
// it (with importv2.ErrSuspended) BEFORE the component context's plain
// cancel wins the cause race.
func (s *service) registerRun(cancel context.CancelCauseFunc) int64 {
	s.activeRunsMu.Lock()
	defer s.activeRunsMu.Unlock()
	s.runSeq++
	if s.activeRuns == nil {
		s.activeRuns = map[int64]context.CancelCauseFunc{}
	}
	s.activeRuns[s.runSeq] = cancel
	return s.runSeq
}

func (s *service) unregisterRun(handle int64) {
	s.activeRunsMu.Lock()
	defer s.activeRunsMu.Unlock()
	delete(s.activeRuns, handle)
}

func (s *service) suspendRuns() {
	s.activeRunsMu.Lock()
	defer s.activeRunsMu.Unlock()
	for _, cancel := range s.activeRuns {
		cancel(importv2.ErrSuspended)
	}
}
