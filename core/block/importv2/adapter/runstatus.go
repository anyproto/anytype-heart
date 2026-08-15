package adapter

import (
	"context"
	"errors"
	"fmt"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/resume"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The pull side of the §15 observability surface: ObjectImportRunStatus /
// ObjectImportRunList, served for LIVE runs from the running run's own
// store handle and for DORMANT runs (a crashed process's dir awaiting the
// sweep, a suspended run) from manifest + ledger alone — the same
// dormant-run reading the pass-3 restart is built on, which is what makes
// polling by importId restart-proof (§15.5).
//
// Scope, stated honestly: the in-memory-only fields of the statistic —
// itemsPerSecond, ETA, currentItem, live throttle/retry state, bytes —
// are served zero/empty until the §15 push event core (the coalescing
// emitter with its pacer/retry hooks) lands; every ledger-derived field
// is exact. That matches §15.4's dormant column; live runs are served the
// same column for now.

// ErrRunNotFound reports an importId with neither a live run nor a run dir.
var ErrRunNotFound = errors.New("import run not found")

// liveRunInfo tracks one durable run whose engine is running right now, so
// status reads share its store handle instead of second-opening a live db.
type liveRunInfo struct {
	store      *runstore.Store
	importType model.ImportType
}

func (s *service) trackLive(runId string, store *runstore.Store, importType model.ImportType) func() {
	s.liveStatusMu.Lock()
	if s.liveStatus == nil {
		s.liveStatus = map[string]*liveRunInfo{}
	}
	s.liveStatus[runId] = &liveRunInfo{store: store, importType: importType}
	s.liveStatusMu.Unlock()
	return func() {
		s.liveStatusMu.Lock()
		delete(s.liveStatus, runId)
		s.liveStatusMu.Unlock()
	}
}

func (s *service) liveRun(runId string) *liveRunInfo {
	s.liveStatusMu.Lock()
	defer s.liveStatusMu.Unlock()
	return s.liveStatus[runId]
}

func (s *service) liveRunIds() map[string]struct{} {
	s.liveStatusMu.Lock()
	defer s.liveStatusMu.Unlock()
	ids := make(map[string]struct{}, len(s.liveStatus))
	for id := range s.liveStatus {
		ids[id] = struct{}{}
	}
	return ids
}

// RunStatus reports one run by its durable importId (= runId).
func (s *service) RunStatus(ctx context.Context, importId string) (*pb.RpcObjectImportRunStatusRun, error) {
	if live := s.liveRun(importId); live != nil {
		return buildRunStatus(ctx, live.store, true)
	}
	dirs, err := runstore.ListRunDirs(runstore.RunsRoot(s.config.RepoPath))
	if err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		if runstore.RunIdOfDir(dir) != importId {
			continue
		}
		return statusOfDormantDir(ctx, dir)
	}
	return nil, ErrRunNotFound
}

// statusOfDormantDir reads one dormant dir through the advisory reader:
// no guard (the sweep must never be made to skip a dir by a poll), no
// sentinel touch (corruption detection stays armed), no writes, and the
// handle is released on every path including panics (review Class E).
func statusOfDormantDir(ctx context.Context, dir string) (*pb.RpcObjectImportRunStatusRun, error) {
	store, err := runstore.OpenStatusReader(ctx, dir)
	if err != nil {
		return nil, fmt.Errorf("open run %s: %w", runstore.RunIdOfDir(dir), err)
	}
	defer store.Close()
	return buildRunStatus(ctx, store, false)
}

// RunList reports every known run: live ones first-hand, dormant dirs from
// disk. A dormant dir that cannot be opened (mid-sweep, corrupt) is
// reported with its error logged and skipped — a listing must not fail
// whole because one dir is sick.
func (s *service) RunList(ctx context.Context) ([]*pb.RpcObjectImportRunStatusRun, error) {
	var runs []*pb.RpcObjectImportRunStatusRun
	live := s.liveRunIds()
	for id := range live {
		if info := s.liveRun(id); info != nil {
			run, err := buildRunStatus(ctx, info.store, true)
			if err != nil {
				log.Warnf("run list: live run %s: %s", id, err)
				continue
			}
			runs = append(runs, run)
		}
	}
	dirs, err := runstore.ListRunDirs(runstore.RunsRoot(s.config.RepoPath))
	if err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		if _, isLive := live[runstore.RunIdOfDir(dir)]; isLive {
			continue
		}
		if ctx.Err() != nil {
			return runs, ctx.Err()
		}
		run, err := statusOfDormantDir(ctx, dir)
		if err != nil {
			log.With("dir", dir).Warnf("run list: skipping unreadable run dir: %s", err)
			continue
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// buildRunStatus derives the §15.4 ledger-backed columns from a run store.
func buildRunStatus(ctx context.Context, store *runstore.Store, live bool) (*pb.RpcObjectImportRunStatusRun, error) {
	manifest, err := store.Manifest(ctx)
	if err != nil {
		return nil, err
	}
	if manifest.SchemaVersion != runstore.SchemaVersion {
		// A cross-version dir is SERVED, from the frozen manifest core alone
		// (review P2: erroring here made v1 dirs vanish silently from the
		// listing — the exact Class-E symptom through a different door).
		// §4.4 froze exactly the fields that let any version say what a run
		// IS; the ledger-derived numbers need same-version reads and are
		// honestly absent.
		return &pb.RpcObjectImportRunStatusRun{
			Status: &pb.EventImportStatistic{
				ImportId:     manifest.RunId,
				ImportType:   model.ImportType(manifest.ImportType),
				Phase:        phaseOf(manifest),
				State:        pb.EventImportStatistic_Running,
				CancelEffect: cancelEffectOf(manifest),
			},
			ManifestState: string(manifest.State),
			Live:          live,
		}, nil
	}
	status := &pb.EventImportStatistic{
		ImportId:     manifest.RunId,
		ImportType:   model.ImportType(manifest.ImportType),
		Phase:        phaseOf(manifest),
		CancelEffect: cancelEffectOf(manifest),
		// The three-state model describes a running engine; ledger-backed
		// serving has no throttle/retry signal yet (event core), so the
		// state stays Running and manifestState carries the lifecycle.
		State: pb.EventImportStatistic_Running,
		// Totals become known at the pass-1/2 boundary and stay known; a
		// run still crawling has none (§15.3: count-up, never a fake bar).
		TotalsKnown: manifest.MaterializeStarted || manifest.State == runstore.StateFetched,
	}
	if (resumable(manifest) && manifest.ResumeAttempts < maxResumeAttempts) ||
		(crawlResumable(manifest) && manifest.CrawlResumeAttempts < maxResumeAttempts) {
		// Closing is lossless when SOME resume class covers the run AND can
		// still be attempted (review P1: keyed off the request alone, this
		// reported true with the attempt cap exhausted — and the very next
		// sweep dropped the dir). The predicates are the sweep's OWN
		// (resumable/crawlResumable, same caps), so this surface and the
		// sweep's actual behavior cannot drift apart. Live runs read the
		// same way: mid-crawl is the crawlResumable shape (running, request
		// held), mid-materialize the resumable one (materializing). A
		// pre-DM-3 run without a stored request is still lost on close, and
		// still says so; so does a dir the next sweep will compensate.
		status.SafeToClose = true
	}
	state, err := resume.Load(ctx, store)
	if err != nil {
		return nil, err
	}
	spool, err := store.Spool(ctx)
	if err != nil {
		return nil, err
	}
	pages, files, _, err := spool.Census(ctx)
	if err != nil {
		return nil, err
	}
	// Derived-class definitions are counted by neither user-facing counter,
	// on this surface and in the engine alike (run.countObject): they carry
	// no pass-1 claim, so folding them in made pagesDone outrun a fetching
	// denominator that IS the claim count.
	status.PagesTotal = int64(pages)
	status.FilesTotal = int64(files)
	status.FilesDone = state.FilesDone
	status.PagesDone = state.PagesDone
	status.ObjectsCreated = state.Engine.Created
	for _, issue := range state.Engine.Issues {
		switch {
		case issue.Severity >= importv2.SeverityObjectError:
			status.ErrorCount++
		case issue.Severity == importv2.SeverityWarning:
			status.WarningCount++
		}
	}
	return &pb.RpcObjectImportRunStatusRun{
		Status:        status,
		ManifestState: string(manifest.State),
		Live:          live,
	}, nil
}

func cancelEffectOf(m runstore.Manifest) pb.EventImportStatisticCancelEffect {
	if m.MaterializeStarted {
		return pb.EventImportStatistic_RemovesCreated
	}
	return pb.EventImportStatistic_NothingToUndo
}

// phaseOf maps the durable lifecycle onto the coarse phase indicator: a
// dormant run has no in-flight stage, so the mapping names the stage the
// run stopped in.
func phaseOf(m runstore.Manifest) pb.EventImportStatisticPhase {
	switch m.State {
	case runstore.StateRunning:
		return pb.EventImportStatistic_Fetching
	case runstore.StateCompleted, runstore.StateFailed, runstore.StateCompensating, runstore.StateCancelling:
		return pb.EventImportStatistic_Finalizing
	default: // fetched | materializing | suspended
		if m.MaterializeStarted || m.State == runstore.StateFetched {
			return pb.EventImportStatistic_Creating
		}
		return pb.EventImportStatistic_Fetching
	}
}
