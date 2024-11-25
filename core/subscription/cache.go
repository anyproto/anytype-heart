package subscription

import (
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

	subIds             []string
	subIsActive        []bool
	subFullDetailsSent []bool
}

// SetSub marks provided subscription for the entry as active (within the current pagination window) or inactive
func (e *entry) SetSub(subId string, isActive bool, isFullDetailSent bool) {
	if pos := slice.FindPos(e.subIds, subId); pos == -1 {
		e.subIds = append(e.subIds, subId)
		e.subIsActive = append(e.subIsActive, isActive)
		e.subFullDetailsSent = append(e.subFullDetailsSent, isFullDetailSent)
	} else {
		e.subIsActive[pos] = isActive
		e.subFullDetailsSent[pos] = isFullDetailSent
	}
}

// IsActive indicates that entry is inside the current pagination window for all of provided subscription IDs
func (e *entry) IsActive(subIds ...string) bool {
	if len(subIds) == 0 {
		return false
	}
	for _, id := range subIds {
		if pos := slice.FindPos(e.subIds, id); pos != -1 {
			if !e.subIsActive[pos] {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

// IsFullDetailsSent that in the context of ALL provided subscriptions we have previously sent the full ObjectSetDetails event
// if true this means that we can send incremental diff updates
func (e *entry) IsFullDetailsSent(subIds ...string) bool {
	if len(subIds) == 0 {
		return false
	}
	for _, id := range subIds {
		if pos := slice.FindPos(e.subIds, id); pos != -1 {
			if !e.subFullDetailsSent[pos] {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

func (e *entry) RemoveSubId(subId string) {
	if pos := slice.FindPos(e.subIds, subId); pos != -1 {
		e.subIds = slice.RemoveMut(e.subIds, subId)
		e.subIsActive = append(e.subIsActive[:pos], e.subIsActive[pos+1:]...)
		e.subFullDetailsSent = append(e.subFullDetailsSent[:pos], e.subFullDetailsSent[pos+1:]...)
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
		return res
	}
	c.entries[e.id] = e
	return e
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
