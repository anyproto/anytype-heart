package sourceimpl

// readPair is a change's local read coordinates. OrderId is a lexid string
// (lexicographic order == topological order); AddSeq is the monotonic insert
// sequence (never reassigned).
type readPair struct {
	OrderId string
	AddSeq  uint64
}

// dominated reports whether X is "read": some seen head H was at-or-after X in
// the timeline (OrderId) AND already existed when the user read H (AddSeq).
// Per-head (∃H), never per-axis max — see the multi-head test.
func dominated(x readPair, frontier []readPair) bool {
	for _, h := range frontier {
		if x.OrderId <= h.OrderId && x.AddSeq <= h.AddSeq {
			return true
		}
	}
	return false
}

// watermark is the per-counter read engine. It holds the cumulative resolved
// seen frontier (id -> pair, pruned to maximal so the synced value stays
// bounded) and fires onRemove with every currently-dominated change id
// (downstream SetReadFlag is idempotent, so re-firing already-read ids is
// harmless — correctness does not depend on computing a minimal delta).
type watermark struct {
	onRemove func([]string)
	seen     map[string]readPair // cumulative resolved seen head id -> pair
	pending  map[string]struct{} // seen ids not yet resolvable
}

func newWatermark(onRemove func([]string)) *watermark {
	return &watermark{onRemove: onRemove, seen: map[string]readPair{}, pending: map[string]struct{}{}}
}

// advance accumulates newly seen ids (deferring unresolved ones), prunes the
// frontier to maximal pairs, then fires onRemove for all dominated changes.
func (w *watermark) advance(seenIds []string, resolve func(string) (readPair, bool), eachChange func(yield func(string, readPair))) {
	for id := range w.pending {
		seenIds = append(seenIds, id)
	}
	for _, id := range seenIds {
		if _, done := w.seen[id]; done {
			continue
		}
		if p, ok := resolve(id); ok {
			w.seen[id] = p
			delete(w.pending, id)
		} else {
			w.pending[id] = struct{}{}
		}
	}
	w.prune()
	if len(w.seen) == 0 {
		return
	}
	frontier := make([]readPair, 0, len(w.seen))
	for _, p := range w.seen {
		frontier = append(frontier, p)
	}
	var read []string
	eachChange(func(id string, p readPair) {
		if dominated(p, frontier) {
			read = append(read, id)
		}
	})
	if len(read) > 0 {
		w.onRemove(read)
	}
}

// prune drops seen ids whose pair is dominated by another seen id's pair
// (equal pairs: keep the lexicographically-greatest id). Removing a dominated
// pair never changes the dominance result — the dominating pair remains — so
// this only bounds the synced seenHeads size; no change graph needed.
func (w *watermark) prune() {
	for aID, aP := range w.seen {
		for bID, bP := range w.seen {
			if aID == bID {
				continue
			}
			if aP.OrderId <= bP.OrderId && aP.AddSeq <= bP.AddSeq && (aP != bP || bID > aID) {
				delete(w.seen, aID)
				break
			}
		}
	}
}

// seenHeadIds returns the cumulative, pruned seen head ids (persisted as the
// synced KeyValueService value, replacing the old DiffManager.SeenHeads()).
func (w *watermark) seenHeadIds() []string {
	out := make([]string, 0, len(w.seen))
	for id := range w.seen {
		out = append(out, id)
	}
	return out
}

// frontierLen reports the resolved seen-frontier size (debug / ProvideStat).
func (w *watermark) frontierLen() int { return len(w.seen) }
