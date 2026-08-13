package adapter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/importv2"
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
	return &runLifecycle{store: store, spillDir: store.SpillDir()}, nil
}

// finishRun disposes a finished run whole: terminal state, then delete the
// dir. The state write is insurance — if Drop fails, the startup sweep sees
// a terminal manifest and just deletes the dir. Runs on a background
// context: the run ctx is typically already cancelled on the failure path.
func (s *service) finishRun(lc *runLifecycle, result *importv2.Result) {
	if lc.store == nil {
		if lc.cleanup != nil {
			lc.cleanup()
		}
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

// onCompensating persists the compensating state before the engine's first
// compensation delete (spec §6.5) so a crash mid-cleanup is finished by the
// sweep. nil in volatile mode.
func (s *service) onCompensating(lc *runLifecycle) func() {
	if lc.store == nil {
		return nil
	}
	return func() {
		if err := lc.store.SetState(context.Background(), runstore.StateCompensating); err != nil {
			log.Errorf("mark run compensating: %s", err)
		}
	}
}
