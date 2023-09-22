package space

import (
	"sync"
)

type idCache struct {
	ids map[string]spaceParams
	sync.RWMutex
}

func newCache() *idCache {
	return &idCache{
		ids: make(map[string]spaceParams),
	}
}

func (c *idCache) Get(spaceID string) (params spaceParams, ok bool) {
	c.RLock()
	defer c.RUnlock()
	params, ok = c.ids[spaceID]
	return
}

func (c *idCache) Set(spaceID string, params spaceParams) {
	c.Lock()
	defer c.Unlock()
	c.ids[spaceID] = params
}
