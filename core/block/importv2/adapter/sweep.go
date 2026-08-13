package adapter

import (
	"context"
	"errors"
	"os"
	"runtime/debug"

	"github.com/anyproto/anytype-heart/core/block/importv2/persist"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/space"
)

// The startup sweep (spec §6.1, phase A subset): every run dir left behind
// by a previous process is either finished being deleted, or compensated
// from its durable ledger and then deleted. There is no resume branch yet —
// running/suspended runs are compensated (spec §10 phase A) — so every
// branch below ends in "dir gone" except the two explicit skips.

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
)

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
func sweepRuns(ctx context.Context, root string, objects persist.ObjectAccess, probe spaceProbe) []sweepOutcome {
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
		outcomes = append(outcomes, sweepOne(ctx, dir, objects, probe))
	}
	return outcomes
}

func sweepOne(ctx context.Context, dir string, objects persist.ObjectAccess, probe spaceProbe) sweepOutcome {
	outcome := sweepOutcome{Dir: dir}
	if runstore.IsActive(dir) {
		// A live Store holds this dir (a run Close's grace gave up on, still
		// finishing in this process). The db's .lock is a dirty sentinel,
		// not a mutex — opening and dropping here would unlink the dir under
		// the live writer, whose subsequent writes would succeed into an
		// unlinked file.
		outcome.Action = sweepSkippedActive
		return outcome
	}
	store, err := runstore.Open(ctx, dir)
	if err != nil {
		switch {
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

	// running | suspended | cancelling | compensating: compensate from the
	// frozen-core view (§4.4) — CompensateIds tolerates already-deleted
	// objects, so re-running a crashed compensation is safe (§6.5).
	inputs, err := store.CompensationInputs(ctx)
	if err != nil {
		_ = store.Close()
		outcome.Action = sweepSkippedError
		outcome.Err = err
		return outcome
	}
	if err = store.SetState(ctx, runstore.StateCompensating); err != nil {
		log.Errorf("sweep: mark %s compensating: %s", dir, err)
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
	outcomes := sweepRuns(s.componentCtx, runstore.RunsRoot(s.config.RepoPath), s.blockService, s.probeSpace)
	for _, outcome := range outcomes {
		logger := log.With(
			"dir", outcome.Dir,
			"action", string(outcome.Action),
			"compensated", outcome.Result.Compensated,
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
