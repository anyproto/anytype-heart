package subscription

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/domain"

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

type Service interface {
	Search(req pb.RpcObjectSearchSubscribeRequest) (resp *pb.RpcObjectSearchSubscribeResponse, err error)
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
	close()
}

type CollectionService interface {
	SubscribeForCollection(collectionID string, subscriptionID string) ([]string, <-chan []string, error)
	UnsubscribeFromCollection(collectionID string, subscriptionID string)
}

type service struct {
	cache         *cache
	ds            *dependencyService
	subscriptions map[string]subscription
	recBatch      *mb.MB

	objectStore       objectstore.ObjectStore
	kanban            kanban.Service
	collectionService CollectionService
	eventSender       event.Sender

	m      sync.Mutex
	ctxBuf *opCtx

	subDebugger *subDebugger
}

func (s *service) Init(a *app.App) (err error) {
	s.cache = newCache()
	s.ds = newDependencyService(s)
	s.subscriptions = make(map[string]subscription)
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.kanban = a.MustComponent(kanban.CName).(kanban.Service)
	s.recBatch = mb.New(0)
	s.collectionService = app.MustComponent[CollectionService](a)
	s.eventSender = a.MustComponent(event.CName).(event.Sender)
	s.ctxBuf = &opCtx{c: s.cache}
	s.initDebugger()
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

func (s *service) Search(req pb.RpcObjectSearchSubscribeRequest) (*pb.RpcObjectSearchSubscribeResponse, error) {
	if req.SubId == "" {
		req.SubId = bson.NewObjectId().Hex()
	}

	q := database.Query{
		Filters: req.Filters,
		Sorts:   req.Sorts,
		Limit:   int(req.Limit),
	}

	f, err := database.NewFilters(q, s.objectStore)
	if err != nil {
		return nil, fmt.Errorf("new database filters: %w", err)
	}

	if len(req.Source) > 0 {
		sourceFilter, err := s.filtersFromSource(req.Source)
		if err != nil {
			return nil, fmt.Errorf("can't make filter from source: %w", err)
		}
		f.FilterObj = database.FiltersAnd{f.FilterObj, sourceFilter}
	}

	s.m.Lock()
	defer s.m.Unlock()

	filterDepIds := s.depIdsFromFilter(req.Filters)
	if exists, ok := s.subscriptions[req.SubId]; ok {
		delete(s.subscriptions, req.SubId)
		exists.close()
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

func (s *service) subscribeForQuery(req pb.RpcObjectSearchSubscribeRequest, f *database.Filters, filterDepIds []string) (*pb.RpcObjectSearchSubscribeResponse, error) {
	sub := s.newSortedSub(req.SubId, req.Keys, f.FilterObj, f.Order, int(req.Limit), int(req.Offset))
	if req.NoDepSubscription {
		sub.disableDep = true
	} else {
		sub.forceSubIds = filterDepIds
	}

	// FIXME Nested subscriptions disabled now. We should enable them only by client's request
	// Uncomment test xTestNestedSubscription after enabling this
	if withNested, ok := f.FilterObj.(database.WithNestedFilter); ok && false {
		var nestedCount int
		err := withNested.IterateNestedFilters(func(nestedFilter database.Filter) error {
			nestedCount++
			f, ok := nestedFilter.(*database.FilterNestedIn)
			if ok {
				childSub := s.newSortedSub(req.SubId+fmt.Sprintf("-nested-%d", nestedCount), []string{"id"}, f.FilterForNestedObjects, nil, 0, 0)
				err := initSubEntries(s.objectStore, &database.Filters{FilterObj: f.FilterForNestedObjects}, childSub)
				if err != nil {
					return fmt.Errorf("init nested sub %s entries: %w", childSub.id, err)
				}
				sub.nested = append(sub.nested, childSub)
				childSub.parent = sub
				childSub.parentFilter = f
				s.subscriptions[childSub.id] = childSub
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("iterate nested filters: %w", err)
		}
	}

	err := initSubEntries(s.objectStore, f, sub)
	if err != nil {
		return nil, fmt.Errorf("init sub entries: %w", err)
	}
	s.subscriptions[sub.id] = sub
	prev, next := sub.counters()

	var depRecords, subRecords []*types.Struct
	subRecords = sub.getActiveRecords()

	if sub.depSub != nil {
		depRecords = sub.depSub.getActiveRecords()
	}

	return &pb.RpcObjectSearchSubscribeResponse{
		Records:      subRecords,
		Dependencies: depRecords,
		SubId:        sub.id,
		Counters: &pb.EventObjectSubscriptionCounters{
			Total:     int64(sub.skl.Len()),
			NextCount: int64(prev),
			PrevCount: int64(next),
		},
	}, nil
}

func initSubEntries(objectStore objectstore.ObjectStore, f *database.Filters, sub *sortedSub) error {
	entries, err := queryEntries(objectStore, f)
	if err != nil {
		return err
	}
	if err = sub.init(entries); err != nil {
		return fmt.Errorf("subscription init error: %w", err)
	}
	return nil
}

func queryEntries(objectStore objectstore.ObjectStore, f *database.Filters) ([]*entry, error) {
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

func (s *service) subscribeForCollection(req pb.RpcObjectSearchSubscribeRequest, f *database.Filters, filterDepIds []string) (*pb.RpcObjectSearchSubscribeResponse, error) {
	sub, err := s.newCollectionSub(req.SubId, req.CollectionId, req.Keys, filterDepIds, f.FilterObj, f.Order, int(req.Limit), int(req.Offset), req.NoDepSubscription)
	if err != nil {
		return nil, err
	}
	if err := sub.init(nil); err != nil {
		return nil, fmt.Errorf("subscription init error: %w", err)
	}
	s.subscriptions[sub.sortedSub.id] = sub
	prev, next := sub.counters()

	var depRecords, subRecords []*types.Struct
	subRecords = sub.getActiveRecords()

	if sub.sortedSub.depSub != nil && !sub.sortedSub.disableDep {
		depRecords = sub.sortedSub.depSub.getActiveRecords()
	}

	return &pb.RpcObjectSearchSubscribeResponse{
		Records:      subRecords,
		Dependencies: depRecords,
		SubId:        sub.sortedSub.id,
		Counters: &pb.EventObjectSubscriptionCounters{
			Total:     int64(sub.sortedSub.skl.Len()),
			NextCount: int64(prev),
			PrevCount: int64(next),
		},
	}, nil
}

func (s *service) SubscribeIdsReq(req pb.RpcObjectSubscribeIdsRequest) (resp *pb.RpcObjectSubscribeIdsResponse, err error) {
	records, err := s.objectStore.QueryByID(req.Ids)
	if err != nil {
		return
	}

	if req.SubId == "" {
		req.SubId = bson.NewObjectId().Hex()
	}

	s.m.Lock()
	defer s.m.Unlock()

	sub := s.newSimpleSub(req.SubId, req.Keys, !req.NoDepSubscription)
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
	s.subscriptions[sub.id] = sub

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

	flt, err := database.NewFilters(q, s.objectStore)
	if err != nil {
		return nil, err
	}

	if len(req.Source) > 0 {
		sourceFilter, err := s.filtersFromSource(req.Source)
		if err != nil {
			return nil, fmt.Errorf("can't make filter from source: %w", err)
		}
		flt.FilterObj = database.FiltersAnd{flt.FilterObj, sourceFilter}
	}

	var colObserver *collectionObserver
	if req.CollectionId != "" {
		colObserver, err = s.newCollectionObserver(req.CollectionId, req.SubId)
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
		s.subscriptions[subId] = sub
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

func (s *service) Unsubscribe(subIds ...string) (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	for _, subId := range subIds {
		if sub, ok := s.subscriptions[subId]; ok {
			sub.close()
			delete(s.subscriptions, subId)
		}
	}
	return
}

func (s *service) UnsubscribeAll() (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	for _, sub := range s.subscriptions {
		sub.close()
	}
	s.subscriptions = make(map[string]subscription)
	return
}

func (s *service) SubscriptionIDs() []string {
	s.m.Lock()
	defer s.m.Unlock()
	return lo.Keys(s.subscriptions)
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
	for _, sub := range s.subscriptions {
		sub.onChange(s.ctxBuf)
		subCount++
		if sub.hasDep() {
			depCount++
		}
	}
	handleTime := time.Since(st)
	event := s.ctxBuf.apply()
	dur := time.Since(st)

	s.debugEvents(event)

	log.Debugf("handle %d entries; %v(handle:%v;genEvents:%v); cacheSize: %d; subCount:%d; subDepCount:%d", len(entries), dur, handleTime, dur-handleTime, len(s.cache.entries), subCount, depCount)
	s.eventSender.Broadcast(event)
	return dur
}

func (s *service) filtersFromSource(sources []string) (database.Filter, error) {
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
		nestedFiler, err := database.MakeFilter("",
			&model.BlockContentDataviewFilter{
				RelationKey: database.NestedRelationKey(bundle.RelationKeyType, bundle.RelationKeyUniqueKey),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(typeUniqueKeys),
			},
			s.objectStore,
		)
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

func (s *service) depIdsFromFilter(filters []*model.BlockContentDataviewFilter) (depIds []string) {
	for _, f := range filters {
		if s.ds.isRelationObject(f.RelationKey) {
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
	for _, sub := range s.subscriptions {
		sub.close()
	}
	return
}
