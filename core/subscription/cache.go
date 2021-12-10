package subscription

import (
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

	refs int
}

func (e *entry) Get(key string) *types.Value {
	return e.data.Fields[key]
}

type cache struct {
	entries map[string]*entry
}

func (c *cache) get(id string) *entry {
	if e := c.entries[id]; e != nil {
		e.refs++
		return e
	}
	return nil
}

func (c *cache) release(id string) {
	if e := c.entries[id]; e != nil {
		e.refs--
		if e.refs == 0 {
			delete(c.entries, id)
		}
	}
}

func (c *cache) pick(id string) *entry {
	return c.entries[id]
}

func (c *cache) exists(id string) bool {
	_, ok := c.entries[id]
	return ok
}

func (c *cache) getOrSet(e *entry) *entry {
	if !c.exists(e.id) {
		c.set(e)
	}
	return c.get(e.id)
}

func (c *cache) set(e *entry) {
	if ex, ok := c.entries[e.id]; ok {
		ex.data = e.data
		ex.refs += e.refs
	} else {
		c.entries[e.id] = e
	}
}
