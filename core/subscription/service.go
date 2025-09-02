package subscription

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/globalsign/mgo/bson"
	"golang.org/x/exp/slices"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

const CName = "subscription"

var log = logging.Logger("anytype-mw-subscription")

var batchTime = 250 * time.Millisecond

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

type subscription interface {
	init(entries []*entry) (err error)
	counters() (prev, next int)
	onChange(ctx *opCtx)
	getActiveRecords() (res []*domain.Details)
	hasDep() bool
	getDep() subscription
	close()
}

type CollectionService interface {
	SubscribeForCollection(collectionID string, subscriptionID string) ([]string, <-chan []string, error)
	UnsubscribeFromCollection(collectionID string, subscriptionID string)
}

type service struct {
	lock      sync.Mutex
	spaceSubs map[string]*spaceSubscriptions

	// Deps
	objectStore       objectstore.ObjectStore
	kanban            kanban.Service
	collectionService CollectionService
	eventSender       event.Sender
	arenaPool         *anyenc.ArenaPool
}

type internalSubOutput struct {
	externallyManaged bool
	queue             *mb.MB[*pb.EventMessage]
}

func newInternalSubOutput(queue *mb.MB[*pb.EventMessage]) *internalSubOutput {
	if queue == nil {
		return &internalSubOutput{
			queue: mb.New[*pb.EventMessage](0),
		}
	}
	return &internalSubOutput{
		externallyManaged: true,
		queue:             queue,
	}
}

func (o *internalSubOutput) add(msgs ...*pb.EventMessage) error {
	return o.queue.Add(context.TODO(), msgs...)
}

func (o *internalSubOutput) close() error {
	if !o.externallyManaged {
		return o.queue.Close()
	}
	return nil
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.kanban = app.MustComponent[kanban.Service](a)
	s.collectionService = app.MustComponent[CollectionService](a)
	s.eventSender = app.MustComponent[event.Sender](a)

	s.spaceSubs = map[string]*spaceSubscriptions{}
	s.arenaPool = &anyenc.ArenaPool{}
	return
}

func (s *service) Run(ctx context.Context) (err error) {
	return
}

func (s *service) Close(ctx context.Context) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	var err error
	for _, spaceSub := range s.spaceSubs {
		err = errors.Join(err, spaceSub.Close(ctx))
	}
	return err
}

func (s *service) Search(req SubscribeRequest) (resp *SubscribeResponse, err error) {
	if req.SpaceId == "" {
		return nil, fmt.Errorf("spaceId should not be empty")
	}
	// todo: removed temp fix after we will have session-scoped subscriptions
	// this is to prevent multiple subscriptions with the same id in different spaces
	err = s.Unsubscribe(req.SubId)
	if err != nil {
		return nil, err
	}
	spaceSubs, err := s.getSpaceSubscriptions(req.SpaceId)
	if err != nil {
		return nil, err
	}
	return spaceSubs.Search(req)
}

func (s *service) SubscribeIdsReq(req pb.RpcObjectSubscribeIdsRequest) (resp *pb.RpcObjectSubscribeIdsResponse, err error) {
	if req.SpaceId == "" {
		return nil, fmt.Errorf("spaceId should not be empty")
	}
	// todo: removed temp fix after we will have session-scoped subscriptions
	// this is to prevent multiple subscriptions with the same id in different spaces
	err = s.Unsubscribe(req.SubId)
	if err != nil {
		return nil, err
	}
	spaceSubs, err := s.getSpaceSubscriptions(req.SpaceId)
	if err != nil {
		return nil, err
	}
	return spaceSubs.SubscribeIdsReq(req)
}

func (s *service) SubscribeIds(subId string, ids []string) (records []*domain.Details, err error) {
	return
}

func (s *service) SubscribeGroups(req SubscribeGroupsRequest) (*pb.RpcObjectGroupsSubscribeResponse, error) {
	// todo: removed temp fix after we will have session-scoped subscriptions
	// this is to prevent multiple subscriptions with the same id in different spaces
	err := s.Unsubscribe(req.SubId)
	if err != nil {
		return nil, err
	}
	spaceSubs, err := s.getSpaceSubscriptions(req.SpaceId)
	if err != nil {
		return nil, err
	}
	return spaceSubs.SubscribeGroups(req)
}

func (s *service) Unsubscribe(subIds ...string) (err error) {
	s.lock.Lock()
	subs := make([]*spaceSubscriptions, 0, len(s.spaceSubs))
	for _, spaceSub := range s.spaceSubs {
		subs = append(subs, spaceSub)
	}
	s.lock.Unlock()
	for _, spaceSub := range subs {
		err = errors.Join(spaceSub.Unsubscribe(subIds...))
	}
	return err
}

func (s *service) UnsubscribeAndReturnIds(spaceId string, subId string) ([]string, error) {
	spaceSub, err := s.getSpaceSubscriptions(spaceId)
	if err != nil {
		return nil, fmt.Errorf("get space subs: %w", err)
	}

	return spaceSub.UnsubscribeAndReturnIds(subId)
}

func (s *service) UnsubscribeAll() (err error) {
	s.lock.Lock()
	for _, spaceSub := range s.spaceSubs {
		err = errors.Join(spaceSub.UnsubscribeAll())
	}
	s.lock.Unlock()
	return err
}

func (s *service) SubscriptionIDs() []string {
	var ids []string
	s.lock.Lock()
	for _, spaceSub := range s.spaceSubs {
		ids = append(ids, spaceSub.SubscriptionIDs()...)
	}
	s.lock.Unlock()
	return ids
}

func (s *service) getSpaceSubscriptions(spaceId string) (*spaceSubscriptions, error) {
	if spaceId == "" {
		return nil, fmt.Errorf("spaceId is empty")
	}
	s.lock.Lock()
	defer s.lock.Unlock()

	spaceSubs, ok := s.spaceSubs[spaceId]
	if !ok {
		cache := newCache()
		spaceSubs = &spaceSubscriptions{
			cache:             cache,
			subscriptionKeys:  make([]string, 0, 20),
			subscriptions:     make(map[string]subscription, 20),
			customOutput:      map[string]*internalSubOutput{},
			recBatch:          mb.New[database.Record](0),
			objectStore:       s.objectStore.SpaceIndex(spaceId),
			kanban:            s.kanban,
			collectionService: s.collectionService,
			eventSender:       s.eventSender,
			ctxBuf:            &opCtx{spaceId: spaceId, c: cache},
			arenaPool:         s.arenaPool,
		}
		spaceSubs.ds = newDependencyService(spaceSubs)
		spaceSubs.om = newOrderManager(spaceSubs)
		spaceSubs.initDebugger()
		err := spaceSubs.Run()
		if err != nil {
			return nil, fmt.Errorf("run space subscriptions: %w", err)
		}
		s.spaceSubs[spaceId] = spaceSubs
	}
	return spaceSubs, nil
}

type spaceSubscriptions struct {
	subscriptionKeys []string
	subscriptions    map[string]subscription

	customOutput map[string]*internalSubOutput
	recBatch     *mb.MB[database.Record]

	// Deps
	objectStore       spaceindex.Store
	kanban            kanban.Service
	collectionService CollectionService
	eventSender       event.Sender

	m      sync.Mutex
	cache  *cache
	ds     *dependencyService
	om     *orderManager
	ctxBuf *opCtx

	subDebugger *subDebugger
	arenaPool   *anyenc.ArenaPool
	ctx         context.Context
	cancelCtx   context.CancelFunc
}

func (s *spaceSubscriptions) Run() (err error) {
	s.ctx, s.cancelCtx = context.WithCancel(context.Background())
	var batchErr error
	s.objectStore.SubscribeForAll(func(rec database.Record) {
		batchErr = s.recBatch.Add(s.ctx, rec)
	})
	if batchErr != nil {
		return batchErr
	}
	go s.recordsHandler()
	return
}

func (s *spaceSubscriptions) getSubscription(id string) (subscription, bool) {
	sub, ok := s.subscriptions[id]
	return sub, ok
}

func (s *spaceSubscriptions) setSubscription(id string, sub subscription) {
	s.subscriptions[id] = sub
	if !slices.Contains(s.subscriptionKeys, id) {
		s.subscriptionKeys = append(s.subscriptionKeys, id)
	}
}

func (s *spaceSubscriptions) deleteSubscription(id string) {
	delete(s.subscriptions, id)
	s.subscriptionKeys = slice.RemoveMut(s.subscriptionKeys, id)
}

func (s *spaceSubscriptions) iterateSubscriptions(proc func(sub subscription)) {
	for _, subId := range s.subscriptionKeys {
		sub, ok := s.getSubscription(subId)
		if ok && sub != nil {
			proc(sub)
		}
	}
}

func (s *spaceSubscriptions) Search(req SubscribeRequest) (*SubscribeResponse, error) {
	if req.SpaceId == "" {
		return nil, fmt.Errorf("spaceId is required")
	}
	if req.SubId == "" {
		req.SubId = bson.NewObjectId().Hex()
	}

	q := database.Query{
		Filters: req.Filters,
		Sorts:   req.Sorts,
		Limit:   int(req.Limit),
	}

	f, err := database.NewFilters(q, s.objectStore, &anyenc.Arena{}, &collate.Buffer{})
	if err != nil {
		return nil, fmt.Errorf("new database filters: %w", err)
	}

	if len(req.Source) > 0 {
		sourceFilter, err := s.filtersFromSource(req.SpaceId, req.Source)
		if err != nil {
			return nil, fmt.Errorf("can't make filter from source: %w", err)
		}
		f.FilterObj = database.FiltersAnd{f.FilterObj, sourceFilter}
	}

	qryEntries := func() ([]*entry, error) {
		return queryEntries(s.objectStore, f)
	}

	s.m.Lock()
	defer s.m.Unlock()

	filterDepIds := s.depIdsFromFilter(req.SpaceId, req.Filters)
	if existing, ok := s.getSubscription(req.SubId); ok {
		s.deleteSubscription(req.SubId)
		existing.close()
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
	if req.Limit < 0 {
		req.Limit = 0
	}

	if req.CollectionId != "" {
		return s.subscribeForCollection(req, f, filterDepIds)
	}
	return s.subscribeForQuery(req, f, qryEntries, filterDepIds)
}

func (s *spaceSubscriptions) subscribeForQuery(req SubscribeRequest, f *database.Filters, queryEntries func() ([]*entry, error), filterDepIds []string) (*SubscribeResponse, error) {
	sub := s.newSortedSub(req.SubId, req.SpaceId, slice.StringsInto[domain.RelationKey](req.Keys), f.FilterObj, f.Order, int(req.Limit), int(req.Offset))
	if req.NoDepSubscription {
		sub.disableDep = true
	} else {
		sub.forceSubIds = filterDepIds
	}
	s.setSubscription(sub.id, sub)

	// FIXME Nested subscriptions disabled now. We should enable them only by client's request
	// Uncomment test xTestNestedSubscription after enabling this
	if withNested, ok := f.FilterObj.(database.WithNestedFilter); ok && false {
		var nestedCount int
		err := withNested.IterateNestedFilters(func(nestedFilter database.Filter) error {
			nestedCount++
			f, ok := nestedFilter.(*database.FilterNestedIn)
			if ok {
				childSub := s.newSortedSub(req.SubId+fmt.Sprintf("-nested-%d", nestedCount), req.SpaceId, []domain.RelationKey{bundle.RelationKeyId}, f.FilterForNestedObjects, nil, 0, 0)
				err := initSubEntries(s.objectStore, &database.Filters{FilterObj: f.FilterForNestedObjects}, childSub)
				if err != nil {
					return fmt.Errorf("init nested sub %s entries: %w", childSub.id, err)
				}
				sub.nested = append(sub.nested, childSub)
				childSub.parent = sub
				childSub.parentFilter = f
				s.setSubscription(childSub.id, childSub)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("iterate nested filters: %w", err)
		}
	}

	for _, sort := range req.Sorts {
		if err := s.om.initOrderSubscription(sort.RelationKey, sub); err != nil {
			log.Errorf("failed to create order subscription: %s", err.Error())
		}
	}

	var outputQueue *mb.MB[*pb.EventMessage]
	if req.Internal {
		output := newInternalSubOutput(req.InternalQueue)
		outputQueue = output.queue
		s.customOutput[req.SubId] = output
	}
	s.m.Unlock()

	// Query initial entries out of critical section to reduce lock contention
	// For full consistency we've already started to observe objects for this subscription, see entriesBeforeStarted
	entries, err := queryEntries()
	if err != nil {
		return nil, fmt.Errorf("query entries: %w", err)
	}

	s.m.Lock()
	entries = append(entries, sub.entriesBeforeStarted...)
	sub.entriesBeforeStarted = nil

	if req.AsyncInit {
		err := sub.init(nil)
		if err != nil {
			return nil, fmt.Errorf("async: init sub entries: %w", err)
		}

		for i, e := range entries {
			e = s.cache.GetOrSet(e)
			entries[i] = e
			e.SetSub(req.SubId, false, false)
		}
		s.onChangeWithinContext(entries, func(ctxBuf *opCtx) {
			sub.onChange(ctxBuf)
		})

	} else {
		err := sub.init(entries)
		if err != nil {
			return nil, fmt.Errorf("init sub entries: %w", err)
		}
	}

	prev, next := sub.counters()

	var depRecords, subRecords []*domain.Details

	if !req.AsyncInit {
		subRecords = sub.getActiveRecords()

		if sub.depSub != nil {
			depRecords = sub.depSub.getActiveRecords()
		}
	}

	return &SubscribeResponse{
		Records:      subRecords,
		Dependencies: depRecords,
		SubId:        sub.id,
		Counters: &pb.EventObjectSubscriptionCounters{
			Total:     int64(sub.skl.Len()),
			NextCount: int64(prev),
			PrevCount: int64(next),
		},
		Output: outputQueue,
	}, nil
}

func initSubEntries(objectStore spaceindex.Store, f *database.Filters, sub *sortedSub) error {
	entries, err := queryEntries(objectStore, f)
	if err != nil {
		return err
	}
	if err = sub.init(entries); err != nil {
		return fmt.Errorf("subscription init error: %w", err)
	}
	return nil
}

func queryEntries(objectStore spaceindex.Store, f *database.Filters) ([]*entry, error) {
	records, err := objectStore.QueryRaw(f, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("objectStore query error: %w", err)
	}
	entries := make([]*entry, 0, len(records))
	for _, r := range records {
		entries = append(entries, newEntry(r.Details.GetString(bundle.RelationKeyId), r.Details))
	}
	return entries, nil
}

func (s *spaceSubscriptions) subscribeForCollection(req SubscribeRequest, f *database.Filters, filterDepIds []string) (*SubscribeResponse, error) {
	sub, err := s.newCollectionSub(req.SubId, req.SpaceId, req.CollectionId, slice.StringsInto[domain.RelationKey](req.Keys), filterDepIds, f.FilterObj, f.Order, int(req.Limit), int(req.Offset), req.NoDepSubscription)
	if err != nil {
		return nil, err
	}
	if err := sub.init(nil); err != nil {
		return nil, fmt.Errorf("subscription init error: %w", err)
	}
	s.setSubscription(sub.sortedSub.id, sub)
	prev, next := sub.counters()

	for _, sort := range req.Sorts {
		if err := s.om.initOrderSubscription(sort.RelationKey, sub.sortedSub); err != nil {
			log.Errorf("failed to create order subscription: %s", err.Error())
		}
	}

	var depRecords, subRecords []*domain.Details
	subRecords = sub.getActiveRecords()

	if sub.sortedSub.depSub != nil && !sub.sortedSub.disableDep {
		depRecords = sub.sortedSub.depSub.getActiveRecords()
	}

	var outputQueue *mb.MB[*pb.EventMessage]
	if req.Internal {
		output := newInternalSubOutput(req.InternalQueue)
		outputQueue = output.queue
		s.customOutput[req.SubId] = output
	}

	return &SubscribeResponse{
		Records:      subRecords,
		Dependencies: depRecords,
		SubId:        sub.sortedSub.id,
		Counters: &pb.EventObjectSubscriptionCounters{
			Total:     int64(sub.sortedSub.skl.Len()),
			NextCount: int64(prev),
			PrevCount: int64(next),
		},
		Output: outputQueue,
	}, nil
}

func (s *spaceSubscriptions) SubscribeIdsReq(req pb.RpcObjectSubscribeIdsRequest) (resp *pb.RpcObjectSubscribeIdsResponse, err error) {
	if req.SpaceId == "" {
		return nil, fmt.Errorf("spaceId is required")
	}
	if req.SubId == "" {
		req.SubId = bson.NewObjectId().Hex()
	}

	s.m.Lock()
	sub := s.newIdsSub(req.SubId, req.SpaceId, slice.StringsInto[domain.RelationKey](req.Keys), req.NoDepSubscription)
	sub.addIds(req.Ids)
	s.m.Unlock()

	// Query initial entries out of critical section to reduce lock contention
	// For full consistency we've already started to observe objects for this subscription, see entriesBeforeStarted
	records, err := s.objectStore.QueryByIds(req.Ids)
	if err != nil {
		return
	}
	entries := make([]*entry, 0, len(records))
	for _, r := range records {
		entries = append(entries, newEntry(r.Details.GetString(bundle.RelationKeyId), r.Details))
	}

	s.m.Lock()
	defer s.m.Unlock()
	// Process entries before started to handle the deferred start pattern
	if len(sub.entriesBeforeStarted) > 0 {
		entries = append(entries, sub.entriesBeforeStarted...)
		sub.entriesBeforeStarted = nil
	}

	if err = sub.init(entries); err != nil {
		return
	}

	s.setSubscription(sub.id, sub)

	var depRecords, subRecords []*domain.Details
	subRecords = sub.getActiveRecords()

	if sub.depSub != nil {
		depRecords = sub.depSub.getActiveRecords()
	}
	return &pb.RpcObjectSubscribeIdsResponse{
		Records:      domain.DetailsListToProtos(subRecords),
		Dependencies: domain.DetailsListToProtos(depRecords),
		SubId:        req.SubId,
	}, nil
}

type SubscribeGroupsRequest struct {
	SpaceId      string
	SubId        string
	RelationKey  string
	Filters      []database.FilterRequest
	Source       []string
	CollectionId string
}

func (s *spaceSubscriptions) SubscribeGroups(req SubscribeGroupsRequest) (*pb.RpcObjectGroupsSubscribeResponse, error) {
	subId := ""

	q := database.Query{
		Filters: req.Filters,
	}

	arena := s.arenaPool.Get()
	defer s.arenaPool.Put(arena)

	flt, err := database.NewFilters(q, s.objectStore, arena, &collate.Buffer{})
	if err != nil {
		return nil, err
	}

	if len(req.Source) > 0 {
		sourceFilter, err := s.filtersFromSource(req.SpaceId, req.Source)
		if err != nil {
			return nil, fmt.Errorf("can't make filter from source: %w", err)
		}
		flt.FilterObj = database.FiltersAnd{flt.FilterObj, sourceFilter}
	}

	var colObserver *collectionObserver
	if req.CollectionId != "" {
		colObserver, err = s.newCollectionObserver(req.SpaceId, req.CollectionId, req.SubId)
		if err != nil {
			return nil, err
		}
		if flt == nil {
			flt = &database.Filters{}
		}
		if flt.FilterObj == nil {
			flt.FilterObj = colObserver
		} else {
			flt.FilterObj = database.FiltersAnd{colObserver, flt.FilterObj}
		}
	}

	grouper, err := s.kanban.Grouper(req.SpaceId, req.RelationKey)
	if err != nil {
		return nil, err
	}

	if err := grouper.InitGroups(req.SpaceId, flt); err != nil {
		return nil, err
	}

	dataViewGroups, err := grouper.MakeDataViewGroups()
	if err != nil {
		return nil, err
	}

	if tagGrouper, ok := grouper.(*kanban.GroupTag); ok {
		groups, err := tagGrouper.MakeDataViewGroups()
		if err != nil {
			return nil, err
		}

		subId = req.SubId
		if subId == "" {
			subId = bson.NewObjectId().Hex()
		}

		s.m.Lock()
		defer s.m.Unlock()

		var sub subscription
		if colObserver != nil {
			sub = s.newCollectionGroupSub(subId, domain.RelationKey(req.RelationKey), flt, groups, colObserver)
		} else {
			sub = s.newGroupSub(subId, domain.RelationKey(req.RelationKey), flt, groups)
		}

		entries := make([]*entry, 0, len(tagGrouper.Records))
		for _, r := range tagGrouper.Records {
			entries = append(entries, newEntry(r.Details.GetString(bundle.RelationKeyId), r.Details))
		}

		if err := sub.init(entries); err != nil {
			return nil, err
		}
		s.setSubscription(subId, sub)
	} else if colObserver != nil {
		colObserver.close()
	}

	return &pb.RpcObjectGroupsSubscribeResponse{
		Error:  &pb.RpcObjectGroupsSubscribeResponseError{},
		Groups: dataViewGroups,
		SubId:  subId,
	}, nil
}

func (s *spaceSubscriptions) SubscribeIds(subId string, ids []string) (records []*domain.Details, err error) {
	return
}

func (s *spaceSubscriptions) Unsubscribe(subIds ...string) error {
	s.m.Lock()
	defer s.m.Unlock()
	for _, subId := range subIds {
		if sub, ok := s.getSubscription(subId); ok {
			err := s.unsubscribe(subId, sub)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *spaceSubscriptions) unsubscribe(subId string, sub subscription) error {
	out := s.customOutput[subId]
	if out != nil {
		err := out.close()
		if err != nil {
			return fmt.Errorf("close subscription %s: %w", subId, err)
		}
		s.customOutput[subId] = nil
	}
	sub.close()
	s.deleteSubscription(subId)
	return nil
}

func (s *spaceSubscriptions) UnsubscribeAndReturnIds(subId string) ([]string, error) {
	s.m.Lock()
	defer s.m.Unlock()

	if sub, ok := s.getSubscription(subId); ok {
		recs := sub.getActiveRecords()
		ids := make([]string, 0, len(recs))
		for _, rec := range recs {
			ids = append(ids, rec.GetString(bundle.RelationKeyId))
		}
		err := s.unsubscribe(subId, sub)
		if err != nil {
			return nil, err
		}
		return ids, nil
	}
	return nil, fmt.Errorf("subscription not found")
}

func (s *spaceSubscriptions) UnsubscribeAll() (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	for _, sub := range s.subscriptions {
		sub.close()
	}
	s.subscriptions = make(map[string]subscription)
	s.subscriptionKeys = s.subscriptionKeys[:0]
	return
}

func (s *spaceSubscriptions) SubscriptionIDs() []string {
	s.m.Lock()
	defer s.m.Unlock()
	return s.subscriptionKeys
}

func (s *spaceSubscriptions) recordsHandler() {
	var entries []*entry
	nilIfExists := func(id string) {
		for i, e := range entries {
			if e != nil && e.id == id {
				entries[i] = nil
				return
			}
		}
	}
	for {
		records, err := s.recBatch.Wait(s.ctx)
		if err != nil {
			return
		}
		if len(records) == 0 {
			return
		}
		for _, rec := range records {
			id := rec.Details.GetString(bundle.RelationKeyId)
			// nil previous version
			nilIfExists(id)
			entries = append(entries, newEntry(id, rec.Details))
		}
		// filter nil entries
		filtered := entries[:0]
		for _, e := range entries {
			if e != nil {
				filtered = append(filtered, e)
			}
		}
		log.Debugf("batch rewrite: %d->%d", len(entries), len(filtered))
		if s.onChange(filtered) < batchTime {
			time.Sleep(batchTime)
		}
		entries = entries[:0]
	}
}

func (s *spaceSubscriptions) onChange(entries []*entry) time.Duration {
	s.m.Lock()
	defer s.m.Unlock()

	return s.onChangeWithinContext(entries, func(ctxBuf *opCtx) {
		s.iterateSubscriptions(func(sub subscription) {
			sub.onChange(s.ctxBuf)
			if sub.hasDep() {
				sub.getDep().onChange(s.ctxBuf)
			}
		})
	})
}

func (s *spaceSubscriptions) onChangeWithinContext(entries []*entry, proc func(ctxBuf *opCtx)) time.Duration {
	st := time.Now()
	s.ctxBuf.reset()
	s.ctxBuf.entries = entries

	proc(s.ctxBuf)

	// Reset output buffer
	for subId := range s.ctxBuf.outputs {
		if subId == defaultOutput {
			s.ctxBuf.outputs[subId] = nil
		} else if _, ok := s.customOutput[subId]; ok {
			s.ctxBuf.outputs[subId] = nil
		} else {
			delete(s.ctxBuf.outputs, subId)
		}
	}
	for subId := range s.customOutput {
		if _, ok := s.ctxBuf.outputs[subId]; !ok {
			s.ctxBuf.outputs[subId] = nil
		}
	}

	s.ctxBuf.apply()

	dur := time.Since(st)

	for id, msgs := range s.ctxBuf.outputs {
		if len(msgs) > 0 {
			s.debugEvents(&pb.Event{Messages: msgs})
			if id == defaultOutput {
				s.eventSender.Broadcast(&pb.Event{Messages: msgs})
			} else {
				err := s.customOutput[id].add(msgs...)
				if err != nil && !errors.Is(err, mb.ErrClosed) {
					log.With("subId", id, "error", err).Errorf("push to output")
				}
			}
		}
	}

	return dur
}

func (s *spaceSubscriptions) filtersFromSource(spaceId string, sources []string) (database.Filter, error) {
	var relTypeFilter database.FiltersOr
	var (
		relKeys        []string
		typeUniqueKeys []string
	)

	var err error
	for _, source := range sources {
		var uk domain.UniqueKey
		if uk, err = domain.UnmarshalUniqueKey(source); err != nil {
			// todo: gradually escalate to return error
			log.Info("Using object id instead of uniqueKey is deprecated in the Source")
			uk, err = s.objectStore.GetUniqueKeyById(source)
			if err != nil {
				return nil, err
			}
		}
		switch uk.SmartblockType() {
		case smartblock.SmartBlockTypeRelation:
			relKeys = append(relKeys, uk.InternalKey())
		case smartblock.SmartBlockTypeObjectType:
			typeUniqueKeys = append(typeUniqueKeys, uk.Marshal())
		}
	}

	if len(typeUniqueKeys) > 0 {
		nestedFiler, err := database.MakeFilter("", database.FilterRequest{
			RelationKey: database.NestedRelationKey(bundle.RelationKeyType, bundle.RelationKeyUniqueKey),
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.StringList(typeUniqueKeys),
		}, s.objectStore)
		if err != nil {
			return nil, fmt.Errorf("make nested filter: %w", err)
		}
		relTypeFilter = append(relTypeFilter, nestedFiler)
	}

	for _, relKey := range relKeys {
		relTypeFilter = append(relTypeFilter, database.FilterExists{
			Key: domain.RelationKey(relKey),
		})
	}
	return relTypeFilter, nil
}

func (s *spaceSubscriptions) depIdsFromFilter(spaceId string, filters []database.FilterRequest) (depIds []string) {
	for _, f := range filters {
		if s.ds.isRelationObject(spaceId, f.RelationKey) {
			for _, id := range f.Value.StringList() {
				if slice.FindPos(depIds, id) == -1 && id != "" {
					depIds = append(depIds, id)
				}
			}
		}
	}
	return
}

func (s *spaceSubscriptions) Close(ctx context.Context) (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	if s.cancelCtx != nil {
		s.cancelCtx()
	}
	s.recBatch.Close()
	for subId, sub := range s.subscriptions {
		sub.close()
		delete(s.subscriptions, subId)
	}
	s.subscriptionKeys = s.subscriptionKeys[:0]
	return
}
