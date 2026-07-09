package objectgc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// ids returns the ids of the items, for order-insensitive assertions.
func ids(items []OrphanItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Details.GetString(bundle.RelationKeyId))
	}
	return out
}

// find returns the item with the given id.
func find(t *testing.T, items []OrphanItem, id string) OrphanItem {
	t.Helper()
	for _, it := range items {
		if it.Details.GetString(bundle.RelationKeyId) == id {
			return it
		}
	}
	t.Fatalf("item %s not found in %v", id, ids(items))
	return OrphanItem{}
}

func TestListOrphans_ArchivedParent_ChildIsRoot(t *testing.T) {
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, basicObjectWithRef("child", "parent", "block1", []string{"parent"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, ids(items))
	child := find(t, items, "child")
	assert.True(t, child.IsRoot)
	assert.Equal(t, OrphanReasonContextArchived, child.Reason)
}

func TestListOrphans_DeletedParent_ReasonDeleted(t *testing.T) {
	fx := newFixture(t)
	fx.addObject(t, deletedObject("parent"))
	fx.addObject(t, basicObjectWithRef("child", "parent", "block1", []string{"parent"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, ids(items))
	assert.Equal(t, OrphanReasonContextDeleted, find(t, items, "child").Reason)
}

func TestListOrphans_LinkRemoved_ParentActive_ReasonUnlinked(t *testing.T) {
	// parent is alive but no longer links the child (child's backlinks are empty)
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, basicObjectWithRef("child", "parent", "block1", nil))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"child"}, ids(items))
	child := find(t, items, "child")
	assert.True(t, child.IsRoot)
	assert.Equal(t, OrphanReasonContextUnlinked, child.Reason)
}

func TestListOrphans_ReachableObject_Excluded(t *testing.T) {
	// parent is alive and still links the child → child has an active backlink outside S
	fx := newFixture(t)
	fx.addObject(t, regularObject("parent"))
	fx.addObject(t, basicObjectWithRef("child", "parent", "block1", []string{"parent"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestListOrphans_CascadeEviction(t *testing.T) {
	// X has an external active backlink → evicted. Y's only backlink is X → also evicted.
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, regularObject("external"))
	fx.addObject(t, basicObjectWithRef("X", "parent", "block1", []string{"external"}))
	fx.addObject(t, basicObjectWithRef("Y", "parent", "block1", []string{"X"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestListOrphans_Descendant_NotRoot(t *testing.T) {
	// parent archived → P is a root; X created in P, backlinked only by P → descendant
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, basicObjectWithRef("P", "parent", "block1", []string{"parent"}))
	fx.addObject(t, basicObjectWithRef("X", "P", "block1", []string{"P"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"P", "X"}, ids(items))
	assert.True(t, find(t, items, "P").IsRoot)
	assert.Equal(t, OrphanReasonContextArchived, find(t, items, "P").Reason)
	x := find(t, items, "X")
	assert.False(t, x.IsRoot)
	assert.Equal(t, OrphanReasonNone, x.Reason)
}

func TestListOrphans_Cycle_DeterministicRoot(t *testing.T) {
	// A created in B, B created in A, nothing else links them
	fx := newFixture(t)
	fx.addObject(t, basicObjectWithRef("A", "B", "block1", []string{"B"}))
	fx.addObject(t, basicObjectWithRef("B", "A", "block1", []string{"A"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"A", "B"}, ids(items))
	assert.True(t, find(t, items, "A").IsRoot, "lowest id is elected root")
	assert.False(t, find(t, items, "B").IsRoot)
}

func TestListOrphans_ParentAbsentFromStore_Skipped(t *testing.T) {
	// "ghost" is never added to the store → sync gap, not a deletion
	fx := newFixture(t)
	fx.addObject(t, basicObjectWithRef("child", "ghost", "block1", nil))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestListOrphans_ChildOfSyncGappedCandidate_StillListed(t *testing.T) {
	// "ghost" never synced → P is dropped by the sync-gap guard. P stays a live object, so its
	// child C is still an orphan (P no longer links it) and must be listed.
	//
	// The guard therefore has to decide "is my parent a candidate" against the *original* candidate
	// set: reading the map it is deleting from makes C's fate depend on Go's randomized map
	// iteration order. Repeat to make that flake deterministic.
	for i := 0; i < 50; i++ {
		fx := newFixture(t)
		fx.addObject(t, basicObjectWithRef("P", "ghost", "block1", nil))
		fx.addObject(t, basicObjectWithRef("C", "P", "block1", nil))

		items, err := fx.ListOrphans(testSpaceId)

		require.NoError(t, err)
		require.ElementsMatch(t, []string{"C"}, ids(items))
		assert.True(t, find(t, items, "C").IsRoot)
		assert.Equal(t, OrphanReasonContextUnlinked, find(t, items, "C").Reason)
	}
}

func TestListOrphans_EmptyRef_Excluded(t *testing.T) {
	// collection-created object: empty createdInContextRef
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, basicObject("child", "parent", []string{"parent"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestListOrphans_SystemLayout_Excluded(t *testing.T) {
	// the object carries a ref, so only the non-GC-eligible layout can exclude it
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, systemObjectWithRef("sysobj", "parent", "block1", []string{"parent"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestListOrphans_FileIncluded(t *testing.T) {
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.addObject(t, fileObjectWithRef("f1", "parent", "block1", []string{"parent"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"f1"}, ids(items))
	assert.True(t, find(t, items, "f1").IsRoot)
}

func TestListOrphans_Ignored_Excluded_AndDropsSubtree(t *testing.T) {
	// ignoring root B must also drop its child C: C's only backlink is B, which is active and
	// no longer a candidate → C is evicted.
	fx := newFixture(t)
	fx.addObject(t, archivedObject("parent"))
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:                      domain.String("B"),
			bundle.RelationKeyResolvedLayout:          domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyCreatedInContext:        domain.String("parent"),
			bundle.RelationKeyCreatedInContextRef:     domain.String("block1"),
			bundle.RelationKeyBacklinks:               domain.StringList([]string{"parent"}),
			bundle.RelationKeyCreatedInContextIgnored: domain.Bool(true),
		},
	})
	fx.addObject(t, basicObjectWithRef("C", "B", "block1", []string{"B"}))

	items, err := fx.ListOrphans(testSpaceId)

	require.NoError(t, err)
	assert.Empty(t, items)
}
