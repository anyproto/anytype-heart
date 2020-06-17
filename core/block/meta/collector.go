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
	s        source.Source
	doc      state.Doc
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

	c.s, err = c.ps.newSource(c.blockId)
	if err != nil {
		setCurrentMeta(*notFoundMeta)
		return errNotFound
	}

	c.doc, err = c.s.ReadDetails(nil)
	if err != nil {
		setCurrentMeta(*notFoundMeta)
		return errNotFound
	}
	setCurrentMeta(core.SmartBlockMeta{
		Details: c.doc.Details(),
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
