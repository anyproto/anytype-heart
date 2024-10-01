package subscription

import (
	"fmt"
	"sync"

	"github.com/anyproto/any-store/query"
	"github.com/cheggaaa/mb"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

type collectionObserver struct {
	spaceId      string
	collectionID string
	subID        string
	objectsCh    <-chan []string

	lock   *sync.RWMutex
	ids    []string
	idsSet map[string]struct{}

	closeCh chan struct{}

	cache             *cache
	objectStore       objectstore.ObjectStore
	collectionService CollectionService
	recBatch          *mb.MB
}

func (s *service) newCollectionObserver(spaceId string, collectionID string, subID string) (*collectionObserver, error) {
	initialObjectIDs, objectsCh, err := s.collectionService.SubscribeForCollection(collectionID, subID)
	if err != nil {
		return nil, fmt.Errorf("subscribe for collection: %w", err)
	}

	obs := &collectionObserver{
		spaceId:      spaceId,
		collectionID: collectionID,
		subID:        subID,
		objectsCh:    objectsCh,
		lock:         &sync.RWMutex{},
		closeCh:      make(chan struct{}),

		cache:             s.cache,
		objectStore:       s.objectStore,
		recBatch:          s.recBatch,
		collectionService: s.collectionService,

		idsSet: map[string]struct{}{},
	}
	obs.ids = initialObjectIDs
	for _, id := range initialObjectIDs {
		obs.idsSet[id] = struct{}{}
	}

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

func (c *collectionObserver) close() {
	close(c.closeCh)
	// Deplete the channel to avoid deadlock in collections service, because broadcasting to a channel and
	// unsubscribing are synchronous between each other
	go func() {
		for range c.objectsCh {

		}
	}()
	c.collectionService.UnsubscribeFromCollection(c.collectionID, c.subID)
}

func (c *collectionObserver) listEntries() []*entry {
	c.lock.RLock()
	defer c.lock.RUnlock()
	entries := c.fetchEntries(c.ids)
	res := make([]*entry, len(entries))
	copy(res, entries)
	return res
}

func (c *collectionObserver) updateIDs(ids []string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	removed, added := slice.DifferenceRemovedAdded(c.ids, ids)
	for _, id := range removed {
		delete(c.idsSet, id)
	}
	for _, id := range added {
		c.idsSet[id] = struct{}{}
	}
	c.ids = ids

	entries := c.fetchEntries(append(removed, added...))
	for _, e := range entries {
		err := c.recBatch.Add(database.Record{
			Details: e.data,
		})
		if err != nil {
			log.Info("failed to add entities to mb: ", err)
		}
	}
}

func (c *collectionObserver) FilterObject(g *types.Struct) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	val := pbtypes.Get(g, bundle.RelationKeyId.String())
	id := val.GetStringValue()
	_, ok := c.idsSet[id]
	return ok
}

// AnystoreSort called only once when subscription is created
func (c *collectionObserver) AnystoreFilter() query.Filter {
	c.lock.RLock()
	defer c.lock.RUnlock()
	path := []string{bundle.RelationKeyId.String()}
	filter := make(query.Or, 0, len(c.idsSet))
	for id := range c.idsSet {
		filter = append(filter, query.Key{
			Path:   path,
			Filter: query.NewComp(query.CompOpEq, id),
		})
	}
	return filter
}

func (c *collectionObserver) String() string {
	return "collectionObserver"
}

type collectionSub struct {
	sortedSub *sortedSub
	observer  *collectionObserver
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

func (c *collectionSub) getDep() subscription {
	return c.sortedSub.depSub
}

func (c *collectionSub) close() {
	c.observer.close()
	c.sortedSub.close()
}

func (s *service) newCollectionSub(id string, spaceId string, collectionID string, keys, filterDepIds []string, flt database.Filter, order database.Order, limit, offset int, disableDepSub bool) (*collectionSub, error) {
	obs, err := s.newCollectionObserver(spaceId, collectionID, id)
	if err != nil {
		return nil, err
	}
	if flt == nil {
		flt = obs
	} else {
		flt = database.FiltersAnd{obs, flt}
	}

	ssub := s.newSortedSub(id, spaceId, keys, flt, order, limit, offset)
	ssub.disableDep = disableDepSub
	if !ssub.disableDep {
		ssub.forceSubIds = filterDepIds
	}

	sub := &collectionSub{
		sortedSub: ssub,
		observer:  obs,
	}

	entries := obs.listEntries()
	filtered := entries[:0]
	for _, e := range entries {
		if flt.FilterObject(e.data) {
			filtered = append(filtered, e)
		}
	}
	if err := ssub.init(filtered); err != nil {
		return nil, err
	}
	return sub, nil
}

func (c *collectionObserver) fetchEntries(ids []string) []*entry {
	res := make([]*entry, 0, len(ids))
	missingIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if e := c.cache.Get(id); e != nil {
			res = append(res, e)
			continue
		}
		missingIDs = append(missingIDs, id)
	}

	if len(missingIDs) == 0 {
		return res
	}
	recs, err := c.objectStore.SpaceStore(c.spaceId).QueryByID(missingIDs)
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
