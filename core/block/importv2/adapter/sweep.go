package adapter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/anyproto/anytype-heart/core/block/importv2/persist"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/space"
)

// The startup sweep (spec §6.1 + DM spec §8.1): every run dir left behind
// by a previous process is finished being deleted, RESUMED (a run whose
// pass 2 completed — fetched/materializing, or suspended after
// materialization began — restarts pass 3 from its spool, attempts-capped),
// or compensated from its durable ledger and then deleted.

type spaceStatus int

const (
	spaceOK spaceStatus = iota
	spaceGone
	spaceUnknown
)

type spaceProbe func(ctx context.Context, spaceId string) spaceStatus

type sweepAction string

const (
	sweepDeletedTerminal         sweepAction = "deleted-terminal"
	sweepCompensated             sweepAction = "compensated"
	sweepCompensatedPartially    sweepAction = "compensated-partially" // leaks left; dir kept for retry
	sweepDeletedCorrupt          sweepAction = "deleted-corrupt"
	sweepDeletedEmpty            sweepAction = "deleted-empty"
	sweepDeletedSpaceGone        sweepAction = "deleted-space-gone"
	sweepSkippedActive           sweepAction = "skipped-active"
	sweepSkippedNewerSchema      sweepAction = "skipped-newer-schema"
	sweepSkippedSpaceUnavailable sweepAction = "skipped-space-unavailable"
	sweepSkippedError            sweepAction = "skipped-error"
	sweepResumedCompleted        sweepAction = "resumed-completed"
	sweepResumedSuspended        sweepAction = "resumed-suspended" // shut down again mid-resume; dir kept
	sweepResumedFailed           sweepAction = "resumed-failed"
)

// maxResumeAttempts caps resume-and-crash loops (08-13 §6.1): the counter
// moves durably BEFORE each attempt (runstore.BeginResume), so however
// early the crash lands, the run reaches compensation after this many
// tries.
const maxResumeAttempts = 3

// resumeFn restarts a resumable run from its open store. The store's
// ownership passes to the callee (it settles or keeps the dir; sweepOne's
// deferred Close is idempotent insurance). nil disables resume — every
// resumable state then falls through to compensation, the phase-A shape.
type resumeFn func(ctx context.Context, store *runstore.Store, manifest runstore.Manifest) sweepOutcome

type sweepOutcome struct {
	Dir    string
	Action sweepAction
	Result persist.CompensationResult
	Err    error
}

// sweepRuns walks the runs root once and settles every dir it finds. New
// dirs created by imports starting mid-sweep are not in the listing
// snapshot, and dirs a live Store holds open are skipped via the active
// registry — an active run is never touched. A dead ctx (the component is
// closing) stops the walk: remaining dirs settle on the next start.
func sweepRuns(ctx context.Context, root string, objects persist.ObjectAccess, probe spaceProbe, resume, crawlResume resumeFn) []sweepOutcome {
	dirs, err := runstore.ListRunDirs(root)
	if err != nil {
		log.Errorf("sweep: list run dirs: %s", err)
		return nil
	}
	var outcomes []sweepOutcome
	for _, dir := range dirs {
		if ctx.Err() != nil {
			return outcomes
		}
		outcomes = append(outcomes, sweepOneGuarded(ctx, dir, objects, probe, resume, crawlResume))
	}
	return outcomes
}

// sweepOneGuarded contains a panic to ITS dir: one poison dir must not
// abort the rest of the sweep (previously the recover sat a level up, so
// aaa-poison left zzz-healthy uncompensated on every start, forever).
func sweepOneGuarded(ctx context.Context, dir string, objects persist.ObjectAccess, probe spaceProbe, resume, crawlResume resumeFn) (outcome sweepOutcome) {
	defer func() {
		if rec := recover(); rec != nil {
			outcome.Dir = dir
			outcome.Action = sweepSkippedError
			outcome.Err = fmt.Errorf("sweep panic: %v", rec)
			log.With("dir", dir).Errorf("sweep of one run dir panicked; continuing with the rest: %v", rec)
		}
	}()
	return sweepOne(ctx, dir, objects, probe, resume, crawlResume)
}

// resumable reports whether the manifest describes a run whose pass 2
// completed — the DM spec §8.1 class: the spool is provably whole (the
// fetched marker or later), so pass 3 can restart from the dir alone. A
// suspend BEFORE materialization began is not in the class (its crawl is
// resumable only with DM-3's pass-2 seam) and keeps compensating —
// trivially, to nothing.
//
// Resume also happens only WITHIN a schema version (§4.4: only the frozen
// compensation core is promised across versions, and resume rehydrates far
// more than the core). An older-schema dir falls through to the compensate
// branch below, which reads only frozen fields — "resume is refused,
// compensation is guaranteed". Newer-schema dirs never reach here (the
// hands-off check above).
func resumable(m runstore.Manifest) bool {
	if m.SchemaVersion != runstore.SchemaVersion {
		return false
	}
	switch m.State {
	case runstore.StateFetched, runstore.StateMaterializing:
		return true
	case runstore.StateSuspended:
		return m.MaterializeStarted
	default:
		return false
	}
}

// crawlResumable is the DM-3 §8.3 class, disjoint from resumable() by the
// sticky marker: a run interrupted BEFORE its crawl completed — crashed
// (running) or suspended pre-materialize — whose manifest still carries the
// request that rebuilds its converter. Dirs written by pre-DM-3 binaries
// have no request and keep the old disposition (compensate — trivially, to
// nothing). Same version gate as resumable (§4.4: resume only within a
// version), belt-checked again in resume.LoadCrawl.
func crawlResumable(m runstore.Manifest) bool {
	if m.SchemaVersion != runstore.SchemaVersion || m.MaterializeStarted || len(m.Request) == 0 {
		return false
	}
	return m.State == runstore.StateRunning || m.State == runstore.StateSuspended
}

func sweepOne(ctx context.Context, dir string, objects persist.ObjectAccess, probe spaceProbe, resume, crawlResume resumeFn) sweepOutcome {
	outcome := sweepOutcome{Dir: dir}
	// OpenExclusive takes the guard atomically with the liveness check —
	// the IsActive-then-Open pair had a gap a DM-2 resume could slip into.
	// A live Store holding the dir (a run Close's grace gave up on, still
	// finishing in this process) yields ErrActive: the db's .lock is a
	// dirty sentinel, not a mutex — opening and dropping here would unlink
	// the dir under the live writer.
	store, err := runstore.OpenExclusive(ctx, dir)
	if err != nil {
		switch {
		case errors.Is(err, runstore.ErrActive):
			outcome.Action = sweepSkippedActive
			return outcome
		case ctx.Err() != nil:
			// The stop is consulted BEFORE the corrupt branch (review P0-A):
			// a shutdown mid-open surfaces through whatever error the driver
			// was in the middle of — including a cancelled quick check that
			// any-store wraps as ErrQuickCheckFailed — and the corrupt
			// branch three lines down answers by UNLINKING the ledger.
			outcome.Action = sweepSkippedError
			outcome.Err = fmt.Errorf("sweep stopped: %w", err)
			return outcome
		case runstore.IsCorrupted(err):
			// The ledger is lost: whatever the run created can no longer be
			// attributed. Delete the dir, say so loudly — leak, never guess.
			outcome.Action = sweepDeletedCorrupt
			outcome.Err = err
			removeDir(dir, &outcome)
		case runstore.IsMissingManifest(err):
			// Crashed between dir creation and the manifest write: nothing
			// was ever recorded, so nothing was ever done. Plain garbage.
			outcome.Action = sweepDeletedEmpty
			removeDir(dir, &outcome)
		default:
			// Transient (IO, lock): keep the dir, retry next start.
			outcome.Action = sweepSkippedError
			outcome.Err = err
		}
		return outcome
	}

	// C1: the sweep's own store hold must survive a panic anywhere below
	// (a panicking DeleteObject is recovered one level up) — Close is
	// idempotent, so the branches that Close/Drop explicitly are unharmed.
	defer store.Close()
	manifest, err := store.Manifest(ctx)
	if err != nil {
		_ = store.Close()
		outcome.Action = sweepSkippedError
		outcome.Err = err
		return outcome
	}
	if manifest.SchemaVersion > runstore.SchemaVersion {
		// A newer binary owns this run (downgrade scenario) — hands off.
		_ = store.Close()
		outcome.Action = sweepSkippedNewerSchema
		return outcome
	}
	if manifest.State == runstore.StateCompleted || manifest.State == runstore.StateFailed {
		// Finished run whose Drop didn't complete: just finish the delete.
		outcome.Action = sweepDeletedTerminal
		dropStore(store, &outcome)
		return outcome
	}

	switch probe(ctx, manifest.SpaceId) {
	case spaceGone:
		// Nothing to compensate into; the objects died with the space.
		outcome.Action = sweepDeletedSpaceGone
		dropStore(store, &outcome)
		return outcome
	case spaceUnknown:
		_ = store.Close()
		outcome.Action = sweepSkippedSpaceUnavailable
		return outcome
	}

	if resume != nil && resumable(manifest) && manifest.ResumeAttempts < maxResumeAttempts {
		// fetched | materializing | suspended-mid-materialize: finish the
		// materialization from the dir instead of destroying it (§8.1) —
		// headlessly: no source, no credentials, no network. Attempts are
		// capped; exhaustion falls through to compensation below.
		return resume(ctx, store, manifest)
	}
	if crawlResume != nil && crawlResumable(manifest) && manifest.ResumeAttempts < maxResumeAttempts {
		// running | suspended mid-crawl, request stored: re-run the crawl
		// with the spool as the skip set (§8.3) — this needs the source and
		// its credentials, which is exactly what the manifest's request
		// carries. Attempts share the same cap; exhaustion falls through to
		// compensation (trivially nothing — pass 2 touched no space).
		return crawlResume(ctx, store, manifest)
	}

	// running | suspended | cancelling | compensating — and resumable runs
	// whose attempts are exhausted: compensate from the frozen-core view
	// (§4.4) — CompensateIds tolerates already-deleted objects, so
	// re-running a crashed compensation is safe (§6.5).
	inputs, err := store.CompensationInputs(ctx)
	if err != nil {
		_ = store.Close()
		outcome.Action = sweepSkippedError
		outcome.Err = err
		return outcome
	}
	if err = store.SetState(ctx, runstore.StateCompensating); err != nil {
		// Same gate as the engine's OnCompensating: no durable marker, no
		// deletes — a crash mid-cleanup without it makes the next start
		// resume a partly-compensated run, silently missing its deleted
		// objects.
		_ = store.Close()
		outcome.Action = sweepSkippedError
		outcome.Err = fmt.Errorf("mark compensating: %w", err)
		return outcome
	}
	if err = store.Flush(ctx); err != nil {
		// The marker must be ON DISK before the first delete (review P2):
		// a committed-but-unflushed marker can be lost to power loss while
		// its authorised deletes are already in the space.
		_ = store.Close()
		outcome.Action = sweepSkippedError
		outcome.Err = fmt.Errorf("flush compensating marker: %w", err)
		return outcome
	}
	outcome.Result = persist.CompensateIds(ctx, objects, inputs.Created, inputs.OwnedFiles, inputs.Updated)
	if outcome.Result.Leaked > 0 {
		// Leaks are retryable — compensation is idempotent, so the next
		// start simply runs it again (already-deleted objects count
		// compensated). Dropping the dir here would turn a retryable leak
		// into a permanent orphan; keep it in the compensating state.
		outcome.Action = sweepCompensatedPartially
		if err = store.Close(); err != nil {
			outcome.Err = errors.Join(outcome.Err, err)
		}
		return outcome
	}
	outcome.Action = sweepCompensated
	if err = store.SetState(ctx, runstore.StateFailed); err != nil {
		log.Errorf("sweep: mark %s failed: %s", dir, err)
	}
	dropStore(store, &outcome)
	return outcome
}

func removeDir(dir string, outcome *sweepOutcome) {
	if err := os.RemoveAll(dir); err != nil {
		outcome.Err = errors.Join(outcome.Err, err)
	}
}

func dropStore(store *runstore.Store, outcome *sweepOutcome) {
	if err := store.Drop(); err != nil {
		outcome.Err = errors.Join(outcome.Err, err)
	}
}

// sweepAbandoned runs the sweep in the background at component start,
// logging one structured line per settled run on the import-v2 scope.
func (s *service) sweepAbandoned() {
	defer func() {
		if rec := recover(); rec != nil {
			log.Errorf("sweep panic: %v\n%s", rec, debug.Stack())
		}
	}()
	outcomes := sweepRuns(s.componentCtx, runstore.RunsRoot(s.config.RepoPath), s.objects, s.probeSpace, s.resumeRunner, s.crawlResumeRunner)
	for _, outcome := range outcomes {
		logger := log.With(
			"dir", outcome.Dir,
			"action", string(outcome.Action),
			"compensated", outcome.Result.Compensated,
			"alreadyGone", outcome.Result.AlreadyGone,
			"leaked", outcome.Result.Leaked,
			"uncovered", len(outcome.Result.Uncovered),
		)
		switch {
		case outcome.Err != nil:
			logger.Errorf("swept abandoned import run: %s", outcome.Err)
		case outcome.Action == sweepDeletedCorrupt:
			logger.Errorf("abandoned import run had a corrupted ledger; its objects may be orphaned")
		case outcome.Action == sweepCompensated || outcome.Action == sweepDeletedSpaceGone:
			logger.Warnf("swept abandoned import run")
		default:
			logger.Infof("swept abandoned import run")
		}
	}
}

// probeSpace classifies whether a run's target space still exists. Only a
// definitive not-exists/deleted answer allows deleting the run dir without
// compensation; anything else is retried on the next start.
//
// ErrSpaceNotExists IS definitive here, reviewed 2026-08-13: Get →
// ensureSpaceStarted (space/load.go) falls through to resolveDerivedInfo in
// lazy mode, which reads the space view directly from techspace rather than
// from deferred statuses — a space that was ever an import target on this
// device has a view here, so a lazily-not-yet-started space resolves fine
// and never reaches the ErrSpaceNotExists branch.
func (s *service) probeSpace(ctx context.Context, spaceId string) spaceStatus {
	if spaceId == "" {
		return spaceGone
	}
	_, err := s.spaceService.Get(ctx, spaceId)
	switch {
	case err == nil:
		return spaceOK
	case errors.Is(err, space.ErrSpaceNotExists),
		errors.Is(err, space.ErrSpaceDeleted),
		errors.Is(err, space.ErrSpaceStorageMissig):
		return spaceGone
	default:
		return spaceUnknown
	}
}
