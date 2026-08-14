package adapter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/identity"
	"github.com/anyproto/anytype-heart/core/block/importv2/resume"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/util/vcs"
)

// runLifecycle owns one engine run's durable state: the per-run store (with
// the effect ledger the journal writes through) and the spill dir, which
// lives inside the run dir so spilled file bytes share the run's lifetime
// (spec §4.1). In volatile mode (no repo path — test environments) there is
// no store and the spill dir is an OS temp dir removed on finish, as before.
type runLifecycle struct {
	store    *runstore.Store
	spillDir string
	cleanup  func()
	settled  bool
	// untrack removes the run from the live-status registry (runstatus.go);
	// called on every settlement path exactly once via settleTracking.
	untrack func()
}

func (lc *runLifecycle) settleTracking() {
	if lc.untrack != nil {
		lc.untrack()
		lc.untrack = nil
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
	lc.settleTracking()
	if lc.store != nil {
		if err := lc.store.Close(); err != nil {
			log.Errorf("release unsettled run store: %s", err)
		}
		log.With("dir", lc.store.Dir()).Errorf("import run was abandoned without settling; dir left for the startup sweep")
		return
	}
	if lc.cleanup != nil {
		lc.cleanup()
	}
}

// beginRun creates the run dir + store before the engine starts. A store
// creation failure fails the run (spec §7.2: a run that cannot journal must
// not create objects).
func (s *service) beginRun(ctx context.Context, request importv2.Request, converterName string, pathIndex int) (*runLifecycle, error) {
	if s.config.RepoPath == "" {
		spillDir, err := os.MkdirTemp("", "anytype-import-v2-*")
		if err != nil {
			return nil, fmt.Errorf("create spill dir: %w", err)
		}
		return &runLifecycle{spillDir: spillDir, cleanup: func() { _ = os.RemoveAll(spillDir) }}, nil
	}
	runId := bson.NewObjectId().Hex()
	store, err := runstore.Create(ctx, filepath.Join(runstore.RunsRoot(s.config.RepoPath), runId), runstore.Manifest{
		RunId:          runId,
		SpaceId:        request.SpaceID,
		ImportType:     int64(request.Origin.ImportType),
		Mode:           int64(request.Mode),
		UpdateExisting: request.UpdateExisting,
		NoCollection:   request.NoCollection,
		PathIndex:      pathIndex,
		Converter:      converterName,
		AppVersion:     vcs.GetVCSInfo().Version(),
	})
	if err != nil {
		return nil, fmt.Errorf("create run store: %w", err)
	}
	return &runLifecycle{
		store:    store,
		spillDir: store.SpillDir(),
		untrack:  s.trackLive(runId, store, request.Origin.ImportType),
	}, nil
}

// finishRun settles a run's durable state. A run the ENGINE says was
// suspended (Result.Suspended — the single source of that verdict; it means
// compensation was skipped) keeps its dir, marked suspended and flushed for
// the startup sweep (§6.4). Every other outcome is disposed whole: terminal
// state, then delete the dir. The state write is insurance — if Drop fails,
// the sweep sees a terminal manifest and just deletes the dir. State writes
// run on a background context: the run ctx is typically already cancelled
// on the failure path.
func (s *service) finishRun(lc *runLifecycle, result *importv2.Result) {
	lc.settled = true
	lc.settleTracking()
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
		log.With("dir", lc.store.Dir()).Warnf("import run suspended for shutdown; state kept for the startup sweep")
		return
	}
	if result.Err != nil && !result.CompensationRan {
		// The disposal invariant (review Class A): a failure whose effects no
		// compensation covered must not destroy the dir — it is the only
		// record of what was created. Keep it EXACTLY as it is (no state
		// change): the sweep decides — resume (attempts-capped) or
		// compensate. Covers prologue failures (spool open, load), the
		// engine's nil-spool guard, and a gated-out compensation alike.
		if err := lc.store.Close(); err != nil {
			log.Errorf("close unsettled failed run: %s", err)
		}
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
		log.With("dir", lc.store.Dir()).Warnf("compensation leaked %d objects; dir kept for the startup sweep to retry", result.Leaked)
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

// identityOptions attaches the durable claim ledger in durable mode (one
// implementation, resume.ClaimLedgerOption — shared with the harnesses).
func (lc *runLifecycle) identityOptions() []identity.Option {
	if lc.store == nil {
		return nil
	}
	return []identity.Option{resume.ClaimLedgerOption(lc.store)}
}

// onIssue writes every retained issue to the durable ledger (one
// implementation, resume.IssueRecorder). nil in volatile mode.
func (s *service) onIssue(lc *runLifecycle) func(importv2.Issue) {
	if lc.store == nil {
		return nil
	}
	return resume.IssueRecorder(lc.store)
}

// onFetched marks the pass-2/pass-3 boundary durably (DM spec §4.1 +
// §6.4): RootSpec, then fetched flushed to disk, then materializing — the
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
// compensation delete (spec §6.5) so a crash mid-cleanup is finished by the
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
