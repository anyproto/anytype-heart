package subscription

import (
	"maps"

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
	subIsActive        map[string]bool
	subFullDetailsSent map[string]bool
}

func newEntry(id string, data *domain.Details) *entry {
	return &entry{id: id, data: data, subIsActive: make(map[string]bool), subFullDetailsSent: make(map[string]bool)}
}

func (e *entry) Copy() *entry {
	newSubIds := make([]string, len(e.subIds))
	copy(newSubIds, e.subIds)
	newSubIsActive := make(map[string]bool, len(e.subIsActive))
	maps.Copy(newSubIsActive, e.subIsActive)
	newSubFullDetailsSent := make(map[string]bool, len(e.subIsActive))
	maps.Copy(newSubFullDetailsSent, e.subFullDetailsSent)
	return &entry{
		id:                 e.id,
		data:               e.data,
		subIds:             newSubIds,
		subIsActive:        newSubIsActive,
		subFullDetailsSent: newSubFullDetailsSent,
	}
}

// SetSub marks provided subscription for the entry as active (within the current pagination window) or inactive
func (e *entry) SetSub(subId string, isActive bool, isFullDetailSent bool) {
	if pos := slice.FindPos(e.subIds, subId); pos == -1 {
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
