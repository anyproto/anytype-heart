package resolver

import (
	"context"
	"net"
	"sync"
	"time"

	dns "github.com/multiformats/go-multiaddr-dns"
)

type entry struct {
	t     time.Time
	addrs []net.IPAddr
}

type cache struct {
	sync.Mutex
	entries map[string]*entry
	ttl     time.Duration
}

func (c *cache) set(k string, addrs []net.IPAddr) {
	c.Lock()
	defer c.Unlock()
	c.entries[k] = &entry{addrs: addrs, t: time.Now()}
}

func (c *cache) get(k string) []net.IPAddr {
	c.Lock()
	defer c.Unlock()
	e, contains := c.entries[k]
	if !contains || time.Now().Sub(e.t) > c.ttl {
		return nil
	}
	return e.addrs
}

func newCacheWithTTL(ttl time.Duration) *cache {
	return &cache{
		entries: map[string]*entry{},
		ttl:     ttl,
	}
}

type Resolver struct {
	c *cache
	r dns.BasicResolver
}

func NewResolverWithTTL(ttl time.Duration) *Resolver {
	return &Resolver{
		newCacheWithTTL(ttl),
		net.DefaultResolver,
	}
}

func (r *Resolver) LookupIPAddr(ctx context.Context, s string) ([]net.IPAddr, error) {
	res := r.c.get(s)
	if res != nil {
		return res, nil
	}

	res, err := r.r.LookupIPAddr(ctx, s)
	if err != nil {
		return nil, err
	}

	r.c.set(s, res)

	return res, nil
}

func (r *Resolver) LookupTXT(ctx context.Context, s string) ([]string, error) {
	return r.LookupTXT(ctx, s)
}
