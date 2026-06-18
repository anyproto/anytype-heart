package chatmodel

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- synthetic DAG helpers ----------------------------------------------

// testDag holds a synthetic change DAG with OrderIds that are a valid linear
// extension of causality: nodes are declared parents-first and OrderId is the
// zero-padded declaration index, so string comparison == topological order
// (the invariant production lexids satisfy).
type testDag struct {
	order []string // ids in OrderId order
	meta  map[string]ChangeMeta
}

func newDag() *testDag { return &testDag{meta: map[string]ChangeMeta{}} }

func (d *testDag) add(id string, prev ...string) *testDag {
	for _, p := range prev {
		if _, ok := d.meta[p]; !ok {
			panic("parent declared after child: " + p)
		}
	}
	d.meta[id] = ChangeMeta{PrevIds: prev, OrderId: fmt.Sprintf("o%04d", len(d.order))}
	d.order = append(d.order, id)
	return d
}

func (d *testDag) resolver() ChangeResolver {
	return func(id string) (ChangeMeta, bool) {
		m, ok := d.meta[id]
		return m, ok
	}
}

// heads = nodes no other node lists as a parent.
func (d *testDag) heads() []string {
	referenced := map[string]bool{}
	for _, m := range d.meta {
		for _, p := range m.PrevIds {
			referenced[p] = true
		}
	}
	var hs []string
	for _, id := range d.order {
		if !referenced[id] {
			hs = append(hs, id)
		}
	}
	return hs
}

// ---- the oracle: literal read* ------------------------------------------

// oracleBand recomputes the band by definition: the causal down-set of the
// resolved frontier (DFS over parents), then {x : g(x) ≤ maxF ∧ x ∉ closure}.
// Deliberately independent of ComputeBand's traversal.
func oracleBand(d *testDag, frontier []string, resolve ChangeResolver) (maxF string, band []string) {
	closure := map[string]bool{}
	var stack []string
	for _, h := range frontier {
		m, ok := resolve(h)
		if !ok {
			continue // pending
		}
		if m.OrderId > maxF {
			maxF = m.OrderId
		}
		if !closure[h] {
			closure[h] = true
			stack = append(stack, h)
		}
	}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		m, _ := resolve(id)
		for _, p := range m.PrevIds {
			if _, ok := resolve(p); ok && !closure[p] {
				closure[p] = true
				stack = append(stack, p)
			}
		}
	}
	if maxF == "" {
		return "", nil
	}
	for _, id := range d.order {
		if d.meta[id].OrderId <= maxF && !closure[id] {
			band = append(band, id)
		}
	}
	return maxF, band
}

func assertMatchesOracle(t *testing.T, d *testDag, frontier []string) BandResult {
	t.Helper()
	got := ComputeBand(frontier, d.heads(), d.resolver())
	wantMaxF, wantBand := oracleBand(d, frontier, d.resolver())
	require.Equal(t, wantMaxF, got.MaxFrontierOrderId, "cut must match oracle")
	assert.ElementsMatch(t, wantBand, got.Candidates, "band must match the read* complement below the cut")
	return got
}

// ---- named scenarios ------------------------------------------------------

// Linear history: the band is empty for ANY frontier position — the dominance
// of the common case. Reading mid-chain leaves the tail to the indexed range.
func TestComputeBand_LinearAlwaysEmpty(t *testing.T) {
	d := newDag().add("G").add("m1", "G").add("m2", "m1").add("m3", "m2").add("m4", "m3")
	for _, f := range [][]string{{"m4"}, {"m2"}, {"G"}} {
		got := assertMatchesOracle(t, d, f)
		assert.Empty(t, got.Candidates, "frontier %v", f)
	}
}

// The canonical cross-device case: a1 ∥ b1 under G, frontier {b1}. a1 sorts
// before b1 but is NOT covered → it is the band. This is the message the
// (OrderId, AddSeq) watermark mis-classified depending on local apply order;
// here the result is a pure function of (DAG, frontier).
func TestComputeBand_CanonicalConcurrentSibling(t *testing.T) {
	d := newDag().add("G").add("a1", "G").add("b1", "G")
	got := assertMatchesOracle(t, d, []string{"b1"})
	assert.Equal(t, []string{"a1"}, got.Candidates)

	// reading the OTHER branch instead: a1 covered, b1 is past the cut (tail) —
	// band empty.
	got = assertMatchesOracle(t, d, []string{"a1"})
	assert.Empty(t, got.Candidates)
}

// A merge closes the band: after reading the merge change, both branches are
// in its causal past.
func TestComputeBand_MergeCloses(t *testing.T) {
	d := newDag().add("G").add("a1", "G").add("b1", "G").add("mm", "a1", "b1")
	got := assertMatchesOracle(t, d, []string{"mm"})
	assert.Empty(t, got.Candidates)
}

// Multi-head frontier: both branch tips read (e.g. merged from two devices'
// read markers) — covered without any merge change existing.
func TestComputeBand_MultiHeadFrontier(t *testing.T) {
	d := newDag().add("G").add("a1", "G").add("a2", "a1").add("b1", "G").add("b2", "b1")
	got := assertMatchesOracle(t, d, []string{"a2", "b2"})
	assert.Empty(t, got.Candidates)

	// partial multi-head: {a2, b1} leaves b2 past the cut? No: g(b2) > g(b1)
	// but g(b2) > g(a2)? declaration order: b2 is last → tail. Band empty.
	got = assertMatchesOracle(t, d, []string{"a2", "b1"})
	assert.Empty(t, got.Candidates)

	// {b2, a1}: a2 has g < g(b2) and is not covered → band = {a2}.
	got = assertMatchesOracle(t, d, []string{"b2", "a1"})
	assert.Equal(t, []string{"a2"}, got.Candidates)
}

// A pending (not-yet-local) frontier head is omitted from the cut — fewer
// reads, never false reads (the safe direction). Matches the existing
// pending-head behavior of the seen-heads machinery.
func TestComputeBand_PendingHeadOmitted(t *testing.T) {
	d := newDag().add("G").add("a1", "G").add("b1", "G")
	got := ComputeBand([]string{"b1", "ghost"}, d.heads(), d.resolver())
	assert.Equal(t, []string{"ghost"}, got.PendingHeads)
	assert.Equal(t, []string{"b1"}, got.ResolvedHeads)
	assert.Equal(t, []string{"a1"}, got.Candidates, "same as frontier {b1}")

	// frontier entirely pending → no cut, band empty (everything is tail).
	got = ComputeBand([]string{"ghost"}, d.heads(), d.resolver())
	assert.Empty(t, got.MaxFrontierOrderId)
	assert.Empty(t, got.Candidates)
}

// Late in-past insert: a change landing BELOW an old frontier cut joins the
// band on recompute (and by spec Theorem 3 can be appended without any
// ancestry check in the incremental path — pinned here by recompute equality).
func TestComputeBand_LateInPastInsert(t *testing.T) {
	d := newDag().add("G").add("m1", "G").add("m2", "m1")
	got := assertMatchesOracle(t, d, []string{"m2"})
	assert.Empty(t, got.Candidates)

	// late arrival anchored at G: sorts… not below m2 in this synthetic
	// encoding (OrderId = declaration index), so model it properly: rebuild
	// the dag with the late change in its true topological slot.
	d2 := newDag().add("G").add("late", "G").add("m1", "G").add("m2", "m1")
	got = assertMatchesOracle(t, d2, []string{"m2"})
	assert.Equal(t, []string{"late"}, got.Candidates, "in-past insert below the cut is unread")
}

// markunread-to-ancient shape: an old frontier in a long chat. The band stays
// tiny (one concurrent straggler below the old cut); the tail is the range
// query's job. Also pins the walk-size bound: O(tail + band + boundary).
func TestComputeBand_AncientFrontier(t *testing.T) {
	// straggler is declared in its true topological slot — right after G — so
	// its OrderId is BELOW the old cut (an in-past insert, concurrent with the
	// whole chain).
	d := newDag().add("G").add("straggler", "G")
	prev := "G"
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("m%02d", i)
		d.add(id, prev)
		prev = id
	}
	got := assertMatchesOracle(t, d, []string{"m05"})
	assert.Equal(t, []string{"straggler"}, got.Candidates)
	assert.LessOrEqual(t, got.Visited, 52+2, "walk bounded by tail+band+boundary, not repeated work")
}

// ---- the property test ----------------------------------------------------

// Randomized DAGs × randomized frontiers (including pending heads) — the walk
// must equal the literal read* complement below the cut, every time.
// Deterministic seed: failures are reproducible.
func TestComputeBand_PropertyMatchesOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(0xD_2026_0610))
	for iter := 0; iter < 3000; iter++ {
		d := newDag().add("n000")
		n := 2 + rng.Intn(30)
		for i := 1; i < n; i++ {
			id := fmt.Sprintf("n%03d", i)
			// 1..3 parents sampled from earlier nodes ⇒ valid DAG, OrderId a
			// linear extension by construction.
			k := 1 + rng.Intn(3)
			seen := map[string]bool{}
			var prev []string
			for j := 0; j < k; j++ {
				p := d.order[rng.Intn(len(d.order))]
				if !seen[p] {
					seen[p] = true
					prev = append(prev, p)
				}
			}
			d.add(id, prev...)
		}
		// frontier: 0..3 random nodes, ~15% replaced by a pending ghost
		var frontier []string
		for j := 0; j < rng.Intn(4); j++ {
			if rng.Intn(100) < 15 {
				frontier = append(frontier, fmt.Sprintf("ghost%d", j))
			} else {
				frontier = append(frontier, d.order[rng.Intn(len(d.order))])
			}
		}
		got := ComputeBand(frontier, d.heads(), d.resolver())
		wantMaxF, wantBand := oracleBand(d, frontier, d.resolver())
		require.Equalf(t, wantMaxF, got.MaxFrontierOrderId, "iter %d: cut", iter)
		sort.Strings(got.Candidates)
		sort.Strings(wantBand)
		require.Equalf(t, wantBand, got.Candidates, "iter %d: band\nfrontier=%v dag=%v", iter, frontier, d.meta)
	}
}
