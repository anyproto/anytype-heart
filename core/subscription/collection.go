package subscription

import (
	"fmt"
	"sync"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database/filter"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

type collectionContainer struct {
	onUpdate func(changes []slice.Change[string])
	filter   filter.Filter

	lock      *sync.RWMutex
	objectIDs []string
}

func newCollectionContainer(ids []string, onUpdate func(changes []slice.Change[string])) *collectionContainer {
	return &collectionContainer{
		onUpdate:  onUpdate,
		lock:      &sync.RWMutex{},
		objectIDs: ids,
	}
}

func (c *collectionContainer) List() []string {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.objectIDs
}

func (c *collectionContainer) Update(changes []slice.Change[string]) {
	c.lock.Lock()
	c.objectIDs = slice.ApplyChanges(c.objectIDs, changes, slice.StringIdentity[string])
	c.lock.Unlock()

	c.onUpdate(changes)
}

func (c *collectionContainer) Compare(a, b filter.Getter) int {
	c.lock.RLock()
	defer c.lock.RUnlock()

	ae, be := a.(*entry), b.(*entry)
	ap, bp := slice.FindPos(c.objectIDs, ae.id), slice.FindPos(c.objectIDs, be.id)
	if ap == bp {
		return 0
	}
	if ap > bp {
		return -1
	}
	return 1
}

func (c *collectionContainer) FilterObject(a filter.Getter) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return slice.FindPos(c.objectIDs, a.(*entry).id) >= 0
}

func (c *collectionContainer) String() string {
	return "collection order"
}

type collectionSub struct {
	id            string
	keys          []string
	col           *collectionContainer
	sendEvent     func(event *pb.Event)
	activeIDs     []string
	activeEntries []*entry
	dependentSub  *simpleSub

	cache       *cache
	objectStore objectstore.ObjectStore
	depService  *dependencyService
}

func (s *service) newCollectionSub(id string, collectionID string, keys []string, filter filter.Filter, order filter.Order, limit, offset int) (*collectionSub, error) {
	initialObjectIDs, changesCh, err := s.collections.SubscribeForCollection(collectionID)
	if err != nil {
		return nil, fmt.Errorf("subscribe for collection: %w", err)
	}
	col := newCollectionContainer(initialObjectIDs, nil)
	col.filter = filter

	sub := &collectionSub{
		id:          id,
		keys:        keys,
		col:         col,
		sendEvent:   s.sendEvent,
		cache:       s.cache,
		objectStore: s.objectStore,
		depService:  s.ds,
	}
	col.onUpdate = sub.onCollectionUpdate

	go func() {
		for ch := range changesCh {
			col.Update(ch)
		}
	}()
	return sub, nil
}

func (c *collectionSub) init(entries []*entry) (err error) {
	ids := c.col.List()
	c.activeIDs = ids
	newEntries := c.fetchEntries(ids)
	for _, e := range newEntries {
		// TODO Filter
		e.SetSub(c.id, true)
	}
	c.activeEntries = newEntries
	return nil
}

func (c *collectionSub) counters() (prev, next int) {
	// TODO
	return
}

func (c *collectionSub) onChange(ctx *opCtx) {
	// TODO Handle all cases, not only updating. Consider having a filter or sorting
	for _, e := range ctx.entries {
		if idx := slice.FindPos(c.activeIDs, e.id); idx >= 0 {
			cur := c.cache.Get(e.id)
			if cur != nil {
				c.activeEntries[idx] = cur
				cur.SetSub(c.id, true)
				ctx.change = append(ctx.change, opChange{
					id:    e.id,
					subId: c.id,
					keys:  c.keys,
				})
			}
		}
	}
}

func (c *collectionSub) fetchEntries(ids []string) []*entry {
	res := make([]*entry, 0, len(ids))
	for _, id := range ids {
		if e := c.cache.Get(id); e != nil {
			res = append(res, e)
			continue
		}
		// TODO query in one batch
		recs, err := c.objectStore.QueryById([]string{id})
		if err != nil {
			// TODO proper logging
			fmt.Println("query new entry:", err)
		}
		if len(recs) > 0 {
			e := &entry{
				id:   id,
				data: recs[0].Details,
			}
			c.cache.Set(e)
			res = append(res, e)
		}
	}
	return res
}

func (c *collectionSub) onCollectionUpdate(changes []slice.Change[string]) {
	c.activeIDs = slice.ApplyChanges(c.activeIDs, changes, slice.StringIdentity[string])

	newEntries := c.fetchEntries(c.activeIDs)
	for _, e := range newEntries {
		// TODO Filter
		e.SetSub(c.id, true)
	}
	ctx := &opCtx{
		entries: newEntries,
		c:       c.cache,
	}

	for _, ch := range changes {
		if add := ch.Add(); add != nil {
			afterID := add.AfterID
			for _, id := range add.Items {
				ctx.position = append(ctx.position, opPosition{
					id:      id,
					subId:   c.id,
					afterId: afterID,
					keys:    c.keys,
					isAdd:   true,
				})
				// Update afterID to save correspondence between subscription changes and generic atomic changes
				// The difference is that generic atomic changes contains a slice, so we need to update insertion position
				afterID = id
			}
			continue
		}

		if rm := ch.Remove(); rm != nil {
			for _, id := range rm.IDs {
				e := c.cache.Get(id)
				if e != nil {
					e.RemoveSubId(c.id)
				}
				ctx.remove = append(ctx.remove, opRemove{
					id:    id,
					subId: c.id,
				})
			}
			continue
		}

		if mv := ch.Move(); mv != nil {
			afterID := mv.AfterID
			for _, id := range mv.IDs {
				ctx.position = append(ctx.position, opPosition{
					id:      id,
					subId:   c.id,
					afterId: afterID,
					keys:    c.keys,
				})
				// Update afterID to save correspondence between subscription changes and generic atomic changes
				// The difference is that generic atomic changes contains a slice, so we need to update moving position
				afterID = id
			}
			continue
		}
	}

	ev := ctx.apply()
	c.activeEntries = newEntries
	c.sendEvent(ev)
}

func (c *collectionSub) getActiveRecords() (res []*types.Struct) {
	// TODO decide where to filter and reorder records. Here or in onChange?
	// TODO check how this method works in simpleSub
	for _, e := range c.activeEntries {
		res = append(res, e.data)
	}
	return
}

func (c *collectionSub) hasDep() bool {
	// TODO
	return false
}

func (c *collectionSub) close() {
	// TODO
}
