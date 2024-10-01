package subscription

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb"
	mb2 "github.com/cheggaaa/mb/v3"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/valyala/fastjson"
	"golang.org/x/exp/slices"
	"golang.org/x/text/collate"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

const CName = "subscription"

var log = logging.Logger("anytype-mw-subscription")

var batchTime = 50 * time.Millisecond

func New() Service {
	return &service{}
}

type SubscribeRequest struct {
	SpaceId string
	SubId   string
	Filters []*model.BlockContentDataviewFilter
	Sorts   []*model.BlockContentDataviewSort
	Limit   int64
	Offset  int64
	// (required)  needed keys in details for return, for object fields mw will return (and subscribe) objects as dependent
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
}

type SubscribeResponse struct {
	SubId        string
	Records      []*types.Struct
	Dependencies []*types.Struct
	Counters     *pb.EventObjectSubscriptionCounters

	// Used when Internal flag is set to true
	Output *mb2.MB[*pb.EventMessage]
}

type Service interface {
	Search(req SubscribeRequest) (resp *SubscribeResponse, err error)
	SubscribeIdsReq(req pb.RpcObjectSubscribeIdsRequest) (resp *pb.RpcObjectSubscribeIdsResponse, err error)
	SubscribeIds(subId string, ids []string) (records []*types.Struct, err error)
	SubscribeGroups(ctx session.Context, req pb.RpcObjectGroupsSubscribeRequest) (*pb.RpcObjectGroupsSubscribeResponse, error)
	Unsubscribe(subIds ...string) (err error)
	UnsubscribeAll() (err error)
	SubscriptionIDs() []string

	app.ComponentRunnable
}

type subscription interface {
	init(entries []*entry) (err error)
	counters() (prev, next int)
	onChange(ctx *opCtx)
	getActiveRecords() (res []*types.Struct)
	hasDep() bool
	getDep() subscription
	close()
}

type CollectionService interface {
	SubscribeForCollection(collectionID string, subscriptionID string) ([]string, <-chan []string, error)
	UnsubscribeFromCollection(collectionID string, subscriptionID string)
}

type service struct {
	cache *cache
	ds    *dependencyService

	subscriptionKeys []string
	subscriptions    map[string]subscription

	customOutput map[string]*mb2.MB[*pb.EventMessage]
	recBatch     *mb.MB

	objectStore       objectstore.ObjectStore
	kanban            kanban.Service
	collectionService CollectionService
	eventSender       event.Sender

	m      sync.Mutex
	ctxBuf *opCtx

	subDebugger *subDebugger
	arenaPool   *fastjson.ArenaPool
}

func (s *service) Init(a *app.App) (err error) {
	s.cache = newCache()
	s.ds = newDependencyService(s)
	s.subscriptions = make(map[string]subscription)
	s.customOutput = map[string]*mb2.MB[*pb.EventMessage]{}
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.kanban = app.MustComponent[kanban.Service](a)
	s.recBatch = mb.New(0)
	s.collectionService = app.MustComponent[CollectionService](a)
	s.eventSender = app.MustComponent[event.Sender](a)
	s.ctxBuf = &opCtx{c: s.cache}
	s.initDebugger()
	s.arenaPool = &fastjson.ArenaPool{}
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(context.Context) (err error) {
	s.objectStore.SubscribeForAll(func(rec database.Record) {
		s.recBatch.Add(rec)
	})
	go s.recordsHandler()
	return
}

func (s *service) getSubscription(id string) (subscription, bool) {
	sub, ok := s.subscriptions[id]
	return sub, ok
}

func (s *service) setSubscription(id string, sub subscription) {
	s.subscriptions[id] = sub
	if !slices.Contains(s.subscriptionKeys, id) {
		s.subscriptionKeys = append(s.subscriptionKeys, id)
	}
}

func (s *service) deleteSubscription(id string) {
	delete(s.subscriptions, id)
	s.subscriptionKeys = slice.RemoveMut(s.subscriptionKeys, id)
}

func (s *service) iterateSubscriptions(proc func(sub subscription)) {
	for _, subId := range s.subscriptionKeys {
		sub, ok := s.getSubscription(subId)
		if ok && sub != nil {
			proc(sub)
		}
	}
}

func (s *service) Search(req SubscribeRequest) (*SubscribeResponse, error) {
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

	arena := s.arenaPool.Get()
	defer s.arenaPool.Put(arena)

	f, err := database.NewFilters(q, s.objectStore.SpaceIndex(req.SpaceId), arena, &collate.Buffer{})
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
	return s.subscribeForQuery(req, f, filterDepIds)
}

func (s *service) subscribeForQuery(req SubscribeRequest, f *database.Filters, filterDepIds []string) (*SubscribeResponse, error) {
	sub := s.newSortedSub(req.SubId, req.SpaceId, req.Keys, f.FilterObj, f.Order, int(req.Limit), int(req.Offset))
	if req.NoDepSubscription {
		sub.disableDep = true
	} else {
		sub.forceSubIds = filterDepIds
	}

	store := s.objectStore.SpaceIndex(req.SpaceId)
	// FIXME Nested subscriptions disabled now. We should enable them only by client's request
	// Uncomment test xTestNestedSubscription after enabling this
	if withNested, ok := f.FilterObj.(database.WithNestedFilter); ok && false {
		var nestedCount int
		err := withNested.IterateNestedFilters(func(nestedFilter database.Filter) error {
			nestedCount++
			f, ok := nestedFilter.(*database.FilterNestedIn)
			if ok {
				childSub := s.newSortedSub(req.SubId+fmt.Sprintf("-nested-%d", nestedCount), req.SpaceId, []string{"id"}, f.FilterForNestedObjects, nil, 0, 0)
				err := initSubEntries(store, &database.Filters{FilterObj: f.FilterForNestedObjects}, childSub)
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

	err := initSubEntries(store, f, sub)
	if err != nil {
		return nil, fmt.Errorf("init sub entries: %w", err)
	}
	s.setSubscription(sub.id, sub)
	prev, next := sub.counters()

	var depRecords, subRecords []*types.Struct
	subRecords = sub.getActiveRecords()

	if sub.depSub != nil {
		depRecords = sub.depSub.getActiveRecords()
	}
	if req.Internal {
		s.customOutput[req.SubId] = mb2.New[*pb.EventMessage](0)
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
		Output: s.customOutput[req.SubId],
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
		entries = append(entries, &entry{
			id:   pbtypes.GetString(r.Details, "id"),
			data: r.Details,
		})
	}
	return entries, nil
}

func (s *service) subscribeForCollection(req SubscribeRequest, f *database.Filters, filterDepIds []string) (*SubscribeResponse, error) {
	sub, err := s.newCollectionSub(req.SubId, req.SpaceId, req.CollectionId, req.Keys, filterDepIds, f.FilterObj, f.Order, int(req.Limit), int(req.Offset), req.NoDepSubscription)
	if err != nil {
		return nil, err
	}
	if err := sub.init(nil); err != nil {
		return nil, fmt.Errorf("subscription init error: %w", err)
	}
	s.setSubscription(sub.sortedSub.id, sub)
	prev, next := sub.counters()

	var depRecords, subRecords []*types.Struct
	subRecords = sub.getActiveRecords()

	if sub.sortedSub.depSub != nil && !sub.sortedSub.disableDep {
		depRecords = sub.sortedSub.depSub.getActiveRecords()
	}

	if req.Internal {
		s.customOutput[req.SubId] = mb2.New[*pb.EventMessage](0)
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
		Output: s.customOutput[req.SubId],
	}, nil
}

func (s *service) SubscribeIdsReq(req pb.RpcObjectSubscribeIdsRequest) (resp *pb.RpcObjectSubscribeIdsResponse, err error) {
	if req.SpaceId == "" {
		return nil, fmt.Errorf("spaceId is required")
	}
	records, err := s.objectStore.SpaceIndex(req.SpaceId).QueryByID(req.Ids)
	if err != nil {
		return
	}

	if req.SubId == "" {
		req.SubId = bson.NewObjectId().Hex()
	}

	s.m.Lock()
	defer s.m.Unlock()

	sub := s.newSimpleSub(req.SubId, req.SpaceId, req.Keys, !req.NoDepSubscription)
	entries := make([]*entry, 0, len(records))
	for _, r := range records {
		entries = append(entries, &entry{
			id:   pbtypes.GetString(r.Details, "id"),
			data: r.Details,
		})
	}
	if err = sub.init(entries); err != nil {
		return
	}
	s.setSubscription(sub.id, sub)

	var depRecords, subRecords []*types.Struct
	subRecords = sub.getActiveRecords()

	if sub.depSub != nil {
		depRecords = sub.depSub.getActiveRecords()
	}

	return &pb.RpcObjectSubscribeIdsResponse{
		Error:        &pb.RpcObjectSubscribeIdsResponseError{},
		Records:      subRecords,
		Dependencies: depRecords,
		SubId:        req.SubId,
	}, nil
}

func (s *service) SubscribeGroups(ctx session.Context, req pb.RpcObjectGroupsSubscribeRequest) (*pb.RpcObjectGroupsSubscribeResponse, error) {
	subId := ""

	s.m.Lock()
	defer s.m.Unlock()

	q := database.Query{
		Filters: req.Filters,
	}

	arena := s.arenaPool.Get()
	defer s.arenaPool.Put(arena)

	flt, err := database.NewFilters(q, s.objectStore.SpaceIndex(req.SpaceId), arena, &collate.Buffer{})
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

		var sub subscription
		if colObserver != nil {
			sub = s.newCollectionGroupSub(subId, req.RelationKey, flt, groups, colObserver)
		} else {
			sub = s.newGroupSub(subId, req.RelationKey, flt, groups)
		}

		entries := make([]*entry, 0, len(tagGrouper.Records))
		for _, r := range tagGrouper.Records {
			entries = append(entries, &entry{
				id:   pbtypes.GetString(r.Details, "id"),
				data: r.Details,
			})
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

func (s *service) SubscribeIds(subId string, ids []string) (records []*types.Struct, err error) {
	return
}

func (s *service) Unsubscribe(subIds ...string) error {
	s.m.Lock()
	defer s.m.Unlock()
	for _, subId := range subIds {
		if sub, ok := s.getSubscription(subId); ok {
			out := s.customOutput[subId]
			if out != nil {
				err := out.Close()
				if err != nil {
					return fmt.Errorf("close subscription %s: %w", subId, err)
				}
				s.customOutput[subId] = nil
			}
			sub.close()
			s.deleteSubscription(subId)
		}
	}
	return nil
}

func (s *service) UnsubscribeAll() (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	for _, sub := range s.subscriptions {
		sub.close()
	}
	s.subscriptions = make(map[string]subscription)
	s.subscriptionKeys = s.subscriptionKeys[:0]
	return
}

func (s *service) SubscriptionIDs() []string {
	s.m.Lock()
	defer s.m.Unlock()
	return s.subscriptionKeys
}

func (s *service) recordsHandler() {
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
		records := s.recBatch.Wait()
		if len(records) == 0 {
			return
		}
		for _, rec := range records {
			id := pbtypes.GetString(rec.(database.Record).Details, "id")
			// nil previous version
			nilIfExists(id)
			entries = append(entries, &entry{
				id:   id,
				data: rec.(database.Record).Details,
			})
		}
		// filter nil entries
		var filtered = entries[:0]
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

func (s *service) onChange(entries []*entry) time.Duration {
	s.m.Lock()
	defer s.m.Unlock()
	var subCount, depCount int
	st := time.Now()
	s.ctxBuf.reset()
	s.ctxBuf.entries = entries
	s.iterateSubscriptions(func(sub subscription) {
		sub.onChange(s.ctxBuf)
		subCount++
		if sub.hasDep() {
			sub.getDep().onChange(s.ctxBuf)
			depCount++
		}
	})
	handleTime := time.Since(st)

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
				err := s.customOutput[id].Add(context.TODO(), msgs...)
				if err != nil && !errors.Is(err, mb2.ErrClosed) {
					log.With("subId", id, "error", err).Errorf("push to output")
				}
			}
		}
	}

	log.Debugf("handle %d entries; %v(handle:%v;genEvents:%v); cacheSize: %d; subCount:%d; subDepCount:%d", len(entries), dur, handleTime, dur-handleTime, len(s.cache.entries), subCount, depCount)

	return dur
}

func (s *service) filtersFromSource(spaceId string, sources []string) (database.Filter, error) {
	var relTypeFilter database.FiltersOr
	var (
		relKeys        []string
		typeUniqueKeys []string
	)

	store := s.objectStore.SpaceIndex(spaceId)
	var err error
	for _, source := range sources {
		var uk domain.UniqueKey
		if uk, err = domain.UnmarshalUniqueKey(source); err != nil {
			// todo: gradually escalate to return error
			log.Info("Using object id instead of uniqueKey is deprecated in the Source")
			uk, err = store.GetUniqueKeyById(source)
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
		nestedFiler, err := database.MakeFilter("", &model.BlockContentDataviewFilter{
			RelationKey: database.NestedRelationKey(bundle.RelationKeyType, bundle.RelationKeyUniqueKey),
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       pbtypes.StringList(typeUniqueKeys),
		}, store)
		if err != nil {
			return nil, fmt.Errorf("make nested filter: %w", err)
		}
		relTypeFilter = append(relTypeFilter, nestedFiler)
	}

	for _, relKey := range relKeys {
		relTypeFilter = append(relTypeFilter, database.FilterExists{
			Key: relKey,
		})
	}
	return relTypeFilter, nil
}

func (s *service) depIdsFromFilter(spaceId string, filters []*model.BlockContentDataviewFilter) (depIds []string) {
	for _, f := range filters {
		if s.ds.isRelationObject(spaceId, f.RelationKey) {
			for _, id := range pbtypes.GetStringListValue(f.Value) {
				if slice.FindPos(depIds, id) == -1 && id != "" {
					depIds = append(depIds, id)
				}
			}
		}
	}
	return
}

func (s *service) Close(ctx context.Context) (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.recBatch.Close()
	s.iterateSubscriptions(func(sub subscription) {
		sub.close()
	})
	return
}
