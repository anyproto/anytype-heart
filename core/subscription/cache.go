package subscription

import (
	"maps"
	"slices"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/util/slice"
)

func newCache() *cache {
	return &cache{
		entries: map[string]*entry{},
	}
}

type entry struct {
	id   string
	data *domain.Details

	// subIsActive and subFullDetailsSent are allocated lazily on the first SetSub:
	// an entry is created for every record passing through the change pipeline,
	// and most of them never join any subscription
	subIds             []string
	subIsActive        map[string]bool
	subFullDetailsSent map[string]bool
}

func newEntry(id string, data *domain.Details) *entry {
	return &entry{id: id, data: data}
}

func (e *entry) Copy() *entry {
	return &entry{
		id:                 e.id,
		data:               e.data,
		subIds:             slices.Clone(e.subIds),
		subIsActive:        maps.Clone(e.subIsActive),
		subFullDetailsSent: maps.Clone(e.subFullDetailsSent),
	}
}

// SetSub marks provided subscription for the entry as active (within the current pagination window) or inactive
func (e *entry) SetSub(subId string, isActive bool, isFullDetailSent bool) {
	if pos := slice.FindPos(e.subIds, subId); pos == -1 {
		if e.subIsActive == nil {
			e.subIsActive = make(map[string]bool, 1)
			e.subFullDetailsSent = make(map[string]bool, 1)
		}
		e.subIds = append(e.subIds, subId)
		e.subIsActive[subId] = isActive
		e.subFullDetailsSent[subId] = isFullDetailSent
	} else {
		e.subIsActive[subId] = isActive
		// Don't override existing value, because if the event was already sent for the subscription during session, it should not be sent again
		if !e.subFullDetailsSent[subId] {
			e.subFullDetailsSent[subId] = isFullDetailSent
		}
	}
}

func (e *entry) IsInSub(subId string) bool {
	if e == nil {
		return false
	}
	for _, id := range e.subIds {
		if id == subId {
			return true
		}
	}
	return false
}

// GetActive returns all active subscriptions for entry
func (e *entry) GetActive() []string {
	var subIsActive []string
	for id, active := range e.subIsActive {
		if active {
			subIsActive = append(subIsActive, id)
		}
	}
	return subIsActive
}

// GetFullDetailsSent returns all subscriptions for entry, for which full details are already sent
func (e *entry) GetFullDetailsSent() []string {
	var detailsSent []string
	for id, isFullDetailsSent := range e.subFullDetailsSent {
		if isFullDetailsSent {
			detailsSent = append(detailsSent, id)
		}
	}
	return detailsSent
}

func (e *entry) RemoveSubId(subId string) {
	if pos := slice.FindPos(e.subIds, subId); pos != -1 {
		e.subIds = slice.RemoveMut(e.subIds, subId)
		delete(e.subIsActive, subId)
		delete(e.subFullDetailsSent, subId)
	}
}

func (e *entry) SubIds() []string {
	return e.subIds
}

// func (e *entry) Get(key string) *types.Value {
// 	return e.data.Fields[key]
// }

type cache struct {
	entries map[string]*entry
}

func (c *cache) Get(id string) *entry {
	return c.entries[id]
}

func (c *cache) GetOrSet(e *entry) *entry {
	if res, ok := c.entries[e.id]; ok {
		res.mergeMissingData(e.data)
		return res
	}
	c.entries[e.id] = e
	return e
}

// mergeMissingData adds keys absent in the entry data: a cached entry is
// projected to the keys of its subscriptions, while a newly subscribing one may
// need more of them. Existing values win, since cache content is kept current
// by change batches. Copy-on-write, so aliased readers of the old map are
// unaffected
func (e *entry) mergeMissingData(src *domain.Details) {
	if src == nil || e.data == nil {
		if e.data == nil {
			e.data = src
		}
		return
	}
	var merged *domain.Details
	for k, v := range src.Iterate() {
		if !e.data.Has(k) {
			if merged == nil {
				merged = e.data.Copy()
			}
			merged.Set(k, v)
		}
	}
	if merged != nil {
		e.data = merged
	}
}

func (c *cache) Set(e *entry) {
	c.entries[e.id] = e
}

func (c *cache) Remove(id string) {
	delete(c.entries, id)
}

func (c *cache) RemoveSubId(id, subId string) {
	if e := c.Get(id); e != nil {
		e.RemoveSubId(subId)
		if len(e.SubIds()) == 0 {
			c.Remove(id)
		}
	}
}
