package ocache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"go.uber.org/zap"
)

var (
	ErrClosed = errors.New("object cache closed")
	ErrExists = errors.New("object exists")
)

var (
	defaultTTL = time.Minute
	defaultGC  = 20 * time.Second
)

var log = logging.Logger("anytype-mw-ocache")

type LoadFunc func(ctx context.Context, id string) (value Object, err error)

type Option func(*oCache)

var WithLogServiceName = func(name string) Option {
	return func(cache *oCache) {
		cache.log = cache.log.With("service_name", name)
	}
}

var WithTTL = func(ttl time.Duration) Option {
	return func(cache *oCache) {
		cache.ttl = ttl
	}
}

var WithGCPeriod = func(gcPeriod time.Duration) Option {
	return func(cache *oCache) {
		cache.gc = gcPeriod
	}
}

func New(loadFunc LoadFunc, opts ...Option) OCache {
	c := &oCache{
		data:     make(map[string]*entry),
		loadFunc: loadFunc,
		timeNow:  time.Now,
		ttl:      defaultTTL,
		gc:       defaultGC,
		closeCh:  make(chan struct{}),
		log:      log.With(),
	}
	for _, o := range opts {
		o(c)
	}
	go c.ticker()
	return c
}

type Object interface {
	ShouldClose() bool
	Close() (err error)
}

type entry struct {
	id        string
	lastUsage time.Time
	refCount  uint32
	load      chan struct{}
	loadErr   error
	value     Object
}

type OCache interface {
	// Get gets an object from cache or create a new one via 'loadFunc'
	// Increases the object refs counter on successful
	// When 'loadFunc' returns a non-nil error, an object will not be stored to cache
	Get(ctx context.Context, id string) (value Object, err error)
	// Add adds new object to cache
	// Returns error when object exists
	Add(id string, value Object) (err error)
	// Release decreases the refs counter
	Release(id string) bool
	// Reset sets refs counter to 0
	Reset(id string) bool
	// ForEach iterates over all loaded objects, breaks when callback returns false
	ForEach(f func(v Object) (isContinue bool))
	// GC frees not used and expired objects
	// Will automatically called every 'gcPeriod'
	GC()
	// Len returns current cache size
	Len() int
	// Close closes all objects and cache
	Close() (err error)
}

type oCache struct {
	mu       sync.Mutex
	data     map[string]*entry
	loadFunc LoadFunc
	timeNow  func() time.Time
	ttl      time.Duration
	gc       time.Duration
	closed   bool
	closeCh  chan struct{}
	log      *zap.SugaredLogger
}

func (c *oCache) Get(ctx context.Context, id string) (value Object, err error) {
	var (
		e    *entry
		ok   bool
		load bool
	)
	c.mu.Lock()
	if c.closed {
		return nil, ErrClosed
	}
	if e, ok = c.data[id]; !ok {
		load = true
		e = &entry{
			id:   id,
			load: make(chan struct{}),
		}
		c.data[id] = e
	}
	e.lastUsage = c.timeNow()
	e.refCount++
	c.mu.Unlock()

	if load {
		c.load(ctx, id, e)
	}
	<-e.load
	return e.value, e.loadErr
}

func (c *oCache) load(ctx context.Context, id string, e *entry) {
	defer close(e.load)
	value, err := c.loadFunc(ctx, id)

	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		e.loadErr = err
		delete(c.data, id)
	} else {
		e.value = value
	}
}

func (c *oCache) Release(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	if e, ok := c.data[id]; ok {
		if e.refCount > 0 {
			e.refCount--
			return true
		}
	}
	return false
}

func (c *oCache) Reset(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	if e, ok := c.data[id]; ok {
		e.refCount = 0
		return true
	}
	return false
}

func (c *oCache) Add(id string, value Object) (err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data[id]; ok {
		return ErrExists
	}
	e := &entry{
		id:        id,
		lastUsage: time.Now(),
		refCount:  0,
		load:      make(chan struct{}),
		value:     value,
	}
	close(e.load)
	c.data[id] = e
	return
}

func (c *oCache) ForEach(f func(obj Object) (isContinue bool)) {
	var objects []Object
	c.mu.Lock()
	for _, v := range c.data {
		select {
		case <-v.load:
			if v.value != nil {
				objects = append(objects, v.value)
			}
		default:
		}
	}
	c.mu.Unlock()
	for _, obj := range objects {
		if !f(obj) {
			return
		}
	}
}

func (c *oCache) ticker() {
	ticker := time.NewTicker(c.gc)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.GC()
		case <-c.closeCh:
			return
		}
	}
}

func (c *oCache) GC() {
	c.mu.Lock()
	if c.closed {
		return
	}
	deadline := c.timeNow().Add(-c.ttl)
	var toClose []*entry
	for k, e := range c.data {
		if e.refCount <= 0 && e.lastUsage.Before(deadline) && e.value.ShouldClose() {
			delete(c.data, k)
			toClose = append(toClose, e)
		}
	}
	size := len(c.data)
	c.mu.Unlock()
	c.log.Infof("GC: removed %d; cache size: %d", len(toClose), size)
	for _, e := range toClose {
		<-e.load
		if e.value != nil {
			if err := e.value.Close(); err != nil {
				c.log.With("object_id", e.id).Warnf("GC: object close error: %v", err)
			}
		}
	}
}

func (c *oCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.data)
}

func (c *oCache) Close() (err error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	c.closed = true
	close(c.closeCh)
	var toClose []*entry
	for _, e := range c.data {
		toClose = append(toClose, e)
	}
	c.mu.Unlock()
	for _, e := range toClose {
		<-e.load
		if e.value != nil {
			if clErr := e.value.Close(); clErr != nil {
				c.log.With("object_id", e.id).Warnf("cache close: object close error: %v", clErr)
			}
		}
	}
	return nil
}
