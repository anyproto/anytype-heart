package persist

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
)

// ledgerWriteTimeout bounds one detached effect write. Measured cost is
// sub-millisecond (spec §8); the timeout only guards a pathological disk.
const ledgerWriteTimeout = 10 * time.Second

// EffectLedger is the durable write-through seam behind the journal
// (docs/superpowers/specs/2026-08-13-importv2-durable-queue-design.md §5.1),
// implemented by runstore.Store. Every effect is recorded here as it
// happens so a crash keeps the run compensable.
type EffectLedger interface {
	RecordCreated(ctx context.Context, sourceKey, objectId string) error
	RecordUpdated(ctx context.Context, sourceKey, objectId string) error
	RecordFile(ctx context.Context, sourceKey, objectId string, preExisting bool) error
}

// Journal records every effect of a run, in order, for compensation.
// Safe for concurrent worker use. With a ledger attached, effects
// additionally write through to durable storage; a ledger failure is
// returned as a fatal issue (spec §7.2 — a run that cannot journal must not
// keep creating objects) while the in-memory record is kept, so in-process
// compensation still covers the effect that just happened.
type Journal struct {
	ledger EffectLedger // nil => volatile (tests, sync callers)

	mu      sync.Mutex
	created []string
	// ownedFiles are file objects the run's uploads brought into existence;
	// matchedFiles pre-dated the run (a content-deduped upload returned an
	// existing object) and are never compensation-deleted. The
	// classification happens at upload time — it cannot be reconstructed at
	// compensation time, when the run's own objects may already be indexed.
	ownedFiles   []string
	matchedFiles []string
	updated      []string
}

func NewJournal() *Journal {
	return &Journal{}
}

func NewJournalWithLedger(ledger EffectLedger) *Journal {
	return &Journal{ledger: ledger}
}

// Record methods deliberately take no context: the effect has already
// happened in the user's space, so its record must be written even — and
// especially — when the run context is already dead (shutdown is exactly
// when the next start's sweep will compensate FROM this ledger). Writes run
// on a detached, time-bounded context instead.

func (j *Journal) CreatedObject(sourceKey, id string) error {
	j.mu.Lock()
	j.created = append(j.created, id)
	j.mu.Unlock()
	if j.ledger == nil {
		return nil
	}
	return j.record(func(ctx context.Context) error {
		return j.ledger.RecordCreated(ctx, sourceKey, id)
	})
}

// CreatedFile records an upload outcome; preExisting marks a content-dedup
// hit on an object that already lived in the space.
func (j *Journal) CreatedFile(sourceKey, id string, preExisting bool) error {
	j.mu.Lock()
	if preExisting {
		j.matchedFiles = append(j.matchedFiles, id)
	} else {
		j.ownedFiles = append(j.ownedFiles, id)
	}
	j.mu.Unlock()
	if j.ledger == nil {
		return nil
	}
	return j.record(func(ctx context.Context) error {
		return j.ledger.RecordFile(ctx, sourceKey, id, preExisting)
	})
}

func (j *Journal) UpdatedObject(sourceKey, id string) error {
	j.mu.Lock()
	j.updated = append(j.updated, id)
	j.mu.Unlock()
	if j.ledger == nil {
		return nil
	}
	return j.record(func(ctx context.Context) error {
		return j.ledger.RecordUpdated(ctx, sourceKey, id)
	})
}

// record runs one ledger write on its own bounded context, detached from
// any run cancellation.
func (j *Journal) record(write func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), ledgerWriteTimeout)
	defer cancel()
	return ledgerIssue(write(ctx))
}

func (j *Journal) Updated() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]string(nil), j.updated...)
}

// ledgerIssue wraps a durable-write failure as a fatal store issue: the
// single abort predicate then stops the run regardless of mode.
func ledgerIssue(err error) error {
	if err == nil {
		return nil
	}
	return importv2.Fatal(importv2.IssueStoreError, fmt.Errorf("journal effect: %w", err))
}

// CompensationResult reports what the abort cleanup achieved. Updated
// objects are deliberately not restored (postponed by design decision —
// docs/ImportV2Design.md §13): they are listed so the result can say so.
type CompensationResult struct {
	Compensated int
	Leaked      int
	Uncovered   []string // updated objects, reported not rolled back
	Issues      []importv2.Issue
}

// Compensate deletes every object and file the run brought into existence,
// newest first. Pre-existing (deduped) file objects are never touched —
// deleting a user's file because an aborted import happened to reference it
// is the one unrecoverable outcome. (An inbound-link check cannot arbitrate
// this: the run's own just-deleted referencers linger in the index, so it
// would both leak owned files and still depend on index freshness.)
// Runs on its own context so user cancellation doesn't abort the cleanup.
func (j *Journal) Compensate(ctx context.Context, objects ObjectAccess) CompensationResult {
	j.mu.Lock()
	created := newestFirst(j.created)
	owned := newestFirst(j.ownedFiles)
	updated := append([]string(nil), j.updated...)
	j.mu.Unlock()
	return CompensateIds(ctx, objects, created, owned, updated)
}

func newestFirst(ids []string) []string {
	reversed := make([]string, 0, len(ids))
	for i := len(ids) - 1; i >= 0; i-- {
		reversed = append(reversed, ids[i])
	}
	return reversed
}

// CompensateIds is the one compensation implementation, shared by the
// in-process journal and the startup sweep's crash path (spec §5.1). Ids
// are expected newest-first (runstore.CompensationInputs' order). An
// already-gone object counts as compensated, not leaked: compensation must
// be idempotent so a crash mid-cleanup can simply re-run it (§6.5).
func CompensateIds(ctx context.Context, objects ObjectAccess, created, ownedFiles, updated []string) CompensationResult {
	result := CompensationResult{Uncovered: updated}
	remaining := append(append([]string(nil), created...), ownedFiles...)
	for i, id := range remaining {
		// A3: the context is a real bound between deletes (each individual
		// DeleteObject still has no ctx — pre-existing seam limitation).
		// Everything not reached is leaked, loudly, so the run dir is kept
		// and the next start retries.
		if err := ctx.Err(); err != nil {
			left := len(remaining) - i
			result.Leaked += left
			result.Issues = append(result.Issues, importv2.Issue{
				Severity: importv2.SeverityWarning,
				Code:     importv2.IssueStoreError,
				Message:  fmt.Sprintf("compensation interrupted with %d deletes remaining", left),
				Err:      err,
			})
			return result
		}
		deleteOne(id, objects, &result)
	}
	return result
}

func deleteOne(id string, objects ObjectAccess, result *CompensationResult) {
	err := objects.DeleteObject(id)
	if err == nil || isAlreadyGone(err) {
		result.Compensated++
		return
	}
	result.Leaked++
	result.Issues = append(result.Issues, importv2.Issue{
		Severity: importv2.SeverityWarning,
		Code:     importv2.IssueStoreError,
		ObjectId: id,
		Message:  "compensation: delete created object",
		Err:      fmt.Errorf("delete %s: %w", id, err),
	})
}

// isAlreadyGone recognizes the delete-path shapes of "this object does not
// exist": an id that was never indexed (resolver miss), a tree the space
// does not know, or a tree already deleted by a previous compensation pass.
func isAlreadyGone(err error) bool {
	return errors.Is(err, domain.ErrObjectNotFound) ||
		errors.Is(err, treestorage.ErrUnknownTreeId) ||
		errors.Is(err, spacestorage.ErrTreeStorageAlreadyDeleted)
}
