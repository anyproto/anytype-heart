package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func node(id, order string, prev ...string) ChangeNode {
	return ChangeNode{Id: id, OrderId: order, PrevIds: prev}
}

// A purely linear history has width 1 everywhere and a zero band: the dominance
// fast path is already exact and Option B's fallback never runs.
func TestMetrics_LinearNoBand(t *testing.T) {
	nodes := []ChangeNode{
		node("G", "001"),
		node("m1", "002", "G"),
		node("m2", "003", "m1"),
		node("m3", "004", "m2"),
	}
	m := ComputeMetrics("t", nodes, true)
	assert.Equal(t, 4, m.N)
	assert.Equal(t, 1, m.Roots)
	assert.Equal(t, 1, m.Heads)
	assert.Equal(t, 0, m.Merges)
	assert.Equal(t, 1, m.MaxOpenWidth)
	assert.Equal(t, 0, m.ConcurrentPos)
	assert.Equal(t, 0, m.LongestConcurrent)
	assert.True(t, m.BandComputed)
	assert.Equal(t, 0, m.BandMax)
	assert.Equal(t, 0, m.BandNonZero)
}

// The canonical cross-device case: G; a1 ∥ b1 both children of G, OrderId(a1) <
// OrderId(b1). band(b1) = 1 (a1 is the concurrent-earlier event a frontier {b1}
// does not cover) — exactly the message the dominance scheme can misclassify.
func TestMetrics_ForkBandIsCanonicalA1B1(t *testing.T) {
	nodes := []ChangeNode{
		node("G", "001"),
		node("a1", "002", "G"),
		node("b1", "003", "G"),
	}
	m := ComputeMetrics("t", nodes, true)
	assert.Equal(t, 2, m.Heads, "a1 and b1 are both tips")
	assert.Equal(t, 2, m.MaxOpenWidth)
	assert.Equal(t, 1, m.ConcurrentPos, "the position at b1 is width 2")
	assert.Equal(t, 1, m.BandMax, "band(b1) = 1 (a1)")
	assert.Equal(t, 1, m.BandNonZero)
}

// Same fork but merged by a following change (Property M). The single-head band
// for {b1} is still 1 (the user who read only b1 has a1 unread); once the user
// reads the merge, band drops to 0 — but the per-head distribution still records
// the transient 1.
func TestMetrics_ForkMergeSpan(t *testing.T) {
	nodes := []ChangeNode{
		node("G", "001"),
		node("a1", "002", "G"),
		node("b1", "003", "G"),
		node("m", "004", "a1", "b1"),
	}
	m := ComputeMetrics("t", nodes, true)
	assert.Equal(t, 1, m.Heads, "merged back to one tip")
	assert.Equal(t, 1, m.Merges)
	assert.Equal(t, 2, m.MaxMergeWidth)
	assert.Equal(t, 2, m.MaxOpenWidth)
	assert.Equal(t, 2, m.ForkSpanMax, "merge index 3 - earliest parent index 1")
	assert.Equal(t, 1, m.BandMax)
	assert.Equal(t, 1, m.BandNonZero)
}

// Two parallel branches of depth 2 before merging: band grows with the depth of
// the unmerged branch (band(b-side heads) = 2 = the two a-side events). This is
// the signal that decides Option B: if real chats keep this small (forks merge
// promptly), the bounded fallback is cheap.
func TestMetrics_DeeperConcurrencyGrowsBand(t *testing.T) {
	nodes := []ChangeNode{
		node("G", "001"),
		node("a1", "002", "G"),
		node("a2", "003", "a1"),
		node("b1", "004", "G"),
		node("b2", "005", "b1"),
		node("m", "006", "a2", "b2"),
	}
	m := ComputeMetrics("t", nodes, true)
	assert.Equal(t, 1, m.Heads)
	assert.Equal(t, 1, m.Merges)
	assert.Equal(t, 2, m.MaxOpenWidth)
	assert.Equal(t, 2, m.ConcurrentPos, "b1 and b2 positions are width 2")
	assert.Equal(t, 2, m.LongestConcurrent, "the unmerged stretch is length 2")
	assert.Equal(t, 3, m.ForkSpanMax, "merge index 5 - earliest parent (a2) index 2")
	assert.Equal(t, 2, m.BandMax, "band(b1)=band(b2)=2 (a1,a2)")
	assert.Equal(t, 2, m.BandNonZero)
}

// computeBand=false still yields the O(N) structural metrics.
func TestMetrics_StructuralOnly(t *testing.T) {
	nodes := []ChangeNode{
		node("G", "001"),
		node("a1", "002", "G"),
		node("b1", "003", "G"),
	}
	m := ComputeMetrics("t", nodes, false)
	assert.Equal(t, 2, m.MaxOpenWidth)
	assert.False(t, m.BandComputed)
	assert.Equal(t, 0, m.BandMax)
}
