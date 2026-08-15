package adapter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
)

// The legacy process scalar is the compatibility path §15.1 promised to
// leave untouched, so the down-projection of the redesigned seam has to
// reproduce what the old one produced — on the resume paths too.

// recordingProgress captures the scalar the adapter drives.
type recordingProgress struct {
	process.Progress
	total   int64
	done    int64
	message string
	// totalWrites counts publishes: SetTotalPreservingRatio is not
	// idempotent once done is non-zero, so how OFTEN it is called is part
	// of the contract, not an implementation detail.
	totalWrites int
}

func (p *recordingProgress) SetTotalPreservingRatio(total int64) {
	p.total = total
	p.totalWrites++
}

func (p *recordingProgress) AddDone(delta int64)           { p.done += delta }
func (p *recordingProgress) SetProgressMessage(msg string) { p.message = msg }
func (p *recordingProgress) Id() string                    { return "proc-1" }

func TestProgressReporterProjection(t *testing.T) {
	t.Run("a fresh run publishes the claim count once, then re-bases on the census", func(t *testing.T) {
		// given
		progress := &recordingProgress{}
		reporter := &progressReporter{progress: progress}

		// when: pass 1 discovers 2,000 claims one at a time
		reporter.Phase(importv2.PhaseScanning)
		for i := 0; i < 2000; i++ {
			reporter.Discovered(importv2.KindPage, 1)
		}

		// then: nothing is published yet — SetTotalPreservingRatio's ratio
		// arithmetic must not run 2,000 times on a multi-path request
		assert.Zero(t, progress.totalWrites)

		// when
		reporter.Phase(importv2.PhaseFetching)

		// then
		assert.Equal(t, 1, progress.totalWrites)
		assert.Equal(t, int64(2000), progress.total)
		assert.Equal(t, "Fetching content", progress.message)

		// and: pass 2's spooling does not fill the one bar pass 3 will fill
		reporter.Completed(importv2.KindPage, 5)
		assert.Zero(t, progress.done)

		// when: pass 3 re-bases on the spool census
		reporter.Phase(importv2.PhaseCreating)
		reporter.Discovered(importv2.KindPage, 30)
		reporter.Discovered(importv2.KindFile, 4)
		reporter.Completed(importv2.KindPage, 30)
		reporter.Completed(importv2.KindFile, 4)

		// then
		assert.Equal(t, int64(34), progress.total)
		assert.Equal(t, int64(34), progress.done, "the bar can now actually reach its end")
	})

	t.Run("a pass-3 restart publishes its total too", func(t *testing.T) {
		// given: engine.Resume never runs pass 1, so the reporter sees
		// CREATING as its first phase. resumerun.go used to set the total by
		// hand for exactly this case; when that moved into the engine, the
		// projection had to stop gating publishes on a scanning stage that
		// never happens — otherwise a resumed import shows a bar that fills
		// against a total of zero.
		progress := &recordingProgress{}
		reporter := &progressReporter{progress: progress}

		// when
		reporter.Phase(importv2.PhaseCreating)
		reporter.Discovered(importv2.KindPage, 9)
		reporter.Completed(importv2.KindPage, 9)

		// then
		assert.Equal(t, int64(9), progress.total)
		assert.Equal(t, int64(9), progress.done)
	})
}

func TestTeeReporterFansOut(t *testing.T) {
	t.Run("every seam method reaches every consumer", func(t *testing.T) {
		// given
		progress := &recordingProgress{}
		emitter := newStatEmitter(statConfig{importId: "run-1", send: func(*pb.EventImportStatistic) {}})
		defer emitter.Close()
		reporter := teeReporter{&progressReporter{progress: progress}, emitter}

		// when
		reporter.Phase(importv2.PhaseCreating)
		reporter.Discovered(importv2.KindPage, 3)
		reporter.Completed(importv2.KindPage, 2)
		reporter.Bytes(64)
		reporter.Created(2)
		reporter.Item("Q3 Planning")

		// then
		assert.Equal(t, int64(3), progress.total)
		assert.Equal(t, int64(2), progress.done)
		status := emitter.Snapshot()
		assert.Equal(t, int64(3), status.PagesTotal)
		assert.Equal(t, int64(2), status.PagesDone)
		assert.Equal(t, int64(64), status.BytesDone)
		assert.Equal(t, int64(2), status.ObjectsCreated)
		assert.Equal(t, "Q3 Planning", status.CurrentItem)
	})
}
