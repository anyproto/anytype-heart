package unsplash

import (
	"sync"
	"time"
)

type entry struct {
	t       time.Time
	results []Result
}

type cache struct {
	sync.Mutex
	entries     map[string]*entry
	lastResults []Result
	ttl         time.Duration
}

func (c *cache) set(k string, results []Result) {
	c.Lock()
	defer c.Unlock()
	c.lastResults = results
	c.entries[k] = &entry{results: results, t: time.Now()}
}

func (c *cache) get(k string) []Result {
	c.Lock()
	defer c.Unlock()
	e, contains := c.entries[k]
	if !contains || time.Now().Sub(e.t) > c.ttl {
		return nil
	}
	return e.results
}

func (c *cache) getLast() []Result {
	c.Lock()
	defer c.Unlock()

	return c.lastResults
}

func newCacheWithTTL(ttl time.Duration) *cache {
	return &cache{
		entries: map[string]*entry{},
		ttl:     ttl,
	}
}
