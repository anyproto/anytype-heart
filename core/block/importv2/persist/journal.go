package persist

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/anytype-heart/core/block/importv2"
)

// Journal records every effect of a run, in order, for compensation.
// Safe for concurrent worker use.
type Journal struct {
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

func (j *Journal) CreatedObject(id string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.created = append(j.created, id)
}

// CreatedFile records an upload outcome; preExisting marks a content-dedup
// hit on an object that already lived in the space.
func (j *Journal) CreatedFile(id string, preExisting bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if preExisting {
		j.matchedFiles = append(j.matchedFiles, id)
		return
	}
	j.ownedFiles = append(j.ownedFiles, id)
}

func (j *Journal) UpdatedObject(id string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.updated = append(j.updated, id)
}

func (j *Journal) Updated() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]string(nil), j.updated...)
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
	created := append([]string(nil), j.created...)
	owned := append([]string(nil), j.ownedFiles...)
	updated := append([]string(nil), j.updated...)
	j.mu.Unlock()

	result := CompensationResult{Uncovered: updated}
	for i := len(created) - 1; i >= 0; i-- {
		j.deleteOne(created[i], objects, &result)
	}
	for i := len(owned) - 1; i >= 0; i-- {
		j.deleteOne(owned[i], objects, &result)
	}
	return result
}

func (j *Journal) deleteOne(id string, objects ObjectAccess, result *CompensationResult) {
	if err := objects.DeleteObject(id); err != nil {
		result.Leaked++
		result.Issues = append(result.Issues, importv2.Issue{
			Severity: importv2.SeverityWarning,
			Code:     importv2.IssueStoreError,
			ObjectId: id,
			Message:  "compensation: delete created object",
			Err:      fmt.Errorf("delete %s: %w", id, err),
		})
		return
	}
	result.Compensated++
}
