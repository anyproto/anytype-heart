package meta

import (
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
)

func newCollector(ps *pubSub, id string) *collector {
	c := &collector{
		blockId: id,
		ps:      ps,
		ready:   make(chan struct{}),
		quit:    make(chan struct{}),
	}
	go c.fetchMeta()
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
	s        source.Source
	doc      state.Doc
}

func (c *collector) StateAppend(f func(d state.Doc) (s *state.State, err error)) error {
	c.m.Lock()
	defer c.m.Unlock()
	s, err := f(c.doc)
	if err != nil {
		return err
	}
	_, _, err = state.ApplyState(s)
	if err != nil {
		return err
	}
	log.Infof("changes: details stateAppend")
	c.updateMeta()
	return nil
}

func (c *collector) StateRebuild(doc state.Doc) (err error) {
	c.m.Lock()
	defer c.m.Unlock()
	c.doc = doc
	log.Infof("changes: details stateRebuild")
	c.updateMeta()
	return nil
}

func (c *collector) updateMeta() {
	d := Meta{
		BlockId: c.blockId,
		SmartBlockMeta: core.SmartBlockMeta{
			Details: c.doc.Details(),
		},
	}
	if !c.lastMeta.Details.Equal(d.Details) {
		c.ps.call(d)
		c.lastMeta = d
	}
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
	c.m.Lock()
	defer c.m.Unlock()
	if c.s != nil {
		c.s.Close()
	}
	c.s, err = c.ps.newSource(c.blockId)
	if err != nil {
		return err
	}
	c.doc, err = c.s.ReadDetails(c)
	if err != nil {
		return err
	}
	c.lastMeta = Meta{
		BlockId: c.blockId,
		SmartBlockMeta: core.SmartBlockMeta{
			Details: c.doc.Details(),
		},
	}
	return nil
}

func (c *collector) fetchMeta() {
	var i time.Duration
	for {
		err := c.fetchInitialMeta()
		if err != nil {
			i++
			wait := time.Second * i
			log.Infof("meta: %s: can't fetch initial meta: %v; - retry after %v", c.blockId, err, wait)
			time.Sleep(wait)
			continue
		}
		select {
		case <-c.ready:
			return
		default:
			close(c.ready)
		}
		return
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
	if c.s != nil {
		c.s.Close()
	}
}
