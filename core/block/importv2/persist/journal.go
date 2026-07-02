package persist

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/anytype-heart/core/block/importv2"
)

// LinkQuerier reports whether other objects link to an id — the guard that
// keeps compensation from deleting a deduped file object that pre-dates the
// run. Satisfied by spaceindex.Store.
type LinkQuerier interface {
	GetInboundLinksById(id string) ([]string, error)
}

// Journal records every effect of a run, in order, for compensation.
// Safe for concurrent worker use.
type Journal struct {
	mu      sync.Mutex
	created []string
	files   []string
	updated []string
}

func NewJournal() *Journal {
	return &Journal{}
}

func (j *Journal) CreatedObject(id string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.created = append(j.created, id)
}

func (j *Journal) CreatedFile(id string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.files = append(j.files, id)
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

// Compensate deletes every object and file created by this run, newest
// first. File objects are deleted only when nothing outside the run links to
// them (a deduped upload may have returned a pre-existing file object).
// Runs on its own context so user cancellation doesn't abort the cleanup.
func (j *Journal) Compensate(ctx context.Context, objects ObjectAccess, links LinkQuerier) CompensationResult {
	j.mu.Lock()
	created := append([]string(nil), j.created...)
	files := append([]string(nil), j.files...)
	updated := append([]string(nil), j.updated...)
	j.mu.Unlock()

	result := CompensationResult{Uncovered: updated}
	for i := len(created) - 1; i >= 0; i-- {
		j.deleteOne(created[i], objects, &result)
	}
	for i := len(files) - 1; i >= 0; i-- {
		id := files[i]
		// A content-deduped upload can return a file object that pre-dates
		// the run; anything still linking TO it means it is not ours to
		// delete. (Inbound, not outbound: a file object links to nothing,
		// so an outbound check would approve every deletion.)
		inbound, err := links.GetInboundLinksById(id)
		if err == nil && len(inbound) > 0 {
			continue
		}
		j.deleteOne(id, objects, &result)
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
