package subscription

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

// intakeDetailsLimit bounds how many full-details payloads the coalescing
// queue may pin at once. Beyond it new entries are recorded id-only and
// re-fetched in a batch when the worker drains — diff-based processing makes
// the re-fetch safe (same or newer state), and an id missing on re-fetch
// means the object was hard-deleted.
const intakeDetailsLimit = 2048

type feedItem struct {
	id      string
	details *domain.Details // nil: degraded entry, re-fetch on drain
}

// intakeQueue is the feed→worker hand-off: coalescing by id (the feed
// carries full state, so a newer snapshot safely replaces a pending one),
// FIFO by first appearance, never blocking the writer goroutine.
type intakeQueue struct {
	mu      sync.Mutex
	pending map[string]*domain.Details
	order   []string
}

func newIntakeQueue() *intakeQueue {
	return &intakeQueue{pending: make(map[string]*domain.Details)}
}

func (q *intakeQueue) add(id string, details *domain.Details) {
	q.mu.Lock()
	if _, ok := q.pending[id]; !ok {
		q.order = append(q.order, id)
		if len(q.pending) >= intakeDetailsLimit {
			details = nil
		}
	}
	q.pending[id] = details
	q.mu.Unlock()
}

func (q *intakeQueue) drain() []feedItem {
	q.mu.Lock()
	if len(q.order) == 0 {
		q.mu.Unlock()
		return nil
	}
	items := make([]feedItem, 0, len(q.order))
	for _, id := range q.order {
		items = append(items, feedItem{id: id, details: q.pending[id]})
	}
	// replace, not clear: a map that once ballooned never shrinks its buckets
	q.pending = make(map[string]*domain.Details)
	q.order = nil
	q.mu.Unlock()
	return items
}

// spaceState is the per-space hub: it owns the store-feed wiring, the intake
// queue, the worker goroutine, the space's subscriptions and the outbox.
//
// Locking: st.mu guards subs/outbox/stopped and all coreSub state. Feed
// callbacks never take it (intake has its own small mutex). No engine lock is
// ever held across store.SpaceIndex() — the index handle is resolved before
// the state is created and captured here.
type spaceState struct {
	spaceId string
	idx     spaceindex.Store
	svc     *service

	mu         sync.Mutex
	subs       []*coreSub
	groupsSubs []*groupsSub
	outbox     [][]subOp
	stopped    bool

	intake   *intakeQueue
	wakeCh   chan struct{}
	closedCh chan struct{}
	wg       sync.WaitGroup
}

func newSpaceState(svc *service, spaceId string, idx spaceindex.Store) *spaceState {
	st := &spaceState{
		spaceId:  spaceId,
		idx:      idx,
		svc:      svc,
		intake:   newIntakeQueue(),
		wakeCh:   make(chan struct{}, 1),
		closedCh: make(chan struct{}),
	}
	// wire the feed before anything can query: a subscription installed
	// later re-queries the store under st.mu, so every write is either in
	// its query result or already in the intake queue (no-data-loss
	// invariant; the store defers tx notifications to commit)
	idx.SubscribeForAll(st.onFeed)
	st.wg.Add(1)
	go st.run()
	return st
}

// onFeed runs on writer goroutines: enqueue and signal, nothing else
func (st *spaceState) onFeed(rec database.Record) {
	if rec.Details == nil {
		return
	}
	id := rec.Details.GetString(bundle.RelationKeyId)
	if id == "" {
		return
	}
	st.intake.add(id, rec.Details)
	st.notify()
}

func (st *spaceState) notify() {
	select {
	case st.wakeCh <- struct{}{}:
	default:
	}
}

func (st *spaceState) run() {
	defer st.wg.Done()
	for {
		select {
		case <-st.closedCh:
			return
		case <-st.wakeCh:
		}
		for {
			items := st.intake.drain()
			if len(items) == 0 {
				break
			}
			st.processBatch(items)
		}
		st.drainOutbox()
	}
}

func (st *spaceState) processBatch(items []feedItem) {
	st.refetchDegraded(items)

	st.mu.Lock()
	if st.stopped {
		st.mu.Unlock()
		return
	}
	var batch opBatch
	for _, it := range items {
		for _, sub := range st.subs {
			sub.checkOrderDep(it.details)
			sub.apply(it.id, it.details, &batch)
		}
		for _, g := range st.groupsSubs {
			g.checkItem(it.id, it.details)
		}
	}
	// parents first (their windows must be final before dep sets derive from
	// them), then dep scope updates, then the dep children — so dep events
	// land in the same batch payload as the parent change that caused them
	for _, sub := range st.subs {
		if !sub.isDepChild {
			sub.finalize(&batch)
		}
	}
	for _, sub := range st.subs {
		if sub.depTracker != nil && sub.depDirty {
			sub.depDirty = false
			st.applyScopeChange(sub.depTracker.child, sub.depTracker.computeDepIds(), &batch)
		}
	}
	for _, sub := range st.subs {
		if sub.isDepChild {
			sub.finalize(&batch)
		}
	}
	if len(batch.ops) > 0 {
		st.outbox = append(st.outbox, batch.ops)
	}
	dirtyGroups := st.collectDirtyGroups()
	st.mu.Unlock()

	// group recomputation queries the store; run it off the space mutex
	for _, g := range dirtyGroups {
		g.recompute()
	}
}

// refetchDegraded restores full details for id-only intake entries with one
// batched store read. Runs before taking st.mu.
func (st *spaceState) refetchDegraded(items []feedItem) {
	var missing []string
	for _, it := range items {
		if it.details == nil {
			missing = append(missing, it.id)
		}
	}
	if len(missing) == 0 {
		return
	}
	records, err := st.idx.QueryByIds(missing)
	if err != nil {
		log.Errorf("subscription space %s: re-fetch degraded intake: %v", st.spaceId, err)
		return
	}
	byId := make(map[string]*domain.Details, len(records))
	for _, rec := range records {
		if rec.Details == nil {
			continue
		}
		byId[rec.Details.GetString(bundle.RelationKeyId)] = rec.Details
	}
	for i := range items {
		if items[i].details == nil {
			// stays nil when the object vanished from the store: apply()
			// treats that as a non-match, i.e. a leave for tracking subs
			items[i].details = byId[items[i].id]
		}
	}
}

func (st *spaceState) drainOutbox() {
	st.mu.Lock()
	groups := st.outbox
	st.outbox = nil
	st.mu.Unlock()
	for _, ops := range groups {
		deliverOps(st.svc.componentCtx, st.svc.eventSender, ops)
	}
}

var errSpaceStopped = errors.New("space state stopped")

// install registers the sub and takes its snapshot atomically with respect to
// the worker (st.mu is held across the store query — the no-data-loss
// invariant, see newSpaceState). Returns the visible snapshot (the window for
// ordered subs, everything for unordered ones) plus the dep child's snapshot;
// for asyncInit the snapshot is appended to the outbox as events instead.
func (st *spaceState) install(sub *coreSub, asyncInit bool) (records, depRecords []*domain.Details, total int, err error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.stopped {
		return nil, nil, 0, errSpaceStopped
	}

	var queryResult []database.Record
	if sub.scopeSet != nil {
		queryResult, err = st.scopedQuery(sub)
	} else {
		// full scan without sort pushdown: ordered subs re-sort with their
		// own comparator anyway, so a SQL ORDER BY would only sort twice
		queryResult, err = st.idx.QueryRaw(&database.Filters{FilterObj: sub.filters.FilterObj}, 0, 0)
	}
	if err != nil {
		return nil, nil, 0, fmt.Errorf("snapshot query: %w", err)
	}

	if sub.ordered {
		records = st.installOrdered(sub, queryResult)
	} else {
		records = make([]*domain.Details, 0, len(queryResult))
		for _, rec := range queryResult {
			if rec.Details == nil {
				continue
			}
			id := rec.Details.GetString(bundle.RelationKeyId)
			if id == "" {
				continue
			}
			proj := projectDetails(rec.Details, sub.keys)
			sub.members[id] = struct{}{}
			sub.vis[id] = &visEntry{id: id, prev: proj}
			records = append(records, proj)
		}
	}
	sub.lastTotal = len(sub.members)
	st.subs = append(st.subs, sub)

	if sub.depTracker != nil {
		depRecords = st.installDepChild(sub.depTracker)
	}

	if asyncInit {
		// the snapshot flows as events through the queue; an empty snapshot
		// emits nothing, not even Counters{0}
		if len(records) > 0 {
			var batch opBatch
			for _, proj := range records {
				id := proj.GetString(bundle.RelationKeyId)
				batch.append(subOp{sub: sub, kind: opSet, id: id, details: proj})
				batch.append(subOp{sub: sub, kind: opAdd, id: id})
			}
			batch.append(subOp{sub: sub, kind: opCounters, total: int64(len(sub.members))})
			st.outbox = append(st.outbox, batch.ops)
		}
		records = nil
	}
	return records, depRecords, len(sub.members), nil
}

// installDepChild seeds and registers the hidden "{subId}/dep" sub from the
// freshly installed parent. A dep snapshot failure degrades to an empty dep
// set (entries arrive via the feed) instead of failing the subscription.
// Runs under the space mutex.
func (st *spaceState) installDepChild(tracker *depTracker) (depRecords []*domain.Details) {
	child := tracker.child
	child.space = st
	child.setScopeIds(tracker.computeDepIds())
	queryResult, err := st.scopedQuery(child)
	if err != nil {
		log.Errorf("subscription %s: dep snapshot: %v", child.subId, err)
		queryResult = nil
	}
	depRecords = make([]*domain.Details, 0, len(queryResult))
	for _, rec := range queryResult {
		id := rec.Details.GetString(bundle.RelationKeyId)
		if id == "" {
			continue
		}
		proj := projectDetails(rec.Details, child.keys)
		child.members[id] = struct{}{}
		child.vis[id] = &visEntry{id: id, prev: proj}
		depRecords = append(depRecords, proj)
	}
	child.lastTotal = len(child.members)
	st.subs = append(st.subs, child)
	return depRecords
}

// installGroups registers a groups adapter and seeds its member relevance
// set (id → grouped value) so checkItem can detect leaves and value changes
func (st *spaceState) installGroups(g *groupsSub) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.stopped {
		return errSpaceStopped
	}
	matchFilters := &database.Filters{FilterObj: g.match}
	records, err := st.idx.QueryRaw(matchFilters, 0, 0)
	if err != nil {
		return fmt.Errorf("groups member query: %w", err)
	}
	g.members = make(map[string]domain.Value, len(records))
	for _, rec := range records {
		if rec.Details == nil {
			continue
		}
		id := rec.Details.GetString(bundle.RelationKeyId)
		if id == "" {
			continue
		}
		g.members[id] = rec.Details.Get(g.relationKey)
	}
	st.groupsSubs = append(st.groupsSubs, g)
	return nil
}

func (st *spaceState) removeGroupsSub(g *groupsSub) (empty bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for i, cur := range st.groupsSubs {
		if cur == g {
			st.groupsSubs = append(st.groupsSubs[:i], st.groupsSubs[i+1:]...)
			break
		}
	}
	return len(st.subs) == 0 && len(st.groupsSubs) == 0
}

func (st *spaceState) collectDirtyGroups() []*groupsSub {
	var res []*groupsSub
	for _, g := range st.groupsSubs {
		if g.dirty {
			g.dirty = false
			res = append(res, g)
		}
	}
	return res
}

// installOrdered seeds the full member set and the ordered window from the
// snapshot, sorted with cmpEntries — the authoritative comparator, with the
// id tiebreak — UNCONDITIONALLY: the snapshot arrives in store scan order
// (index-grouped, not id-ordered), and the live binary-search bookkeeping is
// only correct over a window the engine's own comparator produced. This
// includes no-sort client subs, where cmpEntries degrades to the id compare.
func (st *spaceState) installOrdered(sub *coreSub, queryResult []database.Record) (records []*domain.Details) {
	type pair struct {
		entry   *visEntry
		details *domain.Details
	}
	pairs := make([]pair, 0, len(queryResult))
	for _, rec := range queryResult {
		if rec.Details == nil {
			continue
		}
		id := rec.Details.GetString(bundle.RelationKeyId)
		if id == "" {
			continue
		}
		sub.members[id] = struct{}{}
		pairs = append(pairs, pair{entry: sub.newVisEntry(id, rec.Details), details: rec.Details})
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		return sub.cmpEntries(pairs[i].entry, pairs[j].entry) < 0
	})
	start := sub.offset
	if start > len(pairs) {
		start = len(pairs)
	}
	end := len(pairs)
	if sub.limit > 0 && start+sub.limit < end {
		end = start + sub.limit
	}
	window := pairs[start:end]

	sub.win = make([]*visEntry, 0, len(window))
	records = make([]*domain.Details, 0, len(window))
	for _, p := range window {
		p.entry.prev = projectDetails(p.details, sub.keys)
		sub.win = append(sub.win, p.entry)
		sub.vis[p.entry.id] = p.entry
		records = append(records, p.entry.prev)
	}
	return records
}

// scopedQuery fetches a scope-gated sub's candidates by id, in scope order,
// keeping only records that pass the sub's filters (ids subscriptions have
// none). Bounded by the scope size.
func (st *spaceState) scopedQuery(sub *coreSub) ([]database.Record, error) {
	fetched, err := st.idx.QueryByIds(sub.scope)
	if err != nil {
		return nil, fmt.Errorf("query scope ids: %w", err)
	}
	byId := make(map[string]database.Record, len(fetched))
	for _, rec := range fetched {
		if rec.Details == nil {
			continue
		}
		byId[rec.Details.GetString(bundle.RelationKeyId)] = rec
	}
	records := make([]database.Record, 0, len(fetched))
	for _, id := range sub.scope {
		rec, ok := byId[id]
		if !ok {
			continue
		}
		if !sub.matches(id, rec.Details) {
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// setScope replaces a sub's id scope (collection membership stream) and
// turns the difference into enter/leave transitions through the regular
// apply path
func (st *spaceState) setScope(sub *coreSub, ids []string) {
	st.mu.Lock()
	defer func() {
		st.mu.Unlock()
		st.notify()
	}()
	if st.stopped || !st.hasSub(sub) {
		return
	}
	var batch opBatch
	st.applyScopeChange(sub, ids, &batch)
	sub.finalize(&batch)
	if len(batch.ops) > 0 {
		st.outbox = append(st.outbox, batch.ops)
	}
}

// applyScopeChange diffs the sub's scope against the new id list and feeds
// the difference through the regular apply path; entering objects' details
// come from one batched store read. Runs under the space mutex; the caller
// finalizes the sub.
func (st *spaceState) applyScopeChange(sub *coreSub, ids []string, batch *opBatch) {
	newSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		newSet[id] = struct{}{}
	}
	var added []string
	for _, id := range ids {
		if _, ok := sub.scopeSet[id]; !ok {
			added = append(added, id)
		}
	}
	var removed []string
	for _, id := range sub.scope {
		if _, ok := newSet[id]; !ok {
			removed = append(removed, id)
		}
	}
	sub.setScopeIds(ids)
	if len(added) == 0 && len(removed) == 0 {
		return
	}

	var addedRecords []database.Record
	if len(added) > 0 {
		var err error
		addedRecords, err = st.idx.QueryByIds(added)
		if err != nil {
			log.Errorf("subscription %s: fetch scope additions: %v", sub.subId, err)
		}
	}

	for _, id := range removed {
		// out of scope now: a nil-details apply is a non-match, i.e. a leave
		sub.apply(id, nil, batch)
	}
	for _, rec := range addedRecords {
		if rec.Details == nil {
			continue
		}
		id := rec.Details.GetString(bundle.RelationKeyId)
		if id == "" {
			continue
		}
		sub.apply(id, rec.Details, batch)
	}
}

func (st *spaceState) hasSub(sub *coreSub) bool {
	for _, s := range st.subs {
		if s == sub {
			return true
		}
	}
	return false
}

// removeSub detaches the sub from the space; returns whether the space is
// left without subscriptions
func (st *spaceState) removeSub(sub *coreSub) (empty bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.removeSubLocked(sub)
	if sub.depTracker != nil {
		st.removeSubLocked(sub.depTracker.child)
	}
	return len(st.subs) == 0 && len(st.groupsSubs) == 0
}

func (st *spaceState) removeSubLocked(sub *coreSub) {
	for i, s := range st.subs {
		if s == sub {
			st.subs = append(st.subs[:i], st.subs[i+1:]...)
			return
		}
	}
}

// markStopped flags the state as defunct if it has no subscriptions; a
// concurrent install observing the flag retries with a fresh state
func (st *spaceState) markStopped() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.subs) > 0 || len(st.groupsSubs) > 0 || st.stopped {
		return false
	}
	st.stopped = true
	return true
}

// shutdown unhooks the feed and stops the worker. Must not be called with
// st.mu held. Safe only when no concurrent Search can create a fresh state
// for the same space (service Close, after the closed flag is set); the
// last-sub teardown path must unhook under the registry lock instead — see
// maybeDropSpace.
func (st *spaceState) shutdown() {
	st.idx.SubscribeForAll(nil)
	st.stopWorker()
}

func (st *spaceState) stopWorker() {
	close(st.closedCh)
	st.wg.Wait()
}
