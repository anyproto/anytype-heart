package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

// Resume is pass 3 restarted from a recorded spool (DM spec §8.1): no
// pass 1, no pass 2, no converter — the run is a function of (run dir,
// space). These tests drive engine.Resume over a pre-filled durable spool
// and assert the restart semantics the crash tests then prove end to end.

// fillSpool appends the objects to the fixture's spool as pass 2 would.
func fillSpool(t *testing.T, fx *engineFixture, objects ...*importv2.Object) {
	t.Helper()
	for _, object := range objects {
		require.NoError(t, fx.deps.Spool.Append(context.Background(), object))
	}
}

func resumeRequest() importv2.Request {
	return importv2.Request{SpaceID: "space-1", Mode: importv2.ModeContinueOnError}
}

func TestResumeSkipsTerminalRows(t *testing.T) {
	t.Run("terminal rows are acknowledged and dropped; the rest re-persist", func(t *testing.T) {
		// given: a spool of three pages, one already persisted by a previous
		// incarnation
		fx := newEngineFixture(t)
		fillSpool(t, fx, pageObj("a", false), pageObj("b", false), pageObj("c", false))

		// when
		result := Resume(context.Background(), resumeRequest(), fx.deps, &ResumeState{
			ConverterName: "Markdown",
			SkipKeys:      map[string]struct{}{"a": {}},
			Created:       1, // a's rehydrated count
		})

		// then: only b and c went through persist; the counters continue
		// from the ledger instead of snapping to zero
		require.NoError(t, result.Err)
		assert.Equal(t, []string{"b", "c"}, fx.persister.persisted)
		assert.Equal(t, int64(3), result.Created)
	})

	t.Run("a derived-class row is never skipped: it re-derives to reseed the registries", func(t *testing.T) {
		// given — relation formats and the key table are seeded from the
		// defining object in stream order; a skipped definition would leave
		// pass 3 without them, so derived rows always re-emit (their
		// re-persist converges via dedup/deterministic derivation)
		fx := newEngineFixture(t)
		relation := &importv2.Object{
			SourceKey: "rel-1",
			SbType:    coresb.SmartBlockTypeRelation,
			Payload:   &importv2.Snapshot{Details: domain.NewDetails(), Key: "rel_key"},
		}
		fillSpool(t, fx, relation, pageObj("b", false))

		// when: the ledger says rel-1 is terminal
		result := Resume(context.Background(), resumeRequest(), fx.deps, &ResumeState{
			ConverterName: "Markdown",
			SkipKeys:      map[string]struct{}{"rel-1": {}},
		})

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, []string{"rel-1", "b"}, fx.persister.persisted,
			"the derived definition must replay despite its terminal ledger row")
	})

	t.Run("a done file row is dropped without re-upload", func(t *testing.T) {
		// given
		fx := newEngineFixture(t)
		fillSpool(t, fx, fileObj("img.png"), pageObj("b", false))

		// when
		result := Resume(context.Background(), resumeRequest(), fx.deps, &ResumeState{
			ConverterName: "Markdown",
			SkipKeys:      map[string]struct{}{"img.png": {}},
		})

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, []string{"b"}, fx.persister.persisted)
	})
}

func TestResumeSkipKeepsRootMembership(t *testing.T) {
	t.Run("skipped rows keep their root-collection membership in stream order", func(t *testing.T) {
		// given: two root candidates, the first already persisted
		fx := newEngineFixture(t)
		factory := &fakeCollectionFactory{}
		fx.deps.Collection = factory
		fillSpool(t, fx, pageObj("a", true), pageObj("b", true))

		// when
		result := Resume(context.Background(), resumeRequest(), fx.deps, &ResumeState{
			RootSpec:      importv2.RootSpec{CollectionName: "Import"},
			ConverterName: "Markdown",
			SkipKeys:      map[string]struct{}{"a": {}},
		})

		// then: the collection lists BOTH, in recorded order
		require.NoError(t, result.Err)
		assert.Equal(t, []string{"a", "b"}, factory.members,
			"a skipped row's membership must survive the skip")
		assert.NotEmpty(t, result.RootCollectionId)
	})
}

func TestResumeSkipsCompletedFinalize(t *testing.T) {
	t.Run("a recorded root collection short-circuits finalize", func(t *testing.T) {
		// given: the previous incarnation created the root collection; a
		// resumed finalize would mint a second one (the date-suffixed name
		// yields a fresh key every run)
		fx := newEngineFixture(t)
		factory := &fakeCollectionFactory{}
		fx.deps.Collection = factory
		fillSpool(t, fx, pageObj("a", true))

		// when
		result := Resume(context.Background(), resumeRequest(), fx.deps, &ResumeState{
			RootSpec:         importv2.RootSpec{CollectionName: "Import"},
			ConverterName:    "Markdown",
			SkipKeys:         map[string]struct{}{"a": {}},
			RootCollectionId: "root-from-inc-1",
		})

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, "root-from-inc-1", result.RootCollectionId)
		assert.Empty(t, factory.name, "finalize must not build a second collection")
	})

	t.Run("a recorded report page short-circuits the report", func(t *testing.T) {
		// given: issues from the previous incarnation and its persisted
		// report page
		fx := newEngineFixture(t)
		fillSpool(t, fx, pageObj("a", false))

		// when
		result := Resume(context.Background(), resumeRequest(), fx.deps, &ResumeState{
			ConverterName:  "Markdown",
			SkipKeys:       map[string]struct{}{"a": {}},
			ReportObjectId: "report-from-inc-1",
			Issues: []importv2.Issue{{
				Severity: importv2.SeverityWarning,
				Code:     importv2.IssueDataLoss,
				Message:  "recorded in a previous incarnation",
			}},
		})

		// then: the recorded page is the result's report; no new report
		// object was persisted
		require.NoError(t, result.Err)
		assert.Equal(t, "report-from-inc-1", result.ReportObjectId)
		assert.Empty(t, fx.persister.persisted)
		require.Len(t, result.Issues, 1, "rehydrated issues ride the result")
	})
}

func TestResumeEmitsReportForRehydratedIssues(t *testing.T) {
	t.Run("rehydrated issues alone produce a report when none was persisted", func(t *testing.T) {
		// given: the crash landed after issues were recorded but before the
		// report page existed
		fx := newEngineFixture(t)
		fillSpool(t, fx, pageObj("a", false))

		// when
		result := Resume(context.Background(), resumeRequest(), fx.deps, &ResumeState{
			ConverterName: "Markdown",
			Issues: []importv2.Issue{{
				Severity: importv2.SeverityWarning,
				Code:     importv2.IssueDataLoss,
				Message:  "recorded in a previous incarnation",
			}},
		})

		// then
		require.NoError(t, result.Err)
		assert.NotEmpty(t, result.ReportObjectId,
			"pass-2 issues must survive to the pass-3 report across the restart")
	})
}

type interruptedSpool struct {
	memorySpool
}

func (s *interruptedSpool) Replay(ctx context.Context, emit func(o *importv2.Object) error) error {
	return errors.New("sqlite: prepare: interrupted")
}

func TestReplayStopClassification(t *testing.T) {
	t.Run("a replay dying OF the stop classifies as the stop, not the source", func(t *testing.T) {
		// given — found by the crash harness: sqlite aborts in-flight reads
		// on a cancelled ctx with its own 'interrupted' error, which is not
		// errors.Is-ctx-shaped. classifyFatal then recorded a bogus
		// sourceInvalid fatal — durably, so a resumed run rehydrated it into
		// its report. The stop classification must reach this exit too
		// (Invariant 1).
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		converter := &spoolReplayConverter{spool: &interruptedSpool{}}

		// when
		_, err := converter.Convert(ctx, nil)

		// then
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled,
			"a dead-ctx replay failure is the stop; anything else mislabels every suspend mid-materialize")
	})
}

func TestResumeRequiresSpool(t *testing.T) {
	t.Run("resume without a spool is an invariant failure, not a silent no-op", func(t *testing.T) {
		// given
		fx := newEngineFixture(t)
		fx.deps.Spool = nil

		// when
		result := Resume(context.Background(), resumeRequest(), fx.deps, &ResumeState{ConverterName: "Markdown"})

		// then
		require.Error(t, result.Err)
	})
}
