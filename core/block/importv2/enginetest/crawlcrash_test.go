package enginetest

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/engine"
	"github.com/anyproto/anytype-heart/core/block/importv2/runstore"
)

// The DM-3 equivalence gate (spec §8.3, the DM-2 shape extended to pass 2):
// kill a run MID-CRAWL — before the fetch-complete marker, the class DM-2
// could only compensate — resume it from the dir plus the live source, and
// assert the final object set is IDENTICAL to an uninterrupted run, with
// the spool extended rather than duplicated. The Notion no-refetch half of
// the phase is pinned in the notion and adapter suites (scripted API,
// recorded requests); markdown proves the engine-side equivalence.

// interruptingSpool kills the run at its N-th append: the cancel fires with
// the suspend cause and the append reports the stop — exactly where a
// pass-2 suspend lands (appends are ctx-immune internally; cancellation
// only ever lands between objects).
type interruptingSpool struct {
	engine.Spool
	remaining atomic.Int32
	cancel    context.CancelCauseFunc
}

func (s *interruptingSpool) Append(ctx context.Context, o *importv2.Object) error {
	if s.remaining.Add(-1) < 0 {
		s.cancel(importv2.ErrSuspended)
		return context.Canceled
	}
	return s.Spool.Append(ctx, o)
}

// spoolCensus reads a closed run dir's spool keys, sorted.
func spoolCensus(t *testing.T, dir string) []string {
	t.Helper()
	store, err := runstore.Open(context.Background(), dir)
	require.NoError(t, err)
	defer store.Close()
	spool, err := store.Spool(context.Background())
	require.NoError(t, err)
	keySet, _, err := spool.SourceKeys(context.Background())
	require.NoError(t, err)
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// interruptCrawl runs one incarnation killed at the N-th spool append and
// disarms the wrap for the resume.
func interruptCrawl(t *testing.T, fx *Fixture, root, dir string, appends int32) *importv2.Result {
	t.Helper()
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	fx.WrapSpool = func(inner engine.Spool) engine.Spool {
		wrap := &interruptingSpool{Spool: inner, cancel: cancel}
		wrap.remaining.Store(appends)
		return wrap
	}
	result := fx.RunMarkdownDurable(ctx, t, root, request(false, false), dir)
	fx.WrapSpool = nil
	return result
}

func TestCrashResumeMidCrawl(t *testing.T) {
	t.Run("killed mid-crawl: the resumed crawl extends the recording and converges byte-identically", func(t *testing.T) {
		// given: a control run and a run killed at its third spool append —
		// mid-pass-2, nothing in the space, the class DM-2 discarded
		root := crashTree(t)
		control, controlResult := runControl(t, root)
		fx := NewFixture(t)
		dir := filepath.Join(t.TempDir(), "run-crash")
		inc1 := interruptCrawl(t, fx, root, dir, 3)
		require.Error(t, inc1.Err)
		require.True(t, inc1.Suspended, "the stop must be the suspend shape, not an abort")
		require.Empty(t, fx.Space.Created, "the kill landed mid-crawl: NOTHING may be in the space")
		partial := spoolCensus(t, dir)
		require.Less(t, len(partial), 5, "the kill must leave a partial spool or the test proves nothing")

		// when: resumed from the dir plus the live source
		resumed := fx.ResumeCrawlDurable(context.Background(), t, dir, root, request(false, false))

		// then: identical object set, counters counted once, no invented issues
		require.NoError(t, resumed.Err)
		assert.False(t, resumed.Suspended)
		assert.Empty(t, resumed.Issues, "a resumed crawl must not invent issues")
		assert.Equal(t, controlResult.Created, resumed.Created)
		assert.Equal(t, controlResult.Updated, resumed.Updated)
		assert.Equal(t, control.Dump(), fx.Dump(),
			"the final object set must be identical to the uninterrupted run")
		// and the recording extended without duplicates: recorded rows kept,
		// the remainder appended exactly once
		resumedKeys := spoolCensus(t, dir)
		for _, key := range partial {
			assert.Contains(t, resumedKeys, key, "recorded rows must survive the resume")
		}
		seen := map[string]bool{}
		for _, key := range resumedKeys {
			assert.False(t, seen[key], "spool key %q appears twice — the backstop failed", key)
			seen[key] = true
		}
		// absolute observable, not control-relative (review Class H rule):
		// the pinned fixture mtime must survive the crawl restart too
		wantTs := crashFixtureMtime.Unix()
		assert.Equal(t, map[string]int64{
			"Home": wantTs, "A": wantTs, "B": wantTs, "C": wantTs, "D": wantTs,
			"Author": 0, "Zettel": 0, "Markdown Import": 0,
		}, fx.OriginalTimestamps())
	})

	t.Run("killed mid-crawl TWICE: two partial recordings still converge to one clean import", func(t *testing.T) {
		// given
		root := crashTree(t)
		control, controlResult := runControl(t, root)
		fx := NewFixture(t)
		dir := filepath.Join(t.TempDir(), "run-crash")
		inc1 := interruptCrawl(t, fx, root, dir, 2)
		require.True(t, inc1.Suspended)

		// second incarnation: the crawl resumes and is killed again, further
		// along (2 recorded + 2 more — ResumeCrawlDurable's spool rides the
		// same WrapSpool injection point as a fresh run's)
		ctx2, cancel2 := context.WithCancelCause(context.Background())
		defer cancel2(nil)
		fx.WrapSpool = func(inner engine.Spool) engine.Spool {
			wrap := &interruptingSpool{Spool: inner, cancel: cancel2}
			wrap.remaining.Store(2)
			return wrap
		}
		inc2 := fx.ResumeCrawlDurable(ctx2, t, dir, root, request(false, false))
		fx.WrapSpool = nil
		require.Error(t, inc2.Err)
		require.True(t, inc2.Suspended)
		require.Empty(t, fx.Space.Created, "still mid-crawl: still nothing in the space")

		// when: the third incarnation runs to completion
		resumed := fx.ResumeCrawlDurable(context.Background(), t, dir, root, request(false, false))

		// then
		require.NoError(t, resumed.Err)
		assert.Empty(t, resumed.Issues)
		assert.Equal(t, controlResult.Created, resumed.Created)
		assert.Equal(t, control.Dump(), fx.Dump(),
			"two interrupted crawls must leave no trace in the final object set")
	})

	t.Run("a page deleted between sessions warns and the rest converges", func(t *testing.T) {
		// given — 08-13 §5.4 end to end: incarnation 1 claims every page and
		// records two; the source then loses a page that was NOT recorded.
		root := crashTree(t)
		fx := NewFixture(t)
		dir := filepath.Join(t.TempDir(), "run-crash")
		inc1 := interruptCrawl(t, fx, root, dir, 2)
		require.True(t, inc1.Suspended)
		recorded := spoolCensus(t, dir)
		victim := "notes/d.md" // late in emission order: never among the first two rows
		require.NotContains(t, recorded, victim, "the victim must not be recorded yet")
		require.NoError(t, os.Remove(filepath.Join(root, filepath.FromSlash(victim))))

		// when
		resumed := fx.ResumeCrawlDurable(context.Background(), t, dir, root, request(false, false))

		// then: loud data-loss warning for the vanished page, clean otherwise
		require.NoError(t, resumed.Err)
		assert.Zero(t, resumed.Failed, "source drift is not a converter bug")
		var dataLoss int
		for _, issue := range resumed.Issues {
			if issue.Code == importv2.IssueDataLoss && issue.SourceKey == victim {
				dataLoss++
			}
		}
		assert.Equal(t, 1, dataLoss, "the vanished page must be reported exactly once: %v", resumed.Issues)
	})
}
