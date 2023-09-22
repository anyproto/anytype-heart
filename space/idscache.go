package space

import (
	"sync"
)

type idCache struct {
	ids map[string]SpaceParams
	sync.RWMutex
}

func newCache() *idCache {
	return &idCache{
		ids: make(map[string]SpaceParams),
	}
}

func (c *idCache) Get(spaceID string) (params SpaceParams, ok bool) {
	c.RLock()
	defer c.RUnlock()
	params, ok = c.ids[spaceID]
	return
}

func (c *idCache) Set(spaceID string, params SpaceParams) {
	c.Lock()
	defer c.Unlock()
	c.ids[spaceID] = params
}
