package chatmodel

import "container/heap"

// This file implements the CORE of the causal-ordinal read model (Option D,
// docs/superpowers/specs/2026-06-10-read-counter-option-d-causal-ordinals.md §5):
// the band computation. Read state is causal coverage by the seen-head frontier
// F: read*(X) ⟺ ∃H∈F: X ⪯ H. The unread set decomposes (spec Theorem 1) into
//
//	tail = {counted X : OrderId(X) > maxF}        — an indexed range, repo-side
//	band = {counted X : OrderId(X) ≤ maxF, X ⋠ F} — computed here
//
// ComputeBand walks the change DAG backward from the local heads and the
// frontier simultaneously, in decreasing OrderId, stopping at the read
// boundary. It returns band CANDIDATES — unread change ids at or below the
// frontier cut. Filtering candidates to live counted messages is the caller's
// (repository's) job: this layer sees only the DAG.
//
// Everything here is a pure function of (DAG, frontier-as-id-set): no AddSeq,
// no apply order, no device-local input — which is exactly why the result is
// identical on every device of the account (spec §2).

// ChangeMeta is the minimal projection of a CRDT change the read model needs:
// its parents and its replica-invariant order key.
type ChangeMeta struct {
	PrevIds []string
	OrderId string
}

// ChangeResolver resolves a change id from local storage. ok=false means the
// change is not locally present — for a frontier head that makes it PENDING
// (omitted from the cut: fewer reads, safe over-count of unread).
type ChangeResolver func(id string) (ChangeMeta, bool)

// BandResult is the outcome of one band computation.
type BandResult struct {
	// MaxFrontierOrderId is the cut: max OrderId over the RESOLVED frontier
	// heads. Empty when no head resolved — then everything is tail and the
	// band is empty by definition.
	MaxFrontierOrderId string
	// Candidates are unread change ids with OrderId ≤ MaxFrontierOrderId
	// (any change type; the caller filters to live counted messages).
	Candidates []string
	// ResolvedHeads / PendingHeads partition the input frontier.
	ResolvedHeads []string
	PendingHeads  []string
	// Visited counts walked changes (observability: the walk-size bound).
	Visited int
}

const (
	markUnread = 1
	markRead   = 2
)

type bandEntry struct {
	id      string
	orderId string
}

// bandHeap pops the LARGEST OrderId first (ties broken by id for determinism;
// OrderIds are unique per change in production).
type bandHeap []bandEntry

func (h bandHeap) Len() int { return len(h) }
func (h bandHeap) Less(i, j int) bool {
	if h[i].orderId != h[j].orderId {
		return h[i].orderId > h[j].orderId
	}
	return h[i].id > h[j].id
}
func (h bandHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *bandHeap) Push(x any)        { *h = append(*h, x.(bandEntry)) }
func (h *bandHeap) Pop() any          { old := *h; n := len(old); e := old[n-1]; *h = old[:n-1]; return e }

// ComputeBand computes the band for one counter's frontier.
//
// Mechanics (spec §5 / hybrid design §5): a single backward traversal in
// strictly decreasing OrderId, two sticky-upgradeable colors. Frontier heads
// seed READ; local heads seed UNREAD. A READ pop colors its parents READ
// (downgrading tentative UNREADs — correct: a parent of a read change is
// causally before it, hence read). An UNREAD pop is emitted as a band
// candidate when at/below the cut, and extends the traversal to its unmarked
// parents. The loop stops when no UNREAD entries remain in the heap: every
// open branch has merged into the read region, so no further band member is
// reachable (READ-only expansion can never mint an UNREAD).
//
// Decreasing-OrderId order guarantees colors are final by pop time: if X is
// truly read, it has a witness path X = vk ≺ … ≺ v0 = H ∈ F, every vi has
// OrderId > OrderId(X) (OrderId extends ≺), so the whole path pops first and
// READ propagates parent-ward to X before X pops.
func ComputeBand(frontier []string, localHeads []string, resolve ChangeResolver) BandResult {
	var res BandResult
	marks := make(map[string]int, len(frontier)+len(localHeads))
	h := &bandHeap{}
	unreadLeft := 0

	for _, id := range frontier {
		meta, ok := resolve(id)
		if !ok {
			res.PendingHeads = append(res.PendingHeads, id)
			continue
		}
		res.ResolvedHeads = append(res.ResolvedHeads, id)
		if meta.OrderId > res.MaxFrontierOrderId {
			res.MaxFrontierOrderId = meta.OrderId
		}
		if marks[id] == 0 {
			marks[id] = markRead
			heap.Push(h, bandEntry{id: id, orderId: meta.OrderId})
		}
	}
	// No resolved head: there is no cut — every unread change is in the tail
	// (handled by the indexed range query); the band is empty by definition.
	if res.MaxFrontierOrderId == "" {
		return res
	}

	for _, id := range localHeads {
		if marks[id] != 0 {
			continue // already a frontier head (read)
		}
		meta, ok := resolve(id)
		if !ok {
			continue
		}
		marks[id] = markUnread
		unreadLeft++
		heap.Push(h, bandEntry{id: id, orderId: meta.OrderId})
	}

	for h.Len() > 0 && unreadLeft > 0 {
		e := heap.Pop(h).(bandEntry)
		res.Visited++
		meta, _ := resolve(e.id) // pushed ⇒ resolvable
		if marks[e.id] == markRead {
			for _, p := range meta.PrevIds {
				switch marks[p] {
				case markRead:
					// already known read
				case markUnread:
					// tentative unread proven covered: parent of a read change
					marks[p] = markRead
					unreadLeft--
				default:
					pm, ok := resolve(p)
					if !ok {
						continue
					}
					marks[p] = markRead
					heap.Push(h, bandEntry{id: p, orderId: pm.OrderId})
				}
			}
			continue
		}
		// unread pop
		unreadLeft--
		if e.orderId <= res.MaxFrontierOrderId {
			res.Candidates = append(res.Candidates, e.id)
		}
		for _, p := range meta.PrevIds {
			if marks[p] != 0 {
				continue // read boundary reached, or already queued
			}
			pm, ok := resolve(p)
			if !ok {
				continue
			}
			marks[p] = markUnread
			unreadLeft++
			heap.Push(h, bandEntry{id: p, orderId: pm.OrderId})
		}
	}
	return res
}
