package chatobject

import (
	"context"
	"math/rand"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
)

// Cross-device convergence gate for the causal-ordinal read model (Option D,
// docs/superpowers/specs/2026-06-10-read-counter-option-d-causal-ordinals.md
// §11 test 1). This is the dimension the previous scenario harness could not
// express: each "device" here is a REAL storage-backed objecttree.ObjectTree
// that received the same change DAG in a DIFFERENT batch/apply order, so all
// device-local artifacts (AddSeq, lexid OrderId strings, arrival order)
// genuinely differ — and the read model's output must not.
//
// Context this gate records: the (OrderId, AddSeq) watermark prototype
// classified the canonical a1∥b1 case differently per apply order (read on
// the device that applied a1 first, unread on the other — a permanent 0 vs 1
// count divergence; theory doc §5.3). ComputeBand consumes no device-local
// input, so both devices must agree, and agree with the literal read*.

// dagChange is a synthetic change: id + parents. Ids are author-chosen and
// stable, so the hash-tiebroken sibling order — and thus the OrderId
// COMPARISON order — is deterministic across devices (lexid strings differ).
type dagChange struct {
	ID   string
	Prev []string
}

type deviceTree struct {
	tree    objecttree.ObjectTree
	orderOf map[string]string // changeId -> this device's OrderId string
}

func deviceAclList(t testing.TB) list.AclList {
	keys, err := accountdata.NewRandom()
	require.NoError(t, err)
	acl, err := list.NewInMemoryDerivedAcl("spaceId", keys)
	require.NoError(t, err)
	return acl
}

func deviceAnystore(t testing.TB) anystore.DB {
	db, err := anystore.Open(context.Background(), filepath.Join(t.TempDir(), "tree.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// buildDeviceTree materializes the DAG into a real objecttree by applying the
// non-root changes in the given per-device batches — one AddRawChanges call
// per batch, mirroring how sync delivers changes in arrival-order groups.
// dag[0] is the root. Batches must be causally valid (parents in earlier
// batches or the same batch); every change must appear in exactly one batch.
func buildDeviceTree(t testing.TB, dag []dagChange, batches [][]string) *deviceTree {
	ctx := context.Background()
	require.NotEmpty(t, dag)
	require.Empty(t, dag[0].Prev, "dag[0] is the root and must have no Prev")
	rootId := dag[0].ID

	byId := map[string]dagChange{}
	for _, c := range dag {
		byId[c.ID] = c
	}

	acl := deviceAclList(t)
	mcc := objecttree.NewMockChangeCreator(func() anystore.DB { return deviceAnystore(t) })
	storage := mcc.CreateNewTreeStorage(t.(*testing.T), rootId, acl.Head().Id, false)
	if setter, ok := storage.(interface{ SetAddSeq(*atomic.Uint64) }); ok {
		setter.SetAddSeq(&atomic.Uint64{})
	}
	tree, err := objecttree.BuildTestableTree(storage, acl)
	require.NoError(t, err)
	tree.SetFlusher(objecttree.MarkNewChangeFlusher())

	applied := map[string]bool{rootId: true}
	for _, batch := range batches {
		var raws []*treechangeproto.RawTreeChangeWithId
		for _, id := range batch {
			c, ok := byId[id]
			require.Truef(t, ok, "batch references unknown change %s", id)
			require.NotEmpty(t, c.Prev, "root must not be in batches")
			raws = append(raws, mcc.CreateRaw(c.ID, acl.Head().Id, rootId, false, c.Prev...))
			applied[id] = true
		}
		// expected heads after this batch = heads of the applied prefix
		_, err = tree.AddRawChanges(ctx, objecttree.RawChangesPayload{
			NewHeads:   prefixHeads(dag, applied),
			RawChanges: raws,
		})
		require.NoError(t, err)
	}
	for _, c := range dag {
		require.Truef(t, applied[c.ID], "change %s missing from batches", c.ID)
	}

	orderOf := map[string]string{}
	require.NoError(t, tree.IterateRoot(nil, func(ch *objecttree.Change) bool {
		orderOf[ch.Id] = ch.OrderId
		return true
	}))
	return &deviceTree{tree: tree, orderOf: orderOf}
}

// prefixHeads = applied changes not referenced as Prev by any applied change.
func prefixHeads(dag []dagChange, applied map[string]bool) []string {
	referenced := map[string]bool{}
	for _, c := range dag {
		if !applied[c.ID] {
			continue
		}
		for _, p := range c.Prev {
			referenced[p] = true
		}
	}
	var heads []string
	for _, c := range dag {
		if applied[c.ID] && !referenced[c.ID] {
			heads = append(heads, c.ID)
		}
	}
	return heads
}

// storageResolver adapts a device's tree storage to chatmodel.ChangeResolver —
// the exact production read (StorageChange.PrevIds / OrderId, point lookups).
func storageResolver(t testing.TB, dt *deviceTree) chatmodel.ChangeResolver {
	ctx := context.Background()
	return func(id string) (chatmodel.ChangeMeta, bool) {
		ch, err := dt.tree.Storage().Get(ctx, id)
		if err != nil {
			return chatmodel.ChangeMeta{}, false
		}
		return chatmodel.ChangeMeta{PrevIds: ch.PrevIds, OrderId: ch.OrderId}, true
	}
}

func bandOn(t testing.TB, dt *deviceTree, frontier []string) []string {
	got := chatmodel.ComputeBand(frontier, dt.tree.Heads(), storageResolver(t, dt))
	sort.Strings(got.Candidates)
	return got.Candidates
}

// readClosure = literal read*: the causal down-set of the frontier on the DAG
// (device-independent by construction — computed from the declared DAG).
func readClosure(dag []dagChange, frontier []string) map[string]bool {
	byId := map[string]dagChange{}
	for _, c := range dag {
		byId[c.ID] = c
	}
	closure := map[string]bool{}
	stack := append([]string(nil), frontier...)
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if closure[id] {
			continue
		}
		if _, ok := byId[id]; !ok {
			continue
		}
		closure[id] = true
		stack = append(stack, byId[id].Prev...)
	}
	return closure
}

// assertOrderComparisonsConverge pins the precondition the whole model rests
// on: per-device OrderId STRINGS may differ, but every pairwise comparison
// sign is identical across devices.
func assertOrderComparisonsConverge(t *testing.T, devices []*deviceTree) {
	t.Helper()
	var ids []string
	for id := range devices[0].orderOf {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, d := range devices[1:] {
		require.Equal(t, len(devices[0].orderOf), len(d.orderOf), "devices must hold the same change-set")
		for i := 0; i < len(ids); i++ {
			for j := i + 1; j < len(ids); j++ {
				a := strings.Compare(devices[0].orderOf[ids[i]], devices[0].orderOf[ids[j]])
				b := strings.Compare(d.orderOf[ids[i]], d.orderOf[ids[j]])
				require.Equalf(t, a, b, "OrderId comparison for (%s,%s) must converge across devices", ids[i], ids[j])
			}
		}
	}
}

// TestReadCore_CrossDeviceConvergence_Canonical is the gate for the exact
// case that produced the permanent 0-vs-1 watermark divergence:
// G; a1 ∥ b1; device A received a1 then b1, device B received b1 then a1
// (so on A: AddSeq(a1) < AddSeq(b1); on B the reverse). Frontier = {b1}
// (synced as ids). Both devices must report band {a1} — a1 unread everywhere.
func TestReadCore_CrossDeviceConvergence_Canonical(t *testing.T) {
	dag := []dagChange{
		{ID: "G"},
		{ID: "a1", Prev: []string{"G"}},
		{ID: "b1", Prev: []string{"G"}},
	}
	devA := buildDeviceTree(t, dag, [][]string{{"a1"}, {"b1"}})
	devB := buildDeviceTree(t, dag, [][]string{{"b1"}, {"a1"}})
	assertOrderComparisonsConverge(t, []*deviceTree{devA, devB})

	frontier := []string{"b1"}
	bandA := bandOn(t, devA, frontier)
	bandB := bandOn(t, devB, frontier)

	assert.Equal(t, []string{"a1"}, bandA, "device A: a1 is concurrent with the read head -> unread")
	assert.Equal(t, bandA, bandB, "both devices must compute the identical unread band")

	closure := readClosure(dag, frontier)
	assert.False(t, closure["a1"], "oracle: a1 is not in the causal past of b1")
}

// TestReadCore_CrossDeviceConvergence_BranchesAndLateInsert exercises a richer
// shape: two branches, a merge on one side, a late in-past insert, multi-head
// frontiers — across three devices with different batch deliveries.
func TestReadCore_CrossDeviceConvergence_BranchesAndLateInsert(t *testing.T) {
	dag := []dagChange{
		{ID: "G"},
		{ID: "a1", Prev: []string{"G"}},
		{ID: "a2", Prev: []string{"a1"}},
		{ID: "b1", Prev: []string{"G"}},
		{ID: "b2", Prev: []string{"b1"}},
		{ID: "mm", Prev: []string{"a2", "b2"}}, // merge
		{ID: "c1", Prev: []string{"G"}},        // late in-past insert (third branch)
	}
	devices := []*deviceTree{
		buildDeviceTree(t, dag, [][]string{{"a1", "a2"}, {"b1", "b2"}, {"mm"}, {"c1"}}),
		buildDeviceTree(t, dag, [][]string{{"b1"}, {"c1"}, {"b2", "a1"}, {"a2", "mm"}}),
		buildDeviceTree(t, dag, [][]string{{"c1", "a1", "b1", "a2", "b2", "mm"}}), // one batch
	}
	assertOrderComparisonsConverge(t, devices)

	frontiers := [][]string{
		{"b2"},         // one branch read: other branch's below-cut part + c1 are order-dependent on g
		{"a2", "b1"},   // multi-head
		{"mm"},         // merge read: only c1 can remain (if below the cut)
		{"mm", "c1"},   // everything read
		{"b2", "ghost"}, // pending head omitted
	}
	for _, f := range frontiers {
		want := bandOn(t, devices[0], f)
		for i, d := range devices[1:] {
			assert.Equalf(t, want, bandOn(t, d, f), "frontier %v: device %d diverged", f, i+1)
		}
		// oracle: candidates must be exactly the non-covered changes at/below
		// the per-device cut — verified on device 0 (cut position is
		// comparison-invariant, so the set is the same on all).
		closure := readClosure(dag, f)
		maxF := ""
		for _, h := range f {
			if o, ok := devices[0].orderOf[h]; ok && o > maxF {
				maxF = o
			}
		}
		var oracle []string
		for id, o := range devices[0].orderOf {
			if maxF != "" && o <= maxF && !closure[id] {
				oracle = append(oracle, id)
			}
		}
		sort.Strings(oracle)
		if oracle == nil {
			oracle = []string{}
		}
		got := want
		if got == nil {
			got = []string{}
		}
		assert.Equalf(t, oracle, got, "frontier %v: band must equal the read* complement below the cut", f)
	}
}

// TestReadCore_CrossDeviceConvergence_RandomDeliveries fuzzes the delivery
// dimension: a fixed concurrent DAG delivered to pairs of devices in random
// valid batch partitions; for random frontiers the band id-set must be
// identical on both. Deterministic seed.
func TestReadCore_CrossDeviceConvergence_RandomDeliveries(t *testing.T) {
	if testing.Short() {
		t.Skip("builds many real trees")
	}
	dag := []dagChange{
		{ID: "G"},
		{ID: "a1", Prev: []string{"G"}},
		{ID: "a2", Prev: []string{"a1"}},
		{ID: "b1", Prev: []string{"G"}},
		{ID: "b2", Prev: []string{"b1"}},
		{ID: "c1", Prev: []string{"G"}},
		{ID: "m1", Prev: []string{"a2", "b1"}},
	}
	nonRoot := []dagChange{}
	for _, c := range dag[1:] {
		nonRoot = append(nonRoot, c)
	}
	rng := rand.New(rand.NewSource(0xD_C40))

	randomBatches := func() [][]string {
		// random topological order of non-root changes…
		remaining := append([]dagChange(nil), nonRoot...)
		placed := map[string]bool{"G": true}
		var order []string
		for len(remaining) > 0 {
			var ready []int
			for i, c := range remaining {
				ok := true
				for _, p := range c.Prev {
					if !placed[p] {
						ok = false
						break
					}
				}
				if ok {
					ready = append(ready, i)
				}
			}
			pick := ready[rng.Intn(len(ready))]
			order = append(order, remaining[pick].ID)
			placed[remaining[pick].ID] = true
			remaining = append(remaining[:pick], remaining[pick+1:]...)
		}
		// …cut into random batches
		var batches [][]string
		for start := 0; start < len(order); {
			end := start + 1 + rng.Intn(len(order)-start)
			batches = append(batches, order[start:end])
			start = end
		}
		return batches
	}

	allIds := []string{"a1", "a2", "b1", "b2", "c1", "m1"}
	for iter := 0; iter < 12; iter++ {
		devA := buildDeviceTree(t, dag, randomBatches())
		devB := buildDeviceTree(t, dag, randomBatches())
		assertOrderComparisonsConverge(t, []*deviceTree{devA, devB})
		for fIter := 0; fIter < 6; fIter++ {
			var frontier []string
			for j := 0; j < 1+rng.Intn(2); j++ {
				frontier = append(frontier, allIds[rng.Intn(len(allIds))])
			}
			require.Equalf(t, bandOn(t, devA, frontier), bandOn(t, devB, frontier),
				"iter %d frontier %v: devices diverged (deliveries A vs B differ)", iter, frontier)
		}
	}
}
