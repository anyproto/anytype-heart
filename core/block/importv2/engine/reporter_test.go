package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/domain"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

// The producer seam: the engine publishes PER-KIND, PER-PHASE counters.
// The legacy blended `Step(1)` — one tick for a page, a file and a relation
// alike — is what made an honest `pagesDone` impossible to put on the wire,
// so the seam is redesigned before the emitter sits on it.

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

// Bytes is a LEVEL, so the reporter keeps the newest value.
func (r *recordingReporter) Bytes(total int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bytes = total
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

// createdLevels returns the published sequence in arrival order — the shape
// a consumer of the LEVEL actually sees.
func (r *recordingReporter) createdLevels() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int64(nil), r.created...)
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

	t.Run("bytes are a level on disk, so a resumed crawl keeps its predecessor's", func(t *testing.T) {
		// given: a spill dir a previous incarnation already downloaded into.
		// bytesDone is MEASURED, not accumulated, precisely so the number a
		// resumed run pushes and the number its dir answers when polled are
		// the same number.
		fx := newEngineFixture(t)
		reporter := &recordingReporter{}
		fx.deps.Reporter = reporter
		inherited := filepath.Join(fx.deps.SpillDir, importv2.SpoolSpillPrefix+"999-old.png")
		require.NoError(t, os.WriteFile(inherited, []byte("previously downloaded"), 0o600))
		converter := &scriptConverter{objects: []*importv2.Object{
			pageObj("page-1", false), fileObj("img.png"),
		}}

		// when
		result := Run(context.Background(), importv2.Request{}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		assert.Equal(t, importv2.SpillBytes(fx.deps.SpillDir), reporter.bytes,
			"the run's spool db and the persister's own spills must not be counted")
		assert.Equal(t, int64(len("previously downloaded")+len("bytes-img.png")), reporter.bytes)
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

// TestCreatedLevelIsMonotone pins the LEVEL contract at the producer (review
// item 7). Reporter.Created takes a level rather than a delta because a
// resumed run starts at its ledger's count — but the persist workers publish
// created.Add(1) with nothing ordering the increment against the publish, so
// eight of them interleave and the LOWER level lands last. Every consumer
// sees it: the wire's objectsCreated (the "stop and remove the N objects
// created", which the dormant poll of the same run answers from the ledger),
// and the existing per-kind test above, which was flaky at ~7 runs in 1000
// under -race for exactly this reason.
func TestCreatedLevelIsMonotone(t *testing.T) {
	t.Run("the level published from eight workers never walks backwards", func(t *testing.T) {
		// given: enough objects that the workers really overlap
		fx := newEngineFixture(t)
		reporter := &recordingReporter{}
		fx.deps.Reporter = reporter
		objects := make([]*importv2.Object, 300)
		for i := range objects {
			objects[i] = pageObj(fmt.Sprintf("p-%03d.md", i), false)
		}
		converter := &scriptConverter{objects: objects}

		// when
		result := Run(context.Background(), importv2.Request{Mode: importv2.ModeContinueOnError}, converter, fx.deps)

		// then
		require.NoError(t, result.Err)
		require.Equal(t, int64(300), result.Created)
		levels := reporter.createdLevels()
		require.NotEmpty(t, levels)
		for i := 1; i < len(levels); i++ {
			require.GreaterOrEqual(t, levels[i], levels[i-1],
				"a level that goes backwards is a counter the user watches count down")
		}
		assert.Equal(t, result.Created, levels[len(levels)-1],
			"the last level published IS the run's own count")
	})
}
