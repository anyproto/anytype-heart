package meta

import (
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

func newCollector(ps *pubSub, id string) *collector {
	c := &collector{
		blockId:  id,
		ps:       ps,
		ready:    make(chan struct{}),
		wakeUpCh: make(chan struct{}),
		quit:     make(chan struct{}),
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
	wakeUpCh chan struct{}
	quit     chan struct{}
	closed   bool
	s        source.Source
	doc      state.Doc
}

func (c *collector) StateAppend(f func(d state.Doc) (s *state.State, err error)) error {
	s, err := f(c.doc)
	if err != nil {
		return err
	}
	_, _, err = state.ApplyState(s, false)
	if err != nil {
		return err
	}
	log.Infof("changes: details stateAppend")
	c.updateMeta()
	return nil
}

func (c *collector) StateRebuild(doc state.Doc) (err error) {
	c.doc = doc
	log.Infof("changes: details stateRebuild")
	c.updateMeta()
	return nil
}

func (c *collector) updateMeta() {
	m := Meta{
		BlockId: c.blockId,
		SmartBlockMeta: core.SmartBlockMeta{
			ObjectTypes: c.doc.ObjectTypes(),
			Relations:   c.doc.ExtraRelations(),
			Details:     c.doc.CombinedDetails(),
		},
	}
	if !c.lastMeta.Details.Equal(m.Details) || !slice.SortedEquals(c.lastMeta.ObjectTypes, m.ObjectTypes) || !pbtypes.RelationsEqual(c.lastMeta.Relations, m.Relations) {
		c.ps.call(m)
		c.lastMeta = m
	}
}

func (c *collector) GetMeta() (d Meta) {
	<-c.ready
	c.m.Lock()
	defer c.m.Unlock()
	return c.lastMeta
}

func (c *collector) setMeta(m Meta) {
	c.m.Lock()
	defer c.m.Unlock()
	if !c.lastMeta.Details.Equal(m.Details) || !slice.SortedEquals(c.lastMeta.ObjectTypes, m.ObjectTypes) || !pbtypes.RelationsEqual(c.lastMeta.Relations, m.Relations) {
		c.ps.call(m)
		c.lastMeta = m
	}
}

func (c *collector) fetchInitialMeta() (err error) {
	c.m.Lock()
	defer c.m.Unlock()
	if c.s != nil {
		c.s.Close()
	}
	c.s, err = c.ps.newSource(c.blockId, true)
	if err != nil {
		return err
	}
	c.doc, err = c.s.ReadMeta(c)
	if err != nil {
		return err
	}
	c.lastMeta = Meta{
		BlockId: c.blockId,
		SmartBlockMeta: core.SmartBlockMeta{
			ObjectTypes: c.doc.ObjectTypes(),
			Relations:   c.doc.ExtraRelations(),
			Details:     c.doc.CombinedDetails(),
		},
	}
	return nil
}

func (c *collector) wakeUp() {
	c.m.Lock()
	if c.wakeUpCh != nil {
		close(c.wakeUpCh)
		c.wakeUpCh = nil
	}
	c.m.Unlock()
}

func (c *collector) fetchMeta() {
	defer func() {
		select {
		case <-c.ready:
		default:
			close(c.ready)
		}
	}()
	var i time.Duration
	for {
		if err := c.fetchInitialMeta(); err != nil {
			i++
			wait := time.Second * i
			log.Infof("meta: %s: can't fetch initial meta: %v; - retry after %v", c.blockId, err, wait)
			c.m.Lock()
			wuCh := c.wakeUpCh
			c.m.Unlock()
			select {
			case <-wuCh:
			case <-time.After(wait):
			case <-c.quit:
				return
			}
			continue
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

func (c *collector) Lock() {
	c.m.Lock()
}

func (c *collector) Unlock() {
	c.m.Unlock()
}
