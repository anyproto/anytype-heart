package adapter

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The §15 push producer: a coalescing emitter over the redesigned reporter
// seam. The properties under test are the ones §15.3 fixes — one event per
// window, but NEVER a delayed calm/alarm edge — plus the ETA's honesty and
// the fact that push and pull are the same builder over the same state.

// captureSink collects emitted events without a clock of its own.
type captureSink struct {
	mu     sync.Mutex
	events []*pb.EventImportStatistic
}

func (c *captureSink) send(e *pb.EventImportStatistic) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *captureSink) all() []*pb.EventImportStatistic {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*pb.EventImportStatistic(nil), c.events...)
}

func (c *captureSink) last() *pb.EventImportStatistic {
	all := c.all()
	if len(all) == 0 {
		return &pb.EventImportStatistic{}
	}
	return all[len(all)-1]
}

func (c *captureSink) len() int { return len(c.all()) }

// fakeClock drives the emitter's window and rate math deterministically.
type fakeClock struct {
	mu sync.Mutex
	at time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{at: time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

// newTestEmitter builds an emitter with a window so long that nothing
// coalesced can escape during a test — every event observed is one the
// rules deliberately let through.
func newTestEmitter(t *testing.T, clock *fakeClock, opts ...func(*statConfig)) (*statEmitter, *captureSink) {
	t.Helper()
	sink := &captureSink{}
	cfg := statConfig{
		importId:   "run-1",
		processId:  "proc-1",
		importType: model.Import_Notion,
		window:     time.Hour,
		now:        clock.now,
		send:       sink.send,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	emitter := newStatEmitter(cfg)
	t.Cleanup(func() { emitter.Close(statVerdict{}) })
	return emitter, sink
}

func TestStatEmitterCoalescing(t *testing.T) {
	t.Run("a burst of counter ticks yields one event, not one per tick", func(t *testing.T) {
		// given: pass 3 rates (50-200 objects/s) would flood a per-item
		// emitter — §15.3's reason for the window
		// The window is an hour of FAKE time on purpose: the real trailing
		// timer then cannot fire inside the test, so "exactly one" is a
		// statement about the rule and not about how loaded the machine is.
		clock := newFakeClock()
		emitter, sink := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseCreating)

		// when: the window from the phase event has elapsed, then a burst
		clock.advance(2 * time.Hour)
		before := sink.len()
		for i := 0; i < 500; i++ {
			emitter.Completed(importv2.KindPage, 1)
		}

		// then: the first tick rides the open window, the other 499 coalesce
		assert.Equal(t, 1, sink.len()-before)

		// and: the next window admits exactly one more
		clock.advance(2 * time.Hour)
		for i := 0; i < 500; i++ {
			emitter.Completed(importv2.KindPage, 1)
		}
		assert.Equal(t, 2, sink.len()-before)
		assert.Equal(t, int64(501), sink.last().PagesDone,
			"an event carries the state at the moment it was let through")
		assert.Equal(t, int64(1000), emitter.Snapshot().PagesDone,
			"coalescing delays the stream; it never makes the poll stale")
	})

	t.Run("a phase change is never held behind the window", func(t *testing.T) {
		// given
		clock := newFakeClock()
		emitter, sink := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseScanning)
		emitter.Discovered(importv2.KindPage, 1)
		before := sink.len()

		// when
		emitter.Phase(importv2.PhaseFetching)

		// then
		require.Equal(t, 1, sink.len()-before)
		assert.Equal(t, pb.EventImportStatistic_Fetching, sink.last().Phase)
	})

	t.Run("every state transition is immediate, and a repeat is not a transition", func(t *testing.T) {
		// given: rate limiting is NORMAL operation — the calm badge and its
		// removal are exactly what must not wait 250 ms
		clock := newFakeClock()
		emitter, sink := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseFetching)
		before := sink.len()

		// when
		emitter.Throttled(4 * time.Second)
		emitter.Throttled(4 * time.Second) // same state: coalesced
		emitter.Retrying(2, 5)
		emitter.Recovered()

		// then
		assert.Equal(t, 3, sink.len()-before)
		events := sink.all()[before:]
		assert.Equal(t, pb.EventImportStatistic_Throttled, events[0].State)
		assert.Equal(t, int64(4000), events[0].ResumesInMs)
		assert.Equal(t, pb.EventImportStatistic_Retrying, events[1].State)
		assert.Equal(t, int32(2), events[1].Attempt)
		assert.Equal(t, int32(5), events[1].AttemptsMax)
		assert.Equal(t, pb.EventImportStatistic_Running, events[2].State)
		assert.Zero(t, events[2].ResumesInMs, "the calm badge's countdown must clear with the state")
	})

	t.Run("the trailing edge delivers what the window swallowed", func(t *testing.T) {
		// given: a real clock and a tiny window — the coalesced value must
		// still arrive, or a stalled crawl's last number is lost forever
		sink := &captureSink{}
		emitter := newStatEmitter(statConfig{
			importId: "run-1", window: time.Millisecond, now: time.Now, send: sink.send,
		})
		defer emitter.Close(statVerdict{})
		emitter.Phase(importv2.PhaseCreating)

		// when
		emitter.Completed(importv2.KindPage, 1)
		emitter.Completed(importv2.KindPage, 1)

		// then
		require.Eventually(t, func() bool {
			all := sink.all()
			return len(all) > 0 && all[len(all)-1].PagesDone == 2
		}, 5*time.Second, time.Millisecond)
	})

	t.Run("close flushes the terminal numbers and silences the emitter", func(t *testing.T) {
		// given
		clock := newFakeClock()
		sink := &captureSink{}
		emitter := newStatEmitter(statConfig{
			importId: "run-1", window: time.Hour, now: clock.now, send: sink.send,
		})
		emitter.Phase(importv2.PhaseCreating)
		emitter.Completed(importv2.KindPage, 7)

		// when
		emitter.Close(statVerdict{})
		after := sink.len()
		emitter.Phase(importv2.PhaseFinalizing)
		emitter.Close(statVerdict{})

		// then
		assert.Equal(t, int64(7), sink.all()[after-1].PagesDone)
		assert.Equal(t, after, sink.len(), "a closed emitter must never speak again")
	})
}

func TestStatEmitterCounterEpochs(t *testing.T) {
	t.Run("fetching counts spool rows against claims; materializing re-bases on the census", func(t *testing.T) {
		// given
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)

		// when: pass 1 discovers claims, pass 2 spools some of them
		emitter.Phase(importv2.PhaseScanning)
		emitter.Discovered(importv2.KindPage, 10)
		scanning := emitter.Snapshot()
		emitter.Phase(importv2.PhaseFetching)
		emitter.Completed(importv2.KindPage, 4)
		emitter.Completed(importv2.KindFile, 2)
		emitter.Bytes(2048)
		fetching := emitter.Snapshot()

		// then: totals are indeterminate while scanning, known after it
		assert.False(t, scanning.TotalsKnown, "a cursor-chained scan has no total until it ends")
		assert.Equal(t, int64(10), scanning.PagesTotal, "SCANNING renders as a count-up")
		assert.True(t, fetching.TotalsKnown)
		assert.Equal(t, int64(10), fetching.PagesTotal, "the claim count IS the fetch denominator")
		assert.Equal(t, int64(4), fetching.PagesDone)
		assert.Equal(t, int64(2), fetching.FilesDone)
		assert.Zero(t, fetching.FilesTotal, "files are discovered by crawling; 0 means unknown")
		assert.Equal(t, int64(2048), fetching.BytesDone)

		// and: the analysis stage does not disturb the counters
		emitter.Phase(importv2.PhaseAnalyzing)
		emitter.Phase(importv2.PhaseFetching)
		assert.Equal(t, int64(10), emitter.Snapshot().PagesTotal)
		assert.Equal(t, int64(4), emitter.Snapshot().PagesDone)

		// when: pass 3 begins
		emitter.Phase(importv2.PhaseCreating)
		emitter.Discovered(importv2.KindPage, 12)
		emitter.Discovered(importv2.KindFile, 3)
		emitter.Completed(importv2.KindPage, 1)
		creating := emitter.Snapshot()

		// then: the denominators are the spool's, the numerators start over
		assert.Equal(t, int64(12), creating.PagesTotal)
		assert.Equal(t, int64(3), creating.FilesTotal)
		assert.Equal(t, int64(1), creating.PagesDone)
		assert.Zero(t, creating.FilesDone)
		assert.Equal(t, int64(2048), creating.BytesDone, "bytes on disk are a run total, not a phase one")
	})

	t.Run("a phase is not a denominator: totalsKnown is a fact about the totals", func(t *testing.T) {
		// given — review item 12: totalsKnown read `phase != SCANNING`, so
		// beginMaterialize's very first act — announcing CREATING, which
		// re-bases the counters to zero before the census that fills them —
		// published one event per run claiming a KNOWN total of zero. §15.3
		// exists to stop exactly that: clients render a count-up, never a fake
		// bar or a division by zero.
		//
		// Item 13 is the same reading held for a whole pass: a swallowed
		// census failure leaves the pass-3 denominators at zero, and the
		// numerator then climbs past a total that says it is known.
		clock := newFakeClock()
		emitter, sink := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseScanning)
		emitter.Discovered(importv2.KindPage, 40)
		emitter.Phase(importv2.PhaseFetching)
		require.True(t, emitter.Snapshot().TotalsKnown)

		// when: pass 3 opens — the census has not answered yet
		emitter.Phase(importv2.PhaseCreating)

		// then
		assert.False(t, sink.last().TotalsKnown,
			"the re-base emptied the denominator; nothing knows it yet")
		assert.Zero(t, sink.last().PagesTotal)

		// and: a census failure keeps it unknown however far the run gets
		emitter.Completed(importv2.KindPage, 12)
		assert.False(t, emitter.Snapshot().TotalsKnown)
		assert.Zero(t, emitter.Snapshot().EstimatedRemainingMs,
			"no denominator, no defensible estimate")

		// and: the census landing is what makes it known
		emitter.Discovered(importv2.KindPage, 50)
		assert.True(t, emitter.Snapshot().TotalsKnown)
	})

	t.Run("cancel effect turns at the pass boundary, not before", func(t *testing.T) {
		// given
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)

		// when / then: passes 1-2 have added nothing to the space
		for _, phase := range []importv2.Phase{importv2.PhaseScanning, importv2.PhaseAnalyzing, importv2.PhaseFetching} {
			emitter.Phase(phase)
			assert.Equal(t, pb.EventImportStatistic_NothingToUndo, emitter.Snapshot().CancelEffect)
		}
		for _, phase := range []importv2.Phase{importv2.PhaseCreating, importv2.PhaseFinalizing} {
			emitter.Phase(phase)
			assert.Equal(t, pb.EventImportStatistic_RemovesCreated, emitter.Snapshot().CancelEffect)
		}
	})

	t.Run("safeToClose answers per phase from the sweep's own predicates", func(t *testing.T) {
		// given: a run whose crawl is resumable but whose pass-3 budget is
		// spent — the two halves are separately true
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock, func(c *statConfig) {
			c.safeToCloseFetching = true
			c.safeToCloseMaterializing = false
		})

		// when / then
		emitter.Phase(importv2.PhaseFetching)
		assert.True(t, emitter.Snapshot().SafeToClose)
		emitter.Phase(importv2.PhaseCreating)
		assert.False(t, emitter.Snapshot().SafeToClose)
	})
}

func TestStatEmitterRateAndETA(t *testing.T) {
	t.Run("fetching cannot promise faster than the source's own ceiling", func(t *testing.T) {
		// given: an emitter told the crawl cannot exceed 1.5 pages/s
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock, func(c *statConfig) { c.pageRateCeiling = 1.5 })
		emitter.Phase(importv2.PhaseScanning)
		emitter.Discovered(importv2.KindPage, 100)
		emitter.Phase(importv2.PhaseFetching)

		// when: an unrepresentatively fast burst — 10 pages in 1 s
		clock.advance(time.Second)
		emitter.Completed(importv2.KindPage, 10)
		clock.advance(4 * time.Second)
		emitter.Completed(importv2.KindPage, 40)
		status := emitter.Snapshot()

		// then: 50 pages remain; the observed 10/s is capped at the ceiling
		assert.Equal(t, int64(50), status.PagesTotal-status.PagesDone)
		assert.InDelta(t, 50.0/1.5*1000, float64(status.EstimatedRemainingMs), 100)
		assert.InDelta(t, 10.0, status.ItemsPerSecond, 0.01)
	})

	t.Run("materializing uses the observed rate alone, and counts the files", func(t *testing.T) {
		// given: no ceiling is known a priori for persist speed, and both
		// totals come from the same spool census — so pending uploads are
		// known work, not a rounding error, on an import with an image per
		// page
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock, func(c *statConfig) { c.pageRateCeiling = 1.5 })
		emitter.Phase(importv2.PhaseCreating)
		emitter.Discovered(importv2.KindPage, 100)
		emitter.Discovered(importv2.KindFile, 20)

		// when: 20 objects in 4 s
		clock.advance(2 * time.Second)
		emitter.Completed(importv2.KindPage, 10)
		clock.advance(2 * time.Second)
		emitter.Completed(importv2.KindFile, 10)
		status := emitter.Snapshot()

		// then: 90 pages + 10 files left at 5 items/s. Pricing the uploads
		// at zero would answer 18 s for 20 s of work.
		assert.InDelta(t, 5.0, status.ItemsPerSecond, 0.01)
		assert.InDelta(t, 20000.0, float64(status.EstimatedRemainingMs), 300)
	})

	t.Run("the ETA is zero whenever its inputs are not there", func(t *testing.T) {
		// given
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)

		// when / then: no total yet
		emitter.Phase(importv2.PhaseScanning)
		emitter.Discovered(importv2.KindPage, 10)
		assert.Zero(t, emitter.Snapshot().EstimatedRemainingMs, "totals are indeterminate")

		// and: a total but no observed rate
		emitter.Phase(importv2.PhaseFetching)
		assert.Zero(t, emitter.Snapshot().EstimatedRemainingMs, "no rate has been observed yet")

		// and: a rate window too short to speak
		clock.advance(10 * time.Millisecond)
		emitter.Completed(importv2.KindPage, 1)
		assert.Zero(t, emitter.Snapshot().EstimatedRemainingMs, "one blink is not a rate")

		// and: nothing left to do
		clock.advance(5 * time.Second)
		emitter.Completed(importv2.KindPage, 9)
		assert.Zero(t, emitter.Snapshot().EstimatedRemainingMs)
	})

	t.Run("pass-3 rates do not sample themselves out of a rate", func(t *testing.T) {
		// given: 200 objects/s is the estimated persist speed. A sample per
		// object fills any bounded ring in a fraction of a second, and a
		// window shorter than rateWindowMin is not a rate — so the ETA would
		// read zero for the whole of the phase it is cheapest to compute in.
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseCreating)
		emitter.Discovered(importv2.KindPage, 4000)

		// when: 2,000 objects at 200/s, one report each
		for i := 0; i < 2000; i++ {
			clock.advance(5 * time.Millisecond)
			emitter.Completed(importv2.KindPage, 1)
		}
		status := emitter.Snapshot()

		// then
		assert.InDelta(t, 200.0, status.ItemsPerSecond, 5.0)
		assert.InDelta(t, 10000.0, float64(status.EstimatedRemainingMs), 500,
			"2,000 left at 200/s is ten seconds, and the engine can defend that")
	})

	t.Run("the rate window does not carry across the pass boundary", func(t *testing.T) {
		// given: pass 2 crawls at ~1/s, pass 3 runs orders of magnitude
		// faster — carrying the window over would produce the "one hour
		// left" lie for a pass that takes a minute
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseFetching)
		clock.advance(10 * time.Second)
		emitter.Completed(importv2.KindPage, 10)

		// when
		emitter.Phase(importv2.PhaseCreating)
		emitter.Discovered(importv2.KindPage, 100)

		// then
		assert.Zero(t, emitter.Snapshot().ItemsPerSecond)
		assert.Zero(t, emitter.Snapshot().EstimatedRemainingMs)
	})
}

func TestStatEmitterIssuesAndErrors(t *testing.T) {
	t.Run("live issue counts let a bad import be abandoned at minute 20", func(t *testing.T) {
		// given
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseFetching)

		// when
		emitter.Issue(importv2.Warning(importv2.IssueDataLoss, "k1", "lost"))
		emitter.Issue(importv2.Warning(importv2.IssueDataLoss, "k2", "lost"))
		emitter.Issue(importv2.ObjectError(importv2.IssueObjectFailed, "k3", assertError{}))
		emitter.Issue(importv2.Issue{Severity: importv2.SeverityInfo, Code: importv2.IssueTypeSuggested})
		status := emitter.Snapshot()

		// then
		assert.Equal(t, int64(2), status.WarningCount)
		assert.Equal(t, int64(1), status.ErrorCount, "info diagnostics are not problems")
		assert.Equal(t, pb.EventImportStatistic_Running, status.State,
			"a per-object failure is not the run being wrong")
	})

	t.Run("a resumed run inherits its predecessors' counts", func(t *testing.T) {
		// given: the engine re-seeds a previous incarnation's issues WITHOUT
		// re-reporting them (no OnIssue, no abort predicate), so without a
		// separate door the live surface of a resumed run would show fewer
		// problems than the same dir shows when polled dormant
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)

		// when
		emitter.Seed(statSeed{issues: []importv2.Issue{
			importv2.Warning(importv2.IssueDataLoss, "k1", "lost"),
			importv2.ObjectError(importv2.IssueObjectFailed, "k2", assertError{}),
		}})
		emitter.Issue(importv2.Warning(importv2.IssueDataLoss, "k3", "lost"))
		status := emitter.Snapshot()

		// then
		assert.Equal(t, int64(2), status.WarningCount)
		assert.Equal(t, int64(1), status.ErrorCount)
	})

	t.Run("a fatal turns the state to error; a cancel never does", func(t *testing.T) {
		// given
		clock := newFakeClock()
		emitter, sink := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseFetching)
		before := sink.len()

		// when
		emitter.Issue(importv2.Fatal(importv2.IssueCancelled, assertError{}))

		// then
		assert.Equal(t, pb.EventImportStatistic_Running, emitter.Snapshot().State,
			"the user stopping the import is not the import being broken")
		assert.Equal(t, before, sink.len(), "a non-transition emits nothing extra")

		// when
		emitter.Issue(importv2.Fatal(importv2.IssueStoreError, assertError{}))

		// then
		status := emitter.Snapshot()
		assert.Equal(t, pb.EventImportStatistic_Error, status.State)
		assert.NotEmpty(t, status.ErrorMessage)
		assert.Greater(t, sink.len(), before, "the alarm edge is immediate")

		// and: recovery from the transport does not clear a real error
		emitter.Recovered()
		assert.Equal(t, pb.EventImportStatistic_Error, emitter.Snapshot().State)
	})
}

type assertError struct{}

func (assertError) Error() string { return "boom" }

func TestStatEmitterCurrentItem(t *testing.T) {
	t.Run("the title reaches the wire and nothing that renders the emitter", func(t *testing.T) {
		// given
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseFetching)

		// when
		emitter.Item("Q3 Planning")

		// then
		assert.Equal(t, "Q3 Planning", emitter.Snapshot().CurrentItem)

		// and: no fmt rendering of the emitter's own state can leak it —
		// the §15.2 rule, enforced by DisplayText's String
		assert.NotContains(t, emitter.stateForLog(), "Q3 Planning")
	})

	t.Run("the item clears when the phase does", func(t *testing.T) {
		// given
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseFetching)
		emitter.Item("Q3 Planning")

		// when
		emitter.Phase(importv2.PhaseCreating)

		// then: a stale "Fetching: Q3 Planning" subtitle under CREATING is
		// worse than none
		assert.Empty(t, emitter.Snapshot().CurrentItem)
	})
}

func TestStatEmitterPushEqualsPull(t *testing.T) {
	t.Run("the pushed event and the polled snapshot are the same message", func(t *testing.T) {
		// given: §15.5's rule — a field meaning one thing pushed and another
		// polled is a bug, so both sides are ONE builder over ONE state
		clock := newFakeClock()
		emitter, sink := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseCreating)
		emitter.Discovered(importv2.KindPage, 9)
		emitter.Completed(importv2.KindPage, 3)
		emitter.Created(5)
		emitter.Issue(importv2.Warning(importv2.IssueDataLoss, "k", "lost"))

		// when: the emitter is closed, which flushes the terminal event
		emitter.Close(statVerdict{})

		// then
		pushed := sink.last()
		polled := emitter.Snapshot()
		assert.Equal(t, pushed.PagesTotal, polled.PagesTotal)
		assert.Equal(t, pushed.PagesDone, polled.PagesDone)
		assert.Equal(t, pushed.ObjectsCreated, polled.ObjectsCreated)
		assert.Equal(t, pushed.WarningCount, polled.WarningCount)
		assert.Equal(t, pushed.Phase, polled.Phase)
		assert.Equal(t, "run-1", pushed.ImportId)
		assert.Equal(t, "proc-1", pushed.ProcessId)
		assert.Equal(t, model.Import_Notion, pushed.ImportType)
		assert.Equal(t, clock.now().UnixMilli(), pushed.PhaseStartedAt)
	})
}

func TestStatEmitterConcurrentUse(t *testing.T) {
	t.Run("every seam method is safe from the goroutines that really call it", func(t *testing.T) {
		// given: persist workers, the converter goroutine, prefetch workers
		// and the client's retry loop all report at once
		emitter := newStatEmitter(statConfig{
			importId: "run-1", window: time.Millisecond, now: time.Now,
			send: func(*pb.EventImportStatistic) {},
		})
		defer emitter.Close(statVerdict{})

		// when
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				for n := 0; n < 200; n++ {
					emitter.Completed(importv2.KindPage, 1)
					emitter.Bytes(16)
					emitter.Created(int64(n))
					emitter.Item(importv2.DisplayText("page"))
					emitter.Throttled(time.Second)
					emitter.Recovered()
					emitter.Issue(importv2.Warning(importv2.IssueDataLoss, "k", "lost"))
					if n%50 == 0 {
						emitter.Phase(importv2.PhaseFetching)
					}
					emitter.Snapshot()
				}
			}(i)
		}
		wg.Wait()

		// then
		assert.Equal(t, int64(8*200), emitter.Snapshot().WarningCount)
	})
}

func TestStatEmitterLogSafety(t *testing.T) {
	t.Run("the loggable rendering carries codes and counts, never content", func(t *testing.T) {
		// given
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseFetching)
		emitter.Item("Salary review — confidential")
		emitter.Discovered(importv2.KindPage, 3)

		// when
		rendered := emitter.stateForLog()

		// then
		assert.NotContains(t, rendered, "Salary")
		assert.NotContains(t, rendered, "confidential")
		assert.True(t, strings.Contains(rendered, "fetching"), rendered)
	})
}

func TestStatEmitterCreatedLevel(t *testing.T) {
	t.Run("the created level never walks backwards", func(t *testing.T) {
		// given — review item 7: the engine publishes objectsCreated as a
		// LEVEL from workerCount goroutines (Created(r.created.Add(1))), and
		// nothing orders the increment against the publish. Two workers
		// interleave and the LOWER level arrives last. This is §15.4's cancel
		// affordance — "stop and remove the N objects created" — and the
		// dormant poll of the same run serves the exact ledger count, so a
		// number that walks backwards also breaks §15.5.
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseCreating)

		// when
		emitter.Created(6)
		emitter.Created(5)

		// then
		assert.Equal(t, int64(6), emitter.Snapshot().ObjectsCreated)
	})

	t.Run("racing workers still leave the run's own count standing", func(t *testing.T) {
		// given: the real shape — one shared counter, eight publishers
		emitter := newStatEmitter(statConfig{
			importId: "run-1", window: time.Hour, now: time.Now,
			send: func(*pb.EventImportStatistic) {},
		})
		defer emitter.Close(statVerdict{})
		var counter atomic.Int64
		var wg sync.WaitGroup

		// when
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for n := 0; n < 500; n++ {
					emitter.Created(counter.Add(1))
				}
			}()
		}
		wg.Wait()

		// then
		assert.Equal(t, int64(4000), emitter.Snapshot().ObjectsCreated)
	})
}

func TestStatEmitterErrorIsTerminal(t *testing.T) {
	t.Run("no transport state may repaint a failed run calm", func(t *testing.T) {
		// given — review item 5: the guard lived on Recovered alone, while
		// Throttled and Retrying set the state unconditionally. Both fire from
		// prefetch workers that outlive the run's cancel and neither checks a
		// ctx, so a run whose TERMINAL event should read ERROR could sign off
		// with "waiting for Notion".
		clock := newFakeClock()
		emitter, sink := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseFetching)
		emitter.Issue(importv2.Fatal(importv2.IssueStoreError, assertError{}))
		require.Equal(t, pb.EventImportStatistic_Error, emitter.Snapshot().State)

		// when: the doomed run's in-flight requests keep reporting
		for _, report := range []func(){
			func() { emitter.Throttled(4 * time.Second) },
			func() { emitter.Retrying(2, 5) },
			func() { emitter.Recovered() },
		} {
			report()

			// then
			assert.Equal(t, pb.EventImportStatistic_Error, emitter.Snapshot().State,
				"only a run can leave the error state, and it does so by ending")
		}
		assert.Zero(t, emitter.Snapshot().ResumesInMs, "a failed run has no reopening window")
		assert.NotEmpty(t, emitter.Snapshot().ErrorMessage, "the fatal's message survives")

		// and: the terminal event says so too
		emitter.Close(statVerdict{})
		assert.Equal(t, pb.EventImportStatistic_Error, sink.last().State)
	})
}

func TestStatEmitterThrottleCountdown(t *testing.T) {
	t.Run("the reopening window counts DOWN as it is polled", func(t *testing.T) {
		// given — review item 16: resumesIn was a DURATION frozen at signal
		// time, so a poller watching the calm badge saw "4s" for as long as
		// it cared to ask. Every other time-valued field on this event is an
		// INSTANT for exactly this reason — §15.2's phaseStartedAt is unix ms
		// so "clients show elapsed without their own clock" — and this one
		// was the exception.
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseFetching)

		// when
		emitter.Throttled(4 * time.Second)

		// then
		assert.Equal(t, int64(4000), emitter.Snapshot().ResumesInMs)
		clock.advance(3 * time.Second)
		assert.Equal(t, int64(1000), emitter.Snapshot().ResumesInMs)

		// and: a window that has already reopened is not negative time
		clock.advance(5 * time.Second)
		assert.Zero(t, emitter.Snapshot().ResumesInMs)
	})

	t.Run("recovery clears the countdown outright", func(t *testing.T) {
		// given
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseFetching)
		emitter.Throttled(time.Minute)

		// when
		emitter.Recovered()

		// then
		assert.Zero(t, emitter.Snapshot().ResumesInMs)
	})
}

func TestStatEmitterTerminalVerdict(t *testing.T) {
	t.Run("an all-or-nothing abort's terminal event says ERROR", func(t *testing.T) {
		// given — review item 11, the sibling of item 5 and its shared root:
		// the three hooks that CAN set the state all speak for the TRANSPORT,
		// and Close took no argument, so the terminal state was always a
		// transport artifact and never the run's verdict. An ALL_OR_NOTHING
		// abort — the commonest real failure — aborts on SeverityObjectError,
		// which the issue funnel does not escalate (it escalates at
		// SeverityFatal), and the engine returns that issue as Result.Err
		// without re-reporting it. ERROR was therefore unreachable for it.
		clock := newFakeClock()
		emitter, sink := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseCreating)
		emitter.Issue(importv2.ObjectError(importv2.IssueObjectFailed, "page-7", assertError{}))
		require.Equal(t, pb.EventImportStatistic_Running, emitter.Snapshot().State,
			"an object error is not, on its own, the run's verdict")

		// when: the run settles with that issue as its fatal
		emitter.Close(verdictOf(&importv2.Result{
			Err: importv2.ObjectError(importv2.IssueObjectFailed, "page-7", assertError{}),
		}))

		// then
		assert.Equal(t, pb.EventImportStatistic_Error, sink.last().State)
		assert.Contains(t, sink.last().ErrorMessage, "boom")
	})

	t.Run("a deliberate stop is not an error, however its fatal is shaped", func(t *testing.T) {
		// given: §15's ERROR means something is actually WRONG. A user cancel
		// and a shutdown suspend are neither, and painting the UI red on the
		// way out of a deliberate stop is the same category error as painting
		// it red for a rate limit. The STOP SOURCE decides (review item 1's
		// rule, applied to the surface): Result.Cancelled / Result.Suspended,
		// never the fatal's code.
		for _, tc := range []struct {
			name   string
			result *importv2.Result
		}{
			{"success", &importv2.Result{}},
			{"user cancel", &importv2.Result{
				Err:       importv2.Fatal(importv2.IssueCancelled, context.Canceled),
				Cancelled: true,
			}},
			{"shutdown suspend", &importv2.Result{
				Err:       importv2.Fatal(importv2.IssueCancelled, importv2.ErrSuspended),
				Suspended: true,
			}},
		} {
			// when
			clock := newFakeClock()
			emitter, sink := newTestEmitter(t, clock)
			emitter.Phase(importv2.PhaseCreating)
			emitter.Close(verdictOf(tc.result))

			// then
			assert.Equal(t, pb.EventImportStatistic_Running, sink.last().State, tc.name)
			assert.Empty(t, sink.last().ErrorMessage, tc.name)
		}
	})

	t.Run("a transport timeout wearing the cancel's CODE is still an error", func(t *testing.T) {
		// given — the destructive direction of review item 1, read on this
		// surface: the Notion client's own http.Client{Timeout: time.Minute}
		// makes classifyFatal issue IssueCancelled for a server hang. Nobody
		// cancelled, so the run failed and must say so.
		clock := newFakeClock()
		emitter, sink := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseFetching)

		// when
		emitter.Close(verdictOf(&importv2.Result{
			Err: importv2.Fatal(importv2.IssueCancelled, context.DeadlineExceeded),
			// Cancelled false: the caller's context is alive
		}))

		// then
		assert.Equal(t, pb.EventImportStatistic_Error, sink.last().State)
	})
}

func TestStatEmitterStallDetection(t *testing.T) {
	t.Run("a stalled run stops claiming the rate it no longer has", func(t *testing.T) {
		// given — review item 6: the rolling window was pruned only inside
		// sampleLocked, which runs only from Completed, and ratesLocked never
		// consulted the clock. A run that stops completing anything therefore
		// reported its last healthy rate and a frozen ETA forever — exactly
		// the throttled-vs-stuck distinction §15.1 exists to draw.
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseCreating)
		emitter.Discovered(importv2.KindPage, 100)
		clock.advance(time.Second)
		emitter.Completed(importv2.KindPage, 10)
		clock.advance(time.Second)
		emitter.Completed(importv2.KindPage, 10)
		healthy := emitter.Snapshot()
		require.InDelta(t, 10.0, healthy.ItemsPerSecond, 0.01)
		require.Positive(t, healthy.EstimatedRemainingMs)

		// when: nothing completes for ten seconds
		clock.advance(10 * time.Second)
		stalling := emitter.Snapshot()

		// then: the observed rate decays and the wait grows
		assert.Less(t, stalling.ItemsPerSecond, healthy.ItemsPerSecond)
		assert.Greater(t, stalling.EstimatedRemainingMs, healthy.EstimatedRemainingMs)

		// and: past the whole window there is no defensible rate left
		clock.advance(rateWindowSpan)
		stalled := emitter.Snapshot()
		assert.Zero(t, stalled.ItemsPerSecond, "a run that has done nothing for 30 s has no rate")
		assert.Zero(t, stalled.EstimatedRemainingMs, "and no ETA it could defend")
	})
}

func TestStatEmitterDenominatorInvariant(t *testing.T) {
	t.Run("a resumed crawl never reports more done than total", func(t *testing.T) {
		// given — review item 9: a crawl resume fills its two counters from
		// DIFFERENT sets. Pass 1 discovers only what /search re-enumerated,
		// while pass 2's seed counts the whole spool — including rows for
		// entities a previous incarnation found through a parent's block tree,
		// which /search never returns. The dormant surface of the same dir
		// reads 2/2; the live one read 2/1.
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseScanning)
		emitter.Discovered(importv2.KindPage, 1) // /search re-enumerates one page

		// when: the spool already holds two recorded rows
		emitter.Phase(importv2.PhaseFetching)
		emitter.Completed(importv2.KindPage, 2)
		seeded := emitter.Snapshot()

		// then
		assert.Equal(t, int64(2), seeded.PagesDone)
		assert.GreaterOrEqual(t, seeded.PagesTotal, seeded.PagesDone,
			"a denominator that exists may never be smaller than its own numerator")

		// and: the crawl's further discoveries still move the denominator
		emitter.Discovered(importv2.KindPage, 3)
		assert.Equal(t, int64(5), emitter.Snapshot().PagesTotal)
	})

	t.Run("an unknown denominator stays unknown", func(t *testing.T) {
		// given: filesTotal is 0 = UNKNOWN during the crawl (files are found by
		// crawling), and the schema's convention for unknown is zero. Turning
		// it into "exactly what is done" would render a finished file bar over
		// a crawl that has not found its files yet.
		clock := newFakeClock()
		emitter, _ := newTestEmitter(t, clock)
		emitter.Phase(importv2.PhaseFetching)

		// when
		emitter.Completed(importv2.KindFile, 2)

		// then
		assert.Zero(t, emitter.Snapshot().FilesTotal)
		assert.Equal(t, int64(2), emitter.Snapshot().FilesDone)
	})
}
