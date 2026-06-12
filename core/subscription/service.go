package subscription

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/globalsign/mgo/bson"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The subscription engine: a live-query service over object details. The
// behavioral contract is docs/Subscriptions.md; the architecture is
// docs/SubscriptionsEngineDesign.md — one core primitive (scope + filters +
// order/window over a per-space store feed) surrounded by adapters.

const CName = "subscription"

var log = logging.Logger("anytype-mw-subscription")

func New() Service {
	return &service{}
}

type SubscribeRequest struct {
	SpaceId string
	SubId   string
	Filters []database.FilterRequest
	Sorts   []database.SortRequest
	Limit   int64
	Offset  int64
	// (required) necessary keys in details for return, for object fields mw will return (and subscribe) objects as dependent
	Keys []string
	// (optional) pagination: middleware will return results after given id
	AfterId string
	// (optional) pagination: middleware will return results before given id
	BeforeId string
	Source   []string
	// disable dependent subscription
	NoDepSubscription bool
	CollectionId      string

	// Internal indicates that subscription will send events into message queue instead of global client's event system
	Internal bool
	// InternalQueue is used when Internal flag is set to true. If it's nil, new queue will be created.
	// Caller-provided queues MUST be unbounded (mb.New(0)): delivery happens on the space worker,
	// and a full bounded queue would stall every subscription of that space.
	InternalQueue *mb.MB[*pb.EventMessage]
	AsyncInit     bool
}

type SubscribeResponse struct {
	SubId        string
	Records      []*domain.Details
	Dependencies []*domain.Details
	Counters     *pb.EventObjectSubscriptionCounters

	// Used when Internal flag is set to true
	Output *mb.MB[*pb.EventMessage]
}

type SubscribeGroupsRequest struct {
	SpaceId      string
	SubId        string
	RelationKey  string
	Filters      []database.FilterRequest
	Source       []string
	CollectionId string
}

type Service interface {
	Search(req SubscribeRequest) (resp *SubscribeResponse, err error)
	SubscribeIdsReq(req pb.RpcObjectSubscribeIdsRequest) (resp *pb.RpcObjectSubscribeIdsResponse, err error)
	SubscribeGroups(req SubscribeGroupsRequest) (*pb.RpcObjectGroupsSubscribeResponse, error)
	Unsubscribe(subIds ...string) (err error)
	UnsubscribeAndReturnIds(spaceId string, subId string) ([]string, error)
	SubscriptionIDs() []string

	app.ComponentRunnable
}

type CollectionService interface {
	SubscribeForCollection(collectionID string, subscriptionID string) ([]string, <-chan []string, error)
	UnsubscribeFromCollection(collectionID string, subscriptionID string) error
}

// subSlot is the registry entry for one subId. A Search claims the slot
// (with a unique token) before building its subscription and finalizes it
// only if its token still owns the slot — the single serialization point for
// replace-on-resubscribe, concurrent same-subId subscribes and racing
// unsubscribes. A slot owns either a core subscription or a groups adapter.
type subSlot struct {
	token  uint64
	sub    *coreSub // nil while the owning Search is still building
	groups *groupsSub
}

type service struct {
	store       objectstore.ObjectStore
	eventSender event.Sender
	// soft dependencies: resolved by type if registered; their absence only
	// fails the requests that need them
	kanban            kanban.Service
	collectionService CollectionService

	componentCtx    context.Context
	componentCancel context.CancelFunc

	mu     sync.Mutex
	slots  map[string]*subSlot
	spaces map[string]*spaceState
	seq    uint64
	closed bool
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	s.store = app.MustComponent[objectstore.ObjectStore](a)
	s.eventSender = app.MustComponent[event.Sender](a)
	// the acceptance fixtures register these under arbitrary component names
	// (or not at all), so: by type, non-panicking
	if kanbanService, err := app.GetComponent[kanban.Service](a); err == nil {
		s.kanban = kanbanService
	}
	if collectionService, err := app.GetComponent[CollectionService](a); err == nil {
		s.collectionService = collectionService
	}

	s.componentCtx, s.componentCancel = context.WithCancel(context.Background())
	s.slots = make(map[string]*subSlot)
	s.spaces = make(map[string]*spaceState)
	return nil
}

func (s *service) Run(ctx context.Context) (err error) {
	return nil
}

func (s *service) Close(ctx context.Context) error {
	s.componentCancel()

	s.mu.Lock()
	s.closed = true
	spaces := make([]*spaceState, 0, len(s.spaces))
	for _, st := range s.spaces {
		spaces = append(spaces, st)
	}
	slots := make([]*subSlot, 0, len(s.slots))
	for _, slot := range s.slots {
		slots = append(slots, slot)
	}
	s.spaces = make(map[string]*spaceState)
	s.slots = make(map[string]*subSlot)
	s.mu.Unlock()

	for _, st := range spaces {
		st.mu.Lock()
		st.stopped = true
		st.mu.Unlock()
		st.shutdown()
	}
	for _, slot := range slots {
		if slot.sub == nil {
			continue
		}
		// collection watchers would otherwise outlive the service, blocked
		// on a channel only UnsubscribeFromCollection closes
		if slot.sub.collection != nil {
			slot.sub.collection.stop()
		}
		if slot.sub.queueOwned {
			_ = slot.sub.queue.Close()
		}
	}
	return nil
}

func (s *service) Search(req SubscribeRequest) (resp *SubscribeResponse, err error) {
	spec, err := normalizeSearch(req)
	if err != nil {
		return nil, fmt.Errorf("normalize subscribe request: %w", err)
	}

	// Resolve the space index before taking any engine lock: the first open
	// of a space fires OnSpaceIndexOpened synchronously, which re-enters
	// Search on this goroutine (crossspacesub pending-space promotion).
	idx := s.store.SpaceIndex(spec.spaceId)

	filters, err := s.compileFilters(spec, idx)
	if err != nil {
		return nil, fmt.Errorf("compile filters: %w", err)
	}

	res, err := s.subscribe(spec, filters, idx)
	if err != nil {
		return nil, err
	}

	records := res.records
	if !spec.ordered {
		// ordered snapshots are already the window; unordered ones are
		// truncated for one-shot consumers (objectgraph) only
		records = truncateRecords(records, spec.offset, spec.limit)
	}
	return &SubscribeResponse{
		SubId:        spec.subId,
		Records:      records,
		Dependencies: res.depRecords,
		Counters: &pb.EventObjectSubscriptionCounters{
			SubId: spec.subId,
			Total: int64(res.total),
		},
		Output: res.sub.queue,
	}, nil
}

type subscribeResult struct {
	sub        *coreSub
	records    []*domain.Details
	depRecords []*domain.Details
	total      int
}

// subscribe is the shared core of Search and SubscribeIdsReq: claim the
// subId slot, replace any previous owner, build the sub (including scope
// sources and dep tracking), install it with its snapshot, finalize the slot
func (s *service) subscribe(spec subSpec, filters *database.Filters, idx spaceindex.Store) (subscribeResult, error) {
	old, token, err := s.claimSlot(spec.subId)
	if err != nil {
		return subscribeResult{}, err
	}
	// same-subId resubscribe replaces the subscription silently: no Remove
	// events, the response supersedes the client's list
	s.teardownSlot(old)

	sub := buildCoreSub(spec, filters)

	fail := func(err error) (subscribeResult, error) {
		if sub.collection != nil {
			sub.collection.stop()
		}
		// stop the watcher before releasing the slot: a re-claimed subId
		// must not race our editor unregistration
		s.releaseSlot(spec.subId, token)
		if sub.queueOwned {
			_ = sub.queue.Close()
		}
		if sub.space != nil {
			// a space state created just for this failed subscribe would
			// otherwise idle with a worker and a wired feed
			s.maybeDropSpace(sub.space)
		}
		return subscribeResult{}, err
	}

	if spec.collectionId != "" {
		if s.collectionService == nil {
			return fail(errors.New("collection service is not available"))
		}
		// the editor's registry is keyed by (collectionId, subscriptionId)
		// and returns the EXISTING channel on a duplicate key — a unique
		// per-claim registration id keeps concurrent same-subId generations
		// from sharing (and closing) each other's channel
		registrationId := fmt.Sprintf("%s/c%d", spec.subId, token)
		initialIds, ch, err := s.collectionService.SubscribeForCollection(spec.collectionId, registrationId)
		if err != nil {
			return fail(fmt.Errorf("subscribe for collection %s: %w", spec.collectionId, err))
		}
		sub.setScopeIds(initialIds)
		sub.collection = newCollectionWatcher(s, sub, spec.collectionId, registrationId, ch)
	}
	if spec.withDeps {
		sub.depTracker = newDepTracker(sub, spec, idx)
	}

	var (
		records    []*domain.Details
		depRecords []*domain.Details
		total      int
	)
	for {
		// re-resolve per attempt: the space may have been deleted and
		// re-created since this call started, and a stale handle must not
		// orphan a healthy state built on the current one
		idx = s.store.SpaceIndex(spec.spaceId)
		st := s.getOrCreateSpace(spec.spaceId, idx)
		if st == nil {
			return fail(errors.New("subscription service is closed"))
		}
		sub.space = st
		records, depRecords, total, err = st.install(sub, spec.asyncInit)
		if errors.Is(err, errSpaceStopped) {
			continue
		}
		break
	}
	if err != nil {
		return fail(fmt.Errorf("subscribe %s: %w", spec.subId, err))
	}
	sub.space.notify()

	res := subscribeResult{sub: sub, records: records, depRecords: depRecords, total: total}
	if !s.finalizeSlot(spec.subId, token, sub, nil) {
		// a concurrent Search or Unsubscribe won the slot; the snapshot in
		// the result is still valid, the subscription itself is gone
		s.teardown(sub)
		return res, nil
	}
	if sub.collection != nil {
		sub.collection.start()
	}
	return res, nil
}

func buildCoreSub(spec subSpec, filters *database.Filters) *coreSub {
	sub := &coreSub{
		subId:   spec.subId,
		spaceId: spec.spaceId,
		keys:    spec.keys,
		filters: filters,
		members: make(map[string]struct{}),
		vis:     make(map[string]*visEntry),
	}
	if spec.ordered {
		sub.ordered = true
		sub.limit = spec.limit
		sub.offset = spec.offset
		if len(spec.sorts) > 0 && filters != nil {
			sub.order = filters.Order
			sub.sortKeys = sortProjectionKeys(spec.sorts)
		}
	}
	if spec.scopeIds != nil {
		sub.setScopeIds(spec.scopeIds)
		sub.scopeRequestOrder = spec.scopeRequestOrder
		// ids subscriptions have no counters semantics (contract §3.6)
		sub.noCounters = spec.scopeRequestOrder
	}
	if spec.internal {
		if spec.queue != nil {
			sub.queue = spec.queue
		} else {
			sub.queue = mb.New[*pb.EventMessage](0)
			sub.queueOwned = true
		}
	}
	return sub
}

// sortProjectionKeys returns the relation keys a sort projection must carry
// for the in-memory comparator to agree with the store's ordering
func sortProjectionKeys(sorts []database.SortRequest) []domain.RelationKey {
	keys := make([]domain.RelationKey, 0, len(sorts)+2)
	var sortsByName bool
	for _, s := range sorts {
		keys = append(keys, s.RelationKey)
		if s.RelationKey == bundle.RelationKeyName {
			sortsByName = true
		}
	}
	if sortsByName {
		// the text comparator substitutes a note's snippet for its name
		// (KeyOrder.trySubstituteSnippet) — it reads these two keys, so sort
		// projections must carry them or in-memory ordering diverges from
		// the store's
		keys = append(keys, bundle.RelationKeyResolvedLayout, bundle.RelationKeySnippet)
	}
	return keys
}

func (s *service) compileFilters(spec subSpec, idx spaceindex.Store) (*database.Filters, error) {
	// NewFilters owns default-filter injection (isArchived/isDeleted/type
	// with the Condition-None opt-out), so snapshot queries and live matching
	// can never disagree. The clone is unconditional: injection appends into
	// the slice it is given, and crossspacesub reuses one request across
	// concurrent per-space subscribes.
	filters, err := database.NewFilters(database.Query{
		SpaceId: spec.spaceId,
		Filters: slices.Clone(spec.filters),
		Sorts:   spec.sorts,
	}, idx, &anyenc.Arena{}, &collate.Buffer{})
	if err != nil {
		return nil, err
	}
	// source filters are AND-composed onto the compiled object instead of
	// being appended to the request: their Or-tree would otherwise make the
	// injector's default-filter detection drop flags it had already
	// accumulated from the caller's flat filters (breaking the
	// Condition-None opt-out)
	sourceFilters, err := resolveSources(idx, spec.source)
	if err != nil {
		return nil, fmt.Errorf("resolve sources: %w", err)
	}
	if len(sourceFilters) > 0 {
		sourceObj, err := database.MakeFilters(sourceFilters, idx)
		if err != nil {
			return nil, fmt.Errorf("compile source filters: %w", err)
		}
		filters.FilterObj = database.FiltersAnd{filters.FilterObj, sourceObj}
	}
	return filters, nil
}

// claimSlot atomically claims the subId slot, returning the previous owner
// (to tear down) and the claim token to finalize with
func (s *service) claimSlot(subId string) (old *subSlot, token uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, 0, errors.New("subscription service is closed")
	}
	old = s.slots[subId]
	s.seq++
	s.slots[subId] = &subSlot{token: s.seq}
	return old, s.seq, nil
}

// teardownSlot releases whatever a previously owned slot held
func (s *service) teardownSlot(slot *subSlot) {
	if slot == nil {
		return
	}
	if slot.sub != nil {
		s.teardown(slot.sub)
	}
	if slot.groups != nil {
		s.teardownGroups(slot.groups)
	}
}

func (s *service) finalizeSlot(subId string, token uint64, sub *coreSub, groups *groupsSub) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot, ok := s.slots[subId]
	if !ok || slot.token != token {
		return false
	}
	slot.sub = sub
	slot.groups = groups
	return true
}

func (s *service) releaseSlot(subId string, token uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slot, ok := s.slots[subId]; ok && slot.token == token {
		delete(s.slots, subId)
	}
}

func (s *service) getOrCreateSpace(spaceId string, idx spaceindex.Store) *spaceState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	if st, ok := s.spaces[spaceId]; ok {
		if st.idx == idx {
			return st
		}
		// the space's store was deleted and re-created: the cached state is
		// wired to a dead store and its subscriptions are already
		// event-dead. Orphan it (its teardown paths stay valid — detachSub
		// works on a stopped state, maybeDropSpace skips states no longer
		// in the registry) and build a fresh state on the new index. The
		// old feed is unhooked and the undelivered outbox dropped so a
		// surviving handle can neither pump a dead intake nor deliver stale
		// events arbitrarily late.
		st.mu.Lock()
		st.stopped = true
		st.outbox = nil
		st.mu.Unlock()
		st.idx.SubscribeForAll(nil)
		go st.stopWorker()
		delete(s.spaces, spaceId)
	}
	st := newSpaceState(s, spaceId, idx)
	s.spaces[spaceId] = st
	return st
}

// teardown detaches a subscription: collection watcher stopped, out of its
// space (with the outbox remnant flushed), engine-owned queue closed
// (caller-provided queues are never touched — crossspacesub shares one queue
// across all its per-space subscriptions), empty space states dropped
func (s *service) teardown(sub *coreSub) {
	s.teardownReturnIds(sub)
}

// teardownReturnIds additionally returns the member ids snapshotted in the
// same critical section that detached the sub — see spaceState.detachSub
func (s *service) teardownReturnIds(sub *coreSub) []string {
	if sub.collection != nil {
		sub.collection.stop()
	}
	ids, empty := sub.space.detachSub(sub)
	if sub.queueOwned {
		_ = sub.queue.Close()
	}
	if empty {
		s.maybeDropSpace(sub.space)
	}
	return ids
}

// maybeDropSpace removes a subscription-less space state: feed unhooked,
// worker stopped. A Search that raced us retries against a fresh state (see
// install's errSpaceStopped). The feed MUST be unhooked while s.mu is held:
// released earlier, a concurrent Search could create and wire a fresh
// spaceState for the same space, and this unhook would then silently sever
// the fresh state's feed forever.
func (s *service) maybeDropSpace(st *spaceState) {
	s.mu.Lock()
	if s.spaces[st.spaceId] != st || !st.markStopped() {
		s.mu.Unlock()
		return
	}
	delete(s.spaces, st.spaceId)
	st.idx.SubscribeForAll(nil)
	s.mu.Unlock()
	st.stopWorker()
}

func (s *service) Unsubscribe(subIds ...string) (err error) {
	for _, subId := range subIds {
		s.mu.Lock()
		slot := s.slots[subId]
		delete(s.slots, subId)
		s.mu.Unlock()
		// an empty slot means an in-flight Search owns it; deleting the
		// slot makes its finalize fail and tear the fresh sub down itself
		s.teardownSlot(slot)
	}
	return nil
}

func (s *service) UnsubscribeAndReturnIds(spaceId string, subId string) ([]string, error) {
	s.mu.Lock()
	slot := s.slots[subId]
	if slot == nil || slot.sub == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("subscription %s not found", subId)
	}
	sub := slot.sub
	delete(s.slots, subId)
	s.mu.Unlock()

	// the ids must come from the same critical section that detaches the
	// sub: snapshotting them separately leaves a gap in which the worker
	// can still deliver membership events the returned list doesn't reflect
	return s.teardownReturnIds(sub), nil
}

func (s *service) SubscriptionIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.slots))
	for subId, slot := range s.slots {
		if slot.sub != nil || slot.groups != nil {
			ids = append(ids, subId)
		}
	}
	return ids
}

func (s *service) SubscribeIdsReq(req pb.RpcObjectSubscribeIdsRequest) (resp *pb.RpcObjectSubscribeIdsResponse, err error) {
	spec, err := normalizeSubscribeIds(req)
	if err != nil {
		return nil, fmt.Errorf("normalize subscribe ids request: %w", err)
	}
	idx := s.store.SpaceIndex(spec.spaceId)
	res, err := s.subscribe(spec, nil, idx)
	if err != nil {
		return nil, err
	}
	return &pb.RpcObjectSubscribeIdsResponse{
		SubId:        spec.subId,
		Records:      domain.DetailsListToProtos(res.records),
		Dependencies: domain.DetailsListToProtos(res.depRecords),
	}, nil
}

// compileGroupsFilters compiles a fresh filter object for one groups
// recomputation; source filters are AND-composed onto the compiled object
// (see compileFilters for why)
func compileGroupsFilters(spaceId string, filterRequests, sourceFilters []database.FilterRequest, idx spaceindex.Store) (*database.Filters, error) {
	filters, err := database.NewFilters(database.Query{
		SpaceId: spaceId,
		Filters: slices.Clone(filterRequests),
	}, idx, &anyenc.Arena{}, &collate.Buffer{})
	if err != nil {
		return nil, err
	}
	if len(sourceFilters) > 0 {
		sourceObj, err := database.MakeFilters(sourceFilters, idx)
		if err != nil {
			return nil, fmt.Errorf("compile source filters: %w", err)
		}
		filters.FilterObj = database.FiltersAnd{filters.FilterObj, sourceObj}
	}
	return filters, nil
}

// collectionSnapshotIds reads a point-in-time membership snapshot of the
// collection: subscribe, take the ids, immediately unsubscribe. The editor
// broadcasts on the returned unbuffered channel while holding the collection
// object's lock — the very lock the unsubscribe needs — so the channel is
// drained until the unsubscribe closes it, or both sides deadlock
// permanently.
func (s *service) collectionSnapshotIds(collectionId, subId string) ([]string, error) {
	snapId := fmt.Sprintf("%s/groups-snap-%s", subId, bson.NewObjectId().Hex())
	ids, ch, err := s.collectionService.SubscribeForCollection(collectionId, snapId)
	var drained chan struct{}
	if ch != nil {
		drained = make(chan struct{})
		go func() {
			defer close(drained)
			for range ch {
			}
		}()
	}
	uerr := s.collectionService.UnsubscribeFromCollection(collectionId, snapId)
	if uerr != nil {
		log.Warnf("groups subscription %s: release collection snapshot: %v", subId, uerr)
	} else if drained != nil {
		<-drained
	}
	if err != nil {
		return nil, fmt.Errorf("collection snapshot for groups %s: %w", collectionId, err)
	}
	return ids, nil
}

func (s *service) SubscribeGroups(req SubscribeGroupsRequest) (*pb.RpcObjectGroupsSubscribeResponse, error) {
	if req.SpaceId == "" {
		return nil, errors.New("spaceId is required")
	}
	if req.RelationKey == "" {
		return nil, errors.New("relationKey is required")
	}
	if s.kanban == nil {
		return nil, errors.New("kanban service is not available")
	}
	subId := req.SubId
	if subId == "" {
		subId = bson.NewObjectId().Hex()
	}

	grouper, err := s.kanban.Grouper(req.SpaceId, req.RelationKey)
	if err != nil {
		return nil, fmt.Errorf("get grouper: %w", err)
	}
	idx := s.store.SpaceIndex(req.SpaceId)

	filterRequests := slices.Clone(req.Filters)
	sourceFilters, err := resolveSources(idx, req.Source)
	if err != nil {
		return nil, fmt.Errorf("resolve sources: %w", err)
	}
	if req.CollectionId != "" && s.collectionService == nil {
		return nil, errors.New("collection service is not available")
	}
	if req.CollectionId != "" {
		// collection scoping for groups is a snapshot: the grouper computes
		// from store queries, so membership is folded in as an id filter
		// taken at subscribe time (group sets refresh on re-subscribe)
		ids, err := s.collectionSnapshotIds(req.CollectionId, subId)
		if err != nil {
			return nil, err
		}
		filterRequests = append(filterRequests, database.FilterRequest{
			RelationKey: bundle.RelationKeyId,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.StringList(ids),
		})
	}
	// the kanban groupers mutate the filters they are given, so every
	// recomputation compiles a fresh copy; idx is captured by reference so
	// recomputes after a space re-resolve use the current index
	compile := func() (*database.Filters, error) {
		return compileGroupsFilters(req.SpaceId, filterRequests, sourceFilters, idx)
	}
	matchFilters, err := compile()
	if err != nil {
		return nil, fmt.Errorf("compile filters: %w", err)
	}

	old, token, err := s.claimSlot(subId)
	if err != nil {
		return nil, err
	}
	s.teardownSlot(old)

	g := &groupsSub{
		subId:       subId,
		spaceId:     req.SpaceId,
		relationKey: domain.RelationKey(req.RelationKey),
		grouper:     grouper,
		svc:         s,
		compile:     compile,
		match:       matchFilters.FilterObj,
	}

	fail := func(err error) (*pb.RpcObjectGroupsSubscribeResponse, error) {
		s.releaseSlot(subId, token)
		if g.space != nil {
			s.maybeDropSpace(g.space)
		}
		return nil, err
	}

	for {
		idx = s.store.SpaceIndex(req.SpaceId)
		st := s.getOrCreateSpace(req.SpaceId, idx)
		if st == nil {
			return fail(errors.New("subscription service is closed"))
		}
		g.space = st
		err = st.installGroups(g)
		if errors.Is(err, errSpaceStopped) {
			continue
		}
		break
	}
	if err != nil {
		return fail(fmt.Errorf("subscribe groups %s: %w", subId, err))
	}

	groups, err := g.init()
	if err != nil {
		s.teardownGroups(g)
		return fail(fmt.Errorf("compute groups %s: %w", subId, err))
	}

	if !s.finalizeSlot(subId, token, nil, g) {
		s.teardownGroups(g)
	}
	return &pb.RpcObjectGroupsSubscribeResponse{
		SubId:  subId,
		Groups: groups,
	}, nil
}

func (s *service) teardownGroups(g *groupsSub) {
	g.markDead()
	if g.space.removeGroupsSub(g) {
		s.maybeDropSpace(g.space)
	}
}
