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
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// The crawl-resume equivalence gate, the DM-2 shape extended to pass 2:
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

// spoolCensus reads a closed run dir's spool keys (sorted) and its ROW
// count. The count is the anti-duplication observable (review P2): the key
// set is a map and dedupes by construction, so "no key appears twice"
// asserted over the map alone was structurally vacuous — a duplicated row
// shows up only as count > len(keys).
func spoolCensus(t *testing.T, dir string) ([]string, int) {
	t.Helper()
	store, err := runstore.Open(context.Background(), dir)
	require.NoError(t, err)
	defer store.Close()
	spool, err := store.Spool(context.Background())
	require.NoError(t, err)
	keySet, count, err := spool.SourceKeys(context.Background())
	require.NoError(t, err)
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, count
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
		// given: a control run and a run killed at its FOURTH spool append —
		// mid-pass-2, nothing in the space, the class DM-2 discarded. Four,
		// not three (review P2): the first three rows are the file and the
		// plan's relation/type — the kill must land AFTER a content page is
		// recorded, or "an already-recorded page is not re-fetched/re-spooled"
		// is never exercised. The Contains below makes that load-bearing
		// instead of an accident of emission order.
		root := crashTree(t)
		control, controlResult := runControl(t, root)
		fx := NewFixture(t)
		dir := filepath.Join(t.TempDir(), "run-crash")
		inc1 := interruptCrawl(t, fx, root, dir, 4)
		require.Error(t, inc1.Err)
		require.True(t, inc1.Suspended, "the stop must be the suspend shape, not an abort")
		require.Empty(t, fx.Space.Created, "the kill landed mid-crawl: NOTHING may be in the space")
		partial, partialCount := spoolCensus(t, dir)
		require.Less(t, len(partial), 6, "the kill must leave a partial spool or the test proves nothing")
		require.Contains(t, partial, "index.md",
			"a CONTENT PAGE must be recorded before the kill, or the skip path is never exercised")

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
		// the remainder appended exactly once — by ROW COUNT, the observable
		// a duplicate actually moves (the key set dedupes by construction)
		resumedKeys, resumedCount := spoolCensus(t, dir)
		for _, key := range partial {
			assert.Contains(t, resumedKeys, key, "recorded rows must survive the resume")
		}
		assert.Equal(t, len(resumedKeys), resumedCount,
			"spool rows must be unique per key — a duplicate row means the backstop failed")
		assert.GreaterOrEqual(t, resumedCount, partialCount)
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

	t.Run("a RECORDED file renamed between sessions imports twice — decided drift semantics, pinned", func(t *testing.T) {
		// given — review P2, decided and documented rather than left silent:
		// the crawl artifact is the run's ground truth under the drift rule, so
		// a rename across the crash boundary is a deletion of the recorded
		// path (the recording wins — it materializes) PLUS an addition (the
		// new path imports as a new object). The result is two objects with
		// the same content and no issue — accepted: detecting a rename would
		// need content identity across paths, which markdown source keys
		// (relative paths) do not carry, and the orphan lands on the next
		// updateExisting import as an ordinary duplicate. This test exists so
		// a future change of that trade-off is a conscious one.
		root := crashTree(t)
		fx := NewFixture(t)
		dir := filepath.Join(t.TempDir(), "run-crash")
		inc1 := interruptCrawl(t, fx, root, dir, 4)
		require.True(t, inc1.Suspended)
		recorded, _ := spoolCensus(t, dir)
		require.Contains(t, recorded, "index.md", "the rename victim must already be recorded")
		renamed := filepath.Join(root, "home.md")
		require.NoError(t, os.Rename(filepath.Join(root, "index.md"), renamed))
		require.NoError(t, os.Chtimes(renamed, crashFixtureMtime, crashFixtureMtime))

		// when
		resumed := fx.ResumeCrawlDurable(context.Background(), t, dir, root, request(false, false))

		// then: both the recording and the addition import, silently
		require.NoError(t, resumed.Err)
		assert.Empty(t, resumed.Issues,
			"pinned: the duplication is silent — flagging it would need rename detection the source keys cannot carry")
		var homes int
		fx.Space.mu.Lock()
		for _, st := range fx.Space.Created {
			if st != nil && st.CombinedDetails().GetString(bundle.RelationKeyName) == "Home" {
				homes++
			}
		}
		fx.Space.mu.Unlock()
		assert.Equal(t, 2, homes,
			"the recorded index.md AND the renamed home.md must both materialize (the documented semantics)")
	})

	t.Run("a page deleted between sessions warns and the rest converges", func(t *testing.T) {
		// given — source drift end to end: incarnation 1 claims every page and
		// records two; the source then loses a page that was NOT recorded.
		root := crashTree(t)
		fx := NewFixture(t)
		dir := filepath.Join(t.TempDir(), "run-crash")
		inc1 := interruptCrawl(t, fx, root, dir, 2)
		require.True(t, inc1.Suspended)
		recorded, _ := spoolCensus(t, dir)
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
