package state

import (
	"os"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/util/slice"
)

// parentIndexDebug enables shadow verification: every index consultation also
// runs the old whole-tree scan, the SCAN answer is used (replay behavior stays
// bit-for-bit pre-index), and disagreements are logged. Meant for sweeping a
// real account: rebuild every object with this on and grep the logs.
var parentIndexDebug = os.Getenv("ANYTYPE_PARENT_INDEX_DEBUG") == "1"

// parentIndex is a childId→parentId map that lets PickParentOf and UnlinkAll
// skip whole-tree Iterate walks during change replay. Replay cost is dominated
// by those walks (one per structural op → O(n²) for a flat document), see
// BenchmarkTreeBuild.
//
// The index is ONLY safe on a single-layer (parentless) state whose structure
// mutations all flow through Add/Set/InsertTo/Unlink/UnlinkAll — i.e. the
// change-replay loop in BuildState. It must never be enabled on editing states
// (layered, discardable, full of ChildrenIds writes that bypass the helpers)
// and must be disabled before normalization runs (divBalance and
// removeDuplicates pre-mutate slices in ways no hook can observe).
//
// Correctness model: the index is a verified cache, never an authority.
//   - A hit is returned only when the mapped parent is alive and its
//     ChildrenIds actually contains the child.
//   - A child asserted under two different parents (the concurrent-move merge
//     window that normalize.removeDuplicates exists for) is flagged ambiguous
//     forever: resolution order between duplicate parents is defined by tree
//     traversal order, which a map cannot reproduce, so ambiguous ids always
//     take the Iterate fallback.
//   - Any miss falls back to the same Iterate the code used before.
type parentIndex struct {
	parents   map[string]string
	ambiguous map[string]struct{}

	// shadow-verification counters (parentIndexDebug mode)
	debugLookups    int
	debugMismatches int
	debugFallbacks  int
}

// lookupStatus distinguishes the strength of a lookup answer: a miss that
// positively claims "nothing references this id" (safe to skip a scan) versus
// a miss that only means "cannot verify, use traversal".
type lookupStatus int

const (
	lookupHit lookupStatus = iota
	// lookupNoParent: no entry and the id is not ambiguous. Because every
	// children insertion during replay asserts (or flags ambiguity), and
	// entries are only deleted when their target verifiably dropped the
	// child, this is a positive "no parent reference anywhere" claim.
	lookupNoParent
	// lookupUnverifiable: ambiguity (own or on an ancestor) or a failed chain
	// verification — the caller must fall back to tree traversal.
	lookupUnverifiable
)

// EnableParentIndex builds the index for the current state view and turns on
// the PickParentOf/UnlinkAll fast paths. No-op on layered states: their
// copy-on-write lifecycle (discard on error, cross-layer Pick aliasing) makes
// any index unsound.
func (s *State) EnableParentIndex() {
	if s.parent != nil {
		return
	}
	idx := &parentIndex{
		parents:   make(map[string]string, len(s.blocks)),
		ambiguous: make(map[string]struct{}),
	}
	_ = s.Iterate(func(b simple.Block) bool {
		pid := b.Model().Id
		for _, cid := range b.Model().ChildrenIds {
			idx.assert(cid, pid)
		}
		return true
	})
	s.parentIdx = idx
}

func (s *State) DisableParentIndex() {
	if idx := s.parentIdx; idx != nil && parentIndexDebug && idx.debugLookups > 0 {
		if idx.debugMismatches > 0 {
			log.With("objectID", s.rootId).Errorf(
				"parentIndex shadow check: %d mismatches in %d lookups (%d fallbacks)",
				idx.debugMismatches, idx.debugLookups, idx.debugFallbacks)
		} else {
			log.With("objectID", s.rootId).Debugf(
				"parentIndex shadow check: clean, %d lookups (%d fallbacks)",
				idx.debugLookups, idx.debugFallbacks)
		}
	}
	s.parentIdx = nil
}

func (idx *parentIndex) assert(childId, parentId string) {
	if existing, ok := idx.parents[childId]; ok {
		if existing != parentId {
			idx.ambiguous[childId] = struct{}{}
		}
		return
	}
	idx.parents[childId] = parentId
}

func (idx *parentIndex) remove(childId, parentId string) {
	if _, amb := idx.ambiguous[childId]; amb {
		return
	}
	if idx.parents[childId] == parentId {
		delete(idx.parents, childId)
	}
}

// assertParentIds records parentId as the parent of every id. Called by the
// children-mutation helpers with only the ADDED ids, so maintenance stays
// O(inserted), not O(children list).
func (s *State) assertParentIds(parentId string, ids ...string) {
	if s.parentIdx == nil {
		return
	}
	for _, id := range ids {
		s.parentIdx.assert(id, parentId)
	}
}

// unlinkForReplay removes id from its reachable parent's children, used by
// changeBlockCreate for the re-create-acts-as-move semantics. Unlike Unlink it
// must also handle DANGLING references (a parent listing id in ChildrenIds
// before the block itself exists — e.g. a root create carrying children):
// asserts are driven by parents' children lists, so dangling ids are indexed
// too. The scan may only be skipped on a positive lookupNoParent claim; an
// UNVERIFIABLE answer (e.g. an ambiguous ancestor in the chain) means the id
// may well be reachable and must take the full scan.
func (s *State) unlinkForReplay(id string) {
	if s.parentIdx == nil {
		s.Unlink(id)
		return
	}
	if parentIndexDebug {
		if p, ok := s.lookupParentShadow(id); ok {
			if parent := s.Get(p.Model().Id); parent != nil {
				s.removeChildren(parent.Model(), id)
			}
		}
		return
	}
	p, status := s.lookupParentFast(id)
	switch status {
	case lookupHit:
		if parent := s.Get(p.Model().Id); parent != nil {
			s.removeChildren(parent.Model(), id)
		}
	case lookupNoParent:
		// nothing references id — same no-op outcome as the scan
	default:
		s.Unlink(id)
	}
}

// maxParentChainDepth caps the ancestor-chain verification walk; it only
// exists to terminate on corrupt states with children cycles. Real documents
// are nowhere near this deep.
const maxParentChainDepth = 2048

// lookupParent resolves id's parent through the index. found=false means the
// caller must fall back to tree traversal (index disabled, no entry, ambiguous
// entry, or an entry that failed verification). In shadow-verification mode it
// additionally computes the traversal answer, logs any disagreement, and
// returns the traversal answer so replay behavior is exactly pre-index.
func (s *State) lookupParent(id string) (parent simple.Block, found bool) {
	if s.parentIdx == nil {
		return nil, false
	}
	if parentIndexDebug {
		return s.lookupParentShadow(id)
	}
	p, status := s.lookupParentFast(id)
	return p, status == lookupHit
}

// lookupParentShadow compares the fast answer against a FULL traversal that
// collects every parent containing id (no early exit), which also exposes
// unflagged duplicate-parent situations the ambiguity machinery should have
// caught. The traversal answer wins.
func (s *State) lookupParentShadow(id string) (parent simple.Block, found bool) {
	idx := s.parentIdx
	idx.debugLookups++

	fast, status := s.lookupParentFast(id)

	var all []simple.Block
	_ = s.Iterate(func(b simple.Block) bool {
		if slice.FindPos(b.Model().ChildrenIds, id) != -1 {
			all = append(all, b)
		}
		return true
	})
	var slow simple.Block
	if len(all) > 0 {
		slow = all[0]
	}

	if _, amb := idx.ambiguous[id]; !amb {
		var kind string
		switch {
		case len(all) > 1:
			kind = "duplicate parents not flagged ambiguous"
		case status == lookupUnverifiable:
			// expected fallback (e.g. ambiguous ancestor in the chain) — the
			// production code takes the traversal path here, so it costs
			// speed, never correctness
			idx.debugFallbacks++
		case status == lookupNoParent && slow != nil:
			kind = "index claims no reachable parent"
		case status == lookupHit && slow == nil:
			kind = "index returns parent for unreachable block"
		case status == lookupHit && slow != nil && fast.Model().Id != slow.Model().Id:
			kind = "index returns wrong parent"
		}
		if kind != "" {
			idx.debugMismatches++
			fastId, slowId := "<nil>", "<nil>"
			if fast != nil {
				fastId = fast.Model().Id
			}
			if slow != nil {
				slowId = slow.Model().Id
			}
			allIds := make([]string, 0, len(all))
			for _, b := range all {
				allIds = append(allIds, b.Model().Id)
			}
			log.With("objectID", s.rootId).With("changeID", s.changeId).
				Errorf("parentIndex shadow mismatch (%s): child=%s index=%s traversal=%s allParents=%v",
					kind, id, fastId, slowId, allIds)
		}
	}

	if slow == nil {
		return nil, false
	}
	return slow, true
}

// lookupParentFast is the production lookup.
//
// Verification must prove REACHABILITY, not just map-aliveness: the semantics
// being cached (Iterate) walk from the root, while a block that was unlinked
// but not CleanupBlock-ed (e.g. a move whose wire target no longer exists —
// the insert fails, the detached subtree stays alive in the map) still passes
// alive+contains checks. So a hit requires the whole ancestor chain, resolved
// through the index and verified hop by hop, to terminate at the root.
//
// A positive lookupNoParent is returned only for a non-ambiguous id with no
// entry at all; every failure along the chain is merely lookupUnverifiable.
func (s *State) lookupParentFast(id string) (parent simple.Block, status lookupStatus) {
	idx := s.parentIdx
	if idx == nil {
		return nil, lookupUnverifiable
	}
	if _, amb := idx.ambiguous[id]; amb {
		return nil, lookupUnverifiable
	}
	if _, ok := idx.parents[id]; !ok {
		return nil, lookupNoParent
	}
	rootId := s.RootId()
	var direct simple.Block
	cur := id
	for depth := 0; depth < maxParentChainDepth; depth++ {
		if _, amb := idx.ambiguous[cur]; amb {
			// an ambiguous ancestor makes the chain unverifiable, it says
			// nothing about whether id itself has a parent
			return nil, lookupUnverifiable
		}
		pid, ok := idx.parents[cur]
		if !ok {
			if cur == id {
				return nil, lookupNoParent
			}
			return nil, lookupUnverifiable
		}
		p := s.Pick(pid)
		if p == nil {
			// parent was deleted (e.g. CleanupBlock) — entry is stale
			delete(idx.parents, cur)
			return nil, lookupUnverifiable
		}
		if slice.FindPos(p.Model().ChildrenIds, cur) == -1 {
			// child was dropped without a removeChildren call (e.g. a
			// wholesale children swap via Set) — entry is stale
			delete(idx.parents, cur)
			return nil, lookupUnverifiable
		}
		if direct == nil {
			direct = p
		}
		if pid == rootId {
			return direct, lookupHit
		}
		cur = pid
	}
	return nil, lookupUnverifiable
}
