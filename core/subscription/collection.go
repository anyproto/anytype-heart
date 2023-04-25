package subscription

import (
	"fmt"
	"sync"

	"github.com/cheggaaa/mb"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database/filter"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type collectionObserver struct {
	lock     *sync.RWMutex
	ids      []string
	idsIndex map[string]int

	closeCh chan struct{}

	cache       *cache
	objectStore objectstore.ObjectStore
	recBatch    *mb.MB
}

func (s *service) newCollectionObserver(collectionID string, subID string) (*collectionObserver, error) {
	initialObjectIDs, objectsCh, err := s.collectionService.SubscribeForCollection(collectionID, subID)
	if err != nil {
		return nil, fmt.Errorf("subscribe for collection: %w", err)
	}

	obs := &collectionObserver{
		lock:    &sync.RWMutex{},
		closeCh: make(chan struct{}),

		cache:       s.cache,
		objectStore: s.objectStore,
		recBatch:    s.recBatch,
	}
	obs.setIDs(initialObjectIDs)

	go func() {
		for {
			select {
			case objectIDs := <-objectsCh:
				obs.updateIDs(objectIDs)
			case <-obs.closeCh:
				return
			}
		}
	}()

	return obs, nil
}

func (c *collectionObserver) setIDs(ids []string) {
	c.ids = ids
	c.idsIndex = map[string]int{}

	for i, id := range ids {
		c.idsIndex[id] = i
	}
}

func (c *collectionObserver) close() {
	close(c.closeCh)
}

func (c *collectionObserver) listEntries() []*entry {
	c.lock.RLock()
	defer c.lock.RUnlock()
	entries := fetchEntries(c.cache, c.objectStore, c.ids)
	res := make([]*entry, len(entries))
	copy(res, entries)
	return res
}

func (c *collectionObserver) updateIDs(ids []string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.setIDs(ids)

	entries := fetchEntries(c.cache, c.objectStore, c.ids)
	for _, e := range entries {
		c.recBatch.Add(database.Record{
			Details: e.data,
		})
	}
}

func (c *collectionObserver) Compare(a, b filter.Getter) int {
	c.lock.RLock()
	defer c.lock.RUnlock()

	ae, be := a.(*entry), b.(*entry)
	ap, bp := c.idsIndex[ae.id], c.idsIndex[be.id]
	if ap == bp {
		return 0
	}
	if ap < bp {
		return -1
	}
	return 1
}

func (c *collectionObserver) FilterObject(g filter.Getter) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	_, ok := c.idsIndex[g.(*entry).id]
	return ok
}

func (c *collectionObserver) String() string {
	return "collectionObserver"
}

type collectionSub struct {
	id           string
	collectionID string

	sortedSub         *sortedSub
	observer          *collectionObserver
	collectionService CollectionService
}

func (c *collectionSub) init(entries []*entry) (err error) {
	return nil
}

func (c *collectionSub) counters() (prev, next int) {
	return c.sortedSub.counters()
}

func (c *collectionSub) onChange(ctx *opCtx) {
	c.sortedSub.onChange(ctx)
}

func (c *collectionSub) getActiveRecords() (res []*types.Struct) {
	return c.sortedSub.getActiveRecords()
}

func (c *collectionSub) hasDep() bool {
	return c.sortedSub.hasDep()
}

func (c *collectionSub) close() {
	c.observer.close()
	c.sortedSub.close()
	c.collectionService.UnsubscribeFromCollection(c.collectionID, c.sortedSub.id)
}

func (s *service) newCollectionSub(id string, collectionID string, keys []string, flt filter.Filter, order filter.Order, limit, offset int) (*collectionSub, error) {
	obs, err := s.newCollectionObserver(collectionID, id)
	if err != nil {
		return nil, err
	}
	if flt == nil {
		flt = obs
	} else {
		flt = filter.AndFilters{obs, flt}
	}

	var orderFromCollection bool
	if order == nil {
		// Take an order from collection
		order = obs
		orderFromCollection = true
	}
	ssub := s.newSortedSub(id, keys, flt, order, limit, offset)
	if orderFromCollection {
		ssub.batchUpdate = true
	}

	sub := &collectionSub{
		id:           id,
		collectionID: collectionID,

		sortedSub:         ssub,
		observer:          obs,
		collectionService: s.collectionService,
	}

	if err := ssub.init(obs.listEntries()); err != nil {
		return nil, err
	}
	return sub, nil
}

func fetchEntries(cache *cache, objectStore objectstore.ObjectStore, ids []string) []*entry {
	res := make([]*entry, 0, len(ids))
	var missingIDs []string
	for _, id := range ids {
		if e := cache.Get(id); e != nil {
			res = append(res, e)
			continue
		}
		missingIDs = append(missingIDs, id)
	}

	if len(missingIDs) == 0 {
		return res
	}
	recs, err := objectStore.QueryById(missingIDs)
	if err != nil {
		log.Error("can't query by ids:", err)
	}
	for _, r := range recs {
		e := &entry{
			id:   pbtypes.GetString(r.Details, bundle.RelationKeyId.String()),
			data: r.Details,
		}
		res = append(res, e)
	}
	return res
}
