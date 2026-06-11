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
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

// The subscription engine: a live-query service over object details. The
// behavioral contract is docs/Subscriptions.md; the architecture is
// docs/SubscriptionsEngineDesign.md — one core primitive (scope + filters +
// order/window over a per-space store feed) surrounded by adapters.

const CName = "subscription"

var log = logging.Logger("anytype-mw-subscription")

// ErrNotImplemented is returned by query surfaces that are not reimplemented
// yet (groups and ids subscriptions arrive in later phases)
var ErrNotImplemented = errors.New("subscriptions are not implemented")

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
	// InternalQueue is used when Internal flag is set to true. If it's nil, new queue will be created
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
	SubscribeIds(subId string, ids []string) (records []*domain.Details, err error)
	SubscribeGroups(req SubscribeGroupsRequest) (*pb.RpcObjectGroupsSubscribeResponse, error)
	Unsubscribe(subIds ...string) (err error)
	UnsubscribeAndReturnIds(spaceId string, subId string) ([]string, error)
	UnsubscribeAll() (err error)
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
// unsubscribes.
type subSlot struct {
	token uint64
	sub   *coreSub // nil while the owning Search is still building
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
	s.kanban, _ = app.GetComponent[kanban.Service](a)
	s.collectionService, _ = app.GetComponent[CollectionService](a)

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
		if slot.sub != nil && slot.sub.queueOwned {
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

	old, token, err := s.claimSlot(spec.subId)
	if err != nil {
		return nil, err
	}
	if old != nil {
		// same-subId resubscribe replaces the subscription silently: no
		// Remove events, the response supersedes the client's list
		s.teardown(old)
	}

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
		if len(spec.sorts) > 0 {
			sub.order = filters.Order
			sub.sortKeys = make([]domain.RelationKey, 0, len(spec.sorts))
			for _, s := range spec.sorts {
				sub.sortKeys = append(sub.sortKeys, s.RelationKey)
			}
		}
	}
	if spec.internal {
		if spec.queue != nil {
			sub.queue = spec.queue
		} else {
			sub.queue = mb.New[*pb.EventMessage](0)
			sub.queueOwned = true
		}
	}

	var (
		records []*domain.Details
		total   int
	)
	for {
		st := s.getOrCreateSpace(spec.spaceId, idx)
		if st == nil {
			err = errors.New("subscription service is closed")
			break
		}
		sub.space = st
		records, total, err = st.install(sub, spec.asyncInit)
		if errors.Is(err, errSpaceStopped) {
			continue
		}
		break
	}
	if err != nil {
		s.releaseSlot(spec.subId, token)
		if sub.queueOwned {
			_ = sub.queue.Close()
		}
		return nil, fmt.Errorf("subscribe %s: %w", spec.subId, err)
	}
	sub.space.notify()

	if !s.finalizeSlot(spec.subId, token, sub) {
		// a concurrent Search or Unsubscribe won the slot; the snapshot in
		// the response is still valid, the subscription itself is gone
		s.teardown(sub)
	}

	if !spec.ordered {
		// ordered snapshots are already the window; unordered ones are
		// truncated for one-shot consumers (objectgraph) only
		records = truncateRecords(records, spec.offset, spec.limit)
	}
	return &SubscribeResponse{
		SubId:   spec.subId,
		Records: records,
		Counters: &pb.EventObjectSubscriptionCounters{
			SubId: spec.subId,
			Total: int64(total),
		},
		Output: sub.queue,
	}, nil
}

func (s *service) compileFilters(spec subSpec, idx spaceindex.Store) (*database.Filters, error) {
	filters := spec.filters
	sourceFilters, err := resolveSources(idx, spec.source)
	if err != nil {
		return nil, fmt.Errorf("resolve sources: %w", err)
	}
	if len(sourceFilters) > 0 {
		filters = append(slices.Clone(filters), sourceFilters...)
	}
	// NewFilters owns default-filter injection (isArchived/isDeleted/type
	// with the Condition-None opt-out), so snapshot queries and live matching
	// can never disagree
	return database.NewFilters(database.Query{
		SpaceId: spec.spaceId,
		Filters: filters,
		Sorts:   spec.sorts,
	}, idx, &anyenc.Arena{}, &collate.Buffer{})
}

// claimSlot atomically claims the subId slot, returning the previous owner
// (to tear down) and the claim token to finalize with
func (s *service) claimSlot(subId string) (old *coreSub, token uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, 0, errors.New("subscription service is closed")
	}
	if slot, ok := s.slots[subId]; ok {
		old = slot.sub
	}
	s.seq++
	s.slots[subId] = &subSlot{token: s.seq}
	return old, s.seq, nil
}

func (s *service) finalizeSlot(subId string, token uint64, sub *coreSub) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot, ok := s.slots[subId]
	if !ok || slot.token != token {
		return false
	}
	slot.sub = sub
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
		return st
	}
	st := newSpaceState(s, spaceId, idx)
	s.spaces[spaceId] = st
	return st
}

// teardown detaches a subscription: out of its space, engine-owned queue
// closed (caller-provided queues are never touched — crossspacesub shares one
// queue across all its per-space subscriptions), empty space states dropped
func (s *service) teardown(sub *coreSub) {
	empty := sub.space.removeSub(sub)
	if sub.queueOwned {
		_ = sub.queue.Close()
	}
	if empty {
		s.maybeDropSpace(sub.space)
	}
}

// maybeDropSpace removes a subscription-less space state: feed unhooked,
// worker stopped. A Search that raced us retries against a fresh state (see
// install's errSpaceStopped).
func (s *service) maybeDropSpace(st *spaceState) {
	s.mu.Lock()
	if s.spaces[st.spaceId] != st || !st.markStopped() {
		s.mu.Unlock()
		return
	}
	delete(s.spaces, st.spaceId)
	s.mu.Unlock()
	st.shutdown()
}

func (s *service) Unsubscribe(subIds ...string) (err error) {
	for _, subId := range subIds {
		s.mu.Lock()
		slot := s.slots[subId]
		delete(s.slots, subId)
		s.mu.Unlock()
		// a nil slot.sub means an in-flight Search owns it; deleting the
		// slot makes its finalize fail and tear the fresh sub down itself
		if slot != nil && slot.sub != nil {
			s.teardown(slot.sub)
		}
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

	sub.space.mu.Lock()
	ids := sub.memberIds()
	sub.space.mu.Unlock()

	s.teardown(sub)
	return ids, nil
}

func (s *service) SubscriptionIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.slots))
	for subId, slot := range s.slots {
		if slot.sub != nil {
			ids = append(ids, subId)
		}
	}
	return ids
}

func (s *service) SubscribeIdsReq(req pb.RpcObjectSubscribeIdsRequest) (resp *pb.RpcObjectSubscribeIdsResponse, err error) {
	return nil, ErrNotImplemented
}

func (s *service) SubscribeIds(subId string, ids []string) (records []*domain.Details, err error) {
	return nil, ErrNotImplemented
}

func (s *service) SubscribeGroups(req SubscribeGroupsRequest) (*pb.RpcObjectGroupsSubscribeResponse, error) {
	return nil, ErrNotImplemented
}

func (s *service) UnsubscribeAll() (err error) {
	return nil
}
