package persist

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
)

type ledgerCall struct {
	kind        string
	sourceKey   string
	objectId    string
	preExisting bool
}

type fakeLedger struct {
	calls []ledgerCall
	err   error
}

func (l *fakeLedger) RecordCreated(_ context.Context, sourceKey, objectId string) error {
	l.calls = append(l.calls, ledgerCall{kind: "created", sourceKey: sourceKey, objectId: objectId})
	return l.err
}

func (l *fakeLedger) RecordUpdated(_ context.Context, sourceKey, objectId string) error {
	l.calls = append(l.calls, ledgerCall{kind: "updated", sourceKey: sourceKey, objectId: objectId})
	return l.err
}

func (l *fakeLedger) RecordFile(_ context.Context, sourceKey, objectId string, preExisting bool) error {
	l.calls = append(l.calls, ledgerCall{kind: "file", sourceKey: sourceKey, objectId: objectId, preExisting: preExisting})
	return l.err
}

func TestJournalLedger(t *testing.T) {
	ctx := context.Background()

	t.Run("effects write through to the ledger with source keys", func(t *testing.T) {
		// given
		ledger := &fakeLedger{}
		journal := NewJournalWithLedger(ledger)

		// when
		require.NoError(t, journal.CreatedObject(ctx, "page-1", "obj-1"))
		require.NoError(t, journal.UpdatedObject(ctx, "page-2", "obj-2"))
		require.NoError(t, journal.CreatedFile(ctx, "file-1", "file-obj-1", true))

		// then
		assert.Equal(t, []ledgerCall{
			{kind: "created", sourceKey: "page-1", objectId: "obj-1"},
			{kind: "updated", sourceKey: "page-2", objectId: "obj-2"},
			{kind: "file", sourceKey: "file-1", objectId: "file-obj-1", preExisting: true},
		}, ledger.calls)
	})

	t.Run("a ledger write failure is fatal but keeps the memory record", func(t *testing.T) {
		// given: §7.2 — a run that cannot journal must not keep creating
		// objects, yet the effect that just happened must stay compensable.
		ledger := &fakeLedger{err: assert.AnError}
		journal := NewJournalWithLedger(ledger)

		// when
		err := journal.CreatedObject(ctx, "page-1", "obj-1")

		// then
		require.Error(t, err)
		issue := importv2.AsIssue(err, importv2.SeverityWarning, importv2.IssueObjectFailed)
		assert.Equal(t, importv2.SeverityFatal, issue.Severity)
		assert.Equal(t, importv2.IssueStoreError, issue.Code)

		objects := &fakeObjects{}
		result := journal.Compensate(ctx, objects)
		assert.Equal(t, []string{"obj-1"}, objects.deleted)
		assert.Equal(t, 1, result.Compensated)
	})

	t.Run("the volatile journal reports no errors", func(t *testing.T) {
		// given
		journal := NewJournal()

		// when / then
		assert.NoError(t, journal.CreatedObject(ctx, "page-1", "obj-1"))
		assert.NoError(t, journal.UpdatedObject(ctx, "page-2", "obj-2"))
		assert.NoError(t, journal.CreatedFile(ctx, "file-1", "file-obj-1", false))
	})
}

func TestCompensateIds(t *testing.T) {
	t.Run("deletes in the given order, created before files", func(t *testing.T) {
		// given
		objects := &fakeObjects{}

		// when
		result := CompensateIds(context.Background(), objects,
			[]string{"obj-2", "obj-1"}, []string{"file-obj-1"}, []string{"updated-1"})

		// then
		assert.Equal(t, []string{"obj-2", "obj-1", "file-obj-1"}, objects.deleted)
		assert.Equal(t, 3, result.Compensated)
		assert.Zero(t, result.Leaked)
		assert.Equal(t, []string{"updated-1"}, result.Uncovered)
	})

	t.Run("an already-gone object counts as compensated, not leaked", func(t *testing.T) {
		// given: compensation must be idempotent — a crash mid-cleanup means
		// the sweep re-runs it against objects already deleted (spec §6.5).
		objects := &fakeObjects{failIds: map[string]error{
			"gone-1": fmt.Errorf("resolve spaceID: %w", domain.ErrObjectNotFound),
			"bad":    assert.AnError,
		}}

		// when
		result := CompensateIds(context.Background(), objects,
			[]string{"gone-1", "obj-1", "bad"}, nil, nil)

		// then
		assert.Equal(t, 2, result.Compensated)
		assert.Equal(t, 1, result.Leaked)
		require.Len(t, result.Issues, 1)
		assert.Equal(t, importv2.IssueStoreError, result.Issues[0].Code)
	})
}
