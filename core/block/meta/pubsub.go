package meta

import (
	"errors"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
)

var log = logging.Logger("anytype-mw-service")

func metaError(e string) *core.SmartBlockMeta {
	return &core.SmartBlockMeta{Details: &types.Struct{
		Fields: map[string]*types.Value{
			"error": pbtypes.String(e),
		},
	}}
}

var (
	notFoundMeta = metaError("not_found")
	errNotFound  = errors.New("not found")
	errEmpty     = errors.New("empty")
)

type PubSub interface {
	NewSubscriber() Subscriber
}

type Subscriber interface {
	Subscribe(ids ...string) Subscriber
	ReSubscribe(ids ...string) Subscriber
	Unsubscribe(ids ...string) Subscriber
	Callback(f func(d Meta)) Subscriber
	Close()
}

func newPubSub(a anytype.Service, fetcher func(id string) (m Meta, err error)) *pubSub {
	ps := &pubSub{
		subscribers: make(map[string]map[Subscriber]struct{}),
		collectors:  make(map[string]*collector),
		lastUsage:   make(map[string]time.Time),
		anytype:     a,
		fetcher:     fetcher,
	}
	go ps.ticker()
	return ps
}

type pubSub struct {
	anytype     anytype.Service
	subscribers map[string]map[Subscriber]struct{}
	collectors  map[string]*collector
	lastUsage   map[string]time.Time
	fetcher     func(id string) (m Meta, err error)
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
		go func(id string) {
			p.m.Lock()
			cl, ok := p.collectors[id]
			p.m.Unlock()
			if ok {
				s.(*subscriber).call(cl.GetMeta())
			}
		}(id)
	}
}

func (p *pubSub) reSubscribe(s Subscriber, ids ...string) {
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
		if _, ok := sm[s]; !ok {
			go func(id string) {
				p.m.Lock()
				cl, ok := p.collectors[id]
				p.m.Unlock()
				if ok {
					s.(*subscriber).call(cl.GetMeta())
				}
			}(id)
			sm[s] = struct{}{}
		}
	}
	for id, sm := range p.subscribers {
		if _, ok := sm[s]; ok {
			if slice.FindPos(ids, id) == -1 {
				delete(sm, s)
			}
		}
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
		if _, ok := sm[s]; ok {
			delete(sm, s)
		}
	}
}

func (p *pubSub) call(d Meta) {
	if p.closed {
		return
	}
	d = copyMeta(d)
	ss := p.subscribers[d.BlockId]
	if ss != nil {
		for s := range ss {
			go s.(*subscriber).call(d)
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
	var total, removed int
	var deadLine = now.Add(-2 * time.Minute)
	for id, lastUsage := range p.lastUsage {
		total++
		if p.subscribers[id] == nil || len(p.subscribers[id]) > 0 {
			continue
		}
		if lastUsage.Before(deadLine) {
			p.collectors[id].close()
			delete(p.collectors, id)
			delete(p.lastUsage, id)
			delete(p.subscribers, id)
			removed++
		}
	}
	log.Infof("meta pubsub cleanup: %d removed (from %d)", removed, total)
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

func (s *subscriber) ReSubscribe(ids ...string) Subscriber {
	s.ps.reSubscribe(s, ids...)
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
	log.Infof("metaListener started: %v", id)
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
	defer c.m.Unlock()
	if !c.lastMeta.Details.Equal(d.Details) {
		c.ps.call(d)
		c.lastMeta = d
	}
}

func (c *collector) fetchInitialMeta() (err error) {
	defer func() {
		if err == errEmpty {
			return
		}

		select {
		case <-c.ready:
			return
		default:
			close(c.ready)
		}
	}()
	setCurrentMeta := func(meta core.SmartBlockMeta) {
		c.m.Lock()
		c.lastMeta = Meta{
			BlockId:        c.blockId,
			SmartBlockMeta: meta,
		}
		c.m.Unlock()
	}
	meta, err := c.ps.fetcher(c.blockId)
	if err != nil {
		setCurrentMeta(*notFoundMeta)
		return errNotFound
	}
	setCurrentMeta(core.SmartBlockMeta{
		Details: meta.Details,
	})
	return nil
}

func (c *collector) listener() {
	for {
		err := c.fetchInitialMeta()
		if err != nil {
			if err == errNotFound {
				log.Infof("meta: %s: block not found - listener exit", c.blockId)
				return
			}
			log.Infof("meta: %s: can't fetch initial meta: %v; - retry", c.blockId, err)
			time.Sleep(time.Second * 5)
			continue
		}
		var ch = make(chan core.SmartBlockMetaChange)
		for {
			select {
			case meta := <-ch:
				c.setMeta(Meta{
					BlockId:        c.blockId,
					SmartBlockMeta: meta.SmartBlockMeta,
				})
			case <-c.quit:
				return
			}
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

func copyMeta(m Meta) Meta {
	d := m.Details
	if d != nil {
		d = pbtypes.CopyStruct(m.Details)
	}
	return Meta{
		BlockId:
		m.BlockId,
		SmartBlockMeta: core.SmartBlockMeta{
			Details: d,
		},
	}
}
