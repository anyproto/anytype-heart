package meta

import (
	"sync"

	"github.com/anytypeio/go-anytype-middleware/core/anytype"
)

const (
	cacheSize = 1000
)

type PubSub interface {
	NewSubscriber() Subscriber
}

type Subscriber interface {
	Subscribe(ids ...string) Subscriber
	Unsubscribe(ids ...string) Subscriber
	Callback(f func(d Meta)) Subscriber
	Close()

	call(m Meta)
}

func newPubSub(a anytype.Service) *pubSub {
	return &pubSub{
		subscribers: make(map[string]map[Subscriber]struct{}),
		collectors:  make(map[string]*collector),
	}
}

type pubSub struct {
	anytype     anytype.Service
	subscribers map[string]map[Subscriber]struct{}
	collectors  map[string]*collector
	m           sync.Mutex
}

func (p *pubSub) NewSubscriber() Subscriber {
	return &subscriber{
		ps: p,
	}
}

func (p *pubSub) add(s Subscriber, ids ...string) {
	p.m.Lock()
	defer p.m.Unlock()
	for _, id := range ids {
		sm, ok := p.subscribers[id]
		if !ok {
			p.createCollector(id)
			sm = make(map[Subscriber]struct{})
			p.subscribers[id] = sm
		}
		sm[s] = struct{}{}
		go s.call(p.collectors[id].GetMeta())
	}
}

func (p *pubSub) remove(s Subscriber, ids ...string) {
	p.m.Lock()
	defer p.m.Unlock()
	for _, id := range ids {
		sm, ok := p.subscribers[id]
		if !ok {
			continue
		}
		delete(sm, s)
	}
}

func (p *pubSub) removeAll(s Subscriber) {
	p.m.Lock()
	defer p.m.Unlock()
	for _, sm := range p.subscribers {
		delete(sm, s)
	}
}

func (p *pubSub) call(d Meta) {
	p.m.Lock()
	defer p.m.Unlock()
	ss := p.subscribers[d.BlockId]
	if ss != nil {
		for s := range ss {
			s.call(d)
		}
	}
}

func (p *pubSub) Close() error {
	return nil
}

func (p *pubSub) createCollector(id string) {

}

func (p *pubSub) removeCollector(id string) {

}

func (p *pubSub) setMeta(d Meta) {
	p.m.Lock()
	defer p.m.Unlock()
	if c, ok := p.collectors[d.BlockId]; ok {
		c.setMeta(d)
	}
}

type subscriber struct {
	ps *pubSub
	cb func(d Meta)
}

func (s *subscriber) call(m Meta) {
	if s.cb != nil {
		s.cb(m)
	}
}

func (s *subscriber) Subscribe(ids ...string) Subscriber {
	s.ps.add(s, ids...)
	return s
}

func (s *subscriber) Unsubscribe(ids ...string) Subscriber {
	s.ps.remove(s, ids...)
	return s
}

func (s *subscriber) Callback(cb func(d Meta)) Subscriber {
	s.cb = cb
	return s
}

func (s *subscriber) Close() {
	s.ps.removeAll(s)
	return
}

type collector struct {
	blockId  string
	lastMeta Meta
	ready    chan struct{}
	m        sync.Mutex
	ps       *pubSub
}

func (c *collector) GetMeta() (d Meta) {
	<-c.ready
	c.m.Lock()
	defer c.m.Unlock()
	return c.lastMeta
}

func (c *collector) setMeta(d Meta) {
	c.m.Lock()
	defer c.m.Unlock()
	c.onMetaChange(d)
}

func (c *collector) onMetaChange(d Meta) {
	if !c.lastMeta.Details.Equal(d) {
		c.lastMeta = d
		c.ps.call(d)
	}
}

func (c *collector) listener() {

}
