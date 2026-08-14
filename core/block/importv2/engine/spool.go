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
	s.run.deps.Gauge(1)
	defer s.run.deps.Gauge(-1)
	if object.File != nil && object.File.Open != nil && s.spillDir != "" {
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
		// A spool that cannot absorb is the run store failing: fatal, §7.2.
		s.run.report(importv2.Fatal(importv2.IssueStoreError, fmt.Errorf("spool: %w", err)))
		return s.errIfAborting(ctx)
	}
	return nil
}

// drainFile downloads/copies the file source into the spill dir and rewrites
// the source to a plain path. Synchronous by design for DM-1: correctness
// first — the bounded overlap pool (DM spec §4.1, concurrency ~4) is a
// wall-clock optimization deliberately deferred and reported as such.
func (s *spoolSink) drainFile(ctx context.Context, object *importv2.Object) error {
	reader, err := object.File.Open(ctx)
	if err != nil {
		return fmt.Errorf("open file source: %w", err)
	}
	defer reader.Close()
	spillFile, err := os.CreateTemp(s.spillDir, "spool-*-"+sanitizeBaseName(object.File.Name))
	if err != nil {
		return fmt.Errorf("create spill file: %w", err)
	}
	_, err = io.Copy(spillFile, ctxReader{ctx: ctx, r: reader})
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
	s.run.deps.Reporter.AddTotal(1)
	return nil
}

func (s *spoolSink) Progress(delta int64) {
	s.run.deps.Reporter.Step(delta)
}

// spoolPass is pass 2: fetch, convert, spool — nothing enters the space.
func (r *run) spoolPass(ctx context.Context, converter importv2.Converter, spool Spool) importv2.RootSpec {
	r.deps.Reporter.Phase("Fetching content")
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
	return importv2.RootSpec{}, c.spool.Replay(ctx, func(o *importv2.Object) error {
		return sink.Object(ctx, o)
	})
}
