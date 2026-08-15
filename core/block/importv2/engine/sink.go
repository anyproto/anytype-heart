package engine

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/persist"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

// engineSink receives the converter's stream. It runs on the converter
// goroutine, so identity assignment happens in stream order — the property
// that makes definitions-before-use sufficient for reference resolution.
type engineSink struct {
	run      *run
	objectCh chan work
	fileCh   chan work
}

func (s *engineSink) Object(ctx context.Context, object *importv2.Object) error {
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

	if s.skipResumed(object) {
		return nil
	}

	w := work{object: object}
	switch {
	case isFileClass(object.SbType):
		// Register in stream order so later references find the future.
		s.run.deps.Identity.RegisterFile(object.SourceKey)
		w.isFile = true
	case isDerivedClass(object.SbType):
		// An option's relation key must be final before dedup matching
		// (v1's phase-1 relations→options ordering, stream-order here).
		if object.SbType == coresb.SmartBlockTypeRelationOption {
			if key := object.Payload.Details.GetString(bundle.RelationKeyRelationKey); key != "" {
				if finalKey, ok := s.run.deps.Keys.FinalKey(key); ok && finalKey != key {
					object.Payload.Details.SetString(bundle.RelationKeyRelationKey, finalKey)
				}
			}
		}
		assignment, err := s.run.deps.Identity.AssignDerived(ctx, object)
		if err != nil {
			issue := importv2.ObjectError(importv2.IssueObjectFailed, object.SourceKey, fmt.Errorf("assign derived identity: %w", err))
			s.run.failed.Add(1)
			s.run.report(issue)
			return s.errIfAborting(ctx)
		}
		s.run.registerRelationMeta(object, assignment)
		w.target = persist.Target{Id: assignment.Id, IsExisting: assignment.IsExisting, Payload: assignment.Payload}
	default:
		assignment, err := s.run.deps.Identity.Assign(object.SourceKey)
		if err != nil {
			issue := importv2.ObjectError(importv2.IssueInvariant, object.SourceKey, fmt.Errorf("assign identity: %w", err))
			s.run.failed.Add(1)
			s.run.report(issue)
			return s.errIfAborting(ctx)
		}
		w.target = persist.Target{Id: assignment.Id, IsExisting: assignment.IsExisting, Payload: assignment.Payload}
	}

	if object.IsRootCandidate {
		// Stream order keeps root-collection membership deterministic.
		s.run.rootMu.Lock()
		s.run.rootCandidates = append(s.run.rootCandidates, object.SourceKey)
		s.run.rootMu.Unlock()
	}

	lane := s.objectCh
	if w.isFile {
		lane = s.fileCh
	}
	s.run.deps.Gauge(1)
	select {
	case lane <- w:
		return nil
	case <-ctx.Done():
		s.run.deps.Gauge(-1)
		if w.isFile {
			s.run.deps.Identity.CompleteFile(object.SourceKey, "", ctx.Err())
		}
		return ctx.Err()
	}
}

// skipResumed drops a replayed row a previous incarnation already finished
// (its ledger entry is terminal / its file row is done): the object is
// acknowledged — root-collection membership recorded in stream order,
// progress stepped — and NOT re-persisted or re-counted (the counters were
// rehydrated from the ledger). Derived-class rows are exempt: their
// re-derivation reseeds the format registry and key table, which are
// process memory a crash lost, and converges via dedup or deterministic
// derivation. File rows ride the rehydrated already-resolved future, so
// RegisterFile is a no-op and later references resolve instantly.
func (s *engineSink) skipResumed(object *importv2.Object) bool {
	resume := s.run.resume
	if resume == nil || isDerivedClass(object.SbType) {
		return false
	}
	if _, skip := resume.SkipKeys[object.SourceKey]; !skip {
		return false
	}
	if isFileClass(object.SbType) {
		s.run.deps.Identity.RegisterFile(object.SourceKey)
	}
	if object.IsRootCandidate {
		s.run.rootMu.Lock()
		s.run.rootCandidates = append(s.run.rootCandidates, object.SourceKey)
		s.run.rootMu.Unlock()
	}
	// The row IS done — a previous incarnation's ledger says so — so it
	// counts, through the same classification every other counted object
	// goes through.
	s.run.countObject(object)
	return true
}

// errIfAborting turns a reported per-object issue into a converter stop
// signal only when the run is aborting; under continue-on-error the
// converter keeps streaming.
func (s *engineSink) errIfAborting(ctx context.Context) error {
	if s.run.fatalIssue() != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		return context.Canceled
	}
	return nil
}

func (s *engineSink) Issue(issue importv2.Issue) {
	s.run.report(issue)
}

// Claim serves second-chance discovery: identities found only in pass 2 join
// the index (and the progress total) exactly like pass-1 claims.
func (s *engineSink) Claim(ctx context.Context, claim importv2.IdentityClaim) error {
	if err := s.run.deps.Identity.Claim(ctx, claim); err != nil {
		return err
	}
	s.run.noteClaimed(claim.SourceKey)
	s.run.deps.Reporter.Discovered(importv2.KindPage, 1)
	return nil
}

func (s *engineSink) Phase(p importv2.Phase) { s.run.deps.Reporter.Phase(p) }

func (s *engineSink) Item(item importv2.DisplayText) { s.run.deps.Reporter.Item(item) }
