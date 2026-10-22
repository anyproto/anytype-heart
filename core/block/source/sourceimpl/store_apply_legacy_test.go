package sourceimpl

import (
	"context"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
)

// recordingHook captures every change the replay walks over, which is exactly what the
// legacy-addSeq bug silently truncated.
type recordingHook struct {
	iterated []string
}

func (h *recordingHook) BeforeIteration(objecttree.ObjectTree) {}

func (h *recordingHook) OnIteration(_ objecttree.ObjectTree, change *objecttree.Change) {
	h.iterated = append(h.iterated, change.Id)
}

func (h *recordingHook) AfterDiffManagersInit(context.Context) error { return nil }

// stripAddSeq removes the addSeq key from every stored change, reproducing storage written
// by builds from before any-sync assigned an addSeq. Such changes read back as addSeq == 0.
func stripAddSeq(t testing.TB, treeDb anystore.DB) {
	coll, err := treeDb.OpenCollection(ctx, objecttree.CollName)
	require.NoError(t, err)

	iter, err := coll.Find(nil).Iter(ctx)
	require.NoError(t, err)
	var ids []string
	for iter.Next() {
		doc, err := iter.Doc()
		require.NoError(t, err)
		ids = append(ids, doc.Value().GetString("id"))
	}
	require.NoError(t, iter.Close())

	unsetAddSeq := query.ModifyFunc(func(_ *anyenc.Arena, v *anyenc.Value) (*anyenc.Value, bool, error) {
		if v.Get(objecttree.AddSeqKey) == nil {
			return v, false, nil
		}
		v.Del(objecttree.AddSeqKey)
		return v, true, nil
	})
	for _, id := range ids {
		_, err = coll.UpdateId(ctx, id, unsetAddSeq)
		require.NoError(t, err)
	}
}

// newLegacyTreeFx builds a real object tree whose changes are all stored without an addSeq,
// paired with an empty store state — i.e. the state after the object store is wiped and the
// pre-existing tree has to be replayed from scratch.
func newLegacyTreeFx(t *testing.T) (*storestate.StoreState, objecttree.ObjectTree) {
	var treeDb anystore.DB
	changeCreator := objecttree.NewMockChangeCreator(func() anystore.DB {
		treeDb = createStore(ctx, t)
		return treeDb
	})
	aclList, _ := prepareAclList(t)
	treeStorage := changeCreator.CreateNewTreeStorage(t, "0", aclList.Head().Id, false)

	tree, err := objecttree.BuildTestableTree(treeStorage, aclList)
	require.NoError(t, err)
	tree.SetFlusher(objecttree.MarkNewChangeFlusher())

	_, err = tree.AddRawChanges(ctx, objecttree.RawChangesPayload{
		NewHeads: []string{"3", "5"},
		RawChanges: []*treechangeproto.RawTreeChangeWithId{
			changeCreator.CreateRaw("1", aclList.Head().Id, "0", false, "0"),
			changeCreator.CreateRaw("2", aclList.Head().Id, "0", false, "1"),
			changeCreator.CreateRaw("3", aclList.Head().Id, "0", true, "2"),
			changeCreator.CreateRaw("4", aclList.Head().Id, "0", false, "0"),
			changeCreator.CreateRaw("5", aclList.Head().Id, "0", true, "4"),
		},
	})
	require.NoError(t, err)

	stripAddSeq(t, treeDb)

	// rebuild so the in-memory changes carry the stripped (zero) addSeq, like a real reopen
	legacyTree, err := objecttree.BuildTestableTree(treeStorage, aclList)
	require.NoError(t, err)

	stateDb, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "crdt.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stateDb.Close() })

	state, err := storestate.New(ctx, "source_test", stateDb, storestate.DefaultHandler{Name: "default"})
	require.NoError(t, err)

	return state, legacyTree
}

// Changes stored before any-sync assigned an addSeq carry addSeq == 0. IterateAfterAddSeq
// only yields changes with an addSeq strictly greater than the watermark, so replaying such
// a tree from watermark 0 used to skip its entire history: chat details and messages were
// silently dropped whenever the object store was rebuilt (a reindex, or a deleted
// objectstore folder).
func TestStoreApply_ReplaysChangesWithoutAddSeq(t *testing.T) {
	state, tree := newLegacyTreeFx(t)

	tx, err := state.NewTx(ctx)
	require.NoError(t, err)
	require.False(t, tx.IsFullyReplayed(), "a fresh store state has replayed nothing yet")

	hook := &recordingHook{}
	applier := &storeApply{tx: tx, ot: tree, hook: hook}
	require.NoError(t, applier.Apply(ctx))
	require.NoError(t, tx.Commit())

	assert.Subset(t, hook.iterated, []string{"1", "2", "3", "4", "5"},
		"every change must be replayed even though none of them has an addSeq")

	// the full replay is recorded, so later applies can use the incremental path
	tx, err = state.NewTx(ctx)
	require.NoError(t, err)
	defer tx.Rollback()
	assert.True(t, tx.IsFullyReplayed())
}

// Once the tree has been fully replayed, applies must go back to reading only what was added
// since the watermark instead of rescanning the whole history on every open.
func TestStoreApply_IncrementalAfterFullReplay(t *testing.T) {
	state, tree := newLegacyTreeFx(t)

	tx, err := state.NewTx(ctx)
	require.NoError(t, err)
	applier := &storeApply{tx: tx, ot: tree, hook: &recordingHook{}}
	require.NoError(t, applier.Apply(ctx))
	require.NoError(t, tx.Commit())

	// a new change arrives; it gets a real addSeq, unlike the legacy ones
	_, err = tree.AddRawChanges(ctx, objecttree.RawChangesPayload{
		NewHeads: []string{"6"},
		RawChanges: []*treechangeproto.RawTreeChangeWithId{
			objecttree.NewMockChangeCreator(func() anystore.DB { return createStore(ctx, t) }).
				CreateRaw("6", tree.AclList().Head().Id, "0", false, "3", "5"),
		},
	})
	require.NoError(t, err)

	tx, err = state.NewTx(ctx)
	require.NoError(t, err)
	defer tx.Rollback()

	hook := &recordingHook{}
	applier = &storeApply{tx: tx, ot: tree, hook: hook}
	require.NoError(t, applier.Apply(ctx))

	assert.Equal(t, []string{"6"}, hook.iterated, "only the change added since the watermark")
}
