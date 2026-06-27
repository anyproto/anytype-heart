package sourceimpl

import (
	"context"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ground-truth tests for the cost of opening a big chat (a CRDT "store" object),
// measured as the number of times the full change log is read from storage.
//
// A chat has NO snapshots, so building its in-memory tree reads the entire change
// log from SQLite. The chat registers THREE diff managers (messages, mentions,
// reactions; see chatobject.go). Historically each diff manager built its OWN
// history tree from storage (store.InitDiffManager -> objecttree.NewDiffManager ->
// BuildHistoryTree), so an open read the whole log 1 (sync build) + 3 (diff
// managers) = 4 times.
//
// The fix (store.buildReusedDiffManager) makes the diff managers reuse the
// already-in-memory sync tree instead of re-reading storage: NewChangeDiffer only
// iterates the tree (IterateRoot) and copies each change's Id+PreviousIds into its
// own graph, so no storage scan is needed. That drops 4 scans -> 1.
//
// TestChatOpen_DiffManagersReuseSyncTree proves the production seam does 0 storage
// scans (and contrasts it with the old per-manager-build cost on the same tree).

// chatDiffManagerCount is the number of diff managers a chat registers
// (messages, mentions, reactions). See chatobject.go.
const chatDiffManagerCount = 3

// openScanCounters counts reads against the tree "changes" collection.
type openScanCounters struct {
	findCalls atomic.Int64 // number of range queries (== number of full-log scans)
	rowsRead  atomic.Int64 // number of change rows iterated across all scans
}

func (c *openScanCounters) reset() {
	c.findCalls.Store(0)
	c.rowsRead.Store(0)
}

// --- counting anystore wrappers (embed the interface, override only what we need) ---

type countingDB struct {
	anystore.DB
	c *openScanCounters
}

func (d countingDB) Collection(ctx context.Context, name string) (anystore.Collection, error) {
	coll, err := d.DB.Collection(ctx, name)
	return d.wrap(coll, name), err
}

func (d countingDB) OpenCollection(ctx context.Context, name string) (anystore.Collection, error) {
	coll, err := d.DB.OpenCollection(ctx, name)
	return d.wrap(coll, name), err
}

func (d countingDB) CreateCollection(ctx context.Context, name string) (anystore.Collection, error) {
	coll, err := d.DB.CreateCollection(ctx, name)
	return d.wrap(coll, name), err
}

func (d countingDB) wrap(coll anystore.Collection, name string) anystore.Collection {
	if coll == nil || name != objecttree.CollName {
		return coll
	}
	return countingColl{Collection: coll, c: d.c}
}

type countingColl struct {
	anystore.Collection
	c *openScanCounters
}

func (cc countingColl) Find(filter any) anystore.Query {
	cc.c.findCalls.Add(1)
	return countingQuery{Query: cc.Collection.Find(filter), c: cc.c}
}

type countingQuery struct {
	anystore.Query
	c *openScanCounters
}

func (q countingQuery) Sort(s ...any) anystore.Query {
	return countingQuery{Query: q.Query.Sort(s...), c: q.c}
}
func (q countingQuery) Limit(l uint) anystore.Query {
	return countingQuery{Query: q.Query.Limit(l), c: q.c}
}
func (q countingQuery) Offset(o uint) anystore.Query {
	return countingQuery{Query: q.Query.Offset(o), c: q.c}
}
func (q countingQuery) IndexHint(h ...anystore.IndexHint) anystore.Query {
	return countingQuery{Query: q.Query.IndexHint(h...), c: q.c}
}
func (q countingQuery) Iter(ctx context.Context) (anystore.Iterator, error) {
	it, err := q.Query.Iter(ctx)
	if err != nil {
		return it, err
	}
	return countingIter{Iterator: it, c: q.c}, nil
}

type countingIter struct {
	anystore.Iterator
	c *openScanCounters
}

func (i countingIter) Next() bool {
	ok := i.Iterator.Next()
	if ok {
		i.c.rowsRead.Add(1)
	}
	return ok
}

// setupCountingChatTree builds a real anystore-backed object tree with numChanges
// changes in a linear chain (mirroring a chat with numChanges messages), returns the
// storage, acl, heads and a counter wired into the "changes" collection.
func setupCountingChatTree(t *testing.T, numChanges int) (objecttree.Storage, list.AclList, []string, *openScanCounters) {
	counters := &openScanCounters{}
	aclList, _ := prepareAclList(t)

	creator := objecttree.NewMockChangeCreator(func() anystore.DB {
		db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "changes.db"), nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = db.Close() })
		return countingDB{DB: db, c: counters}
	})

	storage := creator.CreateNewTreeStorage(t, "0", aclList.Head().Id, false)
	tree, err := objecttree.BuildTestableTree(storage, aclList)
	require.NoError(t, err)
	tree.SetFlusher(objecttree.MarkNewChangeFlusher())

	raws := make([]*treechangeproto.RawTreeChangeWithId, 0, numChanges)
	prev := "0"
	for i := 1; i <= numChanges; i++ {
		id := strconv.Itoa(i)
		raws = append(raws, creator.CreateRaw(id, aclList.Head().Id, "0", false, prev))
		prev = id
	}
	_, err = tree.AddRawChanges(ctx, objecttree.RawChangesPayload{
		NewHeads:   []string{prev},
		RawChanges: raws,
	})
	require.NoError(t, err)

	return storage, aclList, []string{prev}, counters
}

// TestChatOpen_DiffManagersReuseSyncTree is the before/after ground truth for the fix.
// It builds the sync tree once, then measures the storage scans of (a) the OLD path
// (one history-tree build per diff manager) and (b) the NEW path (production
// store.buildReusedDiffManager, which reuses the in-memory sync tree). The NEW path
// must do ZERO storage scans, dropping a chat open from 4 full-log scans to 1.
func TestChatOpen_DiffManagersReuseSyncTree(t *testing.T) {
	const numChanges = 300
	storage, aclList, _, counters := setupCountingChatTree(t, numChanges)
	logLen := int64(numChanges + 1) // + root

	// The one mandatory build: the in-memory sync tree (this is the "tree build").
	counters.reset()
	syncTree, err := objecttree.BuildTestableTree(storage, aclList)
	require.NoError(t, err)
	syncScans := counters.findCalls.Load()

	// OLD path: each diff manager re-read the whole log from storage.
	counters.reset()
	for i := 0; i < chatDiffManagerCount; i++ {
		_, err := objecttree.BuildEmptyDataTestableTree(storage, aclList)
		require.NoError(t, err)
	}
	oldDiffScans, oldDiffRows := counters.findCalls.Load(), counters.rowsRead.Load()

	// NEW path: the production seam reuses the already-in-memory sync tree.
	s := &store{treeSource: &treeSource{ObjectTree: syncTree}}
	counters.reset()
	for i := 0; i < chatDiffManagerCount; i++ {
		dm, err := s.buildReusedDiffManager(nil, func([]string) {})
		require.NoError(t, err)
		require.NotNil(t, dm)
	}
	newDiffScans, newDiffRows := counters.findCalls.Load(), counters.rowsRead.Load()

	t.Logf("change log length: %d", logLen)
	t.Logf("sync build:        scans=%d", syncScans)
	t.Logf("OLD diff init:     scans=%d rows=%d  -> total open = %d scans (1 sync + %d diff)", oldDiffScans, oldDiffRows, syncScans+oldDiffScans, chatDiffManagerCount)
	t.Logf("NEW diff init:     scans=%d rows=%d  -> total open = %d scans (1 sync + 0 diff)", newDiffScans, newDiffRows, syncScans+newDiffScans)

	// Sanity: the sync build reads the log exactly once.
	assert.Equal(t, int64(1), syncScans, "sync tree build should scan the log once")
	// The cost we are removing: 3 redundant full-log scans.
	assert.Equal(t, int64(chatDiffManagerCount), oldDiffScans, "old path scanned the log once per diff manager")
	// The fix: diff managers reuse the in-memory sync tree -> no storage scan.
	assert.Equal(t, int64(0), newDiffScans, "reusing the in-memory sync tree must do NO storage scan")
	assert.Equal(t, int64(0), newDiffRows, "no rows read from storage when reusing the in-memory tree")
	// Net: a chat open goes from 4 full-log scans to 1.
	assert.Equal(t, int64(1), syncScans+newDiffScans, "a chat open should scan the full log once after the fix")
}

// TestChatOpen_ScalingCost reports wall-clock + scans as the chat grows, confirming
// the open scans the log exactly once at every size.
// Run: go test ./core/block/source/sourceimpl/ -run TestChatOpen_ScalingCost -v
func TestChatOpen_ScalingCost(t *testing.T) {
	if testing.Short() {
		t.Skip("scaling cost test skipped in -short mode")
	}
	for _, numChanges := range []int{200, 1000, 5000} {
		t.Run(strconv.Itoa(numChanges)+"_changes", func(t *testing.T) {
			storage, aclList, _, counters := setupCountingChatTree(t, numChanges)
			logLen := int64(numChanges + 1)

			counters.reset() // ignore the storage writes/reads done during setup
			start := time.Now()
			syncTree, err := objecttree.BuildTestableTree(storage, aclList)
			require.NoError(t, err)
			s := &store{treeSource: &treeSource{ObjectTree: syncTree}}
			for i := 0; i < chatDiffManagerCount; i++ {
				_, err := s.buildReusedDiffManager(nil, func([]string) {})
				require.NoError(t, err)
			}
			elapsed := time.Since(start)

			totalScans := counters.findCalls.Load()
			totalRows := counters.rowsRead.Load()
			t.Logf("n=%-5d open=%-12v rows_read=%d (%.1fx log) scans=%d (%.1f us/change)",
				numChanges, elapsed, totalRows, float64(totalRows)/float64(logLen), totalScans,
				float64(elapsed.Microseconds())/float64(logLen))

			assert.Equal(t, int64(1), totalScans, "open should scan the full log exactly once at any size")
		})
	}
}
