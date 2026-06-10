package main

import (
	"sort"
)

// ChangeNode is the minimal projection of a stored CRDT change needed to
// analyze the read-counter band: its id, its parents (PrevIds), and its
// replica-invariant order key (OrderId). AddSeq is carried for completeness but
// is deliberately NOT used by any metric here — the whole point is that the band
// and the convergent semantics are a function of (DAG, OrderId) only.
type ChangeNode struct {
	Id      string
	PrevIds []string
	OrderId string
	AddSeq  uint64
}

// TreeMetrics summarizes the concurrency structure of one change DAG and the
// per-single-head "band" distribution. The band(H) for a head H is the number of
// events that sort at-or-before H in OrderId but are NOT causal ancestors of H —
// i.e. exactly the events the dominance read-counter can misclassify, and the
// extra work Option B's bounded fallback would do if the user's frontier were {H}.
// See docs/superpowers/specs/2026-06-03-read-counter-hybrid-design.md.
type TreeMetrics struct {
	TreeID string
	N      int // number of changes
	Roots  int // changes with no in-tree parent
	Heads  int // current heads (tips) of the whole DAG

	// Merge structure (cheap proxy for Property M: forks merge promptly).
	Merges        int // changes with >=2 parents (a merge of concurrent branches)
	MaxMergeWidth int // max number of parents on any change

	// Open-width over the OrderId sweep: at each prefix, how many "open" tips exist.
	// Property M predicts width is 1 almost everywhere with rare, shallow spikes.
	MaxOpenWidth      int         // peak concurrent open tips
	WidthHist         map[int]int // open-width -> number of OrderId positions at that width
	ConcurrentPos     int         // positions with open width >= 2
	LongestConcurrent int         // longest consecutive run of open width >= 2 (longest unmerged stretch)

	// Fork span: for each merge, the OrderId-index gap back to its earliest merged
	// parent — how far apart (in global order) the merged tips were. Small => shallow.
	ForkSpanP50 int
	ForkSpanP95 int
	ForkSpanMax int

	// Band(single-head) distribution over all heads H (band(H) = rank(H) - |past(H)|).
	BandComputed bool
	BandMax      int
	BandP50      int
	BandP95      int
	BandMean     float64
	BandNonZero  int // number of heads H with band(H) > 0
}

// ComputeMetrics analyzes one tree's changes. If computeBand is false (or N is
// above the caller's threshold), the O(N) structural metrics are still produced;
// only the O(N^2/64) exact band distribution is skipped.
func ComputeMetrics(treeID string, nodes []ChangeNode, computeBand bool) TreeMetrics {
	m := TreeMetrics{TreeID: treeID, N: len(nodes), WidthHist: map[int]int{}}
	if len(nodes) == 0 {
		return m
	}

	// Sort by OrderId. Because OrderId is a linear extension of causality, this is
	// a topological order: every parent precedes its children.
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].OrderId < nodes[j].OrderId })

	idToIndex := make(map[string]int, len(nodes))
	for i, n := range nodes {
		idToIndex[n.Id] = i
	}

	// remainingChildren[i] = how many in-tree changes list node i as a parent.
	remainingChildren := make([]int, len(nodes))
	for _, n := range nodes {
		for _, p := range n.PrevIds {
			if pi, ok := idToIndex[p]; ok {
				remainingChildren[pi]++
			}
		}
		if pc := inTreeParents(n.PrevIds, idToIndex); pc == 0 {
			m.Roots++
		}
		if len(n.PrevIds) >= 2 {
			m.Merges++
		}
		if len(n.PrevIds) > m.MaxMergeWidth {
			m.MaxMergeWidth = len(n.PrevIds)
		}
	}

	// Open-width sweep + fork spans.
	liveHeads := make(map[string]struct{}, 16)
	var forkSpans []int
	curRun := 0
	for i, n := range nodes {
		minParentIdx := -1
		mergedParents := 0
		for _, p := range n.PrevIds {
			if pi, ok := idToIndex[p]; ok {
				delete(liveHeads, p) // p is no longer a tip
				mergedParents++
				if minParentIdx == -1 || pi < minParentIdx {
					minParentIdx = pi
				}
			}
		}
		liveHeads[n.Id] = struct{}{}
		w := len(liveHeads)
		m.WidthHist[w]++
		if w > m.MaxOpenWidth {
			m.MaxOpenWidth = w
		}
		if w >= 2 {
			m.ConcurrentPos++
			curRun++
			if curRun > m.LongestConcurrent {
				m.LongestConcurrent = curRun
			}
		} else {
			curRun = 0
		}
		// A merge (>=2 in-tree parents) closes a fork; record its OrderId span.
		if mergedParents >= 2 && minParentIdx >= 0 {
			forkSpans = append(forkSpans, i-minParentIdx)
		}
	}
	m.Heads = len(liveHeads)
	m.ForkSpanP50, m.ForkSpanP95, m.ForkSpanMax = pctl(forkSpans, 50), pctl(forkSpans, 95), maxInt(forkSpans)

	if !computeBand {
		return m
	}

	// Exact per-single-head band via causal-past popcounts.
	// past[i] = bitset of ancestors-or-self of node i (indices). pastSize = popcount.
	// band(i) = rank(i) - pastSize(i), rank(i)=i+1 (events with OrderId<=OrderId(i)).
	// We free a node's bitset once its last child has consumed it, so live memory is
	// O(openWidth * N/64), not O(N^2/64).
	m.BandComputed = true
	past := make([]bitset, len(nodes))
	bands := make([]int, len(nodes))
	var bandSum, bandMax, nonZero int
	for i, n := range nodes {
		b := newBitset(len(nodes))
		for _, p := range n.PrevIds {
			if pi, ok := idToIndex[p]; ok {
				b.or(past[pi])
				remainingChildren[pi]--
				if remainingChildren[pi] == 0 {
					past[pi] = nil // free: no future node references it
				}
			}
		}
		b.set(i)
		pastSize := b.count()
		band := (i + 1) - pastSize
		if band < 0 {
			band = 0 // defensive; cannot happen since past(i) ⊆ {OrderId<=OrderId(i)}
		}
		bands[i] = band
		bandSum += band
		if band > bandMax {
			bandMax = band
		}
		if band > 0 {
			nonZero++
		}
		if remainingChildren[i] == 0 {
			past[i] = nil // leaf: free immediately
		} else {
			past[i] = b
		}
	}
	m.BandMax = bandMax
	m.BandNonZero = nonZero
	m.BandMean = float64(bandSum) / float64(len(nodes))
	m.BandP50 = pctl(bands, 50)
	m.BandP95 = pctl(bands, 95)
	return m
}

func inTreeParents(prevIds []string, idToIndex map[string]int) int {
	c := 0
	for _, p := range prevIds {
		if _, ok := idToIndex[p]; ok {
			c++
		}
	}
	return c
}

// --- tiny bitset ---

type bitset []uint64

func newBitset(n int) bitset { return make(bitset, (n+63)/64) }
func (b bitset) set(i int)   { b[i>>6] |= 1 << (uint(i) & 63) }
func (b bitset) or(o bitset) {
	if o == nil {
		return
	}
	for i := range o {
		b[i] |= o[i]
	}
}
func (b bitset) count() int {
	c := 0
	for _, w := range b {
		c += popcount(w)
	}
	return c
}

func popcount(x uint64) int {
	c := 0
	for x != 0 {
		x &= x - 1
		c++
	}
	return c
}

// --- small stats helpers ---

func pctl(xs []int, p int) int {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int(nil), xs...)
	sort.Ints(s)
	idx := (p * (len(s) - 1)) / 100
	return s[idx]
}

func maxInt(xs []int) int {
	mx := 0
	for _, x := range xs {
		mx = max(mx, x)
	}
	return mx
}
