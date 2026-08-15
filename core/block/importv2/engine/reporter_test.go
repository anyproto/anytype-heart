package engine

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

// The §15 producer seam: the engine publishes PER-KIND, PER-PHASE counters.
// The legacy blended `Step(1)` — one tick for a page, a file and a relation
// alike — is what made an honest `pagesDone` impossible to put on the wire,
// so the seam is redesigned before the emitter sits on it (§15.7).

// recordingReporter records the seam's calls with the phase in effect when
// each landed, so tests read the counters the way the emitter does.
type recordingReporter struct {
	mu      sync.Mutex
	phase   importv2.Phase
	phases  []importv2.Phase
	events  []reporterEvent
	items   []importv2.DisplayText
	created []int64
	bytes   int64
}

type reporterEvent struct {
	phase importv2.Phase
	name  string
	kind  importv2.Kind
	delta int64
}

func (r *recordingReporter) Phase(p importv2.Phase) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.phase = p
	r.phases = append(r.phases, p)
}

func (r *recordingReporter) Discovered(kind importv2.Kind, delta int64) {
	r.record("discovered", kind, delta)
}

func (r *recordingReporter) Completed(kind importv2.Kind, delta int64) {
	r.record("completed", kind, delta)
}

func (r *recordingReporter) record(name string, kind importv2.Kind, delta int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, reporterEvent{phase: r.phase, name: name, kind: kind, delta: delta})
}

func (r *recordingReporter) Bytes(delta int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bytes += delta
}

func (r *recordingReporter) Created(count int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created = append(r.created, count)
}

func (r *recordingReporter) Item(item importv2.DisplayText) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, item)
}

// sum totals one counter within one phase, the way the emitter re-bases on
// every phase change.
func (r *recordingReporter) sum(phase importv2.Phase, name string, kind importv2.Kind) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var total int64
	for _, e := range r.events {
		if e.phase == phase && e.name == name && e.kind == kind {
			total += e.delta
		}
	}
	return total
}

func (r *recordingReporter) phaseOrder() []importv2.Phase {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]importv2.Phase(nil), r.phases...)
}

func (r *recordingReporter) lastCreated() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.created) == 0 {
		return 0
	}
	return r.created[len(r.created)-1]
}

func relationObj(key string) *importv2.Object {
	return &importv2.Object{
		SourceKey: key,
		SbType:    coresb.SmartBlockTypeRelation,
		Payload:   &importv2.Snapshot{Key: key, Details: domain.NewDetails()},
	}
}

func TestReporterPerKindCounters(t *testing.T) {
	t.Run("pages, files and definitions are counted apart, in both passes", func(t *testing.T) {
		// given: two claimed pages, one file and one relation definition —
		// the relation is engine bookkeeping, not one of the user's pages
		fx := newEngineFixture(t)
		reporter := &recordingReporter{}
		fx.deps.Reporter = reporter
		converter := &scriptConverter{objects: []*importv2.Object{
			relationObj("rel-status"),
			pageObj("page-1", true),
			fileObj("img.png"),
			pageObj("page-2", true),
		}}

		// when
		result := Run(context.Background(), importv2.Request{}, converter, fx.deps)

		// then: pass 1 counts claims as the fetching denominator
		require.NoError(t, result.Err)
		assert.Equal(t, int64(2), reporter.sum(importv2.PhaseScanning, "discovered", importv2.KindPage))

		// and: pass 2 counts what it spooled, definitions excluded
		assert.Equal(t, int64(2), reporter.sum(importv2.PhaseFetching, "completed", importv2.KindPage))
		assert.Equal(t, int64(1), reporter.sum(importv2.PhaseFetching, "completed", importv2.KindFile))

		// and: pass 3 re-bases on the spool census and counts up to it
		assert.Equal(t, int64(2), reporter.sum(importv2.PhaseCreating, "discovered", importv2.KindPage))
		assert.Equal(t, int64(1), reporter.sum(importv2.PhaseCreating, "discovered", importv2.KindFile))
		assert.Equal(t, int64(2), reporter.sum(importv2.PhaseCreating, "completed", importv2.KindPage))
		assert.Equal(t, int64(1), reporter.sum(importv2.PhaseCreating, "completed", importv2.KindFile))

		// and: the created level matches the run's own count
		assert.Equal(t, result.Created, reporter.lastCreated())

		// and: bytes drained to the spill are reported
		assert.Equal(t, int64(len("bytes-img.png")), reporter.bytes)
	})

	t.Run("the phase sequence is scanning, fetching, creating, finalizing", func(t *testing.T) {
		// given
		fx := newEngineFixture(t)
		reporter := &recordingReporter{}
		fx.deps.Reporter = reporter
		fx.deps.Collection = &fakeCollectionFactory{}
		converter := &scriptConverter{
			objects:  []*importv2.Object{pageObj("page-1", true)},
			rootSpec: importv2.RootSpec{CollectionName: "Test Import"},
		}

		// when
		result := Run(context.Background(), importv2.Request{}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, []importv2.Phase{
			importv2.PhaseScanning,
			importv2.PhaseFetching,
			importv2.PhaseCreating,
			importv2.PhaseFinalizing,
		}, reporter.phaseOrder())
	})
}

// analyzingConverter announces the converter-side plan stage the way the
// real converters do: analyzing, then back to fetching.
type analyzingConverter struct {
	scriptConverter
	item importv2.DisplayText
}

func (c *analyzingConverter) Convert(ctx context.Context, sink importv2.Sink) (importv2.RootSpec, error) {
	sink.Phase(importv2.PhaseAnalyzing)
	sink.Phase(importv2.PhaseFetching)
	sink.Item(c.item)
	return c.scriptConverter.Convert(ctx, sink)
}

func TestReporterConverterSideSignals(t *testing.T) {
	t.Run("a converter announces its own analysis stage and its current item", func(t *testing.T) {
		// given
		fx := newEngineFixture(t)
		reporter := &recordingReporter{}
		fx.deps.Reporter = reporter
		converter := &analyzingConverter{
			scriptConverter: scriptConverter{objects: []*importv2.Object{pageObj("page-1", false)}},
			item:            importv2.DisplayText("Q3 Planning"),
		}

		// when
		result := Run(context.Background(), importv2.Request{}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, []importv2.Phase{
			importv2.PhaseScanning,
			importv2.PhaseFetching,
			importv2.PhaseAnalyzing,
			importv2.PhaseFetching,
			importv2.PhaseCreating,
			importv2.PhaseFinalizing,
		}, reporter.phaseOrder())
		assert.Equal(t, []importv2.DisplayText{"Q3 Planning"}, reporter.items)
	})
}
