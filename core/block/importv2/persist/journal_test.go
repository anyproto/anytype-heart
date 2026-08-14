package persist

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
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
	// ctxStates pins P0-1: every write must arrive on a live, deadline-
	// bounded context detached from the run (a dead run ctx here would mean
	// effect records are lost exactly at shutdown).
	ctxStates []string
}

func ctxState(ctx context.Context) string {
	if ctx.Err() != nil {
		return "dead"
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return "live-bounded"
	}
	return "live-unbounded"
}

func (l *fakeLedger) RecordCreateIntent(ctx context.Context, sourceKey, objectId string) error {
	l.ctxStates = append(l.ctxStates, ctxState(ctx))
	l.calls = append(l.calls, ledgerCall{kind: "intent", sourceKey: sourceKey, objectId: objectId})
	return l.err
}

func (l *fakeLedger) RecordDerivedMatched(ctx context.Context, sourceKey, objectId string) error {
	l.ctxStates = append(l.ctxStates, ctxState(ctx))
	l.calls = append(l.calls, ledgerCall{kind: "derivedMatched", sourceKey: sourceKey, objectId: objectId})
	return l.err
}

func (l *fakeLedger) RecordCreated(ctx context.Context, sourceKey, objectId string) error {
	l.ctxStates = append(l.ctxStates, ctxState(ctx))
	l.calls = append(l.calls, ledgerCall{kind: "created", sourceKey: sourceKey, objectId: objectId})
	return l.err
}

func (l *fakeLedger) RecordUpdated(ctx context.Context, sourceKey, objectId string) error {
	l.ctxStates = append(l.ctxStates, ctxState(ctx))
	l.calls = append(l.calls, ledgerCall{kind: "updated", sourceKey: sourceKey, objectId: objectId})
	return l.err
}

func (l *fakeLedger) RecordFile(ctx context.Context, sourceKey, objectId string, preExisting bool) error {
	l.ctxStates = append(l.ctxStates, ctxState(ctx))
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
		require.NoError(t, journal.CreatedObject("page-1", "obj-1"))
		require.NoError(t, journal.UpdatedObject("page-2", "obj-2"))
		require.NoError(t, journal.CreatedFile("file-1", "file-obj-1", true))

		// then
		assert.Equal(t, []ledgerCall{
			{kind: "created", sourceKey: "page-1", objectId: "obj-1"},
			{kind: "updated", sourceKey: "page-2", objectId: "obj-2"},
			{kind: "file", sourceKey: "file-1", objectId: "file-obj-1", preExisting: true},
		}, ledger.calls)
		assert.Equal(t, []string{"live-bounded", "live-bounded", "live-bounded"}, ledger.ctxStates,
			"ledger writes must run on a detached, time-bounded context (P0-1)")
	})

	t.Run("a ledger write failure is fatal but keeps the memory record", func(t *testing.T) {
		// given: §7.2 — a run that cannot journal must not keep creating
		// objects, yet the effect that just happened must stay compensable.
		ledger := &fakeLedger{err: assert.AnError}
		journal := NewJournalWithLedger(ledger)

		// when
		err := journal.CreatedObject("page-1", "obj-1")

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
		assert.NoError(t, journal.CreatedObject("page-1", "obj-1"))
		assert.NoError(t, journal.UpdatedObject("page-2", "obj-2"))
		assert.NoError(t, journal.CreatedFile("file-1", "file-obj-1", false))
	})

	t.Run("effect rows survive the run context dying", func(t *testing.T) {
		// given — P0-1: the run context is cancelled exactly at shutdown,
		// which is exactly when the effect record matters most: the tree was
		// already created, and the next start's sweep compensates FROM THE
		// LEDGER. The record methods therefore take no caller context at all
		// (writes run detached and time-bounded); this pins that every row
		// lands durably against a real store, whatever the run's state.
		store, err := runstore.Create(context.Background(),
			filepath.Join(t.TempDir(), "run-1"), runstore.Manifest{RunId: "run-1"})
		require.NoError(t, err)
		defer store.Close()
		journal := NewJournalWithLedger(store)

		// when
		require.NoError(t, journal.CreatedObject("page-1", "obj-1"))
		require.NoError(t, journal.UpdatedObject("page-2", "obj-2"))
		require.NoError(t, journal.CreatedFile("file-1", "file-obj-1", false))

		// then: every row is durably present
		inputs, err := store.CompensationInputs(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{"obj-1"}, inputs.Created)
		assert.Equal(t, []string{"obj-2"}, inputs.Updated)
		assert.Equal(t, []string{"file-obj-1"}, inputs.OwnedFiles)
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

func TestCompensationBounded(t *testing.T) {
	t.Run("a dead context stops compensation between deletes; the rest is leaked loudly", func(t *testing.T) {
		// given — A3 (CONFIRMED): compensation ignored its context entirely,
		// so the engine's timeout was dead code and Close's grace no bound.
		ctx, cancel := context.WithCancel(context.Background())
		objects := &fakeObjects{}
		objects.onDelete = func(id string) {
			if id == "obj-3" {
				cancel() // dies after the first delete completes
			}
		}

		// when
		result := CompensateIds(ctx, objects,
			[]string{"obj-3", "obj-2", "obj-1"}, []string{"file-1"}, nil)

		// then
		assert.Equal(t, 1, result.Compensated)
		assert.Equal(t, 3, result.Leaked, "everything not reached counts leaked")
		require.NotEmpty(t, result.Issues)
	})

	t.Run("an already-dead context leaks everything and touches nothing", func(t *testing.T) {
		// given
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		objects := &fakeObjects{}

		// when
		result := CompensateIds(ctx, objects, []string{"a", "b"}, []string{"f"}, nil)

		// then
		assert.Empty(t, objects.deleted)
		assert.Zero(t, result.Compensated)
		assert.Equal(t, 3, result.Leaked)
	})
}
