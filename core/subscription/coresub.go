package subscription

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/anyproto/any-store/query"
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

// coreSub is the engine's single subscription primitive: an id scope (later
// phases), compiled filters, an optional order+window and the requested keys,
// maintained as a member set over one space's feed.
//
// Memory model: the full matching set costs an id-set entry per member;
// projections (and, for ordered subs, sort values) are kept only for
// *visible* members — the window for ordered subs, everything for unordered
// ones. counters.total is the member count, never derived from details.
//
// All state is private to the sub and mutated only under its space's mutex.
// Every *domain.Details held here is frozen: state updates replace pointers,
// never mutate maps, because member projections double as event payloads.
type coreSub struct {
	subId   string
	spaceId string
	keys    []domain.RelationKey
	filters *database.Filters

	// ordered mode: server-side ordering with a maintained window.
	// order may be nil even when ordered (client request without sorts):
	// the comparator degrades to the bare id tiebreak and entries carry no
	// sort values.
	ordered  bool
	order    database.Order
	sortKeys []domain.RelationKey
	limit    int // 0 = unbounded window
	offset   int

	// scope is an optional id whitelist gating membership: fixed for ids
	// subscriptions (in request order), mutable for collection-backed subs.
	// scopeRequestOrder makes Add events use the nearest preceding present
	// scope id as afterId (ids subscriptions' request-order semantics).
	scope             []string
	scopeSet          map[string]struct{}
	scopeIdx          map[string]int
	scopeRequestOrder bool
	collection        *collectionWatcher

	// members is the full matching set (ids only); vis holds the visible
	// members' state; win additionally orders them for ordered subs
	members map[string]struct{}
	vis     map[string]*visEntry
	win     []*visEntry

	lastTotal int

	// batch-scoped state (ordered subs): the window snapshot the diff script
	// is computed against, detail ops accumulated during apply, and the
	// window-rebuild flag for the cases incremental bookkeeping cannot cover
	batchActive  bool
	oldWin       []string
	detailOps    []subOp
	needsRequery bool

	// detailEventsOnly suppresses Add/Remove/Position/Counters (dep subs);
	// noCounters suppresses only Counters (ids subscriptions have no
	// counters semantics)
	detailEventsOnly bool
	noCounters       bool

	// render dependencies: the tracker owns the hidden child sub under
	// "{subId}/dep"; depDirty marks that the dep id set may have changed
	// this batch; isDepChild marks the child itself (finalized after its
	// parent so dep scope changes land in the same batch)
	depTracker *depTracker
	depDirty   bool
	isDepChild bool
	// orderDepBuf is a reusable argument buffer for per-item UpdateOrderMap
	// probes (checkOrderDep), avoiding a slice allocation per feed item
	orderDepBuf [1]*domain.Details

	// queue is the delivery target for internal subs; nil means broadcast.
	// queueOwned distinguishes engine-created queues (closed on teardown)
	// from caller-provided ones (never touched, e.g. crossspacesub's shared
	// queue)
	queue      *mb.MB[*pb.EventMessage]
	queueOwned bool

	space *spaceState
}

type visEntry struct {
	id       string
	prev     *domain.Details // projection to the requested keys
	sortVals *domain.Details // sort keys only; nil when order == nil
}

// setWindow installs seeded, sorted window entries (prev projections already
// set) and returns their projections in window order
func (c *coreSub) setWindow(entries []*visEntry) (records []*domain.Details) {
	c.win = entries
	c.vis = make(map[string]*visEntry, len(entries))
	records = make([]*domain.Details, 0, len(entries))
	for _, e := range entries {
		c.vis[e.id] = e
		records = append(records, e.prev)
	}
	return records
}

// setScopeIds replaces the scope index structures, deduplicating on first
// occurrence — collection lists can carry duplicates, and a duplicate id
// would seed twin window entries that break the comparator's total-order
// invariant. Called before install or under the space mutex (setScope).
func (c *coreSub) setScopeIds(ids []string) {
	scope := make([]string, 0, len(ids))
	c.scopeSet = make(map[string]struct{}, len(ids))
	c.scopeIdx = make(map[string]int, len(ids))
	for _, id := range ids {
		if _, ok := c.scopeSet[id]; ok {
			continue
		}
		c.scopeIdx[id] = len(scope)
		c.scopeSet[id] = struct{}{}
		scope = append(scope, id)
	}
	c.scope = scope
}

func (c *coreSub) newVisEntry(id string, details *domain.Details) *visEntry {
	e := &visEntry{id: id}
	if c.order != nil {
		e.sortVals = projectDetails(details, c.sortKeys)
	}
	return e
}

// cmpEntries is the authoritative comparator: the compiled order first, the
// id as a total-order tiebreak. Snapshot and re-query results are re-sorted
// with it so binary searches and store queries can never disagree.
func (c *coreSub) cmpEntries(a, b *visEntry) int {
	if c.order != nil {
		if v := c.order.Compare(a.sortVals, b.sortVals); v != 0 {
			return v
		}
	}
	return strings.Compare(a.id, b.id)
}

// matches is the membership predicate: scope whitelist (when present) and
// compiled filters (when present — ids subscriptions have none: explicit ids
// are tracked even when archived or deleted)
func (c *coreSub) matches(id string, details *domain.Details) bool {
	if c.scopeSet != nil {
		if _, ok := c.scopeSet[id]; !ok {
			return false
		}
	}
	if c.filters != nil && c.filters.FilterObj != nil {
		return c.filters.FilterObj.FilterObject(details)
	}
	return true
}

// scopeAfterId returns the nearest preceding scope id that is currently
// visible — the request-order afterId for ids subscriptions
func (c *coreSub) scopeAfterId(id string) string {
	idx, ok := c.scopeIdx[id]
	if !ok {
		return ""
	}
	for i := idx - 1; i >= 0; i-- {
		if _, present := c.vis[c.scope[i]]; present {
			return c.scope[i]
		}
	}
	return ""
}

// apply evaluates one feed update against the sub and appends the resulting
// transition ops. details == nil means the object is gone from the store
// (hard delete observed via re-fetch miss) and is treated as a non-match.
// Runs under the space mutex.
func (c *coreSub) apply(id string, details *domain.Details, out *opBatch) {
	matched := details != nil && c.matches(id, details)
	_, isMember := c.members[id]
	if c.ordered {
		c.applyOrdered(id, details, matched, isMember)
		return
	}
	switch {
	case matched && !isMember:
		proj := projectDetails(details, c.keys)
		c.members[id] = struct{}{}
		c.vis[id] = &visEntry{id: id, prev: proj}
		c.depDirty = c.depTracker != nil
		out.append(subOp{sub: c, kind: opSet, id: id, details: proj})
		if !c.detailEventsOnly {
			var afterId string
			if c.scopeRequestOrder {
				afterId = c.scopeAfterId(id)
			}
			out.append(subOp{sub: c, kind: opAdd, id: id, afterId: afterId})
		}
	case !matched && isMember:
		delete(c.members, id)
		delete(c.vis, id)
		c.depDirty = c.depTracker != nil
		if !c.detailEventsOnly {
			out.append(subOp{sub: c, kind: opRemove, id: id})
		}
	case matched && isMember:
		e := c.vis[id]
		next, amend, unset := diffProject(e.prev, details, c.keys)
		if next == nil {
			return
		}
		e.prev = next
		if c.depTracker != nil && c.depTracker.amendTouchesDepKeys(amend, unset) {
			c.depDirty = true
		}
		if len(amend) > 0 {
			out.append(subOp{sub: c, kind: opAmend, id: id, amend: amend})
		}
		if len(unset) > 0 {
			out.append(subOp{sub: c, kind: opUnset, id: id, unset: unset})
		}
	}
}

// checkOrderDep offers a feed item to the compiled order's dependency maps
// (object/file sorts depend on target names, tag/status sorts on option
// objects; the order map knows exactly which ids it depends on). A reported
// change means the comparator shifted under the window: rebuild it via the
// re-query, which re-sorts with the updated map and emits the resulting
// Position events. Covers the contract's "rename the assignee you sort by".
func (c *coreSub) checkOrderDep(details *domain.Details) {
	if c.order == nil || details == nil {
		return
	}
	c.orderDepBuf[0] = details
	if c.order.UpdateOrderMap(c.orderDepBuf[:]) {
		c.beginBatch()
		c.needsRequery = true
	}
}

// applyOrdered updates membership and window bookkeeping; events are
// produced by the window-diff script in finalize. Incremental window
// maintenance covers offset == 0 (the hot path, including limit 0); every
// other mutation falls back to a window re-query.
func (c *coreSub) applyOrdered(id string, details *domain.Details, matched, isMember bool) {
	switch {
	case matched && !isMember:
		c.beginBatch()
		c.members[id] = struct{}{}
		c.orderedEnter(id, details)
	case !matched && isMember:
		c.beginBatch()
		delete(c.members, id)
		c.orderedLeave(id)
	case matched && isMember:
		c.orderedStay(id, details)
	}
}

func (c *coreSub) beginBatch() {
	if c.batchActive {
		return
	}
	c.batchActive = true
	c.oldWin = make([]string, len(c.win))
	for i, e := range c.win {
		c.oldWin[i] = e.id
	}
}

// windowFull reports whether the window holds limit entries
func (c *coreSub) windowFull() bool {
	return c.limit > 0 && len(c.win) >= c.limit
}

func (c *coreSub) orderedEnter(id string, details *domain.Details) {
	if c.offset > 0 {
		c.needsRequery = true
		return
	}
	e := c.newVisEntry(id, details)
	pos := c.searchWin(e)
	if c.windowFull() && pos >= c.limit {
		return // ranks beyond the window: membership/total bookkeeping only
	}
	e.prev = projectDetails(details, c.keys)
	c.insertWin(pos, e)
	c.evictOverflow()
}

func (c *coreSub) orderedLeave(id string) {
	if c.offset > 0 {
		c.needsRequery = true
		return
	}
	e := c.vis[id]
	if e == nil {
		return // outside the window: total changes, window doesn't
	}
	c.removeWin(e)
	// with members remaining beyond the window a successor must slide in,
	// and only the store knows which one
	if len(c.members) > len(c.win) {
		c.needsRequery = true
	}
}

func (c *coreSub) orderedStay(id string, details *domain.Details) {
	e := c.vis[id]
	if e == nil {
		if c.offset > 0 {
			c.beginBatch()
			c.needsRequery = true
			return
		}
		// for offset==0 an underfull window normally holds every member, so
		// an invisible matching member here means the invariant was broken
		// cross-batch (a re-query read newer store state and dropped the
		// member's vis entry while a coalesced re-match was still queued).
		// Without repair the member would be absorbed out of the window
		// forever — treat it as an enter.
		if !c.windowFull() {
			c.beginBatch()
			e := c.newVisEntry(id, details)
			e.prev = projectDetails(details, c.keys)
			c.insertWin(c.searchWin(e), e)
			return
		}
		// a sort-relevant change may move an outside member into the window
		probe := c.newVisEntry(id, details)
		if c.cmpEntries(probe, c.win[len(c.win)-1]) >= 0 {
			return
		}
		c.beginBatch()
		probe.prev = projectDetails(details, c.keys)
		c.insertWin(c.searchWin(probe), probe)
		c.evictOverflow()
		return
	}

	if c.order != nil {
		newSort := projectDetails(details, c.sortKeys)
		if !newSort.Equal(e.sortVals) {
			c.beginBatch()
			if c.offset > 0 {
				// offset windows rebuild via re-query for EVERY order
				// mutation, the visible-member reposition included: a member
				// moving above the offset boundary stays in the local window
				// while the true occupant (rank offset-1) is unknown
				c.needsRequery = true
			} else {
				c.removeWin(e)
				e.sortVals = newSort
				pos := c.searchWin(e)
				c.insertWin(pos, e)
				// landing last with members beyond the window leaves its
				// true rank relative to them unknown
				if pos == len(c.win)-1 && len(c.members) > len(c.win) {
					c.needsRequery = true
				}
			}
		}
	}

	next, amend, unset := diffProject(e.prev, details, c.keys)
	if next == nil {
		return
	}
	e.prev = next
	if c.depTracker != nil && c.depTracker.amendTouchesDepKeys(amend, unset) {
		c.depDirty = true
	}
	if len(amend) > 0 {
		c.detailOps = append(c.detailOps, subOp{sub: c, kind: opAmend, id: id, amend: amend})
	}
	if len(unset) > 0 {
		c.detailOps = append(c.detailOps, subOp{sub: c, kind: opUnset, id: id, unset: unset})
	}
}

// searchWin returns the insertion position for e
func (c *coreSub) searchWin(e *visEntry) int {
	return sort.Search(len(c.win), func(i int) bool {
		return c.cmpEntries(c.win[i], e) > 0
	})
}

func (c *coreSub) insertWin(pos int, e *visEntry) {
	c.win = append(c.win, nil)
	copy(c.win[pos+1:], c.win[pos:])
	c.win[pos] = e
	c.vis[e.id] = e
}

func (c *coreSub) removeWin(e *visEntry) {
	// the entry's stored sort values still locate it; scan the equal range
	// for the exact id
	pos := sort.Search(len(c.win), func(i int) bool {
		return c.cmpEntries(c.win[i], e) >= 0
	})
	for pos < len(c.win) && c.win[pos] != e {
		pos++
	}
	if pos == len(c.win) {
		return
	}
	c.win = append(c.win[:pos], c.win[pos+1:]...)
	delete(c.vis, e.id)
}

func (c *coreSub) evictOverflow() {
	for c.limit > 0 && len(c.win) > c.limit {
		last := c.win[len(c.win)-1]
		c.win = c.win[:len(c.win)-1]
		delete(c.vis, last.id)
	}
}

// requeryWindow rebuilds the window from the store: the fallback for offset
// windows and for underflows where the successor is unknown. Runs under the
// space mutex; reading newer store state than the intake queue has delivered
// is safe because batch processing is diff-based and idempotent.
func (c *coreSub) requeryWindow() (ok bool) {
	var (
		records []database.Record
		err     error
	)
	if c.scopeSet != nil {
		// scoped sets are bounded by the scope; fetch by ids and filter
		records, err = c.space.scopedQuery(c)
	} else {
		fetch := 0 // everything
		if c.limit > 0 {
			fetch = c.offset + c.limit + requeryMargin
		}
		// The SQL LIMIT cut must approximate cmpEntries' order. For no-sort
		// subs the comparator IS the id order, so push the id sort — exact.
		// For sorted subs push the compiled order: ties are truncated in
		// store order, the margin absorbs small tie groups, and the
		// in-memory re-sort below fixes the order within the fetch. (A
		// compound order+id pushdown would be exact, but anystore's planner
		// silently drops mixed custom+field sorts — probed, not supported.)
		fetchOrder := c.filters.Order
		if c.order == nil {
			fetchOrder = idTiebreakOrder{}
		}
		records, err = c.space.idx.QueryRaw(&database.Filters{
			FilterObj: c.filters.FilterObj,
			Order:     fetchOrder,
		}, fetch, 0)
	}
	if err != nil {
		log.Errorf("subscription %s: window re-query: %v", c.subId, err)
		return false
	}
	entries := make([]*visEntry, 0, len(records))
	var readmitted []string
	for _, rec := range records {
		if rec.Details == nil {
			continue
		}
		id := rec.Details.GetString(bundle.RelationKeyId)
		if id == "" {
			continue
		}
		// the re-query can race ahead of the intake queue: admit unseen
		// members now, their pending feed updates diff to no-ops
		if _, ok := c.members[id]; !ok {
			c.members[id] = struct{}{}
		}
		e := c.newVisEntry(id, rec.Details)
		if old := c.vis[id]; old != nil {
			e.prev = old.prev // preserve diff baseline for stayed members
		} else {
			e.prev = projectDetails(rec.Details, c.keys)
			// an id that was visible at batch start but lost its vis entry
			// mid-batch (left, then re-matched in newer store state read by
			// this re-query) re-enters with a baseline the client never saw:
			// the window diff will find it in both old and new windows and
			// emit nothing, and later feed updates diff against the already
			// fresh baseline — only a forced Set resyncs the client.
			// (oldWin is still set here: finalize clears it after the diff.)
			if slices.Contains(c.oldWin, id) {
				readmitted = append(readmitted, id)
			}
		}
		entries = append(entries, e)
	}
	// unconditional: cmpEntries (with its id tiebreak) is the authoritative
	// order even when the sub has no sorts — see installOrdered
	sort.SliceStable(entries, func(i, j int) bool { return c.cmpEntries(entries[i], entries[j]) < 0 })
	if c.offset > 0 {
		if c.offset >= len(entries) {
			entries = nil
		} else {
			entries = entries[c.offset:]
		}
	}
	if c.limit > 0 && len(entries) > c.limit {
		entries = entries[:c.limit]
	}
	c.win = entries
	c.vis = make(map[string]*visEntry, len(entries))
	for _, e := range entries {
		c.vis[e.id] = e
	}
	for _, id := range readmitted {
		if e := c.vis[id]; e != nil {
			c.detailOps = append(c.detailOps, subOp{sub: c, kind: opSet, id: id, details: e.prev})
		}
	}
	if len(readmitted) > 0 && c.depTracker != nil {
		// a re-admitted baseline may carry different dep-key values than
		// the dep set last saw, and no amend will ever report them
		c.depDirty = true
	}
	return true
}

const requeryMargin = 8

// idTiebreakOrder pushes the bare id sort down to the store — the exact SQL
// counterpart of cmpEntries for no-sort subs. inner is reserved for a future
// compound pushdown; anystore currently drops mixed custom+field sorts, so
// it stays nil in practice.
type idTiebreakOrder struct {
	inner database.Order
}

var anystoreIdSort = func() query.Sort {
	s, err := query.ParseSort("id")
	if err != nil {
		panic(fmt.Errorf("parse id sort: %w", err))
	}
	return s
}()

func (t idTiebreakOrder) Compare(a, b *domain.Details) int {
	if t.inner != nil {
		return t.inner.Compare(a, b)
	}
	return 0
}

func (t idTiebreakOrder) UpdateOrderMap(depDetails []*domain.Details) bool {
	if t.inner != nil {
		return t.inner.UpdateOrderMap(depDetails)
	}
	return false
}

func (t idTiebreakOrder) AnystoreSort() query.Sort {
	if t.inner == nil {
		return anystoreIdSort
	}
	inner := t.inner.AnystoreSort()
	if inner == nil {
		return anystoreIdSort
	}
	return query.Sorts{inner, anystoreIdSort}
}

// finalize turns the batch's accumulated state into events: the window-diff
// script for ordered subs, pending detail ops, and a trailing Counters when
// the total changed
func (c *coreSub) finalize(out *opBatch) {
	if c.ordered && c.batchActive {
		requeryOk := true
		if c.needsRequery {
			requeryOk = c.requeryWindow()
			if requeryOk {
				c.needsRequery = false
			}
		}
		if requeryOk {
			c.batchActive = false
			if !c.detailEventsOnly {
				opsBefore := len(out.ops)
				c.windowDiffOps(out)
				if len(out.ops) > opsBefore && c.depTracker != nil {
					// window membership changed: dep set derives from it
					c.depDirty = true
				}
			}
			c.oldWin = nil
		}
		// on a failed re-query keep batchActive/oldWin/needsRequery: the
		// next batch retries with the client's actual list still being the
		// diff baseline; offset windows rely entirely on the re-query
	}
	if len(c.detailOps) > 0 {
		out.ops = append(out.ops, c.detailOps...)
		c.detailOps = nil
	}
	if total := len(c.members); total != c.lastTotal {
		c.lastTotal = total
		if !c.detailEventsOnly && !c.noCounters {
			out.append(subOp{sub: c, kind: opCounters, total: int64(total)})
		}
	}
}

// windowDiffOps emits a script that replays the old window into the new one
// on a client applying membership events in order: removals first, then a
// left-to-right walk emitting Add (preceded by its DetailsSet) for new ids
// and Position for misplaced ones. Simulating the client list guarantees
// every emitted afterId already sits at its final position — an afterId
// absent from the client list would corrupt it.
func (c *coreSub) windowDiffOps(out *opBatch) {
	newPos := make(map[string]int, len(c.win))
	for i, e := range c.win {
		newPos[e.id] = i
	}
	sim := make([]string, 0, len(c.win)+len(c.oldWin))
	for _, id := range c.oldWin {
		if _, ok := newPos[id]; ok {
			sim = append(sim, id)
		} else {
			out.append(subOp{sub: c, kind: opRemove, id: id})
		}
	}
	prev := ""
	for i, e := range c.win {
		pos := -1
		for j := i; j < len(sim); j++ { // sim[:i] == window[:i] by construction
			if sim[j] == e.id {
				pos = j
				break
			}
		}
		switch {
		case pos == -1:
			out.append(subOp{sub: c, kind: opSet, id: e.id, details: e.prev})
			out.append(subOp{sub: c, kind: opAdd, id: e.id, afterId: prev})
			sim = append(sim, "")
			copy(sim[i+1:], sim[i:])
			sim[i] = e.id
		case pos != i:
			out.append(subOp{sub: c, kind: opPosition, id: e.id, afterId: prev})
			copy(sim[i+1:pos+1], sim[i:pos])
			sim[i] = e.id
		}
		prev = e.id
	}
}

// memberIds snapshots the current member id set, sorted: consumers
// synthesize Remove events from it (crossspacesub space removal), and a
// deterministic order beats leaking map iteration randomness. Runs under
// the space mutex.
func (c *coreSub) memberIds() []string {
	ids := make([]string, 0, len(c.members))
	for id := range c.members {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// projectDetails copies only the requested keys out of full details. Sized
// to the key count: a projection is retained per visible member per sub, so
// default-sized maps would waste empty buckets at full-space scale.
func projectDetails(details *domain.Details, keys []domain.RelationKey) *domain.Details {
	proj := domain.NewDetailsWithSize(len(keys))
	for _, k := range keys {
		if v, ok := details.TryGet(k); ok {
			proj.Set(k, v)
		}
	}
	return proj
}

// diffProject compares the previous projection against fresh full details in
// a single pass over the requested keys. It returns the new projection plus
// the changed and removed keys, or (nil, nil, nil) when nothing changed —
// allocating only on the first detected difference, which keeps the no-op
// path (snapshot/feed overlap, storms of irrelevant changes) allocation-free.
func diffProject(prev, details *domain.Details, keys []domain.RelationKey) (next *domain.Details, amend []amendKV, unset []string) {
	for _, k := range keys {
		nv, nok := details.TryGet(k)
		pv, pok := prev.TryGet(k)
		if nok && (!pok || !nv.Equal(pv)) {
			amend = append(amend, amendKV{key: string(k), value: nv})
		} else if !nok && pok {
			unset = append(unset, string(k))
		}
	}
	if amend == nil && unset == nil {
		return nil, nil, nil
	}
	next = prev.Copy()
	for _, kv := range amend {
		next.Set(domain.RelationKey(kv.key), kv.value)
	}
	for _, k := range unset {
		next.Delete(domain.RelationKey(k))
	}
	return next, amend, unset
}
