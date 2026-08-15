package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/anyproto/anytype-heart/core/block/importv2"
)

// Spool is the pass-2 → pass-3 absorbing queue seam (deferred-
// materialization spec §4): pass 2 appends converted objects in emission
// order, pass 3 replays them through the persist pipeline. Implemented by
// runstore.Spool (disk-backed, the real thing) and memorySpool (unit-test
// default — it deliberately does NOT claim the §5 memory invariant).
type Spool interface {
	Append(ctx context.Context, o *importv2.Object) error
	Replay(ctx context.Context, emit func(o *importv2.Object) error) error
	// Census counts the recorded rows split by class without decoding a
	// single snapshot — the §15.4 totals. Pass 3's denominator, and on a
	// resumed crawl pass 2's already-earned numerator. Derived-class
	// definitions are reported apart because they belong to neither user-
	// facing counter (see run.countObject).
	Census(ctx context.Context) (pages, files, derived int, err error)
}

type memorySpool struct {
	mu      sync.Mutex
	objects []*importv2.Object
}

func (m *memorySpool) Append(ctx context.Context, o *importv2.Object) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects = append(m.objects, o)
	return nil
}

func (m *memorySpool) Replay(ctx context.Context, emit func(o *importv2.Object) error) error {
	m.mu.Lock()
	objects := m.objects
	m.mu.Unlock()
	for _, o := range objects {
		if err := emit(o); err != nil {
			return err
		}
	}
	return nil
}

func (m *memorySpool) Census(ctx context.Context) (pages, files, derived int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, o := range m.objects {
		switch {
		case isFileClass(o.SbType):
			files++
		case isDerivedClass(o.SbType):
			derived++
		default:
			pages++
		}
	}
	return pages, files, derived, nil
}

// spoolSink receives the converter's stream in pass 2. Nothing here touches
// the space: objects are serialized to the spool, file downloads drain to
// the spill dir (dissolving the unserializable Open closure — DM spec
// fact 3), issues and late claims flow to the run ledgers.
type spoolSink struct {
	run      *run
	spool    Spool
	spillDir string
}

func (s *spoolSink) Object(ctx context.Context, object *importv2.Object) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if object == nil || object.Payload == nil {
		return importv2.Issue{
			Severity: importv2.SeverityFatal,
			Code:     importv2.IssueInvariant,
			Message:  "converter emitted an empty object",
		}
	}
	if s.run.recordedInSpool(object.SourceKey) {
		// The crawl-resume backstop (08-13 §6.2 item 5): a previous
		// incarnation already recorded this key, so the re-emission is
		// absorbed — BEFORE the file drain (a recorded file must not
		// re-download; its bytes sit in the spill dir next to its row) and
		// before the append (a duplicate row would materialize twice). The
		// replay serves the recorded version; if the source changed the
		// object between sessions, the recording wins — the crawl artifact
		// is the run's ground truth, drift lands on the next import.
		// Emission order is safe: old rows keep their recorded positions and
		// new rows append after them, so a definition is always at or ahead
		// of its first referencer whichever incarnation recorded either.
		return nil
	}
	s.run.deps.Gauge(1)
	defer s.run.deps.Gauge(-1)
	if object.File != nil && s.spillDir != "" && (object.File.Open != nil || object.File.Path != "") {
		if err := s.drainFile(ctx, object); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// The file object never reaches the spool: references degrade to
			// the missing marker in pass 3, exactly as a failed download
			// degrades them today. Under ALL_OR_NOTHING this aborts pass 2 —
			// while aborting is still free.
			s.run.failed.Add(1)
			s.run.rootMu.Lock()
			s.run.failedKeys[object.SourceKey] = struct{}{}
			s.run.rootMu.Unlock()
			s.run.report(importv2.Issue{
				Severity:  importv2.SeverityObjectError,
				Code:      importv2.IssueFileFetchFailed,
				SourceKey: object.SourceKey,
				Err:       err,
			})
			return s.errIfAborting(ctx)
		}
	}
	if err := s.spool.Append(ctx, object); err != nil {
		if ctx.Err() != nil {
			// The run is being stopped; the failed append is the stop, not
			// a store failure (finish classifies). The §9.1-item-3 entry
			// obligation, discharged for DM-3: the append's DB operation
			// runs on the store's detached opCtx (runstore opCtx — added at
			// the DM-2 fix-round blocker for the connection leak), so a
			// suspend can never truncate a row mid-write; cancellation lands
			// only BETWEEN objects, at this sink's own ctx checks. That
			// whole-rows-only property is now load-bearing, not incidental:
			// a partial spool IS replayed (the crawl resume extends it), and
			// its partiality must always be at an object boundary.
			return ctx.Err()
		}
		// A spool that cannot absorb is the run store failing: fatal, §7.2.
		s.run.report(importv2.Fatal(importv2.IssueStoreError, fmt.Errorf("spool: %w", err)))
		return s.errIfAborting(ctx)
	}
	// Counted only after the row is durable: fetching progress IS the
	// recording (§8.3), so a row that failed to land must not be reported
	// as fetched. The backstop above deliberately does not count — those
	// rows are in spoolPass's census seed already.
	s.run.countObject(object)
	return nil
}

// drainFile downloads/copies the file source into the spill dir and rewrites
// the source to a plain path. This covers ON-DISK paths too, not only Open
// closures (review Class D, reversing the spec's §4.1 "loose files are
// spooled as paths, not copied" cost decision): a path into the user's tree
// serialized into the spool violates the no-source resume invariant — the
// tree can be gone by the time a resumed pass 3 uploads, and §8.1's
// headline property is "the run dir alone suffices". Synchronous by design
// for DM-1: correctness first — the bounded overlap pool (DM spec §4.1,
// concurrency ~4) is a wall-clock optimization deliberately deferred.
func (s *spoolSink) drainFile(ctx context.Context, object *importv2.Object) error {
	open := object.File.Open
	if open == nil {
		sourcePath := object.File.Path
		open = func(context.Context) (io.ReadCloser, error) { return os.Open(sourcePath) }
	}
	reader, err := open(ctx)
	if err != nil {
		return fmt.Errorf("open file source: %w", err)
	}
	defer reader.Close()
	spillFile, err := os.CreateTemp(s.spillDir, "spool-*-"+sanitizeBaseName(object.File.Name))
	if err != nil {
		return fmt.Errorf("create spill file: %w", err)
	}
	copied, err := io.Copy(spillFile, ctxReader{ctx: ctx, r: reader})
	// Reported whatever the copy's outcome: the bytes crossed the wire, and
	// a half-downloaded 2 GB file is precisely the case bytesDone exists to
	// explain (§15.2 — 500 small files and one huge one behave nothing
	// alike).
	s.run.deps.Reporter.Bytes(copied)
	closeErr := spillFile.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(spillFile.Name())
		return fmt.Errorf("spill file source: %w", err)
	}
	object.File.Path = spillFile.Name()
	object.File.Open = nil
	return nil
}

// sanitizeBaseName keeps only the final path element so a hostile name can
// never steer the spill path (mirrors persist's spill discipline).
func sanitizeBaseName(name string) string {
	base := filepath.Base(filepath.FromSlash(name))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "file"
	}
	return base
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (r ctxReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func (s *spoolSink) errIfAborting(ctx context.Context) error {
	if s.run.fatalIssue() != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		return context.Canceled
	}
	return nil
}

func (s *spoolSink) Issue(issue importv2.Issue) {
	s.run.report(issue)
}

func (s *spoolSink) Claim(ctx context.Context, claim importv2.IdentityClaim) error {
	if err := s.run.deps.Identity.Claim(ctx, claim); err != nil {
		return err
	}
	// Write-ahead order (review P0-D): the claim must be DURABLE before any
	// spool row that depends on it, and the converter appends the claimed
	// entity's object right after this call while Spool.Append commits
	// immediately. The batch-at-pass-end justification ("an unflushed
	// batch's loss is harmless — the resumed pass simply re-mints",
	// identity claimBatchSize) expired when DM-3 made the spool a durable
	// artifact a resume replays: a spool row whose claim was lost fails the
	// resumed pass 3 on 'object was not claimed in pass 1'. Late claims are
	// rare (capped second-chance discovery), so the per-claim flush costs
	// one small tx each; pass-1 claims keep the batch — they all flush
	// before pass 2 appends anything.
	if err := s.run.deps.Identity.FlushClaims(ctx); err != nil {
		return err
	}
	s.run.noteClaimed(claim.SourceKey)
	s.run.deps.Reporter.Discovered(importv2.KindPage, 1)
	return nil
}

func (s *spoolSink) Phase(p importv2.Phase) { s.run.deps.Reporter.Phase(p) }

func (s *spoolSink) Item(item importv2.DisplayText) { s.run.deps.Reporter.Item(item) }

// spoolPass is pass 2: fetch, convert, spool — nothing enters the space.
func (r *run) spoolPass(ctx context.Context, converter importv2.Converter, spool Spool) importv2.RootSpec {
	r.deps.Reporter.Phase(importv2.PhaseFetching)
	// A resumed crawl inherits the rows a previous incarnation recorded and
	// its converter SKIPS them, so without this seed the fetch counter
	// would restart at zero and the surface would claim the hours already
	// spent had been lost. Advisory: a census failure costs telemetry only
	// (the replay reads the same rows and fails loudly if they are gone).
	if pages, files, _, err := spool.Census(ctx); err == nil {
		r.deps.Reporter.Completed(importv2.KindPage, int64(pages))
		r.deps.Reporter.Completed(importv2.KindFile, int64(files))
	}
	sink := &spoolSink{run: r, spool: spool, spillDir: r.deps.SpillDir}
	var rootSpec importv2.RootSpec
	var convertErr error
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				convertErr = importv2.Fatal(importv2.IssueInvariant, panicError("converter", rec))
			}
		}()
		rootSpec, convertErr = converter.Convert(ctx, sink)
	}()
	if convertErr != nil && r.fatalIssue() == nil {
		r.report(classifyFatal(convertErr, importv2.IssueSourceInvalid))
	}
	// Late (second-chance) claims buffered during pass 2 flush with pass 2.
	if err := r.deps.Identity.FlushClaims(ctx); err != nil && r.fatalIssue() == nil {
		r.report(classifyFatal(err, importv2.IssueStoreError))
	}
	return rootSpec
}

// spoolReplayConverter adapts the spool back onto the Converter seam: pass 3
// is the existing streaming pipeline fed by a recording — deterministic by
// construction, definitions-before-use preserved as recorded order.
type spoolReplayConverter struct {
	spool Spool
}

func (c *spoolReplayConverter) Name() string { return "materialize" }

func (c *spoolReplayConverter) EnumerateIdentities(ctx context.Context, yield func(importv2.IdentityClaim) error) error {
	return nil // pass 1 already ran against the real converter
}

func (c *spoolReplayConverter) Convert(ctx context.Context, sink importv2.Sink) (importv2.RootSpec, error) {
	err := c.spool.Replay(ctx, func(o *importv2.Object) error {
		return sink.Object(ctx, o)
	})
	if err != nil {
		if ctx.Err() != nil {
			// The replay died OF the stop: sqlite aborts in-flight reads on
			// a cancelled ctx with its own 'interrupted' error, which is not
			// errors.Is-ctx-shaped — returned raw it classified as a bogus
			// (and durably recorded) sourceInvalid fatal. The stop
			// classification must reach this exit like every other
			// (Invariant 1).
			return importv2.RootSpec{}, ctx.Err()
		}
		// A replay failure is OUR storage failing, not the user's source
		// (the source was fine — pass 2 finished): classify accordingly.
		err = importv2.Fatal(importv2.IssueStoreError, err)
	}
	return importv2.RootSpec{}, err
}
