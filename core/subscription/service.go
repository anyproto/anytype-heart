package subscription

import (
	"fmt"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database/filter"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/cheggaaa/mb"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/ipfs/go-datastore/query"
)

const CName = "subscription"

var log = logging.Logger("anytype-mw-subscription")

var batchTime = 50 * time.Millisecond

func New() Service {
	return new(service)
}

type Service interface {
	Search(req pb.RpcObjectSearchSubscribeRequest) (resp *pb.RpcObjectSearchSubscribeResponse, err error)
	SubscribeIdsReq(req pb.RpcObjectIdsSubscribeRequest) (resp *pb.RpcObjectIdsSubscribeResponse, err error)
	SubscribeIds(subId string, ids []string) (records []*types.Struct, err error)
	Unsubscribe(subIds ...string) (err error)
	UnsubscribeAll() (err error)

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

type service struct {
	cache         *cache
	ds            *dependencyService
	subscriptions map[string]subscription
	recBatch      *mb.MB

	objectStore objectstore.ObjectStore
	sendEvent   func(e *pb.Event)

	m      sync.Mutex
	ctxBuf *opCtx
}

func (s *service) Init(a *app.App) (err error) {
	s.cache = newCache()
	s.ds = newDependencyService(s)
	s.subscriptions = make(map[string]subscription)
	s.objectStore = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	s.recBatch = mb.New(0)
	s.sendEvent = a.MustComponent(event.CName).(event.Sender).Send
	s.ctxBuf = &opCtx{c: s.cache}
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run() (err error) {
	s.objectStore.SubscribeForAll(func(rec database.Record) {
		s.recBatch.Add(rec)
	})
	go s.recordsHandler()
	return
}

func (s *service) Search(req pb.RpcObjectSearchSubscribeRequest) (resp *pb.RpcObjectSearchSubscribeResponse, err error) {
	if req.SubId == "" {
		req.SubId = bson.NewObjectId().Hex()
	}

	q := database.Query{
		Filters: req.Filters,
		Sorts:   req.Sorts,
		Limit:   int(req.Limit),
	}

	f, err := database.NewFilters(q, nil)
	if err != nil {
		return
	}

	filterDepIds := s.depIdsFromFilter(req.Filters)

	if len(req.Source) > 0 {
		sourceFilter, err := s.filtersFromSource(req.Source)
		if err != nil {
			return nil, fmt.Errorf("can't make filter from source: %v", err)
		}
		f.FilterObj = filter.AndFilters{f.FilterObj, sourceFilter}
	}

	records, err := s.objectStore.QueryRaw(query.Query{
		Filters: []query.Filter{f},
	})
	if err != nil {
		return nil, fmt.Errorf("objectStore query error: %v", err)
	}

	s.m.Lock()
	defer s.m.Unlock()
	if exists, ok := s.subscriptions[req.SubId]; ok {
		exists.close()
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
	if req.Limit < 0 {
		req.Limit = 0
	}
	sub := s.newSortedSub(req.SubId, req.Keys, f.FilterObj, f.Order, int(req.Limit), int(req.Offset))
	if req.NoDepSubscription {
		sub.disableDep = true
	} else {
		sub.forceSubIds = filterDepIds
	}
	entries := make([]*entry, 0, len(records))
	for _, r := range records {
		entries = append(entries, &entry{
			id:   pbtypes.GetString(r.Details, "id"),
			data: r.Details,
		})
	}
	if err = sub.init(entries); err != nil {
		return nil, fmt.Errorf("subscription init error: %v", err)
	}
	s.subscriptions[sub.id] = sub
	prev, next := sub.counters()

	var depRecords, subRecords []*types.Struct
	subRecords = sub.getActiveRecords()

	if sub.depSub != nil {
		depRecords = sub.depSub.getActiveRecords()
	}

	resp = &pb.RpcObjectSearchSubscribeResponse{
		Records:      subRecords,
		Dependencies: depRecords,
		SubId:        sub.id,
		Counters: &pb.EventObjectSubscriptionCounters{
			Total:     int64(sub.skl.Len()),
			NextCount: int64(prev),
			PrevCount: int64(next),
		},
	}
	return
}

func (s *service) SubscribeIdsReq(req pb.RpcObjectIdsSubscribeRequest) (resp *pb.RpcObjectIdsSubscribeResponse, err error) {
	records, err := s.objectStore.QueryById(req.Ids)
	if err != nil {
		return
	}

	if req.SubId == "" {
		req.SubId = bson.NewObjectId().Hex()
	}

	s.m.Lock()
	defer s.m.Unlock()

	sub := s.newSimpleSub(req.SubId, req.Keys, false)

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

	return &pb.RpcObjectIdsSubscribeResponse{
		Error:        &pb.RpcObjectIdsSubscribeResponseError{},
		Records:      subRecords,
		Dependencies: depRecords,
		SubId:        req.SubId,
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

	log.Debugf("handle %d entries; %v(handle:%v;genEvents:%v); cacheSize: %d; subCount:%d; subDepCount:%d", len(entries), dur, handleTime, dur-handleTime, len(s.cache.entries), subCount, depCount)
	s.sendEvent(event)
	return dur
}

func (s *service) filtersFromSource(sources []string) (filter.Filter, error) {
	var objTypeIds, relTypeKeys []string

	for _, source := range sources {
		sbt, err := smartblock.SmartBlockTypeFromID(source)
		if err != nil {
			return nil, err
		}
		if sbt == smartblock.SmartBlockTypeObjectType || sbt == smartblock.SmartBlockTypeBundledObjectType {
			objTypeIds = append(objTypeIds, source)
		} else {
			relKey, err := pbtypes.RelationIdToKey(source)
			if err != nil {
				return nil, fmt.Errorf("failed to get relation key from id %s: %s", relKey, err.Error())
			}
			relTypeKeys = append(relTypeKeys, relKey)
		}
	}

	var relTypeFilter filter.OrFilters

	if len(objTypeIds) > 0 {
		relTypeFilter = append(relTypeFilter, filter.In{
			Key:   bundle.RelationKeyType.String(),
			Value: pbtypes.StringList(objTypeIds).GetListValue(),
		})
	}

	for _, key := range relTypeKeys {
		relTypeFilter = append(relTypeFilter, filter.Exists{
			Key: key,
		})
	}
	return relTypeFilter, nil
}

func (s *service) depIdsFromFilter(filters []*model.BlockContentDataviewFilter) (depIds []string) {
	for _, f := range filters {
		if s.ds.isRelationObject(f.RelationKey) {
			for _, id := range pbtypes.GetStringListValue(f.Value) {
				if slice.FindPos(depIds, id) == -1 {
					depIds = append(depIds, id)
				}
			}
		}
	}
	return
}

func (s *service) Close() (err error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.recBatch.Close()
	for _, sub := range s.subscriptions {
		sub.close()
	}
	return
}
