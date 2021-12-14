package subscription

import (
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
)

func newCache() *cache {
	return &cache{
		entries: map[string]*entry{},
	}
}

type entry struct {
	id   string
	data *types.Struct

	subIds   []string
	isActive bool
}

func (e *entry) AddSubId(subId string, isActive bool) {
	if slice.FindPos(e.subIds, subId) == -1 {
		e.subIds = append(e.subIds, subId)
	}
	if isActive {
		e.isActive = true
	}
}

func (e *entry) IsActive() bool {
	return e.isActive
}

func (e *entry) RemoveSubId(subId string) {
	e.subIds = slice.Remove(e.subIds, subId)
}

func (e *entry) SubIds() []string {
	return e.subIds
}

func (e *entry) Get(key string) *types.Value {
	return e.data.Fields[key]
}

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