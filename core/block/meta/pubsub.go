package meta

import (
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
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
	ps := &pubSub{
		subscribers: make(map[string]map[Subscriber]struct{}),
		collectors:  make(map[string]*collector),
		lastUsage:   make(map[string]time.Time),
		anytype:     a,
	}
	go ps.ticker()
	return ps
}

type pubSub struct {
	anytype     anytype.Service
	subscribers map[string]map[Subscriber]struct{}
	collectors  map[string]*collector
	lastUsage   map[string]time.Time
	m           sync.Mutex
	closed      bool
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
		p.lastUsage[id] = time.Now()
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
		p.lastUsage[id] = time.Now()
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
	for id, sm := range p.subscribers {
		if _, ok := sm[s]; ok {
			p.lastUsage[id] = time.Now()
			delete(sm, s)
		}
	}
}

func (p *pubSub) call(d Meta) {
	p.m.Lock()
	defer p.m.Unlock()
	if p.closed {
		return
	}
	ss := p.subscribers[d.BlockId]
	if ss != nil {
		for s := range ss {
			s.call(d)
		}
	}
}

func (p *pubSub) ticker() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for tm := range ticker.C {
		if !p.cleanup(tm) {
			return
		}
	}
}

func (p *pubSub) cleanup(now time.Time) bool {
	p.m.Lock()
	defer p.m.Unlock()
	if p.closed {
		return false
	}
	var deadLine = now.Add(-5 * time.Minute)
	for id, lastUsage := range p.lastUsage {
		if p.subscribers[id] != nil && len(p.subscribers[id]) > 0 {
			continue
		}
		if lastUsage.Before(deadLine) {
			p.collectors[id].close()
			delete(p.collectors, id)
			delete(p.lastUsage, id)
			delete(p.subscribers, id)
		}
	}
	return true
}

func (p *pubSub) Close() error {
	p.m.Lock()
	defer p.m.Unlock()
	for _, c := range p.collectors {
		c.close()
	}
	p.closed = true
	return nil
}

func (p *pubSub) createCollector(id string) {
	p.collectors[id] = newCollector(p, id)
}

func (p *pubSub) removeCollector(id string) {
	if c, ok := p.collectors[id]; ok {
		c.close()
		delete(p.collectors, id)
	}
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

func newCollector(ps *pubSub, id string) *collector {
	c := &collector{
		blockId: id,
		ps:      ps,
		ready:   make(chan struct{}),
		quit:    make(chan struct{}),
	}
	go c.listener()
	return c
}

type collector struct {
	blockId  string
	lastMeta Meta
	ready    chan struct{}
	m        sync.Mutex
	ps       *pubSub
	quit     chan struct{}
	closed   bool
}

func (c *collector) GetMeta() (d Meta) {
	<-c.ready
	c.m.Lock()
	defer c.m.Unlock()
	return c.lastMeta
}

func (c *collector) setMeta(d Meta) {
	c.m.Lock()
	var changed bool
	if changed = !c.lastMeta.Details.Equal(d); changed {
		c.lastMeta = d
	}
	c.m.Unlock()
	if changed {
		c.ps.call(d)
	}
}

func (c *collector) listener() {
	defer func() {
		select {
		case <-c.ready:
			return
		default:
			close(c.ready)
		}
	}()
	sb, err := c.ps.anytype.GetBlock(c.blockId)
	if err != nil {
		return
	}

	ss, err := sb.GetLastSnapshot()
	if err != nil {
		return
	}
	meta, err := ss.Meta()
	if err != nil {
		return
	} else {
		c.m.Lock()
		c.lastMeta = Meta{
			BlockId:        c.blockId,
			SmartBlockMeta: *meta,
		}
		c.m.Unlock()
		close(c.ready)
	}
	state := ss.State()
	var ch = make(chan core.SmartBlockMetaChange)
	cancel, err := sb.SubscribeForMetaChanges(state, ch)
	if err != nil {
		return
	}
	for {
		select {
		case meta, ok := <-ch:
			if ! ok {
				return
			}
			c.setMeta(Meta{
				BlockId:        c.blockId,
				SmartBlockMeta: meta.SmartBlockMeta,
			})
		case <-c.quit:
			cancel()
		}
	}
}

func (c *collector) close() {
	c.m.Lock()
	defer c.m.Unlock()
	if c.closed {
		return
	}
	close(c.quit)
	c.closed = true
}
