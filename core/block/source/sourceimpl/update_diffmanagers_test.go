package sourceimpl

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStorage is a minimal objecttree.Storage that serves changes from an
// in-memory map. Only Get and GetAfterOrder are exercised by resolvePair /
// eachChange; every other method is unused in these tests (embedding the
// interface gives nil-panic stubs that must never be reached).
type fakeStorage struct {
	objecttree.Storage
	changes map[string]objecttree.StorageChange
}

func (s *fakeStorage) Get(_ context.Context, id string) (objecttree.StorageChange, error) {
	ch, ok := s.changes[id]
	if !ok {
		return objecttree.StorageChange{}, objecttree.ErrNoChangeInTree
	}
	return ch, nil
}

func (s *fakeStorage) GetAfterOrder(_ context.Context, _ string, iter objecttree.StorageIterator) error {
	for _, ch := range s.changes {
		cont, err := iter(context.Background(), ch)
		if err != nil {
			return err
		}
		if !cont {
			return nil
		}
	}
	return nil
}

// fakeTree is a minimal objecttree.ObjectTree exposing only Storage(); the rest
// of the interface is nil-stubbed and must not be called by the code under test.
type fakeTree struct {
	objecttree.ObjectTree
	storage *fakeStorage
}

func (t *fakeTree) Storage() objecttree.Storage { return t.storage }

func newTestStore(storage *fakeStorage) *store {
	return &store{
		treeSource:   &treeSource{ObjectTree: &fakeTree{storage: storage}},
		diffManagers: map[string]*diffManager{},
	}
}

// TestUpdateInDiffManagers_ResolvesDeferredPendingSeenHead reproduces the
// cross-device regression: a peer's seenHead arrives (and is deferred to
// w.pending) BEFORE its change is synced locally; nothing is marked read. Once
// the change finally lands in tree storage, the data-sync path runs
// update()->updateInDiffManagers, which must re-advance and resolve the pending
// head so the messages it dominates become read.
//
// Fails before the fix (updateInDiffManagers was a no-op): the deferred head is
// never re-resolved, so onRemove never fires and the dominated messages stay
// unread until an unrelated advance (reload / local read / next peer update).
func TestUpdateInDiffManagers_ResolvesDeferredPendingSeenHead(t *testing.T) {
	// given: G and m1 are already synced messages; the peer seen head "H" is
	// NOT yet in local storage.
	storage := &fakeStorage{changes: map[string]objecttree.StorageChange{
		"G":  {Id: "G", OrderId: "o1", AddSeq: 1},
		"m1": {Id: "m1", OrderId: "o2", AddSeq: 2},
	}}
	s := newTestStore(storage)

	var removed []string
	onRemove := func(ids []string) { removed = append(removed, ids...) }
	wm := newWatermark(onRemove)
	s.diffManagers["messages"] = &diffManager{wm: wm, onRemove: onRemove}

	// peer says it has read up to H(o3,a3); H is not local yet → deferred,
	// nothing dominated/marked (G,m1 have orderId < H but H is unresolved).
	wm.advance([]string{"H"}, s.resolvePair, s.eachChangeFor(context.Background(), s.diffManagers["messages"]))
	require.Contains(t, wm.pending, "H", "unresolved peer seen head must be deferred")
	require.Empty(t, removed, "nothing can be marked read while the seen head is unresolved")

	// when: change H finally syncs into local tree storage and the data path
	// runs update()->updateInDiffManagers.
	storage.changes["H"] = objecttree.StorageChange{Id: "H", OrderId: "o3", AddSeq: 3}
	s.updateInDiffManagers(context.Background())

	// then: the deferred head is resolved and the messages it dominates are read.
	assert.NotContains(t, wm.pending, "H", "pending seen head must be re-resolved once its change lands")
	assert.ElementsMatch(t, []string{"G", "m1", "H"}, removed,
		"updateInDiffManagers must mark the now-dominated messages read")
}

// TestUpdateInDiffManagers_NoPendingNoOp confirms the re-advance is a safe no-op
// when there is nothing deferred: a fresh synced change keeps its own (newest)
// AddSeq and is not dominated, and no spurious onRemove is fired.
func TestUpdateInDiffManagers_NoPendingNoOp(t *testing.T) {
	storage := &fakeStorage{changes: map[string]objecttree.StorageChange{
		"G": {Id: "G", OrderId: "o1", AddSeq: 1},
	}}
	s := newTestStore(storage)

	var removed []string
	wm := newWatermark(func(ids []string) { removed = append(removed, ids...) })
	s.diffManagers["messages"] = &diffManager{wm: wm, onRemove: func(ids []string) {}}

	// no seen heads at all → empty frontier, advance returns early.
	s.updateInDiffManagers(context.Background())
	assert.Empty(t, removed, "no seen frontier ⇒ nothing marked read")
	assert.Empty(t, wm.pending)
}
