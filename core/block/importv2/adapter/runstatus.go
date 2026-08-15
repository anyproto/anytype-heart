package adapter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

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
// Scope, stated honestly. A LIVE run is served from its own statistic
// emitter — the same builder over the same state the push event uses, so
// polling and listening cannot disagree by construction (§15.5's "served
// from the adapter's registry snapshot"). A DORMANT run is served from the
// ledger alone, which is everything a restart itself could resume from;
// the purely in-memory fields — itemsPerSecond, ETA, currentItem, live
// throttle/retry state — are then honestly absent, exactly as §15.4's
// dormant column says.

// ErrRunNotFound reports an importId with neither a live run nor a run dir.
var ErrRunNotFound = errors.New("import run not found")

// liveRunInfo tracks one durable run whose engine is running right now: its
// store handle (so a status read never second-opens a live db) and its
// statistic emitter (the live numbers).
type liveRunInfo struct {
	store *runstore.Store
	stats *statEmitter
}

func (s *service) trackLive(runId string, store *runstore.Store, lc *runLifecycle) func() {
	s.liveStatusMu.Lock()
	if s.liveStatus == nil {
		s.liveStatus = map[string]*liveRunInfo{}
	}
	s.liveStatus[runId] = &liveRunInfo{store: store, stats: lc.stats}
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
		return buildLiveRunStatus(ctx, live)
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
	return buildDormantRunStatus(ctx, store)
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
			run, err := buildLiveRunStatus(ctx, info)
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

// buildLiveRunStatus serves a running import from its own emitter — the
// SAME builder the push event uses over the SAME state (§15.5). This is
// what makes "push and pull agree" a property of the code rather than a
// promise: there is no second derivation to drift. Only the durable
// lifecycle label is read from the store.
func buildLiveRunStatus(ctx context.Context, live *liveRunInfo) (*pb.RpcObjectImportRunStatusRun, error) {
	manifest, err := live.store.Manifest(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.RpcObjectImportRunStatusRun{
		Status:        live.stats.Snapshot(),
		ManifestState: string(manifest.State),
		Live:          true,
	}, nil
}

// buildDormantRunStatus derives the §15.4 ledger-backed columns from a run
// dir with no engine behind it. Live runs never come here — they are served
// from their emitter — so there is no `live` flag to get wrong.
func buildDormantRunStatus(ctx context.Context, store *runstore.Store) (*pb.RpcObjectImportRunStatusRun, error) {
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
		}, nil
	}
	status := &pb.EventImportStatistic{
		ImportId:     manifest.RunId,
		ImportType:   model.ImportType(manifest.ImportType),
		Phase:        phaseOf(manifest),
		CancelEffect: cancelEffectOf(manifest),
		// The three-state model describes a RUNNING engine — the pacer's
		// pause, the retry loop's attempt count. A dormant dir has neither,
		// so the state stays Running and manifestState carries the
		// lifecycle instead.
		State: pb.EventImportStatistic_Running,
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
	//
	// The counters are PER PHASE, exactly as the live emitter's are. A dir
	// that stopped mid-crawl must be read as a crawl — spool rows against
	// the claim count — or the same field means one thing pushed and another
	// polled for the phase a big import spends hours in.
	if status.Phase >= pb.EventImportStatistic_Creating {
		status.PagesTotal = int64(pages)
		status.FilesTotal = int64(files)
		status.PagesDone = state.PagesDone
		status.FilesDone = state.FilesDone
		status.TotalsKnown = true
	} else {
		status.PagesTotal = state.ClaimsTotal
		status.PagesDone = int64(pages)
		status.FilesDone = int64(files)
		// filesTotal stays 0 = unknown: files are found by crawling, so
		// during the crawl there is no denominator to render against, and
		// the schema's own convention for that is zero (bytesTotal says so
		// in as many words).
		//
		// A spool row can only exist after pass 1 flushed its claims (the
		// write-ahead rule in the spool sink), so ONE row proves the crawl
		// began and the claim count is final. Nothing on disk distinguishes
		// a dir that died mid-/search from one that died just after, so a
		// spool-less dir answers with the conservative "unknown" — never a
		// fake bar (§15.3).
		status.TotalsKnown = pages+files > 0
	}
	status.BytesDone = spillBytes(store.SpillDir())
	status.ObjectsCreated = state.Engine.Created
	for _, issue := range state.Engine.Issues {
		countIssue(issue.Severity, &status.WarningCount, &status.ErrorCount)
	}
	return &pb.RpcObjectImportRunStatusRun{
		Status:        status,
		ManifestState: string(manifest.State),
	}, nil
}

// spillBytes sums the pass-2 download spill — the dormant column's
// bytesDone (§15.4: "spill dir + spool file rows"). ONLY the spool sink's
// own files count: the persister spills uploads into the same dir under its
// own prefix, and those bytes are the same content read back, not new
// transfer. An unreadable dir reports zero, which the schema already means
// as unknown; a status read must never fail over telemetry.
func spillBytes(dir string) int64 {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), importv2.SpoolSpillPrefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total
}

// countIssue is the ONE severity → wire-counter classification, shared by
// the live emitter and this dormant ledger read. Info diagnostics are not
// problems and count as neither.
//
// A live run counts its own fatal into errorCount; a dormant one does not,
// because resume.rehydrateIssues drops fatal records on load — by design,
// since a fatal that coexists with a resumable dir IS the abort that made
// the dir dormant, not a content problem. That asymmetry belongs to the
// resume seed, not to this classification, which is why it lives in one
// function.
func countIssue(severity importv2.Severity, warnings, errors *int64) {
	switch {
	case severity >= importv2.SeverityObjectError:
		*errors++
	case severity == importv2.SeverityWarning:
		*warnings++
	}
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
//
// The materialize marker is consulted FIRST, whatever lifecycle label the
// run carries. A dir cancelled or compensating mid-crawl used to report
// FINALIZING while cancelEffect — which reads the marker — reported
// NothingToUndo, so one message said "finishing up" and "nothing has
// entered your space" at once; the same contradiction picked the wrong
// counter column. Past the marker the phase is CREATING or later, before it
// the run never left pass 2, and that is exactly the boundary the live
// emitter derives its own cancel effect from.
func phaseOf(m runstore.Manifest) pb.EventImportStatisticPhase {
	if !m.MaterializeStarted && m.State != runstore.StateFetched {
		return pb.EventImportStatistic_Fetching
	}
	switch m.State {
	case runstore.StateCompleted, runstore.StateFailed, runstore.StateCompensating, runstore.StateCancelling:
		return pb.EventImportStatistic_Finalizing
	default: // fetched | materializing | suspended | running
		return pb.EventImportStatistic_Creating
	}
}
