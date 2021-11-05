package subscription

import (
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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
	return pbtypes.Get(e.data, key)
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

func (c *cache) set(e *entry) {
	// if entry exists - update only data
	if ex, ok := c.entries[e.id]; ok {
		ex.data = e.data
	} else {
		c.entries[e.id] = e
	}
}
