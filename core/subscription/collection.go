package subscription

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
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
	objectStore       spaceindex.Store
	collectionService CollectionService
	recBatch          *mb.MB[database.Record]
	recBatchMutex     sync.Mutex

	spaceSubscription *spaceSubscriptions
}

func (s *spaceSubscriptions) newCollectionObserver(spaceId string, collectionID string, subID string) (*collectionObserver, error) {
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

		spaceSubscription: s,
	}
	obs.ids = initialObjectIDs
	for _, id := range initialObjectIDs {
		obs.idsSet[id] = struct{}{}
	}

	go func() {
		for {
			select {
			case objectIDs := <-objectsCh:
				obs.updateIds(objectIDs)
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
	entries := c.spaceSubscription.fetchEntries(c.ids)
	res := make([]*entry, len(entries))
	copy(res, entries)
	return res
}

// updateIds updates the list of ids in the observer and updates the subscription
// IMPORTANT: this function is not thread-safe because of recBatch add is not under the lock and should be called only sequentially
func (c *collectionObserver) updateIds(ids []string) {
	c.lock.Lock()

	removed, added := slice.DifferenceRemovedAdded(c.ids, ids)
	for _, id := range removed {
		delete(c.idsSet, id)
	}
	for _, id := range added {
		c.idsSet[id] = struct{}{}
	}
	c.ids = ids
	c.lock.Unlock()
	entries := c.spaceSubscription.fetchEntriesLocked(append(removed, added...))
	for _, e := range entries {
		err := c.recBatch.Add(context.Background(), database.Record{
			Details: e.data,
		})
		if err != nil {
			log.Info("failed to add entities to mb: ", err)
		}
	}
}

func (c *collectionObserver) FilterObject(g *domain.Details) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	id := g.GetString(bundle.RelationKeyId)
	_, ok := c.idsSet[id]
	return ok
}

// AnystoreSort called only once when subscription is created
// TODO make collectionObserver to satify query.Filter interface
func (c *collectionObserver) AnystoreFilter() query.Filter {
	c.lock.RLock()
	defer c.lock.RUnlock()
	arena := &anyenc.Arena{}
	values := make([]*anyenc.Value, 0, len(c.idsSet))
	for id := range c.idsSet {
		aev := domain.String(id).ToAnyEnc(arena)
		values = append(values, aev)
	}
	filter := query.NewInValue(values...)
	return query.Key{
		Path:   []string{bundle.RelationKeyId.String()},
		Filter: filter,
	}
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

func (c *collectionSub) getActiveRecords() (res []*domain.Details) {
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

func (c *collectionSub) reorder(ctx *opCtx, depDetails []*domain.Details) {
	c.sortedSub.reorder(ctx, depDetails)
}

func (s *spaceSubscriptions) newCollectionSub(req SubscribeRequest, f *database.Filters, filterDepIds []string) (*collectionSub, error) {
	obs, err := s.newCollectionObserver(req.SpaceId, req.CollectionId, req.SubId)
	if err != nil {
		return nil, err
	}
	if f.FilterObj == nil {
		f.FilterObj = obs
	} else {
		f.FilterObj = database.FiltersAnd{obs, f.FilterObj}
	}

	ssub := s.newSortedSub(req.SubId, slice.StringsInto[domain.RelationKey](req.Keys), f.FilterObj, f.Order, int(req.Limit), int(req.Offset))
	ssub.disableDep = req.NoDepSubscription
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
		if f.FilterObj.FilterObject(e.data) {
			filtered = append(filtered, e)
		}
	}

	if req.Sorts != nil {
		s.ds.enregisterObjectSorts(ssub.id, req.Sorts)
	}

	if err = ssub.init(filtered); err != nil {
		return nil, err
	}
	return sub, nil
}

func (s *spaceSubscriptions) fetchEntriesLocked(ids []string) []*entry {
	s.m.Lock()
	defer s.m.Unlock()
	return s.fetchEntries(ids)
}

func (s *spaceSubscriptions) fetchEntries(ids []string) []*entry {
	res := make([]*entry, 0, len(ids))
	missingIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if e := s.cache.Get(id); e != nil {
			res = append(res, e)
			continue
		}
		missingIDs = append(missingIDs, id)
	}

	if len(missingIDs) == 0 {
		return res
	}
	recs, err := s.objectStore.QueryByIds(missingIDs)
	if err != nil {
		log.Error("can't query by ids:", err)
	}
	for _, r := range recs {
		e := newEntry(r.Details.GetString(bundle.RelationKeyId), r.Details)
		res = append(res, e)
	}
	return res
}
